package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/lumpysheppard/lumpy/internal/catalog"
	"github.com/lumpysheppard/lumpy/internal/job"
)

// A frame grabbed from a clip in a project lands in the project's
// OUTPUT/Stills folder, named after the clip and the timecode; it is
// listed for the clip, served back, and deleted with its file.
func TestStillsThroughAPI(t *testing.T) {
	s, ts, vols := setup(t)
	t.Setenv("HOME", t.TempDir())
	root := filepath.Join(vols, "RAID", "Film")
	os.MkdirAll(filepath.Join(root, "01_Original footage", "Online"), 0o755)
	os.MkdirAll(filepath.Join(root, "09_OUTPUT", "Stills"), 0o755)
	os.WriteFile(filepath.Join(root, "01_Original footage", "Online", "A001_C002.mov"), []byte("mov"), 0o644)
	job.AgentRunWith(context.Background(), s.db, job.NewReportTo(io.Discard), func(string, string) {}, nil)
	var st struct{ Drives []DriveView }
	stateSettled(t, s, ts, &st)
	var p catalog.Project
	call(t, s, ts, "POST", "/api/projects", `{"name":"Film","target":2,"sources":[{"driveId":"`+st.Drives[0].ID+`","folder":"Film"}]}`, &p)

	img := image.NewRGBA(image.Rect(0, 0, 64, 36))
	for y := 0; y < 36; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 4), uint8(y * 7), 90, 255})
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	post := func(tc string) (*catalog.Still, int) {
		req, _ := http.NewRequest("POST", ts.URL+"/api/drives/"+st.Drives[0].ID+"/still?path=Film/01_Original%20footage/Online/A001_C002.mov&tc="+tc+"&frame=48", bytes.NewReader(buf.Bytes()))
		req.Header.Set("X-Lumpy-Token", s.token)
		req.Header.Set("Content-Type", "image/png")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out catalog.Still
		data, _ := io.ReadAll(res.Body)
		if res.StatusCode == 200 {
			if err := decodeJSON(data, &out); err != nil {
				t.Fatal(err)
			}
		}
		return &out, res.StatusCode
	}
	st1, code := post("01:00:01:23")
	if code != 200 {
		t.Fatalf("grab: %d", code)
	}
	want := filepath.Join(root, "09_OUTPUT", "Stills", "A001_C002_01.00.01.23.png")
	if st1.File != want || st1.Width != 64 || st1.Height != 36 {
		t.Fatalf("still: %+v", st1)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatal(err)
	}
	// The same frame again does not overwrite the first grab.
	st2, _ := post("01:00:01:23")
	if st2.File == st1.File {
		t.Fatalf("second grab overwrote the first: %s", st2.File)
	}
	var list []catalog.Still
	call(t, s, ts, "GET", "/api/drives/"+st.Drives[0].ID+"/stills?path=Film/01_Original%20footage/Online/A001_C002.mov", "", &list)
	if len(list) != 2 || list[0].ID != st2.ID {
		t.Fatalf("list: %+v", list)
	}
	req, _ := http.NewRequest("GET", ts.URL+"/api/stills/"+strconv.FormatInt(st1.ID, 10)+"/file", nil)
	req.Header.Set("X-Lumpy-Token", s.token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "image/png" || !bytes.Equal(body, buf.Bytes()) {
		t.Fatalf("file: %d %s %d bytes", res.StatusCode, res.Header.Get("Content-Type"), len(body))
	}
	if code := call(t, s, ts, "DELETE", "/api/stills/"+strconv.FormatInt(st1.ID, 10), "", nil); code != 200 {
		t.Fatalf("delete: %d", code)
	}
	if _, err := os.Stat(want); err == nil {
		t.Fatal("still file not removed")
	}
	call(t, s, ts, "GET", "/api/drives/"+st.Drives[0].ID+"/stills?path=Film/01_Original%20footage/Online/A001_C002.mov", "", &list)
	if len(list) != 1 {
		t.Fatalf("after delete: %+v", list)
	}
	// Not a PNG: refused.
	req, _ = http.NewRequest("POST", ts.URL+"/api/drives/"+st.Drives[0].ID+"/still?path=Film/01_Original%20footage/Online/A001_C002.mov", bytes.NewReader([]byte("nope")))
	req.Header.Set("X-Lumpy-Token", s.token)
	res, _ = http.DefaultClient.Do(req)
	res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatalf("junk: %d", res.StatusCode)
	}
}

// A clip in no project goes to ~/Pictures/Lumpy Stills/<drive>.
func TestStillsOutsideProject(t *testing.T) {
	s, ts, vols := setup(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(vols, "LOOSE"), 0o755)
	os.WriteFile(filepath.Join(vols, "LOOSE", "clip.mov"), []byte("mov"), 0o644)
	job.AgentRunWith(context.Background(), s.db, job.NewReportTo(io.Discard), func(string, string) {}, nil)
	var st struct{ Drives []DriveView }
	stateSettled(t, s, ts, &st)
	var d *DriveView
	for i := range st.Drives {
		if st.Drives[i].Name == "LOOSE" {
			d = &st.Drives[i]
		}
	}
	if d == nil {
		t.Fatalf("drives: %+v", st.Drives)
	}
	var buf bytes.Buffer
	png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 8, 8)))
	req, _ := http.NewRequest("POST", ts.URL+"/api/drives/"+d.ID+"/still?path=clip.mov&frame=12", bytes.NewReader(buf.Bytes()))
	req.Header.Set("X-Lumpy-Token", s.token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(res.Body)
	res.Body.Close()
	var out catalog.Still
	if res.StatusCode != 200 || decodeJSON(data, &out) != nil {
		t.Fatalf("grab: %d %s", res.StatusCode, data)
	}
	if want := filepath.Join(home, "Pictures", "Lumpy Stills", "LOOSE", "clip_f000012.png"); out.File != want {
		t.Fatalf("file: %s, want %s", out.File, want)
	}
}

func decodeJSON(data []byte, out any) error { return json.Unmarshal(data, out) }
