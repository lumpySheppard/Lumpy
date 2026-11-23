package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/lumpysheppard/lumpy/internal/catalog"
	"github.com/lumpysheppard/lumpy/internal/edit"
	"github.com/lumpysheppard/lumpy/internal/job"
	"github.com/lumpysheppard/lumpy/internal/volume"
)

// Pull shots: hand Lumpy an edit and it says where each clip's camera
// original is, across every drive, then copies the chosen ones into
// one folder. The analysis is kept for an hour under a token, the way
// Archive keeps its runs, so the copy acts on what the page showed.

type pullRun struct {
	report *catalog.PullReport
	files  []string
	at     time.Time
}

// PullView is the analysis as the page shows it.
type PullView struct {
	Token   string              `json:"token"`
	Files   []string            `json:"files"`
	Refs    []catalog.PullRef   `json:"refs"`
	Found   int                 `json:"found"`
	Missing []string            `json:"missing"`
	Drives  []catalog.PullDrive `json:"drives"`
}

func (s *Server) readExports(w http.ResponseWriter, r *http.Request) ([]catalog.ArchiveRef, []string, error) {
	var body struct {
		Paths   []string `json:"paths"`
		Uploads []struct {
			Name string `json:"name"`
			Text string `json:"text"`
		} `json:"uploads"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxArchiveBody)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, nil, badRequest("bad request: %v", err)
	}
	var refs []catalog.ArchiveRef
	var labels []string
	add := func(name string, data []byte) error {
		rs, kind, err := edit.Parse(name, data)
		if err != nil {
			return err
		}
		if len(rs) == 0 {
			return fmt.Errorf("%s refers to no media files", name)
		}
		for _, x := range rs {
			refs = append(refs, catalog.ArchiveRef{Name: x.Name, Path: x.Path})
		}
		labels = append(labels, fmt.Sprintf("%s · %s · %d clip%s", name, kind, len(rs), plural(len(rs))))
		return nil
	}
	for _, pth := range body.Paths {
		st, err := os.Stat(pth)
		if err != nil {
			return nil, nil, badRequest("%v", err)
		}
		if st.Size() > maxArchiveBody {
			return nil, nil, badRequest("%s is too big to be a timeline export", filepath.Base(pth))
		}
		data, err := os.ReadFile(pth)
		if err != nil {
			return nil, nil, badRequest("%v", err)
		}
		if err := add(filepath.Base(pth), data); err != nil {
			return nil, nil, badRequest("%v", err)
		}
	}
	for _, u := range body.Uploads {
		if err := add(u.Name, []byte(u.Text)); err != nil {
			return nil, nil, badRequest("%v", err)
		}
	}
	if len(refs) == 0 {
		return nil, nil, badRequest("choose a timeline export: FCPXML, Premiere XML, an EDL, or a list of clip names")
	}
	return refs, labels, nil
}

func (s *Server) analysePull(w http.ResponseWriter, r *http.Request) (any, error) {
	refs, labels, err := s.readExports(w, r)
	if err != nil {
		return nil, err
	}
	rep, err := s.db.Pull(refs)
	if err != nil {
		return nil, err
	}
	run := &pullRun{report: rep, files: labels, at: time.Now()}
	s.archMu.Lock()
	if s.pulls == nil {
		s.pulls = map[string]*pullRun{}
	}
	for k, x := range s.pulls {
		if time.Since(x.at) > time.Hour {
			delete(s.pulls, k)
		}
	}
	token := newToken()
	s.pulls[token] = run
	s.archMu.Unlock()
	return PullView{Token: token, Files: labels, Refs: rep.Refs, Found: rep.Found, Missing: rep.Missing, Drives: rep.Drives}, nil
}

// runPull copies the chosen copies into Dest/Name/<drive>/<path>, one
// job per source drive, each queued until its drives are free. Drives
// not plugged in are reported, not copied.
func (s *Server) runPull(w http.ResponseWriter, r *http.Request) (any, error) {
	var body struct {
		Token string `json:"token"`
		Dest  string `json:"dest"` // a folder on a plugged-in drive
		Name  string `json:"name"` // the folder made inside it
		Picks []struct {
			DriveID string `json:"driveId"`
			Path    string `json:"path"`
		} `json:"picks"`
		SourceCheck *bool `json:"sourceCheck"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, badRequest("bad request: %v", err)
	}
	s.archMu.Lock()
	run := s.pulls[body.Token]
	s.archMu.Unlock()
	if run == nil {
		return nil, badRequest("that analysis has expired; choose the export again")
	}
	dest := filepath.Clean(strings.TrimSpace(body.Dest))
	st, err := os.Stat(dest)
	if err != nil || !st.IsDir() {
		return nil, badRequest("choose a folder to pull into")
	}
	dv, err := volume.Of(dest)
	if err != nil {
		return nil, badRequest("%s is not on a drive Lumpy can see: %v", dest, err)
	}
	rel, err := filepath.Rel(dv.Mount, dest)
	if err != nil || strings.HasPrefix(rel, "..") {
		return nil, badRequest("%s is not inside %s", dest, dv.Name)
	}
	name := safeFolder(body.Name)
	if strings.TrimSpace(body.Name) == "" {
		name = "Pull " + time.Now().Format("2006-01-02")
	}
	base := filepath.ToSlash(filepath.Join(rel, name))
	if rel == "." {
		base = name
	}
	// Allowed picks are the hits the analysis found, nothing else.
	allowed := map[string]bool{}
	for _, ref := range run.report.Refs {
		for _, h := range ref.Hits {
			allowed[h.DriveID+"\x00"+h.Path] = true
		}
	}
	byDrive := map[string][]catalog.PlanFile{}
	var order []string
	var missing, there []string
	for _, p := range body.Picks {
		if !allowed[p.DriveID+"\x00"+p.Path] {
			return nil, badRequest("%s is not one of the clips found", p.Path)
		}
		if p.DriveID == dv.ID {
			there = append(there, p.Path) // already on the destination drive
			continue
		}
		if s.mountOf(p.DriveID) == nil {
			missing = append(missing, p.DriveID)
			continue
		}
		files, err := s.db.PullFiles(p.DriveID, p.Path)
		if err != nil {
			return nil, err
		}
		if _, ok := byDrive[p.DriveID]; !ok {
			order = append(order, p.DriveID)
		}
		for _, f := range files {
			byDrive[p.DriveID] = append(byDrive[p.DriveID], catalog.PlanFile{Path: f.Path, Dest: f.Path, Size: f.Size, XXH64: f.XXH64})
		}
	}
	skip := body.SourceCheck != nil && !*body.SourceCheck
	var jobs []int
	for _, id := range order {
		src := s.mountOf(id)
		files := byDrive[id]
		d, _ := s.db.Drive(id)
		title := src.Name
		if d != nil {
			title = d.Title()
		}
		var total int64
		for _, f := range files {
			total += f.Size
		}
		folder := path.Join(base, safeFolder(title))
		jt := fmt.Sprintf("Pull %d clip%s (%s) from %s", len(files), plural(len(files)), job.Bytes(total), title)
		cmd := fmt.Sprintf("lumpy pull --from %s --to %s", shellQuote(title), shellQuote(filepath.Join(dest, name, safeFolder(title))))
		src, dv, files := src, dv, files
		j := s.jobs.StartWhenFree("transfer", jt, cmd, []string{src.ID, dv.ID}, func(ctx context.Context, rep *job.Report) error {
			return job.Transfer(ctx, job.TransferOptions{Source: src, Dest: dv, Folder: folder, Files: files, Meta: s.meta, Library: s.db, SkipSourceVerify: skip}, rep)
		})
		jobs = append(jobs, j.ID)
	}
	sort.Strings(missing)
	seen := map[string]bool{}
	missing = slices.DeleteFunc(missing, func(id string) bool { dup := seen[id]; seen[id] = true; return dup })
	return map[string]any{"jobs": jobs, "unplugged": missing, "there": there, "into": filepath.Join(dest, name)}, nil
}
