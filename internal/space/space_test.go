package space

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path string, n int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, n), 0o644); err != nil {
		t.Fatal(err)
	}
}

// sum checks that every folder's size is its own blocks plus its children.
func sum(t *testing.T, n *Node) int64 {
	t.Helper()
	var files int64
	for _, c := range n.Kids {
		sum(t, c)
		files += c.Files
	}
	if n.Dir && files != n.Files {
		t.Fatalf("%s: %d files, children hold %d", n.Name, n.Files, files)
	}
	return n.Size
}

func TestScanSizesAndOrder(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "big", "a.mov"), 3<<20)
	write(t, filepath.Join(root, "big", "b.mov"), 1<<20)
	write(t, filepath.Join(root, "small", "c.txt"), 10<<10)
	write(t, filepath.Join(root, "top.bin"), 2<<20)
	if err := os.Symlink(filepath.Join(root, "big"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}

	var p Progress
	n, err := Scan(context.Background(), root, &p)
	if err != nil {
		t.Fatal(err)
	}
	sum(t, n)
	if n.Files != 5 { // four files and the link itself
		t.Fatalf("files = %d", n.Files)
	}
	if n.Kids[0].Name != "big" || n.Kids[1].Name != "top.bin" {
		t.Fatalf("order: %s, %s", n.Kids[0].Name, n.Kids[1].Name)
	}
	if n.Kids[0].Size < 4<<20 {
		t.Fatalf("big = %d bytes", n.Kids[0].Size)
	}
	if p.Files.Load() != 5 || p.Bytes.Load() <= 0 {
		t.Fatalf("progress %d files %d bytes", p.Files.Load(), p.Bytes.Load())
	}
}

func TestHardLinksCountOnce(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "a", "clip.mov"), 4<<20)
	if err := os.MkdirAll(filepath.Join(root, "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(root, "a", "clip.mov"), filepath.Join(root, "b", "clip.mov")); err != nil {
		t.Skip("no hard links here:", err)
	}
	n, err := Scan(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n.Size >= 8<<20 {
		t.Fatalf("a hard-linked file was counted twice: %d bytes", n.Size)
	}
}

func TestSmallFilesAreSummed(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < KeepFiles+20; i++ {
		write(t, filepath.Join(root, fmt.Sprintf("f%03d", i)), 5000+i*10)
	}
	n, err := Scan(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Kids) != KeepFiles+1 {
		t.Fatalf("%d entries, want %d", len(n.Kids), KeepFiles+1)
	}
	var rest *Node
	for _, c := range n.Kids {
		if c.Rest {
			rest = c
		}
	}
	if rest == nil || rest.Files != 20 {
		t.Fatalf("rest = %+v", rest)
	}
	if n.Files != int64(KeepFiles+20) {
		t.Fatalf("files = %d", n.Files)
	}
}

func TestUnreadableFolderIsMarked(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads everything")
	}
	root := t.TempDir()
	write(t, filepath.Join(root, "locked", "x"), 100)
	if err := os.Chmod(filepath.Join(root, "locked"), 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(filepath.Join(root, "locked"), 0o755) })
	var p Progress
	n, err := Scan(context.Background(), root, &p)
	if err != nil {
		t.Fatal(err)
	}
	if !n.Kids[0].Unreadable || p.Unreadable.Load() != 1 {
		t.Fatalf("locked folder not marked: %+v", n.Kids[0])
	}
}

func TestFindRemoveAndView(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "x", "y", "z.bin"), 2<<20)
	write(t, filepath.Join(root, "x", "w.bin"), 1<<20)
	n, err := Scan(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	z, up := Find(n, "x/y/z.bin")
	if z == nil || len(up) != 3 {
		t.Fatalf("find: %v %d", z, len(up))
	}
	if f, _ := Find(n, "x/../x"); f != nil {
		t.Fatal(".. must not resolve")
	}
	before := n.Size
	if !Remove(n, "x/y/z.bin") {
		t.Fatal("remove failed")
	}
	if n.Size != before-z.Size || n.Files != 1 {
		t.Fatalf("after remove: %d bytes %d files", n.Size, n.Files)
	}
	if Remove(n, "") {
		t.Fatal("the root must not be removable")
	}
	v := ViewOf(n, "", 2, 0, 10)
	if len(v.Kids) != 1 || v.Kids[0].Path != "x" || v.Kids[0].Kids[0].Path != "x/w.bin" {
		t.Fatalf("view: %+v", v.Kids[0])
	}
	if v := ViewOf(n, "", 1, 1<<30, 10); len(v.Kids) != 0 || v.More != 1 {
		t.Fatalf("small children kept: %+v", v)
	}
}

func TestCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Scan(ctx, t.TempDir(), nil); err == nil {
		t.Fatal("a cancelled scan returned no error")
	}
}

func TestVolumeOf(t *testing.T) {
	v, err := VolumeOf("/")
	if err != nil {
		t.Fatal(err)
	}
	if !v.Root || v.Total <= 0 || v.Free < 0 || v.Free > v.Total {
		t.Fatalf("%+v", v)
	}
}
