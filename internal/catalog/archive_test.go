package catalog

import (
	"reflect"
	"testing"
)

func TestArchive(t *testing.T) {
	db := openTest(t)
	addDrive(t, db, "RAID", 10*gb, 5*gb, map[string]int64{
		"Job/Day01/A001/A001_C001_0101XY.RDC/A001_C001_0101XY_001.R3D": 300,
		"Job/Day01/A001/A001_C001_0101XY.RDC/A001_C001_0101XY_002.R3D": 200,
		"Job/Day01/A001/A001_C002_0101XY.RDC/A001_C002_0101XY_001.R3D": 400,
		"Job/Day01/B001/C0001.MP4":                                     100,
		"Job/Day02/B001/C0001.MP4":                                     110, // same name as Day01's: kept with it
		"Job/Day02/B001/C0002.MP4":                                     120,
		"Job/Day02/S001/T01.WAV":                                       50,
		"Job/Day02/S001/T01.WAV.xml":                                   1, // a sidecar is not footage
		"Job/Notes/readme.txt":                                         1,
		"Elsewhere/clip9.mov":                                          900,
	}, nil)
	refs := []ArchiveRef{
		{Path: "/Volumes/RAID/Job/Day01/A001/A001_C001_0101XY.RDC/A001_C001_0101XY_001.R3D"}, // the whole RED clip
		{Name: "C0001"},         // no extension, as FCPXML names assets
		{Name: "T01_proxy.wav"}, // a proxy of a clip
		{Name: "clip9.mov"},     // outside the project
		{Name: "Music bed"},     // not footage at all
		{Path: "/x/C0001.MP4"},  // the same clip again, by path
	}
	rep, err := db.Archive([]Source{{DriveID: "RAID", Folder: "Job"}}, refs)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Refs != 5 || rep.Found != 3 || !reflect.DeepEqual(rep.NotFound, []string{"Music bed", "clip9.mov"}) {
		t.Errorf("refs: %d found %d notfound %v", rep.Refs, rep.Found, rep.NotFound)
	}
	if rep.UsedFiles != 4 || rep.UsedBytes != 500+100+110+50 || rep.UnusedFiles != 2 || rep.UnusedBytes != 400+120 {
		t.Errorf("totals: used %d/%d unused %d/%d", rep.UsedFiles, rep.UsedBytes, rep.UnusedFiles, rep.UnusedBytes)
	}
	var used, unused []string
	for _, it := range rep.Items {
		if it.Used {
			used = append(used, it.Path)
		} else {
			unused = append(unused, it.Path)
		}
		if it.Name == "C0001.MP4" && !it.Shared {
			t.Errorf("%s should be marked shared", it.Path)
		}
	}
	if !reflect.DeepEqual(unused, []string{"Job/Day01/A001/A001_C002_0101XY.RDC", "Job/Day02/B001/C0002.MP4"}) {
		t.Errorf("unused: %v", unused)
	}
	if len(used) != 4 || used[0] != "Job/Day01/A001/A001_C001_0101XY.RDC" {
		t.Errorf("used: %v", used)
	}
	if len(rep.Folders) != 2 || rep.Folders[0].Folder != "Day01" || rep.Folders[0].UnusedBytes != 400 || rep.Folders[1].Folder != "Day02" || rep.Folders[1].Files != 3 || rep.Folders[1].UnusedFiles != 1 {
		t.Errorf("folders: %+v", rep.Folders)
	}

	// Nested sources: the inner one owns its files, once.
	rep, err = db.Archive([]Source{{DriveID: "RAID", Folder: ""}, {DriveID: "RAID", Folder: "Job/Day02"}}, refs)
	if err != nil {
		t.Fatal(err)
	}
	if rep.UsedFiles+rep.UnusedFiles != 7 || rep.Found != 4 {
		t.Errorf("nested: %d items, found %d", rep.UsedFiles+rep.UnusedFiles, rep.Found)
	}
	for _, it := range rep.Items {
		if it.Path == "Job/Day02/B001/C0002.MP4" && it.Source != 1 {
			t.Errorf("owner of %s: source %d", it.Path, it.Source)
		}
	}

	// Moving and removing follow folders and keep LIKE wildcards literal.
	if err := db.MovePaths("RAID", map[string]string{"Job/Day01/A001/A001_C002_0101XY.RDC": "Job/_Unused/Day01/A001/A001_C002_0101XY.RDC", "Job/Day02/B001/C0002.MP4": "Job/_Unused/Day02/B001/C0002.MP4"}); err != nil {
		t.Fatal(err)
	}
	files, _ := db.Files("RAID")
	paths := map[string]bool{}
	for _, f := range files {
		paths[f.Path] = true
	}
	if !paths["Job/_Unused/Day01/A001/A001_C002_0101XY.RDC/A001_C002_0101XY_001.R3D"] || !paths["Job/_Unused/Day02/B001/C0002.MP4"] || paths["Job/Day02/B001/C0002.MP4"] || !paths["Job/Day02/B001/C0001.MP4"] {
		t.Errorf("after move: %v", paths)
	}
	if err := db.RemovePaths("RAID", []string{"Job/_Unused", "Job/Day02/S001/T01.WAV"}); err != nil {
		t.Fatal(err)
	}
	files, _ = db.Files("RAID")
	if len(files) != 7 {
		t.Errorf("after remove: %d files", len(files))
		for _, f := range files {
			t.Log(f.Path)
		}
	}
}
