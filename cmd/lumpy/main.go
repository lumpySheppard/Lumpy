// Command lumpy copies camera cards and shoot drives to several backup
// drives at once and proves every copy matches, writing ASC MHL manifests.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/lumpysheppard/lumpy/internal/job"
)

var version = "0.1.0"

const usage = `Lumpy copies camera cards and drives to backup drives and verifies every copy.

Usage:
  lumpy offload [--name NAME] [--report FILE] SOURCE DEST [DEST...]
      Copy SOURCE into a folder named after it on every DEST at once, read
      each copy back from its drive, compare xxh64 checksums with the source,
      and write an ASC MHL manifest into each copy. Re-running the same
      offload skips files already copied and verifies them again.

  lumpy verify [--report FILE] PATH
      Re-read every file listed in PATH's ascmhl folder (or a legacy .mhl
      file) and check it still matches.

  lumpy compare [--report FILE] A B
      Check two folders hold identical files, without needing a manifest.

  lumpy version

Exit status: 0 everything matched, 1 a file failed, 2 usage or setup error,
130 interrupted.
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

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	cmd, args := args[0], args[1:]
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	reportPath := fs.String("report", "", "also write the report to this file")
	name := ""
	if cmd == "offload" {
		fs.StringVar(&name, "name", "", "folder name to create in each destination")
	}

	switch cmd {
	case "version", "--version", "-v":
		fmt.Println("lumpy", version)
		return 0
	case "help", "--help", "-h":
		fmt.Print(usage)
		return 0
	case "offload", "verify", "compare":
	default:
		fmt.Fprintf(os.Stderr, "lumpy: unknown command %q\n\n%s", cmd, usage)
		return 2
	}

	pos, err := parse(fs, args)
	if err != nil {
		return 2
	}
	want := map[string]func(int) bool{
		"offload": func(n int) bool { return n >= 2 },
		"verify":  func(n int) bool { return n == 1 },
		"compare": func(n int) bool { return n == 2 },
	}
	if !want[cmd](len(pos)) {
		fmt.Fprintf(os.Stderr, "lumpy %s: wrong number of arguments\n\n%s", cmd, usage)
		return 2
	}

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
	r := job.NewReport(reportFile)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	host, _ := os.Hostname()
	switch cmd {
	case "offload":
		err = job.Offload(ctx, job.OffloadOptions{
			Source: pos[0], Dests: pos[1:], Name: name,
			Meta: job.Meta{Version: version, Hostname: host},
		}, r)
	case "verify":
		err = job.Verify(ctx, pos[0], r)
	case "compare":
		err = job.Compare(ctx, pos[0], pos[1], r)
	}

	switch {
	case errors.Is(err, context.Canceled):
		r.Info("Interrupted. Nothing half-written was left behind; run the same command again to finish.")
		return 130
	case err != nil:
		r.Info("lumpy: %v", err)
		return 2
	case r.Failures() > 0:
		r.Info("%d problem(s) found.", r.Failures())
		return 1
	}
	return 0
}
