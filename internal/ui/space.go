package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lumpysheppard/lumpy/internal/job"
	"github.com/lumpysheppard/lumpy/internal/space"
	"github.com/lumpysheppard/lumpy/internal/volume"
)

// The Space tab: measure a disk or folder, see what takes the room,
// put things in the Trash. Each measurement is a job (progress shows
// in Activity) whose tree is kept on the server until Lumpy quits or
// the place is measured again.

type spaceScan struct {
	root  string
	tree  *space.Node
	at    time.Time
	job   int
	prog  *space.Progress
	total int64 // the volume's capacity
	free  int64
}

type spaceStore struct {
	mu    sync.Mutex
	scans map[string]*spaceScan
}

// spacePlaces lists what can be measured: the Mac's own disk, the home
// folder, and every plugged-in drive.
func (s *Server) spacePlaces(w http.ResponseWriter, r *http.Request) (any, error) {
	type place struct {
		Name  string `json:"name"`
		Path  string `json:"path"`
		Size  int64  `json:"size"`
		Free  int64  `json:"free"`
		Done  bool   `json:"done"`     // measured already
		Busy  bool   `json:"busy"`     // being measured
		Bytes int64  `json:"measured"` // bytes found so far or in all
		At    int64  `json:"at,omitempty"`
	}
	var out []place
	add := func(name, path string) {
		v, _ := volume.Identify(path)
		p := place{Name: name, Path: path}
		if v != nil {
			p.Size, p.Free = v.Size, v.Free
		}
		s.spc.mu.Lock()
		if sc := s.spc.scans[path]; sc != nil {
			p.Done, p.Busy = sc.tree != nil, sc.tree == nil
			if sc.tree != nil {
				p.Bytes, p.At = sc.tree.Bytes, sc.at.Unix()
			} else if sc.prog != nil {
				p.Bytes = sc.prog.Bytes.Load()
			}
		}
		s.spc.mu.Unlock()
		out = append(out, p)
	}
	add("Macintosh HD", "/")
	if home, err := os.UserHomeDir(); err == nil {
		add("Home", home)
	}
	for _, v := range s.mounted() {
		add(v.Name, v.Mount)
	}
	return out, nil
}

// spaceScan measures a place as a job.
func (s *Server) spaceStart(w http.ResponseWriter, r *http.Request) (any, error) {
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, badRequest("bad request: %v", err)
	}
	root := filepath.Clean(strings.TrimSpace(body.Path))
	st, err := os.Stat(root)
	if err != nil || !st.IsDir() {
		return nil, badRequest("choose a folder or disk to measure")
	}
	s.spc.mu.Lock()
	if s.spc.scans == nil {
		s.spc.scans = map[string]*spaceScan{}
	}
	if sc := s.spc.scans[root]; sc != nil && sc.tree == nil {
		s.spc.mu.Unlock()
		return map[string]any{"job": sc.job}, nil // already measuring
	}
	sc := &spaceScan{root: root, prog: &space.Progress{}}
	s.spc.scans[root] = sc
	s.spc.mu.Unlock()
	name := filepath.Base(root)
	if root == "/" {
		name = "Macintosh HD"
	} else if home, err := os.UserHomeDir(); err == nil && root == filepath.Clean(home) {
		name = "Home"
	}
	j := s.jobs.Start("space", "Measure "+name, "lumpy space "+shellQuote(root), "", func(ctx context.Context, rep *job.Report) error {
		rep.Info("Measuring %s…", root)
		done := make(chan struct{})
		go func() {
			t := time.NewTicker(time.Second)
			defer t.Stop()
			for {
				select {
				case <-done:
					return
				case <-t.C:
					cur, _ := sc.prog.Current.Load().(string)
					rep.Detail("  %s · %s files · %s", job.Bytes(sc.prog.Bytes.Load()), fmtInt(sc.prog.Files.Load()), cur)
				}
			}
		}()
		tree, err := space.Scan(ctx, root, sc.prog)
		close(done)
		if err != nil {
			s.spc.mu.Lock()
			delete(s.spc.scans, root)
			s.spc.mu.Unlock()
			return err
		}
		s.spc.mu.Lock()
		sc.tree, sc.at = tree, time.Now()
		s.spc.mu.Unlock()
		rep.Info("%s in %s files", job.Bytes(tree.Bytes), fmtInt(tree.Files))
		return nil
	})
	sc.job = j.ID
	return map[string]any{"job": j.ID}, nil
}

func fmtInt(n int64) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// spaceTree returns the rings around one folder of a measured place:
// ?path= the place, ?at= the folder below it, ?depth= rings.
func (s *Server) spaceTree(w http.ResponseWriter, r *http.Request) (any, error) {
	q := r.URL.Query()
	root := filepath.Clean(q.Get("path"))
	s.spc.mu.Lock()
	sc := s.spc.scans[root]
	s.spc.mu.Unlock()
	if sc == nil {
		return nil, httpError{http.StatusNotFound, "not measured yet"}
	}
	if sc.tree == nil {
		return map[string]any{"busy": true, "bytes": sc.prog.Bytes.Load(), "files": sc.prog.Files.Load(), "job": sc.job}, nil
	}
	at := strings.Trim(q.Get("at"), "/")
	n := sc.tree.At(at)
	if n == nil {
		return nil, httpError{http.StatusNotFound, "that folder is no longer in the measurement"}
	}
	depth := 4
	fmt.Sscan(q.Get("depth"), &depth)
	v, _ := volume.Identify(root)
	var size, free int64
	if v != nil {
		size, free = v.Size, v.Free
	}
	// Files in view the library already holds elsewhere, by name and
	// size: the ones that may be safe to let go.
	copies := map[string]int{}
	for _, c := range n.Children {
		if c.Dir || c.Rest > 0 || c.Bytes < 64<<10 {
			continue
		}
		if k, err := s.db.CopiesOf(c.Name, c.Len); err == nil && k > 0 {
			copies[c.Name] = k
		}
	}
	var suspects []space.Suspect
	if home, err := os.UserHomeDir(); err == nil && at == "" && (root == "/" || root == filepath.Clean(home)) {
		suspects = space.FindSuspects(sc.tree, root, home)
	}
	if suspects == nil {
		suspects = []space.Suspect{}
	}
	return map[string]any{"root": root, "at": at, "node": n.Trim(depth), "total": sc.tree.Bytes, "files": sc.tree.Files, "size": size, "free": free, "measured": sc.at.Unix(), "copies": copies, "suspects": suspects}, nil
}

// spaceTrash puts items in the Trash through the Finder and takes them
// out of the measurement.
func (s *Server) spaceTrash(w http.ResponseWriter, r *http.Request) (any, error) {
	var body struct {
		Path  string   `json:"path"`
		Items []string `json:"items"` // relative to path
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, badRequest("bad request: %v", err)
	}
	root := filepath.Clean(body.Path)
	s.spc.mu.Lock()
	sc := s.spc.scans[root]
	s.spc.mu.Unlock()
	if sc == nil || sc.tree == nil {
		return nil, badRequest("measure it first")
	}
	var full []string
	for _, it := range body.Items {
		it = strings.Trim(it, "/")
		if it == "" || strings.Contains(it, "..") || sc.tree.At(it) == nil {
			return nil, badRequest("%s is not in the measurement", it)
		}
		full = append(full, filepath.Join(root, filepath.FromSlash(it)))
	}
	if len(full) == 0 {
		return nil, badRequest("nothing chosen")
	}
	err := job.TrashPaths(full)
	var gone []string
	var freed int64
	for i, p := range full {
		if _, statErr := os.Lstat(p); statErr != nil {
			rel := strings.Trim(body.Items[i], "/")
			if n := sc.tree.At(rel); n != nil {
				freed += n.Bytes
			}
			s.spc.mu.Lock()
			sc.tree.Remove(rel)
			s.spc.mu.Unlock()
			gone = append(gone, rel)
		}
	}
	if err != nil && len(gone) < len(full) {
		return nil, badRequest("%v", err)
	}
	if gone == nil {
		gone = []string{}
	}
	return map[string]any{"trashed": gone, "freed": freed}, nil
}
