package job

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/lumpysheppard/lumpy/internal/catalog"
	"github.com/lumpysheppard/lumpy/internal/media"
	"github.com/lumpysheppard/lumpy/internal/volume"
)

// probeFiles reads metadata for the given files on a mounted drive and
// records it. It stops quietly when the context ends and returns how
// many files were read.
func probeFiles(ctx context.Context, db *catalog.DB, v *volume.Volume, files []catalog.File, r *Report, prog *Progress) (int, error) {
	n := 0
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return n, err
		}
		if prog != nil {
			prog.SetCurrent(f.Path)
		}
		info, err := media.Probe(filepath.Join(v.Mount, filepath.FromSlash(f.Path)))
		if err != nil {
			r.Warn("%s: reading metadata: %v", f.Path, err)
			continue
		}
		if err := db.SaveMedia(v.ID, f.Path, f.XXH64, f.Size, info); err != nil {
			return n, err
		}
		n++
		if info.Kind == media.Video || info.Kind == media.Audio {
			r.Detail("  metadata  %s  %s", f.Path, Describe(info))
		}
		if prog != nil {
			prog.Add(1)
		}
	}
	return n, nil
}

// ProbeDrive reads metadata for every file on a mounted drive that has
// none yet. limit caps how many; 0 means all.
func ProbeDrive(ctx context.Context, db *catalog.DB, v *volume.Volume, limit int, r *Report) error {
	files, err := db.Unprobed(v.ID)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		r.Info("Every clip on %s already has its metadata", v.Name)
		return nil
	}
	if limit > 0 && len(files) > limit {
		files = files[:limit]
	}
	r.Info("Reading metadata for %d files on %s", len(files), v.Name)
	prog := r.startProgress("Reading metadata", int64(len(files)))
	n, err := probeFiles(ctx, db, v, files, r, prog)
	took := prog.Finish()
	if err != nil {
		return err
	}
	db.Log(v.ID, "metadata", fmt.Sprintf("read metadata for %d files in %s", n, took.Round(time.Second)), 0)
	r.Info("OK    %s  metadata read for %d files in %s", v.Name, n, took.Round(time.Second))
	return nil
}

// Describe sums up a clip in one line, the way a report would.
func Describe(i *media.Info) string {
	var parts []string
	if i.VideoCodec != "" {
		parts = append(parts, i.VideoCodec)
	}
	if i.Width > 0 {
		parts = append(parts, fmt.Sprintf("%d×%d", i.Width, i.Height))
	}
	if i.FrameRate > 0 {
		parts = append(parts, fmt.Sprintf("%g fps", i.FrameRate))
	}
	if i.VideoCodec == "" && i.AudioCodec != "" {
		a := i.AudioCodec
		if i.SampleRate > 0 {
			a += fmt.Sprintf(" %g kHz", float64(i.SampleRate)/1000)
		}
		if i.BitDepth > 0 {
			a += fmt.Sprintf(" %d-bit", i.BitDepth)
		}
		if i.Channels > 0 {
			a += fmt.Sprintf(" %dch", i.Channels)
		}
		parts = append(parts, a)
	}
	if i.DurationMS > 0 {
		d := time.Duration(i.DurationMS) * time.Millisecond
		parts = append(parts, fmt.Sprintf("%02d:%02d:%02d", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60))
	}
	if i.Timecode != "" {
		parts = append(parts, "TC "+i.Timecode)
	}
	if i.Camera != "" {
		parts = append(parts, i.Camera)
	}
	if len(parts) == 0 {
		return string(i.Kind)
	}
	out := parts[0]
	for _, p := range parts[1:] {
		out += " · " + p
	}
	return out
}
