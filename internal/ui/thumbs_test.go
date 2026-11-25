package ui

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/lumpysheppard/lumpy/internal/job"
	"github.com/lumpysheppard/lumpy/internal/media"
)

func getRaw(t *testing.T, s *Server, url string) (int, []byte) {
	t.Helper()
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("X-Lumpy-Token", s.token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	return res.StatusCode, data
}

// A still on a plugged-in drive comes back as a 320-wide JPEG, kept in
// the thumbs folder; a video frame too, when ffmpeg is installed.
func TestThumbnails(t *testing.T) {
	s, ts, vols := setup(t)
	os.MkdirAll(filepath.Join(vols, "A", "STILLS"), 0o755)
	img := image.NewRGBA(image.Rect(0, 0, 640, 360))
	for y := 0; y < 360; y++ {
		for x := 0; x < 640; x++ {
			img.Set(x, y, color.RGBA{uint8(x / 3), uint8(y / 2), 90, 255})
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	os.WriteFile(filepath.Join(vols, "A", "STILLS", "frame.png"), buf.Bytes(), 0o644)
	job.AgentRunWith(context.Background(), s.db, job.NewReportTo(io.Discard), func(string, string) {}, nil)
	var st struct{ Drives []DriveView }
	call(t, s, ts, "GET", "/api/state", "", &st)
	if len(st.Drives) != 1 {
		t.Fatalf("drives: %+v", st.Drives)
	}
	id := st.Drives[0].ID
	base := ts.URL + "/api/drives/" + id + "/thumb?path="

	code, data := getRaw(t, s, base+"STILLS/frame.png")
	if code != 200 {
		t.Fatalf("still: %d %s", code, data)
	}
	got, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil || got.Bounds().Dx() != 320 || got.Bounds().Dy() != 180 {
		t.Fatalf("still thumb: %v %v", err, got.Bounds())
	}
	r, g, b, _ := got.At(319, 179).RGBA()
	if r>>8 < 190 || g>>8 < 160 || g>>8 > 195 || b>>8 < 70 || b>>8 > 110 {
		t.Fatalf("bottom-right pixel: %d %d %d", r>>8, g>>8, b>>8)
	}
	entries, _ := filepath.Glob(filepath.Join(s.thumbDir(), "*", "*.jpg"))
	if len(entries) != 1 {
		t.Fatalf("cache: %v", entries)
	}
	if code, _ := getRaw(t, s, base+"../etc/passwd"); code != 400 {
		t.Fatalf("bad path: %d", code)
	}
	if code, _ := getRaw(t, s, base+"STILLS/missing.png"); code != 404 {
		t.Fatalf("missing: %d", code)
	}
	os.WriteFile(filepath.Join(vols, "A", "STILLS", "clip.r3d"), []byte("raw"), 0o644)
	if code, data := getRaw(t, s, base+"STILLS/clip.r3d"); code != 404 || !bytes.Contains(data, []byte("no preview for r3d")) {
		t.Fatalf("r3d: %d %s", code, data)
	}

	ff := media.FFmpeg()
	if ff == "" {
		t.Log("ffmpeg not installed: video frames not tested")
		return
	}
	clip := filepath.Join(vols, "A", "STILLS", "clip.mp4")
	if out, err := exec.Command(ff, "-v", "error", "-f", "lavfi", "-i", "testsrc=duration=2:size=320x180:rate=25", "-pix_fmt", "yuv420p", clip).CombinedOutput(); err != nil {
		t.Skipf("ffmpeg can't make a test clip: %v %s", err, out)
	}
	for _, at := range []string{"1.0", "99"} { // past the end falls back to the first frame
		code, data := getRaw(t, s, base+"STILLS/clip.mp4&at="+at)
		if code != 200 {
			t.Fatalf("video at %s: %d %s", at, code, data)
		}
		if got, err := jpeg.Decode(bytes.NewReader(data)); err != nil || got.Bounds().Dx() != 320 {
			t.Fatalf("video thumb at %s: %v", at, err)
		}
	}
}

func TestTrimThumbs(t *testing.T) {
	dir := t.TempDir()
	// The trim runs only past the limits; well under them nothing goes.
	os.WriteFile(filepath.Join(dir, "a.jpg"), []byte("x"), 0o644)
	trimThumbs(dir)
	if _, err := os.Stat(filepath.Join(dir, "a.jpg")); err != nil {
		t.Fatal("trimmed a small cache")
	}
}
