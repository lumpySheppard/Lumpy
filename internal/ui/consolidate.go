package ui

import (
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/lumpysheppard/lumpy/internal/catalog"
)

// Consolidate: a project that grew across many drives before there
// was a standard (a day here, a mirror there, a third copy of half of
// it somewhere else) is planned into a clean shape: every shooting
// day (a set) on one primary drive and one mirror that holds only
// what its primary holds, with the redundant copies listed to clear
// once the two verified copies exist. Nothing here moves a byte; it is
// the plan to look at.

// A conSet is one logical shooting day or card: the folders on the
// drives that hold it, matched by content, however they are named.
type conSet struct {
	Name    string             `json:"name"`
	Aliases []string           `json:"aliases"` // other names the same content goes by
	Bytes   int64              `json:"bytes"`
	Files   int                `json:"files"`
	Copies  map[string]float64 `json:"copies"` // drive id -> fraction of the set's bytes there
	Status  string             `json:"status"` // sole, partial, backed
	Primary string             `json:"primary"`
	Mirror  string             `json:"mirror"`
	CopyB   int64              `json:"copyBytes"` // still to copy onto the mirror
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

type conPlan struct {
	Sets      []*conSet         `json:"sets"`
	Drives    []*conDrive       `json:"drives"`
	Transfers []conTransfer     `json:"transfers"`
	Buy       int64             `json:"buy"` // bytes no drive has room to mirror
	Sole      int64             `json:"soleBytes"`
	Partial   int64             `json:"partialBytes"`
	Offline   []string          `json:"offline"` // drives in the project that are not logged
	Pairs     map[string]string `json:"pairs"`   // primary drive -> mirror drive
}

// chunk is one top-level folder of the project on one drive.
type chunk struct {
	drive string
	name  string
	keys  map[string]int64 // content key -> size
	bytes int64
}

func fileKey(f catalog.File) string {
	if f.XXH64 != "" {
		return "h:" + f.XXH64
	}
	return "n:" + path.Base(f.Path) + ":" + strconv.FormatInt(f.Size, 10)
}

// consolidatePlan is GET /api/projects/{id}/consolidate.
func (s *Server) consolidatePlan(w http.ResponseWriter, r *http.Request) (any, error) {
	p, err := s.projectFrom(r)
	if err != nil {
		return nil, err
	}
	all, err := s.db.Drives()
	if err != nil {
		return nil, err
	}
	drives := map[string]*catalog.Drive{}
	for _, d := range all {
		drives[d.ID] = d
	}
	plan := &conPlan{Sets: []*conSet{}, Drives: []*conDrive{}, Transfers: []conTransfer{}, Offline: []string{}, Pairs: map[string]string{}}
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
		sort.Slice(g, func(i, j int) bool { return g[i].bytes > g[j].bytes })
		st := &conSet{Name: g[0].name, Aliases: []string{}, Copies: map[string]float64{}, Clears: []conClear{}}
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
	// Primary: where the most of a set already is; among equals, the
	// drive that is primary for the most bytes already, so days stay
	// together.
	// Among equals the drive holding the most of the project overall,
	// then the id, so the answer is the same every time.
	driveBytes := map[string]int64{}
	for id, have := range byDriveKeys {
		for _, sz := range have {
			driveBytes[id] += sz
		}
	}
	primBytes := map[string]int64{}
	for _, st := range sets {
		best, bestF := "", -1.0
		for id, f := range st.Copies {
			better := f > bestF+0.001
			if !better && f > bestF-0.001 {
				switch {
				case primBytes[id] != primBytes[best]:
					better = primBytes[id] > primBytes[best]
				case driveBytes[id] != driveBytes[best]:
					better = driveBytes[id] > driveBytes[best]
				default:
					better = id < best
				}
			}
			if better {
				best, bestF = id, f
			}
		}
		st.Primary = best
		primBytes[best] += st.Bytes
	}
	// Mirrors: one per primary drive, the drive that already holds the
	// most of that primary's sets and has room for the rest; a drive
	// that is nobody's primary is preferred, so the shape stays simple.
	prims := []string{}
	for id := range primBytes {
		prims = append(prims, id)
	}
	sort.Slice(prims, func(i, j int) bool { return primBytes[prims[i]] > primBytes[prims[j]] })
	spare := map[string]int64{}
	for _, d := range all {
		if d.NoBackups {
			continue
		}
		spare[d.ID] = free(d.ID)
	}
	need := func(prim, m string) (int64, int64) {
		var have, miss int64
		for _, st := range sets {
			if st.Primary != prim {
				continue
			}
			f := st.Copies[m]
			have += int64(f * float64(st.Bytes))
			miss += st.Bytes - int64(f*float64(st.Bytes))
		}
		return have, miss
	}
	for _, prim := range prims {
		best, bestScore := "", int64(-1)
		var bestMiss int64
		ids := make([]string, 0, len(spare))
		for id := range spare {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			if id == prim {
				continue
			}
			have, miss := need(prim, id)
			if miss > spare[id] {
				continue
			}
			score := have
			if primBytes[id] == 0 {
				score += 1 << 40 // a drive with no primaries of its own
			}
			if plan.Pairs[id] != "" {
				score -= 1 << 39 // already someone's mirror
			}
			if score > bestScore {
				best, bestScore, bestMiss = id, score, miss
			}
		}
		if best == "" {
			_, miss := need(prim, "")
			plan.Buy += miss
			continue
		}
		plan.Pairs[prim] = best
		spare[best] -= bestMiss
	}
	for _, st := range sets {
		m := plan.Pairs[st.Primary]
		st.Mirror = m
		if m != "" {
			st.CopyB = st.Bytes - int64(st.Copies[m]*float64(st.Bytes))
			if st.CopyB > 0 {
				files := 0
				keys := setKeys[st]
				have := byDriveKeys[m]
				for k := range keys {
					if _, ok := have[k]; !ok {
						files++
					}
				}
				plan.Transfers = append(plan.Transfers, conTransfer{Set: st.Name, From: st.Primary, To: m, Bytes: st.CopyB, Files: files})
			}
		}
		for id, f := range st.Copies {
			if id != st.Primary && id != m {
				st.Clears = append(st.Clears, conClear{DriveID: id, Bytes: int64(f * float64(st.Bytes))})
			}
		}
		sort.Slice(st.Clears, func(i, j int) bool { return st.Clears[i].Bytes > st.Clears[j].Bytes })
	}
	sort.Slice(plan.Transfers, func(i, j int) bool { return plan.Transfers[i].Bytes > plan.Transfers[j].Bytes })
	// The drives, before and after.
	involved := map[string]bool{}
	for _, st := range sets {
		for id := range st.Copies {
			involved[id] = true
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
			cd.NowBytes += int64(st.Copies[id] * float64(st.Bytes))
			if st.Primary == id {
				cd.PrimaryOf = append(cd.PrimaryOf, st.Name)
			}
			if st.Mirror == id {
				cd.MirrorOf = append(cd.MirrorOf, st.Name)
				cd.CopyIn += st.CopyB
			}
			for _, c := range st.Clears {
				if c.DriveID == id {
					cd.ClearOut += c.Bytes
				}
			}
		}
		cd.FreeAfter = cd.Free - cd.CopyIn + cd.ClearOut
		plan.Drives = append(plan.Drives, cd)
	}
	sort.Slice(plan.Drives, func(i, j int) bool { return plan.Drives[i].NowBytes > plan.Drives[j].NowBytes })
	plan.Sets = sets
	return plan, nil
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
