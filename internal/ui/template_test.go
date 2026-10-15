package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lumpysheppard/lumpy/internal/catalog"
)

func TestProjectFolderFromTemplate(t *testing.T) {
	s, ts, vols := setup(t)
	tpl := t.TempDir()
	files := filepath.Join(tpl, "Video Editing Template.pht", "Files")
	os.MkdirAll(filepath.Join(files, "01_Original footage", "Online"), 0o755)
	os.MkdirAll(filepath.Join(files, "09_OUTPUT"), 0o755)
	os.WriteFile(filepath.Join(files, "01_Original footage", "Description.txt"), []byte("[Project Name] for [Client]"), 0o644)
	os.WriteFile(filepath.Join(files, "09_OUTPUT", "leader.mov"), []byte("mov"), 0o644)
	t.Setenv("LUMPY_TEMPLATES", tpl)
	os.MkdirAll(filepath.Join(vols, "RAID", "01_PROJECTS"), 0o755)

	var list struct {
		Settings  TemplateSettings
		Templates []TemplateView
		Today     string
	}
	call(t, s, ts, "GET", "/api/templates", "", &list)
	if len(list.Templates) != 1 || list.Templates[0].Name != "Video Editing Template" || len(list.Templates[0].Folders) != 2 || list.Templates[0].Files != 2 || list.Settings.NextNumber != 1 || list.Settings.Pattern == "" {
		t.Fatalf("list: %+v", list)
	}
	dest := filepath.Join(vols, "RAID", "01_PROJECTS")
	body := `{"template":"` + strings.ReplaceAll(list.Templates[0].Path, `\`, `\\`) + `","dest":"` + strings.ReplaceAll(dest, `\`, `\\`) + `","params":{"Project Number":"16","Client":"BS","Project Name":"ARIA","Date":"2026-10-14","Editor":"studioshepp"},"addProject":true,"target":2}`
	var made struct {
		Job    int
		Folder string
		Name   string
	}
	if code := call(t, s, ts, "POST", "/api/templates/create", body, &made); code != 200 {
		t.Fatalf("create: %d", code)
	}
	if made.Name != "16_BS_ARIA_2026-10-14_studioshepp" {
		t.Errorf("name: %q", made.Name)
	}
	for _, j := range waitJobs(t, s, ts) {
		if j.State != "ok" {
			t.Errorf("job %d: %s %s", j.ID, j.State, j.Error)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(made.Folder, "01_Original footage", "Description.txt")); string(b) != "ARIA for BS" {
		t.Errorf("description: %q", b)
	}
	var ps []catalog.Project
	call(t, s, ts, "GET", "/api/projects", "", &ps)
	if len(ps) != 1 || ps[0].Name != "ARIA" || ps[0].Target != 2 || len(ps[0].Sources) != 1 || ps[0].Sources[0].Folder != "01_PROJECTS/16_BS_ARIA_2026-10-14_studioshepp" {
		t.Fatalf("project: %+v", ps)
	}
	// The drive was logged with the new files, and next time the number
	// steps on and the choices are remembered.
	var st struct{ Drives []DriveView }
	call(t, s, ts, "GET", "/api/state", "", &st)
	if len(st.Drives) != 1 || st.Drives[0].Files != 2 {
		t.Errorf("drive: %+v", st.Drives)
	}
	call(t, s, ts, "GET", "/api/templates", "", &list)
	if list.Settings.NextNumber != 17 || list.Settings.Client != "BS" || list.Settings.Dest != dest {
		t.Errorf("remembered: %+v", list.Settings)
	}
	// The same name again is refused before anything is written.
	if code := call(t, s, ts, "POST", "/api/templates/create", body, nil); code != 400 {
		t.Errorf("duplicate: %d", code)
	}
}
