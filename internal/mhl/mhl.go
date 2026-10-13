// Package mhl reads and writes ASC MHL v2.0 histories (the ascmhl folder
// at the root of a copied card) and reads legacy MHL v1.1 files.
package mhl

import (
	"bytes"
	"crypto/sha512"
	"encoding/xml"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lumpysheppard/lumpy/internal/hashing"
	"github.com/lumpysheppard/lumpy/internal/scan"
)

const (
	chainFile  = "ascmhl_chain.xml"
	dateLayout = "2006-01-02T15:04:05-07:00"
)

// Hash is one recorded checksum of a file.
type Hash struct {
	Format     hashing.Format
	Hex        string
	Action     string // original, verified or failed
	Size       int64  // -1 when the manifest gives none
	Generation int
}

// History is everything the ascmhl folder of one root says about its files.
type History struct {
	Root        string
	Generations []string // generation file names, oldest first
	// Hashes maps a slash-separated path relative to Root to its hashes,
	// oldest first.
	Hashes map[string][]Hash
	// Problems lists generation files that are missing, unreadable, or
	// whose contents no longer match the C4 ID the chain recorded for them.
	Problems []string
}

// Exists reports whether root has an ascmhl folder.
func Exists(root string) bool {
	st, err := os.Stat(filepath.Join(root, scan.MHLDir))
	return err == nil && st.IsDir()
}

// Expected returns the hash to check a file against: the newest recorded
// hash in the most preferred format Lumpy can compute, skipping "failed" entries.
func (h *History) Expected(p string) (Hash, bool) {
	entries := h.Hashes[p]
	for _, f := range hashing.Preference {
		for i := len(entries) - 1; i >= 0; i-- {
			if entries[i].Format == f && entries[i].Action != "failed" {
				return entries[i], true
			}
		}
	}
	return Hash{}, false
}

// NextGeneration is the sequence number the next generation file gets.
func (h *History) NextGeneration() int { return len(h.Generations) + 1 }

type chainXML struct {
	Hashlists []struct {
		Seq  int    `xml:"sequencenr,attr"`
		Path string `xml:"path"`
		C4   string `xml:"c4"`
	} `xml:"hashlist"`
}

type hashEntryXML struct {
	XMLName xml.Name
	Value   string `xml:",chardata"`
	Action  string `xml:"action,attr"`
}

type hashlistXML struct {
	Hashes []struct {
		Path struct {
			Value string `xml:",chardata"`
			Size  string `xml:"size,attr"`
		} `xml:"path"`
		Entries []hashEntryXML `xml:",any"`
	} `xml:"hashes>hash"`
}

// Load reads the ascmhl folder under root. A root without one gives an
// empty History.
func Load(root string) (*History, error) {
	h := &History{Root: root, Hashes: map[string][]Hash{}}
	dir := filepath.Join(root, scan.MHLDir)
	if !Exists(root) {
		return h, nil
	}
	type gen struct {
		seq  int
		name string
		c4   string
	}
	var gens []gen
	if data, err := os.ReadFile(filepath.Join(dir, chainFile)); err == nil {
		var c chainXML
		if err := xml.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("reading %s: %w", filepath.Join(dir, chainFile), err)
		}
		for _, hl := range c.Hashlists {
			gens = append(gens, gen{hl.Seq, strings.TrimSpace(hl.Path), strings.TrimSpace(hl.C4)})
		}
	} else if errors.Is(err, os.ErrNotExist) {
		names, _ := filepath.Glob(filepath.Join(dir, "*.mhl"))
		for _, n := range names {
			base := filepath.Base(n)
			seq, _ := strconv.Atoi(strings.SplitN(base, "_", 2)[0])
			gens = append(gens, gen{seq, base, ""})
		}
		if len(gens) > 0 {
			h.Problems = append(h.Problems, "ascmhl folder has no "+chainFile)
		}
	} else {
		return nil, err
	}
	sort.Slice(gens, func(i, j int) bool { return gens[i].seq < gens[j].seq })

	for _, g := range gens {
		h.Generations = append(h.Generations, g.name)
		data, err := os.ReadFile(filepath.Join(dir, g.name))
		if err != nil {
			h.Problems = append(h.Problems, fmt.Sprintf("generation %s: %v", g.name, err))
			continue
		}
		if g.c4 != "" && C4(data) != g.c4 {
			h.Problems = append(h.Problems, fmt.Sprintf("generation %s was changed after it was written (C4 ID does not match the chain)", g.name))
		}
		var hl hashlistXML
		if err := xml.Unmarshal(data, &hl); err != nil {
			h.Problems = append(h.Problems, fmt.Sprintf("generation %s: %v", g.name, err))
			continue
		}
		for _, e := range hl.Hashes {
			p := strings.TrimSpace(e.Path.Value)
			size := int64(-1)
			if s, err := strconv.ParseInt(e.Path.Size, 10, 64); err == nil {
				size = s
			}
			for _, he := range e.Entries {
				f := hashing.Format(he.XMLName.Local)
				if f == "c4" || he.XMLName.Local == "previousPath" {
					continue
				}
				h.Hashes[p] = append(h.Hashes[p], Hash{
					Format:     f,
					Hex:        strings.ToLower(strings.TrimSpace(he.Value)),
					Action:     he.Action,
					Size:       size,
					Generation: g.seq,
				})
			}
		}
	}
	return h, nil
}

// legacyXML is the MHL v1.1 layout written by older offload tools.
type legacyXML struct {
	Hashes []struct {
		File       string `xml:"file"`
		Size       string `xml:"size"`
		MD5        string `xml:"md5"`
		SHA1       string `xml:"sha1"`
		XXHash64BE string `xml:"xxhash64be"`
	} `xml:"hash"`
}

// LoadLegacy reads an MHL v1.1 file. Its paths are relative to the folder
// the .mhl file sits in, which is returned as the History's Root.
func LoadLegacy(file string) (*History, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var l legacyXML
	if err := xml.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("reading %s: %w", file, err)
	}
	h := &History{Root: filepath.Dir(file), Hashes: map[string][]Hash{}, Generations: []string{filepath.Base(file)}}
	for _, e := range l.Hashes {
		p := path.Clean(filepath.ToSlash(strings.TrimSpace(e.File)))
		size := int64(-1)
		if s, err := strconv.ParseInt(strings.TrimSpace(e.Size), 10, 64); err == nil {
			size = s
		}
		add := func(f hashing.Format, v string) {
			if v = strings.ToLower(strings.TrimSpace(v)); v != "" {
				h.Hashes[p] = append(h.Hashes[p], Hash{Format: f, Hex: v, Action: "original", Size: size, Generation: 1})
			}
		}
		add(hashing.XXH64, e.XXHash64BE)
		add(hashing.MD5, e.MD5)
		add(hashing.SHA1, e.SHA1)
	}
	return h, nil
}

// Entry is one file in a generation Lumpy writes.
type Entry struct {
	Path     string
	Size     int64
	ModTime  time.Time
	Format   hashing.Format
	Hex      string
	Action   string
	HashDate time.Time
}

// Generation is a new generation file to add to a root's history.
type Generation struct {
	Process     string // "transfer" for a copy, "in-place" for a check
	Created     time.Time
	Hostname    string
	ToolVersion string
	Entries     []Entry
}

func ts(t time.Time) string { return t.UTC().Format(dateLayout) }

func esc(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func (g *Generation) render() []byte {
	var b bytes.Buffer
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	b.WriteString("<hashlist version=\"2.0\" xmlns=\"urn:ASC:MHL:v2.0\">\n")
	b.WriteString("  <creatorinfo>\n")
	fmt.Fprintf(&b, "    <creationdate>%s</creationdate>\n", ts(g.Created))
	fmt.Fprintf(&b, "    <hostname>%s</hostname>\n", esc(g.Hostname))
	fmt.Fprintf(&b, "    <tool version=\"%s\">lumpy</tool>\n", esc(g.ToolVersion))
	b.WriteString("  </creatorinfo>\n")
	b.WriteString("  <processinfo>\n")
	fmt.Fprintf(&b, "    <process>%s</process>\n", g.Process)
	b.WriteString("    <ignore>\n")
	for _, p := range append(append([]string{}, scan.Ignored...), scan.MHLDir, scan.MHLDir+"/") {
		fmt.Fprintf(&b, "      <pattern>%s</pattern>\n", esc(p))
	}
	b.WriteString("    </ignore>\n")
	b.WriteString("  </processinfo>\n")
	b.WriteString("  <hashes>\n")
	for _, e := range g.Entries {
		b.WriteString("    <hash>\n")
		fmt.Fprintf(&b, "      <path size=\"%d\" lastmodificationdate=\"%s\">%s</path>\n", e.Size, ts(e.ModTime), esc(e.Path))
		fmt.Fprintf(&b, "      <%s action=\"%s\" hashdate=\"%s\">%s</%s>\n", e.Format, e.Action, ts(e.HashDate), e.Hex, e.Format)
		b.WriteString("    </hash>\n")
	}
	b.WriteString("  </hashes>\n")
	b.WriteString("</hashlist>\n")
	return b.Bytes()
}

// Write adds g to root's history as the next generation and records it in
// the chain file. It returns the new generation file's path.
func Write(root string, g *Generation) (string, error) {
	h, err := Load(root)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, scan.MHLDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	seq := h.NextGeneration()
	name := fmt.Sprintf("%04d_%s_%s.mhl", seq, filepath.Base(filepath.Clean(root)), g.Created.UTC().Format("2006-01-02_150405Z"))
	data := g.render()
	if err := writeSynced(filepath.Join(dir, name), data); err != nil {
		return "", err
	}

	// Rebuild the chain from the existing one so earlier entries keep the
	// C4 IDs they were recorded with.
	var c chainXML
	if old, err := os.ReadFile(filepath.Join(dir, chainFile)); err == nil {
		if err := xml.Unmarshal(old, &c); err != nil {
			return "", fmt.Errorf("reading %s: %w", chainFile, err)
		}
	}
	var b bytes.Buffer
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	b.WriteString("<ascmhldirectory xmlns=\"urn:ASC:MHL:DIRECTORY:v2.0\">\n")
	writeLink := func(seq int, p, c4 string) {
		fmt.Fprintf(&b, "  <hashlist sequencenr=\"%d\">\n    <path>%s</path>\n    <c4>%s</c4>\n  </hashlist>\n", seq, esc(p), c4)
	}
	for _, hl := range c.Hashlists {
		writeLink(hl.Seq, strings.TrimSpace(hl.Path), strings.TrimSpace(hl.C4))
	}
	writeLink(seq, name, C4(data))
	b.WriteString("</ascmhldirectory>\n")
	if err := writeSynced(filepath.Join(dir, chainFile), b.Bytes()); err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

func writeSynced(p string, data []byte) error {
	tmp := p + scan.PartialSuffix
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, p)
}

const c4Charset = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// C4 returns the C4 ID of data (SHA-512 in base 58, padded to 90
// characters), which ASC MHL chains use to seal each generation file.
func C4(data []byte) string {
	sum := sha512.Sum512(data)
	n := new(big.Int).SetBytes(sum[:])
	base := big.NewInt(58)
	mod := new(big.Int)
	var out []byte
	for n.Sign() > 0 {
		n.DivMod(n, base, mod)
		out = append(out, c4Charset[mod.Int64()])
	}
	for len(out) < 88 {
		out = append(out, '1')
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return "c4" + string(out)
}
