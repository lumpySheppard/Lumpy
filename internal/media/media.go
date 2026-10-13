// Package media reads what post needs to know about a clip from the file
// itself: container, codec, frame size, frame rate, duration, timecode,
// audio format and camera, for QuickTime/MP4 and WAV/BWF files, and
// through ffprobe for anything else when it is installed.
package media

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Kind is the broad type of a file, from its extension.
type Kind string

const (
	Video    Kind = "video"
	Audio    Kind = "audio"
	Image    Kind = "image"
	Document Kind = "document"
	Other    Kind = "other"
)

// Info is what was read from a file. Zero values mean unknown.
type Info struct {
	Kind       Kind    `json:"kind"`
	Container  string  `json:"container,omitempty"`
	VideoCodec string  `json:"videoCodec,omitempty"`
	Width      int     `json:"width,omitempty"`
	Height     int     `json:"height,omitempty"`
	FrameRate  float64 `json:"frameRate,omitempty"` // frames per second
	DurationMS int64   `json:"durationMs,omitempty"`
	Timecode   string  `json:"timecode,omitempty"` // start timecode, HH:MM:SS:FF (or ; for drop frame)
	AudioCodec string  `json:"audioCodec,omitempty"`
	Channels   int     `json:"channels,omitempty"`
	SampleRate int     `json:"sampleRate,omitempty"`
	BitDepth   int     `json:"bitDepth,omitempty"`
	Camera     string  `json:"camera,omitempty"` // make and model
	Reel       string  `json:"reel,omitempty"`
	Scene      string  `json:"scene,omitempty"`
	Take       string  `json:"take,omitempty"`
	Created    string  `json:"created,omitempty"` // as the file records it, RFC 3339 when known
	Source     string  `json:"source"`            // "quicktime", "wav", "ffprobe", "extension"
	// Extra holds anything else of interest, as name -> value.
	Extra map[string]string `json:"extra,omitempty"`
}

// Marshal renders the info as JSON for storage.
func (i *Info) Marshal() string {
	b, _ := json.Marshal(i)
	return string(b)
}

var kinds = map[string]Kind{
	".mov": Video, ".mp4": Video, ".m4v": Video, ".mxf": Video, ".r3d": Video, ".braw": Video, ".ari": Video, ".arx": Video,
	".avi": Video, ".mkv": Video, ".mts": Video, ".m2ts": Video, ".mpg": Video, ".mpeg": Video, ".crm": Video, ".dpx": Image,
	".wav": Audio, ".bwf": Audio, ".aif": Audio, ".aiff": Audio, ".mp3": Audio, ".flac": Audio, ".m4a": Audio, ".caf": Audio,
	".jpg": Image, ".jpeg": Image, ".png": Image, ".tif": Image, ".tiff": Image, ".dng": Image, ".cr2": Image, ".cr3": Image,
	".arw": Image, ".nef": Image, ".raf": Image, ".heic": Image, ".exr": Image, ".psd": Image, ".gif": Image, ".webp": Image,
	".pdf": Document, ".txt": Document, ".csv": Document, ".xml": Document, ".json": Document, ".doc": Document, ".docx": Document,
	".xls": Document, ".xlsx": Document, ".edl": Document, ".ale": Document, ".fcpxml": Document, ".prproj": Document, ".drp": Document,
	".md": Document, ".rtf": Document, ".numbers": Document, ".pages": Document, ".key": Document, ".ppt": Document, ".pptx": Document,
}

// KindOf classifies a file by extension.
func KindOf(name string) Kind {
	if k, ok := kinds[strings.ToLower(filepath.Ext(name))]; ok {
		return k
	}
	return Other
}

// Probable reports whether Probe is likely to learn anything from the file.
func Probable(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mov", ".mp4", ".m4v", ".m4a", ".wav", ".bwf", ".jpg", ".jpeg", ".png", ".tif", ".tiff", ".dng", ".arw", ".nef", ".cr2":
		return true
	}
	k := KindOf(name)
	return (k == Video || k == Audio) && ffprobePath() != ""
}

// Probe reads the file's metadata. It never reads more than the headers
// it needs, so it is cheap even on a 100 GB clip. Files Lumpy cannot
// read in detail still get their Kind.
func Probe(path string) (*Info, error) {
	info := &Info{Kind: KindOf(path), Source: "extension"}
	f, err := os.Open(path)
	if err != nil {
		return info, err
	}
	defer f.Close()
	var head [12]byte
	n, _ := f.ReadAt(head[:], 0)
	switch {
	case n >= 8 && string(head[4:8]) == "ftyp" || n >= 8 && string(head[4:8]) == "moov" || n >= 8 && string(head[4:8]) == "mdat" || n >= 8 && string(head[4:8]) == "wide" || n >= 8 && string(head[4:8]) == "free" || n >= 8 && string(head[4:8]) == "skip":
		err = probeQuickTime(f, info)
	case n >= 12 && string(head[0:4]) == "RIFF" && string(head[8:12]) == "WAVE":
		err = probeWAV(f, info)
	case n >= 12 && string(head[0:4]) == "FORM" && (string(head[8:12]) == "AIFF" || string(head[8:12]) == "AIFC"):
		info.Container = "AIFF"
		info.Kind = Audio
	case n >= 3 && head[0] == 0xFF && head[1] == 0xD8 && head[2] == 0xFF:
		err = probeJPEG(f, info)
	case n >= 8 && string(head[0:8]) == "\x89PNG\r\n\x1a\n":
		err = probePNG(f, info)
	case n >= 4 && (string(head[0:4]) == "II*\x00" || string(head[0:4]) == "MM\x00*"):
		err = probeTIFF(f, info)
		if ext := strings.ToLower(filepath.Ext(path)); ext == ".dng" {
			info.Container = "DNG"
		} else if ext == ".arw" || ext == ".nef" || ext == ".cr2" {
			info.Container = strings.ToUpper(ext[1:])
		}
	default:
		if p := ffprobePath(); p != "" && (info.Kind == Video || info.Kind == Audio) {
			err = probeFFprobe(p, path, info)
		}
	}
	if err != nil && errors.Is(err, errUnsupported) {
		err = nil
	}
	return info, err
}

var errUnsupported = errors.New("unsupported")

// qtEpoch is 1904-01-01, the origin of QuickTime timestamps.
var qtEpoch = time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC)

// Timecode formats a frame count as HH:MM:SS:FF for the given rate. Drop
// frame uses ';' before the frames, like editing systems do.
func Timecode(frames int64, rate float64, dropFrame bool) string {
	if rate <= 0 {
		return ""
	}
	nominal := int64(rate + 0.5) // 30 for 29.97, 24 for 23.976
	if dropFrame && (nominal == 30 || nominal == 60) {
		// Standard drop-frame arithmetic: skip 2 (or 4) frames each
		// minute except every tenth.
		drop := int64(2)
		if nominal == 60 {
			drop = 4
		}
		perMin := nominal*60 - drop
		per10 := perMin*10 + drop
		tens := frames / per10
		rem := frames % per10
		frames += drop * 9 * tens
		if rem > drop {
			frames += drop * ((rem - drop) / perMin)
		}
	}
	ff := frames % nominal
	s := frames / nominal
	sep := ":"
	if dropFrame {
		sep = ";"
	}
	return pad2(s/3600) + ":" + pad2((s/60)%60) + ":" + pad2(s%60) + sep + pad2(ff)
}

func pad2(n int64) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}
