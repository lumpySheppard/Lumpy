// Package update checks GitHub Releases for a newer Lumpy and installs
// it in place: the whole Lumpy.app when running from the app, or the bare
// `lumpy` binary otherwise.
package update

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Repo is the GitHub repository releases are read from.
const Repo = "lumpySheppard/Lumpy"

// BaseURL is the GitHub API; tests point it elsewhere, as does the
// LUMPY_UPDATE_API environment variable.
var BaseURL = "https://api.github.com"

func init() {
	if v := os.Getenv("LUMPY_UPDATE_API"); v != "" {
		BaseURL = strings.TrimRight(v, "/")
	}
}

const (
	appAsset = "Lumpy-macOS.zip"
	binAsset = "lumpy-macos-universal.zip"
	sumsFile = "SHA256SUMS"
)

// Release is a published version.
type Release struct {
	Version     string    `json:"version"` // without the v
	Tag         string    `json:"tag"`
	Notes       string    `json:"notes"`
	URL         string    `json:"url"`
	PublishedAt time.Time `json:"publishedAt"`
	Assets      []Asset   `json:"-"`
}

// Asset is a file attached to a release.
type Asset struct {
	Name string
	URL  string // API URL; downloads with Accept: application/octet-stream
	Size int64
}

func (r *Release) asset(name string) *Asset {
	for i := range r.Assets {
		if r.Assets[i].Name == name {
			return &r.Assets[i]
		}
	}
	return nil
}

// Check fetches the latest release. token is needed while the repository
// is private (a fine-grained token with read access to its contents).
func Check(ctx context.Context, token string) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, BaseURL+"/repos/"+Repo+"/releases/latest", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "lumpy-update")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		if token == "" {
			return nil, errors.New("no version has been released yet (or the repository is private, in which case a GitHub token with read access to it goes in Settings)")
		}
		return nil, errors.New("no version has been released yet, or the token cannot see the repository")
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, fmt.Errorf("GitHub refused the token (HTTP %d)", res.StatusCode)
	default:
		return nil, fmt.Errorf("GitHub: HTTP %d", res.StatusCode)
	}
	var body struct {
		TagName     string    `json:"tag_name"`
		Name        string    `json:"name"`
		Body        string    `json:"body"`
		HTMLURL     string    `json:"html_url"`
		PublishedAt time.Time `json:"published_at"`
		Draft       bool      `json:"draft"`
		Assets      []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
			Size int64  `json:"size"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&body); err != nil {
		return nil, fmt.Errorf("reading the release: %w", err)
	}
	rel := &Release{Version: strings.TrimPrefix(body.TagName, "v"), Tag: body.TagName, Notes: body.Body, URL: body.HTMLURL, PublishedAt: body.PublishedAt}
	for _, a := range body.Assets {
		rel.Assets = append(rel.Assets, Asset{Name: a.Name, URL: a.URL, Size: a.Size})
	}
	return rel, nil
}

// Newer reports whether version a is newer than b ("0.6.0" > "0.5.0").
// Anything unparseable counts as older, so a dev build never updates to
// a broken tag.
func Newer(a, b string) bool {
	pa, pb := parts(a), parts(b)
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return false
}

func parts(v string) [3]int {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	for i, p := range strings.SplitN(v, ".", 3) {
		n, err := strconv.Atoi(p)
		if err != nil {
			return [3]int{}
		}
		out[i] = n
	}
	return out
}

// Target is what gets replaced.
type Target struct {
	// Bundle is the Lumpy.app folder when the running binary is inside
	// one; then the whole app is replaced.
	Bundle string
	// Exe is the running binary, replaced on its own when there is no
	// bundle.
	Exe string
}

// Describe says what will be replaced, for messages.
func (t Target) Describe() string {
	if t.Bundle != "" {
		return t.Bundle
	}
	return t.Exe
}

// Detect finds the running binary and the app bundle around it, if any.
func Detect() (Target, error) {
	exe, err := os.Executable()
	if err != nil {
		return Target{}, err
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	t := Target{Exe: exe}
	// …/Lumpy.app/Contents/Resources/lumpy
	if res := filepath.Dir(exe); filepath.Base(res) == "Resources" {
		if contents := filepath.Dir(res); filepath.Base(contents) == "Contents" {
			if app := filepath.Dir(contents); strings.HasSuffix(app, ".app") {
				t.Bundle = app
			}
		}
	}
	return t, nil
}

// Install downloads the release for the target, checks it against the
// release's SHA256SUMS, and swaps it into place. progress gets a line
// per step. The running program keeps running the old code: restart it
// to finish.
func Install(ctx context.Context, rel *Release, token string, t Target, progress func(string)) error {
	if progress == nil {
		progress = func(string) {}
	}
	name := binAsset
	if t.Bundle != "" {
		name = appAsset
	}
	asset := rel.asset(name)
	if asset == nil {
		return fmt.Errorf("release %s has no %s", rel.Tag, name)
	}
	sums := rel.asset(sumsFile)
	if sums == nil {
		return fmt.Errorf("release %s has no %s, so the download cannot be checked", rel.Tag, sumsFile)
	}
	tmp, err := os.MkdirTemp(filepath.Dir(t.Describe()), ".lumpy-update-")
	if err != nil {
		// No room to write next to the app (an admin-owned /Applications):
		// stage in the temp folder instead and hope the rename works.
		tmp, err = os.MkdirTemp("", "lumpy-update-")
		if err != nil {
			return err
		}
	}
	defer os.RemoveAll(tmp)

	progress(fmt.Sprintf("Downloading %s (%s)…", name, mb(asset.Size)))
	zipPath := filepath.Join(tmp, name)
	if err := download(ctx, asset.URL, token, zipPath, asset.Size, progress); err != nil {
		return fmt.Errorf("download: %w", err)
	}
	progress("Checking the download against " + sumsFile)
	sumsPath := filepath.Join(tmp, sumsFile)
	if err := download(ctx, sums.URL, token, sumsPath, 0, nil); err != nil {
		return fmt.Errorf("download %s: %w", sumsFile, err)
	}
	want, err := expectedSum(sumsPath, name)
	if err != nil {
		return err
	}
	got, err := fileSum(zipPath)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("%s does not match its published checksum (got %s, want %s); not installed", name, got[:12], want[:12])
	}
	progress("Checksum OK " + got[:12] + "…")

	if t.Bundle != "" {
		return installBundle(zipPath, tmp, t.Bundle, progress)
	}
	return installBinary(zipPath, tmp, t.Exe, progress)
}

func installBundle(zipPath, tmp, bundle string, progress func(string)) error {
	progress("Unpacking…")
	stage := filepath.Join(tmp, "unpacked")
	if err := unzip(zipPath, stage); err != nil {
		return fmt.Errorf("unpack: %w", err)
	}
	// The zip holds Lumpy.app at its top level (ditto --keepParent).
	newApp := filepath.Join(stage, filepath.Base(bundle))
	if _, err := os.Stat(filepath.Join(newApp, "Contents", "Resources", "lumpy")); err != nil {
		return fmt.Errorf("the download does not contain %s", filepath.Base(bundle))
	}
	if runtime.GOOS == "darwin" {
		// Files we wrote ourselves carry no quarantine flag, but be sure.
		exec.Command("xattr", "-dr", "com.apple.quarantine", newApp).Run()
		out, err := exec.Command(filepath.Join(newApp, "Contents", "Resources", "lumpy"), "version").Output()
		if err != nil {
			return fmt.Errorf("the downloaded app does not run: %v", err)
		}
		progress("Downloaded app reports " + strings.TrimSpace(string(out)))
	}
	progress("Replacing " + bundle)
	old := bundle + ".old-" + strconv.FormatInt(time.Now().Unix(), 10)
	if err := os.Rename(bundle, old); err != nil {
		return fmt.Errorf("cannot move the current app aside: %w (is %s writable?)", err, filepath.Dir(bundle))
	}
	if err := os.Rename(newApp, bundle); err != nil {
		// Different volume: copy instead, then put things back on failure.
		if cerr := copyTree(newApp, bundle); cerr != nil {
			os.RemoveAll(bundle)
			os.Rename(old, bundle)
			return fmt.Errorf("cannot install the new app: %w", cerr)
		}
	}
	if err := os.RemoveAll(old); err != nil {
		progress("Note: the previous version is still at " + old)
	}
	return nil
}

func installBinary(zipPath, tmp, exe string, progress func(string)) error {
	progress("Unpacking…")
	stage := filepath.Join(tmp, "unpacked")
	if err := unzip(zipPath, stage); err != nil {
		return fmt.Errorf("unpack: %w", err)
	}
	src := filepath.Join(stage, "lumpy")
	if _, err := os.Stat(src); err != nil {
		return errors.New("the download does not contain the lumpy binary")
	}
	if runtime.GOOS == "darwin" {
		exec.Command("xattr", "-d", "com.apple.quarantine", src).Run()
		if out, err := exec.Command(src, "version").Output(); err != nil {
			return fmt.Errorf("the downloaded binary does not run: %v", err)
		} else {
			progress("Downloaded binary reports " + strings.TrimSpace(string(out)))
		}
	}
	progress("Replacing " + exe)
	next := exe + ".new"
	if err := copyFile(src, next, 0o755); err != nil {
		return fmt.Errorf("cannot write next to the current binary: %w", err)
	}
	if err := os.Rename(next, exe); err != nil {
		os.Remove(next)
		return fmt.Errorf("cannot replace the current binary: %w", err)
	}
	return nil
}

// download fetches a release asset, following GitHub's redirect to its
// storage. The token is dropped on the redirect, as Go does across hosts.
func download(ctx context.Context, url, token, dest string, size int64, progress func(string)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", "lumpy-update")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := (&http.Client{Timeout: 15 * time.Minute}).Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", res.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	var done int64
	last := time.Now()
	buf := make([]byte, 256<<10)
	for {
		n, rerr := res.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				f.Close()
				return werr
			}
			done += int64(n)
			if progress != nil && size > 0 && time.Since(last) > 700*time.Millisecond {
				progress(fmt.Sprintf("  %s of %s", mb(done), mb(size)))
				last = time.Now()
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			return rerr
		}
	}
	return f.Close()
}

func expectedSum(sumsPath, name string) (string, error) {
	b, err := os.ReadFile(sumsPath)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name {
			return strings.ToLower(f[0]), nil
		}
	}
	return "", fmt.Errorf("%s has no entry for %s", sumsFile, name)
}

func fileSum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// unzip extracts an archive, keeping modes and symlinks, refusing paths
// that would land outside dest.
func unzip(path, dest string) error {
	r, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		p := filepath.Join(dest, filepath.FromSlash(f.Name))
		if !strings.HasPrefix(p, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("unsafe path in archive: %s", f.Name)
		}
		mode := f.Mode()
		switch {
		case mode.IsDir():
			if err := os.MkdirAll(p, 0o755); err != nil {
				return err
			}
		case mode&os.ModeSymlink != 0:
			rc, err := f.Open()
			if err != nil {
				return err
			}
			target, err := io.ReadAll(io.LimitReader(rc, 4096))
			rc.Close()
			if err != nil {
				return err
			}
			os.MkdirAll(filepath.Dir(p), 0o755)
			if err := os.Symlink(string(target), p); err != nil {
				return err
			}
		default:
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return err
			}
			rc, err := f.Open()
			if err != nil {
				return err
			}
			perm := mode.Perm()
			if perm == 0 {
				perm = 0o644
			}
			out, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
			if err != nil {
				rc.Close()
				return err
			}
			_, err = io.Copy(out, rc)
			rc.Close()
			if cerr := out.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		switch {
		case info.IsDir():
			return os.MkdirAll(target, 0o755)
		case info.Mode()&os.ModeSymlink != 0:
			l, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(l, target)
		default:
			return copyFile(p, target, info.Mode().Perm())
		}
	})
}

func mb(n int64) string {
	return fmt.Sprintf("%.1f MB", float64(n)/1e6)
}
