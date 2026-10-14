package volume

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

func plistValue(plist, key string) string {
	re := regexp.MustCompile(`<key>` + key + `</key>\s*<(?:string|integer)>([^<]*)</`)
	if m := re.FindStringSubmatch(plist); m != nil {
		return m[1]
	}
	return ""
}

// identities remembers what diskutil said about each mount point, keyed
// by the filesystem id as well, so a different volume mounted at the
// same path is asked about afresh. Without this, every poll (the app
// looks every few seconds) would run diskutil once per drive, which
// keeps diskarbitrationd busier than Finder likes.
var (
	identMu    sync.Mutex
	identities = map[string]cachedIdentity{}
)

type cachedIdentity struct {
	fsid [2]int32
	v    Volume
}

func fsidOf(mount string) ([2]int32, bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(mount, &st); err != nil {
		return [2]int32{}, false
	}
	return st.Fsid.Val, true
}

// identify asks diskutil for the volume's UUID, name and size, once per
// mount; later calls reuse the answer and refresh only the free space.
func identify(mount string) (*Volume, error) {
	fsid, ok := fsidOf(mount)
	if ok {
		identMu.Lock()
		c, hit := identities[mount]
		identMu.Unlock()
		if hit && c.fsid == fsid {
			v := c.v
			v.Free = free(mount)
			return &v, nil
		}
	}
	v, err := identifyWithDiskutil(mount)
	if err == nil && ok {
		identMu.Lock()
		identities[mount] = cachedIdentity{fsid: fsid, v: *v}
		identMu.Unlock()
	}
	return v, err
}

func identifyWithDiskutil(mount string) (*Volume, error) {
	out, err := exec.Command("diskutil", "info", "-plist", mount).Output()
	if err != nil {
		return fallback(mount), nil
	}
	p := string(out)
	v := &Volume{Mount: mount, Name: plistValue(p, "VolumeName"), FSType: plistValue(p, "FilesystemType"), Free: free(mount)}
	if v.Name == "" {
		v.Name = filepath.Base(mount)
	}
	if n, err := strconv.ParseInt(plistValue(p, "TotalSize"), 10, 64); err == nil {
		v.Size = n
	} else {
		v.Size = capacity(mount)
	}
	if id := plistValue(p, "VolumeUUID"); id != "" {
		v.ID = strings.ToUpper(id)
	} else {
		v.ID = "name:" + v.Name + ":" + itoa(v.Size)
	}
	v.SolidState = plistBool(p, "SolidState")
	v.Bus = plistValue(p, "BusProtocol")
	v.Disk = plistValue(p, "ParentWholeDisk")
	v.Internal = plistBool(p, "Internal") && !plistBool(p, "RemovableMediaOrExternalDevice")
	v.DiskImage = plistValue(p, "BusProtocol") == "Disk Image" || plistValue(p, "VirtualOrPhysical") == "Virtual" && plistValue(p, "BusProtocol") == ""
	return v, nil
}

// plistBool reads a <true/> or <false/> value from diskutil's plist.
func plistBool(p, key string) bool {
	i := strings.Index(p, "<key>"+key+"</key>")
	if i < 0 {
		return false
	}
	rest := strings.TrimSpace(p[i+len("<key>"+key+"</key>"):])
	return strings.HasPrefix(rest, "<true/>")
}

// skipFS are filesystems that are not drives you plug in.
var skipFS = map[string]bool{"smbfs": true, "afpfs": true, "nfs": true, "webdav": true, "autofs": true, "devfs": true}

// mounted lists the external and secondary volumes under /Volumes,
// leaving out the startup disk, network shares and macOS system volumes.
func mounted() ([]*Volume, error) {
	entries, err := os.ReadDir("/Volumes")
	if err != nil {
		return nil, err
	}
	var vols []*Volume
	for _, e := range entries {
		if e.Type()&os.ModeSymlink != 0 || !e.IsDir() {
			continue // "Macintosh HD" is a symlink to /
		}
		name := e.Name()
		if strings.HasPrefix(name, "com.apple") || strings.HasPrefix(name, ".") || name == "Recovery" || name == "Preboot" {
			continue
		}
		mount := filepath.Join("/Volumes", name)
		if mp, err := MountPoint(mount); err != nil || mp != mount {
			continue // a plain folder, not a mount
		}
		v, err := Identify(mount)
		if err != nil || skipFS[v.FSType] || hidden(v) {
			continue
		}
		vols = append(vols, v)
	}
	return vols, nil
}
