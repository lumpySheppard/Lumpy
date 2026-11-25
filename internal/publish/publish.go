// Package publish writes a status page coworkers can open without
// Lumpy: which drives exist, where they were last seen, how well each
// project is backed up, and which drives hold a given file.
package publish

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lumpysheppard/lumpy/internal/catalog"
	"github.com/lumpysheppard/lumpy/internal/volume"
)

// Settings say where the status page goes. They live in the library's
// settings table under "publish.".
type Settings struct {
	Enabled bool   `json:"enabled"`
	Title   string `json:"title"`
	// Folder is where index.html and status.json are written; empty
	// means a "status" folder next to the library. Point it at a synced
	// folder (Dropbox, iCloud Drive, a server share) and the page is
	// shared with nothing else to set up.
	Folder string `json:"folder"`
	// URL, when set, receives each file with an HTTP PUT to URL/<name>,
	// with Token as a bearer token when given.
	URL   string `json:"url"`
	Token string `json:"token"`
	// Command, when set, runs after the files are written, with the
	// folder in $LUMPY_STATUS_DIR: an rsync, scp or rclone line.
	Command string `json:"command"`
	// Interval is minutes between publishes while the app is open; 0
	// publishes only after jobs and when the app quits.
	Interval int `json:"interval"`
	// Files includes an index of every file so the page can say which
	// drives hold one.
	Files bool `json:"files"`
}

// Load reads the settings; a fresh library has publishing off.
func Load(db *catalog.DB) Settings {
	s := Settings{Title: "Lumpy status", Interval: 30, Files: true}
	if v := db.Setting("publish", ""); v != "" {
		json.Unmarshal([]byte(v), &s)
	}
	return s
}

// Save stores the settings.
func Save(db *catalog.DB, s Settings) error {
	b, _ := json.Marshal(s)
	return db.SetSetting("publish", string(b))
}

// Result is how the last publish went.
type Result struct {
	At      time.Time `json:"at"`
	OK      bool      `json:"ok"`
	Summary string    `json:"summary"`
	Path    string    `json:"path"` // the local index.html
}

// Last returns the last result, if any.
func Last(db *catalog.DB) *Result {
	v := db.Setting("publish.last", "")
	if v == "" {
		return nil
	}
	var r Result
	if json.Unmarshal([]byte(v), &r) != nil {
		return nil
	}
	return &r
}

func record(db *catalog.DB, r Result) {
	b, _ := json.Marshal(r)
	db.SetSetting("publish.last", string(b))
}

// DefaultFolder is where the page is written when no folder is chosen.
func DefaultFolder() string {
	p, err := catalog.DefaultPath()
	if err != nil {
		return "status"
	}
	return filepath.Join(filepath.Dir(p), "status")
}

// Status is what the page shows.
type Status struct {
	Title     string    `json:"title"`
	Generated time.Time `json:"generated"`
	Host      string    `json:"host"`
	Version   string    `json:"version"`
	Drives    []Drive   `json:"drives"`
	Projects  []Project `json:"projects"`
	// Files, when included, is one entry per file: [drive index, path,
	// size, copies on other drives (-1 when unknown)]. A compact form
	// keeps a 100,000-file library around a few megabytes.
	Files [][]any `json:"files,omitempty"`
	// FileCount is how many files there are, whether or not Files is set.
	FileCount int `json:"fileCount"`
}

type Drive struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Name      string    `json:"name"`
	Client    string    `json:"client,omitempty"`
	Project   string    `json:"project,omitempty"`
	Contact   string    `json:"contact,omitempty"`
	Notes     string    `json:"notes,omitempty"`
	Color     string    `json:"color,omitempty"`
	Connected bool      `json:"connected"`
	LastSeen  time.Time `json:"lastSeen"`
	Size      int64     `json:"size"`
	Free      int64     `json:"free"`
	Files     int       `json:"files"`
	Bytes     int64     `json:"bytes"`
	Damaged   int       `json:"damaged"`
	// Backup is "full", "partial", "none", "unknown" or "empty", as the
	// app's cards show it, with the byte shares behind it.
	Backup       string         `json:"backup"`
	BackedBytes  int64          `json:"backedBytes"`
	UnknownBytes int64          `json:"unknownBytes"`
	LikelyBytes  int64          `json:"likelyBytes"` // no checksum, but the same name and size elsewhere
	Holders      map[string]int `json:"holders,omitempty"`
	LastCheck    time.Time      `json:"lastCheck"`
	LastCheckOK  bool           `json:"lastCheckOk"`
	CheckQueued  bool           `json:"checkQueued"`
	Projects     []string       `json:"projects,omitempty"`
	Folders      []string       `json:"folders,omitempty"` // top-level folders, for orientation
}

type Project struct {
	ID      int64           `json:"id"`
	Name    string          `json:"name"`
	Color   string          `json:"color"`
	Target  int             `json:"target"`
	Health  *catalog.Health `json:"health"`
	Sources []Source        `json:"sources"`
	Steps   []Step          `json:"steps"`
	// Unplaced is how much has nowhere to go: no drive has room.
	Unplaced int64 `json:"unplaced"`
}

type Source struct {
	Drive     string `json:"drive"`
	Folder    string `json:"folder"`
	Connected bool   `json:"connected"`
	Bytes     int64  `json:"bytes"`
	SoleBytes int64  `json:"soleBytes"`
}

type Step struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Bytes int64  `json:"bytes"`
	Files int    `json:"files"`
	Ready bool   `json:"ready"` // both drives are plugged in now
}

// backupState mirrors the app's card logic: unknown is bytes with no
// checksum and nothing like them elsewhere; likely is bytes with no
// checksum but a same-name, same-size file on another drive.
func backupState(backed, total, unknown, likely int64) string {
	switch {
	case total == 0:
		return "empty"
	case backed >= total:
		return "full"
	case likely > 0 && backed+likely >= total:
		return "likely"
	case unknown >= total:
		return "unknown"
	case backed <= 0 && likely <= 0:
		return "none"
	}
	return "partial"
}

// Build gathers the status from the library.
func Build(db *catalog.DB, host, version, title string, files bool) (*Status, error) {
	st := &Status{Title: title, Generated: time.Now(), Host: host, Version: version, Drives: []Drive{}, Projects: []Project{}}
	mounted := map[string]*volume.Volume{}
	if vols, err := volume.Mounted(); err == nil {
		for _, v := range vols {
			mounted[v.ID] = v
		}
	}
	drives, err := db.Drives()
	if err != nil {
		return nil, err
	}
	byID := map[string]*catalog.Drive{}
	title_ := func(id string) string {
		if d := byID[id]; d != nil {
			return d.Title()
		}
		return id
	}
	for _, d := range drives {
		byID[d.ID] = d
	}
	projects, err := db.Projects()
	if err != nil {
		return nil, err
	}
	inProjects := map[string][]string{}
	for _, p := range projects {
		for _, s := range p.Sources {
			inProjects[s.DriveID] = append(inProjects[s.DriveID], p.Name)
		}
	}
	index := map[string]int{}
	for i, d := range drives {
		index[d.ID] = i
		v := mounted[d.ID]
		out := Drive{ID: d.ID, Title: d.Title(), Name: d.Name, Client: d.Client, Project: d.Project, Contact: d.Contact, Notes: d.Notes, Color: d.Color,
			Connected: v != nil, LastSeen: d.LastSeen, Size: d.Size, Free: d.Free, Files: d.Files, Bytes: d.Bytes, Damaged: d.Damaged,
			LastCheck: d.LastVerify, LastCheckOK: d.LastVerifyOK, CheckQueued: !d.VerifyRequested.IsZero(), Projects: inProjects[d.ID]}
		if v != nil {
			out.Free, out.Size = v.Free, v.Size
		}
		if b, err := db.Backups(d.ID, 1); err == nil {
			out.BackedBytes = b.EnoughBytes + b.ShortBytes
			out.UnknownBytes = b.UnknownBytes
			out.LikelyBytes = b.LikelyBytes
			out.Backup = backupState(out.BackedBytes, b.Bytes, out.UnknownBytes, out.LikelyBytes)
			out.Holders = b.Holders
		}
		if fs, err := db.Files(d.ID); err == nil {
			st.FileCount += len(fs)
			top := map[string]bool{}
			var states map[string]catalog.FileState
			if files {
				states, _ = db.BackupStates(d.ID)
			}
			for _, f := range fs {
				if i := strings.IndexByte(f.Path, '/'); i > 0 {
					top[f.Path[:i]] = true
				}
				if files {
					copies := -1
					if s, ok := states[f.Path]; ok && !s.Unknown {
						copies = s.Copies
					}
					st.Files = append(st.Files, []any{i, f.Path, f.Size, copies})
				}
			}
			for name := range top {
				out.Folders = append(out.Folders, name)
			}
			sort.Strings(out.Folders)
			if len(out.Folders) > 12 {
				out.Folders = append(out.Folders[:12], fmt.Sprintf("… %d more", len(out.Folders)-12))
			}
		}
		st.Drives = append(st.Drives, out)
	}
	for _, p := range projects {
		h, err := db.ProjectHealth(p.Sources, p.Target)
		if err != nil {
			return nil, err
		}
		out := Project{ID: p.ID, Name: p.Name, Color: p.Color, Target: p.Target, Health: h, Sources: []Source{}, Steps: []Step{}}
		for _, sh := range h.Sources {
			out.Sources = append(out.Sources, Source{Drive: title_(sh.DriveID), Folder: sh.Folder, Connected: mounted[sh.DriveID] != nil, Bytes: sh.Bytes, SoleBytes: sh.SoleBytes})
		}
		// The same plan the app shows: where copies would go.
		allowed := map[string]bool{}
		for _, id := range p.DestIDs {
			allowed[id] = true
		}
		var spaces []catalog.DriveSpace
		for _, d := range drives {
			if len(allowed) > 0 && !allowed[d.ID] || len(allowed) == 0 && d.NoBackups {
				continue
			}
			sp := catalog.DriveSpace{ID: d.ID, Free: d.Free}
			if v := mounted[d.ID]; v != nil {
				sp.Free, sp.Connected = v.Free, true
			}
			spaces = append(spaces, sp)
		}
		plan := catalog.MakePlan(h, spaces, catalog.ProjectLayout(p.Sources, title_))
		for _, t := range plan.Transfers {
			out.Steps = append(out.Steps, Step{From: title_(t.SourceID), To: title_(t.DestID), Bytes: t.Bytes, Files: len(t.Files),
				Ready: mounted[t.SourceID] != nil && mounted[t.DestID] != nil})
		}
		out.Unplaced = plan.UnplacedBytes
		st.Projects = append(st.Projects, out)
	}
	return st, nil
}

//go:embed page.html
var pageHTML string

var page = template.Must(template.New("page").Parse(pageHTML))

// Render writes the page: the data as JSON inside a self-contained HTML
// file, so it works from a folder, a share or any web server.
func Render(st *Status) ([]byte, error) {
	data, err := json.Marshal(st)
	if err != nil {
		return nil, err
	}
	// The JSON sits inside a <script>; "</" must not end it early.
	safe := strings.ReplaceAll(string(data), "</", `<\/`)
	var b bytes.Buffer
	err = page.Execute(&b, map[string]any{"Title": st.Title, "Data": template.JS(safe), "Generated": st.Generated.Format("2 Jan 2006 15:04")})
	return b.Bytes(), err
}

// Write puts index.html and status.json in dir.
func Write(dir string, st *Status) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	html, err := Render(st)
	if err != nil {
		return "", err
	}
	data, _ := json.MarshalIndent(st, "", " ")
	for name, body := range map[string][]byte{"index.html": html, "status.json": data} {
		tmp := filepath.Join(dir, "."+name+".tmp")
		if err := os.WriteFile(tmp, body, 0o644); err != nil {
			return "", err
		}
		if err := os.Rename(tmp, filepath.Join(dir, name)); err != nil {
			return "", err
		}
	}
	return filepath.Join(dir, "index.html"), nil
}

// Run builds, writes and sends the page as the settings say, and records
// how it went. It returns the result and the first error.
func Run(ctx context.Context, db *catalog.DB, host, version string, s Settings) (Result, error) {
	dir := s.Folder
	if dir == "" {
		dir = DefaultFolder()
	}
	st, err := Build(db, host, version, s.Title, s.Files)
	if err != nil {
		return fail(db, "", err)
	}
	path, err := Write(dir, st)
	if err != nil {
		return fail(db, "", err)
	}
	where := []string{dir}
	if s.URL != "" {
		if err := upload(ctx, s.URL, s.Token, dir); err != nil {
			return fail(db, path, fmt.Errorf("upload to %s: %w", s.URL, err))
		}
		where = append(where, s.URL)
	}
	if strings.TrimSpace(s.Command) != "" {
		cmd := exec.CommandContext(ctx, "/bin/sh", "-c", s.Command)
		cmd.Env = append(os.Environ(), "LUMPY_STATUS_DIR="+dir)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fail(db, path, fmt.Errorf("command: %v: %s", err, strings.TrimSpace(string(out))))
		}
		where = append(where, "command run")
	}
	r := Result{At: time.Now(), OK: true, Path: path,
		Summary: fmt.Sprintf("%d drives, %d projects, %s files → %s", len(st.Drives), len(st.Projects), count(st.FileCount), strings.Join(where, ", "))}
	record(db, r)
	return r, nil
}

func fail(db *catalog.DB, path string, err error) (Result, error) {
	r := Result{At: time.Now(), Path: path, Summary: err.Error()}
	record(db, r)
	return r, err
}

func count(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// upload PUTs index.html and status.json under base.
func upload(ctx context.Context, base, token, dir string) error {
	base = strings.TrimRight(base, "/")
	client := &http.Client{Timeout: 2 * time.Minute}
	for _, name := range []string{"status.json", "index.html"} {
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, base+"/"+name, bytes.NewReader(body))
		if err != nil {
			return err
		}
		if strings.HasSuffix(name, ".json") {
			req.Header.Set("Content-Type", "application/json")
		} else {
			req.Header.Set("Content-Type", "text/html; charset=utf-8")
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res, err := client.Do(req)
		if err != nil {
			return err
		}
		io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
		res.Body.Close()
		if res.StatusCode < 200 || res.StatusCode > 299 {
			return fmt.Errorf("%s: HTTP %d", name, res.StatusCode)
		}
	}
	return nil
}
