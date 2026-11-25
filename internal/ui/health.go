package ui

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/lumpysheppard/lumpy/internal/smart"
	"github.com/lumpysheppard/lumpy/internal/volume"
)

// healthEvery is how long a SMART reading is trusted before a
// plugged-in drive is asked again.
const healthEvery = 24 * time.Hour

var healthMu sync.Mutex
var healthBusy = map[string]bool{}

// refreshHealth reads SMART, in the background, for every plugged-in
// logged drive whose last reading is older than a day. Nothing happens
// without smartmontools.
func (s *Server) refreshHealth(ctx context.Context) {
	if smart.Program() == "" {
		return
	}
	for _, v := range s.mounted() {
		d, err := s.db.Drive(v.ID)
		if err != nil || d.NeverLog || v.Disk == "" {
			continue
		}
		if d.Health != nil && time.Since(time.Unix(d.Health.ReadAt, 0)) < healthEvery {
			continue
		}
		v := v
		go s.readHealth(ctx, v)
	}
}

// readHealth takes one SMART reading of a volume's disk and keeps it;
// one at a time per drive.
func (s *Server) readHealth(ctx context.Context, v *volume.Volume) *smart.Health {
	healthMu.Lock()
	if healthBusy[v.ID] {
		healthMu.Unlock()
		return nil
	}
	healthBusy[v.ID] = true
	healthMu.Unlock()
	defer func() {
		healthMu.Lock()
		delete(healthBusy, v.ID)
		healthMu.Unlock()
	}()
	h := smart.Read(ctx, v.Disk)
	s.db.SetHealth(v.ID, h)
	return h
}

// health reads a plugged-in drive's SMART now, for the sheet's button.
func (s *Server) health(w http.ResponseWriter, r *http.Request) (any, error) {
	d, err := s.drive(r)
	if err != nil {
		return nil, err
	}
	v := s.mountOf(d.ID)
	if v == nil {
		return nil, badRequest("%s is not plugged in", d.Title())
	}
	h := s.readHealth(r.Context(), v)
	if h == nil {
		return nil, badRequest("%s is being read already", d.Title())
	}
	return h, nil
}
