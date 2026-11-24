package ui

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lumpysheppard/lumpy/internal/job"
)

// Handing a pull to DaVinci Resolve instead of copying. Resolve's
// scripting API (Python, with Resolve open and Preferences → System →
// General → External scripting set to Local) can replace the file
// behind a media pool clip, which keeps every timeline use of it.
// Lumpy writes the clip-to-path list, runs the script as a job, and
// the Activity log says what was relinked.

// pullPath is one chosen copy with its full path on the Mac.
type pullPath struct {
	Name    string `json:"name"` // the clip as the edit names it
	Drive   string `json:"drive"`
	DriveID string `json:"driveId"`
	Path    string `json:"path"`    // relative to the drive
	Full    string `json:"full"`    // the path on this Mac, now or when last seen
	Mounted bool   `json:"mounted"` // the drive is plugged in
	Proxy   bool   `json:"proxy"`
	Bytes   int64  `json:"bytes"`
	Files   int    `json:"files"`
}

// pullPicks resolves the page's picks against a kept analysis.
func (s *Server) pullPicks(token string, picks []struct {
	DriveID string `json:"driveId"`
	Path    string `json:"path"`
}) ([]pullPath, error) {
	s.archMu.Lock()
	run := s.pulls[token]
	s.archMu.Unlock()
	if run == nil {
		return nil, badRequest("that analysis has expired; choose the export again")
	}
	hits := map[string]struct {
		name  string
		proxy bool
		bytes int64
		files int
	}{}
	for _, ref := range run.report.Refs {
		for _, h := range ref.Hits {
			k := h.DriveID + "\x00" + h.Path
			if _, ok := hits[k]; !ok {
				hits[k] = struct {
					name  string
					proxy bool
					bytes int64
					files int
				}{ref.Name, h.Proxy, h.Bytes, h.Files}
			}
		}
	}
	var out []pullPath
	for _, p := range picks {
		h, ok := hits[p.DriveID+"\x00"+p.Path]
		if !ok {
			return nil, badRequest("%s is not one of the clips found", p.Path)
		}
		d, err := s.db.Drive(p.DriveID)
		if err != nil {
			return nil, err
		}
		pp := pullPath{Name: h.name, Drive: d.Title(), DriveID: d.ID, Path: p.Path, Proxy: h.proxy, Bytes: h.bytes, Files: h.files}
		if v := s.mountOf(d.ID); v != nil {
			pp.Mounted = true
			pp.Full = filepath.Join(v.Mount, filepath.FromSlash(p.Path))
		} else {
			pp.Full = filepath.Join(d.LastMount, filepath.FromSlash(p.Path))
		}
		out = append(out, pp)
	}
	return out, nil
}

// pullPaths lists the chosen copies with their full paths, as JSON or,
// with ?format=csv, a CSV for Resolve, Premiere or a spreadsheet.
func (s *Server) pullPaths(w http.ResponseWriter, r *http.Request) (any, error) {
	var body struct {
		Token  string `json:"token"`
		Format string `json:"format"`
		Picks  []struct {
			DriveID string `json:"driveId"`
			Path    string `json:"path"`
		} `json:"picks"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, badRequest("bad request: %v", err)
	}
	paths, err := s.pullPicks(body.Token, body.Picks)
	if err != nil {
		return nil, err
	}
	if body.Format != "csv" && body.Format != "txt" {
		if paths == nil {
			paths = []pullPath{}
		}
		return paths, nil
	}
	w.Header().Set("Cache-Control", "no-store")
	if body.Format == "txt" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		for _, p := range paths {
			fmt.Fprintln(w, p.Full)
		}
		return nil, nil
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	fmt.Fprintln(w, "Clip,Path,Drive,Plugged in,Proxy,Bytes")
	q := func(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }
	for _, p := range paths {
		fmt.Fprintf(w, "%s,%s,%s,%v,%v,%d\n", q(p.Name), q(p.Full), q(p.Drive), p.Mounted, p.Proxy, p.Bytes)
	}
	return nil, nil
}

// resolveScript is the Python Resolve runs. It reads the clip list
// from the file in argv[1], walks the media pool, and replaces each
// clip whose name (or its file's name, proxy suffix stripped) matches
// a pulled clip with the pulled path.
const resolveScript = `
import json, os, re, sys
api = os.environ.get("RESOLVE_SCRIPT_API", "/Library/Application Support/Blackmagic Design/DaVinci Resolve/Developer/Scripting")
sys.path.append(os.path.join(api, "Modules"))
try:
    import DaVinciResolveScript as dvr
except Exception as e:
    print("ERR cannot load Resolve's scripting module: %s" % e)
    print("ERR install DaVinci Resolve and open it; the module lives in %s" % api)
    sys.exit(2)
resolve = dvr.scriptapp("Resolve")
if resolve is None:
    print("ERR Resolve is not running, or external scripting is off: Preferences > System > General > External scripting using: Local")
    sys.exit(2)
pulls = json.load(open(sys.argv[1]))
pm = resolve.GetProjectManager()
proj = pm.GetCurrentProject()
if proj is None:
    print("ERR open the project in Resolve first")
    sys.exit(2)
print("Project: %s" % proj.GetName())
mp = proj.GetMediaPool()
suffix = re.compile(r"[\s_\-.]*(proxy|prx|offline|lo-?res|s\d{2}|_\d)$", re.I)
chunk = re.compile(r"_\d{3}$")
def stem(name):
    s = os.path.splitext(os.path.basename(name))[0].lower()
    return s
def keys(name):
    s = stem(name)
    out = {s, suffix.sub("", s)}
    if name.lower().endswith(".r3d") or chunk.search(s):
        out.add(chunk.sub("", s))
    return out
want = {}
for p in pulls:
    for k in keys(p["name"]):
        want.setdefault(k, p)
def walk(folder):
    for c in folder.GetClipList() or []:
        yield c
    for sub in folder.GetSubFolderList() or []:
        for c in walk(sub):
            yield c
done, skipped, seen = 0, 0, 0
for clip in walk(mp.GetRootFolder()):
    seen += 1
    try:
        name = clip.GetName() or ""
        fp = clip.GetClipProperty("File Path") or ""
    except Exception:
        continue
    hit = None
    for k in keys(name) | keys(fp):
        if k in want:
            hit = want[k]; break
    if hit is None:
        continue
    target = hit["full"]
    if not os.path.exists(target):
        print("SKIP %s: %s is not plugged in (%s)" % (name, hit["drive"], target))
        skipped += 1
        continue
    if os.path.isdir(target):
        # a RED clip folder: point at its first chunk
        chunks = sorted(f for f in os.listdir(target) if f.lower().endswith(".r3d"))
        if not chunks:
            print("SKIP %s: no .R3D inside %s" % (name, target)); skipped += 1; continue
        target = os.path.join(target, chunks[0])
    if os.path.normpath(fp) == os.path.normpath(target):
        print("  same   %s already points at %s" % (name, target)); continue
    ok = False
    try:
        ok = clip.ReplaceClip(target)
    except Exception as e:
        print("FAIL %s: %s" % (name, e))
    if ok:
        print("  linked %s -> %s" % (name, target)); done += 1
    else:
        print("FAIL %s: Resolve would not replace it with %s" % (name, target)); skipped += 1
print("%d clips in the media pool, %d relinked, %d skipped" % (seen, done, skipped))
`

// relinkResolve runs the script as a job, so its lines land in Activity.
func (s *Server) relinkResolve(w http.ResponseWriter, r *http.Request) (any, error) {
	var body struct {
		Token string `json:"token"`
		Picks []struct {
			DriveID string `json:"driveId"`
			Path    string `json:"path"`
		} `json:"picks"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, badRequest("bad request: %v", err)
	}
	paths, err := s.pullPicks(body.Token, body.Picks)
	if err != nil {
		return nil, err
	}
	n := len(paths)
	j, err := s.runRelink(paths, fmt.Sprintf("Relink %d clip%s in DaVinci Resolve", n, plural(n)))
	if err != nil {
		return nil, err
	}
	return map[string]any{"job": j.ID}, nil
}

// runRelink asks Resolve, through its scripting API, to point every
// clip in the open project's media pool at the path given for its name.
func (s *Server) runRelink(paths []pullPath, title string) (*Job, error) {
	python, err := exec.LookPath("python3")
	if err != nil {
		return nil, badRequest("python3 is needed to talk to Resolve: install Xcode's command line tools (xcode-select --install) or python.org's Python 3")
	}
	sort.SliceStable(paths, func(i, j int) bool { return paths[i].Name < paths[j].Name })
	n := len(paths)
	j := s.jobs.Start("relink", title, "lumpy pull --relink resolve", "", func(ctx context.Context, rep *job.Report) error {
		dir, err := os.MkdirTemp("", "lumpy-resolve")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dir)
		list := filepath.Join(dir, "clips.json")
		data, _ := json.Marshal(paths)
		if err := os.WriteFile(list, data, 0o600); err != nil {
			return err
		}
		script := filepath.Join(dir, "relink.py")
		if err := os.WriteFile(script, []byte(resolveScript), 0o600); err != nil {
			return err
		}
		rep.Info("Asking Resolve to relink %d clip%s to the originals…", n, plural(n))
		cmd := exec.CommandContext(ctx, python, script, list)
		cmd.Env = append(os.Environ(), resolveEnv()...)
		out, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}
		cmd.Stderr = cmd.Stdout
		if err := cmd.Start(); err != nil {
			return err
		}
		sc := bufio.NewScanner(out)
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "ERR "):
				rep.Fail("%s", line[4:])
			case strings.HasPrefix(line, "FAIL "):
				rep.Fail("%s", line[5:])
			case strings.HasPrefix(line, "SKIP "):
				rep.Warn("%s", line[5:])
			default:
				rep.Info("%s", line)
			}
		}
		if err := cmd.Wait(); err != nil && rep.Failures() == 0 {
			return fmt.Errorf("the Resolve script ended with %v", err)
		}
		if rep.Failures() > 0 {
			return fmt.Errorf("%d clip%s could not be relinked", rep.Failures(), plural(rep.Failures()))
		}
		return nil
	})
	return j, nil
}

// resolveEnv points Python at Resolve's scripting module and library
// where Resolve installs them on a Mac, unless already set.
func resolveEnv() []string {
	var env []string
	if os.Getenv("RESOLVE_SCRIPT_API") == "" {
		env = append(env, "RESOLVE_SCRIPT_API=/Library/Application Support/Blackmagic Design/DaVinci Resolve/Developer/Scripting")
	}
	if os.Getenv("RESOLVE_SCRIPT_LIB") == "" {
		env = append(env, "RESOLVE_SCRIPT_LIB=/Applications/DaVinci Resolve/DaVinci Resolve.app/Contents/Libraries/Fusion/fusionscript.so")
	}
	return env
}
