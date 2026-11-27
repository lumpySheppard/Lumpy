package ui

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

// Tags round-trip through the attribute the Finder uses, with the
// Finder's colour numbers, and a Finder-written binary plist reads.
func TestTagsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "clip.mov")
	os.WriteFile(f, []byte("x"), 0o644)
	if err := writeTags(f, []string{"Red", "Selects"}); err != nil {
		t.Skipf("no extended attributes here: %v", err)
	}
	if got := readTags(f); len(got) != 2 || got[0] != "Red" || got[1] != "Selects" {
		t.Fatalf("tags: %v", got)
	}
	if err := writeTags(f, nil); err != nil {
		t.Fatal(err)
	}
	if got := readTags(f); got != nil {
		t.Fatalf("after clearing: %v", got)
	}
}

func TestBplistStrings(t *testing.T) {
	// What the Finder writes for a file tagged Red and "Selects":
	// bplist00, an array of two ASCII strings.
	b := []byte("bplist00")
	// objects: array(2) refs [1,2], "Red\n6", "Selects"
	objs := [][]byte{{0xa2, 1, 2}, append([]byte{0x55}, []byte("Red\n6")...), append([]byte{0x57}, []byte("Selects")...)}
	var offsets []int
	for _, o := range objs {
		offsets = append(offsets, len(b))
		b = append(b, o...)
	}
	table := len(b)
	for _, o := range offsets {
		b = append(b, byte(o))
	}
	trailer := make([]byte, 32)
	trailer[6], trailer[7] = 1, 1 // offset size, ref size
	trailer[15] = byte(len(objs))
	trailer[23] = 0 // top object
	trailer[31] = byte(table)
	b = append(b, trailer...)
	got := bplistStrings(b)
	if len(got) != 2 || got[0] != "Red\n6" || got[1] != "Selects" {
		t.Fatalf("bplist: %q", got)
	}
}

// New folder, rename, move and tag inside a measured place; nothing
// outside it is reachable.
func TestOrganiseThroughAPI(t *testing.T) {
	s, ts, _ := setup(t)
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "Media", "DAY 1"), 0o755)
	os.WriteFile(filepath.Join(root, "Media", "DAY 1", "a.mov"), []byte("aaa"), 0o644)
	os.WriteFile(filepath.Join(root, "loose.mov"), []byte("bbb"), 0o644)
	q := func(s string) string { return string(mustJSON(s)) }
	call(t, s, ts, "POST", "/api/space/scan", `{"path":`+q(root)+`}`, nil)
	waitJobs(t, s, ts)
	post := func(body string) int {
		return call(t, s, ts, "POST", "/api/space/organise", body, nil)
	}
	if code := post(`{"path":` + q(root) + `,"op":"mkdir","rel":"","to":"Selects"}`); code != 200 {
		t.Fatalf("mkdir: %d", code)
	}
	if code := post(`{"path":` + q(root) + `,"op":"move","rel":"loose.mov","to":"Selects"}`); code != 200 {
		t.Fatalf("move: %d", code)
	}
	if code := post(`{"path":` + q(root) + `,"op":"rename","rel":"Selects/loose.mov","to":"keeper.mov"}`); code != 200 {
		t.Fatalf("rename: %d", code)
	}
	if _, err := os.Stat(filepath.Join(root, "Selects", "keeper.mov")); err != nil {
		t.Fatal(err)
	}
	if code := post(`{"path":` + q(root) + `,"op":"move","rel":"../../etc","to":""}`); code == 200 {
		t.Fatal("a path outside the place was accepted")
	}
	if code := post(`{"path":` + q(root) + `,"op":"rename","rel":"Media","to":"bad/name"}`); code == 200 {
		t.Fatal("a name with a slash was accepted")
	}
	if code := post(`{"path":` + q(root) + `,"op":"move","rel":"Media","to":"Media/DAY 1"}`); code == 200 {
		t.Fatal("a folder moved into itself")
	}
	waitJobs(t, s, ts)
	if code := post(`{"path":` + q(root) + `,"op":"tag","rel":"Selects","tags":["Green"]}`); code == 200 {
		var tags map[string][]string
		call(t, s, ts, "GET", "/api/space/tags?path="+urlQuery(root)+"&rel=", "", &tags)
		if len(tags["Selects"]) != 1 || tags["Selects"][0] != "Green" {
			t.Fatalf("tags: %v", tags)
		}
	}
}

func mustJSON(s string) []byte {
	b, _ := json.Marshal(s)
	return b
}

func urlQuery(s string) string { return url.QueryEscape(s) }

// All the PDFs anywhere in a place, picked and moved together into one
// folder; the folder tree offered as destinations.
func TestMoveManyThroughAPI(t *testing.T) {
	s, ts, _ := setup(t)
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "Media", "DAY 1"), 0o755)
	os.MkdirAll(filepath.Join(root, "Paperwork"), 0o755)
	os.MkdirAll(filepath.Join(root, ".hidden"), 0o755)
	os.WriteFile(filepath.Join(root, "Media", "DAY 1", "a.mov"), []byte("aaaa"), 0o644)
	os.WriteFile(filepath.Join(root, "Media", "DAY 1", "callsheet.pdf"), []byte("pp"), 0o644)
	os.WriteFile(filepath.Join(root, "release.pdf"), []byte("ppp"), 0o644)
	os.WriteFile(filepath.Join(root, ".hidden", "x.pdf"), []byte("p"), 0o644)
	q := func(s string) string { return string(mustJSON(s)) }
	call(t, s, ts, "POST", "/api/space/scan", `{"path":`+q(root)+`}`, nil)
	waitJobs(t, s, ts)
	var got struct {
		Files  []spaceFile    `json:"files"`
		Counts map[string]int `json:"counts"`
	}
	call(t, s, ts, "GET", "/api/space/files?path="+urlQuery(root)+"&kind=pdf", "", &got)
	if len(got.Files) != 2 || got.Files[0].Rel != "release.pdf" || got.Files[1].Rel != "Media/DAY 1/callsheet.pdf" {
		t.Fatalf("pdfs: %+v", got.Files)
	}
	if got.Counts["video"] != 1 || got.Counts["pdf"] != 2 {
		t.Fatalf("counts: %v", got.Counts)
	}
	var tree spaceFolder
	call(t, s, ts, "GET", "/api/space/folders?path="+urlQuery(root), "", &tree)
	if len(tree.Children) != 2 || tree.Children[0].Name != "Media" || tree.Children[0].Children[0].Rel != "Media/DAY 1" || tree.Children[0].Children[0].Files != 2 {
		t.Fatalf("folders: %+v", tree)
	}
	var res struct {
		Moved  []string `json:"moved"`
		Failed []string `json:"failed"`
	}
	call(t, s, ts, "POST", "/api/space/organise", `{"path":`+q(root)+`,"op":"move","rels":["release.pdf","Media/DAY 1/callsheet.pdf","../etc/passwd"],"to":"Paperwork"}`, &res)
	if len(res.Moved) != 2 || len(res.Failed) != 1 {
		t.Fatalf("moved %v failed %v", res.Moved, res.Failed)
	}
	for _, n := range []string{"release.pdf", "callsheet.pdf"} {
		if _, err := os.Stat(filepath.Join(root, "Paperwork", n)); err != nil {
			t.Fatal(err)
		}
	}
	waitJobs(t, s, ts)
}
