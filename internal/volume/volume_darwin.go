package volume

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

func plistValue(plist, key string) string {
	re := regexp.MustCompile(`<key>` + key + `</key>\s*<(?:string|integer)>([^<]*)</`)
	if m := re.FindStringSubmatch(plist); m != nil {
		return m[1]
	}
	return ""
}

// identify asks diskutil for the volume's UUID, name and size.
func identify(mount string) (*Volume, error) {
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
