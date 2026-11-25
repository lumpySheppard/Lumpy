package ui

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/lumpysheppard/lumpy/internal/catalog"
	"github.com/lumpysheppard/lumpy/internal/job"
	"github.com/lumpysheppard/lumpy/internal/volume"
)

func TestProxiesThroughAPI(t *testing.T) {
	s, ts, vols := setup(t)
	on := filepath.Join(vols, "RAID", "Job", "01_Original footage", "Online")
	off := filepath.Join(vols, "RAID", "Job", "01_Original footage", "Offline")
	for _, p := range []string{
		filepath.Join(on, "A001", "A001_C001_0101XY.RDC", "A001_C001_0101XY_001.R3D"),
		filepath.Join(on, "A001", "A001_C001_0101XY.RDC", "A001_C001_0101XY_002.R3D"),
		filepath.Join(on, "A001", "A001_C002_0101XY.RDC", "A001_C002_0101XY_001.R3D"),
		filepath.Join(on, "B001", "B001_C001.mov"),
		filepath.Join(off, "A001_C001_0101XY_proxy.mov"),
		filepath.Join(off, "B001", "B001_C001.mov"),
	} {
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("x "+p), 0o644)
	}
	job.AgentRunWith(context.Background(), s.db, job.NewReportTo(io.Discard), func(string, string) {}, nil)
	var st struct{ Drives []DriveView }
	call(t, s, ts, "GET", "/api/state", "", &st)
	if len(st.Drives) != 1 {
		t.Fatalf("drives: %+v", st.Drives)
	}
	id := st.Drives[0].ID

	var views []ProxyView
	call(t, s, ts, "GET", "/api/drives/"+id+"/proxies", "", &views)
	if len(views) != 1 {
		t.Fatalf("pairs: %+v", views)
	}
	v := views[0]
	if v.Online != "Job/01_Original footage/Online" || v.Offline != "Job/01_Original footage/Offline" || v.Total != 3 || v.Proxied != 2 || len(v.Missing) != 1 || v.Missing[0].Name != "A001_C002_0101XY" || v.Missing[0].Files != 1 {
		t.Errorf("drive view: %+v", v)
	}

	// A project of the job folder sees the same pair, once, even when two
	// of its folders contain it.
	var p catalog.Project
	call(t, s, ts, "POST", "/api/projects", `{"name":"Job","target":1,"sources":[{"driveId":"`+id+`","folder":"Job"},{"driveId":"`+id+`","folder":""}]}`, &p)
	views = nil
	call(t, s, ts, "GET", "/api/projects/"+strconv.FormatInt(p.ID, 10)+"/proxies", "", &views)
	if len(views) != 1 || views[0].Proxied != 2 {
		t.Errorf("project view: %+v", views)
	}
	// A folder outside the pair sees nothing.
	call(t, s, ts, "POST", "/api/projects", `{"name":"Other","target":1,"sources":[{"driveId":"`+id+`","folder":"Job/01_Original footage/Offline"}]}`, &p)
	views = nil
	call(t, s, ts, "GET", "/api/projects/"+strconv.FormatInt(p.ID, 10)+"/proxies", "", &views)
	if len(views) != 0 {
		t.Errorf("outside view: %+v", views)
	}
}

func TestEjectThroughAPI(t *testing.T) {
	s, ts, vols := setup(t)
	os.MkdirAll(filepath.Join(vols, "CARD", "CLIPS"), 0o755)
	os.WriteFile(filepath.Join(vols, "CARD", "CLIPS", "C001.mov"), []byte("footage"), 0o644)
	// A card is never logged by itself: the agent leaves it, and it is
	// logged by hand from Connected.
	job.AgentRunWith(context.Background(), s.db, job.NewReportTo(io.Discard), func(string, string) {}, nil)
	var st struct {
		Drives  []DriveView
		Volumes []VolumeView
	}
	call(t, s, ts, "GET", "/api/state", "", &st)
	if len(st.Drives) != 0 || len(st.Volumes) != 1 || !st.Volumes[0].Capture {
		t.Fatalf("card logged by itself: %d drives, %+v", len(st.Drives), st.Volumes)
	}
	call(t, s, ts, "POST", "/api/catalog", `{"id":"`+st.Volumes[0].ID+`"}`, nil)
	waitIdle(t, s)
	call(t, s, ts, "GET", "/api/state", "", &st)
	if len(st.Drives) != 1 {
		t.Fatalf("logged by hand: %d drives", len(st.Drives))
	}
	id := st.Drives[0].ID
	if !st.Drives[0].Connected {
		t.Fatal("not connected")
	}
	// Ejecting takes the mount away, like unplugging.
	old := volume.Eject
	defer func() { volume.Eject = old }()
	var ejected string
	volume.Eject = func(mount string) error { ejected = mount; return os.Rename(mount, mount+".gone") }
	if code := call(t, s, ts, "POST", "/api/drives/"+id+"/eject", "", nil); code != 200 {
		t.Fatalf("eject: %d", code)
	}
	// macOS hands back /private/var/… for a /var/… temp dir.
	want, _ := filepath.EvalSymlinks(filepath.Join(vols, "CARD.gone"))
	got, _ := filepath.EvalSymlinks(ejected + ".gone")
	if got != want || filepath.Base(ejected) != "CARD" {
		t.Errorf("ejected %q", ejected)
	}
	call(t, s, ts, "GET", "/api/state", "", &st)
	if st.Drives[0].Connected {
		t.Error("still connected after eject")
	}
	if code := call(t, s, ts, "POST", "/api/drives/"+id+"/eject", "", nil); code != 400 {
		t.Errorf("eject when unplugged: %d", code)
	}
	var detail struct {
		Events []struct{ Kind string }
	}
	call(t, s, ts, "GET", "/api/drives/"+id, "", &detail)
	if len(detail.Events) == 0 || detail.Events[0].Kind != "eject" {
		t.Errorf("events: %+v", detail.Events)
	}
}

// A card erases only with two verified copies on two other drives, and
// only when its name is typed; the day's report then lists it.
func TestEraseCardThroughAPI(t *testing.T) {
	s, ts, vols := setup(t)
	os.MkdirAll(filepath.Join(vols, "CARD", "CLIPS"), 0o755)
	os.WriteFile(filepath.Join(vols, "CARD", "CLIPS", "C001.mov"), []byte("footage"), 0o644)
	os.Mkdir(filepath.Join(vols, "RAID"), 0o755)
	os.Mkdir(filepath.Join(vols, "SHUTTLE"), 0o755)
	var st struct {
		Drives  []DriveView
		Volumes []VolumeView
	}
	call(t, s, ts, "GET", "/api/state", "", &st)
	var cardID string
	for _, v := range st.Volumes {
		if v.Name == "CARD" {
			cardID = v.ID
		}
	}
	call(t, s, ts, "POST", "/api/catalog", `{"id":"`+cardID+`"}`, nil)
	waitIdle(t, s)
	var cv CardView
	call(t, s, ts, "GET", "/api/drives/"+cardID+"/card", "", &cv)
	if cv.Verdict != "no" || cv.Files != 1 {
		t.Fatalf("before any copy: %+v", cv)
	}
	var started struct{ Job int }
	call(t, s, ts, "POST", "/api/offload", `{"source":"`+filepath.Join(vols, "CARD")+`","dests":["`+filepath.Join(vols, "RAID", "Day01")+`","`+filepath.Join(vols, "SHUTTLE", "Day01")+`"]}`, &started)
	waitJobs(t, s, ts)
	call(t, s, ts, "GET", "/api/drives/"+cardID+"/card", "", &cv)
	if cv.Verdict != "safe" || cv.MinVerified != 2 || len(cv.Holders) != 2 {
		t.Fatalf("after the offload: %+v", cv)
	}
	old := volume.Erase
	defer func() { volume.Erase = old }()
	var erased string
	volume.Erase = func(mount, name, fstype string) error {
		erased = name
		os.RemoveAll(filepath.Join(mount, "CLIPS"))
		return nil
	}
	if code := call(t, s, ts, "POST", "/api/drives/"+cardID+"/erase", `{"name":"WRONG"}`, nil); code != 400 {
		t.Fatalf("erase with the wrong name: %d", code)
	}
	if code := call(t, s, ts, "POST", "/api/drives/"+cardID+"/erase", `{"name":"CARD"}`, nil); code != 200 {
		t.Fatalf("erase: %d", code)
	}
	waitIdle(t, s)
	if erased != "CARD" {
		t.Fatalf("erased %q", erased)
	}
	req, _ := http.NewRequest("GET", ts.URL+"/api/report/day?t="+s.token, nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(string(page), "CARD") || !strings.Contains(string(page), "cards offloaded") || !strings.Contains(string(page), "RAID") {
		t.Fatalf("report:\n%s", page)
	}
}
