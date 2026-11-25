package catalog

import (
	"testing"

	"github.com/lumpysheppard/lumpy/internal/volume"
)

// A drive known by its name comes back with a new name: its log moves
// to the new id whole, checksums and all, and the old entry goes.
func TestRekeyMovesEverything(t *testing.T) {
	db := openTest(t)
	addDrive(t, db, "name:OLD:1000", 1000, 100, map[string]int64{"a.mov": 10, "b.mov": 20}, map[string]string{"a.mov": "aaaa", "b.mov": "bbbb"})
	db.Log("name:OLD:1000", "verify", "all good", 0)
	if err := db.Rekey("name:OLD:1000", "name:NEW:1000"); err != nil {
		t.Fatal(err)
	}
	if db.HasDrive("name:OLD:1000") || !db.HasDrive("name:NEW:1000") {
		t.Fatal("old id still there, or new id missing")
	}
	d, _ := db.Drive("name:NEW:1000")
	if d.Files != 2 || d.Hashed != 2 || d.Bytes != 30 {
		t.Fatalf("totals: %+v", d)
	}
	files, _ := db.Files("name:NEW:1000")
	if len(files) != 2 || files[0].XXH64 == "" {
		t.Fatalf("files: %+v", files)
	}
	evs, _ := db.Events("name:NEW:1000", 10)
	if len(evs) != 1 {
		t.Fatalf("events: %+v", evs)
	}
}

// Folding into a drive that was already scanned afresh: the old rows,
// which carry the checksums, win for a path both know.
func TestRekeyFoldsIntoScannedDrive(t *testing.T) {
	db := openTest(t)
	addDrive(t, db, "name:OLD:1000", 1000, 100, map[string]int64{"a.mov": 10, "b.mov": 20}, map[string]string{"a.mov": "aaaa", "b.mov": "bbbb"})
	addDrive(t, db, "part:GUID", 1000, 100, map[string]int64{"a.mov": 10, "c.mov": 5}, nil)
	if err := db.Rekey("name:OLD:1000", "part:GUID"); err != nil {
		t.Fatal(err)
	}
	d, _ := db.Drive("part:GUID")
	if d.Files != 3 || d.Hashed != 2 || d.Bytes != 35 {
		t.Fatalf("totals after fold: %+v", d)
	}
	files, _ := db.Files("part:GUID")
	got := map[string]string{}
	for _, f := range files {
		got[f.Path] = f.XXH64
	}
	if got["a.mov"] != "aaaa" || got["b.mov"] != "bbbb" || got["c.mov"] != "" {
		t.Fatalf("files: %v", got)
	}
}

// A volume that now carries a partition id but was logged by name is
// carried over when it is next saved.
func TestSaveDriveCarriesLegacyID(t *testing.T) {
	db := openTest(t)
	addDrive(t, db, "name:X:1000", 1000, 100, map[string]int64{"a.mov": 10}, map[string]string{"a.mov": "aaaa"})
	if err := db.SaveDrive(&volume.Volume{ID: "part:ABC", LegacyID: "name:X:1000", Name: "X", Mount: "/Volumes/X", Size: 1000, Free: 100}); err != nil {
		t.Fatal(err)
	}
	if db.HasDrive("name:X:1000") {
		t.Fatal("legacy entry left behind")
	}
	d, _ := db.Drive("part:ABC")
	if d == nil || d.Files != 1 || d.Hashed != 1 {
		t.Fatalf("carried over: %+v", d)
	}
}
