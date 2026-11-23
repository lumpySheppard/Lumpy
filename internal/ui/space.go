package ui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/lumpysheppard/lumpy/internal/job"
	"github.com/lumpysheppard/lumpy/internal/space"
)

// The Space tab: what takes room on this Mac or a drive, drawn as rings,
// with a way to move what isn't needed to the Trash. One scan at a time;
// its tree stays in memory until the next one.

// spaceRun is the latest scan.
type spaceRun struct {
	root     string
	started  time.Time
	finished time.Time
	err      string
	cancel   context.CancelFunc
	prog     *space.Progress
	tree     *space.Node // set when the scan has finished
	vol      space.Volume
}

type spaceState struct {
	mu  sync.Mutex
	run *spaceRun
}

// spaceDepth is how many rings the app draws below the folder shown, and
// spaceMin the smallest slice worth sending (a fraction of that folder).
const (
	spaceDepth = 4
	spaceMin   = 0.003
	spaceLimit = 150
)

// SpacePlace is a starting point offered on the Space tab.
type SpacePlace struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func spacePlaces() []SpacePlace {
	var out []SpacePlace
	// On a Mac, everything people put on the startup disk lives on its
	// Data volume; "/" alone is the sealed system and stops there.
	if runtime.GOOS == "darwin" {
		if _, err := os.Stat("/System/Volumes/Data"); err == nil {
			out = append(out, SpacePlace{"Macintosh HD", "/System/Volumes/Data"})
		}
	}
	if h, err := os.UserHomeDir(); err == nil {
		out = append(out, SpacePlace{"Home", h})
	}
	return out
}

// GET /api/space?path=rel: the scan's progress, or once it is done the
// folder at rel with its rings.
func (s *Server) spaceGet(w http.ResponseWriter, r *http.Request) (any, error) {
	out := map[string]any{"places": spacePlaces()}
	s.space.mu.Lock()
	defer s.space.mu.Unlock()
	run := s.space.run
	if run == nil {
		return out, nil
	}
	st := map[string]any{
		"root":       run.root,
		"started":    run.started.Unix(),
		"scanning":   run.tree == nil && run.err == "",
		"files":      run.prog.Files.Load(),
		"bytes":      run.prog.Bytes.Load(),
		"unreadable": run.prog.Unreadable.Load(),
		"current":    strings.TrimPrefix(run.prog.Current(), run.root),
		"error":      run.err,
		"volume":     run.vol,
	}
	if !run.finished.IsZero() {
		st["finished"] = run.finished.Unix()
	}
	out["run"] = st
	if run.tree != nil {
		rel := strings.Trim(r.URL.Query().Get("path"), "/")
		n, _ := space.Find(run.tree, rel)
		if n == nil {
			rel, n = "", run.tree // gone, e.g. moved to the Trash: back to the top
		}
		out["node"] = space.ViewOf(n, rel, spaceDepth, int64(float64(n.Size)*spaceMin), spaceLimit)
	}
	return out, nil
}

// POST /api/space/scan {"root": "/path"} starts a scan, ending any other.
func (s *Server) spaceScan(w http.ResponseWriter, r *http.Request) (any, error) {
	var body struct {
		Root string `json:"root"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, badRequest("bad request: %v", err)
	}
	root := filepath.Clean(strings.TrimSpace(body.Root))
	if !filepath.IsAbs(root) {
		return nil, badRequest("choose a folder or drive to measure")
	}
	if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
		return nil, badRequest("%s is not a folder that can be opened", root)
	}
	vol, _ := space.VolumeOf(root)
	ctx, cancel := context.WithCancel(context.Background())
	run := &spaceRun{root: root, started: time.Now(), cancel: cancel, prog: &space.Progress{}, vol: vol}
	s.space.mu.Lock()
	if old := s.space.run; old != nil && old.cancel != nil {
		old.cancel()
	}
	s.space.run = run
	s.space.mu.Unlock()
	go func() {
		tree, err := space.Scan(ctx, root, run.prog)
		s.space.mu.Lock()
		defer s.space.mu.Unlock()
		run.finished = time.Now()
		run.cancel = nil
		switch {
		case errors.Is(err, context.Canceled):
			run.err = "Stopped."
		case err != nil:
			run.err = err.Error()
		default:
			run.tree = tree
			run.vol, _ = space.VolumeOf(root)
		}
		cancel()
	}()
	return map[string]any{"ok": true}, nil
}

// POST /api/space/cancel stops a scan that is running.
func (s *Server) spaceCancel(w http.ResponseWriter, r *http.Request) (any, error) {
	s.space.mu.Lock()
	defer s.space.mu.Unlock()
	if run := s.space.run; run != nil && run.cancel != nil {
		run.cancel()
	}
	return map[string]any{"ok": true}, nil
}

// POST /api/space/trash {"paths": ["rel", …]} moves what the person
// collected to the Trash. Only items in the finished scan can be named,
// never the scanned folder itself, a folder above Lumpy's library, or the
// home folder; the space comes back when the Trash is emptied.
func (s *Server) spaceTrash(w http.ResponseWriter, r *http.Request) (any, error) {
	var body struct {
		Paths []string `json:"paths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, badRequest("bad request: %v", err)
	}
	if len(body.Paths) == 0 {
		return nil, badRequest("nothing to move")
	}
	s.space.mu.Lock()
	run := s.space.run
	s.space.mu.Unlock()
	if run == nil || run.tree == nil {
		return nil, badRequest("wait for the scan to finish")
	}
	protected := []string{}
	if h, err := os.UserHomeDir(); err == nil {
		protected = append(protected, h, filepath.Join(h, "Library"))
	}
	if info, err := s.db.Info(); err == nil && info.Path != "" {
		protected = append(protected, filepath.Dir(info.Path))
	}
	type failure struct {
		Path  string `json:"path"`
		Error string `json:"error"`
	}
	var failed []failure
	var moved []string
	var freed int64
	for _, rel := range body.Paths {
		rel = strings.Trim(rel, "/")
		s.space.mu.Lock()
		n, _ := space.Find(run.tree, rel)
		s.space.mu.Unlock()
		if rel == "" || n == nil {
			failed = append(failed, failure{rel, "not in this scan"})
			continue
		}
		abs := filepath.Join(run.root, filepath.FromSlash(rel))
		if runtime.GOOS == "darwin" && !startupDiskAllowed(abs) {
			failed = append(failed, failure{rel, "on the startup disk Lumpy only moves things from Users and Applications"})
			continue
		}
		if p := holds(abs, protected); p != "" {
			failed = append(failed, failure{rel, "Lumpy won't move " + p + " or a folder that holds it"})
			continue
		}
		if err := job.Trash([]string{abs}); err != nil {
			failed = append(failed, failure{rel, err.Error()})
			continue
		}
		if _, err := os.Lstat(abs); err == nil {
			failed = append(failed, failure{rel, "it is still there"})
			continue
		}
		s.space.mu.Lock()
		freed += n.Size
		space.Remove(run.tree, rel)
		s.space.mu.Unlock()
		moved = append(moved, rel)
	}
	s.space.mu.Lock()
	run.vol, _ = space.VolumeOf(run.root)
	s.space.mu.Unlock()
	return map[string]any{"moved": moved, "bytes": freed, "failed": failed}, nil
}

// startupDiskAllowed reports whether abs may go to the Trash: anything on
// another drive, but on a Mac's startup disk only what is inside a user's
// folder or Applications, never the system's own folders.
func startupDiskAllowed(abs string) bool {
	if strings.HasPrefix(abs, "/Volumes/") {
		return true
	}
	p := strings.TrimPrefix(abs, "/System/Volumes/Data")
	return strings.HasPrefix(p, "/Users/") || strings.HasPrefix(p, "/Applications/")
}

// holds returns the protected path that abs is or contains, if any.
func holds(abs string, protected []string) string {
	for _, p := range protected {
		if p == abs || strings.HasPrefix(p, abs+string(filepath.Separator)) {
			return p
		}
	}
	return ""
}
