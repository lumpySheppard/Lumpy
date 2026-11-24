package catalog

import "time"

// A Still is a frame grabbed from a clip in the player: the PNG on
// disk, and the clip and frame it came from.
type Still struct {
	ID      int64     `json:"id"`
	DriveID string    `json:"driveId"`
	Path    string    `json:"path"`
	File    string    `json:"file"`
	TC      string    `json:"tc"`
	Frame   int64     `json:"frame"`
	Width   int       `json:"width"`
	Height  int       `json:"height"`
	At      time.Time `json:"at"`
}

// AddStill records a grabbed frame.
func (db *DB) AddStill(st *Still) error {
	if st.At.IsZero() {
		st.At = time.Now()
	}
	res, err := db.sql.Exec(`INSERT INTO stills (drive_id, path, file, tc, frame, width, height, at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		st.DriveID, st.Path, st.File, st.TC, st.Frame, st.Width, st.Height, st.At.Unix())
	if err != nil {
		return err
	}
	st.ID, _ = res.LastInsertId()
	return nil
}

// Stills lists the frames grabbed from one clip, newest first.
func (db *DB) Stills(driveID, path string) ([]*Still, error) {
	rows, err := db.sql.Query(`SELECT id, drive_id, path, file, tc, frame, width, height, at FROM stills WHERE drive_id = ? AND path = ? ORDER BY at DESC, id DESC`, driveID, path)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Still
	for rows.Next() {
		st, err := scanStill(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// Still is one grabbed frame by id.
func (db *DB) Still(id int64) (*Still, error) {
	return scanStill(db.sql.QueryRow(`SELECT id, drive_id, path, file, tc, frame, width, height, at FROM stills WHERE id = ?`, id))
}

// DeleteStill forgets a grabbed frame (the file is the caller's).
func (db *DB) DeleteStill(id int64) error {
	_, err := db.sql.Exec(`DELETE FROM stills WHERE id = ?`, id)
	return err
}

func scanStill(r interface{ Scan(...any) error }) (*Still, error) {
	var st Still
	var at int64
	if err := r.Scan(&st.ID, &st.DriveID, &st.Path, &st.File, &st.TC, &st.Frame, &st.Width, &st.Height, &at); err != nil {
		return nil, err
	}
	st.At = time.Unix(at, 0)
	return &st, nil
}
