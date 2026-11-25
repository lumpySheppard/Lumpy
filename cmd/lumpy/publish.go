package main

import (
	"context"
	"fmt"

	"github.com/lumpysheppard/lumpy/internal/publish"
)

// publishCmd writes and sends the status page using the saved settings.
func publishCmd(ctx context.Context, c *cmdCtx) error {
	st := publish.Load(c.db)
	if st.Folder == "" && st.URL == "" && st.Command == "" && !st.Enabled {
		c.r.Info("Publishing is not set up yet; writing the page to %s. Open Settings in the app to send it somewhere.", publish.DefaultFolder())
	}
	res, err := publish.Run(ctx, c.db, c.host, version, st)
	if err != nil {
		return fmt.Errorf("publish: %w", err)
	}
	c.r.Info("OK    %s", res.Summary)
	c.r.Info("Page  %s", res.Path)
	return nil
}
