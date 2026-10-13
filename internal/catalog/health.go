package catalog

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// Holder is one drive's copy of a piece of content.
type Holder struct {
	DriveID string `json:"driveId"`
	Path    string `json:"path"`
}

// Content is one distinct file, identified by its checksum and size, and
// the drives that hold a good copy of it. Files with no checksum are
// grouped by name and size instead and marked Unhashed: they may or may
// not be copies of each other.
type Content struct {
	Key      string   `json:"-"`
	Name     string   `json:"name"`
	Size     int64    `json:"size"`
	XXH64    string   `json:"xxh64,omitempty"`
	Unhashed bool     `json:"unhashed,omitempty"`
	Holders  []Holder `json:"holders"` // one per drive, the first path found
}

// Copies is how many different drives hold the content.
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

// DriveHealth is one drive's share of a project, by bytes.
type DriveHealth struct {
	ID           string `json:"id"`
	Files        int    `json:"files"`
	Bytes        int64  `json:"bytes"`
	Backed       int    `json:"backed"`
	BackedBytes  int64  `json:"backedBytes"`
	Short        int    `json:"short"`
	ShortBytes   int64  `json:"shortBytes"`
	Sole         int    `json:"sole"`
	SoleBytes    int64  `json:"soleBytes"`
	Unknown      int    `json:"unknown"`
	UnknownBytes int64  `json:"unknownBytes"`
	Damaged      int    `json:"damaged"`
	// Extra counts files held on this drive that have more than Target
	// copies: space that could be reclaimed if needed.
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

// Health is how well a set of drives backs itself up: every distinct
// file should be on at least Target different drives.
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
	Damaged      int   `json:"damaged"` // copies that failed their last check

	Drives []DriveHealth `json:"drives"`
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

func (db *DB) filesOn(driveIDs []string) ([]fileRow, error) {
	if len(driveIDs) == 0 {
		return nil, nil
	}
	args := make([]any, len(driveIDs))
	marks := make([]string, len(driveIDs))
	for i, id := range driveIDs {
		args[i], marks[i] = id, "?"
	}
	rows, err := db.sql.Query(`SELECT drive_id, path, name, size, COALESCE(xxh64, ''), bad_hash IS NOT NULL
		FROM files WHERE drive_id IN (`+strings.Join(marks, ",")+`) ORDER BY drive_id, path`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []fileRow
	for rows.Next() {
		var f fileRow
		if err := rows.Scan(&f.drive, &f.p, &f.name, &f.size, &f.hash, &f.bad); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// contents groups files into distinct contents.
func contents(rows []fileRow) (hashed, unhashed []*Content, damaged int) {
	byKey := map[string]*Content{}
	var order []*Content
	for _, f := range rows {
		if f.bad {
			damaged++
			continue
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
		if !c.Has(f.drive) {
			c.Holders = append(c.Holders, Holder{f.drive, f.p})
		}
	}
	for _, c := range order {
		if c.Unhashed {
			unhashed = append(unhashed, c)
		} else {
			hashed = append(hashed, c)
		}
	}
	return hashed, unhashed, damaged
}

// ProjectHealth measures how well driveIDs back each other up.
func (db *DB) ProjectHealth(driveIDs []string, target int) (*Health, error) {
	if target < 1 {
		target = 1
	}
	rows, err := db.filesOn(driveIDs)
	if err != nil {
		return nil, err
	}
	hashed, unhashed, damaged := contents(rows)
	h := &Health{Target: target, Damaged: damaged}
	per := map[string]*DriveHealth{}
	for _, id := range driveIDs {
		per[id] = &DriveHealth{ID: id}
	}
	for _, f := range rows {
		if f.bad {
			if d := per[f.drive]; d != nil {
				d.Damaged++
			}
		}
	}
	for _, c := range hashed {
		h.Contents++
		h.Bytes += c.Size
		n := c.Copies()
		switch {
		case n >= target:
			h.Backed++
			h.BackedBytes += c.Size
		case n > 1:
			h.Short++
			h.ShortBytes += c.Size
			h.Needs = append(h.Needs, c)
		default:
			h.Sole++
			h.SoleBytes += c.Size
			h.Needs = append(h.Needs, c)
		}
		for _, hd := range c.Holders {
			d := per[hd.DriveID]
			if d == nil {
				continue
			}
			d.Files++
			d.Bytes += c.Size
			switch {
			case n >= target:
				d.Backed++
				d.BackedBytes += c.Size
				if n > target {
					d.Extra++
					d.ExtraBytes += c.Size
				}
			case n > 1:
				d.Short++
				d.ShortBytes += c.Size
			default:
				d.Sole++
				d.SoleBytes += c.Size
			}
		}
	}
	for _, c := range unhashed {
		h.Contents++
		h.Bytes += c.Size
		h.Unknown++
		h.UnknownBytes += c.Size
		h.Unhashed = append(h.Unhashed, c)
		for _, hd := range c.Holders {
			if d := per[hd.DriveID]; d != nil {
				d.Files++
				d.Bytes += c.Size
				d.Unknown++
				d.UnknownBytes += c.Size
			}
		}
	}
	sort.SliceStable(h.Needs, func(i, j int) bool { return h.Needs[i].Size > h.Needs[j].Size })
	for _, id := range driveIDs {
		h.Drives = append(h.Drives, *per[id])
	}
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

// TopFolder is the first path component, used to summarise plans.
func TopFolder(p string) string {
	if i := strings.IndexByte(p, '/'); i >= 0 {
		return p[:i]
	}
	return path.Base(p)
}
