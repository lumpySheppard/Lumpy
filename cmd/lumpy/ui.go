package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/lumpysheppard/lumpy/internal/catalog"
	"github.com/lumpysheppard/lumpy/internal/job"
	"github.com/lumpysheppard/lumpy/internal/ui"
)

// uiCmd serves the app until interrupted. In --app mode (started by
// Lumpy.app) it prints the URL as its first line and stops when the app
// closes its standard input.
func uiCmd(ctx context.Context, c *cmdCtx) error {
	p, err := catalog.DefaultPath()
	if err != nil {
		return err
	}
	srv := ui.New(c.db, filepath.Join(filepath.Dir(p), "agent.lock"), job.Meta{Version: version, Hostname: c.host})
	srv.App = c.appMode
	url, hs, ln, err := srv.Listen(c.port)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if c.appMode {
		fmt.Println("LUMPY_URL " + url)
		go func() {
			io.Copy(io.Discard, os.Stdin)
			cancel()
		}()
	} else {
		c.r.Info("Lumpy is running at %s", url)
		c.r.Info("Press Ctrl-C to stop.")
		if !c.noBrowser {
			openBrowser(url)
		}
	}
	go srv.Watch(ctx)
	go func() {
		<-ctx.Done()
		hs.Close()
	}()
	if err := hs.Serve(ln); err != nil && ctx.Err() == nil {
		return err
	}
	// The status page is published once more on the way out, so it
	// shows the state the app was closed in.
	srv.Wait()
	return nil
}

func openBrowser(url string) {
	switch runtime.GOOS {
	case "darwin":
		exec.Command("open", url).Start()
	case "linux":
		exec.Command("xdg-open", url).Start()
	}
}
