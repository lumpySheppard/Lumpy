package ui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lumpysheppard/lumpy/internal/catalog"
)

// Keep-awake, after Amphetamine's Drive Alive: a spinning drive that
// nothing touches for a few minutes is spun down by macOS (or by its
// own firmware), and the next read waits five or ten seconds for it to
// come back, or finds it gone. While the preference is on, Lumpy
// writes and removes a tiny file at the root of every plugged-in
// spinning drive on a timer, so none of them ever go idle long enough.
// SSDs and cards don't spin, so they are left alone, as is any drive
// whose label says to let it sleep, a drive that is busy with a job
// (it's awake), and a drive that is read-only.

// awakeFile is the file written to a drive's root and removed again.
const awakeFile = ".lumpy-awake"

type awake struct {
	mu    sync.Mutex
	at    map[string]time.Time // drive id -> last touch
	caff  *exec.Cmd            // caffeinate, while KeepMacAwake is on
	kicks chan struct{}
}

// awakeLoop touches the drives on the preference's interval, and runs
// caffeinate while the Mac is to be kept awake.
func (s *Server) awakeLoop(ctx context.Context) {
	s.awk.kicks = make(chan struct{}, 1)
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	defer s.caffeinate(false)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-s.awk.kicks:
		}
		p := s.db.Prefs()
		s.caffeinate(p.KeepAwake && p.KeepMacAwake)
		if !p.KeepAwake {
			s.awk.mu.Lock()
			s.awk.at = nil
			s.awk.mu.Unlock()
			continue
		}
		every := time.Duration(p.KeepAwakeMinutes) * time.Minute
		if every < time.Minute {
			every = time.Minute
		}
		for _, v := range s.mounted() {
			d, err := s.db.Drive(v.ID)
			if err != nil || !keepAwake(d) || isCard(v.Mount, d.Bus) {
				continue
			}
			s.awk.mu.Lock()
			last := s.awk.at[v.ID]
			s.awk.mu.Unlock()
			if time.Since(last) < every {
				continue
			}
			if s.jobs.Busy(v.ID) || touch(v.Mount) != nil {
				// Busy drives are awake anyway; a drive that can't be
				// written is left alone until next time.
				continue
			}
			s.awk.mu.Lock()
			if s.awk.at == nil {
				s.awk.at = map[string]time.Time{}
			}
			s.awk.at[v.ID] = time.Now()
			s.awk.mu.Unlock()
		}
	}
}

// keepAwake says whether a logged drive is one to keep spinning: a
// hard drive or RAID, not opted out.
func keepAwake(d *catalog.Drive) bool {
	if d.LetSleep || d.NeverLog || d.SolidState {
		return false
	}
	switch d.Icon {
	case "ssd", "card":
		return false
	}
	return true
}

// isCard spots a camera card the way the page does: by its reader's
// bus, or a camera's folder layout at the root. Cards don't spin, and
// writing to one is unwelcome anyway.
func isCard(mount, bus string) bool {
	b := strings.ToLower(bus)
	if strings.Contains(b, "secure digital") || strings.Contains(b, "sd card") || strings.Contains(b, "cfexpress") || strings.Contains(b, "cfast") {
		return true
	}
	for _, top := range []string{"DCIM", "PRIVATE", "CLIPS", "XDROOT", "BPAV", "M4ROOT", "AVCHD", "CONTENTS"} {
		if st, err := os.Stat(filepath.Join(mount, top)); err == nil && st.IsDir() {
			return true
		}
	}
	return false
}

// touch writes a few bytes to a file at the drive's root, forces them
// to the platters, and removes the file again.
func touch(mount string) error {
	path := filepath.Join(mount, awakeFile)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	_, werr := f.WriteString(time.Now().Format(time.RFC3339) + "\n")
	serr := f.Sync()
	cerr := f.Close()
	os.Remove(path)
	for _, e := range []error{werr, serr, cerr} {
		if e != nil {
			return e
		}
	}
	return nil
}

// caffeinate starts or stops macOS's caffeinate, which holds the Mac
// out of idle sleep (the display may still sleep) while it runs.
func (s *Server) caffeinate(on bool) {
	s.awk.mu.Lock()
	defer s.awk.mu.Unlock()
	if on == (s.awk.caff != nil) {
		return
	}
	if !on {
		s.awk.caff.Process.Kill()
		s.awk.caff.Wait()
		s.awk.caff = nil
		return
	}
	if _, err := exec.LookPath("caffeinate"); err != nil {
		return
	}
	// -i: no idle sleep; -w: only while this process lives, so a crash
	// never leaves the Mac held awake.
	c := exec.Command("caffeinate", "-i", "-w", itoa(os.Getpid()))
	if c.Start() == nil {
		s.awk.caff = c
	}
}

// kickAwake applies a preference change without waiting for the tick.
func (s *Server) kickAwake() {
	select {
	case s.awk.kicks <- struct{}{}:
	default:
	}
}

// awakeStatus is the keep-awake summary the page shows.
func (s *Server) awakeStatus() map[string]any {
	p := s.db.Prefs()
	s.awk.mu.Lock()
	defer s.awk.mu.Unlock()
	ids := []string{}
	var last time.Time
	for id, t := range s.awk.at {
		ids = append(ids, id)
		if t.After(last) {
			last = t
		}
	}
	return map[string]any{"on": p.KeepAwake, "mac": p.KeepAwake && p.KeepMacAwake && s.awk.caff != nil, "drives": ids, "last": nullTime(last)}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.Unix()
}
