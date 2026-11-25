package volume

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// Eject unmounts a plugged-in drive and ejects its disk, as the Finder's
// Eject does, so it can be unplugged. Tests replace it.
var Eject = func(mount string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("ejecting a drive is done on a Mac; unmount %s yourself", mount)
	}
	out, err := exec.Command("diskutil", "eject", mount).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if i := strings.LastIndex(msg, "\n"); i >= 0 {
			msg = strings.TrimSpace(msg[i+1:])
		}
		if msg == "" {
			msg = err.Error()
		}
		if strings.Contains(strings.ToLower(msg), "busy") || strings.Contains(strings.ToLower(msg), "in use") {
			msg = "something on this Mac still has files open on it (a player, a Finder window, an editing app). Close them and try again"
		}
		return fmt.Errorf("could not eject: %s", msg)
	}
	return nil
}
