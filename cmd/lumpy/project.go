package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/lumpysheppard/lumpy/internal/catalog"
	"github.com/lumpysheppard/lumpy/internal/job"
	"github.com/lumpysheppard/lumpy/internal/volume"
)

// projectCmd handles `lumpy project ...`: the same health, plan and
// transfer steps the app shows, for the terminal.
func projectCmd(ctx context.Context, c *cmdCtx) error {
	switch c.args[0] {
	case "list":
		return projectList(c)
	case "show":
		if len(c.args) != 2 {
			return usageErr("lumpy project show NAME")
		}
		p, err := findProject(c.db, c.args[1])
		if err != nil {
			return err
		}
		return projectShow(c, p)
	case "transfer":
		if len(c.args) != 2 || c.from == "" || c.to == "" {
			return usageErr("lumpy project transfer NAME --from DRIVE --to DRIVE [--folder NAME]")
		}
		p, err := findProject(c.db, c.args[1])
		if err != nil {
			return err
		}
		return projectTransfer(ctx, c, p)
	}
	return usageErr("lumpy project list | show NAME | transfer NAME --from DRIVE --to DRIVE")
}

func findProject(db *catalog.DB, name string) (*catalog.Project, error) {
	ps, err := db.Projects()
	if err != nil {
		return nil, err
	}
	for _, p := range ps {
		if strings.EqualFold(p.Name, name) {
			return p, nil
		}
	}
	return nil, fmt.Errorf("no project called %q (see `lumpy project list`)", name)
}

// projectSpaces lists where a project's backups may go (its chosen
// destinations, or every drive in the library), with live free space for
// drives that are plugged in, plus every drive by id.
func projectSpaces(db *catalog.DB, p *catalog.Project) ([]catalog.DriveSpace, map[string]*catalog.Drive, map[string]*volume.Volume) {
	mounted := map[string]*volume.Volume{}
	if vols, err := volume.Mounted(); err == nil {
		for _, v := range vols {
			mounted[v.ID] = v
		}
	}
	// Copies stay inside the project, as in the app.
	allowed := map[string]bool{}
	for _, id := range p.BackupDrives() {
		allowed[id] = true
	}
	if len(allowed) == 0 {
		for _, id := range p.DestIDs {
			allowed[id] = true
		}
	}
	if len(allowed) == 0 {
		for _, id := range p.DriveIDs() {
			allowed[id] = true
		}
	}
	drives := map[string]*catalog.Drive{}
	var spaces []catalog.DriveSpace
	all, _ := db.Drives()
	for _, d := range all {
		drives[d.ID] = d
		if len(allowed) > 0 && !allowed[d.ID] || len(allowed) == 0 && d.NoBackups {
			continue
		}
		sp := catalog.DriveSpace{ID: d.ID, Free: d.Free}
		if v := mounted[d.ID]; v != nil {
			sp.Free, sp.Connected = v.Free, true
		}
		spaces = append(spaces, sp)
	}
	return spaces, drives, mounted
}

func projectLayout(p *catalog.Project, drives map[string]*catalog.Drive) func(catalog.Origin) string {
	return catalog.ProjectLayout(p.Sources, func(id string) string {
		if d := drives[id]; d != nil {
			return d.Title()
		}
		return id
	})
}

func projectList(c *cmdCtx) error {
	ps, err := c.db.Projects()
	if err != nil {
		return err
	}
	if len(ps) == 0 {
		c.r.Info("No projects yet. Make one in the app, or group drives there with Select.")
		return nil
	}
	for _, p := range ps {
		h, err := c.db.ProjectHealth(p.Sources, p.Target)
		if err != nil {
			return err
		}
		pct := 0
		if h.Bytes > 0 {
			pct = int(h.BackedBytes * 100 / h.Bytes)
		}
		c.r.Info("%-28s %d folders  %3d%% backed up  %s sole copies  %s unproven", p.Name, len(p.Sources), pct, job.Bytes(h.SoleBytes), job.Bytes(h.UnknownBytes))
	}
	return nil
}

func projectShow(c *cmdCtx, p *catalog.Project) error {
	h, err := c.db.ProjectHealth(p.Sources, p.Target)
	if err != nil {
		return err
	}
	spaces, drives, mounted := projectSpaces(c.db, p)
	title := func(id string) string {
		if d := drives[id]; d != nil {
			return d.Title()
		}
		return id
	}
	c.r.Info("%s  (every file wanted on %d drives)", p.Name, p.Target)
	c.r.Info("")
	c.r.Info("  backed up    %s  (%d files)", job.Bytes(h.BackedBytes), h.Backed)
	if p.Target > 2 {
		c.r.Info("  too few      %s  (%d files on 2 or more drives, fewer than %d)", job.Bytes(h.ShortBytes), h.Short, p.Target)
	}
	c.r.Info("  sole copies  %s  (%d files)", job.Bytes(h.SoleBytes), h.Sole)
	c.r.Info("  no checksum  %s  (%d files)", job.Bytes(h.UnknownBytes), h.Unknown)
	if h.Damaged > 0 {
		c.r.Info("  damaged      %d copies failed their last check", h.Damaged)
	}
	c.r.Info("")
	for _, sh := range h.Sources {
		state := "on shelf"
		if mounted[sh.DriveID] != nil {
			state = "plugged in"
		}
		where := title(sh.DriveID)
		if sh.Folder != "" {
			where += ":" + sh.Folder
		}
		c.r.Info("  %-40s %-10s %s, %s sole copies", where, state, job.Bytes(sh.Bytes), job.Bytes(sh.SoleBytes))
	}
	if len(p.DestIDs) > 0 {
		names := make([]string, len(p.DestIDs))
		for i, id := range p.DestIDs {
			names[i] = title(id)
		}
		c.r.Info("  backups go onto: %s", strings.Join(names, ", "))
	} else {
		c.r.Info("  backups go onto: the project's own drives")
	}
	plan := catalog.MakePlanRouted(h, spaces, projectLayout(p, drives), routeFor(p))
	c.r.Info("")
	if len(plan.Transfers) == 0 && plan.UnplacedBytes == 0 {
		c.r.Info("OK    nothing to copy: every checksummed file is on %d drives", p.Target)
	}
	for _, t := range plan.Transfers {
		ready := mounted[t.SourceID] != nil && mounted[t.DestID] != nil
		note := "plug both in"
		if ready {
			note = fmt.Sprintf("ready: lumpy project transfer %s --from %s --to %s", shellArg(p.Name), shellArg(title(t.SourceID)), shellArg(title(t.DestID)))
		}
		c.r.Info("  copy %s (%d files) from %s to %s  [%s]", job.Bytes(t.Bytes), len(t.Files), title(t.SourceID), title(t.DestID), note)
	}
	if plan.UnplacedBytes > 0 {
		c.r.Fail("%s has nowhere to go: no drive in the project has room. Add a drive with at least %s free", job.Bytes(plan.UnplacedBytes), job.Bytes(plan.UnplacedBytes+plan.Reserve))
	}
	return nil
}

func projectTransfer(ctx context.Context, c *cmdCtx, p *catalog.Project) error {
	h, err := c.db.ProjectHealth(p.Sources, p.Target)
	if err != nil {
		return err
	}
	spaces, drives, mounted := projectSpaces(c.db, p)
	pick := func(arg string) (*volume.Volume, error) {
		d, err := c.db.Find(arg)
		if err != nil {
			return nil, err
		}
		v := mounted[d.ID]
		if v == nil {
			return nil, fmt.Errorf("%s is not plugged in", d.Title())
		}
		return v, nil
	}
	src, err := pick(c.from)
	if err != nil {
		return err
	}
	dst, err := pick(c.to)
	if err != nil {
		return err
	}
	plan := catalog.MakePlanRouted(h, spaces, projectLayout(p, drives), routeFor(p))
	for _, t := range plan.Transfers {
		if t.SourceID == src.ID && t.DestID == dst.ID {
			folder := c.folder
			if t.Folder != "" {
				folder = t.Folder // the project's own folder on that drive
			} else if folder == "" {
				folder = p.Name
			}
			return job.Transfer(ctx, job.TransferOptions{Source: src, Dest: dst, Folder: folder, Files: t.Files,
				Meta: job.Meta{Version: version, Hostname: c.host}, Library: c.db, SkipSourceVerify: c.noSourceCheck}, c.r)
		}
	}
	return fmt.Errorf("the plan has nothing to copy from %s to %s (see `lumpy project show %s`)", drives[src.ID].Title(), drives[dst.ID].Title(), shellArg(p.Name))
}

// routeFor copies a folder's files into the project's own folder on the
// destination drive when it has one, keeping their relative paths.
func routeFor(p *catalog.Project) func(catalog.Origin, string) (string, bool) {
	return func(o catalog.Origin, destID string) (string, bool) {
		if o.Source < 0 || o.Source >= len(p.Sources) {
			return "", false
		}
		if r := p.RouteFor(p.Sources[o.Source], destID); r != nil {
			return r.Folder, true
		}
		return "", false
	}
}

// shellArg quotes an argument for pasting into a shell when needed.
func shellArg(s string) string {
	if s != "" && !strings.ContainsAny(s, " '\"$`\\!*?[]{}()<>|&;") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
