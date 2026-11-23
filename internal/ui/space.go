package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
	home, _ := os.UserHomeDir()
	if home != "" && at == "" && (root == "/" || root == filepath.Clean(home)) {
		suspects = space.FindSuspects(sc.tree, root, home)
	}
	if suspects == nil {
		suspects = []space.Suspect{}
	}
	// A verdict per thing in view, from where it sits.
	verdicts := map[string]space.Verdict{}
	hrel := homeRel(root, home)
	for _, c := range n.Children {
		if c.Rest > 0 {
			continue
		}
		rel := c.Name
		if at != "" {
			rel = at + "/" + c.Name
		}
		if v := space.Judge(rel, hrel, c.Dir); v.Level != "" {
			verdicts[c.Name] = v
		}
	}
	return map[string]any{"root": root, "at": at, "node": n.Trim(depth), "total": sc.tree.Bytes, "files": sc.tree.Files, "size": size, "free": free, "measured": sc.at.Unix(), "copies": copies, "suspects": suspects, "verdicts": verdicts}, nil
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

// homeRel is the home folder relative to the measured root: "." when
// home is the root, "" when home is not below it.
func homeRel(root, home string) string {
	if home == "" {
		return ""
	}
	rel, err := filepath.Rel(root, home)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ""
	}
	return filepath.ToSlash(rel)
}

// spaceCheck is the look before the Trash: for each item, the verdict
// from where it sits plus what can only be known by looking: when it
// last changed, whether a running app has it open, whether it is a
// folder of a Lumpy project, and whether the library holds a copy.
func (s *Server) spaceCheck(w http.ResponseWriter, r *http.Request) (any, error) {
	var body struct {
		Path  string   `json:"path"`
		Items []string `json:"items"`
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
	type check struct {
		Rel     string   `json:"rel"`
		Level   string   `json:"level"`
		Reasons []string `json:"reasons"`
		Bytes   int64    `json:"bytes"`
		Changed int64    `json:"changed,omitempty"` // newest mtime inside, unix
	}
	home, _ := os.UserHomeDir()
	hrel := homeRel(root, home)
	projects, _ := s.db.Projects()
	v, _ := volume.Of(root)
	var out []check
	for _, it := range body.Items {
		rel := strings.Trim(it, "/")
		n := sc.tree.At(rel)
		if rel == "" || strings.Contains(rel, "..") || n == nil {
			continue
		}
		full := filepath.Join(root, filepath.FromSlash(rel))
		c := check{Rel: rel, Bytes: n.Bytes}
		worst := ""
		raise := func(level, why string) {
			if rank(level) > rank(worst) {
				worst = level
			}
			c.Reasons = append(c.Reasons, why)
		}
		if j := space.Judge(rel, hrel, n.Dir); j.Level != "" {
			raise(j.Level, j.Why)
		}
		// Part of a Lumpy project?
		if v != nil {
			if d, err := s.db.Drive(v.ID); err == nil {
				drel, _ := filepath.Rel(v.Mount, full)
				drel = filepath.ToSlash(drel)
				for _, p := range projects {
					for _, src := range p.Sources {
						if src.DriveID != d.ID {
							continue
						}
						if src.Folder == "" || drel == src.Folder || strings.HasPrefix(drel, src.Folder+"/") || strings.HasPrefix(src.Folder, drel+"/") {
							raise("critical", "part of the project "+p.Name+" in Lumpy")
						}
					}
				}
			}
		}
		// Newest change inside, from a bounded walk.
		if newest := newestChange(full, 20000); !newest.IsZero() {
			c.Changed = newest.Unix()
			if time.Since(newest) < 7*24*time.Hour && worst != "safe" {
				raise("check", "changed "+when(newest)) // a cache changes all the time; that's not a reason to keep it
			} else {
				c.Reasons = append(c.Reasons, "changed "+when(newest))
			}
		}
		// Open by a running app?
		if apps := openBy(full, n.Dir, n.Files); len(apps) > 0 {
			raise("check", "open in "+strings.Join(apps, ", "))
		}
		// A copy in the library?
		if !n.Dir && n.Len > 0 {
			if k, err := s.db.CopiesOf(n.Name, n.Len); err == nil && k > 0 {
				c.Reasons = append(c.Reasons, fmt.Sprintf("a file of this name and size is on %d logged drive%s", k, plural(k)))
			}
		}
		if worst == "" {
			worst = "ok"
			if len(c.Reasons) == 0 {
				c.Reasons = append(c.Reasons, "nothing Lumpy knows against it")
			}
		}
		c.Level = worst
		out = append(out, c)
	}
	if out == nil {
		out = []check{}
	}
	return out, nil
}

func rank(level string) int {
	switch level {
	case "critical":
		return 3
	case "check":
		return 2
	case "safe":
		return 1
	}
	return 0
}

func when(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return "in the last hour"
	case d < 24*time.Hour:
		return "today"
	case d < 48*time.Hour:
		return "yesterday"
	default:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	}
}

// newestChange is the newest modification time under p, looking at up
// to limit entries so a huge folder doesn't take all day.
func newestChange(p string, limit int) time.Time {
	var newest time.Time
	seen := 0
	filepath.WalkDir(p, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		seen++
		if seen > limit {
			return filepath.SkipAll
		}
		if info, err := d.Info(); err == nil && info.Mode().IsRegular() && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	return newest
}

// openBy names the apps that have the file, or something inside a
// small folder, open. lsof is slow on big folders, so those are not
// asked about.
func openBy(p string, dir bool, files int64) []string {
	if _, err := exec.LookPath("lsof"); err != nil || (dir && files > 500) {
		return nil
	}
	args := []string{"-F", "c", "--", p}
	if dir {
		args = []string{"-F", "c", "+D", p}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(ctx, "lsof", args...).Output()
	seen := map[string]bool{}
	var apps []string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "c") && len(line) > 1 && !seen[line[1:]] {
			seen[line[1:]] = true
			apps = append(apps, line[1:])
		}
	}
	sort.Strings(apps)
	return apps
}
