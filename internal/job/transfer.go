package job

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/lumpysheppard/lumpy/internal/catalog"
	"github.com/lumpysheppard/lumpy/internal/fsx"
	"github.com/lumpysheppard/lumpy/internal/hashing"
	"github.com/lumpysheppard/lumpy/internal/mhl"
	"github.com/lumpysheppard/lumpy/internal/volume"
)

// TransferOptions is one step of a project plan: copy particular files
// from one drive to another.
type TransferOptions struct {
	Source *volume.Volume
	Dest   *volume.Volume
	// Folder is the folder on Dest the files go into; each file's Dest
	// path (or its source path when Dest is empty) goes below it.
	// Defaults to the source drive's name.
	Folder string
	Files  []catalog.PlanFile
	Meta   Meta
	// Library records the copies. Required: a transfer exists to improve
	// the project's health.
	Library          *catalog.DB
	SkipSourceVerify bool
}

// Transfer copies the files, reads each copy back from the destination
// drive and compares it with the checksum the library recorded for the
// source (so the source is checked against an earlier read as well as
// this one), writes an MHL generation into the destination folder, and
// records the new copies.
func Transfer(ctx context.Context, o TransferOptions, r *Report) error {
	if o.Library == nil {
		return errors.New("a transfer needs the drive library")
	}
	if len(o.Files) == 0 {
		return errors.New("nothing to copy")
	}
	folder := strings.TrimSpace(o.Folder)
	if folder == "" {
		folder = o.Source.Name
	}
	folder = strings.Trim(filepath.ToSlash(folder), "/")
	if folder == "" || folder == "." || strings.Contains(folder, "..") {
		return fmt.Errorf("cannot copy into a folder called %q", o.Folder)
	}
	destRoot := filepath.Join(o.Dest.Mount, filepath.FromSlash(folder))
	if within(destRoot, o.Source.Mount) || within(o.Source.Mount, destRoot) {
		return errors.New("the destination is on the source drive")
	}
	if _, err := os.Stat(o.Source.Mount); err != nil {
		return fmt.Errorf("source %s is not plugged in", o.Source.Name)
	}
	if _, err := os.Stat(o.Dest.Mount); err != nil {
		return fmt.Errorf("destination %s is not plugged in", o.Dest.Name)
	}

	var total int64
	for _, f := range o.Files {
		total += f.Size
	}
	r.Info("Source       %s  (%d files, %s)", o.Source.Mount, len(o.Files), Bytes(total))
	r.Info("Destination  %s", destRoot)
	if o.SkipSourceVerify {
		r.Info("Source check off: each file is compared with the checksum on record only")
	} else {
		r.Info("Source check on: the source is read again during verification")
	}
	free, err := fsx.FreeBytes(existingParent(destRoot))
	if err == nil && uint64(total)+64<<20 > free {
		return fmt.Errorf("%s needs %s but has %s free", o.Dest.Name, Bytes(total), Bytes(int64(free)))
	}
	if err := os.MkdirAll(destRoot, 0o755); err != nil {
		return err
	}

	type done struct {
		f     catalog.PlanFile
		dst   string
		sum   string
		mtime time.Time
		at    time.Time
	}
	var copied []done
	destLabel := driveLabel(destRoot)

	r.Info("")
	r.Info("Copying...")
	prog := r.startProgress("Copying", total)
	for _, f := range o.Files {
		if err := ctx.Err(); err != nil {
			prog.Finish()
			return err
		}
		prog.SetCurrent(f.Path)
		src := filepath.Join(o.Source.Mount, filepath.FromSlash(f.Path))
		dst := filepath.Join(destRoot, filepath.FromSlash(destPath(f)))
		st, err := os.Stat(src)
		if err != nil {
			r.Fail("%s: %v", f.Path, err)
			continue
		}
		if st.Size() != f.Size {
			r.Fail("%s is %d bytes on %s; the library recorded %d. Rescan the drive and plan again", f.Path, st.Size(), o.Source.Name, f.Size)
			continue
		}
		if there, err := os.Lstat(dst); err == nil {
			if there.Mode().IsRegular() && there.Size() == f.Size {
				r.Detail("  exists    %s on %s (checked in the verify pass)", f.Path, destLabel)
				copied = append(copied, done{f: f, dst: dst, sum: f.XXH64, mtime: st.ModTime()})
				prog.Add(int(f.Size))
			} else {
				r.Fail("%s already exists on %s with different contents; left untouched", f.Path, destLabel)
			}
			continue
		}
		cr := copyFile(ctx, src, []string{dst}, st.ModTime(), prog)
		if errors.Is(cr.readErr, context.Canceled) {
			prog.Finish()
			return cr.readErr
		}
		if cr.readErr != nil {
			r.Fail("%s: reading source: %v", f.Path, cr.readErr)
			continue
		}
		if cr.writeErrs[0] != nil {
			r.Fail("%s: writing to %s: %v", f.Path, destLabel, cr.writeErrs[0])
			continue
		}
		if cr.metaErrs[0] != nil {
			r.Warn("%s: could not copy macOS metadata to %s: %v; file contents are unaffected", f.Path, destLabel, cr.metaErrs[0])
		}
		if cr.n != f.Size {
			os.Remove(dst)
			r.Fail("%s changed while it was being copied", f.Path)
			continue
		}
		if f.XXH64 != "" && cr.sum != f.XXH64 {
			os.Remove(dst)
			r.Fail("%s on %s no longer matches the checksum on record (recorded %s, now %s); the source copy may be damaged. Check %s before trusting it", f.Path, o.Source.Name, f.XXH64, cr.sum, o.Source.Name)
			continue
		}
		r.Detail("  copied    %s  %s  xxh64 %s", f.Path, Bytes(f.Size), cr.sum)
		copied = append(copied, done{f: f, dst: dst, sum: cr.sum, mtime: st.ModTime()})
	}
	copyTime := prog.Finish()
	r.Info("Copied %s in %s", Bytes(total), copyTime.Round(time.Second))

	r.Info("")
	r.Info("Verifying...")
	reads := int64(1)
	if !o.SkipSourceVerify {
		reads++
	}
	var verifyBytes int64
	for _, d := range copied {
		verifyBytes += d.f.Size
	}
	prog = r.startProgress("Verifying", verifyBytes*reads)
	var good []done
	for _, d := range copied {
		if err := ctx.Err(); err != nil {
			prog.Finish()
			return err
		}
		prog.SetCurrent(d.f.Path)
		if d.sum == "" {
			// Already on the destination and no checksum on record: the
			// source read now is the reference.
			sum, _, err := hashFile(ctx, filepath.Join(o.Source.Mount, filepath.FromSlash(d.f.Path)), hashing.XXH64, prog)
			if errors.Is(err, context.Canceled) {
				prog.Finish()
				return err
			}
			if err != nil {
				r.Fail("%s: reading source: %v", d.f.Path, err)
				continue
			}
			d.sum = sum
		}
		got, _, err := hashFile(ctx, d.dst, hashing.XXH64, prog)
		if errors.Is(err, context.Canceled) {
			prog.Finish()
			return err
		}
		switch {
		case err != nil:
			r.Fail("%s on %s: %v", d.f.Path, destLabel, err)
			continue
		case got != d.sum:
			r.Fail("%s on %s does not match (source %s, copy %s); delete the copy and run again", d.f.Path, destLabel, d.sum, got)
			continue
		}
		if !o.SkipSourceVerify {
			again, _, err := hashFile(ctx, filepath.Join(o.Source.Mount, filepath.FromSlash(d.f.Path)), hashing.XXH64, prog)
			if errors.Is(err, context.Canceled) {
				prog.Finish()
				return err
			}
			if err != nil || again != d.sum {
				r.Fail("%s read differently from %s the second time: the drive or reader is unreliable, so this copy is not trusted", d.f.Path, o.Source.Name)
				continue
			}
		}
		r.Detail("  verified  %s on %s", d.f.Path, destLabel)
		d.at = time.Now()
		good = append(good, d)
	}
	verifyTime := prog.Finish()
	r.Info("Verified in %s", verifyTime.Round(time.Second))
	r.Info("")

	if len(good) > 0 {
		gen := &mhl.Generation{Process: "transfer", Created: time.Now(), Hostname: o.Meta.Hostname, ToolVersion: o.Meta.Version}
		for _, d := range good {
			gen.Entries = append(gen.Entries, mhl.Entry{Path: destPath(d.f), Size: d.f.Size, ModTime: d.mtime,
				Format: hashing.XXH64, Hex: d.sum, Action: "original", HashDate: d.at})
		}
		if p, err := mhl.Write(destRoot, gen); err != nil {
			r.Fail("writing MHL: %v", err)
		} else {
			r.Info("MHL written  %s", p)
		}
		var ups []catalog.HashUpdate
		for _, d := range good {
			st, err := os.Stat(d.dst)
			if err != nil {
				continue
			}
			ups = append(ups, catalog.HashUpdate{Path: path.Join(folder, destPath(d.f)), Size: d.f.Size,
				ModTime: st.ModTime().UnixNano(), XXH64: d.sum, Verified: true})
		}
		if err := o.Library.SaveDrive(o.Dest); err == nil {
			if err := o.Library.SaveHashes(o.Dest.ID, ups); err != nil {
				r.Fail("drive library: %v", err)
			}
		}
		o.Library.Log(o.Dest.ID, "transfer", fmt.Sprintf("received %d files (%s) from %s into %s", len(good), Bytes(sumBytes(good, func(d done) int64 { return d.f.Size })), o.Source.Name, folder), 0)
		o.Library.Log(o.Source.ID, "transfer", fmt.Sprintf("%d files (%s) copied to %s", len(good), Bytes(sumBytes(good, func(d done) int64 { return d.f.Size })), o.Dest.Name), r.Failures())
	}
	bad := len(o.Files) - len(good)
	if bad == 0 {
		r.Info("OK    %s  all %d files copied and verified", destRoot, len(good))
	} else {
		r.Info("BAD   %s  %d copied and verified, %d failed", destRoot, len(good), bad)
	}
	return nil
}

func sumBytes[T any](xs []T, f func(T) int64) int64 {
	var n int64
	for _, x := range xs {
		n += f(x)
	}
	return n
}

// destPath is where a planned file lands below the destination folder.
func destPath(f catalog.PlanFile) string {
	if f.Dest != "" {
		return f.Dest
	}
	return f.Path
}
