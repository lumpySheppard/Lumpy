package catalog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrefsAndLibraryUpkeep(t *testing.T) {
	db := openTest(t)
	if p := db.Prefs(); !p.HideInternal || !p.HideImages || !p.AutoLog || !p.AutoMetadata || p.RescanMinutes != 0 {
		t.Fatalf("defaults: %+v", p)
	}
	if err := db.SavePrefs(Prefs{Exclude: " .git, node_modules ,, bad/path, Private*", RescanMinutes: -5, AutoLog: true}); err != nil {
		t.Fatal(err)
	}
	p := db.Prefs()
	if p.Exclude != ".git, node_modules, Private*" || p.RescanMinutes != 0 || p.HideInternal || !p.AutoLog {
		t.Fatalf("saved: %+v", p)
	}
	if l := p.ExcludeList(); len(l) != 3 || l[2] != "Private*" {
		t.Fatalf("list: %v", l)
	}

	addDrive(t, db, "D1", 1e12, 5e11, map[string]int64{"A/x.mov": 10, "A/y.mov": 20}, nil)
	db.Log("D1", "scanned", "2 files", 0)
	info, err := db.Info()
	if err != nil || info.Drives != 1 || info.Files != 2 || info.Events != 1 || info.Bytes == 0 || info.Path == "" {
		t.Fatalf("info: %+v %v", info, err)
	}
	// A consistent copy can be taken while the library is open.
	copyPath := filepath.Join(t.TempDir(), "moved", "catalog.db")
	if err := db.CopyTo(copyPath); err != nil {
		t.Fatal(err)
	}
	if err := db.CopyTo(copyPath); err == nil {
		t.Fatal("copying over an existing file should be refused")
	}
	other, err := Open(copyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if oi, _ := other.Info(); oi.Files != 2 {
		t.Fatalf("copy: %+v", oi)
	}
	if err := db.Optimise(); err != nil {
		t.Fatal(err)
	}
	if err := db.ClearHistory(); err != nil {
		t.Fatal(err)
	}
	if err := db.ForgetAll(); err != nil {
		t.Fatal(err)
	}
	if info, _ := db.Info(); info.Drives != 0 || info.Files != 0 || info.Events != 0 {
		t.Fatalf("after clearing: %+v", info)
	}

	// The location file redirects the default path.
	home := t.TempDir()
	t.Setenv("LUMPY_HOME", "")
	t.Setenv("HOME", home)
	if err := SetLibraryLocation("/Volumes/LIBRARY/Lumpy"); err != nil {
		t.Fatal(err)
	}
	if p, _ := DefaultPath(); p != filepath.Join("/Volumes/LIBRARY/Lumpy", "catalog.db") {
		t.Fatalf("redirected path: %s", p)
	}
	if err := SetLibraryLocation(""); err != nil {
		t.Fatal(err)
	}
	if p, _ := DefaultPath(); p != filepath.Join(home, "Library", "Application Support", "Lumpy", "catalog.db") {
		t.Fatalf("default path: %s", p)
	}
	if _, err := os.Stat(filepath.Join(home, "Library", "Application Support", "Lumpy", "location")); err == nil {
		t.Fatal("location file should be gone")
	}
}
