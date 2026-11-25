package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/lumpysheppard/lumpy/internal/update"
)

// updateCmd checks GitHub Releases for a newer Lumpy and installs it:
// the whole Lumpy.app when run from the app's own binary, else just the
// lumpy binary. `lumpy update check` only looks.
func updateCmd(ctx context.Context, c *cmdCtx) error {
	token := c.db.Setting("update.token", "")
	if len(c.args) == 2 && c.args[0] == "token" {
		if err := c.db.SetSetting("update.token", strings.TrimSpace(c.args[1])); err != nil {
			return err
		}
		c.r.Info("GitHub token saved. It is only used to read releases of %s.", update.Repo)
		return nil
	}
	rel, err := update.Check(ctx, token)
	if err != nil {
		return fmt.Errorf("checking for updates: %w", err)
	}
	if !update.Newer(rel.Version, version) {
		c.r.Info("OK    Lumpy %s is the latest version (released %s).", version, rel.PublishedAt.Local().Format("2 Jan 2006"))
		return nil
	}
	c.r.Info("Lumpy %s is available (you have %s), released %s: %s", rel.Version, version, rel.PublishedAt.Local().Format("2 Jan 2006"), rel.URL)
	if len(c.args) == 1 && c.args[0] == "check" {
		return nil
	}
	target, err := update.Detect()
	if err != nil {
		return err
	}
	c.r.Info("Installing over %s", target.Describe())
	if err := update.Install(ctx, rel, token, target, func(line string) { c.r.Info("%s", line) }); err != nil {
		return err
	}
	what := "lumpy"
	if target.Bundle != "" {
		what = "Lumpy.app"
	}
	c.r.Info("OK    %s %s is installed. Quit and reopen Lumpy to run it.", what, rel.Version)
	return nil
}
