package ui

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lumpysheppard/lumpy/internal/catalog"
	"github.com/lumpysheppard/lumpy/internal/job"
	"github.com/lumpysheppard/lumpy/internal/media"
)

// probeIfMounted reads a file's metadata when its drive is plugged in,
// and otherwise records only what the name says.
func probeIfMounted(s *Server, d *catalog.Drive, p string) *media.Info {
	if v := s.mountOf(d.ID); v != nil && media.Probable(p) {
		if info, err := media.Probe(filepath.Join(v.Mount, filepath.FromSlash(p))); err == nil {
			return info
		}
	}
	return &media.Info{Kind: media.KindOf(p)}
}

// fileDetail is everything the app shows for one file.
func (s *Server) fileDetail(w http.ResponseWriter, r *http.Request) (any, error) {
	d, err := s.drive(r)
	if err != nil {
		return nil, err
	}
	p := r.URL.Query().Get("path")
	states, err := s.backupStates(d.ID)
	if err != nil {
		return nil, err
	}
	st := states[p]
	m, err := s.db.MediaFor(d.ID, p)
	if err != nil {
		return nil, err
	}
	files, err := s.db.Files(d.ID)
	if err != nil {
		return nil, err
	}
	var f *catalog.File
	for i := range files {
		if files[i].Path == p {
			f = &files[i]
			break
		}
	}
	if f == nil {
		return nil, httpError{http.StatusNotFound, "no such file on this drive"}
	}
	// Where else the content lives.
	var copies []map[string]any
	if f.XXH64 != "" {
		cov, _ := s.db.Coverage(d.ID)
		for _, c := range cov {
			if c.Path != p {
				continue
			}
			for _, cp := range c.Copies {
				copies = append(copies, map[string]any{"driveId": cp.DriveID, "drive": cp.DriveName, "path": cp.Path, "verified": cp.Verified})
			}
		}
	}
	if copies == nil {
		copies = []map[string]any{}
	}
	v := s.mountOf(d.ID)
	return map[string]any{
		"drive":     map[string]any{"id": d.ID, "title": d.Title(), "connected": v != nil},
		"file":      f,
		"state":     st,
		"media":     m,
		"copies":    copies,
		"playable":  v != nil && playable(p),
		"probeable": v != nil && m == nil,
	}, nil
}

func playable(p string) bool {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".mov", ".mp4", ".m4v", ".m4a", ".wav", ".aif", ".aiff", ".mp3", ".jpg", ".jpeg", ".png", ".gif", ".heic", ".webp":
		return true
	}
	return false
}

func (s *Server) setFileEdits(w http.ResponseWriter, r *http.Request) (any, error) {
	d, err := s.drive(r)
	if err != nil {
		return nil, err
	}
	p := r.URL.Query().Get("path")
	var body struct {
		catalog.MediaEdits
		Reread bool `json:"reread"` // read the file again, keeping the edits
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, badRequest("bad request: %v", err)
	}
	switch body.QC {
	case "", "ok", "flag", "reject":
	default:
		return nil, badRequest("QC must be ok, flag, reject or empty")
	}
	m, _ := s.db.MediaFor(d.ID, p)
	if m == nil || body.Reread {
		// Nothing read from the file yet (or a fresh read was asked for):
		// store the edits against what the library knows about it.
		files, _ := s.db.Files(d.ID)
		for _, f := range files {
			if f.Path == p {
				if body.Reread && s.mountOf(d.ID) == nil {
					return nil, badRequest("%s is not plugged in", d.Title())
				}
				info := probeIfMounted(s, d, p)
				if err := s.db.SaveMedia(d.ID, p, f.XXH64, f.Size, info); err != nil {
					return nil, err
				}
			}
		}
	}
	return map[string]bool{"ok": true}, s.db.SetMediaEdits(d.ID, p, body.MediaEdits)
}

// streamFile serves a file from a plugged-in drive for preview. Range
// requests let the player seek.
func (s *Server) streamFile(w http.ResponseWriter, r *http.Request) (any, error) {
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
	if ct := contentType(p); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, full)
	return nil, nil
}

func contentType(p string) string {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".mov":
		return "video/quicktime"
	case ".mp4", ".m4v":
		return "video/mp4"
	case ".m4a":
		return "audio/mp4"
	case ".wav":
		return "audio/wav"
	case ".aif", ".aiff":
		return "audio/aiff"
	case ".mp3":
		return "audio/mpeg"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".heic":
		return "image/heic"
	case ".webp":
		return "image/webp"
	}
	return ""
}

// probeDrive starts a job that reads metadata for a drive's clips.
func (s *Server) probeDrive(w http.ResponseWriter, r *http.Request) (any, error) {
	d, err := s.drive(r)
	if err != nil {
		return nil, err
	}
	v := s.mountOf(d.ID)
	if v == nil {
		return nil, badRequest("%s is not plugged in", d.Title())
	}
	if err := s.requireIdle(d); err != nil {
		return nil, err
	}
	j := s.jobs.Start("metadata", "Read metadata on "+d.Title(), "lumpy drive metadata "+shellQuote(d.Title()), d.ID, func(ctx context.Context, rep *job.Report) error {
		return job.ProbeDrive(ctx, s.db, v, 0, rep)
	})
	return map[string]any{"job": j.ID}, nil
}

func (s *Server) mediaFilter(r *http.Request) (catalog.MediaFilter, error) {
	q := r.URL.Query()
	f := catalog.MediaFilter{DriveID: q.Get("drive"), Kind: q.Get("kind"), Codec: q.Get("codec"), QC: q.Get("qc"), Text: q.Get("q")}
	if v := q.Get("rate"); v != "" {
		f.FrameRate, _ = strconv.ParseFloat(v, 64)
	}
	if v := q.Get("project"); v != "" {
		id, _ := strconv.ParseInt(v, 10, 64)
		p, err := s.db.Project(id)
		if err != nil {
			return f, httpError{http.StatusNotFound, err.Error()}
		}
		f.Project = p
	}
	return f, nil
}

// listMedia is the Media tab's data.
func (s *Server) listMedia(w http.ResponseWriter, r *http.Request) (any, error) {
	f, err := s.mediaFilter(r)
	if err != nil {
		return nil, err
	}
	f.Limit = 2000
	rows, err := s.db.ListMedia(f)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []catalog.MediaRow{}
	}
	codecs, rates, err := s.db.MediaFacets()
	if err != nil {
		return nil, err
	}
	if codecs == nil {
		codecs = []string{}
	}
	if rates == nil {
		rates = []float64{}
	}
	return map[string]any{"files": rows, "codecs": codecs, "rates": rates, "limit": f.Limit}, nil
}

// exportMedia writes the media listing as CSV for post.
func (s *Server) exportMedia(w http.ResponseWriter, r *http.Request) (any, error) {
	f, err := s.mediaFilter(r)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.ListMedia(f)
	if err != nil {
		return nil, err
	}
	titles := map[string]string{}
	if ds, err := s.db.Drives(); err == nil {
		for _, d := range ds {
			titles[d.ID] = d.Title()
		}
	}
	name := "lumpy-media"
	if f.Project != nil {
		name += "-" + safeFolder(f.Project.Name)
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s.csv"`, name, time.Now().Format("2006-01-02")))
	cw := csv.NewWriter(w)
	cw.Write([]string{"File", "Drive", "Path", "Size (bytes)", "xxh64", "Copies elsewhere", "Kind", "Container", "Video codec", "Width", "Height",
		"Frame rate", "Duration", "Start timecode", "Audio codec", "Channels", "Sample rate", "Bit depth", "Camera", "Reel", "Scene", "Shot", "Take",
		"QC", "Tags", "Notes", "Created"})
	for _, m := range rows {
		dur := ""
		if m.DurationMS > 0 {
			d := time.Duration(m.DurationMS) * time.Millisecond
			dur = fmt.Sprintf("%02d:%02d:%02d.%03d", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60, m.DurationMS%1000)
		}
		num := func(n int) string {
			if n == 0 {
				return ""
			}
			return strconv.Itoa(n)
		}
		rate := ""
		if m.FrameRate > 0 {
			rate = strconv.FormatFloat(m.FrameRate, 'f', -1, 64)
		}
		cw.Write([]string{filepath.Base(m.Path), titles[m.DriveID], m.Path, strconv.FormatInt(m.Size, 10), m.XXH64, strconv.Itoa(m.Copies),
			string(m.Kind), m.Container, m.VideoCodec, num(m.Width), num(m.Height), rate, dur, m.Timecode, m.AudioCodec, num(m.Channels),
			num(m.SampleRate), num(m.BitDepth), m.Camera, m.Reel, m.Edits.Scene, m.Edits.Shot, m.Edits.Take, m.Edits.QC, m.Edits.Tags,
			m.Edits.Notes, m.Created})
	}
	cw.Flush()
	return nil, cw.Error()
}
