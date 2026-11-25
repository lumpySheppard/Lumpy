package mhl

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lumpysheppard/lumpy/internal/hashing"
)

func TestC4MatchesReference(t *testing.T) {
	// From ascmhl.hasher.C4 (the ASC reference implementation).
	want := "c44S4AGdA3Ha4YnTrBjDntgb9yz4zwGs826AbAzNTKa9x1XgmxS3B2ixcLPn7D6DyrX3gNnFoAb5aENiyFzqTuHNar"
	if got := C4([]byte("lumpy")); got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestWriteThenLoad(t *testing.T) {
	root := filepath.Join(t.TempDir(), "A001")
	now := time.Now()
	for i, hex := range []string{"1111111111111111", "2222222222222222"} {
		action := "original"
		if i > 0 {
			action = "verified"
		}
		_, err := Write(root, &Generation{Process: "transfer", Created: now.Add(time.Duration(i) * time.Second), Hostname: "h", ToolVersion: "t",
			Entries: []Entry{{Path: "C/a & b.mov", Size: 3, ModTime: now, Format: hashing.XXH64, Hex: hex, Action: action, HashDate: now}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	h, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Generations) != 2 || len(h.Problems) != 0 {
		t.Fatalf("generations %v, problems %v", h.Generations, h.Problems)
	}
	got, ok := h.Expected("C/a & b.mov")
	if !ok || got.Hex != "2222222222222222" || got.Size != 3 {
		t.Fatalf("expected newest hash, got %+v", got)
	}

	// Editing a generation file must be noticed through the chain's C4 ID.
	p := filepath.Join(root, "ascmhl", h.Generations[0])
	data, _ := os.ReadFile(p)
	os.WriteFile(p, append(data, ' '), 0o644)
	h, _ = Load(root)
	if len(h.Problems) != 1 {
		t.Fatalf("tampering not detected: %v", h.Problems)
	}
}

func TestLoadLegacy(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "A001_2020.mhl")
	os.WriteFile(f, []byte(`<?xml version="1.0" encoding="UTF-8"?>
<hashlist version="1.1">
  <hash>
    <file>A001/CLIPS/x.mov</file>
    <size>5</size>
    <md5>ABCDEF</md5>
    <xxhash64be>0123456789abcdef</xxhash64be>
  </hash>
</hashlist>`), 0o644)
	h, err := LoadLegacy(f)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := h.Expected("A001/CLIPS/x.mov")
	if !ok || got.Format != hashing.XXH64 || got.Hex != "0123456789abcdef" || got.Size != 5 {
		t.Fatalf("got %+v", got)
	}
}
