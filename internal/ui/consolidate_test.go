package ui

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/lumpysheppard/lumpy/internal/catalog"
	"github.com/lumpysheppard/lumpy/internal/job"
)

// A feature that grew across drives: DAY 1 mirrored on two drives,
// DAY 2 whole on one drive and half on another under a different
// name, DAY 3 only on one drive. The plan names the sets by content,
// keeps each day's primary where most of it is, pairs each primary
// drive with one mirror, lists what to copy and what to clear after.
func TestConsolidatePlan(t *testing.T) {
	s, ts, vols := setup(t)
	w := func(drive, rel, body string) {
		p := filepath.Join(vols, drive, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(body), 0o644)
	}
	// MAIN: DAY 1, DAY 2, DAY 3. MIRROR: DAY 1 whole. SCRAPS: half of DAY 2 as "Day Two".
	w("MAIN", "FILM/DAY 1/A001.mov", strings.Repeat("a", 100))
	w("MAIN", "FILM/DAY 1/A002.mov", strings.Repeat("b", 100))
	w("MAIN", "FILM/DAY 2/B001.mov", strings.Repeat("c", 100))
	w("MAIN", "FILM/DAY 2/B002.mov", strings.Repeat("d", 100))
	w("MAIN", "FILM/DAY 3/C001.mov", strings.Repeat("e", 100))
	w("MIRROR", "FILM/DAY 1/A001.mov", strings.Repeat("a", 100))
	w("MIRROR", "FILM/DAY 1/A002.mov", strings.Repeat("b", 100))
	w("SCRAPS", "Day Two/B001.mov", strings.Repeat("c", 100))
	os.MkdirAll(filepath.Join(vols, "SPARE"), 0o755)
	job.AgentRunWith(context.Background(), s.db, job.NewReportTo(io.Discard), func(string, string) {}, nil)
	var st struct{ Drives []DriveView }
	stateSettled(t, s, ts, &st)
	id := map[string]string{}
	for _, d := range st.Drives {
		id[d.Name] = d.ID
	}
	var p catalog.Project
	call(t, s, ts, "POST", "/api/projects", `{"name":"Feature","target":2,"sources":[{"driveId":"`+id["MAIN"]+`","folder":"FILM"},{"driveId":"`+id["MIRROR"]+`","folder":"FILM"},{"driveId":"`+id["SCRAPS"]+`","folder":""}]}`, &p)
	var plan conPlan
	if code := call(t, s, ts, "GET", "/api/projects/"+strconv.FormatInt(p.ID, 10)+"/consolidate", "", &plan); code != 200 {
		t.Fatalf("plan: %d", code)
	}
	if len(plan.Sets) != 3 {
		t.Fatalf("sets: %+v", plan.Sets)
	}
	byName := map[string]*conSet{}
	for _, x := range plan.Sets {
		byName[x.Name] = x
	}
	d1, d2, d3 := byName["DAY 1"], byName["DAY 2"], byName["DAY 3"]
	if d1 == nil || d2 == nil || d3 == nil {
		t.Fatalf("set names: %+v", plan.Sets)
	}
	if d1.Status != "backed" || d2.Status != "partial" || d3.Status != "sole" {
		t.Fatalf("status: %s %s %s", d1.Status, d2.Status, d3.Status)
	}
	if len(d2.Aliases) != 1 || d2.Aliases[0] != "Day Two" {
		t.Fatalf("DAY 2 aliases: %v", d2.Aliases)
	}
	for _, x := range plan.Sets {
		if x.Primary != id["MAIN"] {
			t.Errorf("%s primary %s, want MAIN", x.Name, x.Primary)
		}
	}
	// One pair: MAIN mirrored on MIRROR (it already holds DAY 1 whole),
	// DAY 1 through DAY 3; DAY 2 and DAY 3 copy there; SCRAPS' half of
	// DAY 2 is to clear afterwards.
	if !plan.Proposed || len(plan.Pairs) != 1 || plan.Pairs[0].Primary != id["MAIN"] || plan.Pairs[0].Mirror != id["MIRROR"] || plan.Pairs[0].First != "DAY 1" || plan.Pairs[0].Last != "DAY 3" {
		t.Fatalf("pairs: %+v (MAIN=%s MIRROR=%s)", plan.Pairs, id["MAIN"], id["MIRROR"])
	}
	if d1.First == 0 || d1.Last < d1.First {
		t.Fatalf("dates: %d %d", d1.First, d1.Last)
	}
	if d1.CopyB != 0 || d2.CopyB != 200 || d3.CopyB != 100 {
		t.Fatalf("copy bytes: %d %d %d", d1.CopyB, d2.CopyB, d3.CopyB)
	}
	if len(d2.Clears) != 1 || d2.Clears[0].DriveID != id["SCRAPS"] || d2.Clears[0].Bytes != 100 {
		t.Fatalf("DAY 2 clears: %+v", d2.Clears)
	}
	if len(plan.Transfers) != 2 || plan.Transfers[0].Set != "DAY 2" || plan.Transfers[0].Files != 2 || plan.Transfers[0].From != id["MAIN"] || plan.Transfers[0].To != id["MIRROR"] {
		t.Fatalf("transfers: %+v", plan.Transfers)
	}
	if plan.Sole != 100 || plan.Partial != 200 || plan.Buy != 0 {
		t.Fatalf("totals: sole %d partial %d buy %d", plan.Sole, plan.Partial, plan.Buy)
	}
	var mirror *conDrive
	for _, d := range plan.Drives {
		if d.ID == id["MIRROR"] {
			mirror = d
		}
	}
	if mirror == nil || len(mirror.MirrorOf) != 3 || mirror.CopyIn != 300 {
		t.Fatalf("mirror drive: %+v", mirror)
	}
	// The user's own pairs: DAY 1 and DAY 2 on MAIN mirrored on SCRAPS,
	// the rest on MIRROR mirrored on SPARE. DAY 3 then copies onto
	// MIRROR from MAIN and onto SPARE too.
	body := `{"pairs":[{"primary":"` + id["MAIN"] + `","mirror":"` + id["SCRAPS"] + `","through":"DAY 2"},{"primary":"` + id["MIRROR"] + `","mirror":"` + id["SPARE"] + `"}]}`
	if code := call(t, s, ts, "POST", "/api/projects/"+strconv.FormatInt(p.ID, 10)+"/consolidate", body, &plan); code != 200 {
		t.Fatalf("plan with pairs: %d", code)
	}
	if plan.Proposed || len(plan.Pairs) != 2 || plan.Pairs[0].Last != "DAY 2" || plan.Pairs[1].First != "DAY 3" || plan.Pairs[1].Last != "DAY 3" {
		t.Fatalf("user pairs: %+v", plan.Pairs)
	}
	byName = map[string]*conSet{}
	for _, x := range plan.Sets {
		byName[x.Name] = x
	}
	if byName["DAY 3"].Primary != id["MIRROR"] || byName["DAY 3"].Mirror != id["SPARE"] || byName["DAY 3"].CopyB != 200 {
		t.Fatalf("DAY 3 under user pairs: %+v", byName["DAY 3"])
	}
	// DAY 1 on MIRROR is now a redundant copy.
	if len(byName["DAY 1"].Clears) != 1 || byName["DAY 1"].Clears[0].DriveID != id["MIRROR"] {
		t.Fatalf("DAY 1 clears: %+v", byName["DAY 1"].Clears)
	}
	// Renaming a day's folder renames it on every drive that has it.
	if code := call(t, s, ts, "POST", "/api/projects/"+strconv.FormatInt(p.ID, 10)+"/consolidate/rename", `{"from":"DAY 1","to":"reel01"}`, nil); code != 200 {
		t.Fatalf("rename: %d", code)
	}
	for _, d := range []string{"MAIN", "MIRROR"} {
		if _, err := os.Stat(filepath.Join(vols, d, "FILM", "reel01", "A001.mov")); err != nil {
			t.Errorf("%s not renamed: %v", d, err)
		}
	}
	waitJobs(t, s, ts)
	call(t, s, ts, "GET", "/api/projects/"+strconv.FormatInt(p.ID, 10)+"/consolidate", "", &plan)
	if plan.Sets[0].Name != "DAY 2" && plan.Sets[0].Name != "reel01" {
		t.Fatalf("after rename: %+v", plan.Sets[0])
	}
	found := false
	for _, x := range plan.Sets {
		if x.Name == "reel01" && len(x.Aliases) == 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("reel01 not one set after the rename: %+v", plan.Sets)
	}
}

func TestNatLess(t *testing.T) {
	for _, c := range [][2]string{{"DAY 2", "DAY 10"}, {"Day 9", "DAY 10-14"}, {"A001", "A002"}, {"abc", "abd"}} {
		if !natLess(c[0], c[1]) || natLess(c[1], c[0]) {
			t.Errorf("%q should sort before %q", c[0], c[1])
		}
	}
}
