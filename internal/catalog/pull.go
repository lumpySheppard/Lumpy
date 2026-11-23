package catalog

import (
	"path"
	"sort"
	"strings"

	"github.com/lumpysheppard/lumpy/internal/media"
)

// Pulling shots: an edit (a cut, a trailer, a selects reel) refers to
// twenty clips scattered across the drives of a feature, and the job
// is to gather the camera originals. Pull matches every clip the edit
// names against the whole library, proxies and originals alike, and
// ranks what it finds so the original comes first: an edit cut on
// proxies still points at the raws.

// PullHit is one copy of a clip somewhere in the library.
type PullHit struct {
	DriveID string `json:"driveId"`
	Path    string `json:"path"` // file or .RDC folder, relative to the drive
	Bytes   int64  `json:"bytes"`
	Files   int    `json:"files"`
	Proxy   bool   `json:"proxy"` // a proxy by name or folder: not what a pull wants
	Score   int    `json:"-"`
}

// PullRef is one clip the edit refers to and everywhere it was found,
// best first.
type PullRef struct {
	Name string    `json:"name"`
	Hits []PullHit `json:"hits"`
}

// PullDrive sums what one drive holds of the edit.
type PullDrive struct {
	DriveID string `json:"driveId"`
	Clips   int    `json:"clips"` // clips whose best copy is here
	Bytes   int64  `json:"bytes"`
}

// PullReport is what the library holds of an edit.
type PullReport struct {
	Refs    []PullRef   `json:"refs"`
	Found   int         `json:"found"`
	Missing []string    `json:"missing"`
	Drives  []PullDrive `json:"drives"`
}

// rawExt ranks camera formats above everything else.
var rawExt = map[string]int{".braw": 30, ".r3d": 30, ".ari": 30, ".arx": 30, ".mxf": 25, ".crm": 25, ".dng": 25, ".mov": 15, ".mp4": 5, ".avi": 5, ".wav": 10}

// proxyish says whether a path is a proxy by its name or any folder it
// sits in.
func proxyish(p string) bool {
	parts := strings.Split(p, "/")
	for _, d := range parts[:len(parts)-1] {
		u := strings.ToLower(d)
		if isOffline(d) || strings.Contains(u, "proxy") || strings.Contains(u, "proxies") || strings.Contains(u, "transcode") {
			return true
		}
	}
	stem := clipStem(parts[len(parts)-1])
	return proxySuffix.ReplaceAllString(stem, "") != stem
}

// Pull matches refs against every drive in the library.
func (db *DB) Pull(refs []ArchiveRef) (*PullReport, error) {
	rep := &PullReport{Missing: []string{}, Drives: []PullDrive{}}
	type ref struct {
		name string
		hits []PullHit
	}
	var rs []*ref
	// A reference is known by its stem and by that stem with a proxy
	// suffix stripped. A RED chunk (R003_001.R3D) is also known by the
	// clip name R003, but that key matches only RED clip folders, so an
	// unrelated R003.braw doesn't come along.
	byKey := map[string][]*ref{}
	redKey := map[string][]*ref{}
	seen := map[string]bool{}
	for _, r := range refs {
		name := r.Name
		if name == "" {
			name = path.Base(r.Path)
		}
		if name == "" || seen[clipStem(name)] {
			continue
		}
		seen[clipStem(name)] = true
		x := &ref{name: name, hits: []PullHit{}}
		rs = append(rs, x)
		stem := strings.ToLower(strings.TrimSuffix(name, path.Ext(name)))
		byKey[stem] = append(byKey[stem], x)
		if s := proxySuffix.ReplaceAllString(stem, ""); s != stem {
			byKey[s] = append(byKey[s], x)
		}
		if s := redChunk.ReplaceAllString(stem, ""); s != stem {
			redKey[s] = append(redKey[s], x)
		}
	}
	drives, err := db.Drives()
	if err != nil {
		return nil, err
	}
	for _, d := range drives {
		files, err := db.Files(d.ID)
		if err != nil {
			return nil, err
		}
		items := map[string]*PullHit{}
		var order []string
		for _, f := range files {
			if !media.Probable(f.Path) {
				continue
			}
			key := f.Path
			if strings.EqualFold(path.Ext(f.Path), ".r3d") {
				if dir := path.Dir(f.Path); strings.EqualFold(path.Ext(dir), ".rdc") {
					key = dir
				}
			}
			it := items[key]
			if it == nil {
				it = &PullHit{DriveID: d.ID, Path: key, Proxy: proxyish(f.Path)}
				items[key] = it
				order = append(order, key)
			}
			it.Bytes += f.Size
			it.Files++
		}
		for _, key := range order {
			it := items[key]
			name := path.Base(key)
			if strings.EqualFold(path.Ext(key), ".rdc") {
				name = strings.TrimSuffix(name, path.Ext(name))
			}
			stem := clipStem(name)
			ext := strings.ToLower(path.Ext(key))
			if ext == ".rdc" {
				ext = ".r3d"
			}
			it.Score = rawExt[ext]
			if !it.Proxy {
				it.Score += 100
			}
			matched := map[*ref]bool{}
			take := func(list []*ref) {
				for _, r := range list {
					if !matched[r] {
						matched[r] = true
						r.hits = append(r.hits, *it)
					}
				}
			}
			take(byKey[stem])
			if s := proxySuffix.ReplaceAllString(stem, ""); s != stem {
				take(byKey[s])
			}
			if ext == ".r3d" {
				take(redKey[stem])
			}
		}
	}
	best := map[string]*PullDrive{}
	for _, r := range rs {
		sort.SliceStable(r.hits, func(i, j int) bool {
			if r.hits[i].Score != r.hits[j].Score {
				return r.hits[i].Score > r.hits[j].Score
			}
			return r.hits[i].Bytes > r.hits[j].Bytes
		})
		if len(r.hits) == 0 {
			rep.Missing = append(rep.Missing, r.name)
		} else {
			rep.Found++
			b := r.hits[0]
			pd := best[b.DriveID]
			if pd == nil {
				pd = &PullDrive{DriveID: b.DriveID}
				best[b.DriveID] = pd
			}
			pd.Clips++
			pd.Bytes += b.Bytes
		}
		rep.Refs = append(rep.Refs, PullRef{Name: r.name, Hits: r.hits})
	}
	sort.Strings(rep.Missing)
	for _, pd := range best {
		rep.Drives = append(rep.Drives, *pd)
	}
	sort.Slice(rep.Drives, func(i, j int) bool { return rep.Drives[i].Clips > rep.Drives[j].Clips })
	return rep, nil
}

// PullFiles lists the files behind a hit: the file itself, or every
// chunk of a RED clip folder.
func (db *DB) PullFiles(driveID, p string) ([]File, error) {
	files, err := db.Files(driveID)
	if err != nil {
		return nil, err
	}
	var out []File
	for _, f := range files {
		if f.Path == p || strings.HasPrefix(f.Path, p+"/") {
			out = append(out, f)
		}
	}
	return out, nil
}
