// Package scan lists the files under a folder that Lumpy copies and checks.
package scan

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// MHLDir is the folder ASC MHL keeps its manifests in, at the root of the
// folder they describe.
const MHLDir = "ascmhl"

// PartialSuffix marks a file Lumpy is still writing. It is renamed to its
// real name only once every byte is on the drive.
const PartialSuffix = ".lumpy-partial"

// Ignored names are macOS housekeeping files that differ from drive to
// drive and are not part of the footage. They are skipped at any depth and
// listed in the MHL's ignore section.
var Ignored = []string{
	".DS_Store",
	".Spotlight-V100",
	".Trashes",
	".fseventsd",
	".TemporaryItems",
	".DocumentRevisions-V100",
	".apdisk",
	"._*", // AppleDouble sidecars macOS writes on exFAT and FAT cards
}

// File is one regular file, with Path relative to the scanned root and
// always using forward slashes.
type File struct {
	Path    string
	Size    int64
	ModTime time.Time
}

// Result is what Dir found.
type Result struct {
	Files    []File
	Dirs     []string // every folder, so empty ones are recreated too
	Skipped  []string // symlinks and other non-regular files, relative paths
	HasMHL   bool     // the root holds an ascmhl folder
	TotalLen int64
}

// extra are names a person asked to leave out of library scans
// (Settings → Folder exclusions), on top of Ignored.
var (
	extraMu sync.RWMutex
	extra   []string
)

// SetExtraIgnores replaces the person's exclusions: names, or prefixes
// ending in *, skipped at any depth.
func SetExtraIgnores(names []string) {
	extraMu.Lock()
	extra = append([]string(nil), names...)
	extraMu.Unlock()
}

func matches(name, pattern string) bool {
	return name == pattern || strings.HasSuffix(pattern, "*") && strings.HasPrefix(name, strings.TrimSuffix(pattern, "*"))
}

// IsIgnored reports whether a file or folder name is left out of scans.
func IsIgnored(name string) bool {
	for _, n := range Ignored {
		if matches(name, n) {
			return true
		}
	}
	extraMu.RLock()
	defer extraMu.RUnlock()
	for _, n := range extra {
		if matches(name, n) {
			return true
		}
	}
	return strings.HasSuffix(name, PartialSuffix)
}

func isIgnored(name string) bool { return IsIgnored(name) }

// Dir walks root and returns its regular files sorted by path, leaving out
// ignored names and the root's ascmhl folder.
func Dir(root string) (*Result, error) {
	res := &Result{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == root {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if isIgnored(d.Name()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if rel == MHLDir {
				res.HasMHL = true
				return filepath.SkipDir
			}
			res.Dirs = append(res.Dirs, rel)
			return nil
		}
		if !d.Type().IsRegular() {
			res.Skipped = append(res.Skipped, rel)
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		res.Files = append(res.Files, File{Path: rel, Size: info.Size(), ModTime: info.ModTime()})
		res.TotalLen += info.Size()
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scanning %s: %w", root, err)
	}
	sort.Slice(res.Files, func(i, j int) bool { return res.Files[i].Path < res.Files[j].Path })
	return res, nil
}
