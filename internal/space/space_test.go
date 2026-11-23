package space

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScanPrunesAndSums(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "big", "deep"), 0o755)
	os.WriteFile(filepath.Join(dir, "big", "deep", "clip.mov"), make([]byte, 2000000), 0o644)
	os.MkdirAll(filepath.Join(dir, "many"), 0o755)
	for i := 0; i < Keep+40; i++ {
		os.WriteFile(filepath.Join(dir, "many", fmt.Sprintf("f%03d", i)), make([]byte, 4096+i), 0o644)
	}
	os.Symlink(filepath.Join(dir, "big"), filepath.Join(dir, "link"))
	var prog Progress
	n, err := Scan(context.Background(), dir, &prog)
	if err != nil {
		t.Fatal(err)
	}
	if n.Files != 1+Keep+40 || prog.Files.Load() != n.Files {
		t.Fatalf("files %d, progress %d", n.Files, prog.Files.Load())
	}
	if n.At("link") != nil {
		t.Fatal("symlinks are not followed")
	}
	many := n.At("many")
	if len(many.Children) != Keep+1 || many.Children[Keep].Rest != 40 {
		t.Fatalf("many: %d children, rest %d", len(many.Children), many.Children[Keep].Rest)
	}
	if n.Children[0].Name != "big" {
		t.Fatalf("biggest first, got %s", n.Children[0].Name)
	}
	clip := n.At("big/deep/clip.mov")
	if clip == nil || clip.Bytes < 2000000 {
		t.Fatalf("clip: %+v", clip)
	}
	before := n.Bytes
	if !n.Remove("big/deep/clip.mov") || n.Bytes != before-clip.Bytes || n.At("big").Files != 0 {
		t.Fatalf("remove: bytes %d of %d, big files %d", n.Bytes, before, n.At("big").Files)
	}
	tr := n.Trim(1, 0)
	if len(tr.Children) == 0 || tr.Children[0].Children != nil {
		t.Fatal("trim keeps one level")
	}
	// Below the first level, small children roll up.
	tr = n.Trim(3, 1<<40)
	m := tr.At("many")
	if m == nil || len(m.Children) != 1 || m.Children[0].Rest != Keep+40 {
		t.Fatalf("deep trim should roll many's children into one: %+v", m)
	}
}

func TestDupesAndOld(t *testing.T) {
	dir := t.TempDir()
	same := make([]byte, BigSize+5)
	for i := range same {
		same[i] = byte(i)
	}
	os.MkdirAll(filepath.Join(dir, "a"), 0o755)
	os.MkdirAll(filepath.Join(dir, "b"), 0o755)
	os.WriteFile(filepath.Join(dir, "a", "x.mov"), same, 0o644)
	os.WriteFile(filepath.Join(dir, "b", "copy of x.mov"), same, 0o644)
	diff := make([]byte, BigSize+5)
	os.WriteFile(filepath.Join(dir, "b", "y.mov"), diff, 0o644) // same length, other bytes
	old := filepath.Join(dir, "old.bin")
	os.WriteFile(old, same, 0o644)
	past := time.Now().Add(-400 * 24 * time.Hour)
	os.Chtimes(old, past, past)
	r, err := ScanAll(context.Background(), dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Big) != 4 {
		t.Fatalf("big files: %d", len(r.Big))
	}
	groups, wasted, err := Dupes(context.Background(), dir, r.Big, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || len(groups[0].Paths) != 3 || wasted != 2*int64(len(same)) {
		t.Fatalf("groups %+v wasted %d", groups, wasted)
	}
	olds, total := Old(r.Big, 365*24*time.Hour, 10)
	if len(olds) != 1 || olds[0].Rel != "old.bin" || total == 0 {
		t.Fatalf("old: %+v", olds)
	}
}
