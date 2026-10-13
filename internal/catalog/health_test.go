package catalog

import (
	"path/filepath"
	"testing"

	"github.com/lumpysheppard/lumpy/internal/volume"
)

func openTest(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func addDrive(t *testing.T, db *DB, id string, size, free int64, files map[string]int64, hashes map[string]string) {
	t.Helper()
	if err := db.SaveDrive(&volume.Volume{ID: id, Name: id, Mount: "/Volumes/" + id, Size: size, Free: free}); err != nil {
		t.Fatal(err)
	}
	var listing []File
	for p, sz := range files {
		listing = append(listing, File{Path: p, Size: sz})
	}
	if _, err := db.SyncFiles(id, listing, hashes); err != nil {
		t.Fatal(err)
	}
}

const gb = int64(1e9)

func TestProjectHealthAndPlan(t *testing.T) {
	db := openTest(t)
	// RAID holds everything; Shuttle holds the camera originals too; the
	// edit SSD holds proxies only, plus an unhashed render.
	raid := map[string]int64{"Cam/A001.mov": 60 * gb, "Cam/A002.mov": 60 * gb, "Proxies/A001.mov": 5 * gb, "Audio/day1.wav": 2 * gb}
	hashes := map[string]string{"Cam/A001.mov": "aaaa", "Cam/A002.mov": "bbbb", "Proxies/A001.mov": "cccc", "Audio/day1.wav": "dddd"}
	addDrive(t, db, "RAID", 1000*gb, 400*gb, raid, hashes)
	addDrive(t, db, "SHUTTLE", 500*gb, 300*gb,
		map[string]int64{"Day01/A001.mov": 60 * gb, "Day01/A002.mov": 60 * gb},
		map[string]string{"Day01/A001.mov": "aaaa", "Day01/A002.mov": "bbbb"})
	addDrive(t, db, "EDIT", 200*gb, 50*gb,
		map[string]int64{"Proxies/A001.mov": 5 * gb, "Renders/cut1.mov": 20 * gb},
		map[string]string{"Proxies/A001.mov": "cccc"})

	h, err := db.ProjectHealth([]string{"RAID", "SHUTTLE", "EDIT"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if h.Contents != 5 || h.Backed != 3 || h.Sole != 1 || h.Unknown != 1 {
		t.Fatalf("health: %+v", h)
	}
	if h.SoleBytes != 2*gb || h.UnknownBytes != 20*gb || h.BackedBytes != 125*gb {
		t.Fatalf("bytes: sole %d unknown %d backed %d", h.SoleBytes, h.UnknownBytes, h.BackedBytes)
	}
	var raidH DriveHealth
	for _, d := range h.Drives {
		if d.ID == "RAID" {
			raidH = d
		}
	}
	if raidH.Sole != 1 || raidH.SoleBytes != 2*gb || raidH.Backed != 3 {
		t.Fatalf("RAID: %+v", raidH)
	}

	plan := MakePlan(h, []DriveSpace{{"RAID", 400 * gb, true}, {"SHUTTLE", 300 * gb, false}, {"EDIT", 50 * gb, true}})
	if len(plan.Transfers) != 1 || len(plan.Unplaced) != 0 {
		t.Fatalf("plan: %+v", plan)
	}
	tr := plan.Transfers[0]
	// The audio should go to the plugged-in EDIT drive, which has room.
	if tr.SourceID != "RAID" || tr.DestID != "EDIT" || len(tr.Files) != 1 || tr.Files[0].Path != "Audio/day1.wav" {
		t.Fatalf("transfer: %+v", tr)
	}

	// Raise the target to 3: the camera originals need a third drive, and
	// only RAID/EDIT... EDIT has 50 GB: too small for a 60 GB clip.
	h3, _ := db.ProjectHealth([]string{"RAID", "SHUTTLE", "EDIT"}, 3)
	plan3 := MakePlan(h3, []DriveSpace{{"RAID", 400 * gb, true}, {"SHUTTLE", 300 * gb, false}, {"EDIT", 50 * gb, true}})
	if h3.Short != 3 || h3.Sole != 1 {
		t.Fatalf("target 3: %+v", h3)
	}
	if len(plan3.Unplaced) != 2 || plan3.UnplacedBytes != 120*gb {
		t.Fatalf("expected the two clips to be unplaceable: %+v", plan3.Unplaced)
	}
	// The proxy (5 GB) fits on SHUTTLE, which does not hold it.
	found := false
	for _, tr := range plan3.Transfers {
		for _, f := range tr.Files {
			if f.XXH64 == "cccc" && tr.DestID == "SHUTTLE" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("proxy not planned onto SHUTTLE: %+v", plan3.Transfers)
	}
}

func TestPlanKeepsPairsTogether(t *testing.T) {
	db := openTest(t)
	files := map[string]int64{}
	hashes := map[string]string{}
	for i := 0; i < 20; i++ {
		p := "Cam/clip" + string(rune('A'+i)) + ".mov"
		files[p] = 10 * gb
		hashes[p] = "h" + string(rune('A'+i))
	}
	addDrive(t, db, "CARD", 300*gb, 0, files, hashes)
	addDrive(t, db, "D1", 1000*gb, 500*gb, nil, nil)
	addDrive(t, db, "D2", 1000*gb, 490*gb, nil, nil)
	h, _ := db.ProjectHealth([]string{"CARD", "D1", "D2"}, 2)
	plan := MakePlan(h, []DriveSpace{{"CARD", 0, true}, {"D1", 500 * gb, true}, {"D2", 490 * gb, true}})
	// All 200 GB fit on one drive, so one transfer, not twenty.
	if len(plan.Transfers) != 1 || plan.Transfers[0].Bytes != 200*gb {
		t.Fatalf("got %d transfers: %+v", len(plan.Transfers), plan.Transfers)
	}
}

func TestProjectsCRUD(t *testing.T) {
	db := openTest(t)
	addDrive(t, db, "A", gb, gb, nil, nil)
	p, err := db.CreateProject("Feature", 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetProjectDrive(p.ID, "A", true); err != nil {
		t.Fatal(err)
	}
	p.Name, p.Target, p.Color = "Feature Film", 3, "red"
	if err := db.UpdateProject(p); err != nil {
		t.Fatal(err)
	}
	got, _ := db.Project(p.ID)
	if got.Name != "Feature Film" || got.Target != 3 || len(got.DriveIDs) != 1 || got.DriveIDs[0] != "A" {
		t.Fatalf("got %+v", got)
	}
	m, _ := db.DriveProjects()
	if len(m["A"]) != 1 {
		t.Fatal("drive not linked")
	}
	db.Forget("A")
	got, _ = db.Project(p.ID)
	if len(got.DriveIDs) != 0 {
		t.Fatal("forgotten drive still in project")
	}
	db.DeleteProject(p.ID)
	if ps, _ := db.Projects(); len(ps) != 0 {
		t.Fatal("project not deleted")
	}
}
