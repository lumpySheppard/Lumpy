package ui

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lumpysheppard/lumpy/internal/job"
)

// A finished check posts one line to the webhook and carries a notify
// text for the app; with "problems only" a clean job stays quiet.
func TestWebhookAndNotifyOnJobDone(t *testing.T) {
	s, ts, vols := setup(t)
	os.MkdirAll(filepath.Join(vols, "A"), 0o755)
	os.WriteFile(filepath.Join(vols, "A", "clip.mov"), []byte("footage"), 0o644)

	var mu sync.Mutex
	var got []string
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var m map[string]string
		json.Unmarshal(body, &m)
		mu.Lock()
		got = append(got, m["text"])
		mu.Unlock()
	}))
	defer hook.Close()

	if code := call(t, s, ts, "PUT", "/api/prefs", `{"webhook":"`+hook.URL+`"}`, nil); code != 200 {
		t.Fatalf("prefs: %d", code)
	}
	job.AgentRunWith(context.Background(), s.db, job.NewReportTo(io.Discard), func(string, string) {}, nil)
	var st struct{ Drives []DriveView }
	call(t, s, ts, "GET", "/api/state", "", &st)
	if len(st.Drives) != 1 {
		t.Fatalf("drives: %+v", st.Drives)
	}
	id := st.Drives[0].ID

	// A rescan that went fine is not worth a notification.
	call(t, s, ts, "POST", "/api/drives/"+id+"/rescan", "", nil)
	waitIdle(t, s)
	// A check is.
	call(t, s, ts, "POST", "/api/drives/"+id+"/verify", "", nil)
	waitIdle(t, s)

	lines := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), got...)
	}
	deadline := time.Now().Add(5 * time.Second)
	for len(lines()) == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if l := lines(); len(l) != 1 || !strings.HasPrefix(l[0], "Check ") || !strings.Contains(l[0], " on test: Done in ") {
		t.Fatalf("webhook got %q", l)
	}
	jobs := s.jobs.List(0)
	var verify, rescan *JobView
	for i := range jobs {
		switch jobs[i].Kind {
		case "verify":
			verify = &jobs[i]
		case "rescan":
			rescan = &jobs[i]
		}
	}
	if verify == nil || verify.Notify == "" || !strings.HasPrefix(verify.Summary, "Done in ") {
		t.Fatalf("verify job: %+v", verify)
	}
	if rescan == nil || rescan.Notify != "" || rescan.Summary == "" {
		t.Fatalf("rescan job: %+v", rescan)
	}

	// Problems only: a clean check posts nothing and carries no text.
	call(t, s, ts, "PUT", "/api/prefs", `{"notifyProblemsOnly":true}`, nil)
	call(t, s, ts, "POST", "/api/drives/"+id+"/verify", "", nil)
	waitIdle(t, s)
	time.Sleep(100 * time.Millisecond)
	if l := lines(); len(l) != 1 {
		t.Fatalf("problems only: webhook got %q", l)
	}
	if v := s.jobs.List(0)[0]; v.Kind != "verify" || v.Notify != "" {
		t.Fatalf("problems only: %+v", v)
	}

	// The test button refuses a non-URL and posts a test line.
	if code := call(t, s, ts, "POST", "/api/notify/test", `{"webhook":"nope"}`, nil); code != 400 {
		t.Fatalf("bad webhook: %d", code)
	}
	if code := call(t, s, ts, "POST", "/api/notify/test", `{"webhook":"`+hook.URL+`"}`, nil); code != 200 {
		t.Fatalf("test post: %d", code)
	}
	if l := lines(); len(l) != 2 || !strings.Contains(l[1], "this is a test") {
		t.Fatalf("test line: %q", l)
	}
}

func waitIdle(t *testing.T, s *Server) {
	t.Helper()
	for i := 0; i < 500; i++ {
		if !s.jobs.AnyRunning() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("jobs still running")
}
