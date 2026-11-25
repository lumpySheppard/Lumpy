package job

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/lumpysheppard/lumpy/internal/fsx"
)

// A project folder template is a folder tree copied to start a job's
// folders: the Files folder inside a Post Haste .pht bundle, or any
// folder. Names (and small text files) may carry tokens such as
// [Project Name] or {Client}, filled in from the parameters.

// TemplateParams are the values for the tokens, keyed as Post Haste
// names them: Project Number, Client, Project Name, Date, Editor.
type TemplateParams map[string]string

// TemplateFiles finds the folder tree a template path stands for: a
// .pht bundle's Files folder, or the folder itself.
func TemplateFiles(path string) (string, error) {
	st, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !st.IsDir() {
		return "", fmt.Errorf("%s is not a folder or a Post Haste template", path)
	}
	if files := filepath.Join(path, "Files"); strings.EqualFold(filepath.Ext(path), ".pht") {
		if st, err := os.Stat(files); err == nil && st.IsDir() {
			return files, nil
		}
		return "", fmt.Errorf("%s has no Files folder inside", filepath.Base(path))
	}
	return path, nil
}

// Fill replaces every token in s: [Key], {Key}, %Key% or <Key>, any
// case, with or without the spaces in the key.
func Fill(s string, params TemplateParams) string {
	for key, val := range params {
		compact := strings.ReplaceAll(key, " ", "")
		for _, k := range []string{key, compact} {
			re := regexp.MustCompile(`(?i)[\[{%<]\s*` + regexp.QuoteMeta(k) + `\s*[\]}%>]`)
			s = re.ReplaceAllLiteralString(s, val)
		}
	}
	return s
}

// textFile says whether a template file is text that tokens may sit in.
func textFile(name string, size int64) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".txt", ".md", ".rtf", ".csv", ".json", ".xml", ".plist":
		return size <= 256<<10
	}
	return false
}

type templateEntry struct {
	rel  string
	dir  bool
	size int64
	mode os.FileMode
}

func listTemplate(src string) ([]templateEntry, error) {
	var out []templateEntry
	err := filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == src {
			return nil
		}
		name := d.Name()
		if name == ".DS_Store" || strings.HasPrefix(name, "._") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(src, p)
		info, err := d.Info()
		if err != nil {
			return err
		}
		out = append(out, templateEntry{rel: rel, dir: d.IsDir(), size: info.Size(), mode: info.Mode()})
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].rel < out[j].rel })
	return out, err
}

// CreateFromTemplate copies the template tree src into the new folder
// dest (which must not exist yet), filling tokens in names and in
// small text files. It returns how many files it wrote and their size.
func CreateFromTemplate(ctx context.Context, src, dest string, params TemplateParams, r *Report) (int, int64, error) {
	if _, err := os.Stat(dest); err == nil {
		return 0, 0, fmt.Errorf("%s already exists", dest)
	}
	entries, err := listTemplate(src)
	if err != nil {
		return 0, 0, fmt.Errorf("reading the template: %w", err)
	}
	files := 0
	for _, e := range entries {
		if !e.dir {
			files++
		}
	}
	r.Info("Creating %s from %s (%d files)", dest, filepath.Base(filepath.Dir(src)), files)
	prog := r.startProgress("Creating", int64(files+1))
	defer prog.Finish()
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return 0, 0, err
	}
	prog.Add(1)
	var bytes int64
	n := 0
	for _, e := range entries {
		if ctx.Err() != nil {
			return n, bytes, ctx.Err()
		}
		parts := strings.Split(e.rel, string(filepath.Separator))
		for i, p := range parts {
			parts[i] = Fill(p, params)
		}
		target := filepath.Join(dest, filepath.Join(parts...))
		if e.dir {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return n, bytes, err
			}
			r.Detail("folder   %s", filepath.Join(parts...))
			continue
		}
		prog.SetCurrent(e.rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return n, bytes, err
		}
		srcPath := filepath.Join(src, e.rel)
		if textFile(e.rel, e.size) {
			data, err := os.ReadFile(srcPath)
			if err != nil {
				return n, bytes, err
			}
			filled := Fill(string(data), params)
			if err := os.WriteFile(target, []byte(filled), e.mode.Perm()|0o600); err != nil {
				return n, bytes, err
			}
			bytes += int64(len(filled))
		} else {
			in, err := fsx.OpenRead(srcPath)
			if err != nil {
				return n, bytes, err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, e.mode.Perm()|0o600)
			if err != nil {
				in.Close()
				return n, bytes, err
			}
			w, err := io.Copy(out, in)
			in.Close()
			if cerr := out.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				return n, bytes, fmt.Errorf("copying %s: %w", e.rel, err)
			}
			bytes += w
		}
		r.Detail("file     %s", filepath.Join(parts...))
		n++
		prog.Add(1)
	}
	r.Info("Created %s: %d files", dest, n)
	return n, bytes, nil
}
