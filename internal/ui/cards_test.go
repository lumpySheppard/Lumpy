package ui

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/lumpysheppard/lumpy/internal/catalog"
	"github.com/lumpysheppard/lumpy/internal/job"
)

func TestCardMatches(t *testing.T) {
	for _, c := range []struct {
		pat, name string
		want      bool
	}{{"", "A004", true}, {"*", "B001", true}, {"A*", "A004", true}, {"A*", "B001", false}, {"a*, b*", "B001", true}, {"CARD", "card", true}, {"X?", "X1", true}} {
		if got := cardMatches(c.pat, c.name); got != c.want {
			t.Errorf("cardMatches(%q, %q) = %v", c.pat, c.name, got)
		}
	}
}

// A watched project offers a card as it appears; saying yes offloads it
// to the rule's folders; a rule set to run by itself needs no yes.
func TestCardRulesOfferAndAuto(t *testing.T) {
	s, ts, vols := setup(t)
	os.Mkdir(filepath.Join(vols, "RAID"), 0o755)
	os.Mkdir(filepath.Join(vols, "SHUTTLE"), 0o755)
	job.AgentRunWith(context.Background(), s.db, job.NewReportTo(io.Discard), func(string, string) {}, nil)
	var st struct {
		Drives  []DriveView
		Volumes []VolumeView
		Offers  []cardOffer
	}
	stateSettled(t, s, ts, &st)
	ids := map[string]string{}
	for _, d := range st.Drives {
		ids[d.Name] = d.ID
	}
	var p catalog.Project
	call(t, s, ts, "POST", "/api/projects", `{"name":"Feature","target":2}`, &p)
	body := `{"cards":{"watch":true,"auto":false,"match":"A*","dests":[{"driveId":"` + ids["RAID"] + `","folder":"01_RAW"},{"driveId":"` + ids["SHUTTLE"] + `","folder":"Backup/{year}"}]}}`
	call(t, s, ts, "PATCH", "/api/projects/"+strconv.FormatInt(p.ID, 10), body, nil)
	// A card appears; the agent pass leaves it alone, the watcher offers it.
	os.MkdirAll(filepath.Join(vols, "A004", "CLIPS"), 0o755)
	os.WriteFile(filepath.Join(vols, "A004", "CLIPS", "C001.mov"), []byte("footage"), 0o644)
	s.mu.Lock()
	s.volsAt = s.volsAt.Add(-1e12)
	s.mu.Unlock()
	s.watchCards()
	call(t, s, ts, "GET", "/api/state", "", &st)
	if len(st.Offers) != 1 || st.Offers[0].Card != "A004" || len(st.Offers[0].Dests) != 2 || len(st.Offers[0].Missing) != 0 {
		t.Fatalf("offers: %+v", st.Offers)
	}
	if !filepath.IsAbs(st.Offers[0].Dests[0]) || filepath.Base(st.Offers[0].Dests[0]) != "01_RAW" {
		t.Fatalf("dest: %q", st.Offers[0].Dests[0])
	}
	// A card the pattern does not name is not offered.
	s.watchCards()
	call(t, s, ts, "GET", "/api/state", "", &st)
	if len(st.Offers) != 1 {
		t.Fatalf("offered twice: %+v", st.Offers)
	}
	if code := call(t, s, ts, "POST", "/api/offers/"+st.Offers[0].ID+"/go", "", nil); code != 200 {
		t.Fatalf("go: %d", code)
	}
	jobs := waitJobs(t, s, ts)
	if _, err := os.Stat(filepath.Join(vols, "RAID", "01_RAW", "A004", "CLIPS", "C001.mov")); err != nil {
		for _, j := range jobs {
			t.Logf("job %s %s: %q %v", j.Title, j.State, j.Error, j.Lines)
		}
		t.Fatalf("not offloaded to the RAID: %v", err)
	}
	if _, err := os.Stat(filepath.Join(vols, "SHUTTLE", "Backup", time.Now().Format("2006"), "A004", "CLIPS", "C001.mov")); err != nil {
		t.Fatalf("not offloaded to the shuttle: %v", err)
	}
	call(t, s, ts, "GET", "/api/state", "", &st)
	if len(st.Offers) != 0 {
		t.Fatalf("offer not cleared: %+v", st.Offers)
	}
	// Auto: a second card goes by itself.
	call(t, s, ts, "PATCH", "/api/projects/"+strconv.FormatInt(p.ID, 10), `{"cards":{"watch":true,"auto":true,"match":"","dests":[{"driveId":"`+ids["RAID"]+`","folder":"01_RAW"}]}}`, nil)
	os.MkdirAll(filepath.Join(vols, "B001", "DCIM"), 0o755)
	os.WriteFile(filepath.Join(vols, "B001", "DCIM", "IMG_0001.MOV"), []byte("more"), 0o644)
	s.mu.Lock()
	s.volsAt = s.volsAt.Add(-1e12)
	s.mu.Unlock()
	s.watchCards()
	waitJobs(t, s, ts)
	if _, err := os.Stat(filepath.Join(vols, "RAID", "01_RAW", "B001", "DCIM", "IMG_0001.MOV")); err != nil {
		t.Fatalf("auto offload: %v", err)
	}
	call(t, s, ts, "GET", "/api/state", "", &st)
	if len(st.Offers) != 0 {
		t.Fatalf("auto card left an offer: %+v", st.Offers)
	}
}
