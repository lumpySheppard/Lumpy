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
		Profile       string            `json:"profile"`
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
	if info.Container == "" && ff.Format.FormatLongName != "" {
		info.Container = containerName(ff.Format.FormatLongName)
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
			info.VideoCodec = codecDisplayName(s.CodecName, s.Profile, s.CodecLongName)
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
			info.AudioCodec = codecDisplayName(s.CodecName, s.Profile, s.CodecLongName)
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

// ffCodecs maps ffprobe's codec ids to the names the QuickTime parser
// uses, so a ProRes clip reads the same whichever way it was probed and
// the Clips filter has one entry per codec.
var ffCodecs = map[string]string{
	"h264": "H.264", "hevc": "HEVC", "av1": "AV1", "vp9": "VP9", "vp8": "VP8", "mpeg4": "MPEG-4 Visual", "mpeg2video": "MPEG-2",
	"mpeg1video": "MPEG-1", "mjpeg": "Motion JPEG", "dnxhd": "DNxHD", "cfhd": "CineForm", "prores_raw": "ProRes RAW",
	"v210": "Uncompressed 10-bit 4:2:2", "v410": "Uncompressed 10-bit 4:4:4", "rawvideo": "Uncompressed", "dvvideo": "DV",
	"jpeg2000": "JPEG 2000", "png": "PNG", "gif": "GIF", "qtrle": "Animation", "hap": "Hap", "ffv1": "FFV1", "vc1": "VC-1", "wmv3": "WMV",
	"aac": "AAC", "mp3": "MP3", "mp2": "MP2", "flac": "FLAC", "alac": "Apple Lossless", "ac3": "AC-3", "eac3": "E-AC-3", "opus": "Opus",
	"vorbis": "Vorbis", "wmav2": "WMA", "truehd": "Dolby TrueHD", "dts": "DTS", "amr_nb": "AMR", "amr_wb": "AMR",
}

// codecDisplayName names a codec the way the industry does: ProRes by
// flavour, DNx by family, PCM for every linear PCM layout.
func codecDisplayName(name, profile, long string) string {
	switch name {
	case "prores":
		switch {
		case strings.Contains(profile, "XQ"):
			return "ProRes 4444 XQ"
		case strings.Contains(profile, "4444"):
			return "ProRes 4444"
		case strings.Contains(profile, "HQ"):
			return "ProRes 422 HQ"
		case strings.Contains(profile, "LT"):
			return "ProRes 422 LT"
		case strings.Contains(profile, "Proxy"):
			return "ProRes 422 Proxy"
		default:
			return "ProRes 422"
		}
	case "dnxhd":
		if strings.Contains(profile, "DNxHR") {
			return "DNxHR"
		}
		return "DNxHD"
	case "h264":
		if strings.Contains(profile, "Intra") {
			return "H.264 Intra"
		}
	}
	if strings.HasPrefix(name, "pcm_") {
		return "PCM"
	}
	if c, ok := ffCodecs[name]; ok {
		return c
	}
	if long != "" {
		// "H.264 / AVC / MPEG-4 AVC / MPEG-4 part 10" → "H.264"
		if i := strings.Index(long, " / "); i > 0 {
			return long[:i]
		}
		return long
	}
	return strings.ToUpper(name)
}

// containerName trims ffprobe's long format names: "QuickTime / MOV" →
// "QuickTime", "MXF (Material eXchange Format)" → "MXF".
func containerName(long string) string {
	if i := strings.Index(long, " / "); i > 0 {
		long = long[:i]
	}
	if i := strings.Index(long, " ("); i > 0 {
		long = long[:i]
	}
	return strings.TrimSpace(long)
}
