package catalog

import (
	"fmt"
	"os"
	"testing"
	"time"
)

// A library the size of a busy production: a dozen drives, one with
// 115k files, the rest a few thousand each, half checksummed, with
// names that repeat across cards the way camera files do. Run with
// LUMPY_PERF=1 to see how long the app's queries take on it.
func TestPerfBigLibrary(t *testing.T) {
	if os.Getenv("LUMPY_PERF") == "" {
		t.Skip("set LUMPY_PERF=1")
	}
	db := openTest(t)
	counts := []int{115000, 7400, 5100, 4650, 2500, 2400, 2100, 2080, 1500, 1200, 1000, 900}
	var sources []Source
	for i, n := range counts {
		id := fmt.Sprintf("D%02d", i)
		files := map[string]int64{}
		hashes := map[string]string{}
		for j := 0; j < n; j++ {
			// Sony-style names repeat per card; sizes mostly differ.
			p := fmt.Sprintf("DAY %d/CARD%d/C%04d.MP4", j/400, (j/100)%7, j%400)
			size := int64(1e6 + (j*7919)%5e8)
			files[p] = size
			if j%2 == 0 {
				hashes[p] = fmt.Sprintf("%016x", uint64(j)*2654435761+uint64(i%3)) // copies shared among 3 drives
			}
		}
		addDrive(t, db, id, 10e12, 1e12, files, hashes)
		if i < 8 {
			sources = append(sources, Source{DriveID: id, Folder: ""})
		}
	}
	timeIt := func(name string, f func() error) {
		start := time.Now()
		if err := f(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		t.Logf("%-18s %s", name, time.Since(start).Round(time.Millisecond))
	}
	for _, id := range []string{"D00", "D05"} {
		timeIt("Coverage "+id, func() error { _, err := db.Coverage(id); return err })
		timeIt("Backups "+id, func() error { _, err := db.Backups(id, 1); return err })
		timeIt("FolderHealths "+id, func() error { _, err := db.FolderHealths(id); return err })
		timeIt("FileStates "+id, func() error { _, err := db.BackupStates(id); return err })
	}
	timeIt("ProjectHealth x8", func() error { _, err := db.ProjectHealth(sources, 2); return err })
	timeIt("Drives", func() error { _, err := db.Drives(); return err })
}

// The counts on each drive row must follow every way files change.
func TestDriveCounters(t *testing.T) {
	db := openTest(t)
	addDrive(t, db, "A", 10*gb, 5*gb, map[string]int64{"x/1.mov": 100, "x/2.mov": 200, "y/3.wav": 50}, map[string]string{"x/1.mov": "aa", "x/2.mov": "bb"})
	check := func(when string, files int, bytes int64, hashed, damaged int) {
		t.Helper()
		d, err := db.Drive("A")
		if err != nil {
			t.Fatal(err)
		}
		if d.Files != files || d.Bytes != bytes || d.Hashed != hashed || d.Damaged != damaged {
			t.Errorf("%s: files %d bytes %d hashed %d damaged %d, want %d %d %d %d", when, d.Files, d.Bytes, d.Hashed, d.Damaged, files, bytes, hashed, damaged)
		}
	}
	check("after sync", 3, 350, 2, 0)
	// The same listing again: nothing changes (the upsert path).
	if _, err := db.SyncFiles("A", []File{{Path: "x/1.mov", Size: 100}, {Path: "x/2.mov", Size: 200}, {Path: "y/3.wav", Size: 50}}, nil); err != nil {
		t.Fatal(err)
	}
	check("after resync", 3, 350, 2, 0)
	// One file grew (and lost its checksum), one went, one is new.
	if _, err := db.SyncFiles("A", []File{{Path: "x/1.mov", Size: 150, ModTime: 5}, {Path: "y/3.wav", Size: 50}, {Path: "z/4.mov", Size: 1000}}, nil); err != nil {
		t.Fatal(err)
	}
	check("after changes", 3, 1200, 0, 0)
	if err := db.MovePaths("A", map[string]string{"z": "_Unused/z"}); err != nil {
		t.Fatal(err)
	}
	check("after move", 3, 1200, 0, 0)
	if err := db.RemovePaths("A", []string{"_Unused"}); err != nil {
		t.Fatal(err)
	}
	check("after remove", 2, 200, 0, 0)
	// Opening the library again recounts nothing and loses nothing.
	path := db.path
	db.Close()
	db2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	d, _ := db2.Drive("A")
	if d.Files != 2 || d.Bytes != 200 {
		t.Errorf("after reopen: %+v", d)
	}
}
