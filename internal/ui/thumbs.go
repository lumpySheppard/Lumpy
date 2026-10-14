package ui

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/lumpysheppard/lumpy/internal/media"
)

// Thumbnails for the Clips filmstrip: a frame of a video (through
// ffmpeg) or a small copy of a still, kept next to the library so each
// is made once.
const (
	thumbWidth    = 320
	thumbMaxBytes = 500 << 20 // the cache is trimmed past this
	thumbMaxFiles = 20000
)

var (
	thumbSem    = make(chan struct{}, 2) // ffmpeg runs at a time
	thumbWrites atomic.Int32
)

func init() {
	image.RegisterFormat("jpeg", "\xff\xd8", jpeg.Decode, jpeg.DecodeConfig)
	image.RegisterFormat("png", "\x89PNG", png.Decode, png.DecodeConfig)
	image.RegisterFormat("gif", "GIF8", gif.Decode, gif.DecodeConfig)
}

// thumbDir is where thumbnails are kept, next to the library.
func (s *Server) thumbDir() string { return filepath.Join(filepath.Dir(s.lockPath), "thumbs") }

// thumb serves a JPEG of a file on a plugged-in drive: ?path= on the
// drive and ?at= seconds into a video.
func (s *Server) thumb(w http.ResponseWriter, r *http.Request) (any, error) {
	d, err := s.drive(r)
	if err != nil {
		return nil, err
	}
	v := s.mountOf(d.ID)
	if v == nil {
		return nil, badRequest("%s is not plugged in", d.Title())
	}
	p := filepath.Clean(filepath.FromSlash(r.URL.Query().Get("path")))
	if p == "." || strings.HasPrefix(p, "..") || filepath.IsAbs(p) {
		return nil, badRequest("bad path")
	}
	full := filepath.Join(v.Mount, p)
	st, err := os.Stat(full)
	if err != nil || !st.Mode().IsRegular() {
		return nil, httpError{http.StatusNotFound, "file not found on the drive"}
	}
	at, _ := strconv.ParseFloat(r.URL.Query().Get("at"), 64)
	if at < 0 || at != at {
		at = 0
	}
	sum := sha1.Sum([]byte(fmt.Sprintf("%s\x00%s\x00%d\x00%d\x00%.1f", d.ID, p, st.Size(), st.ModTime().Unix(), at)))
	key := hex.EncodeToString(sum[:])
	cache := filepath.Join(s.thumbDir(), key[:2], key+".jpg")
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	if data, err := os.ReadFile(cache); err == nil {
		w.Write(data)
		return nil, nil
	}
	data, err := makeThumb(r.Context(), full, at)
	if err != nil {
		return nil, httpError{http.StatusNotFound, err.Error()}
	}
	os.MkdirAll(filepath.Dir(cache), 0o755)
	os.WriteFile(cache, data, 0o644)
	if thumbWrites.Add(1)%200 == 0 {
		go trimThumbs(s.thumbDir())
	}
	w.Write(data)
	return nil, nil
}

var videoExts = map[string]bool{".mov": true, ".mp4": true, ".m4v": true, ".mxf": true, ".avi": true, ".mkv": true,
	".mts": true, ".m2ts": true, ".mpg": true, ".mpeg": true, ".webm": true, ".wmv": true, ".3gp": true, ".dv": true}

// makeThumb renders one JPEG, thumbWidth wide, of the file.
func makeThumb(ctx context.Context, full string, at float64) ([]byte, error) {
	ext := strings.ToLower(filepath.Ext(full))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif":
		return stillThumb(full)
	}
	if !videoExts[ext] {
		return nil, fmt.Errorf("no preview for %s files", strings.TrimPrefix(ext, "."))
	}
	ff := media.FFmpeg()
	if ff == "" {
		return nil, fmt.Errorf("ffmpeg is not installed")
	}
	select {
	case thumbSem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-thumbSem }()
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	for _, seek := range []float64{at, 0} {
		cmd := exec.CommandContext(ctx, ff, "-v", "error", "-nostdin", "-ss", fmt.Sprintf("%.2f", seek), "-i", full,
			"-frames:v", "1", "-vf", fmt.Sprintf("scale=%d:-2", thumbWidth), "-q:v", "5", "-f", "image2", "pipe:1")
		var out bytes.Buffer
		cmd.Stdout = &out
		if err := cmd.Run(); err == nil && out.Len() > 0 {
			return out.Bytes(), nil
		}
		if seek == 0 {
			break
		}
		// Past the end, or the stream has no frame there: take the first.
	}
	return nil, fmt.Errorf("ffmpeg couldn't read a frame")
}

// stillThumb shrinks a JPEG, PNG or GIF.
func stillThumb(full string) ([]byte, error) {
	f, err := os.Open(full)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return nil, fmt.Errorf("not a readable image")
	}
	if cfg.Width*cfg.Height > 80_000_000 {
		return nil, fmt.Errorf("image too large to preview")
	}
	f.Seek(0, 0)
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("not a readable image")
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, shrink(img, thumbWidth), &jpeg.Options{Quality: 82}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// shrink scales an image down to width pixels wide, averaging the
// pixels each new one covers; an image already that small is returned
// as is.
func shrink(src image.Image, width int) image.Image {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= width || sw == 0 || sh == 0 {
		return src
	}
	height := sh * width / sw
	if height < 1 {
		height = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		y0, y1 := b.Min.Y+y*sh/height, b.Min.Y+(y+1)*sh/height
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < width; x++ {
			x0, x1 := b.Min.X+x*sw/width, b.Min.X+(x+1)*sw/width
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, bl, n uint64
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					cr, cg, cb, _ := src.At(xx, yy).RGBA()
					r += uint64(cr >> 8)
					g += uint64(cg >> 8)
					bl += uint64(cb >> 8)
					n++
				}
			}
			dst.SetRGBA(x, y, color.RGBA{uint8(r / n), uint8(g / n), uint8(bl / n), 255})
		}
	}
	return dst
}

// trimThumbs drops the oldest thumbnails once the cache is past its
// size or count, down to seven tenths of it.
func trimThumbs(dir string) {
	type entry struct {
		path string
		size int64
		at   time.Time
	}
	var all []entry
	var total int64
	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			all = append(all, entry{path, info.Size(), info.ModTime()})
			total += info.Size()
		}
		return nil
	})
	if total <= thumbMaxBytes && len(all) <= thumbMaxFiles {
		return
	}
	sort.Slice(all, func(i, j int) bool { return all[i].at.Before(all[j].at) })
	wantBytes, wantFiles := int64(thumbMaxBytes)*7/10, thumbMaxFiles*7/10
	for _, e := range all {
		if total <= wantBytes && len(all) <= wantFiles {
			break
		}
		if os.Remove(e.path) == nil {
			total -= e.size
			all = all[1:]
		}
	}
}
