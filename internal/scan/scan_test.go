package scan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtraIgnores(t *testing.T) {
	root := t.TempDir()
	for _, p := range []string{"CLIPS/a.mov", "CLIPS/._a.mov", ".DS_Store", "Private/secret.mov", "node_modules/x.js", "cache_1/y.mov", "keep/z.mov"} {
		os.MkdirAll(filepath.Join(root, filepath.Dir(p)), 0o755)
		os.WriteFile(filepath.Join(root, p), []byte("x"), 0o644)
	}
	SetExtraIgnores([]string{"Private", "node_modules", "cache_*"})
	defer SetExtraIgnores(nil)
	res, err := Dir(root)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range res.Files {
		got = append(got, f.Path)
	}
	if len(got) != 2 || got[0] != "CLIPS/a.mov" || got[1] != "keep/z.mov" {
		t.Fatalf("files: %v", got)
	}
}
