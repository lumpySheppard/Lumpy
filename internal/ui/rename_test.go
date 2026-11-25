package ui

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/lumpysheppard/lumpy/internal/catalog"
	"github.com/lumpysheppard/lumpy/internal/job"
	"github.com/lumpysheppard/lumpy/internal/volume"
)

// A plugged-in drive with no checksums yet is offered the logged,
// unplugged drive of its size as a possible earlier name; adopting it
// carries the files, checksums and history over and drops the old entry.
func TestAdoptRenamedDrive(t *testing.T) {
	s, ts, vols := setup(t)
	os.MkdirAll(filepath.Join(vols, "NEWNAME"), 0o755)
	os.WriteFile(filepath.Join(vols, "NEWNAME", "a.mov"), []byte("footage"), 0o644)
	job.AgentRunWith(context.Background(), s.db, job.NewReportTo(io.Discard), func(string, string) {}, nil)
	var st struct{ Drives []DriveView }
	stateSettled(t, s, ts, &st)
	var nw *DriveView
	for i := range st.Drives {
		if st.Drives[i].Name == "NEWNAME" {
			nw = &st.Drives[i]
		}
	}
	if nw == nil {
		t.Fatalf("drives: %+v", st.Drives)
	}
	// The old entry: same size, not plugged in, checksummed.
	old := &volume.Volume{ID: "name:OLDNAME:" + itoa64(nw.Size), Name: "OLDNAME", Mount: "/Volumes/OLDNAME", Size: nw.Size, Free: nw.Free, FSType: nw.FSType}
	if err := s.db.SaveDrive(old); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.SyncFiles(old.ID, []catalog.File{{Path: "a.mov", Size: 7}, {Path: "b.mov", Size: 9}}, map[string]string{"a.mov": "aaaa", "b.mov": "bbbb"}); err != nil {
		t.Fatal(err)
	}
	s.db.Log(old.ID, "verify", "all good", 0)
	s.invalidate()
	var detail struct {
		Lookalikes []lookalike `json:"lookalikes"`
	}
	call(t, s, ts, "GET", "/api/drives/"+nw.ID, "", &detail)
	if len(detail.Lookalikes) != 1 || detail.Lookalikes[0].ID != old.ID || detail.Lookalikes[0].Hashed != 2 {
		t.Fatalf("lookalikes: %+v", detail.Lookalikes)
	}
	if code := call(t, s, ts, "POST", "/api/drives/"+nw.ID+"/adopt", `{"from":"`+old.ID+`"}`, nil); code != 200 {
		t.Fatalf("adopt: %d", code)
	}
	d, err := s.db.Drive(nw.ID)
	if err != nil || d.Files != 2 || d.Hashed != 2 {
		t.Fatalf("after adopt: %+v %v", d, err)
	}
	if s.db.HasDrive(old.ID) {
		t.Fatal("old entry still there")
	}
	evs, _ := s.db.Events(nw.ID, 10)
	if len(evs) < 2 {
		t.Fatalf("history not carried: %+v", evs)
	}
	// Adopting a plugged-in drive is refused.
	if code := call(t, s, ts, "POST", "/api/drives/"+nw.ID+"/adopt", `{"from":"`+st.Drives[0].ID+`"}`, nil); code == 200 && st.Drives[0].ID != nw.ID {
		t.Fatal("adopting a plugged-in drive should be refused")
	}
}

func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
