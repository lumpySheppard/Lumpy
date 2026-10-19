package ui

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lumpysheppard/lumpy/internal/catalog"
	"github.com/lumpysheppard/lumpy/internal/job"
)

// Preferences and library upkeep, behind Settings.

func (s *Server) getPrefs(w http.ResponseWriter, r *http.Request) (any, error) {
	return s.db.Prefs(), nil
}

func (s *Server) putPrefs(w http.ResponseWriter, r *http.Request) (any, error) {
	p := s.db.Prefs()
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		return nil, badRequest("bad request: %v", err)
	}
	if err := s.db.SavePrefs(p); err != nil {
		return nil, err
	}
	p = s.db.Prefs()
	job.ApplyPrefs(p)
	// Drive filtering changed: list the volumes again.
	s.mu.Lock()
	s.volsAt = time.Time{}
	s.mu.Unlock()
	return p, nil
}

func (s *Server) libraryInfo(w http.ResponseWriter, r *http.Request) (any, error) {
	return s.db.Info()
}

// moveLibrary copies the library into a folder and points future
// launches at it; the copy is used after a relaunch.
func (s *Server) moveLibrary(w http.ResponseWriter, r *http.Request) (any, error) {
	var body struct {
		Folder string `json:"folder"` // "" goes back to the default folder
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, badRequest("bad request: %v", err)
	}
	if os.Getenv("LUMPY_HOME") != "" {
		return nil, badRequest("the library location is fixed by LUMPY_HOME")
	}
	folder := strings.TrimSpace(body.Folder)
	info, err := s.db.Info()
	if err != nil {
		return nil, err
	}
	if folder == "" {
		folder = info.Default
	}
	folder = filepath.Clean(folder)
	if folder == filepath.Dir(info.Path) {
		return nil, badRequest("the library is already in %s", folder)
	}
	if st, err := os.Stat(folder); err != nil || !st.IsDir() {
		return nil, badRequest("%s is not a folder", folder)
	}
	if s.jobs.AnyRunning() {
		return nil, badRequest("wait for the running job to finish first")
	}
	dest := filepath.Join(folder, "catalog.db")
	if err := s.db.CopyTo(dest); err != nil {
		return nil, badRequest("copying the library: %v", err)
	}
	loc := folder
	if folder == info.Default {
		loc = ""
	}
	if err := catalog.SetLibraryLocation(loc); err != nil {
		return nil, err
	}
	return map[string]any{"path": dest, "relaunch": true, "previous": info.Path}, nil
}

func (s *Server) optimiseLibrary(w http.ResponseWriter, r *http.Request) (any, error) {
	before, _ := s.db.Info()
	if err := s.db.Optimise(); err != nil {
		return nil, err
	}
	after, err := s.db.Info()
	if err != nil {
		return nil, err
	}
	return map[string]any{"before": before.Bytes, "after": after.Bytes}, nil
}

// clearLibrary removes every drive ("drives") or only the history
// ("history"). The drives themselves are never touched.
func (s *Server) clearLibrary(w http.ResponseWriter, r *http.Request) (any, error) {
	var body struct {
		What string `json:"what"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, badRequest("bad request: %v", err)
	}
	if s.jobs.AnyRunning() {
		return nil, badRequest("wait for the running job to finish first")
	}
	switch body.What {
	case "drives":
		if err := s.db.ForgetAll(); err != nil {
			return nil, err
		}
	case "history":
		if err := s.db.ClearHistory(); err != nil {
			return nil, err
		}
	default:
		return nil, badRequest("what must be drives or history")
	}
	s.mu.Lock()
	s.cacheKey = ""
	s.mu.Unlock()
	return map[string]bool{"ok": true}, nil
}

// rescanLoop rescans plugged-in drives on the interval in Prefs, as
// jobs, while the app is open.
func (s *Server) rescanLoop(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	var last time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		p := s.db.Prefs()
		if p.RescanMinutes <= 0 || time.Since(last) < time.Duration(p.RescanMinutes)*time.Minute {
			continue
		}
		last = time.Now()
		for _, v := range s.mounted() {
			d, err := s.db.Drive(v.ID)
			if err != nil || d.NeverLog || s.jobs.Busy(v.ID) {
				continue
			}
			v := v
			s.jobs.Start("rescan", "Rescan "+d.Title(), "lumpy catalog "+shellQuote(v.Mount), v.ID, func(ctx context.Context, rep *job.Report) error {
				_, err := job.CatalogDrive(ctx, s.db, v, rep)
				return err
			})
		}
	}
}
