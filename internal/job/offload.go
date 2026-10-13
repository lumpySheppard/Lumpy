package job

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lumpysheppard/lumpy/internal/catalog"
	"github.com/lumpysheppard/lumpy/internal/fsx"
	"github.com/lumpysheppard/lumpy/internal/hashing"
	"github.com/lumpysheppard/lumpy/internal/mhl"
	"github.com/lumpysheppard/lumpy/internal/scan"
	"github.com/lumpysheppard/lumpy/internal/volume"
)

// Meta is written into every MHL generation Lumpy creates.
type Meta struct {
	Version  string
	Hostname string
}

// OffloadOptions describes one offload.
type OffloadOptions struct {
	Source string   // the card or folder to copy
	Dests  []string // drives or folders to copy it into
	Name   string   // folder created in each destination; defaults to the source's name
	Meta   Meta
	// SkipSourceVerify turns off the second read of the source. By default
	// the source is read again while the copies are verified, so a card or
	// reader that returns different data on different reads is caught
	// (otherwise every copy would carry the same bad data and still match).
	SkipSourceVerify bool
	// Library, when set, records every drive involved and the checksums
	// of the files copied to it.
	Library *catalog.DB
}

// Offload copies Source into every destination at once, reading the source
// a single time, then reads every copy back from its drive and compares it
// with the source checksum. Verified files are recorded in an ASC MHL
// generation in each copy's ascmhl folder.
func Offload(ctx context.Context, o OffloadOptions, r *Report) error {
	src, err := filepath.Abs(o.Source)
	if err != nil {
		return err
	}
	if st, err := os.Stat(src); err != nil || !st.IsDir() {
		return fmt.Errorf("source %s is not a folder", o.Source)
	}
	name := o.Name
	if name == "" {
		name = filepath.Base(src)
	}
	if name == "" || name == "/" || name == "." || strings.ContainsRune(name, filepath.Separator) {
		return fmt.Errorf("cannot name the copy %q; pass --name", name)
	}

	var targets []string
	seen := map[string]bool{}
	for _, d := range o.Dests {
		d, err := filepath.Abs(d)
		if err != nil {
			return err
		}
		// A destination may be a new folder, such as a shoot day, as long
		// as the folder it goes in exists (so the drive is mounted).
		if st, err := os.Stat(d); err == nil && !st.IsDir() {
			return fmt.Errorf("destination %s is not a folder", d)
		} else if err != nil {
			if pst, perr := os.Stat(filepath.Dir(d)); perr != nil || !pst.IsDir() {
				return fmt.Errorf("destination %s does not exist (is the drive mounted?)", d)
			}
		}
		t := filepath.Join(d, name)
		if within(t, src) || within(src, t) {
			return fmt.Errorf("destination %s overlaps the source", t)
		}
		if seen[t] {
			return fmt.Errorf("destination %s is listed twice", t)
		}
		seen[t] = true
		targets = append(targets, t)
	}

	res, err := scan.Dir(src)
	if err != nil {
		return err
	}
	if len(res.Files) == 0 {
		return fmt.Errorf("source %s has no files to copy", src)
	}
	for _, s := range res.Skipped {
		r.Warn("%s is not a regular file (symlink or device); not copied", s)
	}
	r.Info("Source       %s  (%d files, %s)", src, len(res.Files), Bytes(res.TotalLen))
	for _, t := range targets {
		r.Info("Destination  %s", t)
	}
	if o.SkipSourceVerify {
		r.Info("Source check off: the source is read once")
	} else {
		r.Info("Source check on: the source is read again during verification")
	}
	labels := make([]string, len(targets))
	for i, t := range targets {
		labels[i] = driveLabel(t)
	}

	srcHist, err := mhl.Load(src)
	if err != nil {
		return fmt.Errorf("source MHL: %w", err)
	}
	for _, p := range srcHist.Problems {
		r.Warn("source MHL: %s", p)
	}

	// Check each drive has room for what it does not already hold.
	for _, t := range targets {
		var need int64
		for _, f := range res.Files {
			if st, err := os.Stat(filepath.Join(t, filepath.FromSlash(f.Path))); err != nil || st.Size() != f.Size {
				need += f.Size
			}
		}
		free, err := fsx.FreeBytes(existingParent(t))
		if err == nil && uint64(need)+64<<20 > free {
			return fmt.Errorf("%s needs %s but its drive has %s free", t, Bytes(need), Bytes(int64(free)))
		}
	}

	for _, t := range targets {
		if err := os.MkdirAll(t, 0o755); err != nil {
			return err
		}
		for _, d := range res.Dirs {
			if err := os.MkdirAll(filepath.Join(t, filepath.FromSlash(d)), 0o755); err != nil {
				return err
			}
		}
		// Carry the card's own MHL history over so the copy keeps it.
		if res.HasMHL && !mhl.Exists(t) {
			if err := copyMHLFolder(ctx, src, t); err != nil {
				return fmt.Errorf("copying source ascmhl folder to %s: %w", t, err)
			}
		}
	}

	// failed[t][path] is set once a file has a problem on target t.
	failed := make([]map[string]bool, len(targets))
	for i := range failed {
		failed[i] = map[string]bool{}
	}
	srcHashes := map[string]string{}
	sourceMHLFormatWarned := false
	metaWarned := make([]bool, len(targets))

	r.Info("")
	r.Info("Copying...")
	prog := r.startProgress("Copying", res.TotalLen)
	for _, f := range res.Files {
		if err := ctx.Err(); err != nil {
			prog.Finish()
			return err
		}
		prog.SetCurrent(f.Path)
		var writes []string
		var writeIdx []int
		for i, t := range targets {
			dst := filepath.Join(t, filepath.FromSlash(f.Path))
			st, err := os.Lstat(dst)
			switch {
			case err == nil && st.Mode().IsRegular() && st.Size() == f.Size:
				// Already there from an earlier run; the verify pass decides whether it is good.
				r.Detail("  exists    %s on %s (checked in the verify pass)", f.Path, labels[i])
			case err == nil:
				r.Fail("%s already exists on %s with different contents; left untouched", f.Path, t)
				failed[i][f.Path] = true
			case errors.Is(err, os.ErrNotExist):
				writes = append(writes, dst)
				writeIdx = append(writeIdx, i)
			default:
				r.Fail("%s on %s: %v", f.Path, t, err)
				failed[i][f.Path] = true
			}
		}
		cr := copyFile(ctx, filepath.Join(src, filepath.FromSlash(f.Path)), writes, f.ModTime, prog)
		sum, n, readErr, writeErrs := cr.sum, cr.n, cr.readErr, cr.writeErrs
		if errors.Is(readErr, context.Canceled) {
			prog.Finish()
			return readErr
		}
		for k, werr := range writeErrs {
			if werr != nil {
				r.Fail("%s: writing to %s: %v", f.Path, targets[writeIdx[k]], werr)
				failed[writeIdx[k]][f.Path] = true
			}
		}
		for k, merr := range cr.metaErrs {
			if merr != nil && !metaWarned[writeIdx[k]] {
				metaWarned[writeIdx[k]] = true
				r.Warn("%s: could not copy macOS metadata (extended attributes) to %s: %v; file contents are unaffected", f.Path, labels[writeIdx[k]], merr)
			}
		}
		if readErr == nil && n != f.Size {
			readErr = fmt.Errorf("file changed while it was being copied (%d bytes expected, %d read)", f.Size, n)
			for k, d := range writes {
				if writeErrs[k] == nil {
					os.Remove(d)
				}
			}
		}
		if readErr != nil {
			r.Fail("%s: reading source: %v", f.Path, readErr)
			for i := range targets {
				failed[i][f.Path] = true
			}
			continue
		}
		srcHashes[f.Path] = sum
		if len(writes) > 0 {
			r.Detail("  copied    %s  %s  xxh64 %s", f.Path, Bytes(f.Size), sum)
		}
		if want, ok := srcHist.Expected(f.Path); ok {
			if want.Format != hashing.XXH64 {
				if !sourceMHLFormatWarned {
					r.Warn("source MHL uses %s, not xxh64; source files were not checked against it", want.Format)
					sourceMHLFormatWarned = true
				}
			} else if want.Hex != sum {
				r.Fail("%s: source does not match its own MHL (MHL %s, now %s); the card may be damaged", f.Path, want.Hex, sum)
			}
		}
	}
	copyTime := prog.Finish()
	r.Info("Copied %s in %s", Bytes(res.TotalLen), copyTime.Round(time.Second))
	if afterCopyHook != nil {
		afterCopyHook()
	}

	// Read every copy back from its drive. Drives are read at the same time;
	// each drive reads its files one after another.
	r.Info("")
	r.Info("Verifying...")
	reads := int64(len(targets))
	if !o.SkipSourceVerify {
		reads++
	}
	prog = r.startProgress("Verifying", res.TotalLen*reads)
	verifiedAt := make([]map[string]time.Time, len(targets))
	sourceBad := map[string]bool{}
	var wg sync.WaitGroup
	if !o.SkipSourceVerify {
		// Read the source again, alongside the destinations (it is on its
		// own drive, so this costs little extra time).
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, f := range res.Files {
				want, ok := srcHashes[f.Path]
				if !ok {
					continue
				}
				got, _, err := hashFile(ctx, filepath.Join(src, filepath.FromSlash(f.Path)), hashing.XXH64, prog)
				if errors.Is(err, context.Canceled) {
					return
				}
				switch {
				case err != nil:
					r.Fail("%s: reading the source again: %v; its copies are not trusted", f.Path, err)
					sourceBad[f.Path] = true
				case got != want:
					r.Fail("%s read differently from the source the second time (first %s, then %s): the card or reader is unreliable, so its copies are not trusted", f.Path, want, got)
					sourceBad[f.Path] = true
				default:
					r.Detail("  source ok %s", f.Path)
				}
			}
		}()
	}
	for i, t := range targets {
		verifiedAt[i] = map[string]time.Time{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, f := range res.Files {
				want, ok := srcHashes[f.Path]
				if !ok || failed[i][f.Path] {
					continue
				}
				prog.SetCurrent(f.Path)
				got, _, err := hashFile(ctx, filepath.Join(t, filepath.FromSlash(f.Path)), hashing.XXH64, prog)
				if errors.Is(err, context.Canceled) {
					return
				}
				switch {
				case err != nil:
					r.Fail("%s on %s: %v", f.Path, t, err)
					failed[i][f.Path] = true
				case got != want:
					r.Fail("%s on %s does not match the source (source %s, copy %s); delete the copy and run again to recopy it", f.Path, t, want, got)
					failed[i][f.Path] = true
				default:
					verifiedAt[i][f.Path] = time.Now()
					r.Detail("  verified  %s on %s", f.Path, labels[i])
				}
			}
		}()
	}
	wg.Wait()
	verifyTime := prog.Finish()
	if err := ctx.Err(); err != nil {
		return err
	}
	// A file the source gave back differently is not recorded as verified
	// anywhere, even if a copy matched the first read.
	for p := range sourceBad {
		for i := range targets {
			delete(verifiedAt[i], p)
			failed[i][p] = true
		}
	}
	r.Info("Verified in %s", verifyTime.Round(time.Second))

	r.Info("")
	for i, t := range targets {
		hist, err := mhl.Load(t)
		if err != nil {
			r.Fail("%s: reading existing MHL: %v", t, err)
			continue
		}
		gen := &mhl.Generation{Process: "transfer", Created: time.Now(), Hostname: o.Meta.Hostname, ToolVersion: o.Meta.Version}
		for _, f := range res.Files {
			at, ok := verifiedAt[i][f.Path]
			if !ok {
				continue
			}
			sum := srcHashes[f.Path]
			action := "original"
			if _, known := hist.Hashes[f.Path]; known {
				action = "verified"
				if want, ok := hist.Expected(f.Path); ok && want.Format == hashing.XXH64 && want.Hex != sum {
					r.Fail("%s on %s differs from its earlier MHL record (%s, now %s)", f.Path, t, want.Hex, sum)
					action = "failed"
				}
			}
			gen.Entries = append(gen.Entries, mhl.Entry{
				Path: f.Path, Size: f.Size, ModTime: f.ModTime,
				Format: hashing.XXH64, Hex: sum, Action: action, HashDate: at,
			})
		}
		bad := len(failed[i])
		if len(gen.Entries) > 0 {
			p, err := mhl.Write(t, gen)
			if err != nil {
				r.Fail("%s: writing MHL: %v", t, err)
			} else {
				r.Info("MHL written  %s", p)
			}
		}
		if bad == 0 {
			r.Info("OK    %s  all %d files verified", t, len(verifiedAt[i]))
		} else {
			r.Info("BAD   %s  %d verified, %d failed", t, len(verifiedAt[i]), bad)
		}
	}
	if o.Library != nil {
		recordOffload(o.Library, src, !o.SkipSourceVerify, sourceBad, targets, res.Files, srcHashes, verifiedAt, r)
	}
	return nil
}

// afterCopyHook lets tests change the source between the copy and the
// verify pass, as a flaky card reader would.
var afterCopyHook func()

// existingParent returns p or its nearest folder that exists.
func existingParent(p string) string {
	for {
		if _, err := os.Stat(p); err == nil || filepath.Dir(p) == p {
			return p
		}
		p = filepath.Dir(p)
	}
}

// within reports whether p is base or inside it.
func within(p, base string) bool {
	rel, err := filepath.Rel(base, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// copyResult is what copyFile did. writeErrs and metaErrs line up with dsts.
type copyResult struct {
	sum       string
	n         int64
	readErr   error
	writeErrs []error
	metaErrs  []error // extended attributes that could not be copied
}

// copyFile reads src once and writes it to every path in dsts, hashing it
// with xxh64 on the way. Each copy is written under a temporary name,
// flushed to the drive, given the source's extended attributes and
// modification time, and only then renamed into place. A failed copy is
// removed.
func copyFile(ctx context.Context, src string, dsts []string, mtime time.Time, prog *Progress) copyResult {
	sum, n, readErr, writeErrs, metaErrs := copyFileInner(ctx, src, dsts, mtime, prog)
	return copyResult{sum, n, readErr, writeErrs, metaErrs}
}

func copyFileInner(ctx context.Context, src string, dsts []string, mtime time.Time, prog *Progress) (sum string, n int64, readErr error, writeErrs, metaErrs []error) {
	writeErrs = make([]error, len(dsts))
	metaErrs = make([]error, len(dsts))
	in, err := fsx.OpenRead(src)
	if err != nil {
		return "", 0, err, writeErrs, metaErrs
	}
	defer in.Close()

	outs := make([]*os.File, len(dsts))
	tmps := make([]string, len(dsts))
	for i, d := range dsts {
		tmps[i] = filepath.Join(filepath.Dir(d), "."+filepath.Base(d)+scan.PartialSuffix)
		if err := os.MkdirAll(filepath.Dir(d), 0o755); err != nil {
			writeErrs[i] = err
			continue
		}
		outs[i], writeErrs[i] = fsx.Create(tmps[i])
	}
	defer func() {
		for i, f := range outs {
			if f != nil {
				f.Close()
			}
			if writeErrs[i] != nil || readErr != nil {
				os.Remove(tmps[i])
			}
		}
	}()

	h := hashing.New(hashing.XXH64)
	bufs := [2][]byte{make([]byte, chunkSize), make([]byte, chunkSize)}
	cur := 0
	m, err := io.ReadFull(in, bufs[cur])
	for m > 0 {
		if ctx.Err() != nil {
			return "", n, ctx.Err(), writeErrs, metaErrs
		}
		chunk := bufs[cur][:m]
		var wg sync.WaitGroup
		for i, f := range outs {
			if f == nil || writeErrs[i] != nil {
				continue
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, writeErrs[i] = f.Write(chunk)
			}()
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			h.Write(chunk)
		}()
		// Read the next chunk while this one is written.
		next := 1 - cur
		var nm int
		var nerr error
		if err == nil {
			nm, nerr = io.ReadFull(in, bufs[next])
		}
		wg.Wait()
		n += int64(m)
		prog.Add(m)
		cur, m, err = next, nm, nerr
	}
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return "", n, err, writeErrs, metaErrs
	}

	for i, f := range outs {
		if f == nil || writeErrs[i] != nil {
			continue
		}
		if err := f.Sync(); err != nil {
			writeErrs[i] = err
			continue
		}
		fsx.DropCache(f)
		metaErrs[i] = fsx.CopyXattrs(src, f)
		err := f.Close()
		outs[i] = nil
		if err == nil {
			err = os.Chtimes(tmps[i], mtime, mtime)
		}
		if err == nil {
			err = os.Rename(tmps[i], dsts[i])
		}
		writeErrs[i] = err
	}
	return h.Hex(), n, nil, writeErrs, metaErrs
}

// copyMHLFolder copies src's ascmhl folder into dst and checks each copied file.
func copyMHLFolder(ctx context.Context, src, dst string) error {
	from := filepath.Join(src, scan.MHLDir)
	to := filepath.Join(dst, scan.MHLDir)
	entries, err := os.ReadDir(from)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(to, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		d := filepath.Join(to, e.Name())
		cr := copyFile(ctx, filepath.Join(from, e.Name()), []string{d}, info.ModTime(), nopProgress())
		if cr.readErr != nil {
			return cr.readErr
		}
		if cr.writeErrs[0] != nil {
			return cr.writeErrs[0]
		}
		sum := cr.sum
		got, _, err := hashFile(ctx, d, hashing.XXH64, nil)
		if err != nil {
			return err
		}
		if got != sum {
			return fmt.Errorf("%s does not match the source after copying", d)
		}
	}
	return nil
}

func nopProgress() *Progress { return &Progress{} }

// driveLabel names a destination briefly for per-file lines: the drive's
// name, plus the folder on it.
func driveLabel(t string) string {
	if mp, err := volume.MountPoint(existingParent(t)); err == nil && mp != "/" {
		if rel, err := filepath.Rel(mp, t); err == nil {
			return filepath.Base(mp) + ":" + filepath.ToSlash(rel)
		}
	}
	return t
}
