package catalog

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// Holder is a drive that holds a good copy of some content, with one
// path on it.
type Holder struct {
	DriveID string `json:"driveId"`
	Path    string `json:"path"`
}

// Origin is where a project's source folder holds the content: the
// source (by index), the path on that drive, and the path relative to
// the source folder.
type Origin struct {
	Source  int    `json:"source"`
	DriveID string `json:"driveId"`
	Path    string `json:"path"`
	Rel     string `json:"rel"`
}

// Content is one distinct file, identified by its checksum and size.
// Files with no checksum are grouped by name and size instead and marked
// Unhashed: they may or may not be copies of each other.
type Content struct {
	Key      string   `json:"-"`
	Name     string   `json:"name"`
	Size     int64    `json:"size"`
	XXH64    string   `json:"xxh64,omitempty"`
	Unhashed bool     `json:"unhashed,omitempty"`
	Holders  []Holder `json:"holders"` // one per drive, anywhere in the library
	Origins  []Origin `json:"-"`       // where the project's folders hold it
}

// Copies is how many different drives hold a good copy.
func (c *Content) Copies() int { return len(c.Holders) }

// Has reports whether a drive holds the content.
func (c *Content) Has(driveID string) bool {
	for _, h := range c.Holders {
		if h.DriveID == driveID {
			return true
		}
	}
	return false
}

func (c *Content) addHolder(driveID, p string) {
	if !c.Has(driveID) {
		c.Holders = append(c.Holders, Holder{driveID, p})
	}
}

// SourceHealth is one source folder's share of a project, by bytes.
type SourceHealth struct {
	Source
	Files        int   `json:"files"`
	Bytes        int64 `json:"bytes"`
	Backed       int   `json:"backed"`
	BackedBytes  int64 `json:"backedBytes"`
	Short        int   `json:"short"`
	ShortBytes   int64 `json:"shortBytes"`
	Sole         int   `json:"sole"`
	SoleBytes    int64 `json:"soleBytes"`
	Unknown      int   `json:"unknown"`
	UnknownBytes int64 `json:"unknownBytes"`
	Damaged      int   `json:"damaged"`
	// Extra counts files with more than Target copies: space that could
	// be reclaimed if needed.
	Extra      int   `json:"extra"`
	ExtraBytes int64 `json:"extraBytes"`
}

// FolderHealth is one top-level folder of a drive, by bytes.
type FolderHealth struct {
	Name         string `json:"name"`
	Folder       bool   `json:"folder"`
	Files        int    `json:"files"`
	Bytes        int64  `json:"bytes"`
	BackedBytes  int64  `json:"backedBytes"`
	SoleBytes    int64  `json:"soleBytes"`
	UnknownBytes int64  `json:"unknownBytes"`
	Damaged      int    `json:"damaged"`
}

// Health is how well a project is backed up: every distinct file in its
// folders should be on at least Target different drives, counting good
// copies anywhere in the library.
type Health struct {
	Target   int   `json:"target"`
	Contents int   `json:"contents"` // distinct files
	Bytes    int64 `json:"bytes"`    // of distinct files

	Backed       int   `json:"backed"` // on Target or more drives
	BackedBytes  int64 `json:"backedBytes"`
	Short        int   `json:"short"` // on 2 or more drives, fewer than Target
	ShortBytes   int64 `json:"shortBytes"`
	Sole         int   `json:"sole"` // on one drive only
	SoleBytes    int64 `json:"soleBytes"`
	Unknown      int   `json:"unknown"` // no checksum, so copies cannot be proven
	UnknownBytes int64 `json:"unknownBytes"`
	Damaged      int   `json:"damaged"` // copies in the folders that failed their last check
	Lost         int   `json:"lost"`    // files whose every known copy is damaged
	LostBytes    int64 `json:"lostBytes"`

	Sources []SourceHealth `json:"sources"`
	// Needs are the contents with fewer than Target copies, largest first.
	Needs []*Content `json:"-"`
	// Unhashed are contents with no checksum, grouped by name and size.
	Unhashed []*Content `json:"-"`
}

type fileRow struct {
	drive, p, name, hash string
	size                 int64
	bad                  bool
}

// under lists the files below a source folder.
func (db *DB) under(src Source) ([]fileRow, error) {
	prefix := src.Folder
	if prefix != "" {
		prefix += "/"
	}
	rows, err := db.sql.Query(`SELECT path, name, size, COALESCE(xxh64, ''), bad_hash IS NOT NULL
		FROM files WHERE drive_id = ? AND path >= ? AND path < ? ORDER BY path`, src.DriveID, prefix, prefix+"\U0010FFFF")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []fileRow
	for rows.Next() {
		f := fileRow{drive: src.DriveID}
		if err := rows.Scan(&f.p, &f.name, &f.size, &f.hash, &f.bad); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// holdersOf finds, anywhere in the library, the good copies of the
// checksummed files below a source folder, keyed like Content.Key.
func (db *DB) holdersOf(src Source, add func(key, driveID, p string)) error {
	prefix := src.Folder
	if prefix != "" {
		prefix += "/"
	}
	rows, err := db.sql.Query(`SELECT o.xxh64, o.size, o.drive_id, o.path
		FROM files f JOIN files o ON o.xxh64 = f.xxh64 AND o.size = f.size AND o.bad_hash IS NULL
		WHERE f.drive_id = ? AND f.path >= ? AND f.path < ? AND f.xxh64 IS NOT NULL`, src.DriveID, prefix, prefix+"\U0010FFFF")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var hash, drive, p string
		var size int64
		if err := rows.Scan(&hash, &size, &drive, &p); err != nil {
			return err
		}
		add("h:"+hash+":"+fmt.Sprint(size), drive, p)
	}
	return rows.Err()
}

// ProjectHealth measures how well the files in sources are backed up.
func (db *DB) ProjectHealth(sources []Source, target int) (*Health, error) {
	if target < 1 {
		target = 1
	}
	h := &Health{Target: target}
	byKey := map[string]*Content{}
	var order []*Content
	perSource := make([]*SourceHealth, len(sources))
	// which contents each source holds, by index, in first-seen order
	inSource := make([][]*Content, len(sources))

	for i, src := range sources {
		perSource[i] = &SourceHealth{Source: src}
		rows, err := db.under(src)
		if err != nil {
			return nil, err
		}
		prefix := src.Folder
		if prefix != "" {
			prefix += "/"
		}
		seenHere := map[string]bool{}
		for _, f := range rows {
			if f.bad {
				h.Damaged++
				perSource[i].Damaged++
			}
			var key string
			if f.hash != "" {
				key = "h:" + f.hash + ":" + fmt.Sprint(f.size)
			} else {
				key = "n:" + strings.ToLower(f.name) + ":" + fmt.Sprint(f.size)
			}
			c := byKey[key]
			if c == nil {
				c = &Content{Key: key, Name: f.name, Size: f.size, XXH64: f.hash, Unhashed: f.hash == ""}
				byKey[key] = c
				order = append(order, c)
			}
			if !f.bad {
				c.Origins = append(c.Origins, Origin{Source: i, DriveID: f.drive, Path: f.p, Rel: f.p[len(prefix):]})
				if c.Unhashed {
					c.addHolder(f.drive, f.p)
				}
			}
			if !seenHere[key] {
				seenHere[key] = true
				inSource[i] = append(inSource[i], c)
			}
		}
		if err := db.holdersOf(src, func(key, driveID, p string) {
			if c := byKey[key]; c != nil {
				c.addHolder(driveID, p)
			}
		}); err != nil {
			return nil, err
		}
	}

	class := func(c *Content) string {
		switch {
		case c.Unhashed:
			return "unknown"
		case c.Copies() == 0:
			return "lost"
		case c.Copies() >= target:
			return "backed"
		case c.Copies() > 1:
			return "short"
		default:
			return "sole"
		}
	}
	for _, c := range order {
		h.Contents++
		h.Bytes += c.Size
		switch class(c) {
		case "unknown":
			h.Unknown++
			h.UnknownBytes += c.Size
			h.Unhashed = append(h.Unhashed, c)
		case "lost":
			h.Lost++
			h.LostBytes += c.Size
		case "backed":
			h.Backed++
			h.BackedBytes += c.Size
		case "short":
			h.Short++
			h.ShortBytes += c.Size
			h.Needs = append(h.Needs, c)
		case "sole":
			h.Sole++
			h.SoleBytes += c.Size
			h.Needs = append(h.Needs, c)
		}
	}
	for i := range sources {
		sh := perSource[i]
		for _, c := range inSource[i] {
			sh.Files++
			sh.Bytes += c.Size
			switch class(c) {
			case "unknown":
				sh.Unknown++
				sh.UnknownBytes += c.Size
			case "backed":
				sh.Backed++
				sh.BackedBytes += c.Size
				if c.Copies() > target {
					sh.Extra++
					sh.ExtraBytes += c.Size
				}
			case "short":
				sh.Short++
				sh.ShortBytes += c.Size
			case "sole":
				sh.Sole++
				sh.SoleBytes += c.Size
			}
		}
		h.Sources = append(h.Sources, *sh)
	}
	sort.SliceStable(h.Needs, func(i, j int) bool { return h.Needs[i].Size > h.Needs[j].Size })
	return h, nil
}

// FolderHealths splits one drive's files by top-level folder, counting a
// file as backed up when another drive anywhere in the library holds it.
func (db *DB) FolderHealths(driveID string) ([]FolderHealth, error) {
	cov, err := db.Coverage(driveID)
	if err != nil {
		return nil, err
	}
	byName := map[string]*FolderHealth{}
	var order []*FolderHealth
	for _, c := range cov {
		top := c.Path
		folder := false
		if i := strings.IndexByte(c.Path, '/'); i >= 0 {
			top, folder = c.Path[:i], true
		}
		f := byName[top]
		if f == nil {
			f = &FolderHealth{Name: top, Folder: folder}
			byName[top] = f
			order = append(order, f)
		}
		f.Files++
		f.Bytes += c.Size
		switch {
		case c.BadHash != "":
			f.Damaged++
		case c.XXH64 == "":
			f.UnknownBytes += c.Size
		case len(c.Copies) > 0:
			f.BackedBytes += c.Size
		default:
			f.SoleBytes += c.Size
		}
	}
	sort.SliceStable(order, func(i, j int) bool { return order[i].Bytes > order[j].Bytes })
	out := make([]FolderHealth, len(order))
	for i, f := range order {
		out[i] = *f
	}
	return out, nil
}

// TopFolder is the first path component, used to summarise plans. A file
// at the top gets ".".
func TopFolder(p string) string {
	if i := strings.IndexByte(p, '/'); i >= 0 {
		return p[:i]
	}
	_ = path.Base
	return "."
}
