package job

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lumpysheppard/lumpy/internal/catalog"
	"github.com/lumpysheppard/lumpy/internal/hashing"
	"github.com/lumpysheppard/lumpy/internal/media"
	"github.com/lumpysheppard/lumpy/internal/mhl"
	"github.com/lumpysheppard/lumpy/internal/scan"
	"github.com/lumpysheppard/lumpy/internal/volume"
)

// listDrive walks a whole drive and returns its files plus every xxh64
// checksum its MHL manifests record, both keyed by drive-relative path.
// ascmhl folders and legacy .mhl files themselves are not listed.
func listDrive(ctx context.Context, root string, r *Report) ([]catalog.File, map[string]string, error) {
	var files []catalog.File
	var mhlRoots, legacy []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == root {
				return err
			}
			r.Warn("cannot read %s: %v", p, err)
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if p == root {
			return nil
		}
		name := d.Name()
		if scan.IsIgnored(name) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if name == scan.MHLDir {
				mhlRoots = append(mhlRoots, filepath.Dir(p))
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || strings.HasSuffix(name, scan.PartialSuffix) {
			return nil
		}
		if strings.EqualFold(filepath.Ext(name), ".mhl") {
			legacy = append(legacy, p)
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		files = append(files, catalog.File{Path: filepath.ToSlash(rel), Size: info.Size(), ModTime: info.ModTime().UnixNano()})
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	sizes := make(map[string]int64, len(files))
	for _, f := range files {
		sizes[f.Path] = f.Size
	}
	hashes := map[string]string{}
	take := func(base string, h *mhl.History) {
		prefix, _ := filepath.Rel(root, base)
		prefix = filepath.ToSlash(prefix)
		for p := range h.Hashes {
			want, ok := h.Expected(p)
			if !ok || want.Format != hashing.XXH64 {
				continue
			}
			full := path.Join(prefix, p)
			if size, there := sizes[full]; there && (want.Size < 0 || want.Size == size) {
				hashes[full] = want.Hex
			}
		}
	}
	for _, f := range legacy {
		if h, err := mhl.LoadLegacy(f); err == nil {
			take(filepath.Dir(f), h)
		}
	}
	for _, m := range mhlRoots {
		if h, err := mhl.Load(m); err == nil {
			take(m, h)
		}
	}
	return files, hashes, nil
}

// CatalogDrive records a mounted drive and refreshes its file list.
func CatalogDrive(ctx context.Context, db *catalog.DB, v *volume.Volume, r *Report) (catalog.SyncResult, error) {
	if _, err := os.ReadDir(v.Mount); err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return catalog.SyncResult{}, fmt.Errorf("macOS is not letting Lumpy read %s. Allow it under System Settings → Privacy & Security → Files and Folders (Removable Volumes), or give Lumpy Full Disk Access, then try again", v.Mount)
		}
		return catalog.SyncResult{}, fmt.Errorf("cannot read %s: %w", v.Mount, err)
	}
	return catalogDrive(ctx, db, v, r, true)
}

func catalogDrive(ctx context.Context, db *catalog.DB, v *volume.Volume, r *Report, warnGone bool) (catalog.SyncResult, error) {
	if err := db.SaveDrive(v); err != nil {
		return catalog.SyncResult{}, err
	}
	files, hashes, err := listDrive(ctx, v.Mount, r)
	if err != nil {
		return catalog.SyncResult{}, err
	}
	res, err := db.SyncFiles(v.ID, files, hashes)
	if err != nil {
		return res, err
	}
	var total int64
	for _, f := range files {
		total += f.Size
	}
	summary := fmt.Sprintf("%d files, %s: %d new, %d changed, %d gone, %d checksums from MHLs",
		len(files), Bytes(total), res.Added, res.Changed, res.Removed, res.FromMHL)
	db.Log(v.ID, "scanned", summary, 0)
	r.Info("Logged       %s  (%s)", v.Name, summary)
	// Read metadata for clips that have none, within reason: a huge
	// drive finishes with "Read metadata" or the next scan.
	if unprobed, err := db.Unprobed(v.ID); AutoMetadata && err == nil && len(unprobed) > 0 {
		const cap = 2000
		batch := unprobed
		if len(batch) > cap {
			batch = batch[:cap]
		}
		if n, err := probeFiles(ctx, db, v, batch, r, nil); err == nil {
			more := ""
			if len(unprobed) > cap {
				more = fmt.Sprintf(" (%d more to read)", len(unprobed)-cap)
			}
			r.Info("Metadata     read for %d clips%s", n, more)
		} else if ctx.Err() != nil {
			return res, err
		}
	}
	shown := 0
	for _, p := range res.RemovedPaths {
		if !warnGone {
			break
		}
		if shown == 20 {
			r.Warn("... and %d more files are gone from %s", len(res.RemovedPaths)-shown, v.Name)
			break
		}
		r.Warn("%s is no longer on %s", p, v.Name)
		shown++
	}
	return res, nil
}

// HashDrive reads files on a mounted drive and records their checksums.
// With all false only files without a checksum are read. With all true
// every file is read and compared with what the library recorded, files
// that disappeared are reported, and the outcome is saved as the drive's
// last full check.
func HashDrive(ctx context.Context, db *catalog.DB, v *volume.Volume, all bool, r *Report) error {
	before, err := db.Files(v.ID)
	if err != nil {
		return err
	}
	known := make(map[string]catalog.File, len(before))
	for _, f := range before {
		known[f.Path] = f
	}
	startFailures := r.Failures()
	res, err := catalogDrive(ctx, db, v, r, !all)
	if err != nil {
		return err
	}
	if all {
		for _, p := range res.RemovedPaths {
			if known[p].XXH64 != "" {
				r.Fail("%s is missing from %s", p, v.Name)
			}
		}
	}
	files, err := db.Files(v.ID)
	if err != nil {
		return err
	}
	var todo []catalog.File
	var total int64
	for _, f := range files {
		if all || f.XXH64 == "" {
			todo = append(todo, f)
			total += f.Size
		}
	}
	verb := "Hashing"
	if all {
		verb = "Checking"
	}
	r.Info("%s %d files (%s) on %s", verb, len(todo), Bytes(total), v.Name)
	prog := r.startProgress(verb, total)
	var batch []catalog.HashUpdate
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		err := db.SaveHashes(v.ID, batch)
		batch = batch[:0]
		return err
	}
	matched, mismatched, fresh := 0, 0, 0
	// Reading is the whole cost (xxh64 keeps up with any drive), so a
	// spinning disk is read one file at a time to avoid seeking, while an
	// SSD or RAID reads several files at once.
	workers := hashWorkers(v)
	if Gentle {
		r.Info("Reading gently: one file at a time, about %d MB/s", GentleRate>>20)
	} else if workers > 1 {
		r.Info("Reading %d files at a time (solid-state drive)", workers)
	}
	type result struct {
		f   catalog.File
		got string
		n   int64
		err error
	}
	hctx, stop := context.WithCancel(ctx)
	defer stop()
	jobs := make(chan catalog.File)
	results := make(chan result)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for f := range jobs {
				got, n, err := hashFile(hctx, filepath.Join(v.Mount, filepath.FromSlash(f.Path)), hashing.XXH64, prog)
				select {
				case results <- result{f, got, n, err}:
				case <-hctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		for _, f := range todo {
			select {
			case jobs <- f:
			case <-hctx.Done():
				break
			}
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()
	for res := range results {
		f, got, n, err := res.f, res.got, res.n, res.err
		if errors.Is(err, context.Canceled) {
			stop()
			prog.Finish()
			flush()
			return ctx.Err()
		}
		if err != nil {
			if _, serr := os.Stat(v.Mount); serr != nil {
				stop()
				prog.Finish()
				flush()
				return fmt.Errorf("%s was disconnected during the check", v.Name)
			}
			r.Fail("%s on %s: %v", f.Path, v.Name, err)
			continue
		}
		if n != f.Size {
			r.Fail("%s on %s changed while being read", f.Path, v.Name)
			continue
		}
		// Compare with the checksum recorded before this run's listing, so
		// a file that changed on disk is still compared with its old value.
		want := f.XXH64
		if k, ok := known[f.Path]; ok && k.XXH64 != "" {
			want = k.XXH64
		}
		switch {
		case want == "":
			fresh++
			batch = append(batch, catalog.HashUpdate{Path: f.Path, Size: f.Size, ModTime: f.ModTime, XXH64: got})
		case want == got:
			matched++
			batch = append(batch, catalog.HashUpdate{Path: f.Path, Size: f.Size, ModTime: f.ModTime, XXH64: got, Verified: true})
		default:
			mismatched++
			r.Fail("%s on %s does not match its recorded checksum (recorded %s, now %s)", f.Path, v.Name, want, got)
			batch = append(batch, catalog.HashUpdate{Path: f.Path, Size: f.Size, ModTime: f.ModTime, XXH64: want, Bad: got})
		}
		if len(batch) >= 200 {
			if err := flush(); err != nil {
				stop()
				prog.Finish()
				return err
			}
		}
	}
	if err := ctx.Err(); err != nil {
		prog.Finish()
		flush()
		return err
	}
	took := prog.Finish()
	if err := flush(); err != nil {
		return err
	}
	failures := r.Failures() - startFailures
	summary := fmt.Sprintf("%d matched, %d newly hashed, %d mismatched, %d problem(s) in %s",
		matched, fresh, mismatched, failures, took.Round(time.Second))
	if all {
		db.Log(v.ID, "verified", summary, failures)
		if err := db.FinishVerify(v.ID, failures == 0); err != nil {
			return err
		}
	} else {
		db.Log(v.ID, "hashed", summary, failures)
	}
	if failures == 0 {
		r.Info("OK    %s  %s", v.Name, summary)
	} else {
		r.Info("BAD   %s  %s", v.Name, summary)
	}
	return nil
}

// Backups reports, from the library alone, whether every file on a drive
// has at least min copies on other drives. The drive does not need to be
// plugged in.
func Backups(db *catalog.DB, d *catalog.Drive, min int, r *Report) error {
	cov, err := db.Coverage(d.ID)
	if err != nil {
		return err
	}
	r.Info("Drive        %s  (%d files, %s, %d with checksums)", d.Title(), d.Files, Bytes(d.Bytes), d.Hashed)
	if d.Hashed < d.Files {
		r.Info("             %d files have no checksum yet; plug the drive in and run `lumpy drive hash %s` to prove their copies", d.Files-d.Hashed, d.Name)
	}
	r.Info("")
	var ok, short, none, likely, unknown, damaged int
	holders := map[string]int{}
	var problems []string
	for _, c := range cov {
		if c.BadHash != "" {
			problems = append(problems, fmt.Sprintf("FAIL  %s is damaged on this drive (last check did not match)", c.Path))
			damaged++
		}
		switch {
		case c.XXH64 != "" && len(c.Copies) >= min:
			ok++
		case c.XXH64 != "" && len(c.Copies) > 0:
			short++
			problems = append(problems, fmt.Sprintf("FAIL  %s has only %d cop%s (on %s)", c.Path, len(c.Copies), plural(len(c.Copies), "y", "ies"), names(c.Copies)))
		case c.XXH64 != "":
			none++
			problems = append(problems, fmt.Sprintf("FAIL  %s has no copy on any other drive", c.Path))
		case len(c.Likely) > 0:
			likely++
		default:
			unknown++
			problems = append(problems, fmt.Sprintf("FAIL  %s has no checksum and no file of the same name and size elsewhere", c.Path))
		}
		for _, cp := range c.Copies {
			holders[cp.DriveName]++
		}
	}
	sort.Strings(problems)
	for i, p := range problems {
		if i == 50 {
			r.Info("      ... and %d more", len(problems)-50)
			break
		}
		r.Info("%s", p)
	}
	r.addFailures(short + none + unknown + damaged)
	if len(problems) > 0 {
		r.Info("")
	}
	if len(holders) > 0 {
		var hs []string
		for n, c := range holders {
			hs = append(hs, fmt.Sprintf("%s (%d files)", n, c))
		}
		sort.Strings(hs)
		r.Info("Copies found on  %s", strings.Join(hs, ", "))
	}
	r.Info("%d files backed up at least %d time(s), %d with fewer copies, %d with none", ok, min, short, none)
	if likely > 0 {
		r.Warn("%d files without checksums have a same-name, same-size file on another drive (probably copies, not proven)", likely)
	}
	if short+none+unknown+damaged == 0 {
		r.Info("OK    %s  every checksummed file has at least %d other cop%s", d.Name, min, plural(min, "y", "ies"))
	} else {
		r.Info("BAD   %s  %d file(s) not backed up enough, %d damaged here", d.Name, short+none+unknown, damaged)
	}
	return nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func names(cs []Copy) string {
	var n []string
	for _, c := range cs {
		n = append(n, c.DriveName)
	}
	return strings.Join(n, ", ")
}

// Copy is re-exported so this file reads naturally.
type Copy = catalog.Copy

// recordOffload adds the files an offload copied and verified to the
// library, under each drive involved.
// The source's files count as verified when the source was read twice and
// matched; files it read differently (sourceBad) are not recorded.
func recordOffload(db *catalog.DB, src string, srcVerified bool, sourceBad map[string]bool, targets []string, files []scan.File, hashes map[string]string, verified []map[string]time.Time, r *Report) {
	record := func(root string, verifiedHere bool, ok func(string) bool) {
		v, err := volume.Of(root)
		if err != nil {
			r.Warn("drive library: cannot identify the drive holding %s: %v", root, err)
			return
		}
		if err := db.SaveDrive(v); err != nil {
			r.Warn("drive library: %v", err)
			return
		}
		// The mount point has symlinks resolved, so the folder must be too
		// (on macOS /var is really /private/var).
		real, err := filepath.EvalSymlinks(root)
		if err != nil {
			real = root
		}
		prefix, err := filepath.Rel(v.Mount, real)
		if err != nil || prefix == ".." || strings.HasPrefix(prefix, ".."+string(filepath.Separator)) {
			r.Warn("drive library: %s is not on %s; not recorded", root, v.Mount)
			return
		}
		var ups []catalog.HashUpdate
		for _, f := range files {
			sum, have := hashes[f.Path]
			if !have || !ok(f.Path) {
				continue
			}
			full := filepath.Join(root, filepath.FromSlash(f.Path))
			st, err := os.Stat(full)
			if err != nil {
				continue
			}
			ups = append(ups, catalog.HashUpdate{
				Path: path.Join(filepath.ToSlash(prefix), f.Path), Size: st.Size(),
				ModTime: st.ModTime().UnixNano(), XXH64: sum, Verified: verifiedHere,
			})
		}
		if err := db.SaveHashes(v.ID, ups); err != nil {
			r.Warn("drive library: %v", err)
			return
		}
		role := "offload destination"
		if root == src {
			role = "offload source"
		}
		db.Log(v.ID, "offload", fmt.Sprintf("%s: %s, %d files recorded", role, root, len(ups)), 0)
	}
	record(src, srcVerified, func(p string) bool { return !sourceBad[p] })
	for i, t := range targets {
		record(t, true, func(p string) bool { _, ok := verified[i][p]; return ok })
	}

	// Metadata is read once from the source and recorded for every copy.
	type where struct {
		id     string
		prefix string
	}
	var places []where
	ok := make([]func(string) bool, 0, len(targets)+1)
	if v, err := volume.Of(src); err == nil {
		if real, err := filepath.EvalSymlinks(src); err == nil {
			if rel, err := filepath.Rel(v.Mount, real); err == nil && rel != ".." && !strings.HasPrefix(rel, "../") {
				places = append(places, where{v.ID, filepath.ToSlash(rel)})
				ok = append(ok, func(p string) bool { return !sourceBad[p] })
			}
		}
	}
	for i, t := range targets {
		i := i
		if v, err := volume.Of(t); err == nil {
			if real, err := filepath.EvalSymlinks(t); err == nil {
				if rel, err := filepath.Rel(v.Mount, real); err == nil && rel != ".." && !strings.HasPrefix(rel, "../") {
					places = append(places, where{v.ID, filepath.ToSlash(rel)})
					ok = append(ok, func(p string) bool { _, ok := verified[i][p]; return ok })
				}
			}
		}
	}
	read := 0
	for _, f := range files {
		if !media.Probable(f.Path) || hashes[f.Path] == "" {
			continue
		}
		info, err := media.Probe(filepath.Join(src, filepath.FromSlash(f.Path)))
		if err != nil {
			continue
		}
		read++
		r.Detail("  metadata  %s  %s", f.Path, Describe(info))
		for k, pl := range places {
			if ok[k](f.Path) {
				db.SaveMedia(pl.id, path.Join(pl.prefix, f.Path), hashes[f.Path], f.Size, info)
			}
		}
	}
	if read > 0 {
		r.Info("Metadata     read for %d clips", read)
	}
}

// hashWorkers is how many files a drive is read from at once:
// LUMPY_HASH_WORKERS when set; otherwise 1 for a spinning disk, 2 for
// a solid-state drive on USB (readers and hubs are the weak link there),
// 4 for a solid-state drive on Thunderbolt, PCI or SATA.
func hashWorkers(v *volume.Volume) int {
	if Gentle {
		return 1
	}
	if n, err := strconv.Atoi(os.Getenv("LUMPY_HASH_WORKERS")); err == nil && n > 0 && n <= 16 {
		return n
	}
	if !v.SolidState {
		return 1
	}
	if strings.EqualFold(v.Bus, "USB") {
		return 2
	}
	return 4
}
