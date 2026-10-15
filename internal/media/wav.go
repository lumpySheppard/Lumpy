package media

import (
	"encoding/binary"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// probeWAV reads the fmt, data, bext and iXML chunks of a WAV/BWF file.
func probeWAV(f *os.File, info *Info) error {
	info.Container = "WAV"
	info.Kind = Audio
	info.Source = "wav"
	st, err := f.Stat()
	if err != nil {
		return err
	}
	size := st.Size()
	var hdr [8]byte
	var dataBytes int64
	var blockAlign int
	for pos := int64(12); pos+8 <= size; {
		if _, err := f.ReadAt(hdr[:], pos); err != nil {
			break
		}
		id := string(hdr[:4])
		n := int64(binary.LittleEndian.Uint32(hdr[4:8]))
		body := pos + 8
		switch id {
		case "fmt ":
			b := make([]byte, 40)
			m, _ := f.ReadAt(b, body)
			if m >= 16 {
				tag := binary.LittleEndian.Uint16(b[0:2])
				info.Channels = int(binary.LittleEndian.Uint16(b[2:4]))
				info.SampleRate = int(binary.LittleEndian.Uint32(b[4:8]))
				blockAlign = int(binary.LittleEndian.Uint16(b[12:14]))
				info.BitDepth = int(binary.LittleEndian.Uint16(b[14:16]))
				switch tag {
				case 1:
					info.AudioCodec = "PCM"
				case 3:
					info.AudioCodec = "PCM float"
				case 0xFFFE:
					info.AudioCodec = "PCM"
					if m >= 26 {
						sub := binary.LittleEndian.Uint16(b[24:26])
						if sub == 3 {
							info.AudioCodec = "PCM float"
						}
					}
				default:
					info.AudioCodec = "WAV format " + strconv.Itoa(int(tag))
				}
			}
			info.Container = "WAV"
		case "data":
			dataBytes = n
			if n == 0 || n == 0xFFFFFFFF {
				dataBytes = size - body
			}
		case "bext":
			b := make([]byte, 602)
			m, _ := f.ReadAt(b, body)
			if m >= 346 {
				desc := strings.TrimRight(string(b[0:256]), "\x00 ")
				orig := strings.TrimRight(string(b[256:288]), "\x00 ")
				date := strings.TrimRight(string(b[320:330]), "\x00 ")
				tm := strings.TrimRight(string(b[330:338]), "\x00 ")
				ref := binary.LittleEndian.Uint64(b[338:346])
				info.Container = "BWF"
				addExtra(info, "description", desc)
				addExtra(info, "originator", orig)
				if date != "" {
					info.Created = date
					if tm != "" {
						info.Created += "T" + tm
					}
				}
				if info.SampleRate > 0 {
					// Time reference is samples since midnight.
					secs := float64(ref) / float64(info.SampleRate)
					h := int64(secs) / 3600
					mnt := (int64(secs) / 60) % 60
					s := int64(secs) % 60
					fr := info.FrameRate
					if fr == 0 {
						fr = 25
					}
					ff := int64((secs - float64(int64(secs))) * fr)
					info.Timecode = pad2(h) + ":" + pad2(mnt) + ":" + pad2(s) + ":" + pad2(ff)
				}
			}
		case "iXML":
			b := make([]byte, min64(n, 1<<16))
			m, _ := f.ReadAt(b, body)
			readIXML(string(b[:m]), info)
		}
		pos = body + n + n&1
	}
	if blockAlign > 0 && info.SampleRate > 0 && dataBytes > 0 {
		info.DurationMS = dataBytes / int64(blockAlign) * 1000 / int64(info.SampleRate)
	}
	return nil
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

var ixmlTag = regexp.MustCompile(`(?s)<(SCENE|TAKE|TAPE|PROJECT|NOTE|CIRCLED|TIMECODE_RATE|FRAME_RATE|UBITS|TRACK_COUNT)>\s*([^<]*?)\s*</`)

// readIXML pulls scene, take, reel and frame rate from an iXML chunk
// without a full XML parse.
func readIXML(x string, info *Info) {
	for _, m := range ixmlTag.FindAllStringSubmatch(x, -1) {
		v := strings.TrimSpace(m[2])
		switch m[1] {
		case "SCENE":
			info.Scene = v
		case "TAKE":
			info.Take = v
		case "TAPE":
			info.Reel = v
		case "PROJECT":
			addExtra(info, "project", v)
		case "NOTE":
			addExtra(info, "note", v)
		case "CIRCLED":
			addExtra(info, "circled", v)
		case "TIMECODE_RATE", "FRAME_RATE":
			if strings.Contains(v, "/") {
				parts := strings.SplitN(v, "/", 2)
				a, _ := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
				b, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
				if b > 0 {
					info.FrameRate = a / b
				}
			} else if fr, err := strconv.ParseFloat(v, 64); err == nil {
				info.FrameRate = fr
			}
		}
	}
	if info.FrameRate > 0 && info.Timecode != "" && info.SampleRate > 0 {
		// Recompute the frames part with the real rate.
		parts := strings.Split(info.Timecode, ":")
		if len(parts) == 4 {
			h, _ := strconv.Atoi(parts[0])
			m, _ := strconv.Atoi(parts[1])
			s, _ := strconv.Atoi(parts[2])
			info.Timecode = pad2(int64(h)) + ":" + pad2(int64(m)) + ":" + pad2(int64(s)) + ":" + parts[3]
		}
	}
}
