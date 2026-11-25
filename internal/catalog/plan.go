package catalog

import (
	"path"
	"sort"
)

// PlanFile is one file a transfer copies.
type PlanFile struct {
	Path  string `json:"path"` // on the source drive
	Dest  string `json:"dest"` // relative path under the destination folder
	Size  int64  `json:"size"`
	XXH64 string `json:"xxh64"`
}

// Transfer is one step of a plan: copy these files from one drive to
// another. Both drives must be plugged in to run it.
type Transfer struct {
	SourceID string `json:"sourceId"`
	DestID   string `json:"destId"`
	// Folder is the project's own folder on the destination drive the
	// files go into, keeping their paths relative to the source folder;
	// "" when they go into a folder named at run time.
	Folder string     `json:"folder"`
	Files  []PlanFile `json:"files"`
	Bytes  int64      `json:"bytes"`
	// Folders summarises Files by top-level destination folder: name -> bytes.
	Folders map[string]int64 `json:"folders"`
}

// Plan is how to get every file in a project onto Target drives.
type Plan struct {
	Transfers []*Transfer `json:"transfers"`
	// Unplaced are contents that no drive has room for, largest first,
	// and how many bytes that is.
	Unplaced      []*Content `json:"unplaced"`
	UnplacedBytes int64      `json:"unplacedBytes"`
	Reserve       int64      `json:"reserve"` // bytes kept free on each drive
}

// DriveSpace is a drive the planner may copy onto.
type DriveSpace struct {
	ID        string
	Free      int64
	Connected bool
}

// MakePlan chooses destinations for every content that needs more copies.
// Candidates are the drives backups may go onto; a drive never receives
// a file it already holds. Destinations that are plugged in alongside a
// plugged-in holder come first (so there is a step to run straight
// away), then drive pairs are kept together (so each step is "plug in
// these two and run"), then the emptiest drive is filled first. layout
// gives the destination path for an origin; see ProjectLayout.
func MakePlan(h *Health, candidates []DriveSpace, layout func(Origin) string) *Plan {
	return MakePlanRouted(h, candidates, layout, nil)
}

// MakePlanRouted is MakePlan with routing: route says, for an origin and
// a destination drive, which project folder on that drive the file is
// copied into with its relative path kept (ok false: none, so layout
// decides the path under a run-time folder). Files bound for different
// folders on the same drive become separate steps.
func MakePlanRouted(h *Health, candidates []DriveSpace, layout func(Origin) string, route func(Origin, string) (string, bool)) *Plan {
	const reserve = 2 << 30 // keep 2 GB free on every destination
	plan := &Plan{Reserve: reserve}
	space := map[string]*DriveSpace{}
	remaining := map[string]int64{}
	for i := range candidates {
		d := &candidates[i]
		space[d.ID] = d
		remaining[d.ID] = d.Free - reserve
	}
	pairs := map[[2]string]*Transfer{}
	steps := map[[3]string]*Transfer{}
	pair := func(src, dst, folder string) *Transfer {
		k := [3]string{src, dst, folder}
		t := steps[k]
		if t == nil {
			t = &Transfer{SourceID: src, DestID: dst, Folder: folder, Folders: map[string]int64{}}
			steps[k] = t
			pairs[[2]string{src, dst}] = t
			plan.Transfers = append(plan.Transfers, t)
		}
		return t
	}
	connected := func(id string) bool { return space[id] != nil && space[id].Connected }

	for _, c := range h.Needs {
		if len(c.Origins) == 0 {
			continue
		}
		missing := h.Target - c.Copies()
		holders := map[string]bool{}
		for _, hd := range c.Holders {
			holders[hd.DriveID] = true
		}
		placed := true
		for k := 0; k < missing; k++ {
			var best *DriveSpace
			bestScore := -1
			for _, d := range candidates {
				d := d
				if holders[d.ID] || remaining[d.ID] < c.Size {
					continue
				}
				score := 0
				for _, o := range c.Origins {
					if pairs[[2]string{o.DriveID, d.ID}] != nil {
						score += 2
						break
					}
				}
				if d.Connected {
					score++
					for _, o := range c.Origins {
						if connected(o.DriveID) {
							score += 4
							break
						}
					}
				}
				if best == nil || score > bestScore || (score == bestScore && remaining[d.ID] > remaining[best.ID]) {
					best, bestScore = space[d.ID], score
				}
			}
			if best == nil {
				placed = false
				break
			}
			// The source is one of the project's own copies: one already
			// paired with this destination, else a plugged-in one, else
			// the first.
			src := c.Origins[0]
			for _, o := range c.Origins {
				if pairs[[2]string{o.DriveID, best.ID}] != nil {
					src = o
					break
				}
				if connected(o.DriveID) && !connected(src.DriveID) {
					src = o
				}
			}
			folder, dest := "", layout(src)
			if route != nil {
				if f, ok := route(src, best.ID); ok {
					folder, dest = f, src.Rel
				}
			}
			t := pair(src.DriveID, best.ID, folder)
			t.Files = append(t.Files, PlanFile{Path: src.Path, Dest: dest, Size: c.Size, XXH64: c.XXH64})
			t.Bytes += c.Size
			t.Folders[TopFolder(dest)] += c.Size
			remaining[best.ID] -= c.Size
			holders[best.ID] = true
		}
		if !placed {
			plan.Unplaced = append(plan.Unplaced, c)
			plan.UnplacedBytes += c.Size
		}
	}
	sort.SliceStable(plan.Transfers, func(i, j int) bool { return plan.Transfers[i].Bytes > plan.Transfers[j].Bytes })
	for _, t := range plan.Transfers {
		sort.Slice(t.Files, func(i, j int) bool { return t.Files[i].Dest < t.Files[j].Dest })
	}
	return plan
}

// ProjectLayout decides where a copied file goes under the destination
// folder. With one source the file keeps its path relative to that
// source. With several, each source gets its own folder first: the
// source folder's name, or the drive's name for a whole drive.
func ProjectLayout(sources []Source, driveName func(id string) string) func(Origin) string {
	return func(o Origin) string {
		if len(sources) <= 1 {
			return o.Rel
		}
		src := sources[o.Source]
		base := src.Base()
		if base == "" {
			base = driveName(src.DriveID)
		}
		return path.Join(base, o.Rel)
	}
}
