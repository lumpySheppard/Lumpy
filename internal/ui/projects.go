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
	return s.db.ProjectHealth(p.DriveIDs, p.Target)
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
		Name     string   `json:"name"`
		Target   int      `json:"target"`
		DriveIDs []string `json:"driveIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, badRequest("bad request: %v", err)
	}
	p, err := s.db.CreateProject(body.Name, body.Target)
	if err != nil {
		return nil, badRequest("%v", err)
	}
	for _, id := range body.DriveIDs {
		if err := s.db.SetProjectDrive(p.ID, id, true); err != nil {
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
		Name   *string  `json:"name"`
		Color  *string  `json:"color"`
		Notes  *string  `json:"notes"`
		Target *int     `json:"target"`
		Add    []string `json:"add"`
		Remove []string `json:"remove"`
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
	for _, id := range body.Add {
		if _, err := s.db.Drive(id); err != nil {
			return nil, badRequest("%v", err)
		}
		if err := s.db.SetProjectDrive(p.ID, id, true); err != nil {
			return nil, err
		}
	}
	for _, id := range body.Remove {
		if err := s.db.SetProjectDrive(p.ID, id, false); err != nil {
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

// spaces describes the project's drives to the planner, with live free
// space for drives that are plugged in.
func (s *Server) spaces(p *catalog.Project) ([]catalog.DriveSpace, map[string]*catalog.Drive, error) {
	drives := map[string]*catalog.Drive{}
	var out []catalog.DriveSpace
	for _, id := range p.DriveIDs {
		d, err := s.db.Drive(id)
		if err != nil {
			continue
		}
		drives[id] = d
		sp := catalog.DriveSpace{ID: id, Free: d.Free}
		if v := s.mountOf(id); v != nil {
			sp.Free, sp.Connected = v.Free, true
		}
		out = append(out, sp)
	}
	return out, drives, nil
}

// TransferView is a plan step with what the app needs to show and run it.
type TransferView struct {
	*catalog.Transfer
	Files     int                `json:"fileCount"`
	Ready     bool               `json:"ready"`   // both drives plugged in
	Running   bool               `json:"running"` // this step is being run now
	Busy      bool               `json:"busy"`    // one of its drives has another job running
	Folders   []catalog.Slice    `json:"folderList"`
	PlanFiles []catalog.PlanFile `json:"-"`
}

type soleGroup struct {
	DriveID string `json:"driveId"`
	Folder  string `json:"folder"`
	Files   int    `json:"files"`
	Bytes   int64  `json:"bytes"`
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
	spaces, _, err := s.spaces(p)
	if err != nil {
		return nil, err
	}
	plan := catalog.MakePlan(h, spaces)
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

	// Sole copies and unhashed files, grouped by drive and top folder.
	group := func(cs []*catalog.Content) []soleGroup {
		m := map[[2]string]*soleGroup{}
		var out []*soleGroup
		for _, c := range cs {
			for _, hd := range c.Holders {
				k := [2]string{hd.DriveID, catalog.TopFolder(hd.Path)}
				g := m[k]
				if g == nil {
					g = &soleGroup{DriveID: hd.DriveID, Folder: k[1]}
					m[k] = g
					out = append(out, g)
				}
				g.Files++
				g.Bytes += c.Size
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Bytes > out[j].Bytes })
		res := make([]soleGroup, len(out))
		for i, g := range out {
			res[i] = *g
		}
		return res
	}
	var unplaced []soleGroup
	if len(plan.Unplaced) > 0 {
		unplaced = group(plan.Unplaced)
	}
	type unknownGroup = soleGroup
	return map[string]any{
		"project":       p,
		"health":        h,
		"transfers":     transfers,
		"unplaced":      unplaced,
		"unplacedBytes": plan.UnplacedBytes,
		"reserve":       plan.Reserve,
		"sole":          group(h.Needs),
		"unknown":       group(h.Unhashed),
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
	spaces, drives, err := s.spaces(p)
	if err != nil {
		return nil, err
	}
	plan := catalog.MakePlan(h, spaces)
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
		folder = safeFolder(srcTitle)
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

// safeFolder turns a drive title into a folder name.
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
