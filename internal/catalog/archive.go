package catalog

import (
	"path"
	"sort"
	"strings"

	"github.com/lumpysheppard/lumpy/internal/media"
)

// When the cut is locked, the timeline export says which originals the
// edit uses. Everything else in the project's folders can be moved
// aside or thrown away. Matching is by clip name, so a name the edit
// uses anywhere keeps every clip with that name: conservative on
// purpose, since the next step may be the Trash.

// ArchiveRef is one piece of media an edit refers to.
type ArchiveRef struct {
	Name string
	Path string
}

// ArchiveItem is one piece of footage in a project's folders: a file,
// or a RED clip folder with all its chunks.
type ArchiveItem struct {
	DriveID string `json:"driveId"`
	Source  int    `json:"source"` // index into the project's sources
	Path    string `json:"path"`   // file or .RDC folder, relative to the drive
	Name    string `json:"name"`
	Bytes   int64  `json:"bytes"`
	Files   int    `json:"files"`
	Used    bool   `json:"used"`
	Shared  bool   `json:"shared,omitempty"` // another clip has the same name
}

// ArchiveFolder sums the footage in one top-level folder of a source.
type ArchiveFolder struct {
	DriveID     string `json:"driveId"`
	Source      string `json:"source"`
	Folder      string `json:"folder"` // "." for files at the source's top level
	Files       int    `json:"files"`
	Bytes       int64  `json:"bytes"`
	UnusedFiles int    `json:"unusedFiles"`
	UnusedBytes int64  `json:"unusedBytes"`
}

// ArchiveReport is what an edit uses of a project and what it doesn't.
type ArchiveReport struct {
	Refs        int             `json:"refs"`  // distinct clips the edit refers to
	Found       int             `json:"found"` // of those, in the project's folders
	NotFound    []string        `json:"notFound"`
	UsedFiles   int             `json:"usedFiles"`
	UsedBytes   int64           `json:"usedBytes"`
	UnusedFiles int             `json:"unusedFiles"`
	UnusedBytes int64           `json:"unusedBytes"`
	Folders     []ArchiveFolder `json:"folders"`
	Items       []ArchiveItem   `json:"items"`
}

// refKeys are the stems a reference may be known by: as named, with a
// proxy suffix stripped, with a RED chunk number stripped.
func refKeys(name string) []string {
	stem := clipStem(name)
	keys := []string{stem}
	if s := proxySuffix.ReplaceAllString(stem, ""); s != stem {
		keys = append(keys, s)
	}
	if s := redChunk.ReplaceAllString(stem, ""); s != stem {
		keys = append(keys, s)
	}
	return keys
}

// Archive matches the edit's references against the footage in sources.
func (db *DB) Archive(sources []Source, refs []ArchiveRef) (*ArchiveReport, error) {
	rep := &ArchiveReport{NotFound: []string{}, Folders: []ArchiveFolder{}, Items: []ArchiveItem{}}
	// References, by every key they may match under. The same clip named
	// twice (with and without its extension, or two RED chunks) is one.
	type ref struct {
		name  string
		keys  []string
		found bool
	}
	var rs []*ref
	byKey := map[string][]*ref{}
	seen := map[string]bool{}
	for _, r := range refs {
		name := r.Name
		if name == "" {
			name = path.Base(r.Path)
		}
		if name == "" || seen[clipStem(name)] {
			continue
		}
		seen[clipStem(name)] = true
		x := &ref{name: name, keys: refKeys(name)}
		rs = append(rs, x)
		for _, k := range x.keys {
			byKey[k] = append(byKey[k], x)
		}
	}
	rep.Refs = len(rs)

	// Footage: every video, audio and still under the sources, the most
	// specific source owning a file that is under two of them.
	order := make([]int, len(sources))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return len(sources[order[a]].Folder) > len(sources[order[b]].Folder) })
	cache := map[string][]File{}
	items := map[string]*ArchiveItem{}
	var keys []string
	for _, si := range order {
		src := sources[si]
		files, ok := cache[src.DriveID]
		if !ok {
			var err error
			if files, err = db.Files(src.DriveID); err != nil {
				return nil, err
			}
			cache[src.DriveID] = files
		}
		prefix := src.Folder
		if prefix != "" {
			prefix += "/"
		}
		for _, f := range files {
			if !strings.HasPrefix(f.Path, prefix) || !media.Probable(f.Path) {
				continue
			}
			key, name := f.Path, path.Base(f.Path)
			if strings.EqualFold(path.Ext(f.Path), ".r3d") {
				if dir := path.Dir(f.Path); strings.EqualFold(path.Ext(dir), ".rdc") {
					key, name = dir, strings.TrimSuffix(path.Base(dir), path.Ext(dir))
				}
			}
			id := src.DriveID + "\x00" + key
			it := items[id]
			if it == nil {
				it = &ArchiveItem{DriveID: src.DriveID, Source: si, Path: key, Name: name}
				items[id] = it
				keys = append(keys, id)
			} else if it.Source != si {
				continue // already owned by a more specific source
			}
			it.Bytes += f.Size
			it.Files++
		}
	}
	// Match. An item is used when any of its keys is a reference's key.
	stems := map[string]int{}
	for _, id := range keys {
		it := items[id]
		stem := clipStem(it.Name)
		stems[stem]++
		for _, k := range []string{stem, proxySuffix.ReplaceAllString(stem, "")} {
			for _, r := range byKey[k] {
				r.found = true
				it.Used = true
			}
		}
	}
	for _, r := range rs {
		if r.found {
			rep.Found++
		} else {
			rep.NotFound = append(rep.NotFound, r.name)
		}
	}
	sort.Strings(rep.NotFound)
	folders := map[string]*ArchiveFolder{}
	var folderKeys []string
	for _, id := range keys {
		it := items[id]
		it.Shared = stems[clipStem(it.Name)] > 1
		src := sources[it.Source]
		top := TopOf(src, it.Path)
		fk := it.DriveID + "\x00" + src.Folder + "\x00" + top
		fo := folders[fk]
		if fo == nil {
			fo = &ArchiveFolder{DriveID: it.DriveID, Source: src.Folder, Folder: top}
			folders[fk] = fo
			folderKeys = append(folderKeys, fk)
		}
		fo.Files++
		fo.Bytes += it.Bytes
		if it.Used {
			rep.UsedFiles++
			rep.UsedBytes += it.Bytes
		} else {
			rep.UnusedFiles++
			rep.UnusedBytes += it.Bytes
			fo.UnusedFiles++
			fo.UnusedBytes += it.Bytes
		}
		rep.Items = append(rep.Items, *it)
	}
	sort.Slice(rep.Items, func(i, j int) bool {
		if rep.Items[i].DriveID != rep.Items[j].DriveID {
			return rep.Items[i].DriveID < rep.Items[j].DriveID
		}
		return rep.Items[i].Path < rep.Items[j].Path
	})
	for _, fk := range folderKeys {
		rep.Folders = append(rep.Folders, *folders[fk])
	}
	sort.Slice(rep.Folders, func(i, j int) bool {
		a, b := rep.Folders[i], rep.Folders[j]
		if a.UnusedBytes != b.UnusedBytes {
			return a.UnusedBytes > b.UnusedBytes
		}
		return a.DriveID+a.Source+a.Folder < b.DriveID+b.Source+b.Folder
	})
	return rep, nil
}

// TopOf is the top-level folder of p below a source folder, or "." for
// a file at the source's top level: the unit the Archive table and its
// folder choice work in.
func TopOf(src Source, p string) string {
	rel := p
	if src.Folder != "" {
		rel = strings.TrimPrefix(p, src.Folder+"/")
	}
	if i := strings.IndexByte(rel, '/'); i >= 0 {
		return rel[:i]
	}
	return "."
}

// likePrefix escapes a path for LIKE ... ESCAPE '\'.
func likePrefix(p string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(p) + "/%"
}

// RemovePaths forgets files, or folders with everything inside, that
// were moved to the Trash. Their clip metadata goes too.
func (db *DB) RemovePaths(driveID string, paths []string) error {
	tx, err := db.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, p := range paths {
		for _, table := range []string{"files", "media"} {
			if _, err := tx.Exec(`DELETE FROM `+table+` WHERE drive_id = ? AND (path = ? OR path LIKE ? ESCAPE '\')`, driveID, p, likePrefix(p)); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// MovePaths records files, or folders with everything inside, moved
// elsewhere on the same drive (from -> to); checksums and clip
// metadata follow them.
func (db *DB) MovePaths(driveID string, moves map[string]string) error {
	tx, err := db.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for from, to := range moves {
		for _, table := range []string{"files", "media"} {
			if _, err := tx.Exec(`UPDATE `+table+` SET path = ? || substr(path, ?) WHERE drive_id = ? AND (path = ? OR path LIKE ? ESCAPE '\')`,
				to, len(from)+1, driveID, from, likePrefix(from)); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`UPDATE files SET name = ? WHERE drive_id = ? AND path = ?`, path.Base(to), driveID, to); err != nil {
			return err
		}
	}
	return tx.Commit()
}
