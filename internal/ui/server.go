// Package ui serves Lumpy's app interface: a local web page and JSON API
// on 127.0.0.1, shown in Lumpy.app's window or a browser.
package ui

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lumpysheppard/lumpy/internal/agent"
	"github.com/lumpysheppard/lumpy/internal/catalog"
	"github.com/lumpysheppard/lumpy/internal/job"
	"github.com/lumpysheppard/lumpy/internal/volume"
)

//go:embed web
var webFS embed.FS

// Server is the app's backend.
type Server struct {
	db       *catalog.DB
	lockPath string
	meta     job.Meta
	token    string
	jobs     Jobs
	addr     string

	pub     publisher
	pubDone chan struct{}
	upd     updater

	mu       sync.Mutex
	vols     []*volume.Volume
	volsAt   time.Time
	cacheKey string
	summary  map[string]*driveExtra
	states   map[string]map[string]catalog.FileState // backup state per drive, until the library changes
}

type driveExtra struct {
	topFolders []string
	topFiles   []string
	backups    *catalog.BackupSummary
}

// New returns a server over the library db. lockPath is the agent lock,
// shared so the app and the launchd agent never log a drive at once.
func New(db *catalog.DB, lockPath string, meta job.Meta) *Server {
	b := make([]byte, 16)
	rand.Read(b)
	s := &Server{db: db, lockPath: lockPath, meta: meta, token: hex.EncodeToString(b), summary: map[string]*driveExtra{},
		pub: publisher{wake: make(chan struct{}, 1)}, pubDone: make(chan struct{})}
	// The status page is refreshed after each job, so coworkers see a
	// finished offload or check within moments.
	s.jobs.OnDone = func(*Job) { go s.publishOnce(context.Background(), false) }
	return s
}

// Listen starts serving on 127.0.0.1:port (0 picks a free port) and
// returns the URL to open, which carries the access token.
func (s *Server) Listen(port int) (string, *http.Server, net.Listener, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return "", nil, nil, err
	}
	s.addr = ln.Addr().String()
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}
	return "http://" + s.addr + "/#t=" + s.token, srv, ln, nil
}

// Watch logs newly mounted drives while the app is open, the same way the
// launchd agent does, so the agent is optional while the app runs. It
// also keeps the status page published.
func (s *Server) Watch(ctx context.Context) {
	go s.publishLoop(ctx)
	go s.updateLoop(ctx)
	t := time.NewTicker(4 * time.Second)
	defer t.Stop()
	for {
		unlock, ok, err := agent.Lock(s.lockPath)
		if err == nil && ok {
			r := job.NewReportTo(nopWriter{})
			job.AgentRunWith(ctx, s.db, r, agent.Notify, s.startQueuedCheck)
			unlock()
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// startQueuedCheck runs a drive's queued check as a job, so it shows in
// Activity with its log.
func (s *Server) startQueuedCheck(v *volume.Volume) {
	if s.jobs.Busy(v.ID) {
		return
	}
	title := v.Name
	if d, err := s.db.Drive(v.ID); err == nil {
		title = d.Title()
	}
	s.jobs.Start("verify", "Queued check of "+title, "lumpy drive verify "+shellQuote(title), v.ID, func(ctx context.Context, rep *job.Report) error {
		job.QueuedCheck(ctx, s.db, v, rep, agent.Notify)
		return nil
	})
}

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }

// Handler routes the page and the API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(webFS, "web")
	mux.Handle("GET /", http.FileServerFS(static))

	api := func(pattern string, h func(w http.ResponseWriter, r *http.Request) (any, error)) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Lumpy-Token") != s.token && r.URL.Query().Get("t") != s.token {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			v, err := h(w, r)
			if err != nil {
				var he httpError
				code := http.StatusInternalServerError
				if errors.As(err, &he) {
					code = he.code
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(code)
				json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			if v == nil {
				return // the handler wrote the response itself
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			json.NewEncoder(w).Encode(v)
		})
	}
	api("GET /api/state", s.state)
	api("GET /api/drives/{id}", s.driveDetail)
	api("PATCH /api/drives/{id}", s.setLabels)
	api("DELETE /api/drives/{id}", s.forget)
	api("GET /api/drives/{id}/list", s.list)
	api("POST /api/drives/{id}/verify", s.verify)
	api("POST /api/drives/{id}/hash", s.hash)
	api("POST /api/drives/{id}/rescan", s.rescan)
	api("POST /api/drives/{id}/cancel", s.cancelQueued)
	api("POST /api/drives/{id}/metadata", s.probeDrive)
	api("GET /api/drives/{id}/file", s.fileDetail)
	api("PATCH /api/drives/{id}/file", s.setFileEdits)
	api("GET /api/drives/{id}/stream", s.streamFile)
	api("GET /api/media", s.listMedia)
	api("GET /api/media.csv", s.exportMedia)
	api("GET /api/settings", s.getSettings)
	api("PUT /api/settings", s.putSettings)
	api("POST /api/publish", s.publishNow)
	api("GET /api/update", s.getUpdate)
	api("PUT /api/update/settings", s.putUpdateSettings)
	api("POST /api/update/check", s.checkUpdateNow)
	api("POST /api/update/install", s.installUpdate)
	api("POST /api/catalog", s.catalogMount)
	api("GET /api/search", s.search)
	api("GET /api/export.csv", s.export)
	api("GET /api/browse", s.browse)
	api("POST /api/offload", s.offload)
	api("GET /api/projects", s.projects)
	api("POST /api/projects", s.createProject)
	api("GET /api/projects/{id}", s.projectDetail)
	api("PATCH /api/projects/{id}", s.updateProject)
	api("DELETE /api/projects/{id}", s.deleteProject)
	api("POST /api/projects/{id}/transfer", s.runTransfer)
	api("GET /api/jobs", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.jobs.List(400), nil })
	api("POST /api/jobs/{id}/cancel", s.cancelJob)

	// Refuse requests addressed to any other host name, so a web page
	// cannot reach the API through DNS rebinding.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.addr != "" && r.Host != s.addr && r.Host != strings.Replace(s.addr, "127.0.0.1", "localhost", 1) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

type httpError struct {
	code int
	msg  string
}

func (e httpError) Error() string { return e.msg }

func badRequest(format string, a ...any) error {
	return httpError{http.StatusBadRequest, fmt.Sprintf(format, a...)}
}

// mounted returns the plugged-in drives, refreshed at most every 2 s.
func (s *Server) mounted() []*volume.Volume {
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Since(s.volsAt) > 2*time.Second {
		if v, err := volume.Mounted(); err == nil {
			s.vols = v
		}
		s.volsAt = time.Now()
	}
	return s.vols
}

func (s *Server) mountOf(id string) *volume.Volume {
	for _, v := range s.mounted() {
		if v.ID == id {
			return v
		}
	}
	return nil
}

// DriveView is a drive as the app shows it.
type DriveView struct {
	ID              string                 `json:"id"`
	Name            string                 `json:"name"`
	Title           string                 `json:"title"`
	Labels          catalog.Labels         `json:"labels"`
	Size            int64                  `json:"size"`
	Free            int64                  `json:"free"`
	FSType          string                 `json:"fstype"`
	FirstSeen       int64                  `json:"firstSeen"`
	LastSeen        int64                  `json:"lastSeen"`
	LastMount       string                 `json:"lastMount"`
	Connected       bool                   `json:"connected"`
	Mount           string                 `json:"mount,omitempty"`
	Files           int                    `json:"files"`
	Bytes           int64                  `json:"bytes"`
	Hashed          int                    `json:"hashed"`
	Damaged         int                    `json:"damaged"`
	LastScan        int64                  `json:"lastScan"`
	LastVerify      int64                  `json:"lastVerify"`
	LastVerifyOK    bool                   `json:"lastVerifyOk"`
	VerifyRequested int64                  `json:"verifyRequested"`
	Busy            bool                   `json:"busy"`
	Projects        []int64                `json:"projects"`
	TopFolders      []string               `json:"topFolders"`
	TopFiles        []string               `json:"topFiles"`
	Backups         *catalog.BackupSummary `json:"backups"`
}

func ts(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

// extras computes the per-drive summaries the cards show. They are cached
// until anything in the library changes, since backup coverage depends on
// every drive.
func (s *Server) extras(ds []*catalog.Drive) map[string]*driveExtra {
	var key strings.Builder
	for _, d := range ds {
		fmt.Fprintf(&key, "%s:%d:%d:%d:%d:%d;", d.ID, d.Files, d.Bytes, d.Hashed, d.Damaged, ts(d.LastScan))
	}
	s.mu.Lock()
	if key.String() == s.cacheKey {
		m := s.summary
		s.mu.Unlock()
		return m
	}
	s.mu.Unlock()
	m := map[string]*driveExtra{}
	for _, d := range ds {
		e := &driveExtra{}
		if slices, err := s.db.Breakdown(d.ID); err == nil {
			for _, sl := range slices {
				if sl.Folder {
					e.topFolders = append(e.topFolders, sl.Name)
				} else {
					e.topFiles = append(e.topFiles, sl.Name)
				}
			}
		}
		e.backups, _ = s.db.Backups(d.ID, 1)
		m[d.ID] = e
	}
	s.mu.Lock()
	s.cacheKey, s.summary = key.String(), m
	s.states = map[string]map[string]catalog.FileState{}
	s.mu.Unlock()
	return m
}

// backupStates returns a drive's per-file backup state, cached until
// anything in the library changes.
func (s *Server) backupStates(driveID string) (map[string]catalog.FileState, error) {
	ds, err := s.db.Drives()
	if err != nil {
		return nil, err
	}
	s.extras(ds) // refreshes the cache key, clearing stale states
	s.mu.Lock()
	st := s.states[driveID]
	s.mu.Unlock()
	if st != nil {
		return st, nil
	}
	st, err = s.db.BackupStates(driveID)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if s.states == nil {
		s.states = map[string]map[string]catalog.FileState{}
	}
	s.states[driveID] = st
	s.mu.Unlock()
	return st, nil
}

func (s *Server) view(d *catalog.Drive, ex *driveExtra) DriveView {
	v := DriveView{
		ID: d.ID, Name: d.Name, Title: d.Title(), Labels: d.Labels, Size: d.Size, Free: d.Free,
		FSType: d.FSType, FirstSeen: ts(d.FirstSeen), LastSeen: ts(d.LastSeen), LastMount: d.LastMount,
		Files: d.Files, Bytes: d.Bytes, Hashed: d.Hashed, Damaged: d.Damaged, LastScan: ts(d.LastScan),
		LastVerify: ts(d.LastVerify), LastVerifyOK: d.LastVerifyOK, VerifyRequested: ts(d.VerifyRequested),
		Busy: s.jobs.Busy(d.ID),
	}
	if m := s.mountOf(d.ID); m != nil {
		v.Connected, v.Mount, v.Free = true, m.Mount, m.Free
	}
	if ex != nil {
		v.TopFolders, v.TopFiles, v.Backups = ex.topFolders, ex.topFiles, ex.backups
	}
	return v
}

// VolumeView is a plugged-in volume, logged or not.
type VolumeView struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Mount  string `json:"mount"`
	Size   int64  `json:"size"`
	Free   int64  `json:"free"`
	FSType string `json:"fstype"`
	Logged bool   `json:"logged"`
}

func (s *Server) state(w http.ResponseWriter, r *http.Request) (any, error) {
	ds, err := s.db.Drives()
	if err != nil {
		return nil, err
	}
	ex := s.extras(ds)
	logged := map[string]bool{}
	drives := make([]DriveView, 0, len(ds))
	for _, d := range ds {
		logged[d.ID] = true
		drives = append(drives, s.view(d, ex[d.ID]))
	}
	vols := []VolumeView{}
	for _, v := range s.mounted() {
		vols = append(vols, VolumeView{v.ID, v.Name, v.Mount, v.Size, v.Free, v.FSType, logged[v.ID]})
	}
	running := 0
	for _, j := range s.jobs.List(0) {
		if j.State == "running" {
			running++
		}
	}
	projects, err := s.db.Projects()
	if err != nil {
		return nil, err
	}
	memberships, _ := s.db.DriveProjects()
	for i := range drives {
		drives[i].Projects = memberships[drives[i].ID]
		if drives[i].Projects == nil {
			drives[i].Projects = []int64{}
		}
	}
	return map[string]any{
		"drives": drives, "volumes": vols, "runningJobs": running, "projects": projects,
		"agent": agent.Status(), "version": s.meta.Version, "home": filepath.Dir(s.lockPath),
		"update": s.updateSummary(), "recent": s.db.RecentOffloads(),
	}, nil
}

func (s *Server) drive(r *http.Request) (*catalog.Drive, error) {
	d, err := s.db.Drive(r.PathValue("id"))
	if err != nil {
		return nil, httpError{http.StatusNotFound, err.Error()}
	}
	return d, nil
}

func (s *Server) driveDetail(w http.ResponseWriter, r *http.Request) (any, error) {
	d, err := s.drive(r)
	if err != nil {
		return nil, err
	}
	folders, err := s.db.FolderHealths(d.ID)
	if err != nil {
		return nil, err
	}
	memberships, _ := s.db.DriveProjects()
	events, err := s.db.Events(d.ID, 50)
	if err != nil {
		return nil, err
	}
	min := 1
	if n, err := strconv.Atoi(r.URL.Query().Get("min")); err == nil && n > 0 {
		min = n
	}
	backups, err := s.db.Backups(d.ID, min)
	if err != nil {
		return nil, err
	}
	type ev struct {
		At       int64  `json:"at"`
		Kind     string `json:"kind"`
		Summary  string `json:"summary"`
		Failures int    `json:"failures"`
	}
	evs := make([]ev, 0, len(events))
	for _, e := range events {
		evs = append(evs, ev{e.At.Unix(), e.Kind, e.Summary, e.Failures})
	}
	ds, _ := s.db.Drives()
	return map[string]any{
		"drive":    s.view(d, s.extras(ds)[d.ID]),
		"folders":  folders,
		"events":   evs,
		"backups":  backups,
		"projects": memberships[d.ID],
	}, nil
}

func (s *Server) setLabels(w http.ResponseWriter, r *http.Request) (any, error) {
	d, err := s.drive(r)
	if err != nil {
		return nil, err
	}
	l := d.Labels
	if err := json.NewDecoder(r.Body).Decode(&l); err != nil {
		return nil, badRequest("bad labels: %v", err)
	}
	if err := s.db.SetLabels(d.ID, l); err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, nil
}

func (s *Server) forget(w http.ResponseWriter, r *http.Request) (any, error) {
	d, err := s.drive(r)
	if err != nil {
		return nil, err
	}
	if s.jobs.Busy(d.ID) {
		return nil, badRequest("%s has a job running", d.Title())
	}
	return map[string]bool{"ok": true}, s.db.Forget(d.ID)
}

func (s *Server) list(w http.ResponseWriter, r *http.Request) (any, error) {
	d, err := s.drive(r)
	if err != nil {
		return nil, err
	}
	q := r.URL.Query()
	states, err := s.backupStates(d.ID)
	if err != nil {
		return nil, err
	}
	entries, err := s.db.List(d.ID, q.Get("dir"), strings.TrimSpace(q.Get("filter")), 2000, states)
	if err != nil {
		return nil, err
	}
	if entries == nil {
		entries = []catalog.Entry{}
	}
	return entries, nil
}

func (s *Server) requireIdle(d *catalog.Drive) error {
	if s.jobs.Busy(d.ID) {
		return badRequest("%s already has a job running", d.Title())
	}
	return nil
}

func (s *Server) verify(w http.ResponseWriter, r *http.Request) (any, error) {
	d, err := s.drive(r)
	if err != nil {
		return nil, err
	}
	v := s.mountOf(d.ID)
	if v == nil {
		if err := s.db.RequestVerify(d.ID, true); err != nil {
			return nil, err
		}
		s.db.Log(d.ID, "queued", "full check requested", 0)
		return map[string]any{"queued": true}, nil
	}
	if err := s.requireIdle(d); err != nil {
		return nil, err
	}
	j := s.jobs.Start("verify", "Check "+d.Title(), "lumpy drive verify "+shellQuote(d.Title()), d.ID, func(ctx context.Context, rep *job.Report) error {
		return job.HashDrive(ctx, s.db, v, true, rep)
	})
	return map[string]any{"job": j.ID}, nil
}

func (s *Server) hash(w http.ResponseWriter, r *http.Request) (any, error) {
	d, err := s.drive(r)
	if err != nil {
		return nil, err
	}
	v := s.mountOf(d.ID)
	if v == nil {
		return nil, badRequest("%s is not plugged in", d.Title())
	}
	if err := s.requireIdle(d); err != nil {
		return nil, err
	}
	j := s.jobs.Start("hash", "Checksum new files on "+d.Title(), "lumpy drive hash "+shellQuote(d.Title()), d.ID, func(ctx context.Context, rep *job.Report) error {
		return job.HashDrive(ctx, s.db, v, false, rep)
	})
	return map[string]any{"job": j.ID}, nil
}

func (s *Server) rescan(w http.ResponseWriter, r *http.Request) (any, error) {
	d, err := s.drive(r)
	if err != nil {
		return nil, err
	}
	v := s.mountOf(d.ID)
	if v == nil {
		return nil, badRequest("%s is not plugged in", d.Title())
	}
	if err := s.requireIdle(d); err != nil {
		return nil, err
	}
	j := s.jobs.Start("rescan", "Rescan "+d.Title(), "lumpy catalog "+shellQuote(v.Mount), d.ID, func(ctx context.Context, rep *job.Report) error {
		_, err := job.CatalogDrive(ctx, s.db, v, rep)
		return err
	})
	return map[string]any{"job": j.ID}, nil
}

func (s *Server) cancelQueued(w http.ResponseWriter, r *http.Request) (any, error) {
	d, err := s.drive(r)
	if err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, s.db.RequestVerify(d.ID, false)
}

func (s *Server) catalogMount(w http.ResponseWriter, r *http.Request) (any, error) {
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, badRequest("bad request: %v", err)
	}
	v := s.mountOf(body.ID)
	if v == nil {
		return nil, badRequest("that drive is not plugged in")
	}
	if d, err := s.db.Drive(v.ID); err == nil && d.NeverLog {
		d.NeverLog = false
		s.db.SetLabels(d.ID, d.Labels)
	}
	j := s.jobs.Start("rescan", "Log "+v.Name, "lumpy catalog "+shellQuote(v.Mount), v.ID, func(ctx context.Context, rep *job.Report) error {
		_, err := job.CatalogDrive(ctx, s.db, v, rep)
		return err
	})
	return map[string]any{"job": j.ID}, nil
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) (any, error) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		return map[string]any{"files": []catalog.Match{}}, nil
	}
	ms, err := s.db.Search(q, 300)
	if err != nil {
		return nil, err
	}
	if ms == nil {
		ms = []catalog.Match{}
	}
	return map[string]any{"files": ms}, nil
}

func (s *Server) export(w http.ResponseWriter, r *http.Request) (any, error) {
	ds, err := s.db.Drives()
	if err != nil {
		return nil, err
	}
	ex := s.extras(ds)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="lumpy-drives-%s.csv"`, time.Now().Format("2006-01-02")))
	cw := csv.NewWriter(w)
	cw.Write([]string{"Name", "Volume", "ID", "Client", "Project", "Contact", "Notes", "Capacity (bytes)", "Free (bytes)",
		"Files", "Data (bytes)", "Checksummed", "Damaged", "Fewest copies elsewhere", "Not backed up", "First seen", "Last seen",
		"Last check", "Last check OK", "Check queued", "Top-level folders"})
	when := func(t time.Time) string {
		if t.IsZero() {
			return ""
		}
		return t.Local().Format("2006-01-02 15:04")
	}
	for _, d := range ds {
		minCopies, notBacked := "", ""
		e := ex[d.ID]
		if e != nil && e.backups != nil {
			if e.backups.MinCopies >= 0 {
				minCopies = strconv.Itoa(e.backups.MinCopies)
			}
			notBacked = strconv.Itoa(e.backups.Short + e.backups.None + e.backups.Unknown)
		}
		var folders []string
		if e != nil {
			folders = e.topFolders
		}
		cw.Write([]string{d.Title(), d.Name, d.ID, d.Client, d.Project, d.Contact, d.Notes,
			strconv.FormatInt(d.Size, 10), strconv.FormatInt(d.Free, 10), strconv.Itoa(d.Files),
			strconv.FormatInt(d.Bytes, 10), strconv.Itoa(d.Hashed), strconv.Itoa(d.Damaged), minCopies, notBacked,
			when(d.FirstSeen), when(d.LastSeen), when(d.LastVerify), strconv.FormatBool(!d.LastVerify.IsZero() && d.LastVerifyOK),
			strconv.FormatBool(!d.VerifyRequested.IsZero()), strings.Join(folders, ", ")})
	}
	cw.Flush()
	return nil, cw.Error()
}

// browse lists folders for the in-app folder picker, used when the app
// runs in a browser instead of Lumpy.app's native picker.
func (s *Server) browse(w http.ResponseWriter, r *http.Request) (any, error) {
	type item struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	p := r.URL.Query().Get("path")
	items := []item{}
	if p == "" {
		for _, v := range s.mounted() {
			items = append(items, item{v.Name, v.Mount})
		}
		if home, err := os.UserHomeDir(); err == nil {
			items = append(items, item{"Home", home})
		}
		items = append(items, item{"Computer", "/"})
		return map[string]any{"path": "", "parent": "", "items": items}, nil
	}
	p = filepath.Clean(p)
	entries, err := os.ReadDir(p)
	if err != nil {
		return nil, badRequest("%v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		full := filepath.Join(p, e.Name())
		if st, err := os.Stat(full); err == nil && st.IsDir() {
			items = append(items, item{e.Name(), full})
		}
	}
	sort.Slice(items, func(i, j int) bool { return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name) })
	parent := filepath.Dir(p)
	if parent == p {
		parent = ""
	}
	return map[string]any{"path": p, "parent": parent, "items": items}, nil
}

func (s *Server) offload(w http.ResponseWriter, r *http.Request) (any, error) {
	var body struct {
		Source      string   `json:"source"`
		Dests       []string `json:"dests"`
		Name        string   `json:"name"`
		SourceCheck *bool    `json:"sourceCheck"` // default true
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, badRequest("bad request: %v", err)
	}
	if body.Source == "" || len(body.Dests) == 0 {
		return nil, badRequest("choose a source and at least one destination")
	}
	name := body.Name
	if name == "" {
		name = filepath.Base(filepath.Clean(body.Source))
	}
	skip := body.SourceCheck != nil && !*body.SourceCheck
	cmd := []string{"lumpy", "offload"}
	if body.Name != "" {
		cmd = append(cmd, "--name", shellQuote(body.Name))
	}
	if skip {
		cmd = append(cmd, "--no-source-check")
	}
	cmd = append(cmd, "--verbose", shellQuote(body.Source))
	for _, d := range body.Dests {
		cmd = append(cmd, shellQuote(d))
	}
	s.db.RememberOffload(body.Source, body.Dests)
	title := fmt.Sprintf("Offload %s to %d drive(s)", name, len(body.Dests))
	j := s.jobs.Start("offload", title, strings.Join(cmd, " "), "", func(ctx context.Context, rep *job.Report) error {
		return job.Offload(ctx, job.OffloadOptions{Source: body.Source, Dests: body.Dests, Name: body.Name,
			Meta: s.meta, Library: s.db, SkipSourceVerify: skip}, rep)
	})
	return map[string]any{"job": j.ID}, nil
}

func (s *Server) cancelJob(w http.ResponseWriter, r *http.Request) (any, error) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	j := s.jobs.Get(id)
	if j == nil {
		return nil, httpError{http.StatusNotFound, "no such job"}
	}
	j.Cancel()
	return map[string]bool{"ok": true}, nil
}

// shellQuote quotes s for a POSIX shell when it needs it.
func shellQuote(s string) string {
	if s != "" && strings.IndexFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("/._-:+,=@", r))
	}) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
