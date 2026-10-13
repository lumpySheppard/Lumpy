package media

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ffOnce sync.Once
	ffPath string
)

// SetFFprobe names the ffprobe binary to use; "" means look on PATH and
// in the usual Homebrew locations.
func SetFFprobe(p string) {
	ffOnce.Do(func() {})
	ffPath = p
}

func ffprobePath() string {
	if ffPath != "" {
		return ffPath
	}
	ffOnce.Do(func() {
		if p, err := exec.LookPath("ffprobe"); err == nil {
			ffPath = p
			return
		}
		for _, p := range []string{"/opt/homebrew/bin/ffprobe", "/usr/local/bin/ffprobe"} {
			if _, err := os.Stat(p); err == nil {
				ffPath = p
				return
			}
		}
	})
	return ffPath
}

type ffOut struct {
	Format struct {
		FormatLongName string            `json:"format_long_name"`
		Duration       string            `json:"duration"`
		Tags           map[string]string `json:"tags"`
	} `json:"format"`
	Streams []struct {
		CodecType     string            `json:"codec_type"`
		CodecName     string            `json:"codec_name"`
		CodecLongName string            `json:"codec_long_name"`
		Width         int               `json:"width"`
		Height        int               `json:"height"`
		RFrameRate    string            `json:"r_frame_rate"`
		AvgFrameRate  string            `json:"avg_frame_rate"`
		SampleRate    string            `json:"sample_rate"`
		Channels      int               `json:"channels"`
		BitsPerSample int               `json:"bits_per_raw_sample,string"`
		Tags          map[string]string `json:"tags"`
	} `json:"streams"`
}

// probeFFprobe uses ffprobe for containers Lumpy does not read itself.
func probeFFprobe(bin, path string, info *Info) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "-v", "quiet", "-print_format", "json", "-show_format", "-show_streams", path).Output()
	if err != nil {
		return errUnsupported
	}
	var ff ffOut
	if err := json.Unmarshal(out, &ff); err != nil {
		return errUnsupported
	}
	info.Source = "ffprobe"
	if ff.Format.FormatLongName != "" {
		info.Container = ff.Format.FormatLongName
	}
	if d, err := strconv.ParseFloat(ff.Format.Duration, 64); err == nil {
		info.DurationMS = int64(d * 1000)
	}
	for _, s := range ff.Streams {
		switch s.CodecType {
		case "video":
			if info.VideoCodec != "" {
				continue
			}
			info.VideoCodec = s.CodecLongName
			if info.VideoCodec == "" {
				info.VideoCodec = strings.ToUpper(s.CodecName)
			}
			info.Width, info.Height = s.Width, s.Height
			info.FrameRate = ratio(s.AvgFrameRate)
			if info.FrameRate == 0 {
				info.FrameRate = ratio(s.RFrameRate)
			}
			if tc := s.Tags["timecode"]; tc != "" {
				info.Timecode = tc
			}
		case "audio":
			if info.AudioCodec != "" {
				continue
			}
			info.AudioCodec = s.CodecLongName
			if info.AudioCodec == "" {
				info.AudioCodec = strings.ToUpper(s.CodecName)
			}
			info.Channels = s.Channels
			info.SampleRate, _ = strconv.Atoi(s.SampleRate)
			info.BitDepth = s.BitsPerSample
		case "data":
			if tc := s.Tags["timecode"]; tc != "" && info.Timecode == "" {
				info.Timecode = tc
			}
		}
	}
	if tc := ff.Format.Tags["timecode"]; tc != "" && info.Timecode == "" {
		info.Timecode = tc
	}
	for k, v := range ff.Format.Tags {
		switch strings.ToLower(k) {
		case "com.apple.quicktime.make", "make":
			addExtra(info, "make", v)
		case "com.apple.quicktime.model", "model":
			addExtra(info, "model", v)
		case "creation_time":
			info.Created = v
		case "reel_name":
			info.Reel = v
		}
	}
	if info.Camera == "" {
		info.Camera = strings.TrimSpace(info.Extra["make"] + " " + info.Extra["model"])
	}
	if info.VideoCodec == "" && info.AudioCodec != "" {
		info.Kind = Audio
	}
	return nil
}

func ratio(s string) float64 {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 {
		f, _ := strconv.ParseFloat(s, 64)
		return f
	}
	a, _ := strconv.ParseFloat(parts[0], 64)
	b, _ := strconv.ParseFloat(parts[1], 64)
	if b == 0 {
		return 0
	}
	return float64(int64(a/b*1000+0.5)) / 1000
}
