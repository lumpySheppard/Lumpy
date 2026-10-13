package media

import (
	"bytes"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// box builds a QuickTime atom.
func box(typ string, parts ...[]byte) []byte {
	var body bytes.Buffer
	for _, p := range parts {
		body.Write(p)
	}
	var b bytes.Buffer
	binary.Write(&b, binary.BigEndian, uint32(8+body.Len()))
	b.WriteString(typ)
	b.Write(body.Bytes())
	return b.Bytes()
}

func be32(v uint32) []byte { b := make([]byte, 4); binary.BigEndian.PutUint32(b, v); return b }
func be16(v uint16) []byte { b := make([]byte, 2); binary.BigEndian.PutUint16(b, v); return b }

// fullBox is an atom with a version/flags word.
func fullBox(typ string, parts ...[]byte) []byte {
	return box(typ, append([][]byte{{0, 0, 0, 0}}, parts...)...)
}

// sampleMOV builds a small ProRes clip description: 48 frames at 23.976,
// stereo 48 kHz 24-bit audio, a timecode track starting at 01:02:03:04,
// and camera make/model metadata.
func sampleMOV(t *testing.T) string {
	t.Helper()
	const tcFrames = 1*24*3600 + 2*24*60 + 3*24 + 4
	tcSample := be32(tcFrames)
	// mdat holds only the timecode sample; its offset is known once the
	// header size is known, so build moov with a placeholder first.
	build := func(tcOffset uint32) []byte {
		mvhd := fullBox("mvhd", be32(0), be32(0), be32(24000), be32(48*1001), bytes.Repeat([]byte{0}, 80))
		video := box("trak",
			fullBox("tkhd", bytes.Repeat([]byte{0}, 80)),
			box("mdia",
				fullBox("mdhd", be32(0), be32(0), be32(24000), be32(48*1001), be16(0), be16(0)),
				fullBox("hdlr", []byte("mhlr"), []byte("vide"), bytes.Repeat([]byte{0}, 13)),
				box("minf", box("stbl",
					fullBox("stsd", be32(1), box("apch", bytes.Repeat([]byte{0}, 6), be16(1),
						be16(0), be16(0), []byte("appl"), be32(0), be32(0), be16(3840), be16(2160), bytes.Repeat([]byte{0}, 50))),
					fullBox("stts", be32(1), be32(48), be32(1001)),
					fullBox("stco", be32(0)),
				)),
			),
		)
		audio := box("trak",
			fullBox("tkhd", bytes.Repeat([]byte{0}, 80)),
			box("mdia",
				fullBox("mdhd", be32(0), be32(0), be32(48000), be32(96000), be16(0), be16(0)),
				fullBox("hdlr", []byte("mhlr"), []byte("soun"), bytes.Repeat([]byte{0}, 13)),
				box("minf", box("stbl",
					fullBox("stsd", be32(1), box("in24", bytes.Repeat([]byte{0}, 6), be16(1),
						be16(0), be16(0), []byte("appl"), be16(2), be16(24), be16(0), be16(0), be32(48000<<16))),
					fullBox("stts", be32(1), be32(96000), be32(1)),
					fullBox("stco", be32(0)),
				)),
			),
		)
		name := box("name", be16(4), be16(0), []byte("A001"))
		tmcd := box("trak",
			fullBox("tkhd", bytes.Repeat([]byte{0}, 80)),
			box("mdia",
				fullBox("mdhd", be32(0), be32(0), be32(24000), be32(48*1001), be16(0), be16(0)),
				fullBox("hdlr", []byte("mhlr"), []byte("tmcd"), bytes.Repeat([]byte{0}, 13)),
				box("minf", box("stbl",
					fullBox("stsd", be32(1), box("tmcd", bytes.Repeat([]byte{0}, 6), be16(1),
						be32(0), be32(0), be32(24000), be32(1001), []byte{24, 0}, name)),
					fullBox("stts", be32(1), be32(1), be32(48*1001)),
					fullBox("stco", be32(1), be32(tcOffset)),
				)),
			),
		)
		keys := fullBox("keys", be32(2),
			box("mdta", []byte("com.apple.quicktime.make")),
			box("mdta", []byte("com.apple.quicktime.model")))
		item := func(idx uint32, v string) []byte {
			return box(string(be32(idx)), box("data", be32(1), be32(0), []byte(v)))
		}
		meta := box("meta", fullBox("hdlr", []byte{0, 0, 0, 0}, []byte("mdta"), bytes.Repeat([]byte{0}, 13)), keys,
			box("ilst", item(1, "ARRI"), item(2, "ALEXA Mini LF")))
		moov := box("moov", mvhd, video, audio, tmcd, meta)
		ftyp := box("ftyp", []byte("qt  "), be32(0), []byte("qt  "))
		mdat := box("mdat", tcSample)
		return append(append(ftyp, moov...), mdat...)
	}
	first := build(0)
	tcOffset := uint32(len(first) - len(tcSample))
	data := build(tcOffset)
	p := filepath.Join(t.TempDir(), "A001C001.mov")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestQuickTime(t *testing.T) {
	info, err := Probe(sampleMOV(t))
	if err != nil {
		t.Fatal(err)
	}
	want := Info{Kind: Video, Container: "QuickTime", VideoCodec: "ProRes 422 HQ", Width: 3840, Height: 2160, FrameRate: 23.976,
		DurationMS: 2002, Timecode: "01:02:03:04", AudioCodec: "PCM 24-bit", Channels: 2, SampleRate: 48000, BitDepth: 24,
		Camera: "ARRI ALEXA Mini LF", Reel: "A001", Source: "quicktime"}
	got := *info
	got.Extra = nil
	if got.Marshal() != want.Marshal() {
		t.Fatalf("got  %+v\nwant %+v", got, want)
	}
}

func sampleWAV(t *testing.T) string {
	t.Helper()
	var b bytes.Buffer
	le32 := func(v uint32) []byte { x := make([]byte, 4); binary.LittleEndian.PutUint32(x, v); return x }
	le16 := func(v uint16) []byte { x := make([]byte, 2); binary.LittleEndian.PutUint16(x, v); return x }
	chunk := func(id string, body []byte) []byte {
		out := append([]byte(id), le32(uint32(len(body)))...)
		out = append(out, body...)
		if len(body)%2 == 1 {
			out = append(out, 0)
		}
		return out
	}
	fmtc := chunk("fmt ", bytes.Join([][]byte{le16(1), le16(2), le32(48000), le32(48000 * 6), le16(6), le16(24)}, nil))
	bext := make([]byte, 602)
	copy(bext[0:], "Sc 12 Tk 3")
	copy(bext[256:], "Sound Devices")
	copy(bext[320:], "2026-10-13")
	copy(bext[330:], "09:30:00")
	ref := uint64(48000) * (1*3600 + 2*60 + 3) // 01:02:03:00
	binary.LittleEndian.PutUint64(bext[338:], ref)
	ixml := chunk("iXML", []byte("<BWFXML><SCENE>12</SCENE><TAKE>3</TAKE><TAPE>S001</TAPE><SPEED><TIMECODE_RATE>24000/1001</TIMECODE_RATE></SPEED></BWFXML>"))
	data := chunk("data", make([]byte, 48000*6*2)) // 2 seconds
	body := bytes.Join([][]byte{[]byte("WAVE"), fmtc, chunk("bext", bext), ixml, data}, nil)
	b.WriteString("RIFF")
	b.Write(le32(uint32(len(body))))
	b.Write(body)
	p := filepath.Join(t.TempDir(), "S001T003.wav")
	if err := os.WriteFile(p, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestWAV(t *testing.T) {
	info, err := Probe(sampleWAV(t))
	if err != nil {
		t.Fatal(err)
	}
	if info.Kind != Audio || info.Container != "BWF" || info.AudioCodec != "PCM" || info.Channels != 2 || info.SampleRate != 48000 ||
		info.BitDepth != 24 || info.DurationMS != 2000 || info.Timecode != "01:02:03:00" || info.Scene != "12" || info.Take != "3" ||
		info.Reel != "S001" || info.FrameRate < 23.9 || info.FrameRate > 24 {
		t.Fatalf("got %+v", info)
	}
}

func TestTimecode(t *testing.T) {
	for _, c := range []struct {
		frames int64
		rate   float64
		drop   bool
		want   string
	}{
		{0, 24, false, "00:00:00:00"},
		{24*3600 + 1, 24, false, "01:00:00:01"},
		{17982, 29.97, true, "00:10:00;00"}, // 10 minutes drop-frame
		{1798, 29.97, true, "00:00:59;28"},  // the last frames of the first minute
		{1800, 29.97, true, "00:01:00;02"},  // the next minute skips ;00 and ;01
		{3597, 29.97, false, "00:01:59:27"},
	} {
		if got := Timecode(c.frames, c.rate, c.drop); got != c.want {
			t.Errorf("Timecode(%d, %v, %v) = %s, want %s", c.frames, c.rate, c.drop, got, c.want)
		}
	}
}

// With ffmpeg available, a real ProRes clip is read the same way.
func TestRealClipWithFFmpeg(t *testing.T) {
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	p := filepath.Join(t.TempDir(), "real.mov")
	cmd := exec.Command(ff, "-v", "quiet", "-f", "lavfi", "-i", "testsrc=size=1920x1080:rate=25:duration=1", "-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-c:v", "prores_ks", "-profile:v", "3", "-c:a", "pcm_s24le", "-timecode", "10:00:00:00", "-metadata", "com.apple.quicktime.make=Test", p)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("ffmpeg failed: %v %s", err, out)
	}
	info, err := Probe(p)
	if err != nil {
		t.Fatal(err)
	}
	if info.VideoCodec != "ProRes 422 HQ" || info.Width != 1920 || info.Height != 1080 || info.FrameRate != 25 || info.Timecode != "10:00:00:00" ||
		info.AudioCodec == "" || info.Channels != 1 || info.SampleRate != 44100 || info.DurationMS < 900 || info.DurationMS > 1100 {
		t.Fatalf("got %+v", info)
	}
}

// Stills get their size and camera from the headers too.
func TestImages(t *testing.T) {
	dir := t.TempDir()
	// A PNG header is enough: IHDR says 640×360, 8-bit.
	png := append([]byte("\x89PNG\r\n\x1a\n"), be32(13)...)
	png = append(png, []byte("IHDR")...)
	png = append(png, be32(640)...)
	png = append(png, be32(360)...)
	png = append(png, 8, 2, 0, 0, 0)
	os.WriteFile(filepath.Join(dir, "a.png"), png, 0o644)
	info, _ := Probe(filepath.Join(dir, "a.png"))
	if info.Kind != Image || info.Container != "PNG" || info.Width != 640 || info.Height != 360 {
		t.Fatalf("png: %+v", info)
	}

	// A JPEG with an EXIF APP1 segment (camera make and model) and a
	// baseline SOF0 frame header of 1920×1080.
	var exif bytes.Buffer
	exif.WriteString("Exif\x00\x00")
	tiff := &bytes.Buffer{}
	tiff.WriteString("MM\x00\x2a")
	tiff.Write(be32(8))
	tiff.Write(be16(2)) // two entries
	// Make at offset after the IFD: 8 + 2 + 2*12 + 4 = 38
	tiff.Write(be16(0x10F))
	tiff.Write(be16(2))
	tiff.Write(be32(6))
	tiff.Write(be32(38))
	tiff.Write(be16(0x110))
	tiff.Write(be16(2))
	tiff.Write(be32(7))
	tiff.Write(be32(44))
	tiff.Write(be32(0)) // next IFD
	tiff.WriteString("Canon\x00")
	tiff.WriteString("EOS R5\x00")
	exif.Write(tiff.Bytes())
	var jpg bytes.Buffer
	jpg.Write([]byte{0xFF, 0xD8})
	jpg.Write([]byte{0xFF, 0xE1})
	jpg.Write(be16(uint16(exif.Len() + 2)))
	jpg.Write(exif.Bytes())
	jpg.Write([]byte{0xFF, 0xC0})
	jpg.Write(be16(11))
	jpg.Write([]byte{8})
	jpg.Write(be16(1080))
	jpg.Write(be16(1920))
	jpg.Write([]byte{3, 1, 0x22, 0, 2, 0x11, 1, 3, 0x11, 1})
	jpg.Write([]byte{0xFF, 0xDA, 0, 2})
	os.WriteFile(filepath.Join(dir, "b.jpg"), jpg.Bytes(), 0o644)
	info, _ = Probe(filepath.Join(dir, "b.jpg"))
	if info.Kind != Image || info.Container != "JPEG" || info.Width != 1920 || info.Height != 1080 || info.Camera != "Canon EOS R5" {
		t.Fatalf("jpeg: %+v", info)
	}

	// A DNG is a TIFF: little-endian here, with the size in the first IFD.
	var dng bytes.Buffer
	le := binary.LittleEndian
	w := func(v any) { binary.Write(&dng, le, v) }
	dng.WriteString("II*\x00")
	w(uint32(8))
	w(uint16(2))
	w(uint16(0x100))
	w(uint16(4))
	w(uint32(1))
	w(uint32(6000))
	w(uint16(0x101))
	w(uint16(3))
	w(uint32(1))
	w(uint16(4000))
	w(uint16(0))
	w(uint32(0))
	os.WriteFile(filepath.Join(dir, "c.dng"), dng.Bytes(), 0o644)
	info, _ = Probe(filepath.Join(dir, "c.dng"))
	if info.Kind != Image || info.Container != "DNG" || info.Width != 6000 || info.Height != 4000 {
		t.Fatalf("dng: %+v", info)
	}
}
