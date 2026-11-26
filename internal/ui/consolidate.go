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
	"strconv"
	"strings"

	"github.com/lumpysheppard/lumpy/internal/catalog"
	"github.com/lumpysheppard/lumpy/internal/job"
)

// Consolidate: a project that grew across many drives before there
// was a standard (a day here, a mirror there, a third copy of half of
// it somewhere else) is planned into a clean shape: the shooting days
// in order, cut into consecutive ranges, each range on one primary
// drive and one mirror that holds only what its primary holds
// (Drive1 and Drive1b hold reel01 to reel18, Drive2 and Drive2b the
// next run), with the redundant copies listed to clear once the two
// verified copies exist. Days are matched by content, so a day is one
// row however its folders are named on each drive, and a day's folder
// can be renamed on every drive at once to put the names in order.
// Nothing here moves a byte; it is the plan to look at.

// A conSet is one logical shooting day or card: the folders on the
// drives that hold it, matched by content, however they are named.
type conSet struct {
	Name    string             `json:"name"`
	Aliases []string           `json:"aliases"` // other names the same content goes by
	Bytes   int64              `json:"bytes"`
	Files   int                `json:"files"`
	First   int64              `json:"first"`  // earliest file, unix seconds, 0 if unknown
	Last    int64              `json:"last"`   // latest file
	Copies  map[string]float64 `json:"copies"` // drive id -> fraction of the set's bytes there
	Status  string             `json:"status"` // sole, partial, backed
	Pair    int                `json:"pair"`   // index into the plan's pairs, -1 when left out
	Primary string             `json:"primary"`
	Mirror  string             `json:"mirror"`
	CopyB   int64              `json:"copyBytes"` // still to copy onto the primary and mirror
	Clears  []conClear         `json:"clears"`
}

type conClear struct {
	DriveID string `json:"driveId"`
	Bytes   int64  `json:"bytes"`
}

type conDrive struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Size      int64    `json:"size"`
	Free      int64    `json:"free"`
	Connected bool     `json:"connected"`
	NowBytes  int64    `json:"nowBytes"` // of this project
	PrimaryOf []string `json:"primaryOf"`
	MirrorOf  []string `json:"mirrorOf"`
	CopyIn    int64    `json:"copyIn"`
	ClearOut  int64    `json:"clearOut"`
	FreeAfter int64    `json:"freeAfter"`
}

type conTransfer struct {
	Set   string `json:"set"`
	From  string `json:"from"`
	To    string `json:"to"`
	Bytes int64  `json:"bytes"`
	Files int    `json:"files"`
}

// A conPair is one primary drive with its mirror and the last day of
// its range; the pairs take the days in order, the last pair the rest.
type conPair struct {
	Primary string `json:"primary"`
	Mirror  string `json:"mirror"`
	Through string `json:"through"`
	First   string `json:"first,omitempty"` // filled in: the range's first and last day
	Last    string `json:"last,omitempty"`
	Bytes   int64  `json:"bytes,omitempty"`
}

type conLayout struct {
	Pairs []conPair `json:"pairs"`
}

type conPlan struct {
	Sets       []*conSet     `json:"sets"`
	Drives     []*conDrive   `json:"drives"`
	Pairs      []conPair     `json:"pairs"`
	Transfers  []conTransfer `json:"transfers"`
	Buy        int64         `json:"buy"` // bytes no drive has room to mirror
	Sole       int64         `json:"soleBytes"`
	Partial    int64         `json:"partialBytes"`
	Offline    []string      `json:"offline"`    // drives in the project that are not logged
	Unassigned []string      `json:"unassigned"` // days after the last pair's range
	Proposed   bool          `json:"proposed"`   // the pairs are Lumpy's, not the user's
}

// chunk is one top-level folder of the project on one drive.
type chunk struct {
	drive string
	name  string
	keys  map[string]int64 // content key -> size
	bytes int64
	first int64
	last  int64
}

func fileKey(f catalog.File) string {
	if f.XXH64 != "" {
		return "h:" + f.XXH64
	}
	return "n:" + path.Base(f.Path) + ":" + strconv.FormatInt(f.Size, 10)
}

// consolidatePlan is GET /api/projects/{id}/consolidate: the plan with
// Lumpy's own pairs. POST with {pairs: [{primary, mirror, through}]}
// plans with those.
func (s *Server) consolidatePlan(w http.ResponseWriter, r *http.Request) (any, error) {
	p, err := s.projectFrom(r)
	if err != nil {
		return nil, err
	}
	var layout *conLayout
	if r.Method == http.MethodPost {
		layout = &conLayout{}
		if err := json.NewDecoder(r.Body).Decode(layout); err != nil {
			return nil, badRequest("bad request: %v", err)
		}
		if len(layout.Pairs) == 0 {
			layout = nil
		}
	}
	return s.buildConsolidate(p, layout)
}

func (s *Server) buildConsolidate(p *catalog.Project, layout *conLayout) (*conPlan, error) {
	all, err := s.db.Drives()
	if err != nil {
		return nil, err
	}
	drives := map[string]*catalog.Drive{}
	for _, d := range all {
		drives[d.ID] = d
	}
	plan := &conPlan{Sets: []*conSet{}, Drives: []*conDrive{}, Pairs: []conPair{}, Transfers: []conTransfer{}, Offline: []string{}, Unassigned: []string{}}
	// Chunks: each top-level folder of each source, by content.
	var chunks []*chunk
	byDriveKeys := map[string]map[string]int64{} // drive -> every key of the project on it
	seenSrc := map[string]bool{}
	for _, src := range p.Sources {
		k := src.DriveID + "\x00" + src.Folder
		if seenSrc[k] {
			continue
		}
		seenSrc[k] = true
		if drives[src.DriveID] == nil {
			plan.Offline = append(plan.Offline, src.DriveID)
			continue
		}
		files, err := s.db.Files(src.DriveID)
		if err != nil {
			return nil, err
		}
		prefix := src.Folder
		if prefix != "" {
			prefix += "/"
		}
		byName := map[string]*chunk{}
		for _, f := range files {
			if !strings.HasPrefix(f.Path, prefix) || f.BadHash != "" {
				continue
			}
			rel := strings.TrimPrefix(f.Path, prefix)
			if strings.HasPrefix(path.Base(rel), ".") {
				continue
			}
			name := path.Base(src.Folder)
			if i := strings.Index(rel, "/"); i >= 0 {
				name = rel[:i]
			} else if name == "" || name == "." {
				name = drives[src.DriveID].Title()
			}
			c := byName[name]
			if c == nil {
				c = &chunk{drive: src.DriveID, name: name, keys: map[string]int64{}}
				byName[name] = c
				chunks = append(chunks, c)
			}
			key := fileKey(f)
			if _, dup := c.keys[key]; !dup {
				c.keys[key] = f.Size
				c.bytes += f.Size
			}
			if t := f.ModTime / 1e9; t > 0 {
				if c.first == 0 || t < c.first {
					c.first = t
				}
				if t > c.last {
					c.last = t
				}
			}
			if byDriveKeys[src.DriveID] == nil {
				byDriveKeys[src.DriveID] = map[string]int64{}
			}
			byDriveKeys[src.DriveID][key] = f.Size
		}
	}
	// Folders that hold the same content on different drives are one set.
	parent := make([]int, len(chunks))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	overlap := func(a, b *chunk) int64 {
		var n int64
		for k, sz := range a.keys {
			if _, ok := b.keys[k]; ok {
				n += sz
			}
		}
		return n
	}
	for i := range chunks {
		for j := i + 1; j < len(chunks); j++ {
			a, b := chunks[i], chunks[j]
			if a.drive == b.drive || a.bytes == 0 || b.bytes == 0 {
				continue
			}
			small := a.bytes
			if b.bytes < small {
				small = b.bytes
			}
			if overlap(a, b)*2 >= small {
				parent[find(i)] = find(j)
			}
		}
	}
	groups := map[int][]*chunk{}
	for i, c := range chunks {
		groups[find(i)] = append(groups[find(i)], c)
	}
	free := func(id string) int64 {
		if v := s.mountOf(id); v != nil {
			return v.Free
		}
		if d := drives[id]; d != nil {
			return d.Free
		}
		return 0
	}
	var sets []*conSet
	setKeys := map[*conSet]map[string]int64{}
	for _, g := range groups {
		sort.Slice(g, func(i, j int) bool {
			return g[i].bytes > g[j].bytes || (g[i].bytes == g[j].bytes && g[i].name < g[j].name)
		})
		st := &conSet{Name: g[0].name, Aliases: []string{}, Copies: map[string]float64{}, Clears: []conClear{}, Pair: -1}
		keys := map[string]int64{}
		for _, c := range g {
			if c.name != st.Name {
				dup := false
				for _, a := range st.Aliases {
					if a == c.name {
						dup = true
					}
				}
				if !dup {
					st.Aliases = append(st.Aliases, c.name)
				}
			}
			for k, sz := range c.keys {
				keys[k] = sz
			}
			if c.first > 0 && (st.First == 0 || c.first < st.First) {
				st.First = c.first
			}
			if c.last > st.Last {
				st.Last = c.last
			}
		}
		for _, sz := range keys {
			st.Bytes += sz
			st.Files++
		}
		if st.Bytes == 0 {
			continue
		}
		// How much of the set each drive holds, anywhere in the project.
		for id, have := range byDriveKeys {
			var n int64
			for k, sz := range keys {
				if _, ok := have[k]; ok {
					n += sz
				}
			}
			if n > 0 {
				st.Copies[id] = float64(n) / float64(st.Bytes)
			}
		}
		full := 0
		for _, f := range st.Copies {
			if f >= 0.999 {
				full++
			}
		}
		switch {
		case full >= 2:
			st.Status = "backed"
		case len(st.Copies) <= 1:
			st.Status = "sole"
			plan.Sole += st.Bytes
		default:
			st.Status = "partial"
			plan.Partial += st.Bytes
		}
		setKeys[st] = keys
		sets = append(sets, st)
	}
	sort.Slice(sets, func(i, j int) bool { return natLess(sets[i].Name, sets[j].Name) })
	plan.Sets = sets
	// What each drive holds of the project, and of one set.
	driveBytes := map[string]int64{}
	for id, have := range byDriveKeys {
		for _, sz := range have {
			driveBytes[id] += sz
		}
	}
	have := func(id string, st *conSet) int64 { return int64(st.Copies[id] * float64(st.Bytes)) }
	spare := map[string]int64{}
	for _, d := range all {
		if d.NoBackups {
			continue
		}
		spare[d.ID] = free(d.ID)
	}
	ids := make([]string, 0, len(spare))
	for id := range spare {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	// bestMirror is the drive for a range's second copy: the one already
	// holding the most of it with room for the rest, preferring a drive
	// with no other part in the plan, so the shape stays one pair per
	// range rather than a web.
	taken := map[string]bool{}
	bestMirror := func(prim string, rng []*conSet) (string, int64) {
		best, bestScore, bestMiss := "", int64(-1), int64(0)
		for _, id := range ids {
			if id == prim {
				continue
			}
			var got, miss int64
			for _, st := range rng {
				got += have(id, st)
				miss += st.Bytes - have(id, st)
			}
			if miss > spare[id] {
				continue
			}
			score := got
			if !taken[id] {
				score += 1 << 40
			}
			if score > bestScore {
				best, bestScore, bestMiss = id, score, miss
			}
		}
		return best, bestMiss
	}
	// The pairs: the user's, or proposed by walking the days in order,
	// each range on the drive that holds the most of its first day, as
	// long as that drive has room for the rest.
	var pairs []conPair
	if layout != nil {
		for _, pr := range layout.Pairs {
			// A mirror has to be another drive.
			if pr.Mirror == pr.Primary {
				pr.Mirror = ""
			}
			pairs = append(pairs, pr)
		}
		plan.Proposed = false
	} else {
		plan.Proposed = true
		i := 0
		for i < len(sets) {
			first := sets[i]
			prim, bestF := "", -1.0
			for _, id := range ids {
				f := first.Copies[id]
				if f > bestF+0.001 || (f > bestF-0.001 && driveBytes[id] > driveBytes[prim]) {
					prim, bestF = id, f
				}
			}
			if prim == "" {
				break
			}
			room := spare[prim]
			j := i
			for j < len(sets) {
				miss := sets[j].Bytes - have(prim, sets[j])
				if j > i && miss > room {
					break
				}
				room -= miss
				j++
			}
			if j == i {
				j = i + 1
			}
			taken[prim] = true
			spare[prim] = room
			m, miss := bestMirror(prim, sets[i:j])
			if m != "" {
				taken[m] = true
				spare[m] -= miss
			}
			pairs = append(pairs, conPair{Primary: prim, Mirror: m, Through: sets[j-1].Name})
			i = j
		}
	}
	// Days flow into the pairs in order; the last pair takes the rest.
	si := 0
	for pi := range pairs {
		pr := &pairs[pi]
		start := si
		for si < len(sets) {
			st := sets[si]
			st.Pair = pi
			st.Primary = pr.Primary
			st.Mirror = pr.Mirror
			pr.Bytes += st.Bytes
			si++
			if pi < len(pairs)-1 && st.Name == pr.Through {
				break
			}
		}
		if si > start {
			pr.First, pr.Last = sets[start].Name, sets[si-1].Name
		}
		pr.Through = pr.Last
	}
	for ; si < len(sets); si++ {
		plan.Unassigned = append(plan.Unassigned, sets[si].Name)
	}
	plan.Pairs = pairs
	// Copies: onto the primary and the mirror, from any drive with a
	// whole copy (a plugged-in one first), else the one with the most.
	source := func(st *conSet, not map[string]bool) string {
		best, bestF := "", -1.0
		for _, id := range ids {
			f := st.Copies[id]
			if not[id] || f == 0 {
				continue
			}
			score := f
			if s.mountOf(id) != nil {
				score += 0.0005
			}
			if score > bestF {
				best, bestF = id, score
			}
		}
		return best
	}
	for _, st := range sets {
		if st.Pair < 0 {
			continue
		}
		for _, to := range []string{st.Primary, st.Mirror} {
			if to == "" {
				if st.Mirror == "" {
					plan.Buy += st.Bytes
				}
				continue
			}
			miss := st.Bytes - have(to, st)
			if miss <= 0 {
				continue
			}
			from := source(st, map[string]bool{to: true})
			files := 0
			for k := range setKeys[st] {
				if _, ok := byDriveKeys[to][k]; !ok {
					files++
				}
			}
			st.CopyB += miss
			plan.Transfers = append(plan.Transfers, conTransfer{Set: st.Name, From: from, To: to, Bytes: miss, Files: files})
		}
		for id, f := range st.Copies {
			if id != st.Primary && id != st.Mirror {
				st.Clears = append(st.Clears, conClear{DriveID: id, Bytes: int64(f * float64(st.Bytes))})
			}
		}
		sort.Slice(st.Clears, func(i, j int) bool { return st.Clears[i].Bytes > st.Clears[j].Bytes })
	}
	// The drives, before and after.
	involved := map[string]bool{}
	for _, st := range sets {
		for id := range st.Copies {
			involved[id] = true
		}
		if st.Primary != "" {
			involved[st.Primary] = true
		}
		if st.Mirror != "" {
			involved[st.Mirror] = true
		}
	}
	for id := range involved {
		d := drives[id]
		if d == nil {
			continue
		}
		cd := &conDrive{ID: id, Title: d.Title(), Size: d.Size, Free: free(id), Connected: s.mountOf(id) != nil, PrimaryOf: []string{}, MirrorOf: []string{}}
		for _, st := range sets {
			cd.NowBytes += have(id, st)
			if st.Primary == id {
				cd.PrimaryOf = append(cd.PrimaryOf, st.Name)
			}
			if st.Mirror == id {
				cd.MirrorOf = append(cd.MirrorOf, st.Name)
			}
			for _, c := range st.Clears {
				if c.DriveID == id {
					cd.ClearOut += c.Bytes
				}
			}
		}
		for _, t := range plan.Transfers {
			if t.To == id {
				cd.CopyIn += t.Bytes
			}
		}
		cd.FreeAfter = cd.Free - cd.CopyIn + cd.ClearOut
		plan.Drives = append(plan.Drives, cd)
	}
	sort.Slice(plan.Drives, func(i, j int) bool { return plan.Drives[i].NowBytes > plan.Drives[j].NowBytes })
	return plan, nil
}

// consolidateRename is POST /api/projects/{id}/consolidate/rename
// {from, to}: a day's folder is renamed on every plugged-in drive in
// the project that has a folder of that name at the top of one of the
// project's folders, so the same day carries the same name everywhere.
// Each drive renamed is rescanned.
func (s *Server) consolidateRename(w http.ResponseWriter, r *http.Request) (any, error) {
	p, err := s.projectFrom(r)
	if err != nil {
		return nil, err
	}
	var body struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, badRequest("bad request: %v", err)
	}
	from, to := strings.TrimSpace(body.From), strings.TrimSpace(body.To)
	if from == "" || to == "" || from == to || strings.ContainsAny(to, `/\:`) || strings.HasPrefix(to, ".") {
		return nil, badRequest("give the folder a plain new name")
	}
	var renamed, skipped []string
	rescan := map[string]bool{}
	for _, src := range p.Sources {
		v := s.mountOf(src.DriveID)
		if v == nil {
			continue
		}
		dir := filepath.Join(v.Mount, filepath.FromSlash(src.Folder))
		old := filepath.Join(dir, from)
		st, err := os.Stat(old)
		if err != nil || !st.IsDir() {
			continue
		}
		next := filepath.Join(dir, to)
		if _, err := os.Stat(next); err == nil {
			skipped = append(skipped, v.Name+": "+to+" is already there")
			continue
		}
		if err := os.Rename(old, next); err != nil {
			skipped = append(skipped, v.Name+": "+err.Error())
			continue
		}
		renamed = append(renamed, v.Name)
		rescan[src.DriveID] = true
	}
	if len(renamed) == 0 && len(skipped) == 0 {
		return nil, badRequest("no plugged-in drive in this project has a folder called %s at the top of a project folder", from)
	}
	for id := range rescan {
		v := s.mountOf(id)
		d, err := s.db.Drive(id)
		if v == nil || err != nil {
			continue
		}
		s.jobs.Start("rescan", "Rescan "+d.Title(), "lumpy catalog "+shellQuote(v.Mount), id, func(ctx context.Context, rep *job.Report) error {
			_, err := job.CatalogDrive(ctx, s.db, v, rep)
			s.invalidate()
			return err
		})
	}
	for id := range rescan {
		s.db.Log(id, "rename", fmt.Sprintf("%s: folder %s renamed to %s", p.Name, from, to), 0)
	}
	return map[string]any{"renamed": renamed, "skipped": skipped}, nil
}

// natLess orders DAY 2 before DAY 10.
func natLess(a, b string) bool {
	ia, ib := 0, 0
	for ia < len(a) && ib < len(b) {
		ca, cb := a[ia], b[ib]
		if isDigit(ca) && isDigit(cb) {
			ja, jb := ia, ib
			for ja < len(a) && isDigit(a[ja]) {
				ja++
			}
			for jb < len(b) && isDigit(b[jb]) {
				jb++
			}
			na, _ := strconv.Atoi(a[ia:ja])
			nb, _ := strconv.Atoi(b[ib:jb])
			if na != nb {
				return na < nb
			}
			ia, ib = ja, jb
			continue
		}
		la, lb := strings.ToLower(string(ca)), strings.ToLower(string(cb))
		if la != lb {
			return la < lb
		}
		ia++
		ib++
	}
	return len(a)-ia < len(b)-ib
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
