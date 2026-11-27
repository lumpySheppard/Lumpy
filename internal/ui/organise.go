package ui

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf16"

	"golang.org/x/sys/unix"
)

// Organising inside a measured place: new folders, renames, moves and
// Finder's colour tags, for a tidy that is more than deleting. On the
// iPad this is the file manager the Files app is not; on a Mac it is
// the same tags and moves the Finder would make. Each change
// re-measures the place so the rings stay true.

// tagColours are the Finder's, by the number it stores with a tag.
var tagColours = map[string]int{"Gray": 1, "Green": 2, "Purple": 3, "Blue": 4, "Yellow": 5, "Red": 6, "Orange": 7}

// tagAttr is where macOS keeps a file's tags; a Linux file system only
// takes attributes in the user namespace, which is where the tests run.
func tagAttr() string {
	if runtime.GOOS == "linux" {
		return "user.com.apple.metadata:_kMDItemUserTags"
	}
	return "com.apple.metadata:_kMDItemUserTags"
}

// readTags returns a file's tags: names, with the colour number
// stripped. Finder writes a binary plist; Lumpy writes an XML one;
// both are read.
func readTags(path string) []string {
	buf := make([]byte, 4096)
	n, err := unix.Getxattr(path, tagAttr(), buf)
	if err != nil || n <= 0 {
		return nil
	}
	data := buf[:n]
	var raw []string
	if bytes.HasPrefix(data, []byte("bplist00")) {
		raw = bplistStrings(data)
	} else {
		var pl struct {
			Array struct {
				Strings []string `xml:"string"`
			} `xml:"array"`
		}
		if err := xml.Unmarshal(data, &pl); err != nil {
			return nil
		}
		raw = pl.Array.Strings
	}
	var out []string
	for _, s := range raw {
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			s = s[:i]
		}
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// writeTags sets a file's tags, colour numbers included for the names
// the Finder colours, as an XML plist the Finder reads.
func writeTags(path string, tags []string) error {
	if len(tags) == 0 {
		if err := unix.Removexattr(path, tagAttr()); err != nil {
			// Nothing to remove is fine; the name of that error differs
			// by system, so the attribute is looked for instead.
			if _, gerr := unix.Getxattr(path, tagAttr(), make([]byte, 1)); gerr != nil {
				return nil
			}
			return err
		}
		return nil
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n" + `<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n" + `<plist version="1.0"><array>`)
	for _, t := range tags {
		v := t
		if c, ok := tagColours[t]; ok {
			v = fmt.Sprintf("%s\n%d", t, c)
		}
		var esc bytes.Buffer
		xml.EscapeText(&esc, []byte(v))
		b.WriteString("<string>" + esc.String() + "</string>")
	}
	b.WriteString("</array></plist>\n")
	return unix.Setxattr(path, tagAttr(), []byte(b.String()), 0)
}

// bplistStrings reads the strings of a binary plist whose top object
// is an array of strings (what the Finder writes for tags).
func bplistStrings(data []byte) []string {
	if len(data) < 40 {
		return nil
	}
	tr := data[len(data)-32:]
	offSize, refSize := int(tr[6]), int(tr[7])
	numObjects := binary.BigEndian.Uint64(tr[8:16])
	top := binary.BigEndian.Uint64(tr[16:24])
	tableOff := binary.BigEndian.Uint64(tr[24:32])
	if offSize == 0 || refSize == 0 || tableOff >= uint64(len(data)) || top >= numObjects {
		return nil
	}
	readN := func(at uint64, n int) (uint64, bool) {
		if at+uint64(n) > uint64(len(data)) {
			return 0, false
		}
		var v uint64
		for i := 0; i < n; i++ {
			v = v<<8 | uint64(data[at+uint64(i)])
		}
		return v, true
	}
	offset := func(i uint64) (uint64, bool) {
		if i >= numObjects {
			return 0, false
		}
		return readN(tableOff+i*uint64(offSize), offSize)
	}
	// An object's header: marker nibble and a count, with the count in
	// a following int when the low nibble is 15.
	head := func(at uint64) (byte, uint64, uint64, bool) {
		if at >= uint64(len(data)) {
			return 0, 0, 0, false
		}
		m := data[at]
		n := uint64(m & 0x0f)
		at++
		if n == 15 {
			if at >= uint64(len(data)) || data[at]&0xf0 != 0x10 {
				return 0, 0, 0, false
			}
			size := 1 << (data[at] & 0x0f)
			v, ok := readN(at+1, size)
			if !ok {
				return 0, 0, 0, false
			}
			n = v
			at += 1 + uint64(size)
		}
		return m & 0xf0, n, at, true
	}
	str := func(i uint64) (string, bool) {
		at, ok := offset(i)
		if !ok {
			return "", false
		}
		m, n, at, ok := head(at)
		if !ok {
			return "", false
		}
		switch m {
		case 0x50: // ASCII
			if at+n > uint64(len(data)) {
				return "", false
			}
			return string(data[at : at+n]), true
		case 0x60: // UTF-16BE
			if at+2*n > uint64(len(data)) {
				return "", false
			}
			u := make([]uint16, n)
			for k := range u {
				u[k] = binary.BigEndian.Uint16(data[at+2*uint64(k):])
			}
			return string(utf16.Decode(u)), true
		}
		return "", false
	}
	at, ok := offset(top)
	if !ok {
		return nil
	}
	m, n, at, ok := head(at)
	if !ok || m != 0xa0 { // an array
		return nil
	}
	var out []string
	for i := uint64(0); i < n; i++ {
		ref, ok := readN(at+i*uint64(refSize), refSize)
		if !ok {
			break
		}
		if s, ok := str(ref); ok {
			out = append(out, s)
		}
	}
	return out
}

// inPlace resolves a relative path inside a measured place, refusing
// anything that would leave it.
func insidePlace(root, rel string) (string, error) {
	rel = filepath.Clean("/" + filepath.FromSlash(rel))
	full := filepath.Join(root, rel)
	if full != root && !strings.HasPrefix(full, root+string(filepath.Separator)) {
		return "", badRequest("that is outside the measured folder")
	}
	return full, nil
}

func plainName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\:`) {
		return badRequest("give it a plain name, without slashes or colons")
	}
	return nil
}

// spaceTags is GET /api/space/tags?path=&rel=: the tags of everything
// directly inside a folder of a measured place, by name.
func (s *Server) spaceTags(w http.ResponseWriter, r *http.Request) (any, error) {
	root := filepath.Clean(r.URL.Query().Get("path"))
	dir, err := insidePlace(root, r.URL.Query().Get("rel"))
	if err != nil {
		return nil, err
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, badRequest("%v", err)
	}
	out := map[string][]string{}
	for _, e := range ents {
		if tags := readTags(filepath.Join(dir, e.Name())); len(tags) > 0 {
			out[e.Name()] = tags
		}
	}
	return out, nil
}

// spaceOrganise is POST /api/space/organise {path, op, rel, to, tags}:
//   - mkdir:  a new folder named to, inside rel
//   - rename: rel takes the name to
//   - move:   rel goes into the folder to (same place)
//   - tag:    rel's tags become tags
//
// After a change to what is where, the place is measured again.
func (s *Server) spaceOrganise(w http.ResponseWriter, r *http.Request) (any, error) {
	var body struct {
		Path string   `json:"path"`
		Op   string   `json:"op"`
		Rel  string   `json:"rel"`
		To   string   `json:"to"`
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, badRequest("bad request: %v", err)
	}
	root := filepath.Clean(body.Path)
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		return nil, badRequest("measure the folder first")
	}
	from, err := insidePlace(root, body.Rel)
	if err != nil {
		return nil, err
	}
	changed := true
	switch body.Op {
	case "mkdir":
		if err := plainName(body.To); err != nil {
			return nil, err
		}
		if err := os.Mkdir(filepath.Join(from, strings.TrimSpace(body.To)), 0o755); err != nil {
			return nil, badRequest("%v", err)
		}
	case "rename":
		if err := plainName(body.To); err != nil {
			return nil, err
		}
		if from == root {
			return nil, badRequest("the measured folder itself keeps its name")
		}
		dest := filepath.Join(filepath.Dir(from), strings.TrimSpace(body.To))
		if _, err := os.Stat(dest); err == nil {
			return nil, badRequest("%s is already there", body.To)
		}
		if err := os.Rename(from, dest); err != nil {
			return nil, badRequest("%v", err)
		}
	case "move":
		if from == root {
			return nil, badRequest("the measured folder itself stays put")
		}
		dir, err := insidePlace(root, body.To)
		if err != nil {
			return nil, err
		}
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			return nil, badRequest("choose a folder to move into")
		}
		if dir == from || strings.HasPrefix(dir, from+string(filepath.Separator)) {
			return nil, badRequest("a folder cannot move into itself")
		}
		dest := filepath.Join(dir, filepath.Base(from))
		if _, err := os.Stat(dest); err == nil {
			return nil, badRequest("%s is already in that folder", filepath.Base(from))
		}
		if err := os.Rename(from, dest); err != nil {
			return nil, badRequest("%v", err)
		}
	case "tag":
		var tags []string
		for _, t := range body.Tags {
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}
		if err := writeTags(from, tags); err != nil {
			return nil, badRequest("tags could not be written here: %v", err)
		}
		changed = false
	default:
		return nil, badRequest("unknown operation")
	}
	out := map[string]any{"ok": true}
	if changed {
		if j, err := s.measure(root); err == nil && j != nil {
			out["job"] = j.ID
		}
	}
	return out, nil
}
