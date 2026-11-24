package ui

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/lumpysheppard/lumpy/internal/catalog"
	"github.com/lumpysheppard/lumpy/internal/job"
)

func TestClassify(t *testing.T) {
	for _, c := range []struct {
		rel  string
		want tidyCategory
	}{
		{"Day1/A001/CLIPS/A001_C001.braw", catCamera},
		{"loose/B002_C004.mov", catCamera},
		{"Proxies/A001_C001_proxy.mp4", catProxy},
		{"Offline/A001_C001.mp4", catProxy},
		{"Exports/final/Film_master_16x9.mov", catOutput},
		{"renders/v3.mov", catOutput},
		{"Sound/SC01_T01.wav", catAudio},
		{"Music/track.mp3", catMusic},
		{"stuff/poster.psd", catGraphics},
		{"stills/IMG_0001.jpg", catStills},
		{"Resolve/Film.drp", catProject},
		{"cut.fcpxml", catProject},
		{"callsheet.pdf", catDocs},
		{"notes.txt", catDocs},
		{"thing.bin", catOther},
	} {
		if got := classify(c.rel); got != c.want {
			t.Errorf("classify(%q) = %s, want %s", c.rel, got, c.want)
		}
	}
	folders := []string{"01_RAW", "02_PROXY", "03_AUDIO", "04_PROJECT", "09_OUTPUT"}
	if f := folderFor(catCamera, folders); f != "01_RAW" {
		t.Errorf("camera -> %q", f)
	}
	if f := folderFor(catDocs, folders); f != "" {
		t.Errorf("docs with no folder -> %q", f)
	}
}

// A loose project is planned into a template's folders and moved on
// the drive; the library follows, and the log of moves is written.
func TestTidyThroughAPI(t *testing.T) {
	s, ts, vols := setup(t)
	tpl := filepath.Join(t.TempDir(), "House.pht", "Files")
	for _, f := range []string{"01_RAW", "02_PROXY", "03_AUDIO", "04_PROJECT", "09_OUTPUT"} {
		os.MkdirAll(filepath.Join(tpl, f), 0o755)
	}
	root := filepath.Join(vols, "RAID", "Short")
	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(body), 0o644)
	}
	write("Day1/A001/CLIPS/A001_C001.braw", "raw one")
	write("Exports/final/Short_master.mov", "master")
	write("Sound/SC01.wav", "sound")
	write("Short.drp", "project")
	write("01_RAW/Day0/A000_C001.braw", "already there")
	write("notes.txt", "no docs folder in the template")
	job.AgentRunWith(context.Background(), s.db, job.NewReportTo(io.Discard), func(string, string) {}, nil)
	var st struct{ Drives []DriveView }
	stateSettled(t, s, ts, &st)
	var p catalog.Project
	call(t, s, ts, "POST", "/api/projects", `{"name":"Short","target":2,"sources":[{"driveId":"`+st.Drives[0].ID+`","folder":"Short"}]}`, &p)
	pid := strconv.FormatInt(p.ID, 10)
	var plan tidyPlan
	if code := call(t, s, ts, "POST", "/api/projects/"+pid+"/tidy/plan", `{"template":"`+filepath.Dir(tpl)+`"}`, &plan); code != 200 {
		t.Fatalf("plan: %d", code)
	}
	if plan.Files != 6 || plan.InPlace != 1 || plan.Unsorted != 1 || plan.Moving != 4 {
		t.Fatalf("plan: files %d in place %d unsorted %d moving %d\n%+v", plan.Files, plan.InPlace, plan.Unsorted, plan.Moving, plan.Groups)
	}
	var moves []map[string]any
	for _, g := range plan.Groups {
		if !g.InPlace && g.Dest != "" {
			moves = append(moves, map[string]any{"driveId": g.DriveID, "source": g.Source, "top": g.Top, "category": g.Category, "dest": g.Dest})
		}
	}
	raw, _ := json.Marshal(map[string]any{"template": filepath.Dir(tpl), "moves": moves})
	body := string(raw)
	var started struct{ Job int }
	if code := call(t, s, ts, "POST", "/api/projects/"+pid+"/tidy/run", body, &started); code != 200 {
		t.Fatalf("run: %d", code)
	}
	jobs := waitJobs(t, s, ts)
	for _, j := range jobs {
		if j.ID == started.Job && j.State != "ok" {
			t.Fatalf("tidy job %s: %q %v", j.State, j.Error, j.Lines)
		}
	}
	for _, want := range []string{"01_RAW/Day1/A001/CLIPS/A001_C001.braw", "09_OUTPUT/final/Short_master.mov", "03_AUDIO/SC01.wav", "04_PROJECT/Short.drp", "01_RAW/Day0/A000_C001.braw", "notes.txt", "02_PROXY"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(want))); err != nil {
			t.Errorf("after tidy: %s missing", want)
		}
	}
	for _, gone := range []string{"Day1", "Exports", "Sound", "Short.drp"} {
		if _, err := os.Stat(filepath.Join(root, gone)); err == nil {
			t.Errorf("after tidy: %s still there", gone)
		}
	}
	logs, _ := filepath.Glob(filepath.Join(root, "_Lumpy tidy *.json"))
	if len(logs) != 1 {
		t.Fatalf("tidy log: %v", logs)
	}
	// The library knows the new paths without another scan being asked for.
	files, _ := s.db.Files(st.Drives[0].ID)
	seen := map[string]bool{}
	for _, f := range files {
		seen[f.Path] = true
	}
	if !seen["Short/09_OUTPUT/final/Short_master.mov"] || seen["Short/Exports/final/Short_master.mov"] {
		t.Fatalf("library after tidy: %v", seen)
	}
	// A second look finds everything in place.
	call(t, s, ts, "POST", "/api/projects/"+pid+"/tidy/plan", `{"template":"`+filepath.Dir(tpl)+`"}`, &plan)
	if plan.Moving != 0 || plan.InPlace != 5 {
		t.Fatalf("second plan: moving %d in place %d", plan.Moving, plan.InPlace)
	}
}
