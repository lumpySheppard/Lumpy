package space

import (
	"path"
	"strings"
)

// Verdict says how safe it is to throw something away, from where it
// sits and what it is. "safe": an app makes it again. "check": your
// own things, or something to look at first. "critical": the system,
// apps, or data an app keeps for you; throwing it away breaks things.
type Verdict struct {
	Level string `json:"level"` // safe, check, critical, or "" for nothing to say
	Why   string `json:"why"`
}

// Judge looks at a path relative to the measured root, with the home
// folder's relative path (or "." when home is the root, "" when home
// is elsewhere).
func Judge(rel, home string, dir bool) Verdict {
	rel = strings.Trim(rel, "/")
	base := path.Base(rel)
	lower := strings.ToLower(rel)
	// Inside the home folder, judge by the folder under it.
	inHome := rel
	if home != "" && home != "." {
		if !strings.HasPrefix(rel, home+"/") {
			inHome = ""
		} else {
			inHome = strings.TrimPrefix(rel, home+"/")
		}
	}
	if home == "" {
		inHome = ""
	}
	top := strings.Split(rel, "/")[0]
	// The system and apps: never from here.
	if home != "." {
		switch top {
		case "System", "Library", "usr", "bin", "sbin", "private", "etc", "var", "cores", "opt", "Volumes", "dev":
			if top == "Library" && strings.HasPrefix(lower, "library/caches") {
				return Verdict{"safe", "system-wide caches; rebuilt as needed"}
			}
			return Verdict{"critical", "part of macOS"}
		case "Applications":
			if strings.HasSuffix(base, ".app") {
				return Verdict{"check", "an app; delete it from Launchpad or the Finder if you don't use it"}
			}
			return Verdict{"critical", "where apps live"}
		}
	}
	ext := strings.ToLower(path.Ext(base))
	switch ext {
	case ".app":
		return Verdict{"check", "an app; delete it from the Finder if you don't use it"}
	case ".photoslibrary":
		return Verdict{"critical", "your Photos library"}
	case ".fcpbundle":
		return Verdict{"check", "a Final Cut Pro library; delete render files from inside Final Cut instead"}
	case ".musiclibrary", ".tvlibrary":
		return Verdict{"critical", "an Apple media library"}
	case ".log", ".tmp", ".cache", ".dmg", ".pkg", ".zip", ".iso":
		if ext == ".log" || ext == ".tmp" || ext == ".cache" {
			return Verdict{"safe", "a scratch file"}
		}
		return Verdict{"check", "an installer or archive; usually done with once installed"}
	}
	if inHome != "" {
		h := strings.ToLower(inHome)
		switch {
		case h == "library", h == "library/application support", h == "library/preferences", h == "library/containers", h == "library/group containers":
			return Verdict{"critical", "where apps keep your data and settings"}
		case strings.HasPrefix(h, "library/keychains"), strings.HasPrefix(h, "library/mail/") && !strings.Contains(h, "mail downloads"), strings.HasPrefix(h, "library/messages"), strings.HasPrefix(h, "library/mobile documents"), strings.HasPrefix(h, "library/preferences"), strings.HasPrefix(h, "library/accounts"), strings.HasPrefix(h, "library/calendars"), strings.HasPrefix(h, "library/safari"), strings.HasPrefix(h, "library/cookies"):
			return Verdict{"critical", "passwords, mail, messages, iCloud or settings"}
		case strings.HasPrefix(h, "library/caches"), strings.HasPrefix(h, "library/logs"), strings.Contains(h, "/caches/"), strings.Contains(h, "/cache/"), strings.HasSuffix(h, "/cache"):
			return Verdict{"safe", "a cache; the app makes it again"}
		case strings.HasPrefix(h, "library/developer/xcode/deriveddata"), strings.HasPrefix(h, "library/developer/xcode/ios devicesupport"), strings.HasPrefix(h, "movies/cacheclip"), strings.Contains(h, "media cache files"):
			return Verdict{"safe", "build products or render cache; made again"}
		case strings.HasPrefix(h, "library/application support/mobilesync"):
			return Verdict{"check", "iPhone and iPad backups"}
		case strings.HasPrefix(h, "library/application support/"), strings.HasPrefix(h, "library/containers/"), strings.HasPrefix(h, "library/group containers/"):
			if strings.Contains(h, "cache") {
				return Verdict{"safe", "an app's cache"}
			}
			return Verdict{"critical", "an app's data"}
		case strings.HasPrefix(h, "library/"):
			return Verdict{"check", "inside Library; look at what it is first"}
		case h == ".trash" || strings.HasPrefix(h, ".trash/"):
			return Verdict{"safe", "already in the Trash"}
		case strings.HasPrefix(h, "downloads"):
			return Verdict{"check", "downloads; often done with"}
		case strings.HasPrefix(h, "documents"), strings.HasPrefix(h, "desktop"), strings.HasPrefix(h, "pictures"), strings.HasPrefix(h, "movies"), strings.HasPrefix(h, "music"):
			return Verdict{"check", "your own files"}
		case strings.HasPrefix(h, "applications"):
			return Verdict{"check", "apps installed for you"}
		case base == "node_modules", strings.HasPrefix(h, ".npm"), strings.HasPrefix(h, ".cache"), strings.HasPrefix(h, "go/pkg/mod"), strings.HasPrefix(h, ".cargo/registry"), strings.HasPrefix(h, ".gradle/caches"), strings.HasPrefix(h, ".m2/repository"), strings.HasPrefix(h, ".rustup/toolchains"):
			return Verdict{"safe", "downloaded packages or build tools; fetched again"}
		case strings.HasPrefix(h, "."):
			return Verdict{"check", "a hidden settings folder; usually small, look first"}
		}
	}
	if base == "node_modules" || strings.Contains(lower, "/cache") || strings.HasSuffix(lower, "cache") {
		return Verdict{"safe", "a cache or downloaded packages; made again"}
	}
	if dir {
		return Verdict{"", ""}
	}
	return Verdict{"", ""}
}
