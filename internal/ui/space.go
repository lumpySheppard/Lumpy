package ui

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path"
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
	big   []space.BigFile
	at    time.Time
	job   int
	prog  *space.Progress
	total int64 // the volume's capacity
	free  int64
	// The duplicate search, once asked for.
	dupes     []space.DupeGroup
	dupeBytes int64
	dupeJob   int
	dupeBusy  bool
	dupeProg  *space.Progress
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
	if !s.Mobile {
		add("Macintosh HD", "/")
		if home, err := os.UserHomeDir(); err == nil {
			add("Home", home)
		}
		for _, v := range s.mounted() {
			add(v.Name, v.Mount)
		}
	}
	// The folders added by hand: on the iPad every place is one.
	for _, pl := range s.places() {
		if st, err := os.Stat(pl.Path); err == nil && st.IsDir() {
			add(pl.Name, pl.Path)
		}
	}
	if out == nil {
		out = []place{}
	}
	return out, nil
}

// A savedPlace is a folder added to the Space tab by hand.
type savedPlace struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func (s *Server) places() []savedPlace {
	var out []savedPlace
	json.Unmarshal([]byte(s.db.Setting("space.places", "[]")), &out)
	return out
}

func (s *Server) savePlaces(list []savedPlace) error {
	data, _ := json.Marshal(list)
	return s.db.SetSetting("space.places", string(data))
}

// AddPlace adds a folder to the Space tab's places, keeping one entry
// per path. The iPad app calls it for every folder the user granted.
func (s *Server) AddPlace(name, path string) error {
	path = filepath.Clean(path)
	if st, err := os.Stat(path); err != nil || !st.IsDir() {
		return fmt.Errorf("%s is not a folder Lumpy can read", path)
	}
	if name == "" {
		name = filepath.Base(path)
	}
	list := s.places()
	for i := range list {
		if list[i].Path == path {
			list[i].Name = name
			return s.savePlaces(list)
		}
	}
	return s.savePlaces(append(list, savedPlace{Name: name, Path: path}))
}

// spaceAddPlace is POST /api/space/places {name, path}.
func (s *Server) spaceAddPlace(w http.ResponseWriter, r *http.Request) (any, error) {
	var body savedPlace
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, badRequest("bad request: %v", err)
	}
	if err := s.AddPlace(body.Name, body.Path); err != nil {
		return nil, badRequest("%v", err)
	}
	return map[string]any{"ok": true}, nil
}

// spaceDropPlace is DELETE /api/space/places?path=: the folder is
// forgotten (nothing on it is touched).
func (s *Server) spaceDropPlace(w http.ResponseWriter, r *http.Request) (any, error) {
	path := filepath.Clean(r.URL.Query().Get("path"))
	var keep []savedPlace
	for _, pl := range s.places() {
		if pl.Path != path {
			keep = append(keep, pl)
		}
	}
	if keep == nil {
		keep = []savedPlace{}
	}
	return map[string]any{"ok": true}, s.savePlaces(keep)
}

// spaceStart measures a place as a job.
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
	j, err := s.measure(root)
	if err != nil {
		return nil, err
	}
	return map[string]any{"job": j.ID}, nil
}

// measure starts measuring a place, or returns the measurement already
// under way.
func (s *Server) measure(root string) (*Job, error) {
	s.spc.mu.Lock()
	if s.spc.scans == nil {
		s.spc.scans = map[string]*spaceScan{}
	}
	if sc := s.spc.scans[root]; sc != nil && sc.tree == nil {
		s.spc.mu.Unlock()
		return s.jobs.Get(sc.job), nil // already measuring
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
		res, err := space.ScanAll(ctx, root, sc.prog)
		close(done)
		if err != nil {
			s.spc.mu.Lock()
			delete(s.spc.scans, root)
			s.spc.mu.Unlock()
			return err
		}
		tree := res.Tree
		s.spc.mu.Lock()
		sc.tree, sc.big, sc.at = tree, res.Big, time.Now()
		sc.dupes, sc.dupeBytes, sc.dupeBusy = nil, 0, false
		s.spc.mu.Unlock()
		rep.Info("%s in %s files; %s of them 1 MB or more", job.Bytes(tree.Bytes), fmtInt(tree.Files), fmtInt(int64(len(res.Big))))
		return nil
	})
	sc.job = j.ID
	return j, nil
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
	// The usual places under the home folder when the disk or the home
	// is measured; folders known by name wherever they sit, always.
	if at == "" {
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
	return map[string]any{"root": root, "at": at, "node": n.Trim(depth, n.Bytes/400), "unreadable": sc.tree.Unreadable(), "total": sc.tree.Bytes, "files": sc.tree.Files, "size": size, "free": free, "measured": sc.at.Unix(), "copies": copies, "suspects": suspects, "verdicts": verdicts}, nil
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
		if it == "" || strings.Contains(it, "..") || (sc.tree.At(it) == nil && sc.bigAt(it) == nil) {
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
			} else if b := sc.bigAt(rel); b != nil {
				freed += b.Disk
			}
			s.spc.mu.Lock()
			sc.tree.Remove(rel)
			sc.dropBig(rel)
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
		if n == nil {
			if b := sc.bigAt(rel); b != nil {
				n = &space.Node{Name: path.Base(rel), Bytes: b.Disk, Len: b.Size, Files: 1}
			}
		}
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

// bigAt finds a remembered big file by its path; dropBig forgets one.
func (sc *spaceScan) bigAt(rel string) *space.BigFile {
	for i := range sc.big {
		if sc.big[i].Rel == rel {
			return &sc.big[i]
		}
	}
	return nil
}
func (sc *spaceScan) dropBig(rel string) {
	kept := sc.big[:0]
	for _, b := range sc.big {
		if b.Rel != rel && !strings.HasPrefix(b.Rel, rel+"/") {
			kept = append(kept, b)
		}
	}
	sc.big = kept
	for gi := range sc.dupes {
		g := &sc.dupes[gi]
		kept := g.Paths[:0]
		for _, p := range g.Paths {
			if p != rel && !strings.HasPrefix(p, rel+"/") {
				kept = append(kept, p)
			}
		}
		g.Paths = kept
		g.Wasted = g.Size * int64(max(0, len(kept)-1))
	}
}

// spaceDupes starts (POST) or reports (GET) the duplicate search for
// a measured place: identical big files, proven by hash.
func (s *Server) spaceDupes(w http.ResponseWriter, r *http.Request) (any, error) {
	root := filepath.Clean(r.URL.Query().Get("path"))
	s.spc.mu.Lock()
	sc := s.spc.scans[root]
	s.spc.mu.Unlock()
	if sc == nil || sc.tree == nil {
		return nil, badRequest("measure it first")
	}
	if r.Method == http.MethodGet {
		s.spc.mu.Lock()
		defer s.spc.mu.Unlock()
		if sc.dupeBusy {
			var hashed int64
			if sc.dupeProg != nil {
				hashed = sc.dupeProg.Bytes.Load()
			}
			return map[string]any{"busy": true, "hashed": hashed, "job": sc.dupeJob}, nil
		}
		if sc.dupes == nil {
			return map[string]any{"ready": false}, nil
		}
		groups := sc.dupes
		if len(groups) > 300 {
			groups = groups[:300]
		}
		var wasted int64
		for _, g := range sc.dupes {
			wasted += g.Wasted
		}
		return map[string]any{"ready": true, "groups": groups, "wasted": wasted, "candidates": len(sc.big)}, nil
	}
	s.spc.mu.Lock()
	if sc.dupeBusy {
		s.spc.mu.Unlock()
		return map[string]any{"job": sc.dupeJob}, nil
	}
	sc.dupeBusy = true
	sc.dupeProg = &space.Progress{}
	big := sc.big
	s.spc.mu.Unlock()
	// Only files that share a length with another get read.
	bySize := map[int64]int{}
	for _, f := range big {
		bySize[f.Size]++
	}
	var toRead int64
	for _, f := range big {
		if bySize[f.Size] > 1 {
			toRead += f.Size
		}
	}
	name := filepath.Base(root)
	if root == "/" {
		name = "Macintosh HD"
	} else if home, err := os.UserHomeDir(); err == nil && root == filepath.Clean(home) {
		name = "Home"
	}
	j := s.jobs.Start("space", "Find duplicates on "+name, "lumpy space --dupes "+shellQuote(root), "", func(ctx context.Context, rep *job.Report) error {
		rep.Info("Reading %s of files that share a size with another, to see which are the same…", job.Bytes(toRead))
		done := make(chan struct{})
		go func() {
			t := time.NewTicker(2 * time.Second)
			defer t.Stop()
			for {
				select {
				case <-done:
					return
				case <-t.C:
					cur, _ := sc.dupeProg.Current.Load().(string)
					rep.Detail("  %s of %s · %s", job.Bytes(sc.dupeProg.Bytes.Load()), job.Bytes(toRead), cur)
				}
			}
		}()
		groups, wasted, err := space.Dupes(ctx, root, big, sc.dupeProg)
		close(done)
		s.spc.mu.Lock()
		sc.dupeBusy = false
		if err == nil {
			if groups == nil {
				groups = []space.DupeGroup{}
			}
			sc.dupes, sc.dupeBytes = groups, wasted
		}
		s.spc.mu.Unlock()
		if err != nil {
			return err
		}
		rep.Info("%d sets of identical files; %s taken by the extra copies", len(groups), job.Bytes(wasted))
		return nil
	})
	s.spc.mu.Lock()
	sc.dupeJob = j.ID
	s.spc.mu.Unlock()
	return map[string]any{"job": j.ID}, nil
}

// spaceOld lists big files neither changed nor opened in a year, and
// apps not opened in a year (from Spotlight's last-used date).
func (s *Server) spaceOld(w http.ResponseWriter, r *http.Request) (any, error) {
	root := filepath.Clean(r.URL.Query().Get("path"))
	s.spc.mu.Lock()
	sc := s.spc.scans[root]
	s.spc.mu.Unlock()
	if sc == nil || sc.tree == nil {
		return nil, badRequest("measure it first")
	}
	days := 365
	fmt.Sscan(r.URL.Query().Get("days"), &days)
	if days < 30 {
		days = 30
	}
	var big []space.BigFile
	for _, f := range sc.big {
		if !space.IsUnderApp(f.Rel) {
			big = append(big, f)
		}
	}
	files, total := space.Old(big, time.Duration(days)*24*time.Hour, 400)
	if files == nil {
		files = []space.BigFile{}
	}
	type app struct {
		Rel      string `json:"rel"`
		Name     string `json:"name"`
		Bytes    int64  `json:"bytes"`
		LastUsed int64  `json:"lastUsed"` // 0: never, as far as Spotlight knows
	}
	apps := []app{}
	home, _ := os.UserHomeDir()
	for _, dir := range []string{"/Applications", filepath.Join(home, "Applications")} {
		rel, err := filepath.Rel(root, dir)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		n := sc.tree.At(filepath.ToSlash(rel))
		if n == nil {
			continue
		}
		for _, c := range n.Children {
			if !strings.HasSuffix(c.Name, ".app") {
				continue
			}
			full := filepath.Join(dir, c.Name)
			used := lastUsed(full)
			if used.IsZero() {
				continue // Spotlight doesn't know; say nothing rather than guess
			}
			if time.Since(used) > time.Duration(days)*24*time.Hour {
				apps = append(apps, app{Rel: filepath.ToSlash(filepath.Join(rel, c.Name)), Name: strings.TrimSuffix(c.Name, ".app"), Bytes: c.Bytes, LastUsed: used.Unix()})
			}
		}
	}
	sort.Slice(apps, func(i, j int) bool { return apps[i].Bytes > apps[j].Bytes })
	return map[string]any{"days": days, "files": files, "total": total, "apps": apps, "candidates": len(big)}, nil
}

// lastUsed asks Spotlight when an app was last opened; zero when it
// doesn't know or this isn't a Mac.
func lastUsed(p string) time.Time {
	if _, err := exec.LookPath("mdls"); err != nil {
		return time.Time{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "mdls", "-name", "kMDItemLastUsedDate", "-raw", p).Output()
	if err != nil {
		return time.Time{}
	}
	t, err := time.Parse("2006-01-02 15:04:05 -0700", strings.TrimSpace(string(out)))
	if err != nil {
		return time.Time{}
	}
	return t
}

// spaceThumb is a preview of a file in a measured place, for looking
// at duplicates side by side: ?path= the place, ?rel= the file.
func (s *Server) spaceThumb(w http.ResponseWriter, r *http.Request) (any, error) {
	root := filepath.Clean(r.URL.Query().Get("path"))
	rel := strings.Trim(r.URL.Query().Get("rel"), "/")
	s.spc.mu.Lock()
	sc := s.spc.scans[root]
	s.spc.mu.Unlock()
	if sc == nil || sc.tree == nil || rel == "" || strings.Contains(rel, "..") {
		return nil, badRequest("measure it first")
	}
	if sc.tree.At(rel) == nil && sc.bigAt(rel) == nil {
		return nil, httpError{http.StatusNotFound, "not in the measurement"}
	}
	full := filepath.Join(root, filepath.FromSlash(rel))
	st, err := os.Stat(full)
	if err != nil {
		return nil, httpError{http.StatusNotFound, err.Error()}
	}
	width := 320
	fmt.Sscan(r.URL.Query().Get("w"), &width)
	if width < 64 {
		width = 64
	}
	if width > 2048 {
		width = 2048
	}
	key := fmt.Sprintf("%x", sha1sum(fmt.Sprintf("%s|%d|%d|%d", full, st.Size(), st.ModTime().UnixNano(), width)))
	dir := filepath.Join(s.thumbDir(), "space")
	os.MkdirAll(dir, 0o755)
	cached := filepath.Join(dir, key+".jpg")
	if b, err := os.ReadFile(cached); err == nil {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "private, max-age=86400")
		w.Write(b)
		return nil, nil
	}
	data, err := makeThumbWidth(r.Context(), full, 0, width)
	if err != nil {
		return nil, httpError{http.StatusUnsupportedMediaType, err.Error()}
	}
	os.WriteFile(cached, data, 0o644)
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Write(data)
	return nil, nil
}

func sha1sum(s string) []byte {
	h := sha1.New()
	h.Write([]byte(s))
	return h.Sum(nil)
}
