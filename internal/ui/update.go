package ui

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/lumpysheppard/lumpy/internal/job"
	"github.com/lumpysheppard/lumpy/internal/update"
)

// updater remembers the last release check and whether a new version
// has been installed and waits for a restart.
type updater struct {
	mu        sync.Mutex
	latest    *update.Release
	checkedAt time.Time
	err       string
	installed string // version installed this session, pending restart
	checking  bool
}

// UpdateView is what the app shows about updates.
type UpdateView struct {
	Current    string          `json:"current"`
	Auto       bool            `json:"auto"`
	HasToken   bool            `json:"hasToken"`
	Latest     *update.Release `json:"latest"`
	Available  bool            `json:"available"` // latest is newer than current
	CheckedAt  int64           `json:"checkedAt"`
	Error      string          `json:"error,omitempty"`
	Installed  string          `json:"installed,omitempty"` // restart to run this version
	Checking   bool            `json:"checking"`
	Installing bool            `json:"installing"`
	Target     string          `json:"target"` // what an update replaces
}

// updateSummary is the little the state poll carries: a version to
// offer, or one installed and waiting for a restart.
func (s *Server) updateSummary() map[string]any {
	s.upd.mu.Lock()
	defer s.upd.mu.Unlock()
	out := map[string]any{"installed": s.upd.installed}
	if s.upd.latest != nil && update.Newer(s.upd.latest.Version, s.meta.Version) && s.upd.latest.Version != s.upd.installed {
		out["available"] = s.upd.latest.Version
	}
	return out
}

func (s *Server) updateView() UpdateView {
	s.upd.mu.Lock()
	defer s.upd.mu.Unlock()
	v := UpdateView{Current: s.meta.Version, Auto: s.db.Setting("update.auto", "on") != "off", HasToken: s.db.Setting("update.token", "") != "",
		Latest: s.upd.latest, Error: s.upd.err, Installed: s.upd.installed, Checking: s.upd.checking, Installing: s.jobs.Running("update")}
	if !s.upd.checkedAt.IsZero() {
		v.CheckedAt = s.upd.checkedAt.Unix()
	}
	if v.Latest != nil {
		v.Available = update.Newer(v.Latest.Version, s.meta.Version) && v.Latest.Version != s.upd.installed
	}
	if t, err := update.Detect(); err == nil {
		v.Target = t.Describe()
	}
	return v
}

// checkUpdate asks GitHub for the latest release and remembers the answer.
func (s *Server) checkUpdate(ctx context.Context) UpdateView {
	s.upd.mu.Lock()
	if s.upd.checking {
		s.upd.mu.Unlock()
		return s.updateView()
	}
	s.upd.checking = true
	s.upd.mu.Unlock()
	rel, err := update.Check(ctx, s.db.Setting("update.token", ""))
	s.upd.mu.Lock()
	s.upd.checking = false
	s.upd.checkedAt = time.Now()
	if err != nil {
		s.upd.err = err.Error()
	} else {
		s.upd.err = ""
		s.upd.latest = rel
	}
	s.upd.mu.Unlock()
	return s.updateView()
}

// updateLoop checks shortly after launch and then twice a day, while
// automatic checks are on.
func (s *Server) updateLoop(ctx context.Context) {
	delay := 15 * time.Second
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		if s.db.Setting("update.auto", "on") != "off" {
			s.checkUpdate(ctx)
		}
		delay = 12 * time.Hour
	}
}

func (s *Server) getUpdate(w http.ResponseWriter, r *http.Request) (any, error) {
	return s.updateView(), nil
}

func (s *Server) putUpdateSettings(w http.ResponseWriter, r *http.Request) (any, error) {
	var body struct {
		Auto  *bool   `json:"auto"`
		Token *string `json:"token"` // "" clears it; absent keeps it
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, badRequest("bad request: %v", err)
	}
	if body.Auto != nil {
		v := "on"
		if !*body.Auto {
			v = "off"
		}
		if err := s.db.SetSetting("update.auto", v); err != nil {
			return nil, err
		}
	}
	if body.Token != nil {
		if err := s.db.SetSetting("update.token", strings.TrimSpace(*body.Token)); err != nil {
			return nil, err
		}
	}
	return s.updateView(), nil
}

func (s *Server) checkUpdateNow(w http.ResponseWriter, r *http.Request) (any, error) {
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	return s.checkUpdate(ctx), nil
}

// installUpdate downloads and installs the latest release as a job, so
// Activity shows its progress. The new version runs after a restart.
func (s *Server) installUpdate(w http.ResponseWriter, r *http.Request) (any, error) {
	v := s.updateView()
	if v.Latest == nil {
		return nil, badRequest("check for updates first")
	}
	if !v.Available {
		return nil, badRequest("Lumpy %s is already the latest version", s.meta.Version)
	}
	if v.Installing {
		return nil, badRequest("an update is already being installed")
	}
	target, err := update.Detect()
	if err != nil {
		return nil, err
	}
	rel := v.Latest
	token := s.db.Setting("update.token", "")
	j := s.jobs.Start("update", "Update to Lumpy "+rel.Version, "lumpy update", "", func(ctx context.Context, rep *job.Report) error {
		rep.Info("Installing Lumpy %s over %s", rel.Version, target.Describe())
		if err := update.Install(ctx, rel, token, target, func(line string) { rep.Info("%s", line) }); err != nil {
			return err
		}
		s.upd.mu.Lock()
		s.upd.installed = rel.Version
		s.upd.mu.Unlock()
		rep.Info("OK    Lumpy %s is installed. Quit and reopen Lumpy to run it.", rel.Version)
		return nil
	})
	return map[string]any{"job": j.ID}, nil
}
