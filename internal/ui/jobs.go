package ui

import (
	"context"
	"errors"
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

	mu     sync.Mutex
	lines  []string
	count  int // lines written so far, including ones dropped from lines
	part   string
	report *job.Report
	cancel context.CancelFunc
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
	Progress *job.ProgressState `json:"progress"`
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
		Started: j.Started, Ended: j.Ended, Failures: j.Failures, Warnings: j.Warnings, Error: j.Error,
		Lines: append([]string(nil), lines...)}
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

// Jobs runs and remembers jobs.
type Jobs struct {
	mu   sync.Mutex
	next int
	all  []*Job
	// OnDone, when set, is called after a job ends.
	OnDone func(*Job)
}

// Start runs fn in the background as a new job. command is the lumpy
// command that does the same thing, shown at the top of its log.
func (js *Jobs) Start(kind, title, command, driveID string, fn func(ctx context.Context, r *job.Report) error) *Job {
	ctx, cancel := context.WithCancel(context.Background())
	js.mu.Lock()
	js.next++
	j := &Job{ID: js.next, Kind: kind, Title: title, Command: command, DriveID: driveID, State: "running", Started: time.Now().Unix(), cancel: cancel}
	j.report = job.NewReportTo(j)
	js.all = append(js.all, j)
	js.mu.Unlock()

	go func() {
		err := fn(ctx, j.report)
		failures, warnings := j.report.Failures(), j.report.Warnings()
		j.mu.Lock()
		defer j.mu.Unlock()
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
		cancel()
		if js.OnDone != nil {
			j.mu.Unlock()
			js.OnDone(j)
			j.mu.Lock()
		}
	}()
	return j
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

// mark records a second drive the job occupies.
func (js *Jobs) mark(j *Job, driveID string) {
	j.mu.Lock()
	j.DriveID2 = driveID
	j.mu.Unlock()
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
func (j *Job) Cancel() { j.cancel() }
