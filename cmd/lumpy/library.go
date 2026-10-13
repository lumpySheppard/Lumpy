package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/lumpysheppard/lumpy/internal/catalog"
	"github.com/lumpysheppard/lumpy/internal/job"
	"github.com/lumpysheppard/lumpy/internal/volume"
)

func day(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return t.Local().Format("2006-01-02 15:04")
}

// mountedAs returns the drive's volume if it is plugged in now.
func mountedAs(d *catalog.Drive) *volume.Volume {
	if d.LastMount != "" {
		if v, err := volume.Of(d.LastMount); err == nil && v.ID == d.ID && v.Mount == d.LastMount {
			return v
		}
	}
	vols, _ := volume.Mounted()
	for _, v := range vols {
		if v.ID == d.ID {
			return v
		}
	}
	return nil
}

func checkState(d *catalog.Drive) string {
	switch {
	case !d.VerifyRequested.IsZero():
		return "queued"
	case d.LastVerify.IsZero():
		return "never checked"
	case d.LastVerifyOK:
		return "OK " + d.LastVerify.Local().Format("2006-01-02")
	default:
		return "FAILED " + d.LastVerify.Local().Format("2006-01-02")
	}
}

func listDrives(c *cmdCtx) error {
	ds, err := c.db.Drives()
	if err != nil {
		return err
	}
	if len(ds) == 0 {
		c.r.Info("The library is empty. Plug in a drive with the agent installed, or run `lumpy catalog`.")
		return nil
	}
	var b strings.Builder
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tID\tSIZE\tFILES\tDATA\tCHECKSUMS\tLAST SEEN\tLAST CHECK\t")
	for _, d := range ds {
		pct := 0
		if d.Files > 0 {
			pct = d.Hashed * 100 / d.Files
		}
		seen := day(d.LastSeen)
		if mountedAs(d) != nil {
			seen = "plugged in"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%d%%\t%s\t%s\t\n", d.Title(), shortID(d.ID), job.Bytes(d.Size), d.Files, job.Bytes(d.Bytes), pct, seen, checkState(d))
	}
	tw.Flush()
	c.r.Info("%s", strings.TrimRight(b.String(), "\n"))
	return nil
}

func shortID(id string) string {
	if len(id) > 8 && !strings.HasPrefix(id, "name:") {
		return id[:8]
	}
	return id
}

func driveCmd(ctx context.Context, c *cmdCtx) error {
	if len(c.args) == 1 {
		d, err := c.db.Find(c.args[0])
		if err != nil {
			return err
		}
		return driveInfo(c, d)
	}
	action, arg := c.args[0], c.args[1]
	d, err := c.db.Find(arg)
	if err != nil {
		return err
	}
	switch action {
	case "verify", "hash":
		v := mountedAs(d)
		if v == nil {
			if action == "hash" {
				return fmt.Errorf("%s is not plugged in", d.Name)
			}
			if err := c.db.RequestVerify(d.ID, true); err != nil {
				return err
			}
			c.db.Log(d.ID, "queued", "full check requested", 0)
			c.r.Info("%s is not plugged in. A full check is queued and will run the next time", d.Name)
			c.r.Info("it is mounted (needs the agent: `lumpy agent status`).")
			c.r.Info("")
			c.r.Info("Meanwhile, from the library:")
			return job.Backups(c.db, d, 1, c.r)
		}
		return job.HashDrive(ctx, c.db, v, action == "verify", c.r)
	case "cancel":
		if err := c.db.RequestVerify(d.ID, false); err != nil {
			return err
		}
		c.r.Info("Queued check of %s cancelled.", d.Name)
		return nil
	case "forget":
		if err := c.db.Forget(d.ID); err != nil {
			return err
		}
		c.r.Info("%s removed from the library.", d.Name)
		return nil
	}
	return usageErr("lumpy drive [verify|hash|cancel|forget] DRIVE")
}

func driveInfo(c *cmdCtx, d *catalog.Drive) error {
	c.r.Info("%s", d.Title())
	c.r.Info("  volume       %s", d.Name)
	c.r.Info("  id           %s", d.ID)
	c.r.Info("  capacity     %s  %s", job.Bytes(d.Size), d.FSType)
	c.r.Info("  files        %d (%s), %d with checksums", d.Files, job.Bytes(d.Bytes), d.Hashed)
	if d.Damaged > 0 {
		c.r.Info("  damaged      %d file(s) failed their last check (`lumpy backups %s` lists them)", d.Damaged, d.Name)
	}
	c.r.Info("  first seen   %s", day(d.FirstSeen))
	if v := mountedAs(d); v != nil {
		c.r.Info("  now          plugged in at %s", v.Mount)
	} else {
		c.r.Info("  last seen    %s at %s", day(d.LastSeen), d.LastMount)
	}
	c.r.Info("  last check   %s", checkState(d))
	if !d.VerifyRequested.IsZero() {
		c.r.Info("  queued       full check requested %s; runs when next plugged in", day(d.VerifyRequested))
	}
	evs, err := c.db.Events(d.ID, 15)
	if err != nil {
		return err
	}
	if len(evs) > 0 {
		c.r.Info("")
		c.r.Info("History (newest first)")
		for _, e := range evs {
			mark := ""
			if e.Failures > 0 {
				mark = "  FAIL"
			}
			c.r.Info("  %s  %-9s %s%s", day(e.At), e.Kind, e.Summary, mark)
		}
	}
	return nil
}

func findCmd(ctx context.Context, c *cmdCtx) error {
	ms, err := c.db.Search(c.args[0], 500)
	if err != nil {
		return err
	}
	if len(ms) == 0 {
		c.r.Info("No file named like %q in the library.", c.args[0])
		return nil
	}
	var b strings.Builder
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "DRIVE\tPATH\tSIZE\tXXH64\t")
	for _, m := range ms {
		h := m.XXH64
		if h == "" {
			h = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t\n", m.DriveName, m.Path, job.Bytes(m.Size), h)
	}
	tw.Flush()
	c.r.Info("%s", strings.TrimRight(b.String(), "\n"))
	if len(ms) == 500 {
		c.r.Info("(first 500 matches)")
	}
	return nil
}

func catalogCmd(ctx context.Context, c *cmdCtx) error {
	var vols []*volume.Volume
	if len(c.args) == 0 {
		var err error
		if vols, err = volume.Mounted(); err != nil {
			return err
		}
		if len(vols) == 0 {
			c.r.Info("No external drives are mounted.")
			return nil
		}
	}
	for _, a := range c.args {
		if _, err := os.Stat(a); err != nil {
			return err
		}
		v, err := volume.Of(a)
		if err != nil {
			return err
		}
		vols = append(vols, v)
	}
	for _, v := range vols {
		if _, err := job.CatalogDrive(ctx, c.db, v, c.r); err != nil {
			return err
		}
	}
	return nil
}
