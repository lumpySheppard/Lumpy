package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lumpysheppard/lumpy/internal/catalog"
	"github.com/lumpysheppard/lumpy/internal/job"
)

// ProjectView is a project with its health, as the app shows it.
type ProjectView struct {
	*catalog.Project
	Health *catalog.Health `json:"health"`
}

// healthEntry is a project's health as last computed, and the library
// it was computed for.
type healthEntry struct {
	key string
	at  time.Time
	h   *catalog.Health
}

// projectHealth caches each project's health until the library changes,
// and while a job keeps changing it, for summaryTTL at a time, since
// the Map and the Projects tab ask for every project on every poll.
func (s *Server) projectHealth(p *catalog.Project) (*catalog.Health, error) {
	ds, err := s.db.Drives()
	if err != nil {
		return nil, err
	}
	key := libraryKey(ds) + fmt.Sprintf("|%d|%v", p.Target, p.Sources)
	s.mu.Lock()
	e := s.healths[p.ID]
	s.mu.Unlock()
	if e != nil && (e.key == key || (s.jobs.AnyRunning() && time.Since(e.at) < summaryTTL)) {
		return e.h, nil
	}
	h, err := s.db.ProjectHealth(p.Sources, p.Target)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if s.healths == nil {
		s.healths = map[int64]*healthEntry{}
	}
	s.healths[p.ID] = &healthEntry{key: key, at: time.Now(), h: h}
	s.mu.Unlock()
	return h, nil
}

func (s *Server) projects(w http.ResponseWriter, r *http.Request) (any, error) {
	ps, err := s.db.Projects()
	if err != nil {
		return nil, err
	}
	out := make([]ProjectView, 0, len(ps))
	for _, p := range ps {
		h, err := s.projectHealth(p)
		if err != nil {
			return nil, err
		}
		out = append(out, ProjectView{p, h})
	}
	return out, nil
}

func (s *Server) projectFrom(r *http.Request) (*catalog.Project, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return nil, httpError{http.StatusNotFound, "no such project"}
	}
	p, err := s.db.Project(id)
	if err != nil {
		return nil, httpError{http.StatusNotFound, err.Error()}
	}
	return p, nil
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) (any, error) {
	var body struct {
		Name    string           `json:"name"`
		Target  int              `json:"target"`
		Sources []catalog.Source `json:"sources"`
		DestIDs []string         `json:"destIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, badRequest("bad request: %v", err)
	}
	p, err := s.db.CreateProject(body.Name, body.Target)
	if err != nil {
		return nil, badRequest("%v", err)
	}
	for _, src := range body.Sources {
		if err := s.db.AddSource(p.ID, src.DriveID, src.Folder); err != nil {
			return nil, badRequest("%v", err)
		}
	}
	if len(body.DestIDs) > 0 {
		if err := s.db.SetDests(p.ID, body.DestIDs); err != nil {
			return nil, err
		}
	}
	return s.db.Project(p.ID)
}

func (s *Server) updateProject(w http.ResponseWriter, r *http.Request) (any, error) {
	p, err := s.projectFrom(r)
	if err != nil {
		return nil, err
	}
	var body struct {
		Name    *string            `json:"name"`
		Color   *string            `json:"color"`
		Notes   *string            `json:"notes"`
		Target  *int               `json:"target"`
		Add     []catalog.Source   `json:"add"`
		Remove  []catalog.Source   `json:"remove"`
		Set     *catalog.Source    `json:"set"` // a folder's role, exclusions and routing
		DestIDs *[]string          `json:"destIds"`
		Cards   *catalog.CardRules `json:"cards"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, badRequest("bad request: %v", err)
	}
	if body.Name != nil {
		p.Name = *body.Name
	}
	if body.Color != nil {
		p.Color = *body.Color
	}
	if body.Notes != nil {
		p.Notes = *body.Notes
	}
	if body.Target != nil {
		p.Target = *body.Target
	}
	if body.Cards != nil {
		p.Cards = body.Cards
		// New orders: cards already plugged in get looked at again.
		s.cardMu.Lock()
		s.cardSeen = nil
		s.cardMu.Unlock()
		defer s.watchCards()
	}
	if err := s.db.UpdateProject(p); err != nil {
		return nil, badRequest("%v", err)
	}
	for _, src := range body.Add {
		if err := s.db.AddSource(p.ID, src.DriveID, src.Folder); err != nil {
			return nil, badRequest("%v", err)
		}
	}
	for _, src := range body.Remove {
		if err := s.db.RemoveSource(p.ID, src.DriveID, src.Folder); err != nil {
			return nil, badRequest("%v", err)
		}
	}
	if body.DestIDs != nil {
		if err := s.db.SetDests(p.ID, *body.DestIDs); err != nil {
			return nil, err
		}
	}
	if body.Set != nil {
		if err := s.db.SetSource(p.ID, *body.Set); err != nil {
			return nil, badRequest("%v", err)
		}
	}
	return s.db.Project(p.ID)
}

func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request) (any, error) {
	p, err := s.projectFrom(r)
	if err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, s.db.DeleteProject(p.ID)
}

// Candidate is a drive the plan may copy onto.
type Candidate struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Free      int64  `json:"free"`
	Connected bool   `json:"connected"`
}

// candidates lists where a project's backups may go, with live free
// space for drives that are plugged in: the chosen destinations, or
// every drive in the library.
func (s *Server) candidates(p *catalog.Project) ([]catalog.DriveSpace, []Candidate, map[string]*catalog.Drive, error) {
	all, err := s.db.Drives()
	if err != nil {
		return nil, nil, nil, err
	}
	drives := map[string]*catalog.Drive{}
	for _, d := range all {
		drives[d.ID] = d
	}
	// Copies stay inside the project: backup folders decide where they
	// go; failing that, the chosen destinations; failing that, the
	// project's own drives. Never a drive outside the project.
	allowed := map[string]bool{}
	for _, id := range p.BackupDrives() {
		allowed[id] = true
	}
	if len(allowed) == 0 {
		for _, id := range p.DestIDs {
			allowed[id] = true
		}
	}
	if len(allowed) == 0 {
		for _, id := range p.DriveIDs() {
			allowed[id] = true
		}
	}
	var spaces []catalog.DriveSpace
	var views []Candidate
	for _, d := range all {
		if len(allowed) > 0 && !allowed[d.ID] || len(allowed) == 0 && d.NoBackups {
			continue
		}
		sp := catalog.DriveSpace{ID: d.ID, Free: d.Free}
		if v := s.mountOf(d.ID); v != nil {
			sp.Free, sp.Connected = v.Free, true
		}
		spaces = append(spaces, sp)
		views = append(views, Candidate{d.ID, d.Title(), sp.Free, sp.Connected})
	}
	sort.SliceStable(views, func(i, j int) bool {
		if views[i].Connected != views[j].Connected {
			return views[i].Connected
		}
		return views[i].Free > views[j].Free
	})
	return spaces, views, drives, nil
}

func (s *Server) layout(p *catalog.Project, drives map[string]*catalog.Drive) func(catalog.Origin) string {
	return catalog.ProjectLayout(p.Sources, func(id string) string {
		if d := drives[id]; d != nil {
			return safeFolder(d.Title())
		}
		return safeFolder(id)
	})
}

// route says which project folder on a destination drive an origin's
// file is copied into, with its relative path kept.
func (s *Server) route(p *catalog.Project) func(catalog.Origin, string) (string, bool) {
	return func(o catalog.Origin, destID string) (string, bool) {
		if o.Source < 0 || o.Source >= len(p.Sources) {
			return "", false
		}
		if r := p.RouteFor(p.Sources[o.Source], destID); r != nil {
			return r.Folder, true
		}
		return "", false
	}
}

// Route is where one of a project's folders is copied on one drive.
type Route struct {
	DestID   string `json:"destId"`
	Folder   string `json:"folder"`
	Explicit bool   `json:"explicit"`
}

// routes lists, for each source folder, the project folders its copies
// go into on each candidate drive.
func (s *Server) routes(p *catalog.Project, spaces []catalog.DriveSpace) [][]Route {
	out := make([][]Route, len(p.Sources))
	for i, src := range p.Sources {
		out[i] = []Route{}
		for _, sp := range spaces {
			if sp.ID == src.DriveID {
				continue
			}
			if r := p.RouteFor(src, sp.ID); r != nil {
				out[i] = append(out[i], Route{DestID: sp.ID, Folder: r.Folder, Explicit: src.Into != nil && src.Into.DriveID == sp.ID})
			}
		}
	}
	return out
}

// TransferView is a plan step with what the app needs to show and run it.
type TransferView struct {
	*catalog.Transfer
	Files   int             `json:"fileCount"`
	Ready   bool            `json:"ready"`   // both drives plugged in
	Running bool            `json:"running"` // this step is being run now
	Busy    bool            `json:"busy"`    // one of its drives has another job running
	Folders []catalog.Slice `json:"folderList"`
}

// group is a set of files on one source folder, by top-level folder.
type group struct {
	Source int    `json:"source"`
	Folder string `json:"folder"`
	Files  int    `json:"files"`
	Bytes  int64  `json:"bytes"`
}

func groupBySource(cs []*catalog.Content) []group {
	m := map[[2]string]*group{}
	var out []*group
	for _, c := range cs {
		seen := map[int]bool{}
		for _, o := range c.Origins {
			if seen[o.Source] {
				continue
			}
			seen[o.Source] = true
			k := [2]string{strconv.Itoa(o.Source), catalog.TopFolder(o.Rel)}
			g := m[k]
			if g == nil {
				g = &group{Source: o.Source, Folder: k[1]}
				m[k] = g
				out = append(out, g)
			}
			g.Files++
			g.Bytes += c.Size
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Bytes > out[j].Bytes })
	res := make([]group, len(out))
	for i, g := range out {
		res[i] = *g
	}
	return res
}

func (s *Server) projectDetail(w http.ResponseWriter, r *http.Request) (any, error) {
	p, err := s.projectFrom(r)
	if err != nil {
		return nil, err
	}
	h, err := s.projectHealth(p)
	if err != nil {
		return nil, err
	}
	spaces, cands, drives, err := s.candidates(p)
	if err != nil {
		return nil, err
	}
	plan := catalog.MakePlanRouted(h, spaces, s.layout(p, drives), s.route(p))
	transfers := make([]TransferView, 0, len(plan.Transfers))
	for _, t := range plan.Transfers {
		tv := TransferView{Transfer: t, Files: len(t.Files),
			Ready:   s.mountOf(t.SourceID) != nil && s.mountOf(t.DestID) != nil,
			Running: s.jobs.RunningPair(t.SourceID, t.DestID)}
		tv.Busy = !tv.Running && (s.jobs.Busy(t.SourceID) || s.jobs.Busy(t.DestID))
		for name, b := range t.Folders {
			tv.Folders = append(tv.Folders, catalog.Slice{Name: name, Bytes: b})
		}
		sort.Slice(tv.Folders, func(i, j int) bool { return tv.Folders[i].Bytes > tv.Folders[j].Bytes })
		t.Files = nil // the file list is fetched when a step runs; it can be huge
		transfers = append(transfers, tv)
	}
	if cands == nil {
		cands = []Candidate{}
	}
	return map[string]any{
		"project":       p,
		"health":        h,
		"masters":       s.masters(p),
		"routes":        s.routes(p, spaces),
		"backupDrives":  p.BackupDrives(),
		"transfers":     transfers,
		"candidates":    cands,
		"unplaced":      groupBySource(plan.Unplaced),
		"unplacedBytes": plan.UnplacedBytes,
		"reserve":       plan.Reserve,
		"sole":          groupBySource(h.Needs),
		"unknown":       groupBySource(h.Unhashed),
	}, nil
}

// runTransfer starts one plan step: the files are planned again now, so
// the step reflects the library as it is.
func (s *Server) runTransfer(w http.ResponseWriter, r *http.Request) (any, error) {
	p, err := s.projectFrom(r)
	if err != nil {
		return nil, err
	}
	var body struct {
		SourceID    string `json:"sourceId"`
		DestID      string `json:"destId"`
		Folder      string `json:"folder"`
		Into        string `json:"into"` // the routed step's project folder on the destination, "" for the unrouted step
		SourceCheck *bool  `json:"sourceCheck"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, badRequest("bad request: %v", err)
	}
	src, dst := s.mountOf(body.SourceID), s.mountOf(body.DestID)
	if src == nil || dst == nil {
		return nil, badRequest("both drives must be plugged in")
	}
	h, err := s.projectHealth(p)
	if err != nil {
		return nil, err
	}
	spaces, _, drives, err := s.candidates(p)
	if err != nil {
		return nil, err
	}
	plan := catalog.MakePlanRouted(h, spaces, s.layout(p, drives), s.route(p))
	var step *catalog.Transfer
	for _, t := range plan.Transfers {
		if t.SourceID == src.ID && t.DestID == dst.ID && t.Folder == body.Into {
			step = t
		}
	}
	if step == nil {
		return nil, badRequest("nothing needs copying from %s to %s any more; the plan has changed", src.Name, dst.Name)
	}
	srcTitle, dstTitle := src.Name, dst.Name
	if d := drives[src.ID]; d != nil {
		srcTitle = d.Title()
	}
	if d := drives[dst.ID]; d != nil {
		dstTitle = d.Title()
	}
	folder := strings.TrimSpace(body.Folder)
	if step.Folder != "" {
		folder = step.Folder // the project's own folder on that drive
	} else if folder == "" {
		folder = safeFolder(p.Name)
	}
	skip := body.SourceCheck != nil && !*body.SourceCheck
	title := fmt.Sprintf("%s: copy %s from %s to %s", p.Name, job.Bytes(step.Bytes), srcTitle, dstTitle)
	cmd := fmt.Sprintf("lumpy project transfer %s --from %s --to %s --folder %s", shellQuote(p.Name), shellQuote(srcTitle), shellQuote(dstTitle), shellQuote(folder))
	files := step.Files
	j := s.jobs.StartWhenFree("transfer", title, cmd, []string{src.ID, dst.ID}, func(ctx context.Context, rep *job.Report) error {
		return job.Transfer(ctx, job.TransferOptions{Source: src, Dest: dst, Folder: folder, Files: files,
			Meta: s.meta, Library: s.db, SkipSourceVerify: skip}, rep)
	})
	return map[string]any{"job": j.ID}, nil
}

// safeFolder turns a name into a folder name.
func safeFolder(title string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(title) {
		switch {
		case r == '/' || r == ':' || r == 0:
			b.WriteRune('-')
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 || b.String() == "." || b.String() == ".." {
		return "Lumpy Backup"
	}
	return b.String()
}

// discover lists folders that look like projects, grouped by first
// word, for the Find projects sheet. ?ignore= lists tokens left out of
// names (default BS).
func (s *Server) discover(w http.ResponseWriter, r *http.Request) (any, error) {
	ignore := []string{"BS"}
	if v, ok := r.URL.Query()["ignore"]; ok {
		ignore = strings.Split(strings.Join(v, ","), ",")
	}
	groups, err := s.db.Discover(ignore)
	if err != nil {
		return nil, err
	}
	if groups == nil {
		groups = []catalog.Group{}
	}
	return groups, nil
}

// MasterView is a project's master print: a video whose name carries
// "master" (or is tagged master), with its open-matte sibling when one
// sits beside it.
type MasterView struct {
	DriveID   string `json:"driveId"`
	Drive     string `json:"drive"`
	Path      string `json:"path"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	Connected bool   `json:"connected"`
	Final     bool   `json:"final"` // under an OUTPUT or final folder
	Matte     *struct {
		DriveID string `json:"driveId"`
		Path    string `json:"path"`
		Name    string `json:"name"`
	} `json:"matte,omitempty"`
	Media *catalog.Media `json:"media,omitempty"`
}

// masters finds the project's master prints and their open-matte
// versions: every video under the project's folders with master in the
// name or tags, finals first, newest copy of each name once.
func (s *Server) masters(p *catalog.Project) []MasterView {
	out := []MasterView{}
	rows, err := s.db.ListMedia(catalog.MediaFilter{Project: p, Kind: "video", Limit: 5000})
	if err != nil {
		return out
	}
	isMaster := func(m *catalog.Media) bool {
		n := strings.ToLower(path.Base(m.Path))
		return strings.Contains(n, "master") || hasTag(m.Edits.Tags, "master")
	}
	isMatte := func(name string) bool {
		n := strings.ToLower(name)
		return strings.Contains(n, "open-matte") || strings.Contains(n, "openmatte") || strings.Contains(n, "open_matte") || strings.Contains(n, "open matte")
	}
	type key struct{ name string }
	seen := map[string]int{}
	var mattes []catalog.MediaRow
	for i := range rows {
		m := &rows[i]
		if isMatte(path.Base(m.Path)) {
			mattes = append(mattes, *m)
			continue
		}
		if !isMaster(&m.Media) {
			continue
		}
		name := path.Base(m.Path)
		connected := s.mountOf(m.DriveID) != nil
		// One row per name; a plugged-in copy beats one that isn't.
		if j, ok := seen[name]; ok {
			if connected && !out[j].Connected {
				out[j].DriveID, out[j].Path, out[j].Connected = m.DriveID, m.Path, true
				if d, err := s.db.Drive(m.DriveID); err == nil {
					out[j].Drive = d.Title()
				}
			}
			continue
		}
		mv := MasterView{DriveID: m.DriveID, Path: m.Path, Name: name, Size: m.Size, Connected: connected, Final: finalish(m.Path)}
		if d, err := s.db.Drive(m.DriveID); err == nil {
			mv.Drive = d.Title()
		}
		mm := m.Media
		mv.Media = &mm
		seen[name] = len(out)
		out = append(out, mv)
	}
	// An open-matte beside a master (same folder, or the same name with
	// the matte words added) belongs to it.
	for i := range out {
		dir := path.Dir(out[i].Path)
		stem := strings.ToLower(strings.TrimSuffix(out[i].Name, path.Ext(out[i].Name)))
		stem = strings.ReplaceAll(stem, "master", "")
		best := -1
		for j := range mattes {
			mt := &mattes[j]
			if path.Dir(mt.Path) != dir && mt.DriveID != out[i].DriveID {
				continue
			}
			ms := strings.ToLower(strings.TrimSuffix(path.Base(mt.Path), path.Ext(mt.Path)))
			for _, w := range []string{"open-matte", "openmatte", "open_matte", "open matte", "master"} {
				ms = strings.ReplaceAll(ms, w, "")
			}
			if path.Dir(mt.Path) == dir || strings.Trim(ms, "_- .") == strings.Trim(stem, "_- .") {
				best = j
				if path.Dir(mt.Path) == dir {
					break
				}
			}
		}
		if best >= 0 {
			mt := mattes[best]
			out[i].Matte = &struct {
				DriveID string `json:"driveId"`
				Path    string `json:"path"`
				Name    string `json:"name"`
			}{mt.DriveID, mt.Path, path.Base(mt.Path)}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Final != out[j].Final {
			return out[i].Final
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// finalish says whether a path sits under an output or final folder.
func finalish(p string) bool {
	for _, d := range strings.Split(strings.ToLower(path.Dir(p)), "/") {
		if strings.Contains(d, "output") || strings.Contains(d, "final") || strings.Contains(d, "deliver") || strings.Contains(d, "export") {
			return true
		}
	}
	return false
}

// hasTag says whether a comma-separated tag list carries tag.
func hasTag(tags, tag string) bool {
	for _, t := range strings.Split(tags, ",") {
		if strings.EqualFold(strings.TrimSpace(t), tag) {
			return true
		}
	}
	return false
}
