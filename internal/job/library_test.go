package job

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lumpysheppard/lumpy/internal/catalog"
	"github.com/lumpysheppard/lumpy/internal/volume"
)

// library sets up a folder whose subfolders act as drives, and an empty
// drive library.
func library(t *testing.T) (vols string, db *catalog.DB) {
	t.Helper()
	vols = t.TempDir()
	t.Setenv("LUMPY_VOLUMES", vols)
	db, err := catalog.Open(filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return vols, db
}

func run(t *testing.T, f func(r *Report) error) (*Report, string) {
	t.Helper()
	r, buf := quiet(t)
	if err := f(r); err != nil {
		t.Fatalf("%v\n%s", err, buf)
	}
	return r, buf.String()
}

func drive(t *testing.T, db *catalog.DB, name string) *catalog.Drive {
	t.Helper()
	d, err := db.Find(name)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestLibraryTracksBackupsOffline(t *testing.T) {
	vols, db := library(t)
	write(t, filepath.Join(vols, "A001", "CLIPS", "C001.mov"), []byte("clip one"))
	write(t, filepath.Join(vols, "A001", "CLIPS", "C002.mov"), []byte("clip two!"))
	os.Mkdir(filepath.Join(vols, "SHUTTLE"), 0o755)
	os.Mkdir(filepath.Join(vols, "RAID"), 0o755)
	ctx := context.Background()

	// The agent logs every drive that is plugged in.
	run(t, func(r *Report) error { return AgentRun(ctx, db, r, func(string, string) {}) })
	if ds, _ := db.Drives(); len(ds) != 3 {
		t.Fatalf("want 3 drives logged, got %d", len(ds))
	}

	// Before any copy the card is not backed up.
	r, _ := run(t, func(r *Report) error { return Backups(db, drive(t, db, "A001"), 1, r) })
	if r.Failures() != 2 {
		t.Fatalf("unbacked card: want 2 failures, got %d", r.Failures())
	}

	run(t, func(r *Report) error {
		return Offload(ctx, OffloadOptions{Source: filepath.Join(vols, "A001"),
			Dests: []string{filepath.Join(vols, "SHUTTLE", "Day01"), filepath.Join(vols, "RAID", "Day01")}, Library: db}, r)
	})
	r, _ = run(t, func(r *Report) error { return Backups(db, drive(t, db, "A001"), 2, r) })
	if r.Failures() != 0 {
		t.Fatal("card should have 2 copies after offload")
	}

	// Unplug the shuttle drive, queue a check, then damage it on the shelf.
	shelf := filepath.Join(t.TempDir(), "SHUTTLE")
	os.Rename(filepath.Join(vols, "SHUTTLE"), shelf)
	run(t, func(r *Report) error { return AgentRun(ctx, db, r, func(string, string) {}) })
	db.RequestVerify(drive(t, db, "SHUTTLE").ID, true)
	os.WriteFile(filepath.Join(shelf, "Day01", "A001", "CLIPS", "C002.mov"), []byte("clip TWO!"), 0o644)
	os.Remove(filepath.Join(shelf, "Day01", "A001", "CLIPS", "C001.mov"))

	// Plugging it back in runs the queued check, which finds both problems.
	os.Rename(shelf, filepath.Join(vols, "SHUTTLE"))
	var notes []string
	r, out := run(t, func(r *Report) error {
		return AgentRun(ctx, db, r, func(title, msg string) { notes = append(notes, title) })
	})
	if r.Failures() != 2 || !strings.Contains(out, "is missing") || !strings.Contains(out, "does not match") {
		t.Fatalf("queued check should report a missing and a damaged file:\n%s", out)
	}
	if len(notes) == 0 || !strings.Contains(notes[len(notes)-1], "FAILED") {
		t.Fatalf("no failure notification: %v", notes)
	}
	d := drive(t, db, "SHUTTLE")
	if !d.VerifyRequested.IsZero() || d.LastVerifyOK || d.Damaged != 1 {
		t.Fatalf("drive state after failed check: %+v", d)
	}

	// The damaged copy no longer counts as a backup.
	r, _ = run(t, func(r *Report) error { return Backups(db, drive(t, db, "A001"), 2, r) })
	if r.Failures() != 2 {
		t.Fatalf("card should be short of copies for both clips, got %d failures", r.Failures())
	}

	// Recopying and checking again restores everything.
	os.Remove(filepath.Join(vols, "SHUTTLE", "Day01", "A001", "CLIPS", "C002.mov"))
	run(t, func(r *Report) error {
		return Offload(ctx, OffloadOptions{Source: filepath.Join(vols, "A001"),
			Dests: []string{filepath.Join(vols, "SHUTTLE", "Day01")}, Library: db}, r)
	})
	v, _ := volume.Of(filepath.Join(vols, "SHUTTLE"))
	r, _ = run(t, func(r *Report) error { return HashDrive(ctx, db, v, true, r) })
	if r.Failures() != 0 {
		t.Fatal("check after repair failed")
	}
	r, _ = run(t, func(r *Report) error { return Backups(db, drive(t, db, "A001"), 2, r) })
	if r.Failures() != 0 {
		t.Fatal("card should have 2 good copies again")
	}

	ms, _ := db.Search("c002", 10)
	if len(ms) != 3 {
		t.Fatalf("find: want the clip on 3 drives, got %d", len(ms))
	}
}

func TestCatalogTakesChecksumsFromMHL(t *testing.T) {
	vols, db := library(t)
	write(t, filepath.Join(vols, "A001", "CLIP.mov"), []byte("footage"))
	os.Mkdir(filepath.Join(vols, "OLD"), 0o755)
	ctx := context.Background()
	// Offload without the library, as another tool would: only the MHL
	// on the drive records the checksum.
	run(t, func(r *Report) error {
		return Offload(ctx, OffloadOptions{Source: filepath.Join(vols, "A001"), Dests: []string{filepath.Join(vols, "OLD")}}, r)
	})
	v, _ := volume.Of(filepath.Join(vols, "OLD"))
	_, out := run(t, func(r *Report) error { _, err := CatalogDrive(ctx, db, v, r); return err })
	if !strings.Contains(out, "1 checksums from MHLs") {
		t.Fatalf("MHL checksum not imported:\n%s", out)
	}
	files, _ := db.Files(v.ID)
	if len(files) != 1 || files[0].Path != "A001/CLIP.mov" || files[0].Source != catalog.SourceMHL {
		t.Fatalf("got %+v", files)
	}
}

// A destination reached through a symlink (as /var is on macOS) must
// still be recorded with paths relative to its drive.
func TestOffloadThroughSymlinkRecordsDrivePaths(t *testing.T) {
	vols, db := library(t)
	write(t, filepath.Join(vols, "A001", "C001.mov"), []byte("clip"))
	os.Mkdir(filepath.Join(vols, "SHUTTLE"), 0o755)
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(vols, link); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	run(t, func(r *Report) error {
		return Offload(context.Background(), OffloadOptions{Source: filepath.Join(link, "A001"),
			Dests: []string{filepath.Join(link, "SHUTTLE", "Day01")}, Library: db}, r)
	})
	for _, name := range []string{"A001", "SHUTTLE"} {
		files, _ := db.Files(drive(t, db, name).ID)
		if len(files) != 1 || strings.HasPrefix(files[0].Path, "..") {
			t.Fatalf("%s recorded as %+v", name, files)
		}
	}
}
