package volume

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// Erase formats a plugged-in volume in place, keeping its name and its
// filesystem, the way a camera's own format does for a card. Tests
// replace it.
var Erase = func(mount, name, fstype string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("erasing a card is done on a Mac; format %s yourself", mount)
	}
	fs := "ExFAT"
	switch strings.ToLower(fstype) {
	case "hfs", "hfs+", "jhfs+":
		fs = "JHFS+"
	case "apfs":
		fs = "APFS"
	case "msdos", "fat32", "fat":
		fs = "FAT32"
	}
	out, err := exec.Command("diskutil", "eraseVolume", fs, name, mount).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if i := strings.LastIndex(msg, "\n"); i >= 0 {
			msg = strings.TrimSpace(msg[i+1:])
		}
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("could not erase: %s", msg)
	}
	return nil
}
