package ui

import (
	"archive/zip"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestPhotoStem(t *testing.T) {
	for in, want := range map[string]string{
		"scan012-Edit.jpg": "scan012", "IMG_0013 copy.jpg": "img_0013", "roll3 (1).tif": "roll3",
		"image-13.jpg": "image-13", "Rome 004_web.jpg": "rome 004", "x.cr2": "x", "DSC_1000-Edit-2.jpg": "dsc_1000-edit-2",
	} {
		if got := photoStem(in); got != want {
			t.Errorf("photoStem(%q) = %q, want %q", in, got, want)
		}
	}
	if got := stemTail("scan012-Edit.jpg"); got != "-Edit" {
		t.Fatalf("tail %q", got)
	}
}

func writePNG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	rng := rand.New(rand.NewSource(1))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// Noise, like grain: a PNG of it is big, a JPEG small.
			img.Set(x, y, color.RGBA{uint8(rng.Intn(256)), uint8(rng.Intn(256)), uint8(rng.Intn(256)), 255})
		}
	}
	os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	png.Encode(f, img)
	f.Close()
}

// A folder renamed by number and by date; the raw beside each JPEG
// and the edit in another folder take the same name.
func TestPhotoRename(t *testing.T) {
	s, ts, _ := setup(t)
	root := t.TempDir()
	q := func(s string) string { return string(mustJSON(s)) }
	dir := filepath.Join(root, "Roll 3")
	os.MkdirAll(filepath.Join(root, "Exports"), 0o755)
	os.MkdirAll(dir, 0o755)
	// image-9, image-10, image-11: by name they sort 10, 11, 9; by number 9, 10, 11.
	base := time.Date(2026, 3, 14, 10, 0, 0, 0, time.Local)
	for i, n := range []int{10, 11, 9} {
		for _, ext := range []string{".jpg", ".tif"} {
			p := filepath.Join(dir, "image-"+strconv.Itoa(n)+ext)
			os.WriteFile(p, []byte("pic"), 0o644)
			os.Chtimes(p, base.Add(time.Duration(2-i)*time.Hour), base.Add(time.Duration(2-i)*time.Hour)) // newest first by name
		}
	}
	os.WriteFile(filepath.Join(root, "Exports", "image-10-Edit.jpg"), []byte("edit"), 0o644)
	call(t, s, ts, "POST", "/api/space/scan", `{"path":`+q(root)+`}`, nil)
	waitJobs(t, s, ts)
	var out struct {
		Plan    []renameStep `json:"plan"`
		Groups  int          `json:"groups"`
		Applied bool         `json:"applied"`
	}
	call(t, s, ts, "GET", "/api/space/photos?path="+urlQuery(root), "", nil)
	code := call(t, s, ts, "POST", "/api/space/photos/rename", `{"path":`+q(root)+`,"rel":"Roll 3","pattern":"Rome {nn}","by":"number","start":1,"partners":true,"dry":true}`, &out)
	if code != 200 || out.Groups != 3 || out.Applied {
		t.Fatalf("dry: %d %+v", code, out)
	}
	want := map[string]string{"Roll 3/image-9.jpg": "Roll 3/Rome 01.jpg", "Roll 3/image-9.tif": "Roll 3/Rome 01.tif", "Roll 3/image-10.jpg": "Roll 3/Rome 02.jpg", "Roll 3/image-11.tif": "Roll 3/Rome 03.tif", "Exports/image-10-Edit.jpg": "Exports/Rome 02-Edit.jpg"}
	got := map[string]string{}
	for _, st := range out.Plan {
		got[st.From] = st.To
	}
	for from, to := range want {
		if got[from] != to {
			t.Errorf("%s → %q, want %q", from, got[from], to)
		}
	}
	// By date: image-9 is the oldest file.
	call(t, s, ts, "POST", "/api/space/photos/rename", `{"path":`+q(root)+`,"rel":"Roll 3","pattern":"{date} {n}","by":"date","dry":true}`, &out)
	for _, st := range out.Plan {
		if st.From == "Roll 3/image-9.jpg" && st.To != "Roll 3/2026-03-14 1.jpg" {
			t.Fatalf("by date: %+v", st)
		}
	}
	// A pattern with no number: refused.
	if code := call(t, s, ts, "POST", "/api/space/photos/rename", `{"path":`+q(root)+`,"rel":"Roll 3","pattern":"Rome","dry":true}`, nil); code == 200 {
		t.Fatal("a pattern without {n} was accepted")
	}
	// Apply: numbers shift over each other without loss.
	call(t, s, ts, "POST", "/api/space/photos/rename", `{"path":`+q(root)+`,"rel":"Roll 3","pattern":"image-{n}","by":"number","start":10,"partners":true}`, &out)
	if !out.Applied {
		t.Fatalf("not applied: %+v", out)
	}
	for _, n := range []string{"image-10.jpg", "image-11.jpg", "image-12.jpg", "image-12.tif"} {
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			t.Error(err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "Exports", "image-11-Edit.jpg")); err != nil {
		t.Error(err)
	}
	waitJobs(t, s, ts)
}

// Negatives matched to positives across folders; an unpacked zip found;
// a negative recompressed to a smaller JPEG with its name and time.
func TestPhotoNegatives(t *testing.T) {
	s, ts, _ := setup(t)
	root := t.TempDir()
	q := func(s string) string { return string(mustJSON(s)) }
	os.MkdirAll(filepath.Join(root, "Edited"), 0o755)
	writePNG(t, filepath.Join(root, "Scans", "roll1_001.png"), 1200, 800)
	os.WriteFile(filepath.Join(root, "Scans", "roll1_002.tif"), make([]byte, 50000), 0o644)
	os.WriteFile(filepath.Join(root, "Edited", "roll1_001-Edit.jpg"), make([]byte, 3000), 0o644)
	os.WriteFile(filepath.Join(root, "Edited", "roll1_002.jpg"), make([]byte, 2000), 0o644)
	os.WriteFile(filepath.Join(root, "Edited", "alone.jpg"), make([]byte, 2000), 0o644)
	// The lab's zip, already unpacked.
	zf, _ := os.Create(filepath.Join(root, "lab-roll1.zip"))
	zw := zip.NewWriter(zf)
	for _, n := range []string{"roll1_001.png", "roll1_002.tif", "__MACOSX/._x"} {
		w, _ := zw.Create(n)
		w.Write([]byte("z"))
	}
	zw.Close()
	zf.Close()
	call(t, s, ts, "POST", "/api/space/scan", `{"path":`+q(root)+`}`, nil)
	waitJobs(t, s, ts)
	var got struct {
		Pairs []photoPair   `json:"pairs"`
		Zips  []unpackedZip `json:"zips"`
	}
	call(t, s, ts, "GET", "/api/space/photos?path="+urlQuery(root), "", &got)
	if len(got.Pairs) != 2 || got.Pairs[0].Negative.Rel != "Scans/roll1_001.png" || got.Pairs[0].Positives[0].Rel != "Edited/roll1_001-Edit.jpg" || got.Pairs[1].Negative.Rel != "Scans/roll1_002.tif" {
		t.Fatalf("pairs: %+v", got.Pairs)
	}
	if len(got.Zips) != 1 || got.Zips[0].Entries != 2 || got.Zips[0].Found != 2 {
		t.Fatalf("zips: %+v", got.Zips)
	}
	before, _ := os.Stat(filepath.Join(root, "Scans", "roll1_001.png"))
	old := time.Date(2020, 5, 6, 7, 8, 9, 0, time.Local)
	os.Chtimes(filepath.Join(root, "Scans", "roll1_001.png"), old, old)
	var res struct{ Job int }
	call(t, s, ts, "POST", "/api/space/photos/recompress", `{"path":`+q(root)+`,"rels":["Scans/roll1_001.png"],"quality":80,"maxEdge":600}`, &res)
	waitJobs(t, s, ts)
	after, err := os.Stat(filepath.Join(root, "Scans", "roll1_001.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if after.Size() >= before.Size() || !after.ModTime().Equal(old) {
		t.Fatalf("recompressed: %d → %d, time %v", before.Size(), after.Size(), after.ModTime())
	}
	if _, err := os.Stat(filepath.Join(root, "Scans", "roll1_001.png")); err == nil {
		t.Fatal("the original is still there")
	}
	f, _ := os.Open(filepath.Join(root, "Scans", "roll1_001.jpg"))
	cfg, _, err := image.DecodeConfig(f)
	f.Close()
	if err != nil || cfg.Width != 600 || cfg.Height != 400 {
		t.Fatalf("size: %+v %v", cfg, err)
	}
}
