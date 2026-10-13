// Package catalog is Lumpy's drive library: a SQLite database of every
// drive Lumpy has seen, the files on it and their checksums, so drives can
// be searched and their backups checked while they sit on a shelf.
package catalog

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/lumpysheppard/lumpy/internal/volume"
)

const schema = `
CREATE TABLE IF NOT EXISTS drives (
	id               TEXT PRIMARY KEY,
	name             TEXT NOT NULL,
	size             INTEGER NOT NULL DEFAULT 0,
	fstype           TEXT NOT NULL DEFAULT '',
	first_seen       INTEGER NOT NULL,
	last_seen        INTEGER NOT NULL,
	last_mount       TEXT NOT NULL DEFAULT '',
	last_scan        INTEGER,
	last_verify      INTEGER,
	last_verify_ok   INTEGER,
	verify_requested INTEGER
);
CREATE TABLE IF NOT EXISTS files (
	drive_id    TEXT NOT NULL REFERENCES drives(id) ON DELETE CASCADE,
	path        TEXT NOT NULL,
	name        TEXT NOT NULL,
	size        INTEGER NOT NULL,
	mtime       INTEGER NOT NULL,
	xxh64       TEXT,
	hash_source TEXT,
	hashed_at   INTEGER,
	verified_at INTEGER,
	bad_hash    TEXT,
	PRIMARY KEY (drive_id, path)
);
CREATE INDEX IF NOT EXISTS files_by_hash ON files (xxh64, size);
CREATE INDEX IF NOT EXISTS files_by_name ON files (name, size);
CREATE TABLE IF NOT EXISTS events (
	id       INTEGER PRIMARY KEY,
	drive_id TEXT NOT NULL REFERENCES drives(id) ON DELETE CASCADE,
	at       INTEGER NOT NULL,
	kind     TEXT NOT NULL,
	summary  TEXT NOT NULL,
	failures INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS mounted (
	drive_id TEXT PRIMARY KEY,
	mount    TEXT NOT NULL
);
`

// Hash sources, from most to least trusted.
const (
	SourceVerified = "verified" // Lumpy read the file back and it matched
	SourceHashed   = "hashed"   // Lumpy read the file and recorded its checksum
	SourceMHL      = "mhl"      // taken from an MHL manifest on the drive
)

// DB is an open drive library.
type DB struct{ sql *sql.DB }

// DefaultPath is ~/Library/Application Support/Lumpy/catalog.db, or
// $LUMPY_HOME/catalog.db when LUMPY_HOME is set.
func DefaultPath() (string, error) {
	if h := os.Getenv("LUMPY_HOME"); h != "" {
		return filepath.Join(h, "catalog.db"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "Application Support", "Lumpy", "catalog.db"), nil
}

// Open opens (creating if needed) the library at path.
func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	dsn := "file:" + path + "?_pragma=busy_timeout(30000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	s, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	s.SetMaxOpenConns(1)
	if _, err := s.Exec(schema); err != nil {
		s.Close()
		return nil, fmt.Errorf("opening drive library %s: %w", path, err)
	}
	// Libraries made by 0.2 betas lack bad_hash; adding it twice fails harmlessly.
	s.Exec(`ALTER TABLE files ADD COLUMN bad_hash TEXT`)
	return &DB{sql: s}, nil
}

// Close closes the library.
func (db *DB) Close() error { return db.sql.Close() }

// Drive is one drive in the library, with totals over its files.
type Drive struct {
	ID              string
	Name            string
	Size            int64
	FSType          string
	FirstSeen       time.Time
	LastSeen        time.Time
	LastMount       string
	LastScan        time.Time // zero if never
	LastVerify      time.Time // zero if never
	LastVerifyOK    bool
	VerifyRequested time.Time // zero if no check is queued
	Files           int
	Bytes           int64
	Hashed          int
	Damaged         int // files whose last check did not match
}

func unix(n sql.NullInt64) time.Time {
	if !n.Valid {
		return time.Time{}
	}
	return time.Unix(n.Int64, 0)
}

func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.Unix()
}

const driveCols = `d.id, d.name, d.size, d.fstype, d.first_seen, d.last_seen, d.last_mount,
	d.last_scan, d.last_verify, d.last_verify_ok, d.verify_requested,
	COUNT(f.path), COALESCE(SUM(f.size), 0), COUNT(f.xxh64), COUNT(f.bad_hash)`

func scanDrive(row interface{ Scan(...any) error }) (*Drive, error) {
	var d Drive
	var first, last int64
	var scanAt, verAt, verOK, req sql.NullInt64
	if err := row.Scan(&d.ID, &d.Name, &d.Size, &d.FSType, &first, &last, &d.LastMount,
		&scanAt, &verAt, &verOK, &req, &d.Files, &d.Bytes, &d.Hashed, &d.Damaged); err != nil {
		return nil, err
	}
	d.FirstSeen, d.LastSeen = time.Unix(first, 0), time.Unix(last, 0)
	d.LastScan, d.LastVerify, d.VerifyRequested = unix(scanAt), unix(verAt), unix(req)
	d.LastVerifyOK = verOK.Valid && verOK.Int64 == 1
	return &d, nil
}

// SaveDrive records that v is mounted now, adding it to the library if new
// and updating its name and mount point if they changed.
func (db *DB) SaveDrive(v *volume.Volume) error {
	now := time.Now().Unix()
	_, err := db.sql.Exec(`INSERT INTO drives (id, name, size, fstype, first_seen, last_seen, last_mount)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET name = excluded.name, size = excluded.size,
			fstype = excluded.fstype, last_seen = excluded.last_seen, last_mount = excluded.last_mount`,
		v.ID, v.Name, v.Size, v.FSType, now, now, v.Mount)
	return err
}

// Drive returns the drive with this exact id.
func (db *DB) Drive(id string) (*Drive, error) {
	d, err := scanDrive(db.sql.QueryRow(`SELECT `+driveCols+` FROM drives d
		LEFT JOIN files f ON f.drive_id = d.id WHERE d.id = ? GROUP BY d.id`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("no drive %s in the library", id)
	}
	return d, err
}

// Drives returns every drive in the library, by name.
func (db *DB) Drives() ([]*Drive, error) {
	rows, err := db.sql.Query(`SELECT ` + driveCols + ` FROM drives d
		LEFT JOIN files f ON f.drive_id = d.id GROUP BY d.id ORDER BY d.name COLLATE NOCASE, d.first_seen`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Drive
	for rows.Next() {
		d, err := scanDrive(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Find returns the drive arg names: a drive name (any case), the start of
// a drive id, or a path on a mounted drive.
func (db *DB) Find(arg string) (*Drive, error) {
	if _, err := os.Stat(arg); err == nil && strings.ContainsRune(arg, os.PathSeparator) {
		v, err := volume.Of(arg)
		if err != nil {
			return nil, err
		}
		d, err := db.Drive(v.ID)
		if err != nil {
			return nil, fmt.Errorf("%s (%s) is not in the library yet; run `lumpy catalog %s`", v.Name, v.Mount, v.Mount)
		}
		return d, nil
	}
	all, err := db.Drives()
	if err != nil {
		return nil, err
	}
	var hits []*Drive
	for _, d := range all {
		if strings.EqualFold(d.Name, arg) || strings.EqualFold(d.ID, arg) ||
			(len(arg) >= 4 && strings.HasPrefix(strings.ToUpper(d.ID), strings.ToUpper(arg))) {
			hits = append(hits, d)
		}
	}
	switch len(hits) {
	case 0:
		return nil, fmt.Errorf("no drive called %q in the library (see `lumpy drives`)", arg)
	case 1:
		return hits[0], nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d drives are called %q; use the start of the id instead:", len(hits), arg)
	for _, d := range hits {
		fmt.Fprintf(&b, "\n  %s  %s, last seen %s", d.ID, d.Name, d.LastSeen.Format("2006-01-02"))
	}
	return nil, errors.New(b.String())
}

// Forget removes a drive and everything recorded about it.
func (db *DB) Forget(id string) error {
	_, err := db.sql.Exec(`DELETE FROM drives WHERE id = ?`, id)
	return err
}

// File is one file on a drive.
type File struct {
	Path       string // relative to the drive root, with forward slashes
	Size       int64
	ModTime    int64 // Unix nanoseconds, as last listed
	XXH64      string
	Source     string
	HashedAt   time.Time
	VerifiedAt time.Time
	// BadHash is what the file read as when its last check failed; the
	// file does not count as a copy until a check matches again.
	BadHash string
}

// Files returns every file recorded for a drive, by path.
func (db *DB) Files(driveID string) ([]File, error) {
	rows, err := db.sql.Query(`SELECT path, size, mtime, COALESCE(xxh64, ''), COALESCE(hash_source, ''),
		hashed_at, verified_at, COALESCE(bad_hash, '') FROM files WHERE drive_id = ? ORDER BY path`, driveID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []File
	for rows.Next() {
		var f File
		var h, v sql.NullInt64
		if err := rows.Scan(&f.Path, &f.Size, &f.ModTime, &f.XXH64, &f.Source, &h, &v, &f.BadHash); err != nil {
			return nil, err
		}
		f.HashedAt, f.VerifiedAt = unix(h), unix(v)
		out = append(out, f)
	}
	return out, rows.Err()
}

// SyncResult counts what changed when a drive's listing was refreshed.
type SyncResult struct {
	Added, Changed, Removed, FromMHL int
	// RemovedPaths are files that were recorded before and are gone now.
	RemovedPaths []string
}

// SyncFiles replaces a drive's file list with listing. Files whose size
// and modification time are unchanged keep their checksums; new or changed
// files take one from mhl (path -> xxh64) when it has one. A file that
// changed since it was last listed gets no checksum: an MHL describes it
// as it was, so it must be read again.
func (db *DB) SyncFiles(driveID string, listing []File, mhl map[string]string) (SyncResult, error) {
	var res SyncResult
	old, err := db.Files(driveID)
	if err != nil {
		return res, err
	}
	prev := make(map[string]File, len(old))
	for _, f := range old {
		prev[f.Path] = f
	}
	tx, err := db.sql.Begin()
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	upsert, err := tx.Prepare(`INSERT INTO files (drive_id, path, name, size, mtime, xxh64, hash_source, hashed_at, verified_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL)
		ON CONFLICT (drive_id, path) DO UPDATE SET name = excluded.name, size = excluded.size,
			mtime = excluded.mtime, xxh64 = excluded.xxh64, hash_source = excluded.hash_source,
			hashed_at = excluded.hashed_at, verified_at = NULL, bad_hash = NULL`)
	if err != nil {
		return res, err
	}
	fill, err := tx.Prepare(`UPDATE files SET xxh64 = ?, hash_source = ?, hashed_at = ? WHERE drive_id = ? AND path = ?`)
	if err != nil {
		return res, err
	}
	now := time.Now().Unix()
	for _, f := range listing {
		p, had := prev[f.Path]
		delete(prev, f.Path)
		m, hasMHL := mhl[f.Path]
		if had && p.Size == f.Size && p.ModTime == f.ModTime {
			if p.XXH64 == "" && hasMHL {
				if _, err := fill.Exec(m, SourceMHL, now, driveID, f.Path); err != nil {
					return res, err
				}
				res.FromMHL++
			}
			continue
		}
		if had {
			res.Changed++
		} else {
			res.Added++
		}
		var hash, src, at any
		if hasMHL && !had {
			hash, src, at = m, SourceMHL, now
			res.FromMHL++
		}
		if _, err := upsert.Exec(driveID, f.Path, baseName(f.Path), f.Size, f.ModTime, hash, src, at); err != nil {
			return res, err
		}
	}
	for p := range prev {
		if _, err := tx.Exec(`DELETE FROM files WHERE drive_id = ? AND path = ?`, driveID, p); err != nil {
			return res, err
		}
		res.Removed++
		res.RemovedPaths = append(res.RemovedPaths, p)
	}
	sort.Strings(res.RemovedPaths)
	if _, err := tx.Exec(`UPDATE drives SET last_scan = ? WHERE id = ?`, now, driveID); err != nil {
		return res, err
	}
	return res, tx.Commit()
}

func baseName(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

// HashUpdate records a checksum Lumpy computed for a file.
type HashUpdate struct {
	Path     string
	Size     int64
	ModTime  int64
	XXH64    string
	Verified bool // the file was read back and matched an earlier checksum
	// Bad, when set, is what the file read as when it did not match XXH64,
	// the checksum it should have.
	Bad string
}

// SaveHashes stores checksums Lumpy computed, adding the files if needed.
func (db *DB) SaveHashes(driveID string, updates []HashUpdate) error {
	tx, err := db.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	st, err := tx.Prepare(`INSERT INTO files (drive_id, path, name, size, mtime, xxh64, hash_source, hashed_at, verified_at, bad_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (drive_id, path) DO UPDATE SET size = excluded.size, mtime = excluded.mtime,
			xxh64 = excluded.xxh64, hash_source = excluded.hash_source, hashed_at = excluded.hashed_at,
			verified_at = CASE WHEN excluded.bad_hash IS NOT NULL THEN NULL
				ELSE COALESCE(excluded.verified_at, files.verified_at) END,
			bad_hash = excluded.bad_hash`)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	for _, u := range updates {
		src, ver, bad := SourceHashed, any(nil), any(nil)
		if u.Verified {
			src, ver = SourceVerified, now
		}
		if u.Bad != "" {
			src, ver, bad = SourceVerified, nil, u.Bad
		}
		if _, err := st.Exec(driveID, u.Path, baseName(u.Path), u.Size, u.ModTime, u.XXH64, src, now, ver, bad); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RequestVerify queues (or, with on false, cancels) a full check of a
// drive for the next time it is mounted.
func (db *DB) RequestVerify(driveID string, on bool) error {
	var v any
	if on {
		v = time.Now().Unix()
	}
	_, err := db.sql.Exec(`UPDATE drives SET verify_requested = ? WHERE id = ?`, v, driveID)
	return err
}

// FinishVerify records the outcome of a full check and clears any queued one.
func (db *DB) FinishVerify(driveID string, ok bool) error {
	_, err := db.sql.Exec(`UPDATE drives SET last_verify = ?, last_verify_ok = ?, verify_requested = NULL WHERE id = ?`,
		time.Now().Unix(), map[bool]int{true: 1, false: 0}[ok], driveID)
	return err
}

// Event is one line of a drive's history.
type Event struct {
	At       time.Time
	Kind     string // mounted, scanned, hashed, verified, offload, queued
	Summary  string
	Failures int
}

// Log adds an event to a drive's history.
func (db *DB) Log(driveID, kind, summary string, failures int) error {
	_, err := db.sql.Exec(`INSERT INTO events (drive_id, at, kind, summary, failures) VALUES (?, ?, ?, ?, ?)`,
		driveID, time.Now().Unix(), kind, summary, failures)
	return err
}

// Events returns a drive's most recent events, newest first.
func (db *DB) Events(driveID string, limit int) ([]Event, error) {
	rows, err := db.sql.Query(`SELECT at, kind, summary, failures FROM events WHERE drive_id = ?
		ORDER BY at DESC, id DESC LIMIT ?`, driveID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var at int64
		if err := rows.Scan(&at, &e.Kind, &e.Summary, &e.Failures); err != nil {
			return nil, err
		}
		e.At = time.Unix(at, 0)
		out = append(out, e)
	}
	return out, rows.Err()
}

// Copy is a matching file on another drive.
type Copy struct {
	DriveID   string
	DriveName string
	Path      string
	Verified  bool // that copy was read back and matched
}

// Coverage is how well one file is backed up on other drives.
type Coverage struct {
	File
	// Copies are files on other drives with the same checksum and size.
	Copies []Copy
	// Likely are files on other drives with the same name and size, for a
	// file with no checksum yet. They are probably copies but unproven.
	Likely []Copy
}

// Coverage lists every file on a drive with its copies on other drives.
func (db *DB) Coverage(driveID string) ([]Coverage, error) {
	files, err := db.Files(driveID)
	if err != nil {
		return nil, err
	}
	byHash := map[string][]Copy{}
	byName := map[string][]Copy{}
	rows, err := db.sql.Query(`SELECT o.drive_id, d.name, o.path, o.size, COALESCE(o.xxh64, ''), o.verified_at IS NOT NULL
		FROM files f JOIN files o ON o.drive_id != f.drive_id AND o.bad_hash IS NULL
			AND ((f.xxh64 IS NOT NULL AND o.xxh64 = f.xxh64 AND o.size = f.size)
			  OR (f.xxh64 IS NULL AND o.name = f.name AND o.size = f.size))
		JOIN drives d ON d.id = o.drive_id
		WHERE f.drive_id = ?`, driveID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var c Copy
		var size int64
		var hash string
		if err := rows.Scan(&c.DriveID, &c.DriveName, &c.Path, &size, &hash, &c.Verified); err != nil {
			return nil, err
		}
		k := c.DriveID + "\x00" + c.Path
		if seen[k] {
			continue
		}
		seen[k] = true
		if hash != "" {
			byHash[hash+fmt.Sprint(size)] = append(byHash[hash+fmt.Sprint(size)], c)
		}
		nk := baseName(c.Path) + "\x00" + fmt.Sprint(size)
		byName[nk] = append(byName[nk], c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Coverage, len(files))
	for i, f := range files {
		out[i].File = f
		if f.XXH64 != "" {
			out[i].Copies = byHash[f.XXH64+fmt.Sprint(f.Size)]
		} else {
			out[i].Likely = byName[baseName(f.Path)+"\x00"+fmt.Sprint(f.Size)]
		}
	}
	return out, nil
}

// Match is a file found by name.
type Match struct {
	DriveID   string
	DriveName string
	Path      string
	Size      int64
	XXH64     string
}

// Search finds files whose name contains text (any case), by drive and path.
func (db *DB) Search(text string, limit int) ([]Match, error) {
	esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(text)
	rows, err := db.sql.Query(`SELECT f.drive_id, d.name, f.path, f.size, COALESCE(f.xxh64, '')
		FROM files f JOIN drives d ON d.id = f.drive_id
		WHERE f.name LIKE ? ESCAPE '\' ORDER BY f.name, d.name, f.path LIMIT ?`, "%"+esc+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Match
	for rows.Next() {
		var m Match
		if err := rows.Scan(&m.DriveID, &m.DriveName, &m.Path, &m.Size, &m.XXH64); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// WasMounted returns the drives that were mounted when the agent last
// looked, as id -> mount point.
func (db *DB) WasMounted() (map[string]string, error) {
	rows, err := db.sql.Query(`SELECT drive_id, mount FROM mounted`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, m string
		if err := rows.Scan(&id, &m); err != nil {
			return nil, err
		}
		out[id] = m
	}
	return out, rows.Err()
}

// SetMounted records which drives are mounted now.
func (db *DB) SetMounted(now map[string]string) error {
	tx, err := db.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM mounted`); err != nil {
		return err
	}
	ids := make([]string, 0, len(now))
	for id := range now {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if _, err := tx.Exec(`INSERT INTO mounted (drive_id, mount) VALUES (?, ?)`, id, now[id]); err != nil {
			return err
		}
	}
	return tx.Commit()
}
