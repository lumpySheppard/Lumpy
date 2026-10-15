package ui

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
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
	job.AgentRunWith(context.Background(), s.db, job.NewReportTo(io.Discard), func(string, string) {}, nil)
	var st struct{ Drives []DriveView }
	call(t, s, ts, "GET", "/api/state", "", &st)
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
