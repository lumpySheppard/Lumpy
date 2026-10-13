// Command lumpy copies camera cards and shoot drives to several backup
// drives at once, proves every copy matches, and keeps a library of every
// drive it has seen so backups can be checked while drives are on a shelf.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/lumpysheppard/lumpy/internal/agent"
	"github.com/lumpysheppard/lumpy/internal/catalog"
	"github.com/lumpysheppard/lumpy/internal/job"
)

var version = "0.3.0"

const usage = `Lumpy copies camera cards and drives to backup drives, verifies every copy,
and keeps a library of every drive it has seen.

Copying and checking:
  lumpy offload [--name NAME] SOURCE DEST [DEST...]
      Copy SOURCE into a folder named after it on every DEST at once, read
      each copy back from its drive, compare xxh64 checksums with the source,
      write an ASC MHL manifest into each copy, and record it all in the
      drive library. Re-running skips files already copied and checks them.
  lumpy verify PATH
      Re-check a folder against its ascmhl folder or a legacy .mhl file.
  lumpy compare A B
      Check two folders hold identical files, without needing a manifest.

Drive library (works for drives that are not plugged in):
  lumpy drives                    List every drive Lumpy has seen.
  lumpy drive DRIVE               Show a drive's details and history.
  lumpy backups [--min N] DRIVE   Check every file on DRIVE has N copies on
                                  other drives (default 1), from the library.
  lumpy find TEXT                 Find which drives hold files named like TEXT.
  lumpy drive verify DRIVE        Re-read every file and compare with the
                                  library. If DRIVE is not plugged in, the check
                                  is queued and runs when it is next mounted.
  lumpy drive cancel DRIVE        Cancel a queued check.
  lumpy drive hash DRIVE          Checksum the files on a plugged-in drive that
                                  have none yet.
  lumpy drive forget DRIVE        Remove a drive from the library.
  lumpy catalog [PATH...]         Log drives now (default: every mounted drive).

  DRIVE is a drive name, the start of its id, or a path on a mounted drive.

Background agent (logs every drive you plug in, runs queued checks):
  lumpy agent install | uninstall | status

App:
  lumpy ui [--port N] [--no-browser]
      Open Lumpy's drive library and offload screens in your browser.
      (Lumpy.app shows the same screens in its own window.)

Options:
  --report FILE   also write the output to FILE (any command)

Exit status: 0 all good, 1 a file failed or is not backed up, 2 usage or
setup error, 130 interrupted.
`

func main() {
	os.Exit(run(os.Args[1:]))
}

// parse reads flags that may appear before, between or after the arguments.
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

type command struct {
	min, max int // positional argument counts; max -1 means any
	library  bool
	run      func(ctx context.Context, c *cmdCtx) error
}

type cmdCtx struct {
	args []string
	r    *job.Report
	db   *catalog.DB
	name string
	min  int
	noDB bool
	host string

	port      int
	noBrowser bool
	appMode   bool
}

func usageErr(format string, a ...any) error {
	return fmt.Errorf("usage: "+format, a...)
}

var commands = map[string]command{
	"offload": {2, -1, true, func(ctx context.Context, c *cmdCtx) error {
		db := c.db
		if c.noDB {
			db = nil
		}
		return job.Offload(ctx, job.OffloadOptions{
			Source: c.args[0], Dests: c.args[1:], Name: c.name,
			Meta: job.Meta{Version: version, Hostname: c.host}, Library: db,
		}, c.r)
	}},
	"verify":  {1, 1, false, func(ctx context.Context, c *cmdCtx) error { return job.Verify(ctx, c.args[0], c.r) }},
	"compare": {2, 2, false, func(ctx context.Context, c *cmdCtx) error { return job.Compare(ctx, c.args[0], c.args[1], c.r) }},
	"drives":  {0, 0, true, func(ctx context.Context, c *cmdCtx) error { return listDrives(c) }},
	"drive":   {1, 2, true, driveCmd},
	"backups": {1, 1, true, func(ctx context.Context, c *cmdCtx) error {
		d, err := c.db.Find(c.args[0])
		if err != nil {
			return err
		}
		if c.min < 1 {
			return usageErr("--min must be at least 1")
		}
		return job.Backups(c.db, d, c.min, c.r)
	}},
	"find":    {1, 1, true, findCmd},
	"catalog": {0, -1, true, catalogCmd},
	"agent":   {1, 1, false, agentCmd},
	"ui":      {0, 0, true, uiCmd},
}

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	name, args := args[0], args[1:]
	switch name {
	case "version", "--version", "-v":
		fmt.Println("lumpy", version)
		return 0
	case "help", "--help", "-h":
		fmt.Print(usage)
		return 0
	}
	cmd, ok := commands[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "lumpy: unknown command %q\n\n%s", name, usage)
		return 2
	}

	c := &cmdCtx{}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	reportPath := fs.String("report", "", "also write the output to this file")
	fs.StringVar(&c.name, "name", "", "folder name to create in each destination (offload)")
	fs.IntVar(&c.min, "min", 1, "copies needed on other drives (backups)")
	fs.BoolVar(&c.noDB, "no-library", false, "do not record this offload in the drive library")
	fs.IntVar(&c.port, "port", 0, "port for `lumpy ui` (default: any free port)")
	fs.BoolVar(&c.noBrowser, "no-browser", false, "do not open a browser (ui)")
	fs.BoolVar(&c.appMode, "app", false, "run for Lumpy.app (ui)")
	pos, err := parse(fs, args)
	if err != nil {
		return 2
	}
	if len(pos) < cmd.min || (cmd.max >= 0 && len(pos) > cmd.max) {
		fmt.Fprintf(os.Stderr, "lumpy %s: wrong number of arguments\n\n%s", name, usage)
		return 2
	}
	c.args = pos

	var reportFile io.Writer
	if *reportPath != "" {
		f, err := os.Create(*reportPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "lumpy:", err)
			return 2
		}
		defer f.Close()
		reportFile = f
	}
	c.r = job.NewReport(reportFile)
	c.host, _ = os.Hostname()

	if cmd.library {
		p, err := catalog.DefaultPath()
		if err == nil {
			c.db, err = catalog.Open(p)
		}
		if err != nil {
			if name != "offload" {
				c.r.Info("lumpy: %v", err)
				return 2
			}
			c.r.Warn("drive library unavailable, this offload will not be recorded: %v", err)
			c.noDB = true
		} else {
			defer c.db.Close()
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err = cmd.run(ctx, c)

	switch {
	case errors.Is(err, context.Canceled):
		c.r.Info("Interrupted. Nothing half-written was left behind; run the same command again to finish.")
		return 130
	case err != nil:
		c.r.Info("lumpy: %v", err)
		return 2
	case c.r.Failures() > 0:
		c.r.Info("%d problem(s) found.", c.r.Failures())
		return 1
	}
	return 0
}

func agentCmd(ctx context.Context, c *cmdCtx) error {
	switch c.args[0] {
	case "install":
		bin, err := os.Executable()
		if err == nil {
			bin, err = filepath.EvalSymlinks(bin)
		}
		if err != nil {
			return err
		}
		env := map[string]string{}
		if h := os.Getenv("LUMPY_HOME"); h != "" {
			env["LUMPY_HOME"] = h
		}
		p, err := agent.Install(bin, env)
		if err != nil {
			return err
		}
		c.r.Info("Installed %s", p)
		c.r.Info("Every drive you plug in will now be logged, and queued checks will run.")
		c.r.Info("If macOS asks whether lumpy may access removable volumes, allow it. To stop")
		c.r.Info("it asking, add %s under System Settings > Privacy & Security > Full Disk Access.", bin)
		return nil
	case "uninstall":
		if err := agent.Uninstall(); err != nil {
			return err
		}
		c.r.Info("Agent removed. The drive library is kept.")
		return nil
	case "status":
		c.r.Info("Agent: %s", agent.Status())
		return nil
	case "run":
		p, err := catalog.DefaultPath()
		if err != nil {
			return err
		}
		unlock, ok, err := agent.Lock(filepath.Join(filepath.Dir(p), "agent.lock"))
		if err != nil {
			return err
		}
		if !ok {
			return nil // another run is working and will pick up this mount
		}
		defer unlock()
		db, err := catalog.Open(p)
		if err != nil {
			return err
		}
		defer db.Close()
		return job.AgentRun(ctx, db, c.r, agent.Notify)
	}
	return usageErr("lumpy agent install | uninstall | status")
}
