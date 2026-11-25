// Package edit reads what an edit refers to: the media files named in a
// timeline export (FCPXML from Final Cut Pro or Resolve, Premiere Pro
// or FCP 7 XML, a CMX 3600 EDL) or in a plain list of clip names.
package edit

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/lumpysheppard/lumpy/internal/media"
)

// Ref is one piece of media an edit uses: its file name, and the full
// path when the export records one.
type Ref struct {
	Name string `json:"name"`
	Path string `json:"path,omitempty"`
}

// Parse reads an export and returns the media it refers to, without
// repeats, and what kind of file it was.
func Parse(name string, data []byte) ([]Ref, string, error) {
	head := strings.ToLower(string(data[:min(len(data), 8192)]))
	switch {
	case strings.Contains(head, "<fcpxml"):
		refs, err := parseFCPXML(data)
		return dedupe(refs), "FCPXML", err
	case strings.Contains(head, "<xmeml"):
		refs, err := parseXMEML(data)
		return dedupe(refs), "Premiere / FCP 7 XML", err
	case strings.HasPrefix(strings.TrimSpace(head), "<"):
		return nil, "", fmt.Errorf("%s is XML, but not FCPXML or Premiere XML", name)
	case bytes.IndexByte(data, 0) >= 0:
		return nil, "", fmt.Errorf("%s is not a text file: export the timeline as FCPXML, XML or EDL", name)
	case isEDL(string(data)):
		return dedupe(parseEDL(string(data))), "EDL", nil
	default:
		return dedupe(parseList(string(data))), "clip list", nil
	}
}

func dedupe(refs []Ref) []Ref {
	seen := map[string]bool{}
	out := []Ref{}
	for _, r := range refs {
		r.Name = strings.TrimSpace(r.Name)
		r.Path = strings.TrimSpace(r.Path)
		if r.Name == "" && r.Path != "" {
			r.Name = path.Base(r.Path)
		}
		if r.Name == "" {
			continue
		}
		key := strings.ToLower(r.Path)
		if key == "" {
			key = strings.ToLower(r.Name)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, r)
	}
	return out
}

// fromURL turns file:///Volumes/RAID/Day%2004/C0001.MP4 into a path.
func fromURL(s string) string {
	s = strings.TrimSpace(s)
	if u, err := url.Parse(s); err == nil && u.Scheme == "file" && u.Path != "" {
		return u.Path
	}
	if p, err := url.PathUnescape(strings.TrimPrefix(s, "file://")); err == nil {
		return strings.TrimPrefix(p, "localhost")
	}
	return s
}

func decoder(data []byte) *xml.Decoder {
	d := xml.NewDecoder(bytes.NewReader(data))
	d.Strict = false
	d.CharsetReader = func(string, io.Reader) (io.Reader, error) { return nil, errors.New("skip") }
	d.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	return d
}

func attr(se xml.StartElement, name string) string {
	for _, a := range se.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// text reads an element's character data up to its end tag.
func text(d *xml.Decoder) string {
	var b strings.Builder
	depth := 0
	for {
		tok, err := d.Token()
		if err != nil {
			return b.String()
		}
		switch t := tok.(type) {
		case xml.CharData:
			if depth == 0 {
				b.Write(t)
			}
		case xml.StartElement:
			depth++
		case xml.EndElement:
			if depth == 0 {
				return b.String()
			}
			depth--
		}
	}
}

// parseFCPXML takes every asset: its media-rep src (FCPXML 1.9+), its
// src attribute (older), or just its name.
func parseFCPXML(data []byte) ([]Ref, error) {
	d := decoder(data)
	var refs []Ref
	var assetName string
	var assetHasSrc bool
	for {
		tok, err := d.Token()
		if err == io.EOF {
			return refs, nil
		}
		if err != nil {
			return refs, fmt.Errorf("reading FCPXML: %v", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "asset":
				assetName, assetHasSrc = attr(t, "name"), false
				if src := attr(t, "src"); src != "" {
					refs = append(refs, Ref{Path: fromURL(src)})
					assetHasSrc = true
				}
			case "media-rep":
				kind := attr(t, "kind")
				if src := attr(t, "src"); src != "" && (kind == "" || kind == "original-media") {
					refs = append(refs, Ref{Path: fromURL(src)})
					assetHasSrc = true
				}
			}
		case xml.EndElement:
			if t.Name.Local == "asset" {
				if !assetHasSrc && assetName != "" {
					refs = append(refs, Ref{Name: assetName})
				}
				assetName = ""
			}
		}
	}
}

// parseXMEML takes every <file> with a <pathurl> or <name>. A clip that
// uses a file already described refers to it by id only.
func parseXMEML(data []byte) ([]Ref, error) {
	d := decoder(data)
	var refs []Ref
	depth, fileDepth := 0, -1
	var cur Ref
	for {
		tok, err := d.Token()
		if err == io.EOF {
			return refs, nil
		}
		if err != nil {
			return refs, fmt.Errorf("reading XML: %v", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			switch {
			case t.Name.Local == "file" && fileDepth < 0:
				fileDepth, cur = depth, Ref{}
			case fileDepth >= 0 && depth == fileDepth+1 && t.Name.Local == "name" && cur.Name == "":
				cur.Name = text(d)
				depth--
			case fileDepth >= 0 && depth == fileDepth+1 && t.Name.Local == "pathurl" && cur.Path == "":
				cur.Path = fromURL(text(d))
				depth--
			}
		case xml.EndElement:
			if t.Name.Local == "file" && depth == fileDepth {
				if cur.Path != "" || cur.Name != "" {
					refs = append(refs, cur)
				}
				fileDepth = -1
			}
			depth--
		}
	}
}

var (
	edlEvent  = regexp.MustCompile(`(?m)^\d{3,6}\s+(\S{1,32})\s+(?:V|A|A2|AA|AA/V|B|NONE|A/V)\b`)
	edlClip   = regexp.MustCompile(`(?im)^\*\s*(?:FROM|TO)\s+CLIP\s+NAME:\s*(.+?)\s*$`)
	edlSource = regexp.MustCompile(`(?im)^\*\s*SOURCE\s+FILE:\s*(.+?)\s*$`)
	edlHeader = regexp.MustCompile(`(?m)^(?:TITLE|FCM):`)
)

func isEDL(s string) bool {
	return edlHeader.MatchString(s) || edlEvent.MatchString(s)
}

// parseEDL takes the FROM/TO CLIP NAME comments (and SOURCE FILE paths,
// which Resolve writes); without any, the reel column of each event.
func parseEDL(s string) []Ref {
	var refs []Ref
	for _, m := range edlClip.FindAllStringSubmatch(s, -1) {
		refs = append(refs, Ref{Name: m[1]})
	}
	for _, m := range edlSource.FindAllStringSubmatch(s, -1) {
		refs = append(refs, Ref{Path: strings.TrimSpace(m[1])})
	}
	if len(refs) > 0 {
		return refs
	}
	for _, m := range edlEvent.FindAllStringSubmatch(s, -1) {
		reel := m[1]
		if reel == "AX" || reel == "BL" {
			continue // placeholders: no tape, black
		}
		refs = append(refs, Ref{Name: reel})
	}
	return refs
}

var listSplit = regexp.MustCompile(`[,\t;"']+`)

// parseList takes every token that looks like a media file: a name with
// a video, audio or still extension, or a path to one.
func parseList(s string) []Ref {
	var refs []Ref
	for _, line := range strings.Split(s, "\n") {
		for _, tok := range listSplit.Split(line, -1) {
			tok = strings.TrimSpace(tok)
			if tok == "" || media.KindOf(tok) == media.Other || !media.Probable(tok) {
				continue
			}
			if strings.Contains(tok, "/") {
				refs = append(refs, Ref{Path: tok})
			} else {
				refs = append(refs, Ref{Name: tok})
			}
		}
	}
	return refs
}
