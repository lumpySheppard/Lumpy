package catalog

import (
	"encoding/json"
	"strings"
)

// Prefs are the library's general preferences, kept in the settings
// table under "prefs" and put into effect by job.ApplyPrefs.
type Prefs struct {
	// HideInternal leaves the Mac's own volumes out; HideImages leaves
	// mounted .dmg and .iso images out.
	HideInternal bool `json:"hideInternal"`
	HideImages   bool `json:"hideImages"`
	// AutoLog logs drives seen for the first time by themselves.
	AutoLog bool `json:"autoLog"`
	// RescanMinutes rescans plugged-in drives on this interval while the
	// app is open; 0 rescans only on mount and by hand.
	RescanMinutes int `json:"rescanMinutes"`
	// AutoMetadata reads clip metadata during scans.
	AutoMetadata bool `json:"autoMetadata"`
	// Exclude is a comma-separated list of names, or prefixes ending in
	// *, left out of library scans at any depth.
	Exclude string `json:"exclude"`
	// Gentle reads one file at a time and holds reads to about 150 MB/s,
	// so the Mac stays responsive while Lumpy works.
	Gentle bool `json:"gentle"`
}

// DefaultPrefs are a fresh library's preferences.
func DefaultPrefs() Prefs {
	return Prefs{HideInternal: true, HideImages: true, AutoLog: true, AutoMetadata: true}
}

// Prefs reads the preferences.
func (db *DB) Prefs() Prefs {
	p := DefaultPrefs()
	if v := db.Setting("prefs", ""); v != "" {
		json.Unmarshal([]byte(v), &p)
	}
	return p
}

// SavePrefs stores the preferences.
func (db *DB) SavePrefs(p Prefs) error {
	if p.RescanMinutes < 0 {
		p.RescanMinutes = 0
	}
	p.Exclude = strings.Join(p.ExcludeList(), ", ")
	b, _ := json.Marshal(p)
	return db.SetSetting("prefs", string(b))
}

// ExcludeList is Exclude split into names, trimmed, without empties.
func (p Prefs) ExcludeList() []string {
	var out []string
	for _, n := range strings.Split(p.Exclude, ",") {
		n = strings.TrimSpace(n)
		if n != "" && n != "*" && !strings.ContainsAny(n, "/\\") {
			out = append(out, n)
		}
	}
	return out
}
