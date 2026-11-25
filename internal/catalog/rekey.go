package catalog

import (
	"errors"
	"fmt"
	"strings"
)

// Rekey moves everything the library holds under one drive id to
// another: the drive row, its files with their checksums, media, events,
// health readings, stills, project folders and mounts. It is how a
// drive that came back under a new identity (renamed, where the OS
// gives no volume UUID; or now known by its partition's GUID) keeps
// its log. When the new id is already a drive, the old one's records
// fold into it: for a path both hold, the old row wins, since it
// carries the checksums and the new is a fresh scan. The old drive
// goes.
func (db *DB) Rekey(oldID, newID string) error {
	if oldID == "" || newID == "" || oldID == newID {
		return errors.New("rekey needs two different drive ids")
	}
	tx, err := db.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var have, haveNew int
	tx.QueryRow(`SELECT COUNT(*) FROM drives WHERE id = ?`, oldID).Scan(&have)
	tx.QueryRow(`SELECT COUNT(*) FROM drives WHERE id = ?`, newID).Scan(&haveNew)
	if have == 0 {
		return fmt.Errorf("no drive %s in the library", oldID)
	}
	if haveNew == 0 {
		// A copy of the row under the new id, so the children can move.
		rows, err := tx.Query(`PRAGMA table_info(drives)`)
		if err != nil {
			return err
		}
		var cols []string
		for rows.Next() {
			var cid int
			var name, typ string
			var notnull int
			var dflt any
			var pk int
			if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
				rows.Close()
				return err
			}
			cols = append(cols, name)
		}
		rows.Close()
		sel := make([]string, len(cols))
		for i, c := range cols {
			sel[i] = c
			if c == "id" {
				sel[i] = "?"
			}
		}
		if _, err := tx.Exec(`INSERT INTO drives (`+strings.Join(cols, ", ")+`) SELECT `+strings.Join(sel, ", ")+` FROM drives WHERE id = ?`, newID, oldID); err != nil {
			return err
		}
	} else {
		// The new drive's own scan of a path the old one knows gives way.
		if _, err := tx.Exec(`DELETE FROM files WHERE drive_id = ? AND path IN (SELECT path FROM files WHERE drive_id = ?)`, newID, oldID); err != nil {
			return err
		}
	}
	for _, t := range []string{"files", "media", "events", "health", "stills", "project_sources", "project_dests", "mounted"} {
		if _, err := tx.Exec(`UPDATE OR IGNORE `+t+` SET drive_id = ? WHERE drive_id = ?`, newID, oldID); err != nil {
			return err
		}
	}
	// The old row goes, with whatever could not move; then the new row's
	// totals are counted afresh, since moving rows does not run the
	// triggers that keep them.
	if _, err := tx.Exec(`DELETE FROM drives WHERE id = ?`, oldID); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE drives SET files = (SELECT COUNT(*) FROM files WHERE drive_id = ?),
		bytes = (SELECT COALESCE(SUM(size), 0) FROM files WHERE drive_id = ?),
		hashed = (SELECT COUNT(*) FROM files WHERE drive_id = ? AND xxh64 IS NOT NULL),
		damaged = (SELECT COUNT(*) FROM files WHERE drive_id = ? AND bad_hash IS NOT NULL) WHERE id = ?`, newID, newID, newID, newID, newID); err != nil {
		return err
	}
	return tx.Commit()
}

// HasDrive says whether the library knows a drive id.
func (db *DB) HasDrive(id string) bool {
	var n int
	db.sql.QueryRow(`SELECT COUNT(*) FROM drives WHERE id = ?`, id).Scan(&n)
	return n > 0
}
