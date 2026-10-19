package fsx

import (
	"os"

	"golang.org/x/sys/unix"
)

// F_NOCACHE keeps reads and writes of this file out of the unified buffer
// cache, so a file written this way is read back from the drive.
func noCache(f *os.File) { _, _ = unix.FcntlInt(f.Fd(), unix.F_NOCACHE, 1) }

func dropCache(f *os.File) {}
