package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lumpysheppard/lumpy/internal/job"
	"github.com/lumpysheppard/lumpy/internal/volume"
)

// New project folders come from a template: the Post Haste templates
// on this Mac, or any folder. The last choices are remembered.

// TemplateSettings is what the New project folder sheet remembers.
type TemplateSettings struct {
	Template   string `json:"template"`   // path of the .pht bundle or folder
	Dest       string `json:"dest"`       // where project folders go
	Pattern    string `json:"pattern"`    // folder name, with tokens
	NextNumber int    `json:"nextNumber"` // the next project number
	Client     string `json:"client"`
	Editor     string `json:"editor"`
	AddProject bool   `json:"addProject"` // make a Lumpy project of it
	Target     int    `json:"target"`     // copies wanted for that project
}

const defaultPattern = "[Project Number]_[Client]_[Project Name]_[Date]_[Editor]"

func (s *Server) templateSettings() TemplateSettings {
	ts := TemplateSettings{Pattern: defaultPattern, NextNumber: 1, AddProject: true, Target: 2}
	if raw := s.db.Setting("project.template", ""); raw != "" {
		json.Unmarshal([]byte(raw), &ts)
	}
	if ts.Pattern == "" {
		ts.Pattern = defaultPattern
	}
	if ts.Target < 1 {
		ts.Target = 2
	}
	if ts.Editor == "" {
		if u, err := user.Current(); err == nil {
			ts.Editor = u.Username
		}
	}
	return ts
}

// TemplateView is one template the sheet can pick.
type TemplateView struct {
	Name    string   `json:"name"`
	Path    string   `json:"path"`
	Folders []string `json:"folders"` // its top-level folders, for the preview
	Files   int      `json:"files"`
}

// postHasteDir is where Post Haste keeps its templates; LUMPY_TEMPLATES
// points somewhere else, for tests and demos.
func postHasteDir() string {
	if d := os.Getenv("LUMPY_TEMPLATES"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Library", "Application Support", "Digital Rebellion", "Post Haste", "Templates")
}

func templateView(path string) (TemplateView, error) {
	files, err := job.TemplateFiles(path)
	if err != nil {
		return TemplateView{}, err
	}
	v := TemplateView{Name: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), Path: path, Folders: []string{}}
	entries, err := os.ReadDir(files)
	if err != nil {
		return v, err
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if e.IsDir() {
			v.Folders = append(v.Folders, e.Name())
		}
	}
	filepath.WalkDir(files, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && !strings.HasPrefix(d.Name(), ".") {
			v.Files++
		}
		return nil
	})
	return v, nil
}

// listTemplates returns the Post Haste templates plus the one chosen
// by hand, if it is elsewhere.
func (s *Server) listTemplates(w http.ResponseWriter, r *http.Request) (any, error) {
	ts := s.templateSettings()
	seen := map[string]bool{}
	out := []TemplateView{}
	add := func(path string) {
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		if v, err := templateView(path); err == nil {
			out = append(out, v)
		}
	}
	if dir := postHasteDir(); dir != "" {
		if entries, err := os.ReadDir(dir); err == nil {
			var names []string
			for _, e := range entries {
				if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
					names = append(names, e.Name())
				}
			}
			sort.Strings(names)
			for _, n := range names {
				add(filepath.Join(dir, n))
			}
		}
	}
	add(ts.Template)
	return map[string]any{
		"settings":  ts,
		"templates": out,
		"today":     time.Now().Format("2006-01-02"),
		"postHaste": postHasteDir(),
	}, nil
}

// createFromTemplate makes the folder, then (when asked) a Lumpy project
// of it and a rescan of the drive it is on, as one job.
func (s *Server) createFromTemplate(w http.ResponseWriter, r *http.Request) (any, error) {
	var body struct {
		Template   string            `json:"template"`
		Dest       string            `json:"dest"`
		Pattern    string            `json:"pattern"`
		Params     map[string]string `json:"params"`
		AddProject bool              `json:"addProject"`
		Target     int               `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, badRequest("bad request: %v", err)
	}
	src, err := job.TemplateFiles(strings.TrimSpace(body.Template))
	if err != nil {
		return nil, badRequest("%v", err)
	}
	dest := strings.TrimSpace(body.Dest)
	if st, err := os.Stat(dest); err != nil || !st.IsDir() {
		return nil, badRequest("choose the folder the project folder goes in")
	}
	params := job.TemplateParams{}
	for k, v := range body.Params {
		params[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	if params["Project Name"] == "" {
		return nil, badRequest("give the project a name")
	}
	pattern := strings.TrimSpace(body.Pattern)
	if pattern == "" {
		pattern = defaultPattern
	}
	name := strings.Trim(strings.NewReplacer("/", "-", ":", "-", "\\", "-").Replace(job.Fill(pattern, params)), " .")
	if name == "" {
		return nil, badRequest("the folder name came out empty")
	}
	full := filepath.Join(dest, name)
	if _, err := os.Stat(full); err == nil {
		return nil, badRequest("%s already exists in %s", name, dest)
	}
	target := body.Target
	if target < 1 {
		target = 2
	}
	// Remember for next time, with the number stepped on.
	ts := s.templateSettings()
	ts.Template, ts.Dest, ts.Pattern, ts.Client, ts.Editor, ts.AddProject, ts.Target = body.Template, dest, pattern, params["Client"], params["Editor"], body.AddProject, target
	if n, err := fmt.Sscanf(params["Project Number"], "%d", new(int)); err == nil && n == 1 {
		var num int
		fmt.Sscanf(params["Project Number"], "%d", &num)
		ts.NextNumber = num + 1
	}
	if raw, err := json.Marshal(ts); err == nil {
		s.db.SetSetting("project.template", string(raw))
	}
	add := body.AddProject
	j := s.jobs.Start("template", "New project folder "+name, "lumpy project new "+shellQuote(name), "", func(ctx context.Context, rep *job.Report) error {
		if _, _, err := job.CreateFromTemplate(ctx, src, full, params, rep); err != nil {
			return err
		}
		if !add {
			return nil
		}
		v, err := volume.Of(full)
		if err != nil {
			rep.Warn("not adding a project: %v", err)
			return nil
		}
		rel, err := filepath.Rel(v.Mount, full)
		if err != nil {
			return err
		}
		// Log the drive (which also indexes the new folder), then make
		// the project of that folder.
		if _, err := job.CatalogDrive(ctx, s.db, v, rep); err != nil {
			return fmt.Errorf("logging %s: %w", v.Name, err)
		}
		p, err := s.db.CreateProject(params["Project Name"], target)
		if err != nil {
			return err
		}
		if err := s.db.AddSource(p.ID, v.ID, filepath.ToSlash(rel)); err != nil {
			return err
		}
		rep.Info("Project %s made of %s on %s", p.Name, rel, v.Name)
		return nil
	})
	return map[string]any{"job": j.ID, "folder": full, "name": name}, nil
}
