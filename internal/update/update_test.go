package update

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"0.6.0", "0.5.0", true}, {"v0.6.0", "0.5.0", true}, {"0.5.0", "0.5.0", false}, {"0.5.1", "0.5.0", true},
		{"1.0.0", "0.9.9", true}, {"0.5.0", "0.6.0", false}, {"junk", "0.5.0", false}, {"0.6.0-rc1", "0.5.0", true},
	} {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

// zipOf builds an archive from name -> content; names ending in / are
// folders.
func zipOf(files map[string]string) []byte {
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for name, content := range files {
		if strings.HasSuffix(name, "/") {
			w.Create(name)
			continue
		}
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		h.SetMode(0o755)
		f, _ := w.CreateHeader(h)
		f.Write([]byte(content))
	}
	w.Close()
	return b.Bytes()
}

func sum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// fakeGitHub serves one release with an app zip, a binary zip and the
// checksum file, with or without a token requirement.
func fakeGitHub(t *testing.T, needToken bool, appZip, binZip []byte, sums string) *httptest.Server {
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if needToken && r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(404)
			return
		}
		switch r.URL.Path {
		case "/repos/" + Repo + "/releases/latest":
			fmt.Fprintf(w, `{"tag_name":"v0.9.0","body":"Faster offloads","html_url":"https://example/rel","published_at":"2026-10-14T10:00:00Z","assets":[
				{"name":"Lumpy-macOS.zip","url":"%s/assets/1","size":%d},
				{"name":"lumpy-macos-universal.zip","url":"%s/assets/2","size":%d},
				{"name":"SHA256SUMS","url":"%s/assets/3","size":%d}]}`, ts.URL, len(appZip), ts.URL, len(binZip), ts.URL, len(sums))
		case "/assets/1", "/assets/2", "/assets/3":
			if r.Header.Get("Accept") != "application/octet-stream" {
				w.WriteHeader(400)
				return
			}
			// GitHub redirects asset downloads to storage.
			http.Redirect(w, r, ts.URL+"/storage"+r.URL.Path, http.StatusFound)
		case "/storage/assets/1":
			w.Write(appZip)
		case "/storage/assets/2":
			w.Write(binZip)
		case "/storage/assets/3":
			w.Write([]byte(sums))
		default:
			w.WriteHeader(404)
		}
	}))
	return ts
}

func TestCheckAndInstallBundle(t *testing.T) {
	appZip := zipOf(map[string]string{"Lumpy.app/": "", "Lumpy.app/Contents/": "", "Lumpy.app/Contents/Resources/": "",
		"Lumpy.app/Contents/Resources/lumpy": "#!/bin/sh\necho lumpy 0.9.0\n", "Lumpy.app/Contents/Info.plist": "<plist/>"})
	binZip := zipOf(map[string]string{"lumpy": "#!/bin/sh\necho lumpy 0.9.0\n"})
	sums := sum(appZip) + "  Lumpy-macOS.zip\n" + sum(binZip) + "  lumpy-macos-universal.zip\n"
	ts := fakeGitHub(t, true, appZip, binZip, sums)
	defer ts.Close()
	BaseURL = ts.URL
	defer func() { BaseURL = "https://api.github.com" }()

	// Without a token the private repository looks empty, and the error
	// says what to do.
	if _, err := Check(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "token") {
		t.Fatalf("expected a token hint, got %v", err)
	}
	rel, err := Check(context.Background(), "tok")
	if err != nil {
		t.Fatal(err)
	}
	if rel.Version != "0.9.0" || rel.Notes != "Faster offloads" || len(rel.Assets) != 3 || !Newer(rel.Version, "0.5.0") {
		t.Fatalf("release: %+v", rel)
	}

	// An installed app: the bundle is replaced whole, the old one removed.
	dir := t.TempDir()
	bundle := filepath.Join(dir, "Lumpy.app")
	os.MkdirAll(filepath.Join(bundle, "Contents", "Resources"), 0o755)
	os.WriteFile(filepath.Join(bundle, "Contents", "Resources", "lumpy"), []byte("old"), 0o755)
	os.WriteFile(filepath.Join(bundle, "Contents", "Info.plist"), []byte("old plist"), 0o644)
	var lines []string
	target := Target{Bundle: bundle, Exe: filepath.Join(bundle, "Contents", "Resources", "lumpy")}
	if err := Install(context.Background(), rel, "tok", target, func(s string) { lines = append(lines, s) }); err != nil {
		t.Fatalf("install: %v\n%s", err, strings.Join(lines, "\n"))
	}
	got, _ := os.ReadFile(filepath.Join(bundle, "Contents", "Resources", "lumpy"))
	if !strings.Contains(string(got), "0.9.0") {
		t.Fatalf("binary not replaced: %q", got)
	}
	if st, err := os.Stat(filepath.Join(bundle, "Contents", "Resources", "lumpy")); err != nil || st.Mode().Perm()&0o100 == 0 {
		t.Fatalf("executable bit lost: %v %v", st, err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "Lumpy.app" {
			t.Fatalf("leftover in %s: %s", dir, e.Name())
		}
	}
	if !strings.Contains(strings.Join(lines, "\n"), "Checksum OK") {
		t.Fatalf("progress:\n%s", strings.Join(lines, "\n"))
	}

	// A bare binary is replaced in place.
	exe := filepath.Join(t.TempDir(), "lumpy")
	os.WriteFile(exe, []byte("old"), 0o755)
	if err := Install(context.Background(), rel, "tok", Target{Exe: exe}, nil); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(exe)
	if !strings.Contains(string(got), "0.9.0") {
		t.Fatalf("binary not replaced: %q", got)
	}
}

func TestInstallRefusesBadChecksum(t *testing.T) {
	appZip := zipOf(map[string]string{"Lumpy.app/Contents/Resources/lumpy": "new"})
	binZip := zipOf(map[string]string{"lumpy": "new"})
	sums := strings.Repeat("0", 64) + "  Lumpy-macOS.zip\n" + strings.Repeat("0", 64) + "  lumpy-macos-universal.zip\n"
	ts := fakeGitHub(t, false, appZip, binZip, sums)
	defer ts.Close()
	BaseURL = ts.URL
	defer func() { BaseURL = "https://api.github.com" }()
	rel, err := Check(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(t.TempDir(), "lumpy")
	os.WriteFile(exe, []byte("old"), 0o755)
	err = Install(context.Background(), rel, "", Target{Exe: exe}, nil)
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("expected a checksum error, got %v", err)
	}
	if got, _ := os.ReadFile(exe); string(got) != "old" {
		t.Fatal("binary was replaced despite the bad checksum")
	}
}

func TestUnzipRefusesEscapingPaths(t *testing.T) {
	z := zipOf(map[string]string{"../evil": "x"})
	p := filepath.Join(t.TempDir(), "a.zip")
	os.WriteFile(p, z, 0o644)
	if err := unzip(p, filepath.Join(t.TempDir(), "out")); err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("expected an unsafe path error, got %v", err)
	}
}
