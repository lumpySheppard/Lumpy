package catalog

import (
	"testing"
)

func TestProxies(t *testing.T) {
	files := map[string]int64{
		// Originals: a RED clip in chunks, a Sony MP4, an ARRI MXF, a still (ignored).
		"03_JOB/01_Original footage/Online/A001/A001_C001_0101XY.RDC/A001_C001_0101XY_001.R3D": 100,
		"03_JOB/01_Original footage/Online/A001/A001_C001_0101XY.RDC/A001_C001_0101XY_002.R3D": 50,
		"03_JOB/01_Original footage/Online/A001/A001_C002_0101XY.RDC/A001_C002_0101XY_001.R3D": 70,
		"03_JOB/01_Original footage/Online/B/C0001.MP4":                                        30,
		"03_JOB/01_Original footage/Online/B/C0002.MP4":                                        30,
		"03_JOB/01_Original footage/Online/C/A004C001_251013_R1ZL.mxf":                         90,
		"03_JOB/01_Original footage/Online/C/REF.jpg":                                          1,
		// Proxies: same stems, various suffixes, nested deeper.
		"03_JOB/01_Original footage/Offline/A001_C001_0101XY.mov":           10,
		"03_JOB/01_Original footage/Offline/Sony/C0001S03.MP4":              3,
		"03_JOB/01_Original footage/Offline/A004C001_251013_R1ZL_proxy.mov": 9,
		"03_JOB/01_Original footage/Offline/stray.mov":                      9,
		// Unrelated folders with the same names elsewhere are their own pair.
		"OTHER/00_ONLINE/X.mov":  5,
		"OTHER/01_OFFLINE/X.mov": 1,
		"OTHER/notes.txt":        1,
	}
	db := openTest(t)
	addDrive(t, db, "D1", 1e12, 5e11, files, nil)
	pairs, err := db.ProxyPairs("D1", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 2 || pairs[0].Online != "03_JOB/01_Original footage/Online" || pairs[0].Offline != "03_JOB/01_Original footage/Offline" ||
		pairs[1].Online != "OTHER/00_ONLINE" || pairs[1].Offline != "OTHER/01_OFFLINE" {
		t.Fatalf("pairs: %+v", pairs)
	}
	if only, _ := db.ProxyPairs("D1", "03_JOB"); len(only) != 1 {
		t.Fatalf("pairs under 03_JOB: %+v", only)
	}
	rep, err := db.Proxies("D1", pairs[0])
	if err != nil {
		t.Fatal(err)
	}
	if rep.Total != 5 || rep.Proxied != 3 || rep.MissingBytes != 100 || rep.Extra != 1 {
		t.Fatalf("report: total %d proxied %d missing %d extra %d\n%+v", rep.Total, rep.Proxied, rep.MissingBytes, rep.Extra, rep.Clips)
	}
	byName := map[string]ProxyClip{}
	for _, c := range rep.Clips {
		byName[c.Name] = c
	}
	if c := byName["A001_C001_0101XY"]; c.Files != 2 || c.Bytes != 150 || c.Proxy != "03_JOB/01_Original footage/Offline/A001_C001_0101XY.mov" {
		t.Fatalf("red clip: %+v", c)
	}
	if c := byName["A001_C002_0101XY"]; c.Proxy != "" || c.Bytes != 70 {
		t.Fatalf("red clip without proxy: %+v", c)
	}
	if c := byName["C0001.MP4"]; c.Proxy != "03_JOB/01_Original footage/Offline/Sony/C0001S03.MP4" {
		t.Fatalf("sony: %+v", c)
	}
	if c := byName["C0002.MP4"]; c.Proxy != "" {
		t.Fatalf("sony without proxy: %+v", c)
	}
	if c := byName["A004C001_251013_R1ZL.mxf"]; c.Proxy == "" {
		t.Fatalf("suffix _proxy not matched: %+v", c)
	}
	states, err := db.ProxyStates("D1")
	if err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]string{
		"03_JOB/01_Original footage/Online/A001/A001_C001_0101XY.RDC":                          "proxied",
		"03_JOB/01_Original footage/Online/A001/A001_C001_0101XY.RDC/A001_C001_0101XY_002.R3D": "proxied",
		"03_JOB/01_Original footage/Online/A001/A001_C002_0101XY.RDC/A001_C002_0101XY_001.R3D": "missing",
		"03_JOB/01_Original footage/Online/B/C0002.MP4":                                        "missing",
		"03_JOB/01_Original footage/Offline/A001_C001_0101XY.mov":                              "proxy",
		"03_JOB/01_Original footage/Online/C/REF.jpg":                                          "",
		"OTHER/00_ONLINE/X.mov": "proxied",
	} {
		if states[p] != want {
			t.Errorf("%s: %q, want %q", p, states[p], want)
		}
	}
}
