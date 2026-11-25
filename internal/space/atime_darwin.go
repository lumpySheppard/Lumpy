package space

import (
	"os"
	"syscall"
)

func atimeOf(st os.FileInfo) int64 {
	if s, ok := st.Sys().(*syscall.Stat_t); ok {
		return s.Atimespec.Sec
	}
	return 0
}
