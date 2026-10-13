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
