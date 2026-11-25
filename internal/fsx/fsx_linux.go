package fsx

import (
	"os"

	"golang.org/x/sys/unix"
)

func noCache(f *os.File) {}

func dropCache(f *os.File) { _ = unix.Fadvise(int(f.Fd()), 0, 0, unix.FADV_DONTNEED) }
