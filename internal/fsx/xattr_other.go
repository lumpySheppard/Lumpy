//go:build !darwin && !linux

package fsx

import "os"

// CopyXattrs does nothing on this platform.
func CopyXattrs(src string, dst *os.File) error { return nil }
