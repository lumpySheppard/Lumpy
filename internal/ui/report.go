package ui

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/lumpysheppard/lumpy/internal/catalog"
)

// The shoot-day report: one page per day saying what came off which
// cards, where every copy went, what was verified, and what is still
// at risk. It is what a producer or an insurer asks for, written from
// what the library already holds.

type dayCard struct {
	Drive    string
	DriveID  string
	Files    int
	Bytes    int64
	Clips    int
	Running  time.Duration
	Hashed   int
	Verdict  string
	Holders  []CardHolder
	Short    int
	Offloads []string // where it went, as the events say
	Sums     []dayFile
}

type dayFile struct {
	Path   string
	Size   int64
	XXH64  string
	Copies int
}

type dayReport struct {
	Date      string
	Made      string
	Version   string
	Cards     []dayCard
	Events    []catalog.DayEvent
	Checks    []catalog.DayEvent
	Logged    []catalog.DayEvent
	Files     int
	Bytes     int64
	Clips     int
	Running   time.Duration
	AtRisk    int
	Verified  int
	NoOffload bool
}

// dayReportData gathers the day: every card that was an offload source
// that day, with its state now.
func (s *Server) dayReportData(day time.Time) (*dayReport, error) {
	from := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.Local)
	to := from.Add(24 * time.Hour)
	evs, err := s.db.EventsBetween(from, to)
	if err != nil {
		return nil, err
	}
	rep := &dayReport{Date: from.Format("Monday 2 January 2006"), Made: time.Now().Format("2 Jan 2006 15:04"), Version: s.meta.Version}
	srcs := map[string]*dayCard{}
	var order []string
	for _, e := range evs {
		switch e.Kind {
		case "offload":
			if strings.HasPrefix(e.Summary, "offload source") {
				c := srcs[e.DriveID]
				if c == nil {
					c = &dayCard{Drive: e.DriveName, DriveID: e.DriveID}
					srcs[e.DriveID] = c
					order = append(order, e.DriveID)
				}
			}
			rep.Events = append(rep.Events, e)
		case "verified", "hashed", "erase":
			rep.Checks = append(rep.Checks, e)
		case "scanned", "mounted":
			rep.Logged = append(rep.Logged, e)
		}
	}
	// Where each card's offloads went: the destination events of the
	// same day name the folder.
	for _, e := range evs {
		if e.Kind == "offload" && strings.HasPrefix(e.Summary, "offload destination") {
			for _, c := range srcs {
				c.Offloads = append(c.Offloads, e.DriveName+": "+strings.TrimPrefix(e.Summary, "offload destination: "))
			}
		}
	}
	for _, id := range order {
		c := srcs[id]
		d, err := s.db.Drive(id)
		if err != nil {
			continue
		}
		cv, err := s.cardView(d)
		if err != nil {
			continue
		}
		c.Files, c.Bytes, c.Hashed, c.Verdict, c.Holders, c.Short = cv.Files, cv.Bytes, cv.Hashed, cv.Verdict, cv.Holders, len(cv.Short)
		if ms, err := s.db.ListMedia(catalog.MediaFilter{DriveID: id, Kind: "video", Limit: 100000}); err == nil {
			c.Clips = len(ms)
			for _, m := range ms {
				c.Running += time.Duration(m.DurationMS) * time.Millisecond
			}
		}
		if cov, err := s.db.Coverage(id); err == nil {
			for _, f := range cov {
				c.Sums = append(c.Sums, dayFile{Path: f.Path, Size: f.Size, XXH64: f.XXH64, Copies: len(f.Copies)})
			}
			sort.Slice(c.Sums, func(i, j int) bool { return c.Sums[i].Path < c.Sums[j].Path })
		}
		rep.Files += c.Files
		rep.Bytes += c.Bytes
		rep.Clips += c.Clips
		rep.Running += c.Running
		rep.AtRisk += c.Short
		if c.Verdict == "safe" {
			rep.Verified++
		}
		rep.Cards = append(rep.Cards, *c)
	}
	rep.NoOffload = len(rep.Cards) == 0
	return rep, nil
}

var dayTmpl = template.Must(template.New("day").Funcs(template.FuncMap{
	"bytes": func(n int64) string { return humanBytes(n) },
	"dur": func(d time.Duration) string {
		d = d.Round(time.Second)
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		sec := int(d.Seconds()) % 60
		return fmt.Sprintf("%02d:%02d:%02d", h, m, sec)
	},
	"clock": func(t time.Time) string { return t.Format("15:04") },
	"verdict": func(v string) string {
		switch v {
		case "safe":
			return "Verified on two other drives: safe to format"
		case "once":
			return "One verified copy elsewhere: the card is still the second copy"
		}
		return "Not yet backed up: keep the card"
	},
}).Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>Shoot day · {{.Date}}</title>
<style>
body { font: 14px/1.5 -apple-system, "Helvetica Neue", Helvetica, Arial, sans-serif; color: #111; margin: 40px auto; max-width: 900px; padding: 0 24px; }
h1 { font-size: 24px; margin: 0 0 4px; } h2 { font-size: 16px; margin: 28px 0 8px; border-bottom: 1px solid #ddd; padding-bottom: 4px; }
.sub { color: #666; margin: 0 0 20px; }
.stats { display: flex; gap: 24px; flex-wrap: wrap; margin: 12px 0 8px; } .stats div { min-width: 120px; } .stats b { display: block; font-size: 20px; }
table { border-collapse: collapse; width: 100%; font-size: 13px; } th, td { text-align: left; padding: 5px 8px; border-bottom: 1px solid #eee; vertical-align: top; } th { color: #666; font-weight: 600; font-size: 12px; }
td.n, th.n { text-align: right; font-variant-numeric: tabular-nums; white-space: nowrap; }
.ok { color: #0a7d3b; } .warn { color: #b06000; } .bad { color: #b3261e; }
.mono { font-family: ui-monospace, Menlo, monospace; font-size: 12px; }
details summary { cursor: pointer; color: #555; margin: 6px 0; }
.foot { color: #888; font-size: 12px; margin-top: 36px; }
@media print { body { margin: 0; } details { display: block; } }
</style></head><body>
<h1>Shoot day · {{.Date}}</h1>
<p class="sub">Made {{.Made}} by Lumpy {{.Version}}</p>
{{if .NoOffload}}<p>No cards were offloaded on this day.</p>{{else}}
<div class="stats"><div><b>{{len .Cards}}</b>cards offloaded</div><div><b>{{.Clips}}</b>clips</div><div><b>{{dur .Running}}</b>running time</div><div><b>{{bytes .Bytes}}</b>{{.Files}} files</div><div><b class="{{if eq .Verified (len .Cards)}}ok{{else}}warn{{end}}">{{.Verified}} of {{len .Cards}}</b>cards safe to format</div>{{if .AtRisk}}<div><b class="bad">{{.AtRisk}}</b>files without two verified copies</div>{{end}}</div>
{{range .Cards}}
<h2>{{.Drive}}</h2>
<p class="{{if eq .Verdict "safe"}}ok{{else if eq .Verdict "once"}}warn{{else}}bad{{end}}"><b>{{verdict .Verdict}}.</b> {{.Files}} files, {{bytes .Bytes}}, {{.Clips}} clips, {{dur .Running}} running time, {{.Hashed}} of {{.Files}} checksummed.</p>
{{if .Offloads}}<p>Offloaded to: {{range $i, $o := .Offloads}}{{if $i}} · {{end}}{{$o}}{{end}}</p>{{end}}
<table><tr><th>Copy on</th><th class="n">Files</th><th class="n">Verified</th><th>Plugged in now</th></tr>
{{range .Holders}}<tr><td>{{.Drive}}</td><td class="n">{{.Files}}</td><td class="n {{if eq .Verified .Files}}ok{{else}}warn{{end}}">{{.Verified}}</td><td>{{if .Connected}}yes{{else}}no{{end}}</td></tr>{{end}}
{{if not .Holders}}<tr><td colspan="4" class="bad">No copies anywhere else.</td></tr>{{end}}</table>
<details><summary>Checksums for {{len .Sums}} files (xxh64)</summary><table><tr><th>File</th><th class="n">Size</th><th>xxh64</th><th class="n">Copies</th></tr>
{{range .Sums}}<tr><td class="mono">{{.Path}}</td><td class="n">{{bytes .Size}}</td><td class="mono">{{if .XXH64}}{{.XXH64}}{{else}}—{{end}}</td><td class="n">{{.Copies}}</td></tr>{{end}}</table></details>
{{end}}{{end}}
{{if .Checks}}<h2>Checks and scans that day</h2><table>{{range .Checks}}<tr><td class="n">{{clock .At}}</td><td>{{.DriveName}}</td><td>{{.Summary}}</td><td class="n {{if .Failures}}bad{{end}}">{{if .Failures}}{{.Failures}} failed{{end}}</td></tr>{{end}}</table>{{end}}
{{if .Events}}<h2>Offload log</h2><table>{{range .Events}}<tr><td class="n">{{clock .At}}</td><td>{{.DriveName}}</td><td>{{.Summary}}</td></tr>{{end}}</table>{{end}}
<p class="foot">Verified means a copy was read back from its drive after the offload and its xxh64 checksum matched the source. Lumpy counts copies on different drives only; two copies on one drive protect nothing.</p>
</body></html>`))

// dayReport is GET /api/report/day?date=YYYY-MM-DD, an HTML page.
func (s *Server) dayReport(w http.ResponseWriter, r *http.Request) (any, error) {
	day := time.Now()
	if v := r.URL.Query().Get("date"); v != "" {
		t, err := time.ParseInLocation("2006-01-02", v, time.Local)
		if err != nil {
			return nil, badRequest("date must be YYYY-MM-DD")
		}
		day = t
	}
	rep, err := s.dayReportData(day)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := dayTmpl.Execute(&buf, rep); err != nil {
		return nil, err
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(buf.Bytes())
	return nil, nil
}

func humanBytes(n int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	f := float64(n)
	i := 0
	for f >= 1000 && i < len(units)-1 {
		f /= 1000
		i++
	}
	if i < 2 {
		return fmt.Sprintf("%.0f %s", f, units[i])
	}
	if f < 10 {
		return fmt.Sprintf("%.2f %s", f, units[i])
	}
	return fmt.Sprintf("%.1f %s", f, units[i])
}
