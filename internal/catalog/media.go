package catalog

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/lumpysheppard/lumpy/internal/media"
)

// Media is what the library knows about one clip: what was read from the
// file, and what a person added.
type Media struct {
	DriveID string `json:"driveId"`
	Path    string `json:"path"`
	XXH64   string `json:"xxh64,omitempty"`
	Size    int64  `json:"size"`
	media.Info
	ProbedAt time.Time  `json:"-"`
	Edits    MediaEdits `json:"edits"`
}

// MediaEdits are the fields a person fills in. They follow the content:
// an edit on one copy is applied to every copy with the same checksum.
type MediaEdits struct {
	Scene string `json:"scene"`
	Shot  string `json:"shot"`
	Take  string `json:"take"`
	Notes string `json:"notes"`
	Tags  string `json:"tags"` // comma separated
	QC    string `json:"qc"`   // "", "ok", "flag", "reject"
}

const mediaCols = `drive_id, path, COALESCE(xxh64, ''), size, kind, container, video_codec, width, height, frame_rate,
	duration_ms, timecode, audio_codec, channels, sample_rate, bit_depth, camera, reel, created, extra, probed_at,
	scene, shot, take, notes, tags, qc`

func scanMedia(row interface{ Scan(...any) error }) (*Media, error) {
	var m Media
	var extra string
	var probed int64
	if err := row.Scan(&m.DriveID, &m.Path, &m.XXH64, &m.Size, &m.Kind, &m.Container, &m.VideoCodec, &m.Width, &m.Height, &m.FrameRate,
		&m.DurationMS, &m.Timecode, &m.AudioCodec, &m.Channels, &m.SampleRate, &m.BitDepth, &m.Camera, &m.Reel, &m.Created, &extra, &probed,
		&m.Edits.Scene, &m.Edits.Shot, &m.Edits.Take, &m.Edits.Notes, &m.Edits.Tags, &m.Edits.QC); err != nil {
		return nil, err
	}
	if extra != "" {
		json.Unmarshal([]byte(extra), &m.Extra)
	}
	m.ProbedAt = time.Unix(probed, 0)
	return &m, nil
}

// SaveMedia records what was read from a file. Fields a person edited
// are kept; scene and take read from the file fill them in only when
// they are empty.
func (db *DB) SaveMedia(driveID, path, xxh64 string, size int64, info *media.Info) error {
	extra := ""
	if len(info.Extra) > 0 {
		b, _ := json.Marshal(info.Extra)
		extra = string(b)
	}
	var hash any
	if xxh64 != "" {
		hash = xxh64
	}
	_, err := db.sql.Exec(`INSERT INTO media (drive_id, path, xxh64, size, kind, container, video_codec, width, height, frame_rate,
			duration_ms, timecode, audio_codec, channels, sample_rate, bit_depth, camera, reel, created, extra, probed_at, scene, take)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (drive_id, path) DO UPDATE SET xxh64 = COALESCE(excluded.xxh64, media.xxh64), size = excluded.size,
			kind = excluded.kind, container = excluded.container, video_codec = excluded.video_codec, width = excluded.width,
			height = excluded.height, frame_rate = excluded.frame_rate, duration_ms = excluded.duration_ms, timecode = excluded.timecode,
			audio_codec = excluded.audio_codec, channels = excluded.channels, sample_rate = excluded.sample_rate, bit_depth = excluded.bit_depth,
			camera = excluded.camera, reel = excluded.reel, created = excluded.created, extra = excluded.extra, probed_at = excluded.probed_at,
			scene = CASE WHEN media.scene = '' THEN excluded.scene ELSE media.scene END,
			take = CASE WHEN media.take = '' THEN excluded.take ELSE media.take END`,
		driveID, path, hash, size, string(info.Kind), info.Container, info.VideoCodec, info.Width, info.Height, info.FrameRate,
		info.DurationMS, info.Timecode, info.AudioCodec, info.Channels, info.SampleRate, info.BitDepth, info.Camera, info.Reel,
		info.Created, extra, time.Now().Unix(), info.Scene, info.Take)
	return err
}

// CopyMedia carries a file's metadata and edits to a new copy of it.
func (db *DB) CopyMedia(fromDrive, fromPath, toDrive, toPath string) error {
	_, err := db.sql.Exec(`INSERT OR REPLACE INTO media (drive_id, path, xxh64, size, kind, container, video_codec, width, height, frame_rate,
			duration_ms, timecode, audio_codec, channels, sample_rate, bit_depth, camera, reel, created, extra, probed_at,
			scene, shot, take, notes, tags, qc, edited_at)
		SELECT ?, ?, xxh64, size, kind, container, video_codec, width, height, frame_rate,
			duration_ms, timecode, audio_codec, channels, sample_rate, bit_depth, camera, reel, created, extra, probed_at,
			scene, shot, take, notes, tags, qc, edited_at FROM media WHERE drive_id = ? AND path = ?`,
		toDrive, toPath, fromDrive, fromPath)
	return err
}

// MediaFor returns a file's metadata, if it has been read.
func (db *DB) MediaFor(driveID, path string) (*Media, error) {
	m, err := scanMedia(db.sql.QueryRow(`SELECT `+mediaCols+` FROM media WHERE drive_id = ? AND path = ?`, driveID, path))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return m, err
}

// SetMediaEdits saves a person's fields for a file and for every other
// copy of the same content in the library.
func (db *DB) SetMediaEdits(driveID, path string, e MediaEdits) error {
	m, err := db.MediaFor(driveID, path)
	if err != nil {
		return err
	}
	if m == nil {
		return fmt.Errorf("no metadata for %s", path)
	}
	now := time.Now().Unix()
	if m.XXH64 != "" {
		_, err = db.sql.Exec(`UPDATE media SET scene = ?, shot = ?, take = ?, notes = ?, tags = ?, qc = ?, edited_at = ?
			WHERE xxh64 = ? AND size = ?`, e.Scene, e.Shot, e.Take, e.Notes, e.Tags, e.QC, now, m.XXH64, m.Size)
	} else {
		_, err = db.sql.Exec(`UPDATE media SET scene = ?, shot = ?, take = ?, notes = ?, tags = ?, qc = ?, edited_at = ?
			WHERE drive_id = ? AND path = ?`, e.Scene, e.Shot, e.Take, e.Notes, e.Tags, e.QC, now, driveID, path)
	}
	return err
}

// Unprobed lists a drive's files whose metadata Lumpy could read but has
// not yet, by path.
func (db *DB) Unprobed(driveID string) ([]File, error) {
	rows, err := db.sql.Query(`SELECT f.path, f.size, f.mtime, COALESCE(f.xxh64, '') FROM files f
		LEFT JOIN media m ON m.drive_id = f.drive_id AND m.path = f.path AND m.size = f.size
		WHERE f.drive_id = ? AND m.path IS NULL ORDER BY f.path`, driveID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []File
	for rows.Next() {
		var f File
		if err := rows.Scan(&f.Path, &f.Size, &f.ModTime, &f.XXH64); err != nil {
			return nil, err
		}
		if media.Probable(f.Path) {
			out = append(out, f)
		}
	}
	return out, rows.Err()
}

// MediaFilter narrows a media listing.
type MediaFilter struct {
	DriveID   string
	Project   *Project // files under the project's folders
	Kind      string
	Container string // format: QuickTime, R3D, BWF…
	Codec     string
	FrameRate float64
	QC        string
	Text      string // matches path, camera, reel, scene, notes, tags
	Limit     int
}

// MediaRow is one file in a media listing, with the drives that hold it.
type MediaRow struct {
	Media
	Copies int `json:"copies"` // other drives with a good copy
}

// ListMedia returns files with metadata, one row per distinct content
// where the checksum is known (the first drive by name), by path.
func (db *DB) ListMedia(f MediaFilter) ([]MediaRow, error) {
	var where []string
	var args []any
	if f.DriveID != "" {
		where = append(where, "m.drive_id = ?")
		args = append(args, f.DriveID)
	}
	if f.Project != nil {
		var ors []string
		for _, s := range f.Project.Sources {
			prefix := s.Folder
			if prefix != "" {
				prefix += "/"
			}
			ors = append(ors, "(m.drive_id = ? AND m.path >= ? AND m.path < ?)")
			args = append(args, s.DriveID, prefix, prefix+"\U0010FFFF")
		}
		if len(ors) == 0 {
			return []MediaRow{}, nil
		}
		where = append(where, "("+strings.Join(ors, " OR ")+")")
	}
	if f.Kind != "" {
		where = append(where, "m.kind = ?")
		args = append(args, f.Kind)
	}
	if f.Container != "" {
		where = append(where, "m.container = ?")
		args = append(args, f.Container)
	}
	if f.Codec != "" {
		where = append(where, "(m.video_codec = ? OR m.audio_codec = ?)")
		args = append(args, f.Codec, f.Codec)
	}
	if f.FrameRate > 0 {
		where = append(where, "ABS(m.frame_rate - ?) < 0.01")
		args = append(args, f.FrameRate)
	}
	if f.QC != "" {
		where = append(where, "m.qc = ?")
		args = append(args, f.QC)
	}
	if t := strings.TrimSpace(f.Text); t != "" {
		esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(t)
		like := "%" + esc + "%"
		where = append(where, `(m.path LIKE ? ESCAPE '\' OR m.camera LIKE ? ESCAPE '\' OR m.reel LIKE ? ESCAPE '\' OR m.scene LIKE ? ESCAPE '\' OR m.notes LIKE ? ESCAPE '\' OR m.tags LIKE ? ESCAPE '\' OR m.timecode LIKE ? ESCAPE '\')`)
		args = append(args, like, like, like, like, like, like, like)
	}
	q := `SELECT ` + mediaCols + `,
		(SELECT COUNT(DISTINCT o.drive_id) FROM files o WHERE o.xxh64 = m.xxh64 AND o.size = m.size AND o.drive_id != m.drive_id AND o.bad_hash IS NULL)
		FROM media m`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY m.drive_id, m.path"
	rows, err := db.sql.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	var out []MediaRow
	for rows.Next() {
		var m Media
		var extra string
		var probed int64
		var copies int
		if err := rows.Scan(&m.DriveID, &m.Path, &m.XXH64, &m.Size, &m.Kind, &m.Container, &m.VideoCodec, &m.Width, &m.Height, &m.FrameRate,
			&m.DurationMS, &m.Timecode, &m.AudioCodec, &m.Channels, &m.SampleRate, &m.BitDepth, &m.Camera, &m.Reel, &m.Created, &extra, &probed,
			&m.Edits.Scene, &m.Edits.Shot, &m.Edits.Take, &m.Edits.Notes, &m.Edits.Tags, &m.Edits.QC, &copies); err != nil {
			return nil, err
		}
		if f.DriveID == "" && m.XXH64 != "" {
			k := m.XXH64 + ":" + fmt.Sprint(m.Size)
			if seen[k] {
				continue
			}
			seen[k] = true
		}
		if extra != "" {
			json.Unmarshal([]byte(extra), &m.Extra)
		}
		m.ProbedAt = time.Unix(probed, 0)
		out = append(out, MediaRow{Media: m, Copies: copies})
		if f.Limit > 0 && len(out) >= f.Limit {
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, rows.Err()
}

// MediaFacets lists the formats, codecs and frame rates present, for
// filters.
func (db *DB) MediaFacets() (formats, codecs []string, rates []float64, err error) {
	strs := func(q string) ([]string, error) {
		rows, err := db.sql.Query(q)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var c string
			rows.Scan(&c)
			out = append(out, c)
		}
		return out, rows.Err()
	}
	if formats, err = strs(`SELECT DISTINCT container FROM media WHERE container != '' ORDER BY 1`); err != nil {
		return nil, nil, nil, err
	}
	if codecs, err = strs(`SELECT DISTINCT video_codec FROM media WHERE video_codec != '' UNION SELECT DISTINCT audio_codec FROM media WHERE audio_codec != '' ORDER BY 1`); err != nil {
		return nil, nil, nil, err
	}
	rows, err := db.sql.Query(`SELECT DISTINCT frame_rate FROM media WHERE frame_rate > 0 ORDER BY 1`)
	if err != nil {
		return nil, nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var r float64
		rows.Scan(&r)
		rates = append(rates, r)
	}
	return formats, codecs, rates, nil
}

// Setting reads a stored setting, or def when unset.
func (db *DB) Setting(key, def string) string {
	var v string
	if err := db.sql.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v); err != nil {
		return def
	}
	return v
}

// SetSetting stores a setting.
func (db *DB) SetSetting(key, value string) error {
	_, err := db.sql.Exec(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// Settings returns every setting with the given prefix.
func (db *DB) Settings(prefix string) (map[string]string, error) {
	rows, err := db.sql.Query(`SELECT key, value FROM settings WHERE key LIKE ? ESCAPE '\'`, strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(prefix)+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// RecentOffloads returns the last sources and destination folders used
// for offloads, newest first, so the form can offer them again.
func (db *DB) RecentOffloads() map[string][]string {
	out := map[string][]string{"sources": {}, "dests": {}}
	if v := db.Setting("offload.recent", ""); v != "" {
		json.Unmarshal([]byte(v), &out)
	}
	for k, v := range out {
		if v == nil {
			out[k] = []string{}
		}
	}
	return out
}

// RememberOffload records an offload's source and destinations.
func (db *DB) RememberOffload(source string, dests []string) error {
	r := db.RecentOffloads()
	push := func(list []string, p string) []string {
		p = strings.TrimRight(p, "/")
		if p == "" {
			return list
		}
		out := []string{p}
		for _, x := range list {
			if x != p && len(out) < 10 {
				out = append(out, x)
			}
		}
		return out
	}
	r["sources"] = push(r["sources"], source)
	for i := len(dests) - 1; i >= 0; i-- {
		r["dests"] = push(r["dests"], dests[i])
	}
	b, _ := json.Marshal(r)
	return db.SetSetting("offload.recent", string(b))
}
