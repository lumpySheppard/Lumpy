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
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/lumpysheppard/lumpy/internal/catalog"
	"github.com/lumpysheppard/lumpy/internal/job"
	"github.com/lumpysheppard/lumpy/internal/volume"
)

// Proxies through DaVinci Resolve. Lumpy cannot decode RAW itself
// (each camera maker's format needs that maker's SDK); Resolve decodes
// them all and its scripting API renders. The job makes a throwaway
// Resolve project, one timeline per clip, renders ProRes 422 Proxy
// into the Offline folder with the originals' names, then deletes the
// project. Resolve is opened if it is not running.

// proxyClip is one original to render.
type proxyClip struct {
	Path string `json:"path"` // on this Mac; a RED clip's .RDC folder
	Stem string `json:"stem"` // the proxy's name without extension
}

// proxySpec is what the script is handed.
type proxySpec struct {
	Project string      `json:"project"`
	Dest    string      `json:"dest"`
	Scale   string      `json:"scale"` // full, half, quarter
	Clips   []proxyClip `json:"clips"`
}

// proxyWant is the proxy wish that rides along with an offload.
type proxyWant struct {
	On    bool   `json:"on"`
	Scale string `json:"scale"`
}

// proxyable says which files get a proxy: RAW and the camera codecs.
var proxyable = map[string]bool{".braw": true, ".r3d": true, ".ari": true, ".arx": true, ".crm": true, ".mxf": true, ".cine": true,
	".mov": true, ".mp4": true, ".m4v": true, ".mts": true, ".m2ts": true, ".avi": true, ".mkv": true}

var redChunkRe = regexp.MustCompile(`(?i)_\d{3}\.r3d$`)

func proxyScale(s string) string {
	switch strings.ToLower(s) {
	case "full", "quarter":
		return strings.ToLower(s)
	}
	return "half"
}

// proxyDirFor is where the proxies of a copied folder go: the Offline
// folder beside the Online folder it sits under, keeping the path
// beneath Online (Online/2026-11-25/A001 → Offline/2026-11-25/A001);
// without an Online folder above it, a Proxies folder beside it.
func proxyDirFor(copied string) string {
	copied = filepath.Clean(copied)
	for dir := filepath.Dir(copied); dir != "/" && dir != "." && filepath.Dir(dir) != dir; dir = filepath.Dir(dir) {
		if !catalog.IsOnline(filepath.Base(dir)) {
			continue
		}
		parent := filepath.Dir(dir)
		off := filepath.Join(parent, "Offline")
		if ents, err := os.ReadDir(parent); err == nil {
			for _, e := range ents {
				if e.IsDir() && catalog.IsOffline(e.Name()) {
					off = filepath.Join(parent, e.Name())
					break
				}
			}
		}
		rel, _ := filepath.Rel(dir, copied)
		return filepath.Join(off, rel)
	}
	return filepath.Join(filepath.Dir(copied), "Proxies", filepath.Base(copied))
}

// clipsUnder lists the originals under a folder, RED clips as their
// .RDC folders, each once.
func clipsUnder(root string) []proxyClip {
	var out []proxyClip
	seen := map[string]bool{}
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if !proxyable[ext] {
			return nil
		}
		if ext == ".r3d" {
			dir := filepath.Dir(p)
			if seen[dir] {
				return nil
			}
			seen[dir] = true
			stem := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))
			stem = regexp.MustCompile(`_\d{3}$`).ReplaceAllString(stem, "")
			out = append(out, proxyClip{Path: dir, Stem: stem})
			return nil
		}
		out = append(out, proxyClip{Path: p, Stem: strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))})
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// withoutProxies drops the clips that already have a proxy in dest.
func withoutProxies(clips []proxyClip, dest string) []proxyClip {
	var out []proxyClip
	for _, c := range clips {
		if _, err := os.Stat(filepath.Join(dest, c.Stem+".mov")); err == nil {
			continue
		}
		out = append(out, c)
	}
	return out
}

// startProxies queues the render. clips is worked out when the job
// runs, since an offload may still be copying; after, when set, is
// the offload the proxies follow: the job waits for it and stands
// down unless it verified. No drive is held, so the next card's
// offload is not kept waiting by a render.
func (s *Server) startProxies(title string, clips func() ([]proxyClip, string, error), scale string, after *Job) *Job {
	scale = proxyScale(scale)
	return s.jobs.Start("proxies", title, "lumpy proxies --via resolve", "", func(ctx context.Context, rep *job.Report) error {
		if after != nil {
			rep.Info("Waiting for %s to finish…", after.Title)
			for {
				after.mu.Lock()
				st := after.State
				after.mu.Unlock()
				if st != "running" && st != "queued" {
					if st != "ok" {
						rep.Warn("%s ended %s, so no proxies were made", after.Title, st)
						return fmt.Errorf("the offload did not finish cleanly")
					}
					break
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(2 * time.Second):
				}
			}
		}
		list, dest, err := clips()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return fmt.Errorf("making %s: %w", dest, err)
		}
		list = withoutProxies(list, dest)
		if len(list) == 0 {
			rep.Info("Every clip already has a proxy in %s", dest)
			return nil
		}
		return s.renderProxies(ctx, rep, list, dest, scale)
	})
}

func (s *Server) renderProxies(ctx context.Context, rep *job.Report, clips []proxyClip, dest, scale string) error {
	python, err := exec.LookPath("python3")
	if err != nil {
		return fmt.Errorf("python3 is needed to talk to Resolve: install Xcode's command line tools (xcode-select --install) or python.org's Python 3")
	}
	dir, err := os.MkdirTemp("", "lumpy-resolve")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	spec := proxySpec{Project: "Lumpy proxies " + time.Now().Format("2006-01-02 15.04.05"), Dest: dest, Scale: scale, Clips: clips}
	data, _ := json.Marshal(spec)
	specPath := filepath.Join(dir, "proxies.json")
	if err := os.WriteFile(specPath, data, 0o600); err != nil {
		return err
	}
	script := filepath.Join(dir, "proxies.py")
	if err := os.WriteFile(script, []byte(resolveProxyScript), 0o600); err != nil {
		return err
	}
	n := len(clips)
	rep.Info("%d clip%s to render as ProRes 422 Proxy (%s size) into %s", n, plural(n), scale, dest)
	prog := rep.StartProgress("Rendering", int64(n))
	cmd := exec.CommandContext(ctx, python, script, specPath)
	cmd.Env = append(os.Environ(), resolveEnv()...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return err
	}
	failed, fatal := 0, ""
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "ERR "):
			rep.Fail("%s", line[4:])
			if fatal == "" {
				fatal = line[4:]
			}
		case strings.HasPrefix(line, "FAIL "):
			rep.Fail("%s", line[5:])
			failed++
			prog.Add(1)
		case strings.HasPrefix(line, "SKIP "):
			rep.Warn("%s", line[5:])
			prog.Add(1)
		case strings.HasPrefix(line, "DONE "):
			rep.Info("Rendered %s", line[5:])
			prog.Add(1)
		case strings.HasPrefix(line, "NOW "):
			prog.SetCurrent(line[4:])
		default:
			rep.Info("%s", line)
		}
	}
	err = cmd.Wait()
	prog.Finish()
	if fatal != "" {
		return fmt.Errorf("%s", fatal)
	}
	if err != nil && rep.Failures() == 0 {
		return fmt.Errorf("the Resolve script ended with %v", err)
	}
	// The proxies are new files on the drive; the library follows.
	if v, err := volume.Of(dest); err == nil {
		if _, err := job.CatalogDrive(ctx, s.db, v, rep); err != nil {
			rep.Warn("rescan: %v", err)
		}
		s.invalidate()
	}
	if failed > 0 {
		return fmt.Errorf("%d clip%s could not be rendered", failed, plural(failed))
	}
	return nil
}

// proxiesAfterOffload queues proxies for the card an offload is copying,
// into the Offline folder beside the destination's Online folder.
func (s *Server) proxiesAfterOffload(off *Job, source, dest0, name, scale string) *Job {
	card := name
	if card == "" {
		card = filepath.Base(filepath.Clean(source))
	}
	copied := filepath.Join(dest0, card)
	return s.startProxies(fmt.Sprintf("Proxies for %s via Resolve", card), func() ([]proxyClip, string, error) {
		if _, err := os.Stat(copied); err != nil {
			return nil, "", fmt.Errorf("the copy at %s is not there", copied)
		}
		return clipsUnder(copied), proxyDirFor(copied), nil
	}, scale, off)
}

// makeProxies is POST /api/proxies: {driveId, online, scale} renders the
// originals in an Online folder that have no proxy yet, into the
// Offline folder beside it.
func (s *Server) makeProxies(w http.ResponseWriter, r *http.Request) (any, error) {
	var body struct {
		DriveID string `json:"driveId"`
		Online  string `json:"online"`
		Scale   string `json:"scale"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, badRequest("bad request: %v", err)
	}
	v := s.mountOf(body.DriveID)
	if v == nil {
		return nil, badRequest("that drive is not plugged in")
	}
	reps, err := s.db.ProxyReports(body.DriveID, body.Online)
	if err != nil {
		return nil, err
	}
	var rep *catalog.ProxyReport
	for _, x := range reps {
		if x.Online == body.Online {
			rep = x
		}
	}
	if rep == nil {
		return nil, badRequest("no Online folder at %s", body.Online)
	}
	var clips []proxyClip
	for _, c := range rep.Clips {
		if c.Proxy != "" {
			continue
		}
		clips = append(clips, proxyClip{Path: filepath.Join(v.Mount, filepath.FromSlash(c.Path)), Stem: strings.TrimSuffix(c.Name, filepath.Ext(c.Name))})
	}
	if len(clips) == 0 {
		return nil, badRequest("every original there has a proxy already")
	}
	dest := filepath.Join(v.Mount, filepath.FromSlash(rep.Offline))
	if rep.Offline == "" {
		dest = filepath.Join(filepath.Dir(filepath.Join(v.Mount, filepath.FromSlash(body.Online))), "Offline")
	}
	j := s.startProxies(fmt.Sprintf("Proxies for %s via Resolve", filepath.Base(body.Online)), func() ([]proxyClip, string, error) { return clips, dest, nil }, body.Scale, nil)
	return map[string]any{"job": j.ID, "count": len(clips), "dest": dest}, nil
}

// resolveProxyScript is the Python Resolve runs: a throwaway project,
// one timeline per clip at the clip's own size and frame rate, a
// ProRes 422 Proxy render of each named after the clip, and the
// project deleted at the end.
const resolveProxyScript = `
import json, os, subprocess, sys, time
api = os.environ.get("RESOLVE_SCRIPT_API", "/Library/Application Support/Blackmagic Design/DaVinci Resolve/Developer/Scripting")
sys.path.append(os.path.join(api, "Modules"))
try:
    import DaVinciResolveScript as dvr
except Exception as e:
    print("ERR cannot load Resolve's scripting module: %s" % e)
    print("ERR install DaVinci Resolve; the module lives in %s" % api)
    sys.exit(2)
spec = json.load(open(sys.argv[1]))
resolve = dvr.scriptapp("Resolve")
if resolve is None:
    print("Opening DaVinci Resolve...")
    subprocess.Popen(["open", "-a", "DaVinci Resolve"])
    for i in range(60):
        time.sleep(3)
        resolve = dvr.scriptapp("Resolve")
        if resolve is not None:
            break
if resolve is None:
    print("ERR Resolve did not answer. Open it, and set Preferences > System > General > External scripting using: Local")
    sys.exit(2)
pm = resolve.GetProjectManager()
name = spec["project"]
proj = pm.CreateProject(name)
if not proj:
    print("ERR Resolve would not make a project (a project with unsaved changes may be open; save or close it)")
    sys.exit(2)
print("Made Resolve project %s" % name)
mp = proj.GetMediaPool()
codecs = proj.GetRenderCodecs("mov") or {}
codec = None
for desc, key in codecs.items():
    d = (desc or "").lower()
    if "prores" in d and "proxy" in d:
        codec = key
        break
if codec is None:
    for desc, key in codecs.items():
        if "proxy" in (key or "").lower():
            codec = key
            break
if codec is None:
    codec = "ProRes422Proxy"
dest = spec["dest"]
scale = {"full": 1, "half": 2, "quarter": 4}.get(spec.get("scale", "half"), 2)
jobs = []
def even(n):
    n = int(round(n))
    return n if n % 2 == 0 else n + 1
for c in spec["clips"]:
    stem = c["stem"]
    print("NOW " + stem)
    items = mp.ImportMedia([c["path"]]) or []
    if not items:
        print("FAIL %s: Resolve could not import %s" % (stem, c["path"]))
        continue
    item = items[0]
    res = item.GetClipProperty("Resolution") or ""
    fps = item.GetClipProperty("FPS") or ""
    try:
        w, h = [int(x) for x in res.lower().split("x")]
    except Exception:
        w, h = 1920, 1080
    tl = mp.CreateTimelineFromClips(stem, [item])
    if not tl:
        print("FAIL %s: could not make a timeline" % stem)
        continue
    try:
        tl.SetSetting("useCustomSettings", "1")
        tl.SetSetting("timelineResolutionWidth", str(w))
        tl.SetSetting("timelineResolutionHeight", str(h))
        if fps:
            tl.SetSetting("timelineFrameRate", str(fps))
    except Exception as e:
        print("timeline settings for %s: %s" % (stem, e))
    proj.SetCurrentTimeline(tl)
    proj.SetCurrentRenderFormatAndCodec("mov", codec)
    proj.SetRenderSettings({"SelectAllFrames": True, "TargetDir": dest, "CustomName": stem, "UniqueFilenameStyle": 0,
        "ExportVideo": True, "ExportAudio": True, "FormatWidth": even(w / scale), "FormatHeight": even(h / scale)})
    jid = proj.AddRenderJob()
    if not jid:
        print("FAIL %s: Resolve would not queue the render" % stem)
        continue
    jobs.append((jid, stem))
if jobs:
    print("Rendering %d clip(s) as %s, %s size..." % (len(jobs), codec, spec.get("scale", "half")))
    proj.StartRendering([j for j, _ in jobs], False)
    reported = set()
    while True:
        busy = proj.IsRenderingInProgress()
        for jid, stem in jobs:
            if jid in reported:
                continue
            st = proj.GetRenderJobStatus(jid) or {}
            status = (st.get("JobStatus") or "").lower()
            if status in ("complete", "completed"):
                reported.add(jid)
                out = os.path.join(dest, stem + ".mov")
                if os.path.exists(out):
                    print("DONE %s" % stem)
                else:
                    print("FAIL %s: Resolve said done but %s is not there" % (stem, out))
            elif status in ("failed", "cancelled", "canceled"):
                reported.add(jid)
                print("FAIL %s: %s %s" % (stem, status, st.get("Error") or ""))
            else:
                print("NOW %s %s%%" % (stem, st.get("CompletionPercentage") or 0))
        if not busy and len(reported) == len(jobs):
            break
        if not busy:
            # the queue stopped with jobs unaccounted for
            for jid, stem in jobs:
                if jid not in reported:
                    reported.add(jid)
                    print("FAIL %s: the render stopped" % stem)
            break
        time.sleep(2)
try:
    pm.CloseProject(proj)
    if pm.DeleteProject(name):
        print("Deleted Resolve project %s" % name)
    else:
        print("SKIP the Resolve project %s was left behind; delete it in Resolve's project manager" % name)
except Exception as e:
    print("SKIP could not delete the Resolve project %s: %s" % (name, e))
`
