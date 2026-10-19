//go:build darwin || linux

package fsx

import (
	"errors"
	"os"
	"runtime"
	"strings"

	"golang.org/x/sys/unix"
)

// CopyXattrs copies src's extended attributes (Finder tags, comments,
// resource forks and the like) onto dst. The quarantine flag is left
// behind. A destination that cannot store attributes at all is skipped
// silently; any other failure is returned for the caller to report.
func CopyXattrs(src string, dst *os.File) error {
	size, err := unix.Listxattr(src, nil)
	if err != nil {
		return ignoreUnsupported(err)
	}
	if size == 0 {
		return nil
	}
	buf := make([]byte, size)
	n, err := unix.Listxattr(src, buf)
	if err != nil {
		return ignoreUnsupported(err)
	}
	var first error
	for _, name := range strings.Split(string(buf[:n]), "\x00") {
		if name == "" || name == "com.apple.quarantine" {
			continue
		}
		if runtime.GOOS == "linux" && !strings.HasPrefix(name, "user.") {
			continue // other namespaces need privileges
		}
		sz, err := unix.Getxattr(src, name, nil)
		if err != nil {
			continue
		}
		val := make([]byte, sz)
		if sz > 0 {
			if sz, err = unix.Getxattr(src, name, val); err != nil {
				continue
			}
			val = val[:sz]
		}
		if err := unix.Fsetxattr(int(dst.Fd()), name, val, 0); err != nil && first == nil {
			first = err
		}
	}
	return ignoreUnsupported(first)
}

func ignoreUnsupported(err error) error {
	if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EOPNOTSUPP) {
		return nil
	}
	return err
}
