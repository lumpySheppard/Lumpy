package ui

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lumpysheppard/lumpy/internal/agent"
	"github.com/lumpysheppard/lumpy/internal/catalog"
	"github.com/lumpysheppard/lumpy/internal/volume"
)

// A card offer is Lumpy noticing a card and knowing, from a project's
// card rules, where it should go. It waits for a yes in the app, or
// runs by itself when the rule says so.
type cardOffer struct {
	ID        string   `json:"id"`
	VolumeID  string   `json:"volumeId"`
	Card      string   `json:"card"`
	Mount     string   `json:"mount"`
	Bytes     int64    `json:"bytes"`
	ProjectID int64    `json:"projectId"`
	Project   string   `json:"project"`
	Color     string   `json:"color"`
	Dests     []string `json:"dests"`   // resolved folders on plugged-in drives
	Missing   []string `json:"missing"` // destination drives not plugged in
	Auto      bool     `json:"auto"`
	At        int64    `json:"at"`
}

// watchCards runs after every agent pass: a capture volume that has
// just appeared is matched against every project that watches for
// cards. One match with every destination drive plugged in and the
// rule set to run by itself starts the offload; anything else becomes
// an offer for the app to show.
func (s *Server) watchCards() {
	projects, err := s.db.Projects()
	if err != nil {
		return
	}
	var watching []*catalog.Project
	for _, p := range projects {
		if p.Cards != nil && p.Cards.Watch && len(p.Cards.Dests) > 0 {
			watching = append(watching, p)
		}
	}
	s.cardMu.Lock()
	defer s.cardMu.Unlock()
	if s.cardSeen == nil {
		s.cardSeen = map[string]string{}
	}
	if s.offers == nil {
		s.offers = map[string]*cardOffer{}
	}
	now := map[string]bool{}
	for _, v := range s.mounted() {
		if !v.Capture {
			continue
		}
		now[v.ID] = true
		if s.cardSeen[v.ID] == v.Mount {
			continue // already offered at this mount
		}
		s.cardSeen[v.ID] = v.Mount
		for _, p := range watching {
			if !cardMatches(p.Cards.Match, v.Name) {
				continue
			}
			o := s.makeOffer(v, p)
			if o.Auto && len(o.Missing) == 0 && len(o.Dests) > 0 {
				makeDests(o.Dests)
				s.startOffload(o.Mount, o.Dests, "", false)
				agent.Notify("Offloading "+v.Name, fmt.Sprintf("To %s, as %s's card rule says.", strings.Join(destNames(o.Dests), " and "), p.Name))
				continue
			}
			s.offers[o.ID] = o
			if len(o.Missing) > 0 {
				agent.Notify(v.Name+" is in", fmt.Sprintf("%s wants it offloaded, but %s is not plugged in.", p.Name, strings.Join(o.Missing, " and ")))
			} else {
				agent.Notify(v.Name+" is in", fmt.Sprintf("Offload it to %s for %s? Open Lumpy to say yes.", strings.Join(destNames(o.Dests), " and "), p.Name))
			}
		}
	}
	// Offers for cards that have gone are dropped, and a card that comes
	// back is offered again.
	for id, o := range s.offers {
		if !now[o.VolumeID] {
			delete(s.offers, id)
		}
	}
	for id := range s.cardSeen {
		if !now[id] {
			delete(s.cardSeen, id)
		}
	}
}

func (s *Server) makeOffer(v *volume.Volume, p *catalog.Project) *cardOffer {
	o := &cardOffer{ID: fmt.Sprintf("%s|%d", v.ID, p.ID), VolumeID: v.ID, Card: v.Name, Mount: v.Mount, Bytes: v.Size - v.Free,
		ProjectID: p.ID, Project: p.Name, Color: p.Color, Dests: []string{}, Missing: []string{}, Auto: p.Cards.Auto, At: time.Now().Unix()}
	for _, d := range p.Cards.Dests {
		m := s.mountOf(d.DriveID)
		if m == nil {
			name := d.DriveID
			if dr, err := s.db.Drive(d.DriveID); err == nil {
				name = dr.Title()
			}
			o.Missing = append(o.Missing, name)
			continue
		}
		o.Dests = append(o.Dests, filepath.Join(m.Mount, filepath.FromSlash(fillCard(d.Folder, v.Name))))
	}
	return o
}

// fillCard fills a destination folder pattern: {date} is today as
// 2006-01-02, {day} is Day_MMDD, {year} the year, {card} the card's
// name. The card's own folder is created inside the result.
func fillCard(pattern, card string) string {
	t := time.Now()
	r := strings.NewReplacer("{card}", card, "{date}", t.Format("2006-01-02"), "{day}", t.Format("Day_0102"), "{year}", t.Format("2006"))
	out := strings.Trim(r.Replace(pattern), "/")
	return out
}

// cardMatches is a glob on the card's name, or everything when empty.
func cardMatches(pattern, name string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || pattern == "*" {
		return true
	}
	for _, alt := range strings.Split(pattern, ",") {
		alt = strings.TrimSpace(alt)
		if alt == "" {
			continue
		}
		if ok, _ := path.Match(strings.ToUpper(alt), strings.ToUpper(name)); ok {
			return true
		}
		if strings.EqualFold(alt, name) {
			return true
		}
	}
	return false
}

func destNames(dests []string) []string {
	var out []string
	for _, d := range dests {
		if v, err := volume.Of(d); err == nil {
			out = append(out, v.Name)
		} else {
			out = append(out, filepath.Base(d))
		}
	}
	return out
}

func (s *Server) offerList() []*cardOffer {
	s.cardMu.Lock()
	defer s.cardMu.Unlock()
	out := []*cardOffer{}
	for _, o := range s.offers {
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At < out[j].At })
	return out
}

// offerGo is POST /api/offers/{id}/go: run the offer's offload now.
func (s *Server) offerGo(w http.ResponseWriter, r *http.Request) (any, error) {
	s.cardMu.Lock()
	o := s.offers[r.PathValue("id")]
	if o != nil {
		delete(s.offers, o.ID)
	}
	s.cardMu.Unlock()
	if o == nil {
		return nil, httpError{http.StatusNotFound, "that offer is gone"}
	}
	if s.mountOf(o.VolumeID) == nil {
		return nil, badRequest("%s is no longer plugged in", o.Card)
	}
	if len(o.Dests) == 0 {
		return nil, badRequest("none of %s's destination drives is plugged in", o.Project)
	}
	makeDests(o.Dests)
	j := s.startOffload(o.Mount, o.Dests, "", false)
	return map[string]any{"job": j.ID}, nil
}

// makeDests creates the folders a card is offloaded into; the card's
// own folder is made inside each by the offload.
func makeDests(dests []string) {
	for _, d := range dests {
		os.MkdirAll(d, 0o755)
	}
}

// offerDrop is DELETE /api/offers/{id}: not this time.
func (s *Server) offerDrop(w http.ResponseWriter, r *http.Request) (any, error) {
	s.cardMu.Lock()
	delete(s.offers, r.PathValue("id"))
	s.cardMu.Unlock()
	return map[string]any{"ok": true}, nil
}
