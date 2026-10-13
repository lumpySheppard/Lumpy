package job

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lumpysheppard/lumpy/internal/mhl"
	"github.com/lumpysheppard/lumpy/internal/scan"
)

// Verify re-reads every file a manifest lists and checks it still matches.
// target is a folder with an ascmhl folder, a folder holding legacy .mhl
// files, or a single .mhl file.
func Verify(ctx context.Context, target string, r *Report) error {
	hist, legacy, err := loadManifest(target)
	if err != nil {
		return err
	}
	root := hist.Root
	r.Info("Checking     %s", root)
	if legacy {
		r.Info("Manifest     %s (MHL v1)", strings.Join(hist.Generations, ", "))
	} else {
		r.Info("Manifest     ascmhl, %d generation(s)", len(hist.Generations))
	}
	for _, p := range hist.Problems {
		r.Fail("MHL history: %s", p)
	}

	res, err := scan.Dir(root)
	if err != nil {
		return err
	}
	onDisk := map[string]scan.File{}
	for _, f := range res.Files {
		onDisk[f.Path] = f
	}

	paths := make([]string, 0, len(hist.Hashes))
	var total int64
	for p := range hist.Hashes {
		paths = append(paths, p)
		total += onDisk[p].Size
	}
	sort.Strings(paths)

	r.Info("")
	prog := r.startProgress("verify", total)
	ok := 0
	for _, p := range paths {
		want, have := hist.Expected(p)
		if !have {
			r.Warn("%s: manifest has no checksum Lumpy can compute", p)
			continue
		}
		f, there := onDisk[p]
		if !there {
			r.Fail("%s is missing", p)
			continue
		}
		if want.Size >= 0 && want.Size != f.Size {
			r.Fail("%s is %d bytes; the manifest says %d", p, f.Size, want.Size)
			prog.Add(int(f.Size))
			continue
		}
		got, _, err := hashFile(ctx, filepath.Join(root, filepath.FromSlash(p)), want.Format, prog)
		if errors.Is(err, context.Canceled) {
			prog.Finish()
			return err
		}
		switch {
		case err != nil:
			r.Fail("%s: %v", p, err)
		case got != want.Hex:
			r.Fail("%s does not match (%s in manifest %s, now %s)", p, want.Format, want.Hex, got)
		default:
			ok++
		}
	}
	took := prog.Finish()

	for _, f := range res.Files {
		if _, listed := hist.Hashes[f.Path]; listed {
			continue
		}
		if legacy && !strings.Contains(f.Path, "/") && strings.HasSuffix(strings.ToLower(f.Path), ".mhl") {
			continue
		}
		r.Warn("%s is not in the manifest", f.Path)
	}

	r.Info("")
	r.Info("Checked %d files (%s) in %s", len(paths), Bytes(total), took.Round(time.Second))
	if r.Failures() == 0 {
		r.Info("OK    %s  all %d files match", root, ok)
	} else {
		r.Info("BAD   %s  %d match, %d problem(s)", root, ok, r.Failures())
	}
	return nil
}

// loadManifest finds the manifest for target. legacy is true for MHL v1.
func loadManifest(target string) (h *mhl.History, legacy bool, err error) {
	target, err = filepath.Abs(target)
	if err != nil {
		return nil, false, err
	}
	st, err := os.Stat(target)
	if err != nil {
		return nil, false, err
	}
	if !st.IsDir() {
		if filepath.Base(filepath.Dir(target)) == scan.MHLDir {
			h, err := mhl.Load(filepath.Dir(filepath.Dir(target)))
			return h, false, err
		}
		h, err := mhl.LoadLegacy(target)
		return h, true, err
	}
	if mhl.Exists(target) {
		h, err := mhl.Load(target)
		return h, false, err
	}
	files, _ := filepath.Glob(filepath.Join(target, "*.mhl"))
	if len(files) == 0 {
		return nil, false, fmt.Errorf("%s has no ascmhl folder or .mhl file; use `lumpy compare` to check it against another copy", target)
	}
	merged := &mhl.History{Root: target, Hashes: map[string][]mhl.Hash{}}
	for _, f := range files {
		l, err := mhl.LoadLegacy(f)
		if err != nil {
			return nil, true, err
		}
		merged.Generations = append(merged.Generations, filepath.Base(f))
		for p, hs := range l.Hashes {
			merged.Hashes[p] = append(merged.Hashes[p], hs...)
		}
	}
	return merged, true, nil
}
