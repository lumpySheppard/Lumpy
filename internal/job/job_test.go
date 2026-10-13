package job

import (
	"bytes"
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lumpysheppard/lumpy/internal/mhl"
)

func write(t *testing.T, p string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// card makes a small camera card: a clip bigger than one copy chunk, an
// empty file, an empty folder and a Finder file that must not be copied.
func card(t *testing.T) (dir, src string) {
	dir = t.TempDir()
	src = filepath.Join(dir, "A001")
	clip := make([]byte, chunkSize*2+123)
	rand.Read(clip)
	write(t, filepath.Join(src, "CLIPS", "A001C001.mov"), clip)
	write(t, filepath.Join(src, "CLIPS", "empty.bin"), nil)
	write(t, filepath.Join(src, ".DS_Store"), []byte("finder"))
	os.MkdirAll(filepath.Join(src, "EMPTY"), 0o755)
	return dir, src
}

func quiet(t *testing.T) (*Report, *bytes.Buffer) {
	var buf bytes.Buffer
	return &Report{out: &buf}, &buf
}

func offload(t *testing.T, src string, dests ...string) (*Report, string) {
	r, buf := quiet(t)
	err := Offload(context.Background(), OffloadOptions{Source: src, Dests: dests, Meta: Meta{Version: "test", Hostname: "test"}}, r)
	if err != nil {
		t.Fatalf("offload: %v\n%s", err, buf)
	}
	return r, buf.String()
}

func TestOffloadToTwoDrives(t *testing.T) {
	dir, src := card(t)
	d1, d2 := filepath.Join(dir, "d1"), filepath.Join(dir, "d2")
	os.Mkdir(d1, 0o755)
	os.Mkdir(d2, 0o755)

	r, out := offload(t, src, d1, d2)
	if r.Failures() != 0 {
		t.Fatalf("failures:\n%s", out)
	}
	for _, d := range []string{d1, d2} {
		root := filepath.Join(d, "A001")
		a, _ := os.ReadFile(filepath.Join(src, "CLIPS", "A001C001.mov"))
		b, err := os.ReadFile(filepath.Join(root, "CLIPS", "A001C001.mov"))
		if err != nil || !bytes.Equal(a, b) {
			t.Fatalf("%s: clip not copied intact", d)
		}
		if _, err := os.Stat(filepath.Join(root, "EMPTY")); err != nil {
			t.Errorf("%s: empty folder not recreated", d)
		}
		if _, err := os.Stat(filepath.Join(root, ".DS_Store")); err == nil {
			t.Errorf("%s: .DS_Store was copied", d)
		}
		h, _ := mhl.Load(root)
		if len(h.Generations) != 1 || len(h.Hashes) != 2 {
			t.Errorf("%s: MHL has %d generations, %d files", d, len(h.Generations), len(h.Hashes))
		}
		vr, vbuf := quiet(t)
		if err := Verify(context.Background(), root, vr); err != nil || vr.Failures() != 0 {
			t.Errorf("%s: verify failed: %v\n%s", d, err, vbuf)
		}
	}
	vr, _ := quiet(t)
	Compare(context.Background(), filepath.Join(d1, "A001"), filepath.Join(d2, "A001"), vr)
	if vr.Failures() != 0 {
		t.Error("compare of two good copies failed")
	}
}

func TestVerifyCatchesDamage(t *testing.T) {
	dir, src := card(t)
	d1 := filepath.Join(dir, "d1")
	os.Mkdir(d1, 0o755)
	offload(t, src, d1)

	clip := filepath.Join(d1, "A001", "CLIPS", "A001C001.mov")
	f, _ := os.OpenFile(clip, os.O_WRONLY, 0)
	f.WriteAt([]byte{0xff, 0x00, 0xff}, 5000)
	f.Close()
	os.Remove(filepath.Join(d1, "A001", "CLIPS", "empty.bin"))

	r, buf := quiet(t)
	Verify(context.Background(), filepath.Join(d1, "A001"), r)
	out := buf.String()
	if r.Failures() != 2 || !strings.Contains(out, "does not match") || !strings.Contains(out, "is missing") {
		t.Fatalf("expected a mismatch and a missing file:\n%s", out)
	}

	// Offloading again restores the missing file but leaves the damaged
	// one alone and reports it.
	r, out2 := offload(t, src, d1)
	if r.Failures() != 1 || !strings.Contains(out2, "does not match the source") {
		t.Fatalf("re-run:\n%s", out2)
	}
	if _, err := os.Stat(filepath.Join(d1, "A001", "CLIPS", "empty.bin")); err != nil {
		t.Fatal("missing file was not recopied")
	}
}

func TestSourceCheckedAgainstItsOwnMHL(t *testing.T) {
	dir, src := card(t)
	d1, d2 := filepath.Join(dir, "d1"), filepath.Join(dir, "d2")
	os.Mkdir(d1, 0o755)
	os.Mkdir(d2, 0o755)
	// First offload, then treat the copy as a card that arrives with an MHL.
	offload(t, src, d1)
	card2 := filepath.Join(d1, "A001")
	f, _ := os.OpenFile(filepath.Join(card2, "CLIPS", "A001C001.mov"), os.O_WRONLY, 0)
	f.WriteAt([]byte{1}, 10)
	f.Close()

	r, out := offload(t, card2, d2)
	if r.Failures() == 0 || !strings.Contains(out, "does not match its own MHL") {
		t.Fatalf("damaged source not reported:\n%s", out)
	}
	h, _ := mhl.Load(filepath.Join(d2, "A001"))
	if len(h.Generations) != 2 {
		t.Fatalf("card's MHL history not carried over: %v", h.Generations)
	}
}

func TestRefusesOverlap(t *testing.T) {
	_, src := card(t)
	r, _ := quiet(t)
	err := Offload(context.Background(), OffloadOptions{Source: src, Dests: []string{src}}, r)
	if err == nil || !strings.Contains(err.Error(), "overlaps") {
		t.Fatalf("copying into the source was allowed: %v", err)
	}
}

func TestInterruptLeavesNoPartialFiles(t *testing.T) {
	dir, src := card(t)
	d1 := filepath.Join(dir, "d1")
	os.Mkdir(d1, 0o755)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, _ := quiet(t)
	if err := Offload(ctx, OffloadOptions{Source: src, Dests: []string{d1}}, r); err != context.Canceled {
		t.Fatalf("got %v", err)
	}
	filepath.Walk(d1, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			t.Errorf("left behind %s", p)
		}
		return nil
	})
}

func TestSourceReadTwiceCatchesFlakyReader(t *testing.T) {
	dir, src := card(t)
	d1 := filepath.Join(dir, "d1")
	os.Mkdir(d1, 0o755)
	clip := filepath.Join(src, "CLIPS", "A001C001.mov")
	// The card hands back different bytes on the second read.
	afterCopyHook = func() {
		f, _ := os.OpenFile(clip, os.O_WRONLY, 0)
		f.WriteAt([]byte{0x42}, 77)
		f.Close()
	}
	defer func() { afterCopyHook = nil }()

	r, out := offload(t, src, d1)
	if r.Failures() != 1 || !strings.Contains(out, "read differently from the source the second time") {
		t.Fatalf("flaky source not caught:\n%s", out)
	}
	h, _ := mhl.Load(filepath.Join(d1, "A001"))
	if _, listed := h.Hashes["CLIPS/A001C001.mov"]; listed {
		t.Fatal("a file the source read inconsistently was recorded as verified")
	}
	if _, listed := h.Hashes["CLIPS/empty.bin"]; !listed {
		t.Fatal("the good file was not recorded")
	}

	// With the source check off, the same fault goes unnoticed (which is
	// why the check is on by default).
	d2 := filepath.Join(dir, "d2")
	os.Mkdir(d2, 0o755)
	afterCopyHook = func() {
		f, _ := os.OpenFile(clip, os.O_WRONLY, 0)
		f.WriteAt([]byte{0x43}, 77)
		f.Close()
	}
	r2, buf := quiet(t)
	Offload(context.Background(), OffloadOptions{Source: src, Dests: []string{d2}, SkipSourceVerify: true}, r2)
	if r2.Failures() != 0 {
		t.Fatalf("expected no failures without the source check:\n%s", buf)
	}
}

func TestOffloadCopiesExtendedAttributes(t *testing.T) {
	dir, src := card(t)
	d1 := filepath.Join(dir, "d1")
	os.Mkdir(d1, 0o755)
	clip := filepath.Join(src, "CLIPS", "A001C001.mov")
	if err := setXattr(clip, "user.lumpy.test", "tagged"); err != nil {
		t.Skip("filesystem has no extended attributes:", err)
	}
	r, out := offload(t, src, d1)
	if r.Failures() != 0 {
		t.Fatal(out)
	}
	got, err := getXattr(filepath.Join(d1, "A001", "CLIPS", "A001C001.mov"), "user.lumpy.test")
	if err != nil || got != "tagged" {
		t.Fatalf("attribute not copied: %q %v", got, err)
	}
}

func TestVerboseListsEveryFile(t *testing.T) {
	dir, src := card(t)
	d1 := filepath.Join(dir, "d1")
	os.Mkdir(d1, 0o755)
	r, buf := quiet(t)
	r.Verbose = true
	if err := Offload(context.Background(), OffloadOptions{Source: src, Dests: []string{d1}}, r); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"copied    CLIPS/A001C001.mov", "verified  CLIPS/empty.bin on", "source ok CLIPS/A001C001.mov"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}
