package job

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/lumpysheppard/lumpy/internal/hashing"
	"github.com/lumpysheppard/lumpy/internal/scan"
)

// Compare checks two folders hold the same files with the same contents,
// reading both drives at the same time. Neither folder needs a manifest.
func Compare(ctx context.Context, a, b string, r *Report) error {
	var sides [2]*scan.Result
	for i, p := range []string{a, b} {
		if st, err := os.Stat(p); err != nil || !st.IsDir() {
			return fmt.Errorf("%s is not a folder", p)
		}
		res, err := scan.Dir(p)
		if err != nil {
			return err
		}
		sides[i] = res
	}
	r.Info("A  %s  (%d files, %s)", a, len(sides[0].Files), Bytes(sides[0].TotalLen))
	r.Info("B  %s  (%d files, %s)", b, len(sides[1].Files), Bytes(sides[1].TotalLen))
	r.Info("")

	inB := map[string]scan.File{}
	for _, f := range sides[1].Files {
		inB[f.Path] = f
	}
	var both []scan.File
	var total int64
	for _, f := range sides[0].Files {
		g, ok := inB[f.Path]
		switch {
		case !ok:
			r.Fail("%s is only in A", f.Path)
		case g.Size != f.Size:
			r.Fail("%s is %d bytes in A and %d bytes in B", f.Path, f.Size, g.Size)
		default:
			both = append(both, f)
			total += f.Size
		}
		delete(inB, f.Path)
	}
	var onlyB []string
	for p := range inB {
		onlyB = append(onlyB, p)
	}
	sort.Strings(onlyB)
	for _, p := range onlyB {
		r.Fail("%s is only in B", p)
	}

	prog := r.startProgress("compare", 2*total)
	ok := 0
	for _, f := range both {
		var sums [2]string
		var errs [2]error
		done := make(chan struct{}, 2)
		for i, root := range []string{a, b} {
			go func() {
				sums[i], _, errs[i] = hashFile(ctx, filepath.Join(root, filepath.FromSlash(f.Path)), hashing.XXH64, prog)
				done <- struct{}{}
			}()
		}
		<-done
		<-done
		if errors.Is(errs[0], context.Canceled) || errors.Is(errs[1], context.Canceled) {
			prog.Finish()
			return context.Canceled
		}
		switch {
		case errs[0] != nil:
			r.Fail("%s in A: %v", f.Path, errs[0])
		case errs[1] != nil:
			r.Fail("%s in B: %v", f.Path, errs[1])
		case sums[0] != sums[1]:
			r.Fail("%s differs (A %s, B %s)", f.Path, sums[0], sums[1])
		default:
			ok++
		}
	}
	took := prog.Finish()
	r.Info("")
	r.Info("Compared %d files (%s each side) in %s", len(both), Bytes(total), took.Round(time.Second))
	if r.Failures() == 0 {
		r.Info("OK    all %d files match", ok)
	} else {
		r.Info("BAD   %d match, %d problem(s)", ok, r.Failures())
	}
	return nil
}
