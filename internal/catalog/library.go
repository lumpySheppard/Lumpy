package catalog

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The library normally lives in ~/Library/Application Support/Lumpy. A
// "location" file there can point somewhere else (an external drive,
// a synced folder), for a Mac with little room.

// defaultDir is the folder DefaultPath would use with no location file.
func defaultDir() (string, error) {
	if h := os.Getenv("LUMPY_HOME"); h != "" {
		return h, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "Application Support", "Lumpy"), nil
}

// LibraryLocation returns the folder a location file points at, or "".
func LibraryLocation() string {
	dir, err := defaultDir()
	if err != nil {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(dir, "location"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// SetLibraryLocation points future opens at dir ("" goes back to the
// default folder).
func SetLibraryLocation(dir string) error {
	base, err := defaultDir()
	if err != nil {
		return err
	}
	p := filepath.Join(base, "location")
	if dir == "" {
		err := os.Remove(p)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(dir+"\n"), 0o644)
}

// LibraryInfo describes the open library.
type LibraryInfo struct {
	Path     string `json:"path"`
	Bytes    int64  `json:"bytes"`
	Drives   int    `json:"drives"`
	Files    int    `json:"files"`
	Events   int    `json:"events"`
	Media    int    `json:"media"`
	Default  string `json:"default"`  // the standard folder
	Location string `json:"location"` // the location file's folder, if any
}

// Info sums the library up.
func (db *DB) Info() (LibraryInfo, error) {
	info := LibraryInfo{Path: db.path, Location: LibraryLocation()}
	info.Default, _ = defaultDir()
	if st, err := os.Stat(db.path); err == nil {
		info.Bytes = st.Size()
		if st, err := os.Stat(db.path + "-wal"); err == nil {
			info.Bytes += st.Size()
		}
	}
	for _, q := range []struct {
		sql string
		dst *int
	}{{"SELECT COUNT(*) FROM drives", &info.Drives}, {"SELECT COUNT(*) FROM files", &info.Files}, {"SELECT COUNT(*) FROM events", &info.Events}, {"SELECT COUNT(*) FROM media", &info.Media}} {
		if err := db.sql.QueryRow(q.sql).Scan(q.dst); err != nil {
			return info, err
		}
	}
	return info, nil
}

// Optimise compacts the database file.
func (db *DB) Optimise() error {
	_, err := db.sql.Exec("VACUUM")
	return err
}

// CopyTo writes a consistent copy of the library to path, which must
// not exist yet. The open library keeps running.
func (db *DB) CopyTo(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	_, err := db.sql.Exec("VACUUM INTO ?", path)
	return err
}

// ForgetAll removes every drive, its files, checksums and history.
func (db *DB) ForgetAll() error {
	drives, err := db.Drives()
	if err != nil {
		return err
	}
	for _, d := range drives {
		if err := db.Forget(d.ID); err != nil {
			return err
		}
	}
	return nil
}

// ClearHistory removes the event log but keeps drives and files.
func (db *DB) ClearHistory() error {
	_, err := db.sql.Exec("DELETE FROM events")
	return err
}
