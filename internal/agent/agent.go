// Package agent installs the launchd agent that runs `lumpy agent run`
// whenever a drive is mounted, and posts macOS notifications.
package agent

import (
	"os"
	"path/filepath"
	"syscall"
)

// Label is the launchd job name.
const Label = "com.lumpysheppard.lumpy.agent"

// Lock takes an exclusive lock on path so only one agent run works at a
// time. ok is false when another run holds it.
func Lock(path string) (unlock func(), ok bool, err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, false, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, false, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, false, nil
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, true, nil
}
