package catalog

import (
	"sort"
	"strings"
)

// SetLabels replaces a drive's labels.
func (db *DB) SetLabels(id string, l Labels) error {
	_, err := db.sql.Exec(`UPDATE drives SET nickname = ?, client = ?, project = ?, contact = ?,
		notes = ?, color = ?, never_log = ?, no_backups = ?, icon = ? WHERE id = ?`,
		strings.TrimSpace(l.Nickname), strings.TrimSpace(l.Client), strings.TrimSpace(l.Project),
		strings.TrimSpace(l.Contact), strings.TrimSpace(l.Notes), l.Color, l.NeverLog, l.NoBackups, l.Icon, id)
	return err
}

// Slice is one top-level folder's share of a drive.
type Slice struct {
	Name   string `json:"name"`
	Bytes  int64  `json:"bytes"`
	Files  int    `json:"files"`
	Folder bool   `json:"folder"` // false for a file at the top of the drive
}

// Breakdown sums a drive's files by top-level folder, largest first.
func (db *DB) Breakdown(driveID string) ([]Slice, error) {
	rows, err := db.sql.Query(`SELECT
			CASE WHEN instr(path, '/') > 0 THEN substr(path, 1, instr(path, '/') - 1) ELSE path END,
			instr(path, '/') > 0, SUM(size), COUNT(*)
		FROM files WHERE drive_id = ? GROUP BY 1, 2 ORDER BY 3 DESC`, driveID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Slice
	for rows.Next() {
		var s Slice
		if err := rows.Scan(&s.Name, &s.Folder, &s.Bytes, &s.Files); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Entry is one item in a folder of a drive.
type Entry struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Folder   bool   `json:"folder"`
	Bytes    int64  `json:"bytes"`
	Files    int    `json:"files"` // files inside, for a folder
	XXH64    string `json:"xxh64,omitempty"`
	Verified bool   `json:"verified,omitempty"`
	Damaged  bool   `json:"damaged,omitempty"`
	// Backup state, from copies elsewhere in the library: how many files
	// inside (or this file, 0 or 1) have a copy on another drive, how
	// many have no checksum, how many damaged copies, and the bytes of
	// files with a copy.
	Backed      int   `json:"backed"`
	BackedBytes int64 `json:"backedBytes"`
	Unknown     int   `json:"unknown"` // no checksum (Likely among them)
	Likely      int   `json:"likely"`  // no checksum, but a same-name, same-size file on another drive
	LikelyBytes int64 `json:"likelyBytes"`
	Copies      int   `json:"copies,omitempty"` // for a file: drives holding a copy
}

// FileState is what the library knows about one file's backups.
type FileState struct {
	Copies  int  // other drives holding a good copy
	Likely  int  // other drives holding a same-name, same-size file, for a file with no checksum
	Unknown bool // no checksum
	Damaged bool // failed its last check
}

// BackupStates returns the backup state of every file on a drive, by path.
func (db *DB) BackupStates(driveID string) (map[string]FileState, error) {
	cov, err := db.Coverage(driveID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]FileState, len(cov))
	for _, c := range cov {
		out[c.Path] = FileState{Copies: len(c.Copies), Likely: len(c.Likely), Unknown: c.XXH64 == "", Damaged: c.BadHash != ""}
	}
	return out, nil
}

// List returns what a folder of a drive holds, folders first. dir is a
// drive-relative path; "" is the top of the drive. With filter set it
// instead returns every file whose name contains filter (any case).
// states, when given, fills in each entry's backup state.
func (db *DB) List(driveID, dir, filter string, limit int, states map[string]FileState) ([]Entry, error) {
	mark := func(e *Entry) {
		st, ok := states[e.Path]
		if !ok {
			return
		}
		e.Copies = st.Copies
		if st.Copies > 0 {
			e.Backed, e.BackedBytes = 1, e.Bytes
		}
		if st.Unknown {
			e.Unknown = 1
			if st.Likely > 0 {
				e.Likely, e.LikelyBytes = 1, e.Bytes
			}
		}
	}
	if filter != "" {
		esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(filter)
		rows, err := db.sql.Query(`SELECT path, size, COALESCE(xxh64, ''), verified_at IS NOT NULL, bad_hash IS NOT NULL
			FROM files WHERE drive_id = ? AND name LIKE ? ESCAPE '\' ORDER BY path LIMIT ?`, driveID, "%"+esc+"%", limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []Entry
		for rows.Next() {
			var e Entry
			if err := rows.Scan(&e.Path, &e.Bytes, &e.XXH64, &e.Verified, &e.Damaged); err != nil {
				return nil, err
			}
			e.Name = e.Path
			e.Files = 1
			mark(&e)
			out = append(out, e)
		}
		return out, rows.Err()
	}

	prefix := ""
	if dir != "" {
		prefix = strings.TrimSuffix(dir, "/") + "/"
	}
	// The primary key orders files by path, so a range scan reads only
	// this folder's files.
	rows, err := db.sql.Query(`SELECT path, size, COALESCE(xxh64, ''), verified_at IS NOT NULL, bad_hash IS NOT NULL
		FROM files WHERE drive_id = ? AND path >= ? AND path < ?`, driveID, prefix, prefix+"\U0010FFFF")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	folders := map[string]*Entry{}
	var files []Entry
	for rows.Next() {
		var p, hash string
		var size int64
		var ver, bad bool
		if err := rows.Scan(&p, &size, &hash, &ver, &bad); err != nil {
			return nil, err
		}
		rest := p[len(prefix):]
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			name := rest[:i]
			f := folders[name]
			if f == nil {
				f = &Entry{Name: name, Path: prefix + name, Folder: true}
				folders[name] = f
			}
			f.Bytes += size
			f.Files++
			f.Damaged = f.Damaged || bad
			if st, ok := states[p]; ok {
				if st.Copies > 0 {
					f.Backed++
					f.BackedBytes += size
				}
				if st.Unknown {
					f.Unknown++
					if st.Likely > 0 {
						f.Likely++
						f.LikelyBytes += size
					}
				}
			}
			continue
		}
		e := Entry{Name: rest, Path: p, Bytes: size, Files: 1, XXH64: hash, Verified: ver, Damaged: bad}
		mark(&e)
		files = append(files, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(folders)+len(files))
	for _, f := range folders {
		out = append(out, *f)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	sort.Slice(files, func(i, j int) bool { return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name) })
	out = append(out, files...)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// BackupSummary counts how well a drive's files are backed up elsewhere.
type BackupSummary struct {
	Files int   `json:"files"`
	Bytes int64 `json:"bytes"`
	// Byte totals of the categories below, for the ratio bar.
	EnoughBytes  int64 `json:"enoughBytes"`
	ShortBytes   int64 `json:"shortBytes"`
	NoneBytes    int64 `json:"noneBytes"`
	LikelyBytes  int64 `json:"likelyBytes"`
	UnknownBytes int64 `json:"unknownBytes"`
	DamagedBytes int64 `json:"damagedBytes"`

	Enough  int `json:"enough"`  // checksummed, with at least Min copies
	Short   int `json:"short"`   // checksummed, with fewer copies
	None    int `json:"none"`    // checksummed, with no copy
	Likely  int `json:"likely"`  // no checksum, same name and size elsewhere
	Unknown int `json:"unknown"` // no checksum and nothing similar elsewhere
	Damaged int `json:"damaged"` // failed their last check on this drive
	Min     int `json:"min"`     // copies asked for
	// Fewest copies any checksummed file has elsewhere; -1 when no file
	// has a checksum.
	MinCopies int            `json:"minCopies"`
	Holders   map[string]int `json:"holders"`  // drive title -> files it holds copies of
	Problems  []string       `json:"problems"` // paths that are short, missing or damaged (first 200)
}

// Backups summarises Coverage for a drive.
func (db *DB) Backups(driveID string, min int) (*BackupSummary, error) {
	cov, err := db.Coverage(driveID)
	if err != nil {
		return nil, err
	}
	titles := map[string]string{}
	ds, err := db.Drives()
	if err != nil {
		return nil, err
	}
	for _, d := range ds {
		titles[d.ID] = d.Title()
	}
	s := &BackupSummary{Files: len(cov), Min: min, MinCopies: -1, Holders: map[string]int{}}
	for _, c := range cov {
		bad := false
		s.Bytes += c.Size
		if c.XXH64 != "" && (s.MinCopies < 0 || len(c.Copies) < s.MinCopies) {
			s.MinCopies = len(c.Copies)
		}
		switch {
		case c.BadHash != "":
			s.Damaged++
			s.DamagedBytes += c.Size
			bad = true
		case c.XXH64 != "" && len(c.Copies) >= min:
			s.Enough++
			s.EnoughBytes += c.Size
		case c.XXH64 != "" && len(c.Copies) > 0:
			s.Short++
			s.ShortBytes += c.Size
			bad = true
		case c.XXH64 != "":
			s.None++
			s.NoneBytes += c.Size
			bad = true
		case len(c.Likely) > 0:
			s.Likely++
			s.LikelyBytes += c.Size
		default:
			s.Unknown++
			s.UnknownBytes += c.Size
			bad = true
		}
		seen := map[string]bool{}
		for _, cp := range c.Copies {
			if !seen[cp.DriveID] {
				s.Holders[titles[cp.DriveID]]++
				seen[cp.DriveID] = true
			}
		}
		if bad && len(s.Problems) < 200 {
			s.Problems = append(s.Problems, c.Path)
		}
	}
	return s, nil
}
