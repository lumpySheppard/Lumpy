package catalog

import "testing"

// A backup folder on another drive that mirrors the source's layout gets
// the copies at the same relative paths; excluded sub-folders and files
// found only in the backup folder are left alone.
func TestRoutingBackupsAndExclusions(t *testing.T) {
	db := openTest(t)
	addDrive(t, db, "A", 1<<40, 1<<39, map[string]int64{"09_OUTPUT/Final/x.mov": 10, "02_Scratch/tmp.mov": 7, "01_Original footage/a.mov": 5},
		map[string]string{"09_OUTPUT/Final/x.mov": "h1", "02_Scratch/tmp.mov": "h2", "01_Original footage/a.mov": "h4"})
	addDrive(t, db, "B", 1<<40, 1<<39, map[string]int64{"Backup/Proj/09_OUTPUT/Final/old.mov": 3, "Backup/Proj/01_Original footage/a.mov": 5},
		map[string]string{"Backup/Proj/09_OUTPUT/Final/old.mov": "h3", "Backup/Proj/01_Original footage/a.mov": "h4"})
	addDrive(t, db, "C", 1<<40, 1<<39, map[string]int64{}, nil)
	p, err := db.CreateProject("Proj", 2)
	if err != nil {
		t.Fatal(err)
	}
	db.AddSource(p.ID, "A", "")
	db.AddSource(p.ID, "B", "Backup/Proj")
	if err := db.SetSource(p.ID, Source{DriveID: "A", Exclude: []string{"02_Scratch"}}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetSource(p.ID, Source{DriveID: "B", Folder: "Backup/Proj", Role: "backup"}); err != nil {
		t.Fatal(err)
	}
	p, _ = db.Project(p.ID)
	if p.Sources[0].Role != "" || len(p.Sources[0].Exclude) != 1 || p.Sources[1].Role != "backup" {
		t.Fatalf("sources: %+v", p.Sources)
	}
	if bd := p.BackupDrives(); len(bd) != 1 || bd[0] != "B" {
		t.Fatalf("backup drives: %v", bd)
	}
	h, err := db.ProjectHealth(p.Sources, 2)
	if err != nil {
		t.Fatal(err)
	}
	// x.mov needs a copy; a.mov is on both; tmp.mov is excluded; old.mov is an orphan.
	if h.Contents != 2 || h.Sole != 1 || h.Backed != 1 || h.Sources[0].Excluded != 1 || h.Sources[1].Orphans != 1 {
		t.Fatalf("health: contents %d sole %d backed %d excluded %d orphans %d", h.Contents, h.Sole, h.Backed, h.Sources[0].Excluded, h.Sources[1].Orphans)
	}
	route := func(o Origin, dest string) (string, bool) {
		if r := p.RouteFor(p.Sources[o.Source], dest); r != nil {
			return r.Folder, true
		}
		return "", false
	}
	plan := MakePlanRouted(h, []DriveSpace{{ID: "B", Free: 1 << 39, Connected: true}, {ID: "C", Free: 1 << 39}}, ProjectLayout(p.Sources, func(id string) string { return id }), route)
	if len(plan.Transfers) != 1 {
		t.Fatalf("steps: %+v", plan.Transfers)
	}
	st := plan.Transfers[0]
	if st.DestID != "B" || st.Folder != "Backup/Proj" || len(st.Files) != 1 || st.Files[0].Dest != "09_OUTPUT/Final/x.mov" {
		t.Fatalf("step: dest %s folder %q files %+v", st.DestID, st.Folder, st.Files)
	}
	// No project folder on C: the copy would go under a run-time folder.
	plan = MakePlanRouted(h, []DriveSpace{{ID: "C", Free: 1 << 39, Connected: true}}, ProjectLayout(p.Sources, func(id string) string { return id }), route)
	if len(plan.Transfers) != 1 || plan.Transfers[0].Folder != "" {
		t.Fatalf("unrouted: %+v", plan.Transfers)
	}
	// An explicit route wins over inference.
	db.AddSource(p.ID, "B", "Other")
	if err := db.SetSource(p.ID, Source{DriveID: "A", Exclude: []string{"02_Scratch"}, Into: &Source{DriveID: "B", Folder: "Other"}}); err != nil {
		t.Fatal(err)
	}
	p, _ = db.Project(p.ID)
	if r := p.RouteFor(p.Sources[0], "B"); r == nil || r.Folder != "Other" {
		t.Fatalf("explicit route: %+v", r)
	}
}
