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

func whole(ids ...string) []Source {
	var out []Source
	for _, id := range ids {
		out = append(out, Source{DriveID: id})
	}
	return out
}

func anyDrive(db *DB, connected ...string) []DriveSpace {
	ds, _ := db.Drives()
	on := map[string]bool{}
	for _, c := range connected {
		on[c] = true
	}
	var out []DriveSpace
	for _, d := range ds {
		out = append(out, DriveSpace{ID: d.ID, Free: d.Free, Connected: on[d.ID]})
	}
	return out
}

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

	sources := whole("RAID", "SHUTTLE", "EDIT")
	layout := ProjectLayout(sources, func(id string) string { return id })
	h, err := db.ProjectHealth(sources, 2)
	if err != nil {
		t.Fatal(err)
	}
	if h.Contents != 5 || h.Backed != 3 || h.Sole != 1 || h.Unknown != 1 {
		t.Fatalf("health: %+v", h)
	}
	if h.SoleBytes != 2*gb || h.UnknownBytes != 20*gb || h.BackedBytes != 125*gb {
		t.Fatalf("bytes: sole %d unknown %d backed %d", h.SoleBytes, h.UnknownBytes, h.BackedBytes)
	}
	if h.Sources[0].Sole != 1 || h.Sources[0].SoleBytes != 2*gb || h.Sources[0].Backed != 3 {
		t.Fatalf("RAID: %+v", h.Sources[0])
	}

	plan := MakePlan(h, anyDrive(db, "RAID", "EDIT"), layout)
	if len(plan.Transfers) != 1 || len(plan.Unplaced) != 0 {
		t.Fatalf("plan: %+v", plan)
	}
	tr := plan.Transfers[0]
	// The audio should go to the plugged-in EDIT drive, which has room,
	// under a folder named after its source drive (several sources).
	if tr.SourceID != "RAID" || tr.DestID != "EDIT" || len(tr.Files) != 1 || tr.Files[0].Path != "Audio/day1.wav" || tr.Files[0].Dest != "RAID/Audio/day1.wav" {
		t.Fatalf("transfer: %+v", tr)
	}

	// Raise the target to 3: the camera originals need a third drive and
	// EDIT, with 50 GB, cannot take a 60 GB clip.
	h3, _ := db.ProjectHealth(sources, 3)
	plan3 := MakePlan(h3, anyDrive(db, "RAID", "EDIT"), layout)
	if h3.Short != 3 || h3.Sole != 1 {
		t.Fatalf("target 3: %+v", h3)
	}
	if len(plan3.Unplaced) != 2 || plan3.UnplacedBytes != 120*gb {
		t.Fatalf("expected the two clips to be unplaceable: %+v", plan3.Unplaced)
	}
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

func TestFolderProjectCountsCopiesAnywhere(t *testing.T) {
	db := openTest(t)
	// One shuttle holds three jobs. The Nike job is also on the office
	// RAID under a different path; the BMW job is nowhere else.
	addDrive(t, db, "SHUTTLE", 4000*gb, 1000*gb,
		map[string]int64{"Jobs/Nike/A001.mov": 10 * gb, "Jobs/Nike/A002.mov": 10 * gb, "Jobs/BMW/B001.mov": 30 * gb, "Jobs/Apple/C001.mov": 5 * gb},
		map[string]string{"Jobs/Nike/A001.mov": "n1", "Jobs/Nike/A002.mov": "n2", "Jobs/BMW/B001.mov": "b1", "Jobs/Apple/C001.mov": "c1"})
	addDrive(t, db, "RAID", 12000*gb, 3000*gb,
		map[string]int64{"Clients/Nike/2025/A001.mov": 10 * gb, "Clients/Nike/2025/A002.mov": 10 * gb},
		map[string]string{"Clients/Nike/2025/A001.mov": "n1", "Clients/Nike/2025/A002.mov": "n2"})
	addDrive(t, db, "PORTABLE", 1000*gb, 900*gb, nil, nil)

	nike, _ := db.ProjectHealth([]Source{{DriveID: "SHUTTLE", Folder: "Jobs/Nike"}}, 2)
	if nike.Contents != 2 || nike.Backed != 2 || nike.Sole != 0 {
		t.Fatalf("Nike should be backed up by the RAID copy: %+v", nike)
	}
	bmwSrc := []Source{{DriveID: "SHUTTLE", Folder: "Jobs/BMW"}}
	bmw, _ := db.ProjectHealth(bmwSrc, 2)
	if bmw.Contents != 1 || bmw.Sole != 1 {
		t.Fatalf("BMW should be a sole copy: %+v", bmw)
	}
	// With no destination restriction the plan may use any drive; the
	// emptiest with room is the RAID.
	plan := MakePlan(bmw, anyDrive(db), ProjectLayout(bmwSrc, func(id string) string { return id }))
	if len(plan.Transfers) != 1 || plan.Transfers[0].DestID != "RAID" || plan.Transfers[0].Files[0].Dest != "B001.mov" {
		t.Fatalf("plan: %+v", plan.Transfers)
	}
	// Restricted to the portable, it goes there instead.
	only := []DriveSpace{{ID: "PORTABLE", Free: 900 * gb}}
	plan = MakePlan(bmw, only, ProjectLayout(bmwSrc, func(id string) string { return id }))
	if len(plan.Transfers) != 1 || plan.Transfers[0].DestID != "PORTABLE" {
		t.Fatalf("restricted plan: %+v", plan.Transfers)
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
	src := whole("CARD")
	h, _ := db.ProjectHealth(src, 2)
	plan := MakePlan(h, anyDrive(db, "CARD", "D1", "D2"), ProjectLayout(src, func(id string) string { return id }))
	// All 200 GB fit on one drive, so one transfer, not twenty.
	if len(plan.Transfers) != 1 || plan.Transfers[0].Bytes != 200*gb {
		t.Fatalf("got %d transfers: %+v", len(plan.Transfers), plan.Transfers)
	}
}

func TestProjectsCRUD(t *testing.T) {
	db := openTest(t)
	addDrive(t, db, "A", gb, gb, nil, nil)
	addDrive(t, db, "B", gb, gb, nil, nil)
	p, err := db.CreateProject("Feature", 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AddSource(p.ID, "A", "/Jobs/Nike/"); err != nil {
		t.Fatal(err)
	}
	if err := db.AddSource(p.ID, "A", "../etc"); err == nil {
		t.Fatal("accepted a path outside the drive")
	}
	if err := db.SetDests(p.ID, []string{"B"}); err != nil {
		t.Fatal(err)
	}
	p.Name, p.Target, p.Color = "Feature Film", 3, "red"
	if err := db.UpdateProject(p); err != nil {
		t.Fatal(err)
	}
	got, _ := db.Project(p.ID)
	if got.Name != "Feature Film" || got.Target != 3 || len(got.Sources) != 1 || got.Sources[0] != (Source{"A", "Jobs/Nike"}) || len(got.DestIDs) != 1 {
		t.Fatalf("got %+v", got)
	}
	m, _ := db.DriveProjects()
	if len(m["A"]) != 1 || len(m["B"]) != 0 {
		t.Fatalf("drive links: %v", m)
	}
	db.RemoveSource(p.ID, "A", "Jobs/Nike")
	got, _ = db.Project(p.ID)
	if len(got.Sources) != 0 {
		t.Fatal("source not removed")
	}
	db.Forget("B")
	got, _ = db.Project(p.ID)
	if len(got.DestIDs) != 0 {
		t.Fatal("forgotten drive still a destination")
	}
	db.DeleteProject(p.ID)
	if ps, _ := db.Projects(); len(ps) != 0 {
		t.Fatal("project not deleted")
	}
}
