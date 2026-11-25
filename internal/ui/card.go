package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/lumpysheppard/lumpy/internal/catalog"
	"github.com/lumpysheppard/lumpy/internal/job"
	"github.com/lumpysheppard/lumpy/internal/volume"
)

// CardView is the one screen that answers "can I format this card?":
// every file, every copy, each copy's checksum state, and one verdict.
type CardView struct {
	Drive     string `json:"drive"`
	DriveID   string `json:"driveId"`
	Connected bool   `json:"connected"`
	Files     int    `json:"files"`
	Bytes     int64  `json:"bytes"`
	Hashed    int    `json:"hashed"`
	Clips     int    `json:"clips"`
	// Verdict is "safe" (every file has two verified copies on two
	// other drives), "once" (every file has one), or "no".
	Verdict string `json:"verdict"`
	// MinVerified is the fewest verified copies any file has elsewhere.
	MinVerified int          `json:"minVerified"`
	Holders     []CardHolder `json:"holders"`
	// Short lists files without two verified copies, with what they do
	// have (first 300).
	Short   []CardShort `json:"short"`
	Damaged int         `json:"damaged"`
	Busy    bool        `json:"busy"`
	// Why says what stops the verdict being "safe".
	Why []string `json:"why"`
}

type CardHolder struct {
	DriveID   string `json:"driveId"`
	Drive     string `json:"drive"`
	Files     int    `json:"files"`    // files of the card it holds a copy of
	Verified  int    `json:"verified"` // of those, read back and matched
	Connected bool   `json:"connected"`
}

type CardShort struct {
	Path     string   `json:"path"`
	Verified int      `json:"verified"`
	Copies   int      `json:"copies"`
	Likely   int      `json:"likely"`
	Unhashed bool     `json:"unhashed"`
	Damaged  bool     `json:"damaged"`
	On       []string `json:"on"`
}

// cardStatus is GET /api/drives/{id}/card.
func (s *Server) cardStatus(w http.ResponseWriter, r *http.Request) (any, error) {
	d, err := s.drive(r)
	if err != nil {
		return nil, err
	}
	return s.cardView(d)
}

func (s *Server) cardView(d *catalog.Drive) (*CardView, error) {
	cov, err := s.db.Coverage(d.ID)
	if err != nil {
		return nil, err
	}
	cv := &CardView{Drive: d.Title(), DriveID: d.ID, Connected: s.mountOf(d.ID) != nil, Busy: s.jobs.Busy(d.ID), Files: len(cov), MinVerified: -1, Holders: []CardHolder{}, Short: []CardShort{}, Why: []string{}}
	holders := map[string]*CardHolder{}
	unhashed, damaged := 0, 0
	for _, c := range cov {
		cv.Bytes += c.Size
		if c.XXH64 != "" {
			cv.Hashed++
		} else {
			unhashed++
		}
		if c.BadHash != "" {
			damaged++
		}
		// Verified copies on distinct other drives.
		ver := map[string]bool{}
		any := map[string]bool{}
		var on []string
		for _, cp := range c.Copies {
			any[cp.DriveID] = true
			h := holders[cp.DriveID]
			if h == nil {
				h = &CardHolder{DriveID: cp.DriveID, Drive: cp.DriveName, Connected: s.mountOf(cp.DriveID) != nil}
				holders[cp.DriveID] = h
			}
			h.Files++
			if cp.Verified {
				h.Verified++
				ver[cp.DriveID] = true
			}
			on = append(on, cp.DriveName)
		}
		n := len(ver)
		if c.XXH64 != "" && (cv.MinVerified < 0 || n < cv.MinVerified) {
			cv.MinVerified = n
		}
		if n < 2 && len(cv.Short) < 300 {
			cv.Short = append(cv.Short, CardShort{Path: c.Path, Verified: n, Copies: len(any), Likely: len(c.Likely), Unhashed: c.XXH64 == "", Damaged: c.BadHash != "", On: on})
		}
	}
	cv.Damaged = damaged
	for _, h := range holders {
		cv.Holders = append(cv.Holders, *h)
	}
	sort.Slice(cv.Holders, func(i, j int) bool {
		if cv.Holders[i].Verified != cv.Holders[j].Verified {
			return cv.Holders[i].Verified > cv.Holders[j].Verified
		}
		return cv.Holders[i].Drive < cv.Holders[j].Drive
	})
	if ms, err := s.db.ListMedia(catalog.MediaFilter{DriveID: d.ID, Kind: "video", Limit: 100000}); err == nil {
		cv.Clips = len(ms)
	}
	switch {
	case len(cov) == 0:
		cv.Verdict = "no"
		cv.Why = append(cv.Why, "Nothing is recorded on this card yet: scan it first")
	case damaged > 0:
		cv.Verdict = "no"
		cv.Why = append(cv.Why, fmt.Sprintf("%d file%s failed its last check here", damaged, plural(damaged)))
	case unhashed > 0:
		cv.Verdict = "no"
		cv.Why = append(cv.Why, fmt.Sprintf("%d file%s have no checksum: Checksum new files, then check the copies", unhashed, plural(unhashed)))
	case cv.MinVerified >= 2:
		cv.Verdict = "safe"
	case cv.MinVerified == 1:
		cv.Verdict = "once"
		cv.Why = append(cv.Why, "every file has one verified copy elsewhere: this card is still the second copy")
	default:
		cv.Verdict = "no"
		cv.Why = append(cv.Why, "some files have no verified copy anywhere else")
	}
	return cv, nil
}

// eraseCard is POST /api/drives/{id}/erase: the card is formatted in
// place only when two verified copies of every file exist on two other
// drives and the request repeats the card's name.
func (s *Server) eraseCard(w http.ResponseWriter, r *http.Request) (any, error) {
	d, err := s.drive(r)
	if err != nil {
		return nil, err
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, badRequest("bad request: %v", err)
	}
	v := s.mountOf(d.ID)
	if v == nil {
		return nil, badRequest("%s is not plugged in", d.Title())
	}
	if strings.TrimSpace(body.Name) != d.Title() && strings.TrimSpace(body.Name) != d.Name {
		return nil, badRequest("type the card's name, %s, to erase it", d.Title())
	}
	if s.jobs.Busy(d.ID) {
		return nil, badRequest("a job is still running on %s", d.Title())
	}
	cv, err := s.cardView(d)
	if err != nil {
		return nil, err
	}
	if cv.Verdict != "safe" {
		return nil, badRequest("%s is not safe to erase: %s", d.Title(), strings.Join(cv.Why, "; "))
	}
	title := d.Title()
	j := s.jobs.Start("erase", "Erase "+title, "diskutil eraseVolume "+shellQuote(v.Mount), d.ID, func(ctx context.Context, rep *job.Report) error {
		rep.Info("Every one of %d files on %s has at least %d verified copies on other drives; erasing it.", cv.Files, title, cv.MinVerified)
		if err := volume.Erase(v.Mount, v.Name, v.FSType); err != nil {
			return err
		}
		s.db.Log(d.ID, "erase", fmt.Sprintf("erased after %d files were verified on %d other drives", cv.Files, len(cv.Holders)), 0)
		rep.Info("%s is empty. Bringing the library up to date.", title)
		s.mu.Lock()
		s.volsAt = time.Time{}
		s.mu.Unlock()
		if nv := s.mountOf(d.ID); nv != nil {
			_, err := job.CatalogDrive(ctx, s.db, nv, rep)
			return err
		}
		return nil
	})
	return map[string]any{"job": j.ID}, nil
}
