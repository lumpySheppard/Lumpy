package space

import (
	"path/filepath"
	"strings"
)

// Suspect is a place on a Mac that fills up with things nobody needs:
// caches apps rebuild, renders that can be made again, backups of
// phones long replaced. Each says how safe it is to empty.
type Suspect struct {
	Name string `json:"name"`
	Rel  string `json:"rel"` // relative to the home folder
	Note string `json:"note"`
	// Risk: "safe" (an app rebuilds it), "check" (look first), "keep"
	// (shown for size, not for emptying).
	Risk  string `json:"risk"`
	Bytes int64  `json:"bytes"`
	Files int64  `json:"files"`
}

// Suspects are the usual places, under the home folder.
var Suspects = []Suspect{
	{Name: "Trash", Rel: ".Trash", Note: "Empty it from the Finder; everything here is already thrown away.", Risk: "safe"},
	{Name: "Downloads", Rel: "Downloads", Note: "Installers and things opened once. Look through it.", Risk: "check"},
	{Name: "App caches", Rel: "Library/Caches", Note: "Every app's scratch space; they rebuild what they need. Quit apps first.", Risk: "safe"},
	{Name: "Logs", Rel: "Library/Logs", Note: "Diagnostic logs. Safe to clear.", Risk: "safe"},
	{Name: "Resolve cache", Rel: "Movies/CacheClip", Note: "DaVinci Resolve's render cache and optimised media; Resolve makes them again.", Risk: "safe"},
	{Name: "Resolve gallery stills", Rel: "Movies/.gallery", Note: "Grabbed stills and their PowerGrades. Keep if you use them.", Risk: "check"},
	{Name: "Adobe media cache", Rel: "Library/Application Support/Adobe/Common/Media Cache Files", Note: "Premiere and After Effects peak and conform files; rebuilt on open.", Risk: "safe"},
	{Name: "Adobe media cache database", Rel: "Library/Application Support/Adobe/Common/Media Cache", Note: "Goes with the media cache files.", Risk: "safe"},
	{Name: "Xcode derived data", Rel: "Library/Developer/Xcode/DerivedData", Note: "Build products; Xcode rebuilds them.", Risk: "safe"},
	{Name: "Xcode archives", Rel: "Library/Developer/Xcode/Archives", Note: "Old app archives. Keep the ones you shipped.", Risk: "check"},
	{Name: "iOS device support", Rel: "Library/Developer/Xcode/iOS DeviceSupport", Note: "Symbols for every iOS version ever plugged in. Xcode fetches them again.", Risk: "safe"},
	{Name: "Simulators", Rel: "Library/Developer/CoreSimulator", Note: "iPhone simulator images and their data.", Risk: "check"},
	{Name: "iPhone and iPad backups", Rel: "Library/Application Support/MobileSync/Backup", Note: "Finder backups of devices, often of phones you no longer have. Look first.", Risk: "check"},
	{Name: "Mail downloads", Rel: "Library/Containers/com.apple.mail/Data/Library/Mail Downloads", Note: "Attachments Mail opened. Safe to clear.", Risk: "safe"},
	{Name: "Docker disk image", Rel: "Library/Containers/com.docker.docker/Data/vms", Note: "Everything Docker has. Prune from Docker instead.", Risk: "check"},
	{Name: "Spotify cache", Rel: "Library/Application Support/Spotify/PersistentCache", Note: "Streamed songs. Safe to clear.", Risk: "safe"},
	{Name: "Slack cache", Rel: "Library/Application Support/Slack/Cache", Note: "Safe to clear.", Risk: "safe"},
	{Name: "Chrome cache", Rel: "Library/Caches/Google/Chrome", Note: "Safe to clear.", Risk: "safe"},
	{Name: "npm cache", Rel: ".npm/_cacache", Note: "Downloaded packages; npm fetches them again.", Risk: "safe"},
	{Name: "Yarn and pnpm store", Rel: "Library/pnpm", Note: "Downloaded packages; fetched again.", Risk: "safe"},
	{Name: "Go module cache", Rel: "go/pkg/mod", Note: "Downloaded Go modules; fetched again.", Risk: "safe"},
	{Name: "Cargo registry", Rel: ".cargo/registry", Note: "Downloaded Rust crates; fetched again.", Risk: "safe"},
	{Name: "Homebrew downloads", Rel: "Library/Caches/Homebrew", Note: "Downloaded bottles; brew fetches again.", Risk: "safe"},
	{Name: "Pip cache", Rel: "Library/Caches/pip", Note: "Downloaded Python packages.", Risk: "safe"},
	{Name: "Photos library", Rel: "Pictures/Photos Library.photoslibrary", Note: "Your photos. Shown for size only; manage it in Photos.", Risk: "keep"},
	{Name: "Music library", Rel: "Music", Note: "Shown for size only.", Risk: "keep"},
	{Name: "Final Cut libraries", Rel: "Movies", Note: "FCP libraries and render files live in .fcpbundle packages here; delete render files from inside Final Cut.", Risk: "keep"},
}

// Anywhere are folders known by name wherever they sit: what DaVinci
// Resolve makes beside a project, on a Mac or inside Resolve for iPad's
// own folder.
var Anywhere = []Suspect{
	{Name: "Resolve render cache", Rel: "CacheClip", Note: "DaVinci Resolve's render cache; Resolve makes it again (Playback › Delete Render Cache).", Risk: "safe"},
	{Name: "Resolve optimised media", Rel: "OptimizedMedia", Note: "Resolve's optimised media; made again on demand (Playback › Delete Optimized Media).", Risk: "safe"},
	{Name: "Resolve proxies", Rel: "ProxyMedia", Note: "Proxies Resolve generated; made again from the originals.", Risk: "check"},
	{Name: "Resolve gallery stills", Rel: ".gallery", Note: "Grabbed stills and their PowerGrades. Keep if you use them.", Risk: "check"},
	{Name: "Resolve project backups", Rel: "Project Backups", Note: "Timed backups of every project; the recent ones are worth keeping.", Risk: "check"},
}

// FindSuspects sizes the usual places from a measurement whose root
// is the home folder or holds it (the whole disk), and the folders
// known by name wherever they sit. Places with nothing in them are
// left out.
func FindSuspects(tree *Node, root, home string) []Suspect {
	var out []Suspect
	if rel, err := filepath.Rel(root, home); err == nil && !strings.HasPrefix(rel, "..") {
		for _, s := range Suspects {
			p := filepath.ToSlash(filepath.Join(rel, s.Rel))
			if rel == "." {
				p = s.Rel
			}
			n := tree.At(p)
			if n == nil || n.Bytes == 0 {
				continue
			}
			s.Rel = p
			s.Bytes, s.Files = n.Bytes, n.Files
			out = append(out, s)
		}
	}
	var walk func(n *Node, p string, depth int)
	walk = func(n *Node, p string, depth int) {
		for _, c := range n.Children {
			if !c.Dir {
				continue
			}
			cp := c.Name
			if p != "" {
				cp = p + "/" + c.Name
			}
			hit := false
			for _, s := range Anywhere {
				if strings.EqualFold(c.Name, s.Rel) && c.Bytes > 0 {
					s.Rel = cp
					s.Bytes, s.Files = c.Bytes, c.Files
					out = append(out, s)
					hit = true
				}
			}
			if !hit && depth < 6 {
				walk(c, cp, depth+1)
			}
		}
	}
	walk(tree, "", 0)
	return out
}
