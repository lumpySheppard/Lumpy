package ui

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lumpysheppard/lumpy/internal/catalog"
	"github.com/lumpysheppard/lumpy/internal/job"
	"github.com/lumpysheppard/lumpy/internal/volume"
)

func setup(t *testing.T) (*Server, *httptest.Server, string) {
	t.Helper()
	vols := t.TempDir()
	t.Setenv("LUMPY_VOLUMES", vols)
	home := t.TempDir()
	db, err := catalog.Open(filepath.Join(home, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s := New(db, filepath.Join(home, "agent.lock"), job.Meta{Version: "test", Hostname: "test"})
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return s, ts, vols
}

func call(t *testing.T, s *Server, ts *httptest.Server, method, path, body string, out any) int {
	t.Helper()
	req, _ := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
	req.Header.Set("X-Lumpy-Token", s.token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	if out != nil && res.StatusCode < 300 {
		if err := json.Unmarshal(data, out); err != nil {
			t.Fatalf("%s %s: %v\n%s", method, path, err, data)
		}
	}
	return res.StatusCode
}

func TestAPIRequiresToken(t *testing.T) {
	_, ts, _ := setup(t)
	res, err := http.Get(ts.URL + "/api/state")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("got %d without a token", res.StatusCode)
	}
	page, _ := http.Get(ts.URL + "/")
	b, _ := io.ReadAll(page.Body)
	page.Body.Close()
	if !strings.Contains(string(b), "<title>Lumpy</title>") {
		t.Fatal("page not served")
	}
}

func TestOffloadAndLibraryThroughAPI(t *testing.T) {
	s, ts, vols := setup(t)
	clip := filepath.Join(vols, "A001", "CLIPS", "C001.mov")
	os.MkdirAll(filepath.Dir(clip), 0o755)
	os.WriteFile(clip, []byte("footage"), 0o644)
	os.Mkdir(filepath.Join(vols, "SHUTTLE"), 0o755)

	var started struct{ Job int }
	body := `{"source":"` + filepath.Join(vols, "A001") + `","dests":["` + filepath.Join(vols, "SHUTTLE", "Day01") + `"]}`
	if code := call(t, s, ts, "POST", "/api/offload", body, &started); code != 200 || started.Job == 0 {
		t.Fatalf("offload start: %d", code)
	}
	var jobs []JobView
	for i := 0; i < 100; i++ {
		call(t, s, ts, "GET", "/api/jobs", "", &jobs)
		if len(jobs) == 1 && jobs[0].State != "running" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if jobs[0].State != "ok" {
		t.Fatalf("offload ended %s: %v", jobs[0].State, jobs[0].Lines)
	}

	var st struct {
		Drives []DriveView
	}
	call(t, s, ts, "GET", "/api/state", "", &st)
	var card DriveView
	for _, d := range st.Drives {
		if d.Name == "A001" {
			card = d
		}
	}
	if card.ID == "" || card.Backups == nil || card.Backups.MinCopies != 1 {
		t.Fatalf("card not backed up once: %+v", card)
	}

	labels := `{"nickname":"Card A","client":"CTD","color":"blue"}`
	if code := call(t, s, ts, "PATCH", "/api/drives/"+card.ID, labels, nil); code != 200 {
		t.Fatalf("labels: %d", code)
	}
	var detail struct {
		Drive   DriveView
		Folders []catalog.FolderHealth
	}
	call(t, s, ts, "GET", "/api/drives/"+card.ID, "", &detail)
	if detail.Drive.Title != "Card A" || detail.Drive.Labels.Client != "CTD" || len(detail.Folders) != 1 || detail.Folders[0].Name != "CLIPS" || detail.Folders[0].BackedBytes != 7 {
		t.Fatalf("detail: %+v", detail)
	}
	var entries []catalog.Entry
	call(t, s, ts, "GET", "/api/drives/"+card.ID+"/list?dir=CLIPS", "", &entries)
	if len(entries) != 1 || entries[0].Name != "C001.mov" || entries[0].XXH64 == "" {
		t.Fatalf("list: %+v", entries)
	}

	// A drive that is not plugged in gets its check queued.
	shelf := filepath.Join(t.TempDir(), "A001")
	os.Rename(filepath.Join(vols, "A001"), shelf)
	s.volsAt = time.Time{}
	var q struct{ Queued bool }
	call(t, s, ts, "POST", "/api/drives/"+card.ID+"/verify", "", &q)
	if !q.Queued {
		t.Fatal("check of an unplugged drive was not queued")
	}

	// Plugging it back in while the app is open runs the queued check as
	// a job that Activity shows.
	os.Rename(filepath.Join(filepath.Dir(shelf), "A001"), filepath.Join(vols, "A001"))
	job.AgentRunWith(context.Background(), s.db, job.NewReportTo(io.Discard), func(string, string) {}, s.startQueuedCheck)
	var check JobView
	for i := 0; i < 100; i++ {
		call(t, s, ts, "GET", "/api/jobs", "", &jobs)
		if jobs[0].Kind == "verify" && jobs[0].State != "running" {
			check = jobs[0]
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if check.Title != "Queued check of Card A" || check.State != "ok" {
		t.Fatalf("queued check job: %+v", check)
	}

	req, _ := http.NewRequest("GET", ts.URL+"/api/export.csv?t="+s.token, nil)
	res, _ := http.DefaultClient.Do(req)
	csv, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(string(csv), "Card A,A001,") {
		t.Fatalf("csv:\n%s", csv)
	}
}

func TestProjectsThroughAPI(t *testing.T) {
	s, ts, vols := setup(t)
	os.MkdirAll(filepath.Join(vols, "A001", "CLIPS"), 0o755)
	os.WriteFile(filepath.Join(vols, "A001", "CLIPS", "C001.mov"), []byte("footage one"), 0o644)
	os.Mkdir(filepath.Join(vols, "RAID"), 0o755)
	os.Mkdir(filepath.Join(vols, "SPARE"), 0o755)
	job.AgentRunWith(context.Background(), s.db, job.NewReportTo(io.Discard), func(string, string) {}, nil)
	var st struct{ Drives []DriveView }
	call(t, s, ts, "GET", "/api/state", "", &st)
	ids := map[string]string{}
	for _, d := range st.Drives {
		ids[d.Name] = d.ID
	}
	// Offload the card to RAID only, then make a project of RAID + SPARE.
	var started struct{ Job int }
	call(t, s, ts, "POST", "/api/offload", `{"source":"`+filepath.Join(vols, "A001")+`","dests":["`+filepath.Join(vols, "RAID", "Day01")+`"]}`, &started)
	waitJobs(t, s, ts)

	var p catalog.Project
	if code := call(t, s, ts, "POST", "/api/projects", `{"name":"Feature","target":2,"driveIds":["`+ids["RAID"]+`","`+ids["SPARE"]+`"]}`, &p); code != 200 {
		t.Fatalf("create: %d", code)
	}
	var detail struct {
		Health    catalog.Health
		Transfers []TransferView
		Sole      []soleGroup
	}
	path := "/api/projects/" + strconv.FormatInt(p.ID, 10)
	call(t, s, ts, "GET", path, "", &detail)
	if detail.Health.Sole != 1 || len(detail.Transfers) != 1 || !detail.Transfers[0].Ready || len(detail.Sole) != 1 {
		t.Fatalf("detail: %+v %+v", detail.Health, detail.Transfers)
	}
	tr := detail.Transfers[0]
	var run struct{ Job int }
	if code := call(t, s, ts, "POST", path+"/transfer", `{"sourceId":"`+tr.SourceID+`","destId":"`+tr.DestID+`"}`, &run); code != 200 {
		t.Fatalf("transfer: %d", code)
	}
	jobs := waitJobs(t, s, ts)
	if jobs[0].Kind != "transfer" || jobs[0].State != "ok" {
		t.Fatalf("transfer job: %+v", jobs[0])
	}
	call(t, s, ts, "GET", path, "", &detail)
	if detail.Health.Sole != 0 || detail.Health.Backed != 1 || len(detail.Transfers) != 0 {
		t.Fatalf("after transfer: %+v", detail.Health)
	}
	if code := call(t, s, ts, "PATCH", path, `{"name":"Feature Film","target":3,"remove":["`+ids["SPARE"]+`"]}`, &p); code != 200 || p.Name != "Feature Film" || len(p.DriveIDs) != 1 {
		t.Fatalf("update: %d %+v", code, p)
	}
	if code := call(t, s, ts, "DELETE", path, "", nil); code != 200 {
		t.Fatalf("delete: %d", code)
	}
}

func waitJobs(t *testing.T, s *Server, ts *httptest.Server) []JobView {
	t.Helper()
	var jobs []JobView
	for i := 0; i < 200; i++ {
		call(t, s, ts, "GET", "/api/jobs", "", &jobs)
		running := false
		for _, j := range jobs {
			running = running || j.State == "running"
		}
		if !running {
			return jobs
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("jobs still running")
	return nil
}

func TestPlanStepsShowRunningAndBusy(t *testing.T) {
	s, ts, vols := setup(t)
	for _, n := range []string{"A", "B", "C"} {
		os.MkdirAll(filepath.Join(vols, n), 0o755)
	}
	os.WriteFile(filepath.Join(vols, "A", "clip.mov"), []byte("only here"), 0o644)
	job.AgentRunWith(context.Background(), s.db, job.NewReportTo(io.Discard), func(string, string) {}, nil)
	var st struct{ Drives []DriveView }
	call(t, s, ts, "GET", "/api/state", "", &st)
	ids := map[string]string{}
	for _, d := range st.Drives {
		ids[d.Name] = d.ID
	}
	va, _ := volume.Of(filepath.Join(vols, "A"))
	s.db.SaveHashes(va.ID, []catalog.HashUpdate{{Path: "clip.mov", Size: 9, XXH64: "abcd"}})
	var p catalog.Project
	call(t, s, ts, "POST", "/api/projects", `{"name":"P","target":3,"driveIds":["`+ids["A"]+`","`+ids["B"]+`","`+ids["C"]+`"]}`, &p)

	// Hold a transfer A -> B open and look at the plan.
	release := make(chan struct{})
	j := s.jobs.Start("transfer", "t", "", ids["A"], func(ctx context.Context, r *job.Report) error { <-release; return nil })
	s.jobs.mark(j, ids["B"])
	var detail struct{ Transfers []TransferView }
	call(t, s, ts, "GET", "/api/projects/"+strconv.FormatInt(p.ID, 10), "", &detail)
	close(release)
	got := map[string][2]bool{}
	for _, tr := range detail.Transfers {
		got[tr.DestID] = [2]bool{tr.Running, tr.Busy}
	}
	if got[ids["B"]] != [2]bool{true, false} || got[ids["C"]] != [2]bool{false, true} {
		t.Fatalf("running/busy flags: %v", got)
	}
}
