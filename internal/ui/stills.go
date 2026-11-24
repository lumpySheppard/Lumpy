package ui

import (
	"fmt"
	"image/png"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/lumpysheppard/lumpy/internal/catalog"
)

// Stills: a frame grabbed from the player, at the clip's own size, as
// a PNG. It goes into the project's Stills folder when the project has
// one on a plugged-in drive (09_OUTPUT/Stills in the house template),
// otherwise into ~/Pictures/Lumpy Stills, and the library remembers
// which clip and frame it came from.

// stillsLimit is the most a grab may be: a 8K frame as a PNG is well
// under it.
const stillsLimit = 400 << 20

// clipPath checks the ?path= of a request against a plugged-in drive.
func (s *Server) clipPath(r *http.Request) (*catalog.Drive, string, string, error) {
	d, err := s.drive(r)
	if err != nil {
		return nil, "", "", err
	}
	p := filepath.Clean(filepath.FromSlash(r.URL.Query().Get("path")))
	if p == "." || strings.HasPrefix(p, "..") || filepath.IsAbs(p) {
		return nil, "", "", badRequest("bad path")
	}
	v := s.mountOf(d.ID)
	if v == nil {
		return d, filepath.ToSlash(p), "", nil
	}
	return d, filepath.ToSlash(p), filepath.Join(v.Mount, p), nil
}

// stillsDir is where a clip's grabs go: the first Stills folder under
// an output folder of a project the clip is in, on a plugged-in drive;
// otherwise ~/Pictures/Lumpy Stills/<project or drive>.
func (s *Server) stillsDir(d *catalog.Drive, rel string) string {
	projects, _ := s.db.Projects()
	var in *catalog.Project
	for _, p := range projects {
		for _, src := range p.Sources {
			if src.DriveID == d.ID && (src.Folder == "" || rel == src.Folder || strings.HasPrefix(rel, src.Folder+"/")) {
				in = p
				break
			}
		}
		if in != nil {
			break
		}
	}
	if in != nil {
		for _, src := range in.Sources {
			if src.Role == "backup" {
				continue
			}
			v := s.mountOf(src.DriveID)
			if v == nil {
				continue
			}
			if dir := findStills(filepath.Join(v.Mount, filepath.FromSlash(src.Folder))); dir != "" {
				return dir
			}
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	name := d.Title()
	if in != nil {
		name = in.Name
	}
	return filepath.Join(home, "Pictures", "Lumpy Stills", safeName(name))
}

// findStills looks two levels into a folder for OUTPUT/Stills.
func findStills(root string) string {
	tops, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	for _, t := range tops {
		if !t.IsDir() || !strings.Contains(strings.ToLower(t.Name()), "output") {
			continue
		}
		subs, err := os.ReadDir(filepath.Join(root, t.Name()))
		if err != nil {
			continue
		}
		for _, sd := range subs {
			if sd.IsDir() && strings.Contains(strings.ToLower(sd.Name()), "still") {
				return filepath.Join(root, t.Name(), sd.Name())
			}
		}
	}
	return ""
}

var unsafeName = regexp.MustCompile(`[/\\:*?"<>|]+`)

func safeName(s string) string {
	s = strings.TrimSpace(unsafeName.ReplaceAllString(s, "-"))
	if s == "" {
		return "Untitled"
	}
	return s
}

// grabStill is POST /api/drives/{id}/still?path=&tc=&frame= with the
// PNG as the body. The file is named after the clip and the frame's
// timecode, and never overwrites an earlier grab.
func (s *Server) grabStill(w http.ResponseWriter, r *http.Request) (any, error) {
	d, rel, _, err := s.clipPath(r)
	if err != nil {
		return nil, err
	}
	var body []byte
	if from := r.URL.Query().Get("from"); from != "" {
		// The app rendered the frame itself (player.swift) into its
		// temp folder; it is taken from there.
		clean := filepath.Clean(from)
		if !strings.Contains(clean, string(filepath.Separator)+"lumpy-stills"+string(filepath.Separator)) || !strings.HasSuffix(clean, ".png") {
			return nil, badRequest("bad still file")
		}
		body, err = os.ReadFile(clean)
		if err != nil {
			return nil, badRequest("reading the frame: %v", err)
		}
		defer os.Remove(clean)
	} else if body, err = io.ReadAll(io.LimitReader(r.Body, stillsLimit+1)); err != nil {
		return nil, badRequest("reading the frame: %v", err)
	}
	if len(body) == 0 || len(body) > stillsLimit {
		return nil, badRequest("the frame is empty or too big")
	}
	cfg, err := png.DecodeConfig(strings.NewReader(string(body)))
	if err != nil {
		return nil, badRequest("not a PNG: %v", err)
	}
	q := r.URL.Query()
	tc := q.Get("tc")
	frame, _ := strconv.ParseInt(q.Get("frame"), 10, 64)
	dir := s.stillsDir(d, rel)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("making %s: %w", dir, err)
	}
	base := strings.TrimSuffix(path.Base(rel), path.Ext(rel))
	tag := strings.ReplaceAll(tc, ":", ".")
	if tag == "" {
		tag = fmt.Sprintf("f%06d", frame)
	}
	name := fmt.Sprintf("%s_%s.png", base, tag)
	file := filepath.Join(dir, name)
	for n := 2; ; n++ {
		if _, err := os.Stat(file); err != nil {
			break
		}
		file = filepath.Join(dir, fmt.Sprintf("%s_%s_%d.png", base, tag, n))
	}
	if err := os.WriteFile(file, body, 0o644); err != nil {
		return nil, fmt.Errorf("writing %s: %w", file, err)
	}
	st := &catalog.Still{DriveID: d.ID, Path: rel, File: file, TC: tc, Frame: frame, Width: cfg.Width, Height: cfg.Height, At: time.Now()}
	if err := s.db.AddStill(st); err != nil {
		return nil, err
	}
	return st, nil
}

// clipStills is GET /api/drives/{id}/stills?path=: the frames grabbed
// from a clip, newest first, minus any whose file has gone.
func (s *Server) clipStills(w http.ResponseWriter, r *http.Request) (any, error) {
	d, rel, _, err := s.clipPath(r)
	if err != nil {
		return nil, err
	}
	all, err := s.db.Stills(d.ID, rel)
	if err != nil {
		return nil, err
	}
	out := []*catalog.Still{}
	for _, st := range all {
		if _, err := os.Stat(st.File); err != nil {
			s.db.DeleteStill(st.ID)
			continue
		}
		out = append(out, st)
	}
	return out, nil
}

func (s *Server) stillByID(r *http.Request) (*catalog.Still, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return nil, badRequest("bad still id")
	}
	st, err := s.db.Still(id)
	if err != nil {
		return nil, httpError{http.StatusNotFound, "no such still"}
	}
	return st, nil
}

// stillFile is GET /api/stills/{id}/file: the PNG itself.
func (s *Server) stillFile(w http.ResponseWriter, r *http.Request) (any, error) {
	st, err := s.stillByID(r)
	if err != nil {
		return nil, err
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeFile(w, r, st.File)
	return nil, nil
}

// dropStill is DELETE /api/stills/{id}: the file goes too.
func (s *Server) dropStill(w http.ResponseWriter, r *http.Request) (any, error) {
	st, err := s.stillByID(r)
	if err != nil {
		return nil, err
	}
	if err := os.Remove(st.File); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("removing %s: %w", st.File, err)
	}
	return map[string]any{"ok": true}, s.db.DeleteStill(st.ID)
}
