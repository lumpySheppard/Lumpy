package ui

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

type spaceReply struct {
	Places []SpacePlace `json:"places"`
	Run    *struct {
		Root     string `json:"root"`
		Scanning bool   `json:"scanning"`
		Files    int64  `json:"files"`
		Error    string `json:"error"`
	} `json:"run"`
	Node *struct {
		Path string `json:"path"`
		Size int64  `json:"size"`
		Kids []struct {
			Name string `json:"name"`
			Path string `json:"path"`
			Size int64  `json:"size"`
		} `json:"kids"`
	} `json:"node"`
}

func waitScan(t *testing.T, get func() spaceReply) spaceReply {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		r := get()
		if r.Run != nil && !r.Run.Scanning {
			return r
		}
		if time.Now().After(deadline) {
			t.Fatal("scan did not finish")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestSpaceScanAndTrash(t *testing.T) {
	s, ts, _ := setup(t)
	t.Setenv("HOME", t.TempDir()) // the real home is protected; keep the test out of it
	root := t.TempDir()
	for name, n := range map[string]int{"renders/a.mov": 3 << 20, "renders/b.mov": 1 << 20, "notes.txt": 4 << 10} {
		p := filepath.Join(root, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, make([]byte, n), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	get := func(path string) func() spaceReply {
		return func() spaceReply {
			var r spaceReply
			if code := call(t, s, ts, "GET", "/api/space?path="+path, "", &r); code != 200 {
				t.Fatalf("GET /api/space: %d", code)
			}
			return r
		}
	}

	if code := call(t, s, ts, "POST", "/api/space/trash", `{"paths":["renders"]}`, nil); code != 400 {
		t.Fatalf("trash before any scan: %d", code)
	}
	if code := call(t, s, ts, "POST", "/api/space/scan", `{"root":"relative/path"}`, nil); code != 400 {
		t.Fatalf("relative root: %d", code)
	}
	if code := call(t, s, ts, "POST", "/api/space/scan", `{"root":"`+root+`"}`, nil); code != 200 {
		t.Fatalf("scan: %d", code)
	}
	r := waitScan(t, get(""))
	if r.Run.Error != "" || r.Run.Files != 3 || r.Node == nil || r.Node.Kids[0].Name != "renders" {
		t.Fatalf("after scan: %+v %+v", r.Run, r.Node)
	}
	if r := get("renders")(); r.Node.Path != "renders" || len(r.Node.Kids) != 2 {
		t.Fatalf("renders view: %+v", r.Node)
	}

	var res struct {
		Moved  []string `json:"moved"`
		Bytes  int64    `json:"bytes"`
		Failed []struct {
			Path string `json:"path"`
		} `json:"failed"`
	}
	body := `{"paths":["renders/a.mov","../etc","", "nope"]}`
	if code := call(t, s, ts, "POST", "/api/space/trash", body, &res); code != 200 {
		t.Fatalf("trash: %d", code)
	}
	if len(res.Moved) != 1 || res.Moved[0] != "renders/a.mov" || res.Bytes < 3<<20 || len(res.Failed) != 3 {
		t.Fatalf("trash result: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(root, "renders", "a.mov")); !os.IsNotExist(err) {
		t.Fatal("the file is still there")
	}
	after := get("")()
	if after.Node.Size >= r.Node.Size-(3<<20)+1 {
		t.Fatalf("size not reduced: %d → %d", r.Node.Size, after.Node.Size)
	}
}

func TestSpaceTrashRefusesProtected(t *testing.T) {
	s, ts, _ := setup(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, "Movies"), 0o755)
	os.WriteFile(filepath.Join(home, "Movies", "x.mov"), make([]byte, 1000), 0o644)
	parent := filepath.Dir(home)
	if code := call(t, s, ts, "POST", "/api/space/scan", `{"root":"`+parent+`"}`, nil); code != 200 {
		t.Fatalf("scan: %d", code)
	}
	waitScan(t, func() spaceReply {
		var r spaceReply
		call(t, s, ts, "GET", "/api/space", "", &r)
		return r
	})
	var res struct {
		Moved  []string `json:"moved"`
		Failed []struct {
			Error string `json:"error"`
		} `json:"failed"`
	}
	call(t, s, ts, "POST", "/api/space/trash", `{"paths":["`+filepath.Base(home)+`"]}`, &res)
	if len(res.Moved) != 0 || len(res.Failed) != 1 {
		t.Fatalf("the home folder was not protected: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(home, "Movies", "x.mov")); err != nil {
		t.Fatal(err)
	}
}

func TestStartupDiskAllowed(t *testing.T) {
	for p, want := range map[string]bool{
		"/Users/will/Movies/old":                    true,
		"/System/Volumes/Data/Users/will/Downloads": true,
		"/Applications/Big.app":                     true,
		"/Volumes/RAID/renders":                     true,
		"/System/Volumes/Data/private":              false,
		"/System/Volumes/Data/Library/Caches":       false,
		"/Library/Application Support":              false,
		"/Users":                                    false,
		"/Applications":                             false,
		"/System/Volumes/Data/Applications":         false,
	} {
		if got := startupDiskAllowed(p); got != want {
			t.Errorf("%s: %v, want %v", p, got, want)
		}
	}
}
