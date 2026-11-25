package edit

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	fcp := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE fcpxml>
<fcpxml version="1.9"><resources>
  <format id="r1" name="FFVideoFormat1080p2398"/>
  <asset id="r2" name="A004C001_251013_R1ZL" start="0s" duration="1s" hasVideo="1"><media-rep kind="original-media" src="file:///Volumes/RAID/CTD_S01/Day04/A004/A004C001_251013_R1ZL.mov"/><media-rep kind="proxy-media" src="file:///Users/me/Proxies/A004C001_251013_R1ZL.mov"/></asset>
  <asset id="r3" name="C0001" src="file:///Volumes/RAID/Day%2004/C0001.MP4"/>
  <asset id="r4" name="Music bed"/>
  <asset id="r5" name="A004C001_251013_R1ZL" src="file:///Volumes/RAID/CTD_S01/Day04/A004/A004C001_251013_R1ZL.mov"/>
</resources></fcpxml>`
	refs, kind, err := Parse("cut.fcpxml", []byte(fcp))
	if err != nil || kind != "FCPXML" {
		t.Fatalf("fcpxml: %v %q", err, kind)
	}
	want := []Ref{
		{Name: "A004C001_251013_R1ZL.mov", Path: "/Volumes/RAID/CTD_S01/Day04/A004/A004C001_251013_R1ZL.mov"},
		{Name: "C0001.MP4", Path: "/Volumes/RAID/Day 04/C0001.MP4"},
		{Name: "Music bed"},
	}
	if !reflect.DeepEqual(refs, want) {
		t.Errorf("fcpxml refs:\n got %+v\nwant %+v", refs, want)
	}

	xmeml := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE xmeml>
<xmeml version="5"><sequence><name>Cut v3</name><media><video><track>
  <clipitem id="c1"><name>clip1</name><file id="f1"><name>clip1.mov</name><pathurl>file://localhost/Volumes/RAID/Day06/A002/clip1.mov</pathurl><timecode><reel><name>A002</name></reel></timecode></file></clipitem>
  <clipitem id="c2"><name>clip1 again</name><file id="f1"/></clipitem>
  <clipitem id="c3"><name>title</name><file id="f2"><name>Title Card</name></file></clipitem>
</track></video></media></sequence></xmeml>`
	refs, kind, err = Parse("cut.xml", []byte(xmeml))
	if err != nil || kind != "Premiere / FCP 7 XML" {
		t.Fatalf("xmeml: %v %q", err, kind)
	}
	want = []Ref{{Name: "clip1.mov", Path: "/Volumes/RAID/Day06/A002/clip1.mov"}, {Name: "Title Card"}}
	if !reflect.DeepEqual(refs, want) {
		t.Errorf("xmeml refs:\n got %+v\nwant %+v", refs, want)
	}

	edl := "TITLE: Cut v3\nFCM: NON-DROP FRAME\n\n001  A001C001 V     C        01:00:00:00 01:00:05:00 00:00:00:00 00:00:05:00\n* FROM CLIP NAME: A001_C001_0101XY.R3D\n002  AX       AA/V  C        01:00:00:00 01:00:05:00 00:00:05:00 00:00:10:00\n* FROM CLIP NAME:  B001_C001.mov\n* SOURCE FILE: /Volumes/RAID/B001/B001_C001.mov\n003  A001C001 V     D    024 01:00:00:00 01:00:05:00 00:00:10:00 00:00:15:00\n* FROM CLIP NAME: A001_C001_0101XY.R3D\n* TO CLIP NAME: A001_C003_0101XY.R3D\n"
	refs, kind, err = Parse("cut.edl", []byte(edl))
	if err != nil || kind != "EDL" {
		t.Fatalf("edl: %v %q", err, kind)
	}
	want = []Ref{{Name: "A001_C001_0101XY.R3D"}, {Name: "B001_C001.mov"}, {Name: "A001_C003_0101XY.R3D"}, {Name: "B001_C001.mov", Path: "/Volumes/RAID/B001/B001_C001.mov"}}
	if !reflect.DeepEqual(refs, want) {
		t.Errorf("edl refs:\n got %+v\nwant %+v", refs, want)
	}
	// An EDL with no comments falls back to reel names.
	refs, _, _ = Parse("bare.edl", []byte("TITLE: x\n001  TAPE01   V     C        01:00:00:00 01:00:05:00 00:00:00:00 00:00:05:00\n002  AX       V     C        01:00:00:00 01:00:05:00 00:00:00:00 00:00:05:00\n"))
	if !reflect.DeepEqual(refs, []Ref{{Name: "TAPE01"}}) {
		t.Errorf("bare edl: %+v", refs)
	}

	list := "File Name,Scene\nA004C002_251013_R1ZL.mov,12\nnotes about nothing\n\"C0002.MP4\"; C0003.MP4\n/Volumes/RAID/Day06/clip2.mov\nC0002.MP4\n"
	refs, kind, err = Parse("list.csv", []byte(list))
	if err != nil || kind != "clip list" {
		t.Fatalf("list: %v %q", err, kind)
	}
	want = []Ref{{Name: "A004C002_251013_R1ZL.mov"}, {Name: "C0002.MP4"}, {Name: "C0003.MP4"}, {Name: "clip2.mov", Path: "/Volumes/RAID/Day06/clip2.mov"}}
	if !reflect.DeepEqual(refs, want) {
		t.Errorf("list refs:\n got %+v\nwant %+v", refs, want)
	}

	if _, _, err := Parse("x.xml", []byte("<html><body>no</body></html>")); err == nil {
		t.Error("html accepted")
	}
	if _, _, err := Parse("x.aaf", []byte("\x00\x01binary")); err == nil {
		t.Error("binary accepted")
	}
}
