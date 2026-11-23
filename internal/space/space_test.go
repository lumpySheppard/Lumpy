package space

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
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
	tr := n.Trim(1)
	if len(tr.Children) == 0 || tr.Children[0].Children != nil {
		t.Fatal("trim keeps one level")
	}
}
