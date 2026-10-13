// Package job runs Lumpy's commands: offload, verify and compare.
package job

import (
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// Report prints results as they happen and keeps count of problems.
type Report struct {
	mu       sync.Mutex
	out      io.Writer
	failures int
	warnings int
	prog     atomic.Pointer[Progress]
	terminal bool // draw the live progress line on stderr
	// Verbose adds a line per file (copied, verified, ...).
	Verbose bool
}

// NewReportTo writes only to w, with a line per file, and draws no
// terminal progress line; the app reads progress with Now instead.
func NewReportTo(w io.Writer) *Report { return &Report{out: w, Verbose: true} }

// Detail prints a per-file line when the report is verbose.
func (r *Report) Detail(format string, a ...any) {
	if r.Verbose {
		r.line(format, a...)
	}
}

// ProgressState is a snapshot of the phase a command is in.
type ProgressState struct {
	Label string `json:"label"`
	Done  int64  `json:"done"`
	Total int64  `json:"total"`
	Rate  int64  `json:"rate"` // bytes per second
	// Current is the file being worked on, when there is one.
	Current string `json:"current,omitempty"`
}

// Now returns the running phase's progress, or nil between phases.
func (r *Report) Now() *ProgressState {
	p := r.prog.Load()
	if p == nil {
		return nil
	}
	done := p.done.Load()
	st := &ProgressState{Label: p.label, Done: done, Total: p.total, Current: p.Current()}
	if secs := time.Since(p.start).Seconds(); secs > 0 {
		st.Rate = int64(float64(done) / secs)
	}
	return st
}

// NewReport writes to stdout and, when extra is not nil, to extra as well.
func NewReport(extra io.Writer) *Report {
	var w io.Writer = os.Stdout
	if extra != nil {
		w = io.MultiWriter(os.Stdout, extra)
	}
	return &Report{out: w, terminal: stderrIsTerminal()}
}

func (r *Report) line(format string, a ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	clearProgress()
	fmt.Fprintf(r.out, format+"\n", a...)
}

// Info prints a line.
func (r *Report) Info(format string, a ...any) { r.line(format, a...) }

// Fail records and prints a problem that makes the run fail.
func (r *Report) Fail(format string, a ...any) {
	r.mu.Lock()
	r.failures++
	r.mu.Unlock()
	r.line("FAIL  "+format, a...)
}

// Warn records and prints something worth knowing that does not fail the run.
func (r *Report) Warn(format string, a ...any) {
	r.mu.Lock()
	r.warnings++
	r.mu.Unlock()
	r.line("WARN  "+format, a...)
}

func (r *Report) addFailures(n int) {
	r.mu.Lock()
	r.failures += n
	r.mu.Unlock()
}

// Failures is the number of Fail calls so far.
func (r *Report) Failures() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.failures
}

// Warnings is the number of Warn calls so far.
func (r *Report) Warnings() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.warnings
}

// Bytes formats n as a decimal size, the way Finder shows drive sizes.
func Bytes(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "kMGTPE"[exp])
}

var progressShown atomic.Bool

func stderrIsTerminal() bool {
	fi, err := os.Stderr.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func clearProgress() {
	if progressShown.Swap(false) {
		fmt.Fprint(os.Stderr, "\r\033[K")
	}
}

// Progress shows a live byte counter on the terminal while a phase runs.
type Progress struct {
	label string
	total int64
	done  atomic.Int64
	start time.Time
	stop  chan struct{}
	wg    sync.WaitGroup
	mu    *sync.Mutex
	owner *Report
	cur   atomic.Value // string
}

// SetCurrent names the file being worked on.
func (p *Progress) SetCurrent(name string) { p.cur.Store(name) }

// Current returns the file being worked on, or "".
func (p *Progress) Current() string {
	s, _ := p.cur.Load().(string)
	return s
}

func (r *Report) startProgress(label string, total int64) *Progress {
	p := &Progress{label: label, total: total, start: time.Now(), stop: make(chan struct{}), mu: &r.mu, owner: r}
	r.prog.Store(p)
	if !r.terminal {
		return p
	}
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		t := time.NewTicker(500 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-p.stop:
				return
			case <-t.C:
				p.mu.Lock()
				fmt.Fprintf(os.Stderr, "\r\033[K%s", p.status())
				progressShown.Store(true)
				p.mu.Unlock()
			}
		}
	}()
	return p
}

func (p *Progress) status() string {
	done := p.done.Load()
	secs := time.Since(p.start).Seconds()
	rate := int64(0)
	if secs > 0 {
		rate = int64(float64(done) / secs)
	}
	pct := 100.0
	if p.total > 0 {
		pct = float64(done) * 100 / float64(p.total)
	}
	s := fmt.Sprintf("%s  %s / %s  (%.0f%%)  %s/s", p.label, Bytes(done), Bytes(p.total), pct, Bytes(rate))
	if rate > 0 && done < p.total {
		left := time.Duration(float64(p.total-done)/float64(rate)) * time.Second
		s += fmt.Sprintf("  ~%s left", left.Round(time.Second))
	}
	return s
}

// Add counts n more bytes processed.
func (p *Progress) Add(n int) { p.done.Add(int64(n)) }

// Finish stops the counter and returns how long the phase took.
func (p *Progress) Finish() time.Duration {
	if p.owner != nil {
		p.owner.prog.CompareAndSwap(p, nil)
	}
	close(p.stop)
	p.wg.Wait()
	p.mu.Lock()
	clearProgress()
	p.mu.Unlock()
	return time.Since(p.start)
}
