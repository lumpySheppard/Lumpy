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
		{"CRM/Z002C056_26091608_CANON.CRM", catCamera},
		{"CONTENTS/Z002C077_2609168Q_CANON.MXF", catCamera},
		{"cards/C0012.MP4", catCamera},
		{"phone/IMG_4821.MOV", catCamera},
		{"DJI_0042.MP4", catCamera},
		{"PRIVATE/M4ROOT/CLIP/anything.MP4", catCamera},
		{"ARI_FINAL_OCT20.mp4", catDraft},
		{"DDD_2_CLEAN.mov", catDraft},
		{"reels/dive_reel_1.mp4", catDraft},
		{"Col_Pass_1_proxy.mp4", catProxy},
		{"Proxies/A001_C001_proxy.mp4", catProxy},
		{"Offline/A001_C001.mp4", catProxy},
		{"Exports/final/Film_master_16x9.mov", catMaster},
		{"Film_master_open-matte.mov", catMaster},
		{"renders/v3.mov", catDraft},
		{"Exports/social_9x16.mov", catDraft},
		{"Sound/SC01_T01.wav", catAudio},
		{"Music/track.mp3", catMusic},
		{"stuff/poster.psd", catGraphics},
		{"GFX/lower_third.mov", catGfxExport},
		{"stills/IMG_0001.jpg", catOutStill},
		{"photos/IMG_0001.jpg", catStills},
		{"stock/ocean.mov", catStock},
		{"Resolve/Film.drp", catResolve},
		{"edit/Film.prproj", catPremiere},
		{"cut.fcpxml", catProject},
		{"Film_script_v4.pdf", catScript},
		{"callsheet.pdf", catDocs},
		{"notes.txt", catDocs},
		{"thing.bin", catOther},
	} {
		if got := classify(c.rel); got != c.want {
			t.Errorf("classify(%q) = %s, want %s", c.rel, got, c.want)
		}
	}
}

// A proxy is known by its original's name: the same stem in a smaller
// codec, whether the stem reads like a camera's or not.
func TestProxiesByStem(t *testing.T) {
	files := []tidyFile{
		{rel: "CRM/Z002C056_26091608_CANON.CRM", cat: catCamera},
		{rel: "offl/Z002C056_26091608_CANON.mp4", cat: catCamera},
		{rel: "cards/C0012.MP4", cat: catCamera},
		{rel: "ARI_FINAL_OCT20.mp4", cat: catDraft},
		{rel: "Day1/A001_C001.braw", cat: catCamera},
		{rel: "A001_C001.mov", cat: catCamera},
		{rel: "cut_v2.mov", cat: catDraft},
	}
	proxiesByStem(files)
	want := []tidyCategory{catCamera, catProxy, catCamera, catDraft, catCamera, catProxy, catDraft}
	for i, f := range files {
		if f.cat != want[i] {
			t.Errorf("%s: %s, want %s", f.rel, f.cat, want[i])
		}
	}
}

// houseTemplate writes the shipped template's shape into a temp folder.
func houseTemplate(t *testing.T) string {
	t.Helper()
	tpl := filepath.Join(t.TempDir(), "House.pht")
	for _, f := range []string{"01_Original footage/Offline", "01_Original footage/Online", "02_Scratch disk", "03_Project files/DRP", "03_Project files/Premiere",
		"04_Graphics Sources", "05_Graphics exports", "06_Audio sources", "07_Stock footage", "08_Production paperwork/Scripts", "09_OUTPUT/Drafts", "09_OUTPUT/Final", "09_OUTPUT/Stills"} {
		os.MkdirAll(filepath.Join(tpl, "Files", filepath.FromSlash(f)), 0o755)
	}
	return tpl
}

func TestFolderForHouseTemplate(t *testing.T) {
	tv, err := templateView(houseTemplate(t))
	if err != nil {
		t.Fatal(err)
	}
	for cat, want := range map[tidyCategory]string{
		catCamera: "01_Original footage/Online", catProxy: "01_Original footage/Offline", catResolve: "03_Project files/DRP", catPremiere: "03_Project files/Premiere",
		catProject: "03_Project files", catGraphics: "04_Graphics Sources", catGfxExport: "05_Graphics exports", catAudio: "06_Audio sources", catMusic: "06_Audio sources",
		catStock: "07_Stock footage", catScript: "08_Production paperwork/Scripts", catDocs: "08_Production paperwork", catMaster: "09_OUTPUT/Final", catDraft: "09_OUTPUT/Drafts",
		catOutStill: "09_OUTPUT/Stills", catStills: "", catOther: "",
	} {
		if got := folderFor(cat, tv); got != want {
			t.Errorf("%s -> %q, want %q", cat, got, want)
		}
	}
}

// A loose project is planned into the house template's folders and
// moved on the drive; the library follows, and the log of moves is
// written.
func TestTidyThroughAPI(t *testing.T) {
	s, ts, vols := setup(t)
	tpl := houseTemplate(t)
	root := filepath.Join(vols, "RAID", "Short")
	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(body), 0o644)
	}
	write("Day1/A001/CLIPS/A001_C001.braw", "raw one")
	write("Exports/final/Short_master.mov", "master")
	write("Exports/social_9x16.mov", "social")
	write("Sound/SC01.wav", "sound")
	write("Short.drp", "project")
	write("01_Original footage/Online/Day0/A000_C001.braw", "already there")
	write("photos/IMG_0001.jpg", "no folder for photos in the template")
	job.AgentRunWith(context.Background(), s.db, job.NewReportTo(io.Discard), func(string, string) {}, nil)
	var st struct{ Drives []DriveView }
	stateSettled(t, s, ts, &st)
	var p catalog.Project
	call(t, s, ts, "POST", "/api/projects", `{"name":"Short","target":2,"sources":[{"driveId":"`+st.Drives[0].ID+`","folder":"Short"}]}`, &p)
	pid := strconv.FormatInt(p.ID, 10)
	var plan tidyPlan
	if code := call(t, s, ts, "POST", "/api/projects/"+pid+"/tidy/plan", `{"template":"`+tpl+`"}`, &plan); code != 200 {
		t.Fatalf("plan: %d", code)
	}
	if plan.Files != 7 || plan.InPlace != 1 || plan.Unsorted != 1 || plan.Moving != 5 {
		t.Fatalf("plan: files %d in place %d unsorted %d moving %d\n%+v", plan.Files, plan.InPlace, plan.Unsorted, plan.Moving, plan.Groups)
	}
	var moves []map[string]any
	for _, g := range plan.Groups {
		if !g.InPlace && g.Dest != "" {
			moves = append(moves, map[string]any{"driveId": g.DriveID, "source": g.Source, "top": g.Top, "category": g.Category, "dest": g.Dest})
		}
	}
	raw, _ := json.Marshal(map[string]any{"template": tpl, "moves": moves})
	var started struct{ Job int }
	if code := call(t, s, ts, "POST", "/api/projects/"+pid+"/tidy/run", string(raw), &started); code != 200 {
		t.Fatalf("run: %d", code)
	}
	jobs := waitJobs(t, s, ts)
	for _, j := range jobs {
		if j.ID == started.Job && j.State != "ok" {
			t.Fatalf("tidy job %s: %q %v", j.State, j.Error, j.Lines)
		}
	}
	for _, want := range []string{
		"01_Original footage/Online/Day1/A001/CLIPS/A001_C001.braw", "09_OUTPUT/Final/Short_master.mov", "09_OUTPUT/Drafts/social_9x16.mov",
		"06_Audio sources/SC01.wav", "03_Project files/DRP/Short.drp", "01_Original footage/Online/Day0/A000_C001.braw", "photos/IMG_0001.jpg", "02_Scratch disk", "08_Production paperwork/Scripts",
	} {
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
	files, _ := s.db.Files(st.Drives[0].ID)
	seen := map[string]bool{}
	for _, f := range files {
		seen[f.Path] = true
	}
	if !seen["Short/09_OUTPUT/Final/Short_master.mov"] || seen["Short/Exports/final/Short_master.mov"] {
		t.Fatalf("library after tidy: %v", seen)
	}
	call(t, s, ts, "POST", "/api/projects/"+pid+"/tidy/plan", `{"template":"`+tpl+`"}`, &plan)
	if plan.Moving != 0 || plan.InPlace != 6 {
		t.Fatalf("second plan: moving %d in place %d\n%+v", plan.Moving, plan.InPlace, plan.Groups)
	}
}

// The shipped template is written out beside the library and listed first.
func TestBuiltinTemplate(t *testing.T) {
	s, ts, _ := setup(t)
	var data struct{ Templates []TemplateView }
	call(t, s, ts, "GET", "/api/templates", "", &data)
	if len(data.Templates) == 0 || data.Templates[0].Name != "Blue Sheep Films" {
		t.Fatalf("templates: %+v", data.Templates)
	}
	tv := data.Templates[0]
	want := map[string]bool{"01_Original footage/Online": true, "09_OUTPUT/Final": true, "03_Project files/DRP": true}
	for _, f := range tv.Subfolders {
		delete(want, f)
	}
	if len(want) != 0 || len(tv.Folders) != 9 {
		t.Fatalf("shipped template: folders %v subfolders %v", tv.Folders, tv.Subfolders)
	}
	if _, err := os.Stat(filepath.Join(tv.Path, "Files", "01_Original footage", "Description.txt")); err != nil {
		t.Fatalf("notes inside the template: %v", err)
	}
}
