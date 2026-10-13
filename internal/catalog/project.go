package catalog

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Project is a group of drives that together hold one body of work, such
// as a film. Target is how many drives every file should be on.
type Project struct {
	ID       int64     `json:"id"`
	Name     string    `json:"name"`
	Color    string    `json:"color"`
	Notes    string    `json:"notes"`
	Target   int       `json:"target"`
	Created  time.Time `json:"-"`
	DriveIDs []string  `json:"driveIds"`
}

// Projects lists every project with its drives, by name.
func (db *DB) Projects() ([]*Project, error) {
	rows, err := db.sql.Query(`SELECT id, name, color, notes, target, created FROM projects ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Project
	byID := map[int64]*Project{}
	for rows.Next() {
		p := &Project{DriveIDs: []string{}}
		var created int64
		if err := rows.Scan(&p.ID, &p.Name, &p.Color, &p.Notes, &p.Target, &created); err != nil {
			return nil, err
		}
		p.Created = time.Unix(created, 0)
		out = append(out, p)
		byID[p.ID] = p
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	links, err := db.sql.Query(`SELECT pd.project_id, pd.drive_id FROM project_drives pd
		JOIN drives d ON d.id = pd.drive_id ORDER BY d.name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer links.Close()
	for links.Next() {
		var pid int64
		var did string
		if err := links.Scan(&pid, &did); err != nil {
			return nil, err
		}
		if p := byID[pid]; p != nil {
			p.DriveIDs = append(p.DriveIDs, did)
		}
	}
	return out, links.Err()
}

// Project returns one project.
func (db *DB) Project(id int64) (*Project, error) {
	all, err := db.Projects()
	if err != nil {
		return nil, err
	}
	for _, p := range all {
		if p.ID == id {
			return p, nil
		}
	}
	return nil, fmt.Errorf("no project %d", id)
}

// CreateProject adds a project and returns it.
func (db *DB) CreateProject(name string, target int) (*Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("a project needs a name")
	}
	if target < 1 {
		target = 2
	}
	res, err := db.sql.Exec(`INSERT INTO projects (name, target, created) VALUES (?, ?, ?)`, name, target, time.Now().Unix())
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return db.Project(id)
}

// UpdateProject changes a project's name, color, notes and target.
func (db *DB) UpdateProject(p *Project) error {
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("a project needs a name")
	}
	if p.Target < 1 {
		p.Target = 1
	}
	_, err := db.sql.Exec(`UPDATE projects SET name = ?, color = ?, notes = ?, target = ? WHERE id = ?`,
		strings.TrimSpace(p.Name), p.Color, strings.TrimSpace(p.Notes), p.Target, p.ID)
	return err
}

// DeleteProject removes a project. Its drives stay in the library.
func (db *DB) DeleteProject(id int64) error {
	_, err := db.sql.Exec(`DELETE FROM projects WHERE id = ?`, id)
	return err
}

// SetProjectDrive adds a drive to a project, or removes it.
func (db *DB) SetProjectDrive(projectID int64, driveID string, in bool) error {
	var err error
	if in {
		_, err = db.sql.Exec(`INSERT OR IGNORE INTO project_drives (project_id, drive_id) VALUES (?, ?)`, projectID, driveID)
	} else {
		_, err = db.sql.Exec(`DELETE FROM project_drives WHERE project_id = ? AND drive_id = ?`, projectID, driveID)
	}
	return err
}

// DriveProjects maps each drive to the ids of the projects it is in.
func (db *DB) DriveProjects() (map[string][]int64, error) {
	rows, err := db.sql.Query(`SELECT drive_id, project_id FROM project_drives ORDER BY project_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]int64{}
	for rows.Next() {
		var did string
		var pid int64
		if err := rows.Scan(&did, &pid); err != nil {
			return nil, err
		}
		out[did] = append(out[did], pid)
	}
	return out, rows.Err()
}

var _ = sql.ErrNoRows
