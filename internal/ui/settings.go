package ui

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/lumpysheppard/lumpy/internal/publish"
)

// publisher keeps the status page fresh: on the chosen interval while
// the app is open, after every job, and when the app quits.
type publisher struct {
	mu      sync.Mutex
	running bool
	pending bool // a publish was asked for while one ran
	wake    chan struct{}
}

func (s *Server) settingsView() map[string]any {
	st := publish.Load(s.db)
	info, _ := s.db.Info()
	return map[string]any{
		"prefs":         s.db.Prefs(),
		"library":       info,
		"publish":       st,
		"last":          publish.Last(s.db),
		"defaultFolder": publish.DefaultFolder(),
		"publishing":    s.pub.busy(),
	}
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) (any, error) {
	return s.settingsView(), nil
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) (any, error) {
	var body publish.Settings
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, badRequest("bad request: %v", err)
	}
	body.URL = strings.TrimSpace(body.URL)
	if body.URL != "" && !strings.HasPrefix(body.URL, "http://") && !strings.HasPrefix(body.URL, "https://") {
		return nil, badRequest("the upload URL must start with http:// or https://")
	}
	if body.Interval < 0 {
		body.Interval = 0
	}
	if strings.TrimSpace(body.Title) == "" {
		body.Title = "Lumpy status"
	}
	if err := publish.Save(s.db, body); err != nil {
		return nil, err
	}
	s.pub.poke()
	return s.settingsView(), nil
}

// publishNow runs a publish right away, whether or not it is enabled,
// and reports how it went.
func (s *Server) publishNow(w http.ResponseWriter, r *http.Request) (any, error) {
	res, err := s.publishOnce(r.Context(), true)
	if err != nil {
		return nil, badRequest("%v", err)
	}
	return map[string]any{"result": res, "settings": s.settingsView()}, nil
}

func (p *publisher) busy() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.running
}

// poke restarts the publish loop's timer, after settings change.
func (p *publisher) poke() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// publishOnce publishes if enabled (or when forced), one at a time.
func (s *Server) publishOnce(ctx context.Context, force bool) (*publish.Result, error) {
	st := publish.Load(s.db)
	if !st.Enabled && !force {
		return nil, nil
	}
	s.pub.mu.Lock()
	if s.pub.running {
		// Whatever changed is picked up by a second run afterwards.
		s.pub.pending = true
		s.pub.mu.Unlock()
		return nil, nil
	}
	s.pub.running = true
	s.pub.mu.Unlock()
	var res publish.Result
	var err error
	for {
		res, err = publish.Run(ctx, s.db, s.meta.Hostname, s.meta.Version, st)
		s.pub.mu.Lock()
		again := s.pub.pending && ctx.Err() == nil
		s.pub.pending = false
		if !again {
			s.pub.running = false
		}
		s.pub.mu.Unlock()
		if !again {
			return &res, err
		}
		st = publish.Load(s.db)
	}
}

// publishLoop publishes on the interval until ctx ends, then once more
// so the page shows the state the app was closed in.
func (s *Server) publishLoop(ctx context.Context) {
	for {
		st := publish.Load(s.db)
		var tick <-chan time.Time
		var t *time.Timer
		if st.Enabled && st.Interval > 0 {
			t = time.NewTimer(time.Duration(st.Interval) * time.Minute)
			tick = t.C
		}
		select {
		case <-ctx.Done():
			if t != nil {
				t.Stop()
			}
			final, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			s.publishOnce(final, false)
			cancel()
			close(s.pubDone)
			return
		case <-tick:
			s.publishOnce(ctx, false)
		case <-s.pub.wake:
			if t != nil {
				t.Stop()
			}
		}
	}
}

// Wait blocks until the final publish after ctx ended has finished, so
// the process can exit cleanly.
func (s *Server) Wait() {
	<-s.pubDone
}
