package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/lumpysheppard/lumpy/internal/catalog"
	"github.com/lumpysheppard/lumpy/internal/job"
)

// ProjectView is a project with its health, as the app shows it.
type ProjectView struct {
	*catalog.Project
	Health *catalog.Health `json:"health"`
}

func (s *Server) projectHealth(p *catalog.Project) (*catalog.Health, error) {
	return s.db.ProjectHealth(p.Sources, p.Target)
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
		Name    *string          `json:"name"`
		Color   *string          `json:"color"`
		Notes   *string          `json:"notes"`
		Target  *int             `json:"target"`
		Add     []catalog.Source `json:"add"`
		Remove  []catalog.Source `json:"remove"`
		DestIDs *[]string        `json:"destIds"`
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
	allowed := map[string]bool{}
	for _, id := range p.DestIDs {
		allowed[id] = true
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
	plan := catalog.MakePlan(h, spaces, s.layout(p, drives))
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
		SourceCheck *bool  `json:"sourceCheck"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, badRequest("bad request: %v", err)
	}
	src, dst := s.mountOf(body.SourceID), s.mountOf(body.DestID)
	if src == nil || dst == nil {
		return nil, badRequest("both drives must be plugged in")
	}
	if s.jobs.Busy(src.ID) || s.jobs.Busy(dst.ID) {
		return nil, badRequest("one of the drives already has a job running")
	}
	h, err := s.projectHealth(p)
	if err != nil {
		return nil, err
	}
	spaces, _, drives, err := s.candidates(p)
	if err != nil {
		return nil, err
	}
	plan := catalog.MakePlan(h, spaces, s.layout(p, drives))
	var step *catalog.Transfer
	for _, t := range plan.Transfers {
		if t.SourceID == src.ID && t.DestID == dst.ID {
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
	if folder == "" {
		folder = safeFolder(p.Name)
	}
	skip := body.SourceCheck != nil && !*body.SourceCheck
	title := fmt.Sprintf("%s: copy %s from %s to %s", p.Name, job.Bytes(step.Bytes), srcTitle, dstTitle)
	cmd := fmt.Sprintf("lumpy project transfer %s --from %s --to %s --folder %s", shellQuote(p.Name), shellQuote(srcTitle), shellQuote(dstTitle), shellQuote(folder))
	files := step.Files
	j := s.jobs.Start("transfer", title, cmd, src.ID, func(ctx context.Context, rep *job.Report) error {
		return job.Transfer(ctx, job.TransferOptions{Source: src, Dest: dst, Folder: folder, Files: files,
			Meta: s.meta, Library: s.db, SkipSourceVerify: skip}, rep)
	})
	s.jobs.mark(j, dst.ID)
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
