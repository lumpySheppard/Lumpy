package ui

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/lumpysheppard/lumpy/internal/catalog"
	"github.com/lumpysheppard/lumpy/internal/edit"
	"github.com/lumpysheppard/lumpy/internal/job"
)

// An archive analysis is kept in memory under a token until it is
// applied, so the apply step works on exactly what the user saw.
type archiveRun struct {
	project int64
	sources []catalog.Source
	report  *catalog.ArchiveReport
	files   []string
	at      time.Time
}

const (
	maxArchiveBody = 256 << 20
	maxUnusedShown = 500
	maxNotFound    = 300
)

// ArchiveView is an analysis as the app shows it.
type ArchiveView struct {
	Token         string                  `json:"token"`
	Files         []string                `json:"files"`
	Refs          int                     `json:"refs"`
	Found         int                     `json:"found"`
	NotFound      []string                `json:"notFound"`
	NotFoundTotal int                     `json:"notFoundTotal"`
	UsedFiles     int                     `json:"usedFiles"`
	UsedBytes     int64                   `json:"usedBytes"`
	UnusedFiles   int                     `json:"unusedFiles"`
	UnusedBytes   int64                   `json:"unusedBytes"`
	Folders       []catalog.ArchiveFolder `json:"folders"`
	Unused        []catalog.ArchiveItem   `json:"unused"`
	Drives        []ArchiveDrive          `json:"drives"`
}

// ArchiveDrive says how much unused footage sits on a drive and whether
// it is plugged in, which the actions need.
type ArchiveDrive struct {
	ID          string `json:"id"`
	Connected   bool   `json:"connected"`
	UnusedFiles int    `json:"unusedFiles"`
	UnusedBytes int64  `json:"unusedBytes"`
}

func (s *Server) archiveView(token string, run *archiveRun) ArchiveView {
	rep := run.report
	v := ArchiveView{Token: token, Files: run.files, Refs: rep.Refs, Found: rep.Found, NotFound: rep.NotFound, NotFoundTotal: len(rep.NotFound),
		UsedFiles: rep.UsedFiles, UsedBytes: rep.UsedBytes, UnusedFiles: rep.UnusedFiles, UnusedBytes: rep.UnusedBytes, Folders: rep.Folders, Unused: []catalog.ArchiveItem{}, Drives: []ArchiveDrive{}}
	if len(v.NotFound) > maxNotFound {
		v.NotFound = v.NotFound[:maxNotFound]
	}
	drives := map[string]*ArchiveDrive{}
	var order []string
	for _, it := range rep.Items {
		if it.Used {
			continue
		}
		if len(v.Unused) < maxUnusedShown {
			v.Unused = append(v.Unused, it)
		}
		d := drives[it.DriveID]
		if d == nil {
			d = &ArchiveDrive{ID: it.DriveID, Connected: s.mountOf(it.DriveID) != nil}
			drives[it.DriveID] = d
			order = append(order, it.DriveID)
		}
		d.UnusedFiles++
		d.UnusedBytes += it.Bytes
	}
	for _, id := range order {
		v.Drives = append(v.Drives, *drives[id])
	}
	return v
}

func (s *Server) keepArchive(run *archiveRun) string {
	b := make([]byte, 12)
	rand.Read(b)
	token := hex.EncodeToString(b)
	s.archMu.Lock()
	defer s.archMu.Unlock()
	if s.archives == nil {
		s.archives = map[string]*archiveRun{}
	}
	for k, r := range s.archives {
		if time.Since(r.at) > time.Hour {
			delete(s.archives, k)
		}
	}
	s.archives[token] = run
	return token
}

func (s *Server) takeArchive(token string) *archiveRun {
	s.archMu.Lock()
	defer s.archMu.Unlock()
	run := s.archives[token]
	delete(s.archives, token)
	return run
}

// analyseArchive reads one or more timeline exports (paths on this Mac,
// or uploaded text) and says what of the project they use.
func (s *Server) analyseArchive(w http.ResponseWriter, r *http.Request) (any, error) {
	p, err := s.projectFrom(r)
	if err != nil {
		return nil, err
	}
	if len(p.Sources) == 0 {
		return nil, badRequest("add the project's folders first, so there is footage to compare the edit with")
	}
	var body struct {
		Paths   []string `json:"paths"`
		Uploads []struct {
			Name string `json:"name"`
			Text string `json:"text"`
		} `json:"uploads"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxArchiveBody)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, badRequest("bad request: %v", err)
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
			return nil, badRequest("%v", err)
		}
		if st.Size() > maxArchiveBody {
			return nil, badRequest("%s is too big to be a timeline export", filepath.Base(pth))
		}
		data, err := os.ReadFile(pth)
		if err != nil {
			return nil, badRequest("%v", err)
		}
		if err := add(filepath.Base(pth), data); err != nil {
			return nil, badRequest("%v", err)
		}
	}
	for _, u := range body.Uploads {
		if err := add(u.Name, []byte(u.Text)); err != nil {
			return nil, badRequest("%v", err)
		}
	}
	if len(refs) == 0 {
		return nil, badRequest("choose a timeline export: FCPXML, Premiere XML, an EDL, or a list of clip names")
	}
	rep, err := s.db.Archive(p.Sources, refs)
	if err != nil {
		return nil, err
	}
	run := &archiveRun{project: p.ID, sources: p.Sources, report: rep, files: labels, at: time.Now()}
	return s.archiveView(s.keepArchive(run), run), nil
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// applyArchive moves the unused footage of an analysis into a folder
// inside each project folder (keeping paths), or to the Trash. One job
// per drive; every drive involved must be plugged in and idle.
func (s *Server) applyArchive(w http.ResponseWriter, r *http.Request) (any, error) {
	p, err := s.projectFrom(r)
	if err != nil {
		return nil, err
	}
	var body struct {
		Token  string `json:"token"`
		Action string `json:"action"`
		Folder string `json:"folder"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, badRequest("bad request: %v", err)
	}
	trash := body.Action == "trash"
	if !trash && body.Action != "move" {
		return nil, badRequest("action must be move or trash")
	}
	folder := strings.Trim(strings.ReplaceAll(strings.TrimSpace(body.Folder), "\\", "/"), "/")
	if folder == "" {
		folder = "_Unused"
	}
	if folder != path.Clean(folder) || strings.HasPrefix(folder, "../") || folder == ".." || strings.Contains(folder, "/../") || folder == "." {
		return nil, badRequest("not a folder name: %q", body.Folder)
	}
	s.archMu.Lock()
	run := s.archives[body.Token]
	s.archMu.Unlock()
	if run == nil || run.project != p.ID {
		return nil, badRequest("that analysis has expired; choose the timeline export again")
	}
	byDrive := map[string][]job.ArchiveOp{}
	var order []string
	for _, it := range run.report.Items {
		if it.Used {
			continue
		}
		op := job.ArchiveOp{Path: it.Path}
		if !trash {
			src := run.sources[it.Source]
			rel := it.Path
			if src.Folder != "" {
				rel = strings.TrimPrefix(it.Path, src.Folder+"/")
			}
			op.Dest = path.Join(src.Folder, folder, rel)
		}
		if _, ok := byDrive[it.DriveID]; !ok {
			order = append(order, it.DriveID)
		}
		byDrive[it.DriveID] = append(byDrive[it.DriveID], op)
	}
	if len(order) == 0 {
		return nil, badRequest("nothing is unused")
	}
	drives, _ := s.db.Drives()
	titles := map[string]string{}
	for _, d := range drives {
		titles[d.ID] = d.Title()
	}
	for _, id := range order {
		if s.mountOf(id) == nil {
			return nil, badRequest("%s is not plugged in", titles[id])
		}
		if s.jobs.Busy(id) {
			return nil, badRequest("%s already has a job running", titles[id])
		}
	}
	s.takeArchive(body.Token)
	var ids []int
	for _, id := range order {
		v := s.mountOf(id)
		ops := byDrive[id]
		title := fmt.Sprintf("Move %d unused clip%s on %s into %s", len(ops), plural(len(ops)), titles[id], folder)
		if trash {
			title = fmt.Sprintf("Move %d unused clip%s on %s to the Trash", len(ops), plural(len(ops)), titles[id])
		}
		j := s.jobs.Start("archive", title, "lumpy project archive "+shellQuote(p.Name), id, func(ctx context.Context, rep *job.Report) error {
			return job.ArchiveFiles(ctx, s.db, v, ops, trash, rep)
		})
		ids = append(ids, j.ID)
	}
	return map[string]any{"jobs": ids}, nil
}
