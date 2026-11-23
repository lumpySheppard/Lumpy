package catalog

import (
	"testing"
)

func TestPullPrefersOriginals(t *testing.T) {
	db := openTest(t)
	raid, shuttle := "RAID", "SHUTTLE"
	addDrive(t, db, raid, 100*gb, 50*gb, map[string]int64{
		"01_PROJECTS/TA/Online/A001/A001_C003.braw":   5000,
		"01_PROJECTS/TA/Offline/A001_C003_proxy.mp4":  50,
		"01_PROJECTS/TA/Online/R001.RDC/R001_001.R3D": 3000,
		"01_PROJECTS/TA/Online/R001.RDC/R001_002.R3D": 3000,
	}, nil)
	addDrive(t, db, shuttle, 100*gb, 50*gb, map[string]int64{"Day_02/A002_C001.mov": 7000}, nil)
	rep, err := db.Pull([]ArchiveRef{{Name: "A001_C003_proxy.mp4"}, {Name: "R001_001.R3D"}, {Name: "A002_C001"}, {Name: "ZZZ_C009.mov"}})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Found != 3 || len(rep.Missing) != 1 || rep.Missing[0] != "ZZZ_C009.mov" {
		t.Fatalf("found %d, missing %v", rep.Found, rep.Missing)
	}
	byName := map[string]PullRef{}
	for _, r := range rep.Refs {
		byName[r.Name] = r
	}
	a := byName["A001_C003_proxy.mp4"]
	if len(a.Hits) != 2 || a.Hits[0].Proxy || a.Hits[0].Path != "01_PROJECTS/TA/Online/A001/A001_C003.braw" || !a.Hits[1].Proxy {
		t.Fatalf("proxy ref should find the braw first: %+v", a.Hits)
	}
	r := byName["R001_001.R3D"]
	if len(r.Hits) != 1 || r.Hits[0].Path != "01_PROJECTS/TA/Online/R001.RDC" || r.Hits[0].Files != 2 || r.Hits[0].Bytes != 6000 {
		t.Fatalf("RED clip should be its folder with both chunks: %+v", r.Hits)
	}
	if byName["A002_C001"].Hits[0].DriveID != shuttle {
		t.Fatal("A002 is on the shuttle")
	}
	if len(rep.Drives) != 2 || rep.Drives[0].DriveID != raid || rep.Drives[0].Clips != 2 {
		t.Fatalf("drives: %+v", rep.Drives)
	}
	files, _ := db.PullFiles(raid, "01_PROJECTS/TA/Online/R001.RDC")
	if len(files) != 2 {
		t.Fatalf("RDC files: %d", len(files))
	}
}
