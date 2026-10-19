package catalog

import (
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/lumpysheppard/lumpy/internal/media"
)

// Post-production keeps camera originals and their proxies apart: an
// "Online" folder of camera raws next to an "Offline" folder of proxies
// (the names vary: 00_ONLINE, Online footage…). Lumpy pairs the two and
// says which originals have no proxy yet.

// ProxyPair is an Online folder and the Offline folder beside it.
type ProxyPair struct {
	Online  string `json:"online"`
	Offline string `json:"offline"`
}

// ProxyClip is one camera original and the proxy found for it.
type ProxyClip struct {
	Path  string `json:"path"` // the clip, or its .RDC folder for RED chunks
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
	Files int    `json:"files"` // chunks, for RED
	Proxy string `json:"proxy,omitempty"`
}

// ProxyReport is how far an Online folder's proxies have got.
type ProxyReport struct {
	ProxyPair
	Clips        []ProxyClip `json:"clips"`
	Total        int         `json:"total"`
	Proxied      int         `json:"proxied"`
	MissingBytes int64       `json:"missingBytes"`
	Extra        int         `json:"extra"` // proxies with no original
}

var onlineRe = regexp.MustCompile(`(?i)^[0-9_\-. ]*on[\s_\-]?line\b`)
var offlineRe = regexp.MustCompile(`(?i)^[0-9_\-. ]*off[\s_\-]?line\b`)

// proxySuffix is what proxy generators tack onto a name: _proxy, -PRX,
// Sony's S03, a Resolve "_1" etc.
var proxySuffix = regexp.MustCompile(`(?i)[\s_\-.]*(proxy|prx|offline|lo-?res|s\d{2}|_\d)$`)

// redChunk is the _001 on each file of a RED clip.
var redChunk = regexp.MustCompile(`_\d{3}$`)

// clipStem is the part of a name both an original and its proxy share.
func clipStem(name string) string {
	stem := strings.ToLower(strings.TrimSuffix(name, path.Ext(name)))
	if strings.EqualFold(path.Ext(name), ".r3d") {
		stem = redChunk.ReplaceAllString(stem, "")
	}
	return stem
}

// isOnline and isOffline recognise the folder names.
func isOnline(base string) bool  { return onlineRe.MatchString(base) }
func isOffline(base string) bool { return offlineRe.MatchString(base) }

// ProxyPairs finds Online/Offline folder pairs on a drive, under folder
// ("" for the whole drive).
func (db *DB) ProxyPairs(driveID, folder string) ([]ProxyPair, error) {
	files, err := db.Files(driveID)
	if err != nil {
		return nil, err
	}
	return proxyPairsFrom(files, folder), nil
}

// ProxyReports is every pair under folder with its report, reading the
// drive's file list once.
func (db *DB) ProxyReports(driveID, folder string) ([]*ProxyReport, error) {
	files, err := db.Files(driveID)
	if err != nil {
		return nil, err
	}
	var out []*ProxyReport
	for _, pair := range proxyPairsFrom(files, folder) {
		out = append(out, proxiesFrom(files, pair))
	}
	return out, nil
}

func proxyPairsFrom(files []File, folder string) []ProxyPair {
	prefix := folder
	if prefix != "" {
		prefix += "/"
	}
	dirs := map[string]bool{}
	for _, f := range files {
		if !strings.HasPrefix(f.Path, prefix) {
			continue
		}
		d := path.Dir(f.Path)
		for d != "." && d != "/" && !dirs[d] {
			dirs[d] = true
			d = path.Dir(d)
		}
	}
	var pairs []ProxyPair
	for d := range dirs {
		if !isOnline(path.Base(d)) {
			continue
		}
		parent := path.Dir(d)
		for o := range dirs {
			if path.Dir(o) == parent && o != d && isOffline(path.Base(o)) {
				pairs = append(pairs, ProxyPair{Online: d, Offline: o})
				break
			}
		}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Online < pairs[j].Online })
	return pairs
}

// Proxies reports each original in the pair's Online folder with the
// proxy found for it in the Offline folder: a video with the same name
// stem, at any depth, with or without a proxy suffix.
func (db *DB) Proxies(driveID string, pair ProxyPair) (*ProxyReport, error) {
	files, err := db.Files(driveID)
	if err != nil {
		return nil, err
	}
	return proxiesFrom(files, pair), nil
}

func proxiesFrom(files []File, pair ProxyPair) *ProxyReport {
	rep := &ProxyReport{ProxyPair: pair, Clips: []ProxyClip{}}
	on, off := pair.Online+"/", pair.Offline+"/"
	// Proxies by stem, exact and with the suffix stripped.
	proxies := map[string]string{}
	stripped := map[string]string{}
	var extra int
	for _, f := range files {
		if !strings.HasPrefix(f.Path, off) || media.KindOf(f.Path) != media.Video {
			continue
		}
		stem := clipStem(path.Base(f.Path))
		if _, ok := proxies[stem]; !ok {
			proxies[stem] = f.Path
		}
		if s := proxySuffix.ReplaceAllString(stem, ""); s != stem {
			if _, ok := stripped[s]; !ok {
				stripped[s] = f.Path
			}
		}
	}
	// Originals, with RED chunks folded into their clip.
	type clip struct {
		ProxyClip
		stem string
	}
	order := []string{}
	clips := map[string]*clip{}
	for _, f := range files {
		if !strings.HasPrefix(f.Path, on) || media.KindOf(f.Path) != media.Video {
			continue
		}
		key, name := f.Path, path.Base(f.Path)
		if strings.EqualFold(path.Ext(f.Path), ".r3d") {
			if dir := path.Dir(f.Path); strings.EqualFold(path.Ext(dir), ".rdc") {
				key, name = dir, strings.TrimSuffix(path.Base(dir), path.Ext(dir))
			} else {
				name = redChunk.ReplaceAllString(strings.TrimSuffix(name, path.Ext(name)), "") + path.Ext(name)
				key = path.Join(path.Dir(f.Path), name)
			}
		}
		c := clips[key]
		if c == nil {
			c = &clip{ProxyClip: ProxyClip{Path: key, Name: name}, stem: clipStem(name)}
			clips[key] = c
			order = append(order, key)
		}
		c.Bytes += f.Size
		c.Files++
	}
	used := map[string]bool{}
	for _, key := range order {
		c := clips[key]
		if p, ok := proxies[c.stem]; ok {
			c.Proxy = p
		} else if p, ok := stripped[c.stem]; ok {
			c.Proxy = p
		}
		if c.Proxy != "" {
			used[c.Proxy] = true
			rep.Proxied++
		} else {
			rep.MissingBytes += c.Bytes
		}
		rep.Clips = append(rep.Clips, c.ProxyClip)
	}
	rep.Total = len(rep.Clips)
	for _, p := range proxies {
		if !used[p] {
			extra++
		}
	}
	rep.Extra = extra
	return rep
}

// ProxyStates says, for every path on a drive that takes part in an
// Online/Offline pair, whether it is an original with a proxy
// ("proxied"), an original without one ("missing"), or a proxy itself
// ("proxy"). Folder paths of RED clips are included, as are their chunks.
func (db *DB) ProxyStates(driveID string) (map[string]string, error) {
	pairs, err := db.ProxyPairs(driveID, "")
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	if len(pairs) == 0 {
		return out, nil
	}
	files, err := db.Files(driveID)
	if err != nil {
		return nil, err
	}
	for _, pair := range pairs {
		rep := proxiesFrom(files, pair)
		for _, c := range rep.Clips {
			st := "missing"
			if c.Proxy != "" {
				st = "proxied"
				out[c.Proxy] = "proxy"
			}
			out[c.Path] = st
			if c.Files > 1 || strings.EqualFold(path.Ext(c.Path), ".rdc") {
				// Mark the chunks too, so a file row finds its state.
				for _, f := range files {
					if strings.HasPrefix(f.Path, c.Path+"/") || (strings.EqualFold(path.Ext(f.Path), ".r3d") && path.Dir(f.Path) == path.Dir(c.Path) && clipStem(path.Base(f.Path)) == clipStem(c.Name)) {
						out[f.Path] = st
					}
				}
			}
		}
	}
	return out, nil
}
