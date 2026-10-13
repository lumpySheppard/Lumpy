package ui

import (
	"net/http"

	"github.com/lumpysheppard/lumpy/internal/catalog"
)

// ProxyView is one Online/Offline pair as the app shows it: the counts,
// and the originals that still have no proxy (the first maxMissing).
type ProxyView struct {
	DriveID      string              `json:"driveId"`
	Online       string              `json:"online"`
	Offline      string              `json:"offline"`
	Total        int                 `json:"total"`
	Proxied      int                 `json:"proxied"`
	MissingBytes int64               `json:"missingBytes"`
	Extra        int                 `json:"extra"`
	Missing      []catalog.ProxyClip `json:"missing"`
	Truncated    bool                `json:"truncated"`
}

const maxMissing = 500

func (s *Server) proxyViews(driveID, folder string) ([]ProxyView, error) {
	reps, err := s.db.ProxyReports(driveID, folder)
	if err != nil {
		return nil, err
	}
	out := []ProxyView{}
	for _, rep := range reps {
		v := ProxyView{DriveID: driveID, Online: rep.Online, Offline: rep.Offline, Total: rep.Total, Proxied: rep.Proxied, MissingBytes: rep.MissingBytes, Extra: rep.Extra, Missing: []catalog.ProxyClip{}}
		for _, c := range rep.Clips {
			if c.Proxy != "" {
				continue
			}
			if len(v.Missing) >= maxMissing {
				v.Truncated = true
				break
			}
			v.Missing = append(v.Missing, c)
		}
		out = append(out, v)
	}
	return out, nil
}

// driveProxies lists the Online/Offline pairs anywhere on a drive.
func (s *Server) driveProxies(w http.ResponseWriter, r *http.Request) (any, error) {
	d, err := s.drive(r)
	if err != nil {
		return nil, err
	}
	return s.proxyViews(d.ID, "")
}

// projectProxies lists the pairs inside a project's folders, each once.
func (s *Server) projectProxies(w http.ResponseWriter, r *http.Request) (any, error) {
	p, err := s.projectFrom(r)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := []ProxyView{}
	for _, src := range p.Sources {
		vs, err := s.proxyViews(src.DriveID, src.Folder)
		if err != nil {
			return nil, err
		}
		for _, v := range vs {
			key := v.DriveID + "\x00" + v.Online
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, v)
		}
	}
	return out, nil
}
