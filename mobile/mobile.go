// Package mobile is Lumpy's engine inside Lumpy for iPad: the same
// server the Mac app talks to, started in the app's own process and
// reached over the loopback by the app's web view. gomobile binds it
// into a framework the Swift shell (ios/) calls.
package mobile

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	_ "golang.org/x/mobile/bind" // keeps the binding runtime in the module

	"github.com/lumpysheppard/lumpy/internal/catalog"
	"github.com/lumpysheppard/lumpy/internal/job"
	"github.com/lumpysheppard/lumpy/internal/ui"
)

var (
	mu     sync.Mutex
	db     *catalog.DB
	srv    *ui.Server
	hs     *http.Server
	ln     net.Listener
	cancel context.CancelFunc
	url    string
)

// Version is stamped by the build.
var Version = "dev"

// Start opens the library under home (the app's Documents folder) and
// serves the app on the loopback. It returns the URL, token included,
// for the web view. Starting again returns the same URL.
func Start(home string) (string, error) {
	mu.Lock()
	defer mu.Unlock()
	if srv != nil {
		return url, nil
	}
	if home == "" {
		return "", errors.New("no home folder")
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		return "", err
	}
	d, err := catalog.Open(filepath.Join(home, "catalog.db"))
	if err != nil {
		return "", err
	}
	s := ui.New(d, filepath.Join(home, "agent.lock"), job.Meta{Version: Version, Hostname: "iPad"})
	s.Mobile = true
	u, h, l, err := s.Listen(0)
	if err != nil {
		d.Close()
		return "", err
	}
	ctx, c := context.WithCancel(context.Background())
	db, srv, hs, ln, cancel, url = d, s, h, l, c, u
	go h.Serve(l)
	go s.WatchMobile(ctx)
	return u, nil
}

// AddPlace registers a folder the app was granted (through the Files
// picker) as a place the Space tab can measure. It is remembered by
// name; the app grants it again each launch.
func AddPlace(name, path string) error {
	mu.Lock()
	s := srv
	mu.Unlock()
	if s == nil {
		return errors.New("not started")
	}
	return s.AddPlace(name, path)
}

// Stop shuts the server and closes the library.
func Stop() {
	mu.Lock()
	defer mu.Unlock()
	if srv == nil {
		return
	}
	cancel()
	hs.Close()
	db.Close()
	db, srv, hs, ln, cancel, url = nil, nil, nil, nil, nil, ""
}
