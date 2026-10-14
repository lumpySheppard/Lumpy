package ui

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/lumpysheppard/lumpy/internal/catalog"
	"github.com/lumpysheppard/lumpy/internal/job"
)

func TestArchiveThroughAPI(t *testing.T) {
	s, ts, vols := setup(t)
	raid := filepath.Join(vols, "RAID")
	for _, p := range []string{
		"Job/Day01/A001/A001_C001_0101XY.RDC/A001_C001_0101XY_001.R3D",
		"Job/Day01/A001/A001_C001_0101XY.RDC/A001_C001_0101XY_002.R3D",
		"Job/Day01/A001/A001_C002_0101XY.RDC/A001_C002_0101XY_001.R3D",
		"Job/Day01/B001/C0001.MP4",
		"Job/Day02/B001/C0002.MP4",
		"Job/Day02/S001/T01.WAV",
		"Job/Notes/readme.txt",
	} {
		full := filepath.Join(raid, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte("x "+p), 0o644)
	}
	job.AgentRunWith(context.Background(), s.db, job.NewReportTo(io.Discard), func(string, string) {}, nil)
	var st struct{ Drives []DriveView }
	call(t, s, ts, "GET", "/api/state", "", &st)
	id := st.Drives[0].ID
	var p catalog.Project
	call(t, s, ts, "POST", "/api/projects", `{"name":"Job","target":1,"sources":[{"driveId":"`+id+`","folder":"Job"}]}`, &p)
	pid := strconv.FormatInt(p.ID, 10)

	// The cut uses the first RED clip and C0001; an EDL from disk and a
	// list uploaded from the browser both count.
	edl := filepath.Join(t.TempDir(), "cut.edl")
	os.WriteFile(edl, []byte("TITLE: Cut\n001  A001 V C 01:00:00:00 01:00:01:00 00:00:00:00 00:00:01:00\n* FROM CLIP NAME: A001_C001_0101XY_001.R3D\n"), 0o644)
	var a ArchiveView
	if code := call(t, s, ts, "POST", "/api/projects/"+pid+"/archive", `{"paths":["`+strings.ReplaceAll(edl, `\`, `\\`)+`"],"uploads":[{"name":"list.txt","text":"C0001.MP4\nmissing.mov\n"}]}`, &a); code != 200 {
		t.Fatalf("analyse: %d", code)
	}
	if a.Refs != 3 || a.Found != 2 || a.NotFoundTotal != 1 || a.NotFound[0] != "missing.mov" || a.UsedFiles != 2 || a.UnusedFiles != 3 || len(a.Unused) != 3 || len(a.Files) != 2 || len(a.Drives) != 1 || !a.Drives[0].Connected {
		t.Fatalf("view: %+v", a)
	}
	if a.Unused[0].Path != "Job/Day01/A001/A001_C002_0101XY.RDC" || a.Unused[0].Files != 1 {
		t.Errorf("unused: %+v", a.Unused)
	}

	// Move only Day02's unused clips aside: Day01's RED clip stays.
	var started struct{ Jobs []int }
	if code := call(t, s, ts, "POST", "/api/projects/"+pid+"/archive/apply", `{"token":"`+a.Token+`","action":"move","folder":"_Unused","folders":[{"driveId":"`+id+`","source":"Job","folder":"Day02"}]}`, &started); code != 200 || len(started.Jobs) != 1 {
		t.Fatalf("apply: %d %+v", code, started)
	}
	for _, j := range waitJobs(t, s, ts) {
		if j.State != "ok" {
			t.Errorf("job %d: %s %s", j.ID, j.State, j.Error)
		}
	}
	if _, err := os.Stat(filepath.Join(raid, "Job", "Day01", "A001", "A001_C002_0101XY.RDC")); err != nil {
		t.Errorf("Day01 should be untouched: %v", err)
	}
	if _, err := os.Stat(filepath.Join(raid, "Job", "_Unused", "Day02", "B001", "C0002.MP4")); err != nil {
		t.Errorf("Day02 should be moved: %v", err)
	}
	// Then everything: the RED clip moves, what is already in _Unused is
	// left where it is rather than nested deeper.
	if code := call(t, s, ts, "POST", "/api/projects/"+pid+"/archive", `{"paths":["`+strings.ReplaceAll(edl, `\`, `\\`)+`"],"uploads":[{"name":"list.txt","text":"C0001.MP4\n"}]}`, &a); code != 200 || a.UnusedFiles != 3 {
		t.Fatalf("analyse again: %d %+v", code, a)
	}
	if code := call(t, s, ts, "POST", "/api/projects/"+pid+"/archive/apply", `{"token":"`+a.Token+`","action":"move","folder":"_Unused"}`, &started); code != 200 {
		t.Fatalf("apply all: %d", code)
	}
	for _, j := range waitJobs(t, s, ts) {
		if j.State != "ok" {
			t.Errorf("job %d: %s %s", j.ID, j.State, j.Error)
		}
	}
	if _, err := os.Stat(filepath.Join(raid, "Job", "_Unused", "_Unused")); err == nil {
		t.Error("_Unused was nested inside itself")
	}
	for _, p := range []string{"Job/_Unused/Day01/A001/A001_C002_0101XY.RDC/A001_C002_0101XY_001.R3D", "Job/_Unused/Day02/B001/C0002.MP4", "Job/_Unused/Day02/S001/T01.WAV", "Job/Day01/B001/C0001.MP4"} {
		if _, err := os.Stat(filepath.Join(raid, filepath.FromSlash(p))); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
	if _, err := os.Stat(filepath.Join(raid, "Job", "Day02", "B001", "C0002.MP4")); err == nil {
		t.Error("C0002 still in place")
	}
	files, _ := s.db.Files(id)
	have := map[string]bool{}
	for _, f := range files {
		have[f.Path] = true
	}
	if !have["Job/_Unused/Day02/B001/C0002.MP4"] || have["Job/Day02/B001/C0002.MP4"] || len(files) != 7 {
		t.Errorf("library after move: %v", have)
	}
	// The token is spent.
	if code := call(t, s, ts, "POST", "/api/projects/"+pid+"/archive/apply", `{"token":"`+a.Token+`","action":"trash"}`, nil); code != 400 {
		t.Errorf("reuse: %d", code)
	}

	// Analyse again and trash what is left unused (the _Unused folder is
	// in the project too), which removes the files and their rows.
	if code := call(t, s, ts, "POST", "/api/projects/"+pid+"/archive", `{"uploads":[{"name":"list.txt","text":"A001_C001_0101XY_002.R3D\nC0001.MP4\n"}]}`, &a); code != 200 {
		t.Fatalf("analyse 2: %d", code)
	}
	if a.UnusedFiles != 3 {
		t.Fatalf("unused 2: %+v", a)
	}
	call(t, s, ts, "POST", "/api/projects/"+pid+"/archive/apply", `{"token":"`+a.Token+`","action":"trash"}`, &started)
	for _, j := range waitJobs(t, s, ts) {
		if j.State != "ok" {
			t.Errorf("job %d: %s %s", j.ID, j.State, j.Error)
		}
	}
	if _, err := os.Stat(filepath.Join(raid, "Job", "_Unused", "Day02", "B001", "C0002.MP4")); err == nil {
		t.Error("C0002 not trashed")
	}
	files, _ = s.db.Files(id)
	if len(files) != 4 {
		t.Errorf("library after trash: %d files", len(files))
		for _, f := range files {
			t.Log(f.Path)
		}
	}
}
