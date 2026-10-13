// Package volume finds which drive a path is on and identifies drives so
// the same drive is recognised every time it is plugged in, even renamed.
package volume

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Volume is a mounted drive.
type Volume struct {
	// ID stays the same across mounts: the volume UUID where the OS gives
	// one, otherwise a value built from the name and size.
	ID     string
	Name   string
	Mount  string // where it is mounted now, e.g. /Volumes/SHUTTLE_1
	Size   int64  // capacity in bytes
	FSType string
}

// MountPoint returns the root of the mounted volume that holds path.
func MountPoint(path string) (string, error) {
	p, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	p, err = filepath.EvalSymlinks(p)
	if err != nil {
		return "", err
	}
	if root := testRoot(); root != "" {
		if r, err := filepath.EvalSymlinks(root); err == nil {
			root = r
		}
		if rel, err := filepath.Rel(root, p); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			return filepath.Join(root, strings.SplitN(rel, string(filepath.Separator), 2)[0]), nil
		}
	}
	dev, err := device(p)
	if err != nil {
		return "", err
	}
	for {
		parent := filepath.Dir(p)
		if parent == p {
			return p, nil
		}
		pd, err := device(parent)
		if err != nil || pd != dev {
			return p, nil
		}
		p = parent
	}
}

func device(p string) (uint64, error) {
	st, err := os.Stat(p)
	if err != nil {
		return 0, err
	}
	return uint64(st.Sys().(*syscall.Stat_t).Dev), nil
}

// testRoot is a folder whose subfolders stand in for drives, set with
// LUMPY_VOLUMES so the drive library can be tested without real drives.
func testRoot() string { return os.Getenv("LUMPY_VOLUMES") }

// Identify returns the volume mounted at mount.
func Identify(mount string) (*Volume, error) {
	if testRoot() != "" {
		if _, err := os.Stat(mount); err != nil {
			return nil, err
		}
		return fallback(mount), nil
	}
	return identify(mount)
}

// Mounted lists the drives plugged in now.
func Mounted() ([]*Volume, error) {
	dir := testRoot()
	if dir == "" {
		return mounted()
	}
	if r, err := filepath.EvalSymlinks(dir); err == nil {
		dir = r
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var vols []*Volume
	for _, e := range entries {
		if e.IsDir() {
			vols = append(vols, fallback(filepath.Join(dir, e.Name())))
		}
	}
	return vols, nil
}

// Of returns the volume that holds path.
func Of(path string) (*Volume, error) {
	mp, err := MountPoint(path)
	if err != nil {
		return nil, err
	}
	return Identify(mp)
}

func capacity(mount string) int64 {
	var st syscall.Statfs_t
	if err := syscall.Statfs(mount, &st); err != nil {
		return 0
	}
	return int64(st.Blocks) * int64(st.Bsize)
}

// fallback identifies a volume with no UUID by its name and capacity.
func fallback(mount string) *Volume {
	name := filepath.Base(mount)
	size := capacity(mount)
	return &Volume{ID: "name:" + name + ":" + itoa(size), Name: name, Mount: mount, Size: size}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
