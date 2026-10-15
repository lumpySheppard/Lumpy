package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/lumpysheppard/lumpy/internal/agent"
)

// Jobs the user is told about when they end however they went; others
// (rescans, metadata, updates, new folders) only when something went
// wrong.
var notifyKinds = map[string]bool{"offload": true, "transfer": true, "verify": true, "hash": true, "archive": true}

// wantNotify says whether a job's ending is worth telling the user
// about, by the preferences. Called with the job's mu held.
func (s *Server) wantNotify(j *Job) bool {
	p := s.db.Prefs()
	bad := j.Failures > 0 || j.State == "error"
	if !notifyKinds[j.Kind] && !bad {
		return false
	}
	return !p.NotifyProblemsOnly || bad
}

// notifyText is the notification for a job that has ended: "Offload A004
// to 2 drives: Done in 4m 12s". Called with the job's mu held.
func (s *Server) notifyText(j *Job) string {
	if !s.db.Prefs().Notify || !s.wantNotify(j) {
		return ""
	}
	return j.Summary
}

// notifyDone tells the user a job has ended: a macOS notification when
// the engine runs on its own (Lumpy.app posts its own, from the job's
// Notify text), and a line to the webhook.
func (s *Server) notifyDone(j *Job) {
	j.mu.Lock()
	text, want := j.Notify, s.wantNotify(j)
	title, summary := j.Title, j.Summary
	j.mu.Unlock()
	if text != "" && !s.App {
		agent.Notify(title, text)
	}
	if hook := strings.TrimSpace(s.db.Prefs().Webhook); hook != "" && want {
		line := fmt.Sprintf("%s on %s: %s", title, s.meta.Hostname, summary)
		if err := postWebhook(context.Background(), hook, line); err != nil {
			fmt.Fprintf(os.Stderr, "webhook: %v\n", err)
		}
	}
}

// postWebhook sends one line to a Slack-style incoming webhook. The body
// carries the text under both the key Slack and Teams read and the one
// Discord reads.
func postWebhook(ctx context.Context, url, text string) error {
	body, _ := json.Marshal(map[string]string{"text": text, "content": text})
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("%s answered %s", url, res.Status)
	}
	return nil
}

// notifyTest posts a test line to a webhook, before the user relies on it.
func (s *Server) notifyTest(w http.ResponseWriter, r *http.Request) (any, error) {
	var body struct {
		Webhook string `json:"webhook"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, badRequest("bad request: %v", err)
	}
	hook := strings.TrimSpace(body.Webhook)
	if !strings.HasPrefix(hook, "https://") && !strings.HasPrefix(hook, "http://") {
		return nil, badRequest("the webhook must be an https:// URL")
	}
	if err := postWebhook(r.Context(), hook, fmt.Sprintf("Lumpy on %s: this is a test. Job notifications will arrive here.", s.meta.Hostname)); err != nil {
		return nil, badRequest("couldn't post to the webhook: %v", err)
	}
	return map[string]bool{"ok": true}, nil
}
