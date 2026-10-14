package ui

import (
	"context"
	"testing"
	"time"

	"github.com/lumpysheppard/lumpy/internal/job"
)

// A job writing output while the app polls its view must never deadlock:
// the report locks itself and then the job (through Write), so the view
// must not lock the job and then the report.
func TestJobViewWhileWriting(t *testing.T) {
	var js Jobs
	j := js.Start("test", "chatty", "", "", func(ctx context.Context, r *job.Report) error {
		for i := 0; i < 20000; i++ {
			r.Info("line %d", i)
			if i%1000 == 0 {
				r.Warn("warning %d", i)
			}
		}
		return nil
	})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			v := js.List(50)
			if v[0].State != "running" {
				return
			}
			_ = js.Busy("")
		}
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("polling a running job deadlocked")
	}
	v := j.view(10)
	if v.LineCount != 20020 || v.Warnings != 20 || v.State != "ok" {
		t.Fatalf("final view: %+v", v)
	}
}

// A job that needs a drive another job is using waits its turn and
// starts by itself when the drive is free; jobs on other drives run
// alongside; a queued job can be cancelled before it starts.
func TestJobsQueuePerDrive(t *testing.T) {
	var js Jobs
	state := func(j *Job) string { j.mu.Lock(); defer j.mu.Unlock(); return j.State }
	wait := func(j *Job, want string) {
		t.Helper()
		for i := 0; i < 500; i++ {
			if state(j) == want {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("job %d: state %q, want %q", j.ID, state(j), want)
	}
	hold := make(chan struct{})
	a := js.Start("verify", "a", "", "X", func(ctx context.Context, r *job.Report) error { <-hold; return nil })
	b := js.Start("hash", "b", "", "X", func(ctx context.Context, r *job.Report) error { return nil })
	c := js.Start("hash", "c", "", "Y", func(ctx context.Context, r *job.Report) error { return nil })
	d := js.StartWhenFree("transfer", "d", "", []string{"Y", "X"}, func(ctx context.Context, r *job.Report) error { return nil })
	e := js.Start("hash", "e", "", "X", func(ctx context.Context, r *job.Report) error { return nil })
	if state(a) != "running" || state(b) != "queued" || state(d) != "queued" || state(e) != "queued" {
		t.Fatalf("states: a %s b %s d %s e %s", state(a), state(b), state(d), state(e))
	}
	wait(c, "ok") // another drive: ran straight away
	if v := b.view(0); v.State != "queued" || len(v.Needs) != 1 || v.Needs[0] != "X" {
		t.Fatalf("queued view: %+v", v)
	}
	e.Cancel()
	if state(e) != "cancelled" || e.Ended == 0 {
		t.Fatalf("cancelled queued job: %s", state(e))
	}
	if js.Busy("X") != true || js.Busy("Y") != false {
		t.Fatal("busy: queued jobs must not count")
	}
	close(hold)
	wait(a, "ok")
	wait(b, "ok")
	wait(d, "ok")
	if b.Started < a.Ended || d.Started < b.Ended {
		t.Fatalf("order: a ended %d, b started %d ended %d, d started %d", a.Ended, b.Started, b.Ended, d.Started)
	}
	js.mu.Lock()
	n := len(js.pending)
	js.mu.Unlock()
	if n != 0 {
		t.Fatalf("%d jobs still pending", n)
	}
}
