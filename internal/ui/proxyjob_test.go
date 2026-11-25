package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Proxies go to the Offline folder beside the Online folder the copy
// sits under, mirroring the path beneath Online; with no Online folder
// above, to a Proxies folder beside the copy.
func TestProxyDirFor(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "Film", "01_Original footage", "Online", "2026-11-25", "A001"), 0o755)
	os.MkdirAll(filepath.Join(root, "Film", "01_Original footage", "OFFLINE"), 0o755)
	got := proxyDirFor(filepath.Join(root, "Film", "01_Original footage", "Online", "2026-11-25", "A001"))
	if want := filepath.Join(root, "Film", "01_Original footage", "OFFLINE", "2026-11-25", "A001"); got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	os.MkdirAll(filepath.Join(root, "Other", "Online", "B002"), 0o755)
	got = proxyDirFor(filepath.Join(root, "Other", "Online", "B002"))
	if want := filepath.Join(root, "Other", "Offline", "B002"); got != want {
		t.Errorf("no offline yet: got %s, want %s", got, want)
	}
	got = proxyDirFor(filepath.Join(root, "Loose", "01_RAW", "2026-11-25", "C003"))
	if want := filepath.Join(root, "Loose", "01_RAW", "2026-11-25", "Proxies", "C003"); got != want {
		t.Errorf("no online: got %s, want %s", got, want)
	}
}

// Originals under a folder are listed once each, RED clips as their
// .RDC folder, and the ones with a proxy already are dropped.
func TestClipsUnder(t *testing.T) {
	root := t.TempDir()
	w := func(rel string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("x"), 0o644)
	}
	w("CLIPS/A001_C001.braw")
	w("CLIPS/A001_C002.braw")
	w("CLIPS/A001_C002.braw.sidecar")
	w("CONTENTS/Z002C056_26091608_CANON.CRM")
	w("R001.RDC/R001_C001_0101_001.R3D")
	w("R001.RDC/R001_C001_0101_002.R3D")
	w("notes.txt")
	w(".hidden.mov")
	clips := clipsUnder(root)
	if len(clips) != 4 {
		t.Fatalf("clips: %+v", clips)
	}
	stems := map[string]bool{}
	for _, c := range clips {
		stems[c.Stem] = true
	}
	for _, want := range []string{"A001_C001", "A001_C002", "Z002C056_26091608_CANON", "R001_C001_0101"} {
		if !stems[want] {
			t.Errorf("missing %s in %+v", want, clips)
		}
	}
	dest := t.TempDir()
	os.WriteFile(filepath.Join(dest, "A001_C001.mov"), []byte("proxy"), 0o644)
	if left := withoutProxies(clips, dest); len(left) != 3 {
		t.Errorf("after one proxy: %+v", left)
	}
	if proxyScale("QUARTER") != "quarter" || proxyScale("") != "half" || proxyScale("odd") != "half" {
		t.Error("scale")
	}
}

// The proxy job queued behind an offload waits for it, makes the
// Offline folder beside the destination's Online folder, and then
// (with no Resolve here) ends failed, saying so.
func TestProxiesAfterOffload(t *testing.T) {
	s, ts, vols := setup(t)
	card := filepath.Join(vols, "A001")
	os.MkdirAll(filepath.Join(card, "CLIPS"), 0o755)
	os.WriteFile(filepath.Join(card, "CLIPS", "A001_C001.braw"), []byte("raw"), 0o644)
	dest := filepath.Join(vols, "RAID", "Film", "01_Original footage", "Online", "2026-11-25")
	os.MkdirAll(dest, 0o755)
	var started struct{ Job int }
	body := `{"source":"` + card + `","dests":["` + dest + `"],"sourceCheck":false,"proxies":{"on":true,"scale":"half"}}`
	if code := call(t, s, ts, "POST", "/api/offload", body, &started); code != 200 {
		t.Fatalf("offload: %d", code)
	}
	jobs := waitJobs(t, s, ts)
	var off, px *JobView
	for i := range jobs {
		switch jobs[i].Kind {
		case "offload":
			off = &jobs[i]
		case "proxies":
			px = &jobs[i]
		}
	}
	if off == nil || off.State != "ok" {
		t.Fatalf("offload: %+v", off)
	}
	if px == nil {
		t.Fatalf("no proxy job queued: %+v", jobs)
	}
	if px.State != "failed" && px.State != "error" {
		t.Fatalf("proxy job without Resolve: %s %q", px.State, px.Error)
	}
	if !strings.Contains(strings.ToLower(px.Error), "resolve") && !strings.Contains(strings.ToLower(px.Error), "python") {
		t.Fatalf("the error should say what is missing: %q", px.Error)
	}
	want := filepath.Join(vols, "RAID", "Film", "01_Original footage", "Offline", "2026-11-25", "A001")
	if st, err := os.Stat(want); err != nil || !st.IsDir() {
		t.Fatalf("Offline folder not made at %s: %v", want, err)
	}
}
