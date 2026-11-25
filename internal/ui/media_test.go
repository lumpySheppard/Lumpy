package ui

import (
	"bytes"
	"encoding/binary"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lumpysheppard/lumpy/internal/catalog"
)

// wav builds a 2-channel 48 kHz 24-bit file half a second long.
func wav() []byte {
	le32 := func(v uint32) []byte { x := make([]byte, 4); binary.LittleEndian.PutUint32(x, v); return x }
	le16 := func(v uint16) []byte { x := make([]byte, 2); binary.LittleEndian.PutUint16(x, v); return x }
	chunk := func(id string, body []byte) []byte {
		return append(append([]byte(id), le32(uint32(len(body)))...), body...)
	}
	fmtc := chunk("fmt ", bytes.Join([][]byte{le16(1), le16(2), le32(48000), le32(48000 * 6), le16(6), le16(24)}, nil))
	data := chunk("data", make([]byte, 48000*6/2))
	body := bytes.Join([][]byte{[]byte("WAVE"), fmtc, data}, nil)
	return append(append([]byte("RIFF"), le32(uint32(len(body)))...), body...)
}

func TestMediaThroughAPI(t *testing.T) {
	s, ts, vols := setup(t)
	clip := filepath.Join(vols, "S001", "AUDIO", "S001T003.wav")
	os.MkdirAll(filepath.Dir(clip), 0o755)
	os.WriteFile(clip, wav(), 0o644)
	os.Mkdir(filepath.Join(vols, "SHUTTLE"), 0o755)

	var started struct{ Job int }
	body := `{"source":"` + filepath.Join(vols, "S001") + `","dests":["` + filepath.Join(vols, "SHUTTLE", "Day01") + `"]}`
	if code := call(t, s, ts, "POST", "/api/offload", body, &started); code != 200 {
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

	// The offload read the clip once and recorded it for both drives.
	var list struct {
		Files  []catalog.MediaRow
		Codecs []string
	}
	call(t, s, ts, "GET", "/api/media", "", &list)
	if len(list.Files) != 1 || list.Files[0].AudioCodec != "PCM" || list.Files[0].Channels != 2 || list.Files[0].SampleRate != 48000 ||
		list.Files[0].BitDepth != 24 || list.Files[0].DurationMS != 500 || list.Files[0].Copies != 1 {
		t.Fatalf("media list: %+v", list)
	}
	var st struct{ Drives []DriveView }
	call(t, s, ts, "GET", "/api/state", "", &st)
	var card, shuttle DriveView
	for _, d := range st.Drives {
		switch d.Name {
		case "S001":
			card = d
		case "SHUTTLE":
			shuttle = d
		}
	}
	var detail struct {
		Media    *catalog.Media
		Copies   []map[string]any
		Playable bool
	}
	call(t, s, ts, "GET", "/api/drives/"+card.ID+"/file?path=AUDIO/S001T003.wav", "", &detail)
	if detail.Media == nil || detail.Media.Container != "WAV" || len(detail.Copies) != 1 || detail.Copies[0]["path"] != "Day01/S001/AUDIO/S001T003.wav" || !detail.Playable {
		t.Fatalf("file detail: %+v", detail)
	}

	// Edits follow the content onto the other copy, and filter the list.
	if code := call(t, s, ts, "PATCH", "/api/drives/"+card.ID+"/file?path=AUDIO/S001T003.wav", `{"scene":"12","take":"3","qc":"flag","notes":"plane overhead"}`, nil); code != 200 {
		t.Fatalf("edit: %d", code)
	}
	m, err := s.db.MediaFor(shuttle.ID, "Day01/S001/AUDIO/S001T003.wav")
	if err != nil || m == nil || m.Edits.Scene != "12" || m.Edits.QC != "flag" {
		t.Fatalf("edit did not reach the copy: %+v %v", m, err)
	}
	call(t, s, ts, "GET", "/api/media?qc=ok", "", &list)
	if len(list.Files) != 0 {
		t.Fatalf("qc=ok should match nothing: %+v", list.Files)
	}
	call(t, s, ts, "GET", "/api/media?qc=flag&q=plane", "", &list)
	if len(list.Files) != 1 {
		t.Fatalf("qc=flag&q=plane: %+v", list.Files)
	}
	if code := call(t, s, ts, "PATCH", "/api/drives/"+card.ID+"/file?path=AUDIO/S001T003.wav", `{"qc":"maybe"}`, nil); code != 400 {
		t.Fatalf("bad QC accepted: %d", code)
	}

	get := func(path string) (*http.Response, []byte) {
		req, _ := http.NewRequest("GET", ts.URL+path+"&t="+s.token, nil)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		return res, b
	}
	_, csv := get("/api/media.csv?qc=flag")
	if !strings.Contains(string(csv), "S001T003.wav,S001,AUDIO/S001T003.wav,") || !strings.Contains(string(csv), ",12,,3,flag,,plane overhead,") {
		t.Fatalf("csv:\n%s", csv)
	}

	// Preview streams the file itself, with seeking, and only from inside
	// the drive.
	res, b := get("/api/drives/" + card.ID + "/stream?path=AUDIO/S001T003.wav")
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "audio/wav" || res.Header.Get("Accept-Ranges") != "bytes" || !bytes.Equal(b, wav()) {
		t.Fatalf("stream: %d %s %d bytes", res.StatusCode, res.Header.Get("Content-Type"), len(b))
	}
	if res, _ := get("/api/drives/" + card.ID + "/stream?path=../SHUTTLE/Day01/S001/AUDIO/S001T003.wav"); res.StatusCode != 400 {
		t.Fatalf("path outside the drive: %d", res.StatusCode)
	}
}
