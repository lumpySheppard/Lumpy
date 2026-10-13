// Package scan lists the files under a folder that Lumpy copies and checks.
package scan

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
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

func isIgnored(name string) bool {
	for _, n := range Ignored {
		if name == n {
			return true
		}
	}
	return strings.HasSuffix(name, PartialSuffix)
}

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
