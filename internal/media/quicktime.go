package media

import (
	"encoding/binary"
	"io"
	"math"
	"os"
	"strings"
	"time"
)

// Video codec names by QuickTime/MP4 sample description format.
var videoCodecs = map[string]string{
	"apch": "ProRes 422 HQ", "apcn": "ProRes 422", "apcs": "ProRes 422 LT", "apco": "ProRes 422 Proxy",
	"ap4h": "ProRes 4444", "ap4x": "ProRes 4444 XQ", "aprh": "ProRes RAW HQ", "aprn": "ProRes RAW",
	"avc1": "H.264", "avc3": "H.264", "hvc1": "HEVC", "hev1": "HEVC", "dvh1": "Dolby Vision", "dvhe": "Dolby Vision",
	"mp4v": "MPEG-4 Visual", "jpeg": "Motion JPEG", "mjpa": "Motion JPEG A", "mjpb": "Motion JPEG B",
	"AVdn": "DNxHD", "AVdh": "DNxHR", "CFHD": "CineForm", "v210": "Uncompressed 10-bit 4:2:2", "v410": "Uncompressed 10-bit 4:4:4",
	"2vuy": "Uncompressed 8-bit 4:2:2", "yuv2": "Uncompressed 8-bit 4:2:2", "raw ": "Uncompressed RGB",
	"xd54": "XDCAM HD 422", "xd55": "XDCAM HD 422", "xd59": "XDCAM HD 422", "xd5a": "XDCAM HD 422", "xd5b": "XDCAM HD 422",
	"xd5c": "XDCAM HD 422", "xd5d": "XDCAM HD 422", "xd5e": "XDCAM HD 422", "xd5f": "XDCAM HD 422",
	"xdvb": "XDCAM EX", "xdvc": "XDCAM EX", "xdvd": "XDCAM EX", "xdve": "XDCAM EX", "xdvf": "XDCAM EX",
	"dvc ": "DV", "dvcp": "DV PAL", "dvpp": "DVCPRO", "dv5n": "DVCPRO50", "dv5p": "DVCPRO50", "dvh5": "DVCPRO HD", "dvh6": "DVCPRO HD",
	"ai5p": "AVC-Intra 50", "ai5q": "AVC-Intra 50", "ai52": "AVC-Intra 50", "ai53": "AVC-Intra 50", "ai55": "AVC-Intra 50", "ai56": "AVC-Intra 50",
	"ai1p": "AVC-Intra 100", "ai1q": "AVC-Intra 100", "ai12": "AVC-Intra 100", "ai13": "AVC-Intra 100", "ai15": "AVC-Intra 100", "ai16": "AVC-Intra 100",
	"png ": "PNG", "gif ": "GIF", "rle ": "Animation", "SVQ3": "Sorenson 3", "av01": "AV1", "vp09": "VP9",
}

var audioCodecs = map[string]string{
	"lpcm": "PCM", "sowt": "PCM", "twos": "PCM", "in24": "PCM 24-bit", "in32": "PCM 32-bit", "fl32": "PCM float", "fl64": "PCM float 64",
	"NONE": "PCM", "raw ": "PCM 8-bit", "mp4a": "AAC", "ac-3": "AC-3", "ec-3": "E-AC-3", "alac": "Apple Lossless", "samr": "AMR",
	"Opus": "Opus", ".mp3": "MP3", "ms\x00U": "MP3",
}

type atom struct {
	typ        string
	start, end int64 // of the whole atom
	body       int64 // where the data starts
}

// atoms lists the atoms between from and to.
func atoms(r io.ReaderAt, from, to int64) ([]atom, error) {
	var out []atom
	var hdr [16]byte
	for pos := from; pos+8 <= to; {
		if _, err := r.ReadAt(hdr[:8], pos); err != nil {
			return out, err
		}
		size := int64(binary.BigEndian.Uint32(hdr[:4]))
		typ := string(hdr[4:8])
		body := pos + 8
		switch size {
		case 0:
			size = to - pos
		case 1:
			if _, err := r.ReadAt(hdr[8:16], pos+8); err != nil {
				return out, err
			}
			size = int64(binary.BigEndian.Uint64(hdr[8:16]))
			body = pos + 16
		}
		if size < 8 || pos+size > to {
			return out, nil // damaged or truncated: keep what we have
		}
		out = append(out, atom{typ, pos, pos + size, body})
		pos += size
	}
	return out, nil
}

func find(as []atom, typ string) (atom, bool) {
	for _, a := range as {
		if a.typ == typ {
			return a, true
		}
	}
	return atom{}, false
}

func readAll(r io.ReaderAt, a atom, max int64) []byte {
	n := a.end - a.body
	if n > max {
		n = max
	}
	if n <= 0 {
		return nil
	}
	buf := make([]byte, n)
	m, _ := r.ReadAt(buf, a.body)
	return buf[:m]
}

// probeQuickTime reads the moov atom of a QuickTime or MP4 file. Only
// the headers are read; a clip's media data is skipped by its size.
func probeQuickTime(f *os.File, info *Info) error {
	st, err := f.Stat()
	if err != nil {
		return err
	}
	top, err := atoms(f, 0, st.Size())
	if err != nil && len(top) == 0 {
		return err
	}
	if ft, ok := find(top, "ftyp"); ok {
		brand := strings.TrimRight(string(readAll(f, ft, 4)), "\x00 ")
		switch {
		case brand == "qt":
			info.Container = "QuickTime"
		case strings.HasPrefix(brand, "mp4"), brand == "isom", brand == "iso2", brand == "avc1", brand == "iso5", brand == "iso6":
			info.Container = "MP4"
		case strings.HasPrefix(brand, "M4V"):
			info.Container = "MP4"
		case strings.HasPrefix(brand, "M4A"):
			info.Container = "MP4"
			info.Kind = Audio
		default:
			info.Container = "MP4 (" + brand + ")"
		}
	} else {
		info.Container = "QuickTime"
	}
	moov, ok := find(top, "moov")
	if !ok {
		return errUnsupported
	}
	info.Source = "quicktime"
	kids, _ := atoms(f, moov.body, moov.end)
	var movieScale uint32
	if mv, ok := find(kids, "mvhd"); ok {
		b := readAll(f, mv, 32)
		if len(b) >= 20 {
			if b[0] == 1 && len(b) >= 32 {
				movieScale = binary.BigEndian.Uint32(b[20:24])
				dur := binary.BigEndian.Uint64(b[24:32])
				if movieScale > 0 {
					info.DurationMS = int64(dur) * 1000 / int64(movieScale)
				}
				info.Created = qtTime(binary.BigEndian.Uint64(b[4:12]))
			} else {
				movieScale = binary.BigEndian.Uint32(b[12:16])
				dur := binary.BigEndian.Uint32(b[16:20])
				if movieScale > 0 {
					info.DurationMS = int64(dur) * 1000 / int64(movieScale)
				}
				info.Created = qtTime(uint64(binary.BigEndian.Uint32(b[4:8])))
			}
		}
	}
	var tmcd *track
	for _, a := range kids {
		if a.typ != "trak" {
			continue
		}
		t := readTrack(f, a)
		switch t.handler {
		case "vide":
			if info.VideoCodec == "" {
				info.VideoCodec = codecName(videoCodecs, t.format)
				info.Width, info.Height = t.width, t.height
				if t.frameRate > 0 {
					info.FrameRate = t.frameRate
				}
				if info.DurationMS == 0 && t.durationMS > 0 {
					info.DurationMS = t.durationMS
				}
			}
		case "soun":
			if info.AudioCodec == "" {
				info.AudioCodec = codecName(audioCodecs, t.format)
				info.Channels, info.SampleRate, info.BitDepth = t.channels, t.sampleRate, t.bitDepth
			}
		case "tmcd":
			if tmcd == nil {
				tt := t
				tmcd = &tt
			}
		}
	}
	if tmcd != nil && tmcd.tcScale > 0 && tmcd.tcFrameDur > 0 && tmcd.firstChunk > 0 {
		var b [4]byte
		if _, err := f.ReadAt(b[:], tmcd.firstChunk); err == nil {
			frames := int64(int32(binary.BigEndian.Uint32(b[:])))
			rate := float64(tmcd.tcScale) / float64(tmcd.tcFrameDur)
			if frames >= 0 {
				info.Timecode = Timecode(frames, rate, tmcd.tcDrop)
			}
			if info.FrameRate == 0 {
				info.FrameRate = rate
			}
		}
		if tmcd.reel != "" {
			info.Reel = tmcd.reel
		}
	}
	if info.VideoCodec == "" && info.AudioCodec != "" {
		info.Kind = Audio
	}
	if m, ok := find(kids, "meta"); ok {
		readMeta(f, m, info)
	}
	if u, ok := find(kids, "udta"); ok {
		ukids, _ := atoms(f, u.body, u.end)
		if m, ok := find(ukids, "meta"); ok {
			readMeta(f, m, info)
		}
		for _, k := range ukids {
			if k.typ == "\xa9mak" || k.typ == "\xa9mod" || k.typ == "\xa9day" {
				b := readAll(f, k, 256)
				if len(b) > 4 {
					v := strings.TrimRight(string(b[4:]), "\x00")
					addExtra(info, map[string]string{"\xa9mak": "make", "\xa9mod": "model", "\xa9day": "date"}[k.typ], v)
				}
			}
		}
	}
	if info.Camera == "" {
		mk, md := info.Extra["make"], info.Extra["model"]
		if mk != "" || md != "" {
			info.Camera = camera(mk, md)
		}
	}
	// Make, model and timecode have fields of their own; keep Extra for
	// the rest.
	delete(info.Extra, "make")
	delete(info.Extra, "model")
	if info.Timecode != "" {
		delete(info.Extra, "timecode")
	}
	if len(info.Extra) == 0 {
		info.Extra = nil
	}
	return nil
}

type track struct {
	handler    string
	format     string
	width      int
	height     int
	frameRate  float64
	durationMS int64
	channels   int
	sampleRate int
	bitDepth   int
	tcScale    uint32
	tcFrameDur uint32
	tcDrop     bool
	firstChunk int64
	reel       string
}

func readTrack(r io.ReaderAt, trak atom) track {
	var t track
	kids, _ := atoms(r, trak.body, trak.end)
	mdia, ok := find(kids, "mdia")
	if !ok {
		return t
	}
	mk, _ := atoms(r, mdia.body, mdia.end)
	var scale uint32
	var dur uint64
	if mh, ok := find(mk, "mdhd"); ok {
		b := readAll(r, mh, 32)
		if len(b) >= 20 {
			if b[0] == 1 && len(b) >= 32 {
				scale = binary.BigEndian.Uint32(b[20:24])
				dur = binary.BigEndian.Uint64(b[24:32])
			} else {
				scale = binary.BigEndian.Uint32(b[12:16])
				dur = uint64(binary.BigEndian.Uint32(b[16:20]))
			}
		}
	}
	if scale > 0 {
		t.durationMS = int64(dur) * 1000 / int64(scale)
	}
	if hd, ok := find(mk, "hdlr"); ok {
		b := readAll(r, hd, 12)
		if len(b) >= 12 {
			t.handler = string(b[8:12])
		}
	}
	minf, ok := find(mk, "minf")
	if !ok {
		return t
	}
	mik, _ := atoms(r, minf.body, minf.end)
	stbl, ok := find(mik, "stbl")
	if !ok {
		return t
	}
	sk, _ := atoms(r, stbl.body, stbl.end)
	if sd, ok := find(sk, "stsd"); ok {
		b := readAll(r, sd, 4096)
		if len(b) >= 16 {
			t.format = string(b[12:16])
			entry := b[8:]
			switch t.handler {
			case "vide":
				if len(entry) >= 36 {
					t.width = int(binary.BigEndian.Uint16(entry[32:34]))
					t.height = int(binary.BigEndian.Uint16(entry[34:36]))
				}
			case "soun":
				if len(entry) >= 36 {
					version := binary.BigEndian.Uint16(entry[16:18])
					t.channels = int(binary.BigEndian.Uint16(entry[24:26]))
					t.bitDepth = int(binary.BigEndian.Uint16(entry[26:28]))
					t.sampleRate = int(binary.BigEndian.Uint32(entry[32:36]) >> 16)
					// Older descriptions say 16 whatever the PCM width; the
					// format itself is reliable.
					if w, ok := map[string]int{"in24": 24, "in32": 32, "fl32": 32, "fl64": 64}[t.format]; ok && t.bitDepth <= 16 {
						t.bitDepth = w
					}
					if version == 2 && len(entry) >= 72 {
						// QuickTime sound description v2: rate as float64, channels as uint32.
						t.sampleRate = int(math.Float64frombits(binary.BigEndian.Uint64(entry[40:48])))
						t.channels = int(binary.BigEndian.Uint32(entry[48:52]))
						t.bitDepth = int(binary.BigEndian.Uint32(entry[56:60]))
					}
				}
			case "tmcd":
				// After the 16-byte entry header: reserved(4), flags(4),
				// timescale(4), frame duration(4), frames per second(1),
				// reserved(1), then an optional 'name' atom with the reel.
				if len(entry) >= 34 {
					flags := binary.BigEndian.Uint32(entry[20:24])
					t.tcDrop = flags&1 != 0
					t.tcScale = binary.BigEndian.Uint32(entry[24:28])
					t.tcFrameDur = binary.BigEndian.Uint32(entry[28:32])
				}
				if len(entry) > 42 {
					rest := entry[34:]
					if len(rest) >= 8 && string(rest[4:8]) == "name" && len(rest) >= 12 {
						n := int(binary.BigEndian.Uint16(rest[8:10]))
						if 12+n <= len(rest) {
							t.reel = strings.TrimSpace(string(rest[12 : 12+n]))
						}
					}
				}
			}
		}
	}
	if st, ok := find(sk, "stts"); ok && scale > 0 {
		b := readAll(r, st, 1<<16)
		if len(b) >= 8 {
			n := int(binary.BigEndian.Uint32(b[4:8]))
			var samples, total uint64
			for i := 0; i < n && 8+i*8+8 <= len(b); i++ {
				c := uint64(binary.BigEndian.Uint32(b[8+i*8:]))
				d := uint64(binary.BigEndian.Uint32(b[12+i*8:]))
				samples += c
				total += c * d
			}
			if t.handler == "vide" && total > 0 {
				t.frameRate = math.Round(float64(samples)*float64(scale)/float64(total)*1000) / 1000
			}
		}
	}
	if co, ok := find(sk, "stco"); ok {
		b := readAll(r, co, 12)
		if len(b) >= 12 && binary.BigEndian.Uint32(b[4:8]) > 0 {
			t.firstChunk = int64(binary.BigEndian.Uint32(b[8:12]))
		}
	} else if co, ok := find(sk, "co64"); ok {
		b := readAll(r, co, 16)
		if len(b) >= 16 && binary.BigEndian.Uint32(b[4:8]) > 0 {
			t.firstChunk = int64(binary.BigEndian.Uint64(b[8:16]))
		}
	}
	return t
}

func codecName(table map[string]string, fourcc string) string {
	if n, ok := table[fourcc]; ok {
		return n
	}
	return strings.TrimSpace(strings.ToUpper(fourcc))
}

func qtTime(secs uint64) string {
	if secs == 0 {
		return ""
	}
	t := qtEpoch.Add(time.Duration(secs) * time.Second)
	if t.Year() < 1990 || t.Year() > 2100 {
		return ""
	}
	return t.Format(time.RFC3339)
}

func addExtra(info *Info, k, v string) {
	if v == "" {
		return
	}
	if info.Extra == nil {
		info.Extra = map[string]string{}
	}
	info.Extra[k] = v
}

// readMeta reads a QuickTime metadata atom with keys and ilst children:
// camera make and model, creation date, and anything else short.
func readMeta(r io.ReaderAt, meta atom, info *Info) {
	body := meta.body
	var hdr [8]byte
	if _, err := r.ReadAt(hdr[:], body); err == nil && string(hdr[4:8]) != "hdlr" && string(hdr[4:8]) != "keys" && string(hdr[4:8]) != "ilst" {
		body += 4 // MP4-style meta has a version/flags word first
	}
	kids, _ := atoms(r, body, meta.end)
	var names []string
	if k, ok := find(kids, "keys"); ok {
		b := readAll(r, k, 1<<16)
		if len(b) >= 8 {
			n := int(binary.BigEndian.Uint32(b[4:8]))
			pos := 8
			for i := 0; i < n && pos+8 <= len(b); i++ {
				size := int(binary.BigEndian.Uint32(b[pos:]))
				if size < 8 || pos+size > len(b) {
					break
				}
				names = append(names, string(b[pos+8:pos+size]))
				pos += size
			}
		}
	}
	il, ok := find(kids, "ilst")
	if !ok {
		return
	}
	items, _ := atoms(r, il.body, il.end)
	for _, it := range items {
		var key string
		idx := int(binary.BigEndian.Uint32([]byte(it.typ)))
		if idx >= 1 && idx <= len(names) {
			key = names[idx-1]
		} else {
			key = it.typ
		}
		dk, _ := atoms(r, it.body, it.end)
		d, ok := find(dk, "data")
		if !ok {
			continue
		}
		b := readAll(r, d, 512)
		if len(b) < 8 {
			continue
		}
		typ := binary.BigEndian.Uint32(b[:4])
		val := b[8:]
		var s string
		switch typ {
		case 1: // UTF-8
			s = strings.TrimRight(string(val), "\x00")
		default:
			continue
		}
		switch key {
		case "com.apple.quicktime.make", "\xa9mak":
			addExtra(info, "make", s)
		case "com.apple.quicktime.model", "\xa9mod":
			addExtra(info, "model", s)
		case "com.apple.quicktime.creationdate", "\xa9day":
			if info.Created == "" {
				info.Created = s
			}
		case "com.apple.quicktime.software", "\xa9swr":
			addExtra(info, "software", s)
		default:
			if strings.HasPrefix(key, "com.apple.quicktime.") {
				key = strings.TrimPrefix(key, "com.apple.quicktime.")
			}
			if len(s) <= 120 && len(info.Extra) < 24 {
				addExtra(info, key, s)
			}
		}
	}
}
