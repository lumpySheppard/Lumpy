// Package catalog is Lumpy's drive library: a SQLite database of every
// drive Lumpy has seen, the files on it and their checksums, so drives can
// be searched and their backups checked while they sit on a shelf.
package catalog

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/lumpysheppard/lumpy/internal/smart"
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
CREATE TABLE IF NOT EXISTS media (
	drive_id    TEXT NOT NULL REFERENCES drives(id) ON DELETE CASCADE,
	path        TEXT NOT NULL,
	xxh64       TEXT,
	size        INTEGER NOT NULL DEFAULT 0,
	kind        TEXT NOT NULL DEFAULT '',
	container   TEXT NOT NULL DEFAULT '',
	video_codec TEXT NOT NULL DEFAULT '',
	width       INTEGER NOT NULL DEFAULT 0,
	height      INTEGER NOT NULL DEFAULT 0,
	frame_rate  REAL NOT NULL DEFAULT 0,
	duration_ms INTEGER NOT NULL DEFAULT 0,
	timecode    TEXT NOT NULL DEFAULT '',
	audio_codec TEXT NOT NULL DEFAULT '',
	channels    INTEGER NOT NULL DEFAULT 0,
	sample_rate INTEGER NOT NULL DEFAULT 0,
	bit_depth   INTEGER NOT NULL DEFAULT 0,
	camera      TEXT NOT NULL DEFAULT '',
	reel        TEXT NOT NULL DEFAULT '',
	created     TEXT NOT NULL DEFAULT '',
	extra       TEXT NOT NULL DEFAULT '',
	probed_at   INTEGER NOT NULL DEFAULT 0,
	scene       TEXT NOT NULL DEFAULT '',
	shot        TEXT NOT NULL DEFAULT '',
	take        TEXT NOT NULL DEFAULT '',
	notes       TEXT NOT NULL DEFAULT '',
	tags        TEXT NOT NULL DEFAULT '',
	qc          TEXT NOT NULL DEFAULT '',
	edited_at   INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (drive_id, path)
);
CREATE INDEX IF NOT EXISTS media_by_hash ON media (xxh64, size);
CREATE TABLE IF NOT EXISTS settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS projects (
	id      INTEGER PRIMARY KEY,
	name    TEXT NOT NULL,
	color   TEXT NOT NULL DEFAULT '',
	notes   TEXT NOT NULL DEFAULT '',
	target  INTEGER NOT NULL DEFAULT 2,
	created INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS project_sources (
	project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	drive_id   TEXT NOT NULL REFERENCES drives(id) ON DELETE CASCADE,
	folder     TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (project_id, drive_id, folder)
);
CREATE TABLE IF NOT EXISTS project_dests (
	project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	drive_id   TEXT NOT NULL REFERENCES drives(id) ON DELETE CASCADE,
	PRIMARY KEY (project_id, drive_id)
);
`

// Hash sources, from most to least trusted.
const (
	SourceVerified = "verified" // Lumpy read the file back and it matched
	SourceHashed   = "hashed"   // Lumpy read the file and recorded its checksum
	SourceMHL      = "mhl"      // taken from an MHL manifest on the drive
)

// DB is an open drive library.
type DB struct {
	sql  *sql.DB
	path string
}

// DefaultPath is ~/Library/Application Support/Lumpy/catalog.db, or
// $LUMPY_HOME/catalog.db when LUMPY_HOME is set.
func DefaultPath() (string, error) {
	if h := os.Getenv("LUMPY_HOME"); h != "" {
		return filepath.Join(h, "catalog.db"), nil
	}
	if loc := LibraryLocation(); loc != "" {
		return filepath.Join(loc, "catalog.db"), nil
	}
	dir, err := defaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "catalog.db"), nil
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
	// WAL lets readers run alongside one writer, so a long coverage query
	// for the cards never holds up a click; busy_timeout covers the rest.
	s.SetMaxOpenConns(4)
	if _, err := s.Exec(schema); err != nil {
		s.Close()
		return nil, fmt.Errorf("opening drive library %s: %w", path, err)
	}
	// Projects made before folders were supported listed whole drives.
	s.Exec(`INSERT OR IGNORE INTO project_sources (project_id, drive_id, folder) SELECT project_id, drive_id, '' FROM project_drives`)
	s.Exec(`DROP TABLE IF EXISTS project_drives`)
	// Columns added after the first release; adding one twice fails harmlessly.
	for _, col := range []string{
		`files ADD COLUMN bad_hash TEXT`,
		`drives ADD COLUMN free INTEGER NOT NULL DEFAULT 0`,
		`drives ADD COLUMN nickname TEXT NOT NULL DEFAULT ''`,
		`drives ADD COLUMN client TEXT NOT NULL DEFAULT ''`,
		`drives ADD COLUMN project TEXT NOT NULL DEFAULT ''`,
		`drives ADD COLUMN contact TEXT NOT NULL DEFAULT ''`,
		`drives ADD COLUMN notes TEXT NOT NULL DEFAULT ''`,
		`drives ADD COLUMN color TEXT NOT NULL DEFAULT ''`,
		`drives ADD COLUMN never_log INTEGER NOT NULL DEFAULT 0`,
		`drives ADD COLUMN no_backups INTEGER NOT NULL DEFAULT 0`,
		`drives ADD COLUMN solid_state INTEGER NOT NULL DEFAULT 0`,
		`drives ADD COLUMN bus TEXT NOT NULL DEFAULT ''`,
		`drives ADD COLUMN icon TEXT NOT NULL DEFAULT ''`,
		`drives ADD COLUMN files INTEGER NOT NULL DEFAULT 0`,
		`drives ADD COLUMN bytes INTEGER NOT NULL DEFAULT 0`,
		`drives ADD COLUMN hashed INTEGER NOT NULL DEFAULT 0`,
		`drives ADD COLUMN damaged INTEGER NOT NULL DEFAULT 0`,
		`drives ADD COLUMN health TEXT NOT NULL DEFAULT ''`,
		`project_sources ADD COLUMN role TEXT NOT NULL DEFAULT ''`,
		`project_sources ADD COLUMN exclude TEXT NOT NULL DEFAULT ''`,
		`project_sources ADD COLUMN route TEXT NOT NULL DEFAULT ''`,
	} {
		s.Exec(`ALTER TABLE ` + col)
	}
	// Each drive's file count, size, checksummed and damaged counts are
	// kept on the drive row by triggers, so listing drives never has to
	// count a hundred thousand files.
	if _, err := s.Exec(counters); err != nil {
		s.Close()
		return nil, fmt.Errorf("opening drive library %s: %w", path, err)
	}
	// Clip rows for files that left their drive before the trigger above
	// existed are dropped once.
	var mc string
	if err := s.QueryRow(`SELECT value FROM settings WHERE key = 'media_clean'`).Scan(&mc); err != nil || mc != "1" {
		s.Exec(`DELETE FROM media WHERE NOT EXISTS (SELECT 1 FROM files f WHERE f.drive_id = media.drive_id AND f.path = media.path)`)
		s.Exec(`INSERT OR REPLACE INTO settings (key, value) VALUES ('media_clean', '1')`)
	}
	var v string
	if err := s.QueryRow(`SELECT value FROM settings WHERE key = 'counters'`).Scan(&v); err != nil || v != "1" {
		s.Exec(`UPDATE drives SET files = (SELECT COUNT(*) FROM files WHERE drive_id = drives.id),
			bytes = (SELECT COALESCE(SUM(size), 0) FROM files WHERE drive_id = drives.id),
			hashed = (SELECT COUNT(xxh64) FROM files WHERE drive_id = drives.id),
			damaged = (SELECT COUNT(bad_hash) FROM files WHERE drive_id = drives.id)`)
		s.Exec(`INSERT OR REPLACE INTO settings (key, value) VALUES ('counters', '1')`)
	}
	// Raw formats logged before their codec was implied from the extension.
	for c, codec := range map[string]string{"R3D": "REDCODE RAW", "BRAW": "Blackmagic RAW", "ARRIRAW": "ARRIRAW", "Canon RAW": "Cinema RAW Light"} {
		s.Exec(`UPDATE media SET video_codec = ? WHERE container = ? AND video_codec = ''`, codec, c)
	}
	return &DB{sql: s, path: path}, nil
}

const counters = `
CREATE TRIGGER IF NOT EXISTS files_ai AFTER INSERT ON files BEGIN
	UPDATE drives SET files = files + 1, bytes = bytes + NEW.size,
		hashed = hashed + (NEW.xxh64 IS NOT NULL), damaged = damaged + (NEW.bad_hash IS NOT NULL)
	WHERE id = NEW.drive_id;
END;
CREATE TRIGGER IF NOT EXISTS media_ad AFTER DELETE ON files BEGIN
	DELETE FROM media WHERE drive_id = OLD.drive_id AND path = OLD.path;
END;
CREATE TRIGGER IF NOT EXISTS files_ad AFTER DELETE ON files BEGIN
	UPDATE drives SET files = files - 1, bytes = bytes - OLD.size,
		hashed = hashed - (OLD.xxh64 IS NOT NULL), damaged = damaged - (OLD.bad_hash IS NOT NULL)
	WHERE id = OLD.drive_id;
END;
CREATE TRIGGER IF NOT EXISTS files_au AFTER UPDATE ON files BEGIN
	UPDATE drives SET bytes = bytes - OLD.size + NEW.size,
		hashed = hashed - (OLD.xxh64 IS NOT NULL) + (NEW.xxh64 IS NOT NULL),
		damaged = damaged - (OLD.bad_hash IS NOT NULL) + (NEW.bad_hash IS NOT NULL)
	WHERE id = NEW.drive_id;
END;`

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
	Free            int64     // bytes free when last seen
	SolidState      bool      // an SSD or NVMe drive, when macOS said so
	Bus             string    // USB, Thunderbolt, SATA… as diskutil names it
	// Health is the drive's last SMART reading, when one was taken.
	Health *smart.Health
	Labels
	Files   int
	Bytes   int64
	Hashed  int
	Damaged int // files whose last check did not match
}

// Labels are the notes a person attaches to a drive.
type Labels struct {
	Nickname string `json:"nickname"`
	Client   string `json:"client"`
	Project  string `json:"project"`
	Contact  string `json:"contact"`
	Notes    string `json:"notes"`
	Color    string `json:"color"`
	NeverLog bool   `json:"neverLog"` // the agent leaves this drive alone
	// NoBackups keeps project plans from copying onto this drive: camera
	// cards that get wiped, loaners, the edit SSD.
	NoBackups bool `json:"noBackups"`
	// Icon overrides the picture the card shows: "" (from what macOS
	// says about the drive), hdd, ssd, card or raid.
	Icon string `json:"icon"`
}

// Title is the nickname if the drive has one, else its volume name.
func (d *Drive) Title() string {
	if d.Nickname != "" {
		return d.Nickname
	}
	return d.Name
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
	d.free, d.nickname, d.client, d.project, d.contact, d.notes, d.color, d.never_log, d.no_backups,
	d.solid_state, d.bus, d.icon,
	d.files, d.bytes, d.hashed, d.damaged, d.health`

func scanDrive(row interface{ Scan(...any) error }) (*Drive, error) {
	var d Drive
	var first, last int64
	var scanAt, verAt, verOK, req sql.NullInt64
	var health string
	if err := row.Scan(&d.ID, &d.Name, &d.Size, &d.FSType, &first, &last, &d.LastMount,
		&scanAt, &verAt, &verOK, &req,
		&d.Free, &d.Nickname, &d.Client, &d.Project, &d.Contact, &d.Notes, &d.Color, &d.NeverLog, &d.NoBackups,
		&d.SolidState, &d.Bus, &d.Icon,
		&d.Files, &d.Bytes, &d.Hashed, &d.Damaged, &health); err != nil {
		return nil, err
	}
	if health != "" {
		var h smart.Health
		if json.Unmarshal([]byte(health), &h) == nil {
			d.Health = &h
		}
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
	_, err := db.sql.Exec(`INSERT INTO drives (id, name, size, free, fstype, first_seen, last_seen, last_mount, solid_state, bus)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET name = excluded.name, size = excluded.size, free = excluded.free,
			fstype = excluded.fstype, last_seen = excluded.last_seen, last_mount = excluded.last_mount,
			solid_state = excluded.solid_state, bus = excluded.bus`,
		v.ID, v.Name, v.Size, v.Free, v.FSType, now, now, v.Mount, v.SolidState, v.Bus)
	return err
}

// SetHealth records a drive's SMART reading.
func (db *DB) SetHealth(id string, h *smart.Health) error {
	b, err := json.Marshal(h)
	if err != nil {
		return err
	}
	_, err = db.sql.Exec(`UPDATE drives SET health = ? WHERE id = ?`, string(b), id)
	return err
}

// Drive returns the drive with this exact id.
func (db *DB) Drive(id string) (*Drive, error) {
	d, err := scanDrive(db.sql.QueryRow(`SELECT `+driveCols+` FROM drives d WHERE d.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("no drive %s in the library", id)
	}
	return d, err
}

// Drives returns every drive in the library, by name.
func (db *DB) Drives() ([]*Drive, error) {
	rows, err := db.sql.Query(`SELECT ` + driveCols + ` FROM drives d ORDER BY d.name COLLATE NOCASE, d.first_seen`)
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
		if strings.EqualFold(d.Name, arg) || strings.EqualFold(d.ID, arg) || (d.Nickname != "" && strings.EqualFold(d.Nickname, arg)) ||
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
	// Two joins, each on its own index: checksummed files find copies
	// by checksum and size, the rest find likely copies by name and size.
	byHash := map[string][]Copy{}
	byName := map[string][]Copy{}
	collect := func(query string, key func(c Copy, hash string, size int64) string, into map[string][]Copy) error {
		rows, err := db.sql.Query(query, driveID)
		if err != nil {
			return err
		}
		defer rows.Close()
		seen := map[string]bool{}
		for rows.Next() {
			var c Copy
			var size int64
			var hash string
			if err := rows.Scan(&c.DriveID, &c.DriveName, &c.Path, &size, &hash, &c.Verified); err != nil {
				return err
			}
			k := key(c, hash, size) + "\x00" + c.DriveID + "\x00" + c.Path
			if seen[k] {
				continue
			}
			seen[k] = true
			into[key(c, hash, size)] = append(into[key(c, hash, size)], c)
		}
		return rows.Err()
	}
	const cols = `SELECT o.drive_id, COALESCE(NULLIF(d.nickname, ''), d.name), o.path, o.size, COALESCE(o.xxh64, ''), o.verified_at IS NOT NULL FROM files f `
	if err := collect(cols+`JOIN files o ON o.xxh64 = f.xxh64 AND o.size = f.size AND o.drive_id != f.drive_id AND o.bad_hash IS NULL
		JOIN drives d ON d.id = o.drive_id WHERE f.drive_id = ? AND f.xxh64 IS NOT NULL`,
		func(c Copy, hash string, size int64) string { return hash + fmt.Sprint(size) }, byHash); err != nil {
		return nil, err
	}
	if err := collect(cols+`JOIN files o ON o.name = f.name AND o.size = f.size AND o.drive_id != f.drive_id AND o.bad_hash IS NULL
		JOIN drives d ON d.id = o.drive_id WHERE f.drive_id = ? AND f.xxh64 IS NULL`,
		func(c Copy, hash string, size int64) string { return baseName(c.Path) + "\x00" + fmt.Sprint(size) }, byName); err != nil {
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
	rows, err := db.sql.Query(`SELECT f.drive_id, COALESCE(NULLIF(d.nickname, ''), d.name), f.path, f.size, COALESCE(f.xxh64, '')
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
