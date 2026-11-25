package catalog

import (
	"regexp"
	"sort"
	"strings"
)

// Finding projects in the library. Production drives carry project
// folders the way Post Haste makes them, "04_BS_REDHAND_2025-03-01",
// at the root of a drive or under a folder like 01_PROJECTS. Discover
// walks every logged drive's index for folders like that, takes a
// name from each (the number, a house prefix such as BS, dates and
// initials dropped), and groups folders that start with the same word,
// so ICEMAN_EP1 and ICEMAN_EP2 become one project with two folders.
// The page lets the person rename, regroup and untick before anything
// is created.

// Found is one folder that looks like a project.
type Found struct {
	DriveID   string `json:"driveId"`
	Folder    string `json:"folder"`
	Name      string `json:"name"`  // the cleaned-up name, e.g. "REDHAND"
	Bytes     int64  `json:"bytes"` //
	Files     int    `json:"files"`
	InProject int64  `json:"inProject,omitempty"` // already a source of this project
}

// Group is a set of found folders that share a first word.
type Group struct {
	Key     string  `json:"key"`  // the shared word, upper case
	Name    string  `json:"name"` // suggested project name
	Bytes   int64   `json:"bytes"`
	Folders []Found `json:"folders"`
}

var (
	numbered = regexp.MustCompile(`^\d{1,3}[_\-. ]+(.+)$`)
	dateTok  = regexp.MustCompile(`^(\d{4}[-_.]?\d{2}[-_.]?\d{2}|\d{6}|\d{8}|\d{4})$`)
	sizeTok  = regexp.MustCompile(`^\d+(TB|GB|MB)$`)
)

// templateFolders are the subfolders a project template makes: never a
// project themselves.
var templateFolders = map[string]bool{}

func init() {
	for _, n := range []string{"RAW", "PROXY", "PROXIES", "OUTPUT", "OUTPUTS", "EXPORT", "EXPORTS", "AUDIO", "SOUND", "MUSIC", "FOOTAGE", "RUSHES", "MEDIA",
		"GRAPHICS", "GFX", "VFX", "STILLS", "PHOTOS", "DOCS", "DOCUMENTS", "PAPERWORK", "PROJECT", "PROJECTS", "PROJECT FILES", "PREMIERE", "RESOLVE", "FCP", "EDIT", "EDITS",
		"CACHE", "RENDER", "RENDERS", "DELIVERABLES", "DELIVERY", "FINAL", "ASSETS", "SCRIPTS", "SCRIPT", "REFERENCE", "REFS", "LUTS", "TRANSCODES", "DAILIES", "ARCHIVE", "MISC"} {
		templateFolders[n] = true
	}
}

// cleanName takes the project name out of a folder name: the leading
// number goes, then every token in ignore (a house prefix like BS),
// dates, and anything after a date (initials). The tokens left are
// joined with spaces.
func cleanName(folder string, ignore map[string]bool) (name, key string) {
	base := folder
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	if m := numbered.FindStringSubmatch(base); m != nil {
		base = m[1]
	}
	var toks []string
	for _, t := range strings.FieldsFunc(base, func(r rune) bool { return r == '_' || r == '-' || r == ' ' || r == '.' }) {
		u := strings.ToUpper(t)
		if ignore[u] {
			continue
		}
		if dateTok.MatchString(t) {
			break
		}
		toks = append(toks, t)
	}
	if len(toks) == 0 {
		return "", ""
	}
	return strings.Join(toks, " "), strings.ToUpper(toks[0])
}

// looksLikeProject says whether a folder at this path could be a project:
// numbered at the root (and not a template subfolder or a size), or
// anything inside a folder whose name says projects.
func looksLikeProject(path string) bool {
	parts := strings.Split(path, "/")
	base := parts[len(parts)-1]
	up := strings.ToUpper(base)
	if strings.Contains(up, "PROJECT") {
		return false // the projects folder itself
	}
	if len(parts) == 2 && strings.Contains(strings.ToUpper(parts[0]), "PROJECT") {
		return true
	}
	if len(parts) != 1 {
		return false
	}
	m := numbered.FindStringSubmatch(base)
	if m == nil {
		return false
	}
	rest := strings.ToUpper(m[1])
	if sizeTok.MatchString(strings.ReplaceAll(rest, "_", "")) || templateFolders[rest] {
		return false
	}
	return true
}

// Discover finds project folders on every drive and groups them. ignore
// lists tokens (upper case) left out of names, such as BS.
func (db *DB) Discover(ignore []string) ([]Group, error) {
	ig := map[string]bool{}
	for _, t := range ignore {
		if t = strings.ToUpper(strings.TrimSpace(t)); t != "" {
			ig[t] = true
		}
	}
	drives, err := db.Drives()
	if err != nil {
		return nil, err
	}
	projects, err := db.Projects()
	if err != nil {
		return nil, err
	}
	held := map[string]int64{} // driveID|folder -> project
	for _, p := range projects {
		for _, s := range p.Sources {
			held[s.DriveID+"|"+s.Folder] = p.ID
		}
	}
	groups := map[string]*Group{}
	for _, d := range drives {
		rows, err := db.sql.Query(`SELECT path, size FROM files WHERE drive_id = ?`, d.ID)
		if err != nil {
			return nil, err
		}
		type agg struct {
			bytes int64
			files int
		}
		folders := map[string]*agg{}
		for rows.Next() {
			var path string
			var size int64
			if err := rows.Scan(&path, &size); err != nil {
				rows.Close()
				return nil, err
			}
			parts := strings.Split(path, "/")
			for depth := 1; depth <= 2 && depth < len(parts); depth++ {
				f := strings.Join(parts[:depth], "/")
				a := folders[f]
				if a == nil {
					a = &agg{}
					folders[f] = a
				}
				a.bytes += size
				a.files++
			}
		}
		rows.Close()
		for f, a := range folders {
			if !looksLikeProject(f) {
				continue
			}
			name, key := cleanName(f, ig)
			if key == "" {
				continue
			}
			found := Found{DriveID: d.ID, Folder: f, Name: name, Bytes: a.bytes, Files: a.files, InProject: held[d.ID+"|"+f]}
			g := groups[key]
			if g == nil {
				g = &Group{Key: key, Name: name}
				groups[key] = g
			}
			g.Folders = append(g.Folders, found)
			g.Bytes += a.bytes
		}
	}
	out := make([]Group, 0, len(groups))
	for _, g := range groups {
		sort.Slice(g.Folders, func(i, j int) bool { return g.Folders[i].Bytes > g.Folders[j].Bytes })
		// Several folders: the shared word is the name. One folder: its
		// whole cleaned name.
		if len(g.Folders) > 1 {
			g.Name = g.Key
			for _, f := range g.Folders {
				if strings.ToUpper(f.Name) == g.Key {
					g.Name = f.Name
				}
			}
		}
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Bytes > out[j].Bytes })
	return out, nil
}
