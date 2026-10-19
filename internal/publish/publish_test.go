package publish

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/lumpysheppard/lumpy/internal/catalog"
	"github.com/lumpysheppard/lumpy/internal/job"
	"github.com/lumpysheppard/lumpy/internal/volume"
)

func TestPublishWritesPageAndUploads(t *testing.T) {
	vols := t.TempDir()
	t.Setenv("LUMPY_VOLUMES", vols)
	home := t.TempDir()
	t.Setenv("LUMPY_HOME", home)
	db, err := catalog.Open(filepath.Join(home, "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// A card offloaded to one drive, in a project wanting two copies.
	clip := filepath.Join(vols, "A001", "CLIPS", "C001.mov")
	os.MkdirAll(filepath.Dir(clip), 0o755)
	os.WriteFile(clip, []byte("footage"), 0o644)
	os.Mkdir(filepath.Join(vols, "SHUTTLE"), 0o755)
	r := job.NewReportTo(io.Discard)
	if err := job.Offload(context.Background(), job.OffloadOptions{Source: filepath.Join(vols, "A001"), Dests: []string{filepath.Join(vols, "SHUTTLE", "Day01")}, Library: db, Meta: job.Meta{Version: "t"}}, r); err != nil {
		t.Fatal(err)
	}
	var shuttle *volume.Volume
	mounted, _ := volume.Mounted()
	for _, v := range mounted {
		if v.Name == "SHUTTLE" {
			shuttle = v
		}
	}
	p, _ := db.CreateProject("Pilot", 2)
	db.AddSource(p.ID, shuttle.ID, "Day01")

	// The upload target: a server that accepts PUTs.
	var mu sync.Mutex
	got := map[string]string{}
	var auth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPut {
			w.WriteHeader(405)
			return
		}
		b, _ := io.ReadAll(req.Body)
		mu.Lock()
		got[req.URL.Path] = string(b)
		auth = req.Header.Get("Authorization")
		mu.Unlock()
		w.WriteHeader(201)
	}))
	defer ts.Close()

	marker := filepath.Join(home, "ran")
	s := Settings{Enabled: true, Title: "Blue Sheep status", Folder: filepath.Join(home, "out"), URL: ts.URL + "/lumpy/", Token: "secret",
		Command: "cp \"$LUMPY_STATUS_DIR/status.json\" " + marker, Files: true}
	if err := Save(db, s); err != nil {
		t.Fatal(err)
	}
	if l := Load(db); l != s {
		t.Fatalf("settings round trip: %+v", l)
	}
	res, err := Run(context.Background(), db, "mac", "1.0", s)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK || !strings.HasPrefix(res.Summary, "2 drives, 1 projects, 2 files") {
		t.Fatalf("result: %+v", res)
	}
	if Last(db) == nil || !Last(db).OK {
		t.Fatal("last result not recorded")
	}

	page, err := os.ReadFile(filepath.Join(home, "out", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), "<title>Blue Sheep status</title>") || !strings.Contains(string(page), `"CLIPS/C001.mov"`) {
		t.Fatalf("page:\n%s", page)
	}
	var st Status
	if err := json.Unmarshal([]byte(got["/lumpy/status.json"]), &st); err != nil {
		t.Fatalf("uploaded json: %v", err)
	}
	if auth != "Bearer secret" || len(st.Drives) != 2 || len(st.Projects) != 1 || st.Projects[0].Name != "Pilot" || len(st.Files) != 2 {
		t.Fatalf("uploaded: auth=%q %+v", auth, st)
	}
	// The project wants 2 copies and has them (card + shuttle): no steps.
	if st.Projects[0].Health.Backed != 1 || len(st.Projects[0].Steps) != 0 {
		t.Fatalf("project: %+v", st.Projects[0])
	}
	// The card is backed up on the shuttle, so its state is full; the
	// uploaded page holds the same data.
	for _, d := range st.Drives {
		if d.Backup != "full" {
			t.Fatalf("%s backup %q", d.Name, d.Backup)
		}
	}
	if !strings.Contains(got["/lumpy/index.html"], "Blue Sheep status") {
		t.Fatal("index.html not uploaded")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("command did not run")
	}

	// A failed upload is recorded, and the local page is still written.
	ts.Close()
	if _, err := Run(context.Background(), db, "mac", "1.0", s); err == nil {
		t.Fatal("expected an upload error")
	}
	if l := Last(db); l == nil || l.OK || !strings.Contains(l.Summary, "upload") {
		t.Fatalf("failure not recorded: %+v", l)
	}
}
