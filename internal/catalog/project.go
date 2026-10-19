package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"
)

// Source is a folder on a drive that belongs to a project. Folder is a
// slash-separated path relative to the drive; "" is the whole drive.
type Source struct {
	DriveID string `json:"driveId"`
	Folder  string `json:"folder"`
	// Role is "" for a folder that holds the work, or "backup" for a
	// folder that exists to hold copies of it: when a project has backup
	// folders, the plan copies only onto their drives, and files found
	// only in backup folders are not copied anywhere.
	Role string `json:"role,omitempty"`
	// Exclude lists sub-folders (relative to Folder) left out of the
	// project: scratch, drafts, renders.
	Exclude []string `json:"exclude,omitempty"`
	// Into, when set, is the project folder on another drive that this
	// folder's files are copied into, keeping their relative paths.
	// Unset, the project works it out (see RouteFor).
	Into *Source `json:"into,omitempty"`
}

// Same reports whether two sources name the same folder.
func (s Source) Same(o Source) bool { return s.DriveID == o.DriveID && s.Folder == o.Folder }

// Excludes reports whether a path on the drive falls under one of the
// source's excluded sub-folders.
func (s Source) Excludes(p string) bool {
	for _, ex := range s.Exclude {
		full := path.Join(s.Folder, ex)
		if p == full || strings.HasPrefix(p, full+"/") {
			return true
		}
	}
	return false
}

// Base is the folder's own name, or "" for a whole drive.
func (s Source) Base() string {
	if s.Folder == "" {
		return ""
	}
	return path.Base(s.Folder)
}

// Project is one body of work: the folders that hold it, wherever they
// are, and how many drives every file in them should be on (Target). The
// plan copies missing files onto DestIDs, or onto any drive in the
// library when DestIDs is empty.
type Project struct {
	ID      int64     `json:"id"`
	Name    string    `json:"name"`
	Color   string    `json:"color"`
	Notes   string    `json:"notes"`
	Target  int       `json:"target"`
	Created time.Time `json:"-"`
	Sources []Source  `json:"sources"`
	DestIDs []string  `json:"destIds"`
}

// BackupDrives lists the drives of the project's backup folders.
func (p *Project) BackupDrives() []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range p.Sources {
		if s.Role == "backup" && !seen[s.DriveID] {
			seen[s.DriveID] = true
			out = append(out, s.DriveID)
		}
	}
	return out
}

// RouteFor says which of the project's folders on the destination drive
// a source folder's files are copied into, keeping their paths relative
// to the source folder: the one the source names (Into), else the only
// project folder on that drive, else the one whose own name matches,
// else the first. Nil when the project has no folder on that drive, so
// the copy goes into a folder named after the project.
func (p *Project) RouteFor(src Source, destID string) *Source {
	if src.Into != nil && src.Into.DriveID == destID {
		for i := range p.Sources {
			if p.Sources[i].Same(*src.Into) {
				return &p.Sources[i]
			}
		}
	}
	var there []*Source
	for i := range p.Sources {
		if p.Sources[i].DriveID == destID && !p.Sources[i].Same(src) {
			there = append(there, &p.Sources[i])
		}
	}
	switch len(there) {
	case 0:
		return nil
	case 1:
		return there[0]
	}
	for _, t := range there {
		if t.Base() != "" && strings.EqualFold(t.Base(), src.Base()) {
			return t
		}
	}
	return there[0]
}

// DriveIDs lists the drives the project's sources are on, without repeats.
func (p *Project) DriveIDs() []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range p.Sources {
		if !seen[s.DriveID] {
			seen[s.DriveID] = true
			out = append(out, s.DriveID)
		}
	}
	return out
}

func cleanFolder(folder string) (string, error) {
	f := strings.Trim(strings.ReplaceAll(strings.TrimSpace(folder), "\\", "/"), "/")
	if f == "" || f == "." {
		return "", nil
	}
	if f != path.Clean(f) || strings.HasPrefix(f, "../") || f == ".." || strings.Contains(f, "/../") {
		return "", fmt.Errorf("not a folder path: %q", folder)
	}
	return f, nil
}

// Projects lists every project with its sources and destinations, by name.
func (db *DB) Projects() ([]*Project, error) {
	rows, err := db.sql.Query(`SELECT id, name, color, notes, target, created FROM projects ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Project
	byID := map[int64]*Project{}
	for rows.Next() {
		p := &Project{Sources: []Source{}, DestIDs: []string{}}
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
	srcs, err := db.sql.Query(`SELECT ps.project_id, ps.drive_id, ps.folder, ps.role, ps.exclude, ps.route FROM project_sources ps
		JOIN drives d ON d.id = ps.drive_id ORDER BY d.name COLLATE NOCASE, ps.folder`)
	if err != nil {
		return nil, err
	}
	defer srcs.Close()
	for srcs.Next() {
		var pid int64
		var s Source
		var exclude, into string
		if err := srcs.Scan(&pid, &s.DriveID, &s.Folder, &s.Role, &exclude, &into); err != nil {
			return nil, err
		}
		if exclude != "" {
			json.Unmarshal([]byte(exclude), &s.Exclude)
		}
		if into != "" {
			var t Source
			if json.Unmarshal([]byte(into), &t) == nil && t.DriveID != "" {
				s.Into = &t
			}
		}
		if p := byID[pid]; p != nil {
			p.Sources = append(p.Sources, s)
		}
	}
	if err := srcs.Err(); err != nil {
		return nil, err
	}
	dests, err := db.sql.Query(`SELECT pd.project_id, pd.drive_id FROM project_dests pd
		JOIN drives d ON d.id = pd.drive_id ORDER BY d.name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer dests.Close()
	for dests.Next() {
		var pid int64
		var did string
		if err := dests.Scan(&pid, &did); err != nil {
			return nil, err
		}
		if p := byID[pid]; p != nil {
			p.DestIDs = append(p.DestIDs, did)
		}
	}
	return out, dests.Err()
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

// AddSource puts a folder (or, with folder "", a whole drive) in a project.
func (db *DB) AddSource(projectID int64, driveID, folder string) error {
	f, err := cleanFolder(folder)
	if err != nil {
		return err
	}
	if _, err := db.Drive(driveID); err != nil {
		return err
	}
	_, err = db.sql.Exec(`INSERT OR IGNORE INTO project_sources (project_id, drive_id, folder) VALUES (?, ?, ?)`, projectID, driveID, f)
	return err
}

// SetSource changes a folder's role, exclusions and routing.
func (db *DB) SetSource(projectID int64, s Source) error {
	f, err := cleanFolder(s.Folder)
	if err != nil {
		return err
	}
	if s.Role != "" && s.Role != "backup" {
		return fmt.Errorf("unknown role %q", s.Role)
	}
	var ex []string
	for _, e := range s.Exclude {
		if c, err := cleanFolder(e); err == nil && c != "" {
			ex = append(ex, c)
		}
	}
	exclude := ""
	if len(ex) > 0 {
		b, _ := json.Marshal(ex)
		exclude = string(b)
	}
	into := ""
	if s.Into != nil && s.Into.DriveID != "" {
		t := Source{DriveID: s.Into.DriveID}
		if t.Folder, err = cleanFolder(s.Into.Folder); err != nil {
			return err
		}
		b, _ := json.Marshal(t)
		into = string(b)
	}
	res, err := db.sql.Exec(`UPDATE project_sources SET role = ?, exclude = ?, route = ? WHERE project_id = ? AND drive_id = ? AND folder = ?`,
		s.Role, exclude, into, projectID, s.DriveID, f)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("that folder is not in the project")
	}
	return nil
}

// RemoveSource takes a folder out of a project.
func (db *DB) RemoveSource(projectID int64, driveID, folder string) error {
	f, err := cleanFolder(folder)
	if err != nil {
		return err
	}
	_, err = db.sql.Exec(`DELETE FROM project_sources WHERE project_id = ? AND drive_id = ? AND folder = ?`, projectID, driveID, f)
	return err
}

// SetDests restricts where a project's backups may go. An empty list
// means any drive in the library.
func (db *DB) SetDests(projectID int64, driveIDs []string) error {
	tx, err := db.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM project_dests WHERE project_id = ?`, projectID); err != nil {
		return err
	}
	for _, id := range driveIDs {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO project_dests (project_id, drive_id) VALUES (?, ?)`, projectID, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DriveProjects maps each drive to the ids of the projects with a
// source on it.
func (db *DB) DriveProjects() (map[string][]int64, error) {
	rows, err := db.sql.Query(`SELECT DISTINCT drive_id, project_id FROM project_sources ORDER BY project_id`)
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
