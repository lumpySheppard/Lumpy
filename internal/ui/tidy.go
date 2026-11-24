package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lumpysheppard/lumpy/internal/catalog"
	"github.com/lumpysheppard/lumpy/internal/job"
	"github.com/lumpysheppard/lumpy/internal/media"
)

// Tidy up: a project whose files are loose on a drive (a quick edit,
// someone else's layout, a short from before the house structure) is
// sorted into the standard template's folders. Lumpy classifies every
// file, shows the plan as groups to adjust, moves them on the same
// drive (a rename, so it is instant and nothing is copied), writes a
// log of old and new paths beside them, brings the library up to
// date, and can then tell DaVinci Resolve where everything went.

// tidyCategory is what a file is, as the template's folders see it.
type tidyCategory string

const (
	catCamera    tidyCategory = "camera"
	catProxy     tidyCategory = "proxy"
	catAudio     tidyCategory = "audio"
	catMusic     tidyCategory = "music"
	catMaster    tidyCategory = "master"
	catDraft     tidyCategory = "draft"
	catOutStill  tidyCategory = "outstill"
	catGraphics  tidyCategory = "graphics"
	catGfxExport tidyCategory = "gfxexport"
	catStills    tidyCategory = "stills"
	catStock     tidyCategory = "stock"
	catResolve   tidyCategory = "resolve"
	catPremiere  tidyCategory = "premiere"
	catProject   tidyCategory = "project"
	catScript    tidyCategory = "script"
	catDocs      tidyCategory = "docs"
	catOther     tidyCategory = "other"
)

var catLabels = map[tidyCategory]string{
	catCamera: "Camera originals", catProxy: "Proxies and offline", catAudio: "Audio", catMusic: "Music", catMaster: "Masters",
	catDraft: "Drafts and other exports", catOutStill: "Frame grabs and output stills", catGraphics: "Graphics sources", catGfxExport: "Graphics exports",
	catStills: "Stills and photos", catStock: "Stock footage", catResolve: "Resolve projects", catPremiere: "Premiere projects", catProject: "Other edit project files",
	catScript: "Scripts", catDocs: "Paperwork and documents", catOther: "Everything else",
}

// catWords are the words a template folder's name carries for each
// category, best first. A folder one level down is tried before a
// top-level one, so Online beats Original footage for camera files.
var catWords = map[tidyCategory][]string{
	catCamera:    {"online", "raw", "ocf", "camera", "rushes", "original", "footage", "source"},
	catProxy:     {"offline", "proxy", "proxies", "transcode"},
	catAudio:     {"audio", "sound", "sfx"},
	catMusic:     {"music", "score", "audio", "sound"},
	catMaster:    {"final", "master", "deliver", "output", "export"},
	catDraft:     {"draft", "output", "export", "render", "deliver"},
	catOutStill:  {"stills", "still", "output"},
	catGraphics:  {"graphics source", "graphic source", "graphics", "graphic", "gfx", "design", "artwork", "title", "motion", "vfx"},
	catGfxExport: {"graphics export", "graphic export", "graphics", "graphic", "gfx"},
	catStills:    {"photo", "picture", "stills source", "image"},
	catStock:     {"stock", "footage"},
	catResolve:   {"drp", "resolve", "davinci", "project"},
	catPremiere:  {"premiere", "prproj", "project"},
	catProject:   {"project", "edit", "nle", "timeline", "cut"},
	catScript:    {"script", "paperwork", "production"},
	catDocs:      {"paperwork", "doc", "admin", "production", "notes", "release"},
}

var rawVideo = map[string]bool{".braw": true, ".r3d": true, ".ari": true, ".arx": true, ".crm": true, ".mxf": true, ".cine": true, ".dng": true}
var projectExt = map[string]bool{".fcpxml": true, ".fcpbundle": true, ".aep": true, ".xml": true, ".edl": true, ".ale": true, ".otio": true, ".motn": true, ".drb": true, ".drt": true, ".dra": true}
var graphicExt = map[string]bool{".psd": true, ".psb": true, ".ai": true, ".afdesign": true, ".afphoto": true, ".sketch": true, ".fig": true, ".svg": true, ".mogrt": true, ".c4d": true, ".blend": true}
var scriptExt = map[string]bool{".fdx": true, ".fountain": true, ".celtx": true}
var cameraDirs = map[string]bool{"CLIPS": true, "DCIM": true, "PRIVATE": true, "XDROOT": true, "BPAV": true, "M4ROOT": true, "AVCHD": true, "CONTENTS": true}

// classify says what a file is from its name and the folders it sits in.
func classify(rel string) tidyCategory {
	parts := strings.Split(rel, "/")
	name := parts[len(parts)-1]
	lname := strings.ToLower(name)
	ext := strings.ToLower(path.Ext(name))
	dirs := parts[:len(parts)-1]
	inDir := func(words ...string) bool {
		for _, d := range dirs {
			ld := strings.ToLower(d)
			for _, w := range words {
				if strings.Contains(ld, w) {
					return true
				}
			}
		}
		return false
	}
	switch {
	case ext == ".drp":
		return catResolve
	case ext == ".prproj":
		return catPremiere
	case projectExt[ext]:
		return catProject
	case scriptExt[ext] || (strings.Contains(lname, "script") && (ext == ".pdf" || ext == ".docx" || ext == ".doc" || ext == ".txt")):
		return catScript
	case graphicExt[ext]:
		return catGraphics
	}
	gfxDir := inDir("graphic", "gfx", "title", "lower third", "motion", "vfx", "design")
	outDir := inDir("output", "export", "deliver", "final", "render", "draft")
	switch media.KindOf(name) {
	case media.Video:
		if strings.Contains(lname, "master") {
			return catMaster
		}
		if gfxDir {
			return catGfxExport
		}
		if inDir("stock") {
			return catStock
		}
		if outDir {
			if inDir("final", "master", "deliver") {
				return catMaster
			}
			return catDraft
		}
		if rawVideo[ext] {
			return catCamera
		}
		if catalog.Proxyish(rel) {
			return catProxy
		}
		return catCamera
	case media.Audio:
		if inDir("music", "score") || strings.Contains(lname, "music") {
			return catMusic
		}
		return catAudio
	case media.Image:
		if gfxDir {
			return catGfxExport
		}
		if outDir || inDir("still", "grab", "frame") {
			return catOutStill
		}
		return catStills
	case media.Document:
		return catDocs
	}
	return catOther
}

// folderFor picks the template folder for a category: the first word
// that names a folder one level down, else the first that names a
// top-level folder, else "" when the template has none for it.
func folderFor(cat tidyCategory, tv TemplateView) string {
	for _, w := range catWords[cat] {
		for _, f := range tv.Subfolders {
			if strings.Contains(strings.ToLower(path.Base(f)), w) {
				return f
			}
		}
	}
	for _, w := range catWords[cat] {
		for _, f := range tv.Folders {
			if strings.Contains(strings.ToLower(f), w) {
				return f
			}
		}
	}
	return ""
}

// allFolders is every folder the plan may send files to.
func allFolders(tv TemplateView) []string {
	out := append([]string{}, tv.Folders...)
	out = append(out, tv.Subfolders...)
	sort.Strings(out)
	return out
}

// topMatches says whether a file's own top folder already means the
// category, so it can be dropped when the file moves under the
// template's folder (Exports/final/x.mov becomes 09_OUTPUT/Final/x.mov).
func topMatches(top string, cat tidyCategory) bool {
	lt := strings.ToLower(top)
	for _, w := range catWords[cat] {
		if strings.Contains(lt, w) {
			return true
		}
	}
	return false
}

// inPlace says whether a file already sits under its destination.
func inPlace(rel, dest string) bool {
	return dest != "" && strings.HasPrefix(rel, dest+"/")
}

// TidyGroup is one row of the plan: the files of one category from one
// top-level folder of one project source, and where they would go.
type TidyGroup struct {
	ID       string       `json:"id"`
	DriveID  string       `json:"driveId"`
	Drive    string       `json:"drive"`
	Source   string       `json:"source"` // the project folder on the drive
	Top      string       `json:"top"`    // the files' top folder inside it, "" for the root
	Category tidyCategory `json:"category"`
	Label    string       `json:"label"`
	Dest     string       `json:"dest"` // template folder, "" to leave
	Files    int          `json:"files"`
	Bytes    int64        `json:"bytes"`
	Samples  []string     `json:"samples"`
	InPlace  bool         `json:"inPlace"` // already under the right folder
}

type tidyPlan struct {
	Template string      `json:"template"`
	Folders  []string    `json:"folders"`
	Groups   []TidyGroup `json:"groups"`
	Files    int         `json:"files"`
	Moving   int         `json:"moving"`
	InPlace  int         `json:"inPlace"`
	Unsorted int         `json:"unsorted"`
	Offline  []string    `json:"offline"` // sources whose drive is not plugged in
}

type tidyFile struct {
	rel  string // within the source folder
	size int64
	top  string
	cat  tidyCategory
}

// tidyFiles lists a source folder's files from the library, with what
// each is and the top folder it sits in.
func (s *Server) tidyFiles(src catalog.Source) ([]tidyFile, error) {
	files, err := s.db.Files(src.DriveID)
	if err != nil {
		return nil, err
	}
	prefix := ""
	if src.Folder != "" {
		prefix = src.Folder + "/"
	}
	var out []tidyFile
	for _, f := range files {
		if !strings.HasPrefix(f.Path, prefix) {
			continue
		}
		rel := strings.TrimPrefix(f.Path, prefix)
		base := path.Base(rel)
		if strings.HasPrefix(base, ".") || strings.HasPrefix(base, "_Lumpy tidy") || strings.HasSuffix(base, ".mhl") {
			continue
		}
		skip := false
		for _, ex := range src.Exclude {
			if rel == ex || strings.HasPrefix(rel, ex+"/") {
				skip = true
			}
		}
		if skip {
			continue
		}
		top := ""
		if i := strings.Index(rel, "/"); i >= 0 {
			top = rel[:i]
		}
		out = append(out, tidyFile{rel: rel, size: f.Size, top: top, cat: classify(rel)})
	}
	return out, nil
}

func (s *Server) tidyPlanFor(p *catalog.Project, template string) (*tidyPlan, error) {
	tv, err := templateView(template)
	if err != nil {
		return nil, badRequest("%v", err)
	}
	plan := &tidyPlan{Template: template, Folders: allFolders(tv), Groups: []TidyGroup{}, Offline: []string{}}
	groups := map[string]*TidyGroup{}
	for _, src := range p.Sources {
		if src.Role == "backup" {
			continue
		}
		d, err := s.db.Drive(src.DriveID)
		if err != nil {
			continue
		}
		if s.mountOf(src.DriveID) == nil {
			plan.Offline = append(plan.Offline, d.Title()+"/"+src.Folder)
			continue
		}
		files, err := s.tidyFiles(src)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			plan.Files++
			dest := folderFor(f.cat, tv)
			here := inPlace(f.rel, dest)
			k := src.DriveID + "\x00" + src.Folder + "\x00" + f.top + "\x00" + string(f.cat) + "\x00" + fmt.Sprint(here)
			g := groups[k]
			if g == nil {
				g = &TidyGroup{ID: fmt.Sprintf("%d", len(groups)+1), DriveID: src.DriveID, Drive: d.Title(), Source: src.Folder, Top: f.top, Category: f.cat, Label: catLabels[f.cat], Dest: dest, InPlace: here}
				groups[k] = g
				plan.Groups = append(plan.Groups, *g)
			}
			g.Files++
			g.Bytes += f.size
			if len(g.Samples) < 3 {
				g.Samples = append(g.Samples, path.Base(f.rel))
			}
		}
	}
	plan.Groups = plan.Groups[:0]
	for _, g := range groups {
		if g.InPlace {
			plan.InPlace += g.Files
			g.Dest = ""
		} else if g.Dest == "" {
			plan.Unsorted += g.Files
		} else {
			plan.Moving += g.Files
		}
		plan.Groups = append(plan.Groups, *g)
	}
	sort.Slice(plan.Groups, func(i, j int) bool {
		a, b := plan.Groups[i], plan.Groups[j]
		if a.InPlace != b.InPlace {
			return !a.InPlace
		}
		if a.Bytes != b.Bytes {
			return a.Bytes > b.Bytes
		}
		return a.Top < b.Top
	})
	return plan, nil
}

// tidyPlanHandler is POST /api/projects/{id}/tidy/plan {template}.
func (s *Server) tidyPlanHandler(w http.ResponseWriter, r *http.Request) (any, error) {
	p, err := s.projectFrom(r)
	if err != nil {
		return nil, err
	}
	var body struct {
		Template string `json:"template"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, badRequest("bad request: %v", err)
	}
	if body.Template == "" {
		return nil, badRequest("choose a template")
	}
	return s.tidyPlanFor(p, body.Template)
}

// tidyMove is one group as the page sends it back: where it goes now.
type tidyMove struct {
	DriveID  string       `json:"driveId"`
	Source   string       `json:"source"`
	Top      string       `json:"top"`
	Category tidyCategory `json:"category"`
	Dest     string       `json:"dest"`
}

// tidyRunHandler is POST /api/projects/{id}/tidy/run {template, moves}.
func (s *Server) tidyRunHandler(w http.ResponseWriter, r *http.Request) (any, error) {
	p, err := s.projectFrom(r)
	if err != nil {
		return nil, err
	}
	var body struct {
		Template string     `json:"template"`
		Moves    []tidyMove `json:"moves"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, badRequest("bad request: %v", err)
	}
	tv, err := templateView(body.Template)
	if err != nil {
		return nil, badRequest("%v", err)
	}
	want := map[string]string{}
	for _, m := range body.Moves {
		if m.Dest == "" {
			continue
		}
		ok := false
		for _, f := range allFolders(tv) {
			ok = ok || f == m.Dest
		}
		if !ok {
			return nil, badRequest("%s is not a folder of the template", m.Dest)
		}
		want[m.DriveID+"\x00"+m.Source+"\x00"+m.Top+"\x00"+string(m.Category)] = m.Dest
	}
	if len(want) == 0 {
		return nil, badRequest("nothing to move")
	}
	// Every move, worked out now so the job has nothing to decide.
	type move struct {
		driveID        string
		mount, root    string
		from, to       string // absolute
		relFrom, relTo string // within the project folder
	}
	var moves []move
	var drives []string
	seenDrive := map[string]bool{}
	for _, src := range p.Sources {
		if src.Role == "backup" {
			continue
		}
		v := s.mountOf(src.DriveID)
		if v == nil {
			continue
		}
		files, err := s.tidyFiles(src)
		if err != nil {
			return nil, err
		}
		root := filepath.Join(v.Mount, filepath.FromSlash(src.Folder))
		for _, f := range files {
			dest := want[src.DriveID+"\x00"+src.Folder+"\x00"+f.top+"\x00"+string(f.cat)]
			if dest == "" || inPlace(f.rel, dest) {
				continue
			}
			rest := f.rel
			if f.top != "" && topMatches(f.top, f.cat) {
				rest = strings.TrimPrefix(rest, f.top+"/")
			}
			// A folder named like its destination (Exports/final into
			// 09_OUTPUT/Final) is not kept as an extra level.
			if seg, tail, ok := strings.Cut(rest, "/"); ok && strings.EqualFold(seg, path.Base(dest)) {
				rest = tail
			}
			relTo := dest + "/" + rest
			if relTo == f.rel {
				continue
			}
			moves = append(moves, move{driveID: src.DriveID, mount: v.Mount, root: root, from: filepath.Join(root, filepath.FromSlash(f.rel)), to: filepath.Join(root, filepath.FromSlash(relTo)), relFrom: f.rel, relTo: relTo})
			if !seenDrive[src.DriveID] {
				seenDrive[src.DriveID] = true
				drives = append(drives, src.DriveID)
			}
		}
	}
	if len(moves) == 0 {
		return nil, badRequest("every file is already where it belongs")
	}
	n := len(moves)
	title := fmt.Sprintf("Tidy up %s: move %d file%s", p.Name, n, plural(n))
	pid := p.ID
	j := s.jobs.StartWhenFree("tidy", title, "lumpy tidy "+shellQuote(p.Name), drives, func(ctx context.Context, rep *job.Report) error {
		rep.Info("Sorting %d file%s into %s's folders. Files move on their own drive, so nothing is copied.", n, plural(n), filepath.Base(body.Template))
		// The template's folders exist in every project root first.
		roots := map[string]bool{}
		for _, m := range moves {
			roots[m.root] = true
		}
		for root := range roots {
			for _, f := range allFolders(tv) {
				os.MkdirAll(filepath.Join(root, filepath.FromSlash(f)), 0o755)
			}
		}
		done := 0
		var paths []pullPath
		logs := map[string][]map[string]string{}
		prog := rep.StartProgress("Moving", int64(len(moves)))
		defer prog.Finish()
		for _, m := range moves {
			if err := ctx.Err(); err != nil {
				return err
			}
			if _, err := os.Stat(m.to); err == nil {
				rep.Warn("%s: %s already exists; left where it was", m.relFrom, m.relTo)
				continue
			}
			if err := os.MkdirAll(filepath.Dir(m.to), 0o755); err != nil {
				rep.Fail("%s: %v", m.relFrom, err)
				continue
			}
			if err := os.Rename(m.from, m.to); err != nil {
				rep.Fail("%s: %v", m.relFrom, err)
				continue
			}
			done++
			logs[m.root] = append(logs[m.root], map[string]string{"from": m.relFrom, "to": m.relTo})
			d, _ := s.db.Drive(m.driveID)
			title := m.driveID
			if d != nil {
				title = d.Title()
			}
			rel, _ := filepath.Rel(m.mount, m.to)
			paths = append(paths, pullPath{Name: filepath.Base(m.to), Drive: title, DriveID: m.driveID, Path: filepath.ToSlash(rel), Full: m.to, Mounted: true})
			prog.Add(1)
			prog.SetCurrent(m.relTo)
		}
		// Empty folders the files left behind go too, from the deepest up.
		for _, m := range moves {
			dir := filepath.Dir(m.from)
			for dir != m.root && strings.HasPrefix(dir, m.root) {
				if entries, err := os.ReadDir(dir); err != nil || len(entries) > 0 {
					if len(entries) == 1 && entries[0].Name() == ".DS_Store" {
						os.Remove(filepath.Join(dir, ".DS_Store"))
					} else {
						break
					}
				}
				if os.Remove(dir) != nil {
					break
				}
				dir = filepath.Dir(dir)
			}
		}
		for root, list := range logs {
			name := filepath.Join(root, "_Lumpy tidy "+time.Now().Format("2006-01-02 15.04")+".json")
			b, _ := json.MarshalIndent(map[string]any{"project": p.Name, "template": body.Template, "at": time.Now().Format(time.RFC3339), "moves": list}, "", "  ")
			if err := os.WriteFile(name, b, 0o644); err == nil {
				rep.Info("Wrote %s: every old and new path, in case anything needs undoing.", filepath.Base(name))
			}
		}
		s.archMu.Lock()
		if s.tidies == nil {
			s.tidies = map[int64][]pullPath{}
		}
		s.tidies[pid] = paths
		s.archMu.Unlock()
		rep.Info("%d of %d file%s moved. Bringing the library up to date.", done, n, plural(n))
		for _, id := range drives {
			if v := s.mountOf(id); v != nil {
				if _, err := job.CatalogDrive(ctx, s.db, v, rep); err != nil {
					rep.Warn("rescan: %v", err)
				}
			}
		}
		s.invalidate()
		if rep.Failures() > 0 {
			return fmt.Errorf("%d file%s could not be moved", rep.Failures(), plural(rep.Failures()))
		}
		return nil
	})
	return map[string]any{"job": j.ID, "files": n}, nil
}

// tidyRelinkHandler is POST /api/projects/{id}/tidy/relink: Resolve is
// told where the last tidy-up put every clip.
func (s *Server) tidyRelinkHandler(w http.ResponseWriter, r *http.Request) (any, error) {
	p, err := s.projectFrom(r)
	if err != nil {
		return nil, err
	}
	s.archMu.Lock()
	paths := s.tidies[p.ID]
	s.archMu.Unlock()
	if len(paths) == 0 {
		return nil, badRequest("no tidy-up to relink: run one first, in this session")
	}
	j, err := s.runRelink(paths, fmt.Sprintf("Relink %s in DaVinci Resolve after tidying", p.Name))
	if err != nil {
		return nil, err
	}
	return map[string]any{"job": j.ID}, nil
}
