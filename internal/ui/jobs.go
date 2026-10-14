package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/lumpysheppard/lumpy/internal/job"
)

// Job is a long-running command started from the app: an offload, a
// drive check, hashing or a rescan. Its fields are guarded by mu.
//
// Lock order: the job's Report takes its own lock and then, through
// Write, the job's mu. Nothing may hold mu while calling into report.
type Job struct {
	ID       int
	Kind     string
	Title    string
	Command  string // the equivalent lumpy command, for the log header
	DriveID  string
	DriveID2 string // a second drive the job occupies (transfers)
	State    string // running, ok, failed, cancelled, error
	Started  int64
	Ended    int64
	Failures int
	Warnings int
	Error    string
	// Summary is the one line that says how the job went, once it has
	// ended; Notify is that line again when the user wants telling, so
	// Lumpy.app can post it as a notification.
	Summary string
	Notify  string
	// Params are what started the job, so it can be run again: an
	// offload's source, destinations and name.
	Params map[string]any

	mu     sync.Mutex
	lines  []string
	count  int // lines written so far, including ones dropped from lines
	part   string
	report *job.Report
	cancel context.CancelFunc
	needs  []string // drives the job must have to itself
	fn     func(ctx context.Context, r *job.Report) error
	began  time.Time
}

const maxLines = 5000

// Write collects the job's output a line at a time.
func (j *Job) Write(p []byte) (int, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	text := j.part + string(p)
	parts := strings.Split(text, "\n")
	j.part = parts[len(parts)-1]
	j.lines = append(j.lines, parts[:len(parts)-1]...)
	j.count += len(parts) - 1
	if over := len(j.lines) - maxLines; over > 0 {
		j.lines = append([]string(nil), j.lines[over:]...)
	}
	return len(p), nil
}

// JobView is a snapshot of a job, as sent to the app.
type JobView struct {
	ID       int                `json:"id"`
	Kind     string             `json:"kind"`
	Title    string             `json:"title"`
	Command  string             `json:"command"`
	DriveID  string             `json:"driveId,omitempty"`
	State    string             `json:"state"`
	Started  int64              `json:"started"`
	Ended    int64              `json:"ended,omitempty"`
	Failures int                `json:"failures"`
	Warnings int                `json:"warnings"`
	Error    string             `json:"error,omitempty"`
	Summary  string             `json:"summary,omitempty"`
	Notify   string             `json:"notify,omitempty"`
	Progress *job.ProgressState `json:"progress"`
	Params   map[string]any     `json:"params,omitempty"`
	Needs    []string           `json:"needs,omitempty"` // drives a queued job waits for
	Lines    []string           `json:"lines"`
	// LineCount is how many lines the job has written in all; Lines are
	// the last of them.
	LineCount int `json:"lineCount"`
}

func (j *Job) view(tail int) JobView {
	j.mu.Lock()
	lines := j.lines
	if len(lines) > tail {
		lines = lines[len(lines)-tail:]
	}
	v := JobView{ID: j.ID, Kind: j.Kind, Title: j.Title, Command: j.Command, DriveID: j.DriveID, State: j.State, LineCount: j.count,
		Started: j.Started, Ended: j.Ended, Failures: j.Failures, Warnings: j.Warnings, Error: j.Error, Params: j.Params,
		Summary: j.Summary, Notify: j.Notify,
		Lines: append(make([]string, 0, len(lines)), lines...)} // never null: a queued job has no lines yet
	if j.State == "queued" {
		v.Needs = append([]string(nil), j.needs...)
	}
	running := j.State == "running"
	rep := j.report // set once in Start, never changed
	j.mu.Unlock()
	if running {
		// Outside mu: the report may be writing a line to this job now.
		v.Progress = rep.Now()
		v.Failures, v.Warnings = rep.Failures(), rep.Warnings()
	}
	return v
}

// Jobs runs and remembers jobs. A job that needs a drive another job is
// using waits in a queue, in order, and starts when the drive is free.
//
// Lock order: js.mu, then a job's mu. Nothing holds a job's mu while
// taking js.mu.
type Jobs struct {
	mu      sync.Mutex
	next    int
	all     []*Job
	pending []*Job // queued, oldest first
	// Notify, when set, is asked as a job ends (with the job's mu held,
	// its outcome filled in) whether the user wants telling: it returns
	// the notification's text, or "" for none.
	Notify func(*Job) string
	// OnDone, when set, is called after a job ends.
	OnDone func(*Job)
}

// Start runs fn as a new job on one drive ("" for none), queued behind
// any job already on that drive. command is the lumpy command that does
// the same thing, shown at the top of its log.
func (js *Jobs) Start(kind, title, command, driveID string, fn func(ctx context.Context, r *job.Report) error) *Job {
	return js.StartWhenFree(kind, title, command, []string{driveID}, fn)
}

// StartWhenFree runs fn as a new job once none of drives has a job
// running; until then it is queued and shows as such.
func (js *Jobs) StartWhenFree(kind, title, command string, drives []string, fn func(ctx context.Context, r *job.Report) error) *Job {
	var needs []string
	for _, d := range drives {
		if d != "" {
			needs = append(needs, d)
		}
	}
	js.mu.Lock()
	js.next++
	j := &Job{ID: js.next, Kind: kind, Title: title, Command: command, Started: time.Now().Unix(), needs: needs, fn: fn}
	if len(needs) > 0 {
		j.DriveID = needs[0]
	}
	if len(needs) > 1 {
		j.DriveID2 = needs[1]
	}
	j.report = job.NewReportTo(j)
	js.all = append(js.all, j)
	free := js.freeLocked(needs)
	if free {
		j.State = "running"
	} else {
		j.State = "queued"
		j.cancel = func() { js.dropQueued(j) }
		js.pending = append(js.pending, j)
	}
	js.mu.Unlock()
	if free {
		js.run(j)
	}
	return j
}

// freeLocked says whether none of the drives has a running job. Called
// with js.mu held.
func (js *Jobs) freeLocked(needs []string) bool {
	if len(needs) == 0 {
		return true
	}
	for _, o := range js.all {
		o.mu.Lock()
		running := o.State == "running"
		var held []string
		if running {
			held = append(append([]string(nil), o.needs...), o.DriveID, o.DriveID2)
		}
		o.mu.Unlock()
		if !running {
			continue
		}
		for _, h := range held {
			if h == "" {
				continue
			}
			for _, n := range needs {
				if n == h {
					return false
				}
			}
		}
	}
	return true
}

// run starts a job's work in the background.
func (js *Jobs) run(j *Job) {
	ctx, cancel := context.WithCancel(context.Background())
	j.mu.Lock()
	j.cancel = cancel
	j.began = time.Now()
	j.Started = j.began.Unix()
	j.mu.Unlock()
	go func() {
		err := j.fn(ctx, j.report)
		failures, warnings := j.report.Failures(), j.report.Warnings()
		j.mu.Lock()
		j.Ended = time.Now().Unix()
		j.Failures, j.Warnings = failures, warnings
		switch {
		case errors.Is(err, context.Canceled):
			j.State = "cancelled"
		case err != nil:
			j.State, j.Error = "error", err.Error()
		case j.Failures > 0:
			j.State = "failed"
		default:
			j.State = "ok"
		}
		j.Summary = summarise(j.State, time.Since(j.began), failures, warnings, j.Error)
		if js.Notify != nil {
			j.Notify = js.Notify(j)
		}
		j.mu.Unlock()
		cancel()
		if js.OnDone != nil {
			js.OnDone(j)
		}
		js.kick()
	}()
}

// summarise is the one line that says how a job went: "Done in 4m 12s ·
// 2 problems".
func summarise(state string, took time.Duration, failures, warnings int, errText string) string {
	var parts []string
	switch state {
	case "cancelled":
		parts = append(parts, "Cancelled after "+tookText(took))
	case "error":
		parts = append(parts, "Stopped after "+tookText(took))
	default:
		parts = append(parts, "Done in "+tookText(took))
	}
	if failures > 0 {
		parts = append(parts, fmt.Sprintf("%d problem%s", failures, plural(failures)))
	}
	if warnings > 0 {
		parts = append(parts, fmt.Sprintf("%d warning%s", warnings, plural(warnings)))
	}
	if errText != "" {
		parts = append(parts, errText)
	}
	return strings.Join(parts, " · ")
}

func tookText(d time.Duration) string {
	d = d.Round(time.Second)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm %02ds", int(d.Minutes()), int(d.Seconds())%60)
	default:
		return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
	}
}

// kick starts every queued job whose drives are now free, in order.
func (js *Jobs) kick() {
	js.mu.Lock()
	var start []*Job
	keep := js.pending[:0]
	for _, j := range js.pending {
		if js.freeLocked(j.needs) {
			j.mu.Lock()
			j.State = "running"
			j.mu.Unlock()
			start = append(start, j)
		} else {
			keep = append(keep, j)
		}
	}
	js.pending = keep
	js.mu.Unlock()
	for _, j := range start {
		js.run(j)
	}
}

// dropQueued takes a job out of the queue: cancelled before it began.
func (js *Jobs) dropQueued(j *Job) {
	js.mu.Lock()
	for i, p := range js.pending {
		if p == j {
			js.pending = append(js.pending[:i], js.pending[i+1:]...)
			break
		}
	}
	js.mu.Unlock()
	j.mu.Lock()
	if j.State == "queued" {
		j.State, j.Ended = "cancelled", time.Now().Unix()
	}
	j.mu.Unlock()
}

// AnyRunning reports whether any job is running now.
func (js *Jobs) AnyRunning() bool {
	js.mu.Lock()
	all := append([]*Job(nil), js.all...)
	js.mu.Unlock()
	for _, j := range all {
		j.mu.Lock()
		hit := j.State == "running"
		j.mu.Unlock()
		if hit {
			return true
		}
	}
	return false
}

// Running reports whether a job of this kind is running now.
func (js *Jobs) Running(kind string) bool {
	js.mu.Lock()
	all := append([]*Job(nil), js.all...)
	js.mu.Unlock()
	for _, j := range all {
		j.mu.Lock()
		hit := j.Kind == kind && j.State == "running"
		j.mu.Unlock()
		if hit {
			return true
		}
	}
	return false
}

// RunningPair reports whether a transfer between these two drives is
// running now.
func (js *Jobs) RunningPair(a, b string) bool {
	js.mu.Lock()
	all := append([]*Job(nil), js.all...)
	js.mu.Unlock()
	for _, j := range all {
		j.mu.Lock()
		hit := j.Kind == "transfer" && j.State == "running" && j.DriveID == a && j.DriveID2 == b
		j.mu.Unlock()
		if hit {
			return true
		}
	}
	return false
}

// Get returns a job by id.
func (js *Jobs) Get(id int) *Job {
	js.mu.Lock()
	defer js.mu.Unlock()
	for _, j := range js.all {
		if j.ID == id {
			return j
		}
	}
	return nil
}

// List returns every job, newest first.
func (js *Jobs) List(tail int) []JobView {
	js.mu.Lock()
	all := append([]*Job(nil), js.all...)
	js.mu.Unlock()
	out := make([]JobView, 0, len(all))
	for i := len(all) - 1; i >= 0; i-- {
		out = append(out, all[i].view(tail))
	}
	return out
}

// Busy reports whether a job is running on the drive.
func (js *Jobs) Busy(driveID string) bool {
	js.mu.Lock()
	all := append([]*Job(nil), js.all...)
	js.mu.Unlock()
	for _, j := range all {
		j.mu.Lock()
		busy := (j.DriveID == driveID || j.DriveID2 == driveID) && j.State == "running"
		j.mu.Unlock()
		if busy {
			return true
		}
	}
	return false
}

// Cancel stops a running job.
func (j *Job) Cancel() {
	j.mu.Lock()
	c := j.cancel
	j.mu.Unlock()
	if c != nil {
		c()
	}
}
