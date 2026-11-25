package catalog

import (
	"testing"

	"github.com/lumpysheppard/lumpy/internal/media"
)

// A clip's metadata row goes when its file leaves the drive, so the
// Clips page never lists a file that isn't there.
func TestMediaRowGoesWithFile(t *testing.T) {
	db := openTest(t)
	addDrive(t, db, "A", 100, 50, map[string]int64{"Day01/a.mov": 10, "Day01/b.mov": 10}, nil)
	for _, p := range []string{"Day01/a.mov", "Day01/b.mov"} {
		if err := db.SaveMedia("A", p, "", 10, &media.Info{Kind: media.Video, Container: "QuickTime"}); err != nil {
			t.Fatal(err)
		}
	}
	if rows, _ := db.ListMedia(MediaFilter{DriveID: "A"}); len(rows) != 2 {
		t.Fatalf("before: %d rows", len(rows))
	}
	// b.mov is gone from the drive at the next scan.
	if _, err := db.SyncFiles("A", []File{{Path: "Day01/a.mov", Size: 10}}, nil); err != nil {
		t.Fatal(err)
	}
	rows, _ := db.ListMedia(MediaFilter{DriveID: "A"})
	if len(rows) != 1 || rows[0].Path != "Day01/a.mov" {
		t.Fatalf("after: %+v", rows)
	}
}
