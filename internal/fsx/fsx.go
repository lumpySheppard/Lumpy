// Package fsx opens files so that Lumpy's verification reads come from the
// drive, not from the copy macOS still holds in memory, and reports free space.
package fsx

import (
	"os"
	"syscall"
)

// OpenRead opens path for reading with caching turned off where the OS allows it.
func OpenRead(path string) (*os.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	noCache(f)
	return f, nil
}

// Create creates (or truncates) path for writing with caching turned off
// where the OS allows it.
func Create(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, err
	}
	noCache(f)
	return f, nil
}

// DropCache asks the OS to forget cached pages of an open file, so the
// next read of it goes to the drive. Call it after Sync.
func DropCache(f *os.File) { dropCache(f) }

// FreeBytes returns the space available to this user on the volume holding path.
func FreeBytes(path string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	return uint64(st.Bavail) * uint64(st.Bsize), nil
}
