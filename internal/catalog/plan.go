package catalog

import (
	"sort"
)

// PlanFile is one file a transfer copies.
type PlanFile struct {
	Path  string `json:"path"` // on the source drive
	Size  int64  `json:"size"`
	XXH64 string `json:"xxh64"`
}

// Transfer is one step of a plan: copy these files from one drive to
// another. Both drives must be plugged in to run it.
type Transfer struct {
	SourceID string     `json:"sourceId"`
	DestID   string     `json:"destId"`
	Files    []PlanFile `json:"files"`
	Bytes    int64      `json:"bytes"`
	// Folders summarises Files by top-level folder: name -> bytes.
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

// DriveSpace is what the planner knows about a drive.
type DriveSpace struct {
	ID        string
	Free      int64
	Connected bool
}

// MakePlan chooses destinations for every content that needs more copies.
// It prefers drives that are plugged in now (so there is a step to run
// straight away), then keeps the number of drive pairs small (so each
// step is "plug in these two and run"), then fills the emptiest drive
// first. The plan is recomputed whenever it is shown, so it follows the
// drives that come and go.
func MakePlan(h *Health, drives []DriveSpace) *Plan {
	const reserve = 2 << 30 // keep 2 GB free on every destination
	plan := &Plan{Reserve: reserve}
	space := map[string]*DriveSpace{}
	remaining := map[string]int64{}
	for i := range drives {
		d := &drives[i]
		space[d.ID] = d
		remaining[d.ID] = d.Free - reserve
	}
	pairs := map[[2]string]*Transfer{}
	pair := func(src, dst string) *Transfer {
		k := [2]string{src, dst}
		t := pairs[k]
		if t == nil {
			t = &Transfer{SourceID: src, DestID: dst, Folders: map[string]int64{}}
			pairs[k] = t
			plan.Transfers = append(plan.Transfers, t)
		}
		return t
	}

	for _, c := range h.Needs {
		missing := h.Target - c.Copies()
		holders := map[string]bool{}
		for _, hd := range c.Holders {
			holders[hd.DriveID] = true
		}
		placed := true
		for k := 0; k < missing; k++ {
			// Choose the destination: not a holder, with room; prefer one
			// plugged in alongside a plugged-in holder, then one already
			// paired with a holder, then the most free space.
			var best *DriveSpace
			bestScore := -1
			for _, d := range drives {
				d := d
				if holders[d.ID] || remaining[d.ID] < c.Size {
					continue
				}
				score := 0
				for _, hd := range c.Holders {
					if pairs[[2]string{hd.DriveID, d.ID}] != nil {
						score += 2
						break
					}
				}
				if d.Connected {
					score++
					for _, hd := range c.Holders {
						if sp := space[hd.DriveID]; sp != nil && sp.Connected {
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
			// Choose the source among holders: one already paired with
			// this destination, else a plugged-in one, else the first.
			src := c.Holders[0]
			for _, hd := range c.Holders {
				if pairs[[2]string{hd.DriveID, best.ID}] != nil {
					src = hd
					break
				}
				if sp := space[hd.DriveID]; sp != nil && sp.Connected && !(space[src.DriveID] != nil && space[src.DriveID].Connected) {
					src = hd
				}
			}
			t := pair(src.DriveID, best.ID)
			t.Files = append(t.Files, PlanFile{Path: src.Path, Size: c.Size, XXH64: c.XXH64})
			t.Bytes += c.Size
			t.Folders[TopFolder(src.Path)] += c.Size
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
		sort.Slice(t.Files, func(i, j int) bool { return t.Files[i].Path < t.Files[j].Path })
	}
	return plan
}
