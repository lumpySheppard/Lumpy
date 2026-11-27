package ui

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/tiff"

	"github.com/lumpysheppard/lumpy/internal/job"
	"github.com/lumpysheppard/lumpy/internal/media"
)

// Photos in a place: a folder renamed by a pattern, in the order the
// pictures were taken or numbered, with a picture's other files (the
// raw beside the JPEG, the edit beside the scan) renamed in step; and
// negatives (the lab's scans, the raws) matched to their positives
// (the edits and exports) so the big originals can go, or be made
// small, without losing the picture.

// editSuffix is what editing and exporting apps hang on a name:
// "scan012-Edit.jpg", "IMG_0013 copy.jpg", "roll3 (1).tif".
var editSuffix = regexp.MustCompile(`(?i)(?:[-_ ]+(?:edit|edited|export|exported|web|small|sm|hdr|enhanced|final|retouched|bw|mono|crop|cropped|print|insta|ig|lr|positive|pos|negative|neg|scan|copy(?: \d+)?)|\s*\(\d+\))$`)

// photoStem is a picture's name with its extension and any editing
// suffix removed, lower case: what its negative, positive and
// sidecar files share.
func photoStem(name string) string {
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	for {
		s2 := editSuffix.ReplaceAllString(stem, "")
		if s2 == stem || strings.TrimSpace(s2) == "" {
			break
		}
		stem = s2
	}
	return strings.ToLower(strings.TrimSpace(stem))
}

// stemTail is what photoStem took off a name, case kept, so a renamed
// file keeps its "-Edit".
func stemTail(name string) string {
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	key := photoStem(name)
	if len(stem) >= len(key) && strings.ToLower(stem[:len(key)]) == key {
		return stem[len(key):]
	}
	return ""
}

var trailingNumber = regexp.MustCompile(`(\d+)\D*$`)

// negativeScore says how much a file looks like the original of a
// picture rather than an edit of it: raws and TIFFs over JPEGs, and
// the folder's name counts.
func negativeScore(rel string) int {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(rel), "."))
	score := map[string]int{"dng": 50, "cr2": 50, "cr3": 50, "arw": 50, "nef": 50, "raf": 50, "psd": 45, "tif": 40, "tiff": 40, "exr": 40, "png": 30, "heic": 25, "jpg": 10, "jpeg": 10, "webp": 5, "gif": 5}[ext]
	low := strings.ToLower(rel)
	for _, w := range []string{"negative", "scan", "original", "raw", "lab", "master", "tiff"} {
		if strings.Contains(low, w) {
			score += 20
			break
		}
	}
	for _, w := range []string{"export", "web", "edit", "positive", "final", "instagram", "social", "small", "share", "upload"} {
		if strings.Contains(low, w) {
			score -= 20
			break
		}
	}
	return score
}

type photoFile struct {
	Rel   string `json:"rel"`
	Size  int64  `json:"size"`
	Mtime int64  `json:"mtime"`
}

type photoPair struct {
	Key       string      `json:"key"`
	Negative  photoFile   `json:"negative"`
	Positives []photoFile `json:"positives"`
}

type unpackedZip struct {
	Rel     string `json:"rel"`
	Size    int64  `json:"size"`
	Entries int    `json:"entries"`
	Found   int    `json:"found"`
}

// walkPlace lists the regular files of a place, hidden ones and the
// insides of bundles left out, up to a limit.
func walkPlace(root string, limit int, fn func(rel string, info os.FileInfo)) {
	n := 0
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || p == root {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if bundleExts[strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))] {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || n >= limit {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		n++
		fn(filepath.ToSlash(rel), info)
		return nil
	})
}

// photoPairs finds every picture that exists as more than one file:
// the one most like an original is the negative, the rest positives.
func photoPairs(root string) ([]photoPair, map[string]bool) {
	groups := map[string][]photoFile{}
	names := map[string]bool{}
	walkPlace(root, 40000, func(rel string, info os.FileInfo) {
		names[strings.ToLower(filepath.Base(rel))] = true
		if fileKind(rel) != "image" {
			return
		}
		key := photoStem(filepath.Base(rel))
		if key == "" {
			return
		}
		groups[key] = append(groups[key], photoFile{Rel: rel, Size: info.Size(), Mtime: info.ModTime().Unix()})
	})
	var pairs []photoPair
	for key, files := range groups {
		if len(files) < 2 {
			continue
		}
		sort.Slice(files, func(i, j int) bool {
			a, b := negativeScore(files[i].Rel), negativeScore(files[j].Rel)
			if a != b {
				return a > b
			}
			if files[i].Size != files[j].Size {
				return files[i].Size > files[j].Size
			}
			return files[i].Rel < files[j].Rel
		})
		pairs = append(pairs, photoPair{Key: key, Negative: files[0], Positives: files[1:]})
	}
	sort.Slice(pairs, func(i, j int) bool { return natLess(pairs[i].Negative.Rel, pairs[j].Negative.Rel) })
	if len(pairs) > 3000 {
		pairs = pairs[:3000]
	}
	return pairs, names
}

// unpackedZips finds archives whose files are already out: the zip
// the lab sent, kept after unpacking.
func unpackedZips(root string, names map[string]bool) []unpackedZip {
	var out []unpackedZip
	walkPlace(root, 40000, func(rel string, info os.FileInfo) {
		if strings.ToLower(filepath.Ext(rel)) != ".zip" {
			return
		}
		zr, err := zip.OpenReader(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return
		}
		defer zr.Close()
		z := unpackedZip{Rel: rel, Size: info.Size()}
		for _, f := range zr.File {
			if f.FileInfo().IsDir() || strings.HasPrefix(f.Name, "__MACOSX") || strings.HasPrefix(filepath.Base(f.Name), ".") {
				continue
			}
			z.Entries++
			if names[strings.ToLower(filepath.Base(f.Name))] {
				z.Found++
			}
		}
		if z.Entries > 0 && z.Found*10 >= z.Entries*9 {
			out = append(out, z)
		}
	})
	return out
}

// spacePhotos is GET /api/space/photos?path=: the negatives and their
// positives, and the unpacked zips, of a place.
func (s *Server) spacePhotos(w http.ResponseWriter, r *http.Request) (any, error) {
	root := filepath.Clean(r.URL.Query().Get("path"))
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		return nil, badRequest("measure the folder first")
	}
	pairs, names := photoPairs(root)
	zips := unpackedZips(root, names)
	if pairs == nil {
		pairs = []photoPair{}
	}
	if zips == nil {
		zips = []unpackedZip{}
	}
	var negBytes, posBytes int64
	for _, p := range pairs {
		negBytes += p.Negative.Size
		for _, q := range p.Positives {
			posBytes += q.Size
		}
	}
	return map[string]any{"pairs": pairs, "zips": zips, "negativeBytes": negBytes, "positiveBytes": posBytes}, nil
}

// takenAt is when a picture was taken, from its EXIF when it has one,
// else when the file last changed.
func takenAt(full string, info os.FileInfo) time.Time {
	if fileKind(full) == "image" {
		if mi, err := media.Probe(full); err == nil && mi.Created != "" {
			for _, layout := range []string{"2006:01:02 15:04:05", time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
				if t, err := time.ParseInLocation(layout, mi.Created, time.Local); err == nil {
					return t
				}
			}
		}
	}
	return info.ModTime()
}

type renameStep struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Partner bool   `json:"partner,omitempty"` // a file elsewhere in the place, renamed in step
}

// spacePhotoRename is POST /api/space/photos/rename {path, rel,
// pattern, by, start, partners, dry}: the files directly in a folder
// renamed by a pattern, grouped by picture (a raw, its JPEG and its
// sidecar take one name), in the order chosen. The pattern's words:
//
//	{n} {nn} {nnn} {nnnn}  the number, padded to that many places
//	{date} {year} {month} {day} {time}  when the picture was taken
//	{name}  the old name, without its number   {num}  its old number
//
// With dry, only the plan comes back.
func (s *Server) spacePhotoRename(w http.ResponseWriter, r *http.Request) (any, error) {
	var body struct {
		Path     string `json:"path"`
		Rel      string `json:"rel"`
		Pattern  string `json:"pattern"`
		By       string `json:"by"`
		Start    int    `json:"start"`
		Partners bool   `json:"partners"`
		Dry      bool   `json:"dry"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, badRequest("bad request: %v", err)
	}
	root := filepath.Clean(body.Path)
	dir, err := insidePlace(root, body.Rel)
	if err != nil {
		return nil, err
	}
	pattern := strings.TrimSpace(body.Pattern)
	if pattern == "" || strings.ContainsAny(pattern, `/\:`) {
		return nil, badRequest("give a pattern without slashes or colons, with {n} or {date} in it")
	}
	if !strings.Contains(pattern, "{") {
		return nil, badRequest("the pattern needs {n} or {date} in it, or every file would get the same name")
	}
	if body.Start <= 0 {
		body.Start = 1
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, badRequest("%v", err)
	}
	type group struct {
		key   string
		name  string // the old stem, case kept
		num   string
		when  time.Time
		files []string // names in dir
	}
	byKey := map[string]*group{}
	var order []*group
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, ".") || !e.Type().IsRegular() {
			continue
		}
		key := photoStem(name)
		if key == "" {
			continue
		}
		g := byKey[key]
		if g == nil {
			stem := strings.TrimSuffix(name, filepath.Ext(name))
			g = &group{key: key, name: stem[:len(key)]}
			if m := trailingNumber.FindStringSubmatch(key); m != nil {
				g.num = m[1]
				g.name = strings.TrimRight(g.name[:strings.LastIndex(key, m[1])], " -_.")
			}
			byKey[key] = g
			order = append(order, g)
		}
		g.files = append(g.files, name)
		info, err := e.Info()
		if err == nil {
			t := takenAt(filepath.Join(dir, name), info)
			if g.when.IsZero() || t.Before(g.when) {
				g.when = t
			}
		}
	}
	if len(order) == 0 {
		return nil, badRequest("nothing to rename here")
	}
	switch body.By {
	case "date":
		sort.SliceStable(order, func(i, j int) bool {
			if !order[i].when.Equal(order[j].when) {
				return order[i].when.Before(order[j].when)
			}
			return natLess(order[i].key, order[j].key)
		})
	case "number":
		sort.SliceStable(order, func(i, j int) bool {
			a, _ := strconv.Atoi(order[i].num)
			b, _ := strconv.Atoi(order[j].num)
			if a != b && order[i].num != "" && order[j].num != "" {
				return a < b
			}
			if (order[i].num == "") != (order[j].num == "") {
				return order[i].num != ""
			}
			return natLess(order[i].key, order[j].key)
		})
	default:
		sort.SliceStable(order, func(i, j int) bool { return natLess(order[i].key, order[j].key) })
	}
	// Partners elsewhere in the place, by the same key.
	partners := map[string][]string{}
	if body.Partners {
		relDir := strings.Trim(filepath.ToSlash(strings.TrimPrefix(dir, root)), "/")
		walkPlace(root, 40000, func(rel string, info os.FileInfo) {
			d := ""
			if i := strings.LastIndex(rel, "/"); i >= 0 {
				d = rel[:i]
			}
			if d == relDir {
				return
			}
			if k := photoStem(filepath.Base(rel)); byKey[k] != nil {
				partners[k] = append(partners[k], rel)
			}
		})
	}
	width := len(strconv.Itoa(body.Start + len(order) - 1))
	var plan []renameStep
	newStems := map[string]bool{}
	for i, g := range order {
		n := body.Start + i
		stem := pattern
		rep := func(tok, val string) { stem = strings.ReplaceAll(stem, tok, val) }
		rep("{nnnn}", fmt.Sprintf("%04d", n))
		rep("{nnn}", fmt.Sprintf("%03d", n))
		rep("{nn}", fmt.Sprintf("%02d", n))
		rep("{n}", fmt.Sprintf("%0*d", width, n))
		rep("{date}", g.when.Format("2006-01-02"))
		rep("{year}", g.when.Format("2006"))
		rep("{month}", g.when.Format("01"))
		rep("{day}", g.when.Format("02"))
		rep("{time}", g.when.Format("1504"))
		rep("{name}", g.name)
		rep("{num}", g.num)
		stem = strings.TrimSpace(stem)
		if stem == "" {
			return nil, badRequest("the pattern leaves %s with no name", g.files[0])
		}
		if newStems[strings.ToLower(stem)] {
			return nil, badRequest("two pictures would both be called %s; put {n} in the pattern", stem)
		}
		newStems[strings.ToLower(stem)] = true
		relDir := strings.Trim(filepath.ToSlash(strings.TrimPrefix(dir, root)), "/")
		join := func(d, name string) string {
			if d == "" {
				return name
			}
			return d + "/" + name
		}
		for _, name := range g.files {
			to := stem + stemTail(name) + filepath.Ext(name)
			if to != name {
				plan = append(plan, renameStep{From: join(relDir, name), To: join(relDir, to)})
			}
		}
		for _, rel := range partners[g.key] {
			name := filepath.Base(rel)
			d := strings.TrimSuffix(rel, "/"+name)
			if d == rel {
				d = ""
			}
			to := stem + stemTail(name) + filepath.Ext(name)
			if to != name {
				plan = append(plan, renameStep{From: rel, To: join(d, to), Partner: true})
			}
		}
	}
	// Nothing may land on a file that is not itself moving.
	moving := map[string]bool{}
	for _, st := range plan {
		moving[strings.ToLower(st.From)] = true
	}
	for _, st := range plan {
		if moving[strings.ToLower(st.To)] {
			continue
		}
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(st.To))); err == nil {
			return nil, badRequest("%s is already there and is not being renamed", st.To)
		}
	}
	out := map[string]any{"plan": plan, "groups": len(order), "applied": false}
	if body.Dry || len(plan) == 0 {
		return out, nil
	}
	// Two steps, so numbers can shift without files landing on each other.
	tmp := make([]string, len(plan))
	for i, st := range plan {
		tmp[i] = filepath.Join(root, filepath.FromSlash(st.From)) + fmt.Sprintf(".lumpy-rename-%d", i)
		if err := os.Rename(filepath.Join(root, filepath.FromSlash(st.From)), tmp[i]); err != nil {
			for k := 0; k < i; k++ {
				os.Rename(tmp[k], filepath.Join(root, filepath.FromSlash(plan[k].From)))
			}
			return nil, badRequest("%v", err)
		}
	}
	for i, st := range plan {
		if err := os.Rename(tmp[i], filepath.Join(root, filepath.FromSlash(st.To))); err != nil {
			return nil, badRequest("%s could not take its new name: %v", st.From, err)
		}
	}
	out["applied"] = true
	if j, err := s.measure(root); err == nil && j != nil {
		out["job"] = j.ID
	}
	return out, nil
}

// spacePhotoRecompress is POST /api/space/photos/recompress {path,
// rels, quality, maxEdge}: each negative made into a JPEG of that
// quality, no longer than maxEdge on its long side (0 keeps the
// size), beside it with the same name; the original goes to the
// Trash. On a Mac the picture's metadata and colour profile come
// along. Runs as a job.
func (s *Server) spacePhotoRecompress(w http.ResponseWriter, r *http.Request) (any, error) {
	var body struct {
		Path    string   `json:"path"`
		Rels    []string `json:"rels"`
		Quality int      `json:"quality"`
		MaxEdge int      `json:"maxEdge"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, badRequest("bad request: %v", err)
	}
	root := filepath.Clean(body.Path)
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		return nil, badRequest("measure the folder first")
	}
	if body.Quality < 50 || body.Quality > 100 {
		body.Quality = 85
	}
	var fulls []string
	for _, rel := range body.Rels {
		full, err := insidePlace(root, rel)
		if err != nil {
			return nil, err
		}
		if full == root || fileKind(full) != "image" {
			return nil, badRequest("%s is not a picture", rel)
		}
		fulls = append(fulls, full)
	}
	if len(fulls) == 0 {
		return nil, badRequest("nothing chosen")
	}
	quality, maxEdge := body.Quality, body.MaxEdge
	j := s.jobs.Start("photos", fmt.Sprintf("Recompress %d negatives", len(fulls)), "", "", func(ctx context.Context, rep *job.Report) error {
		var before, after int64
		done := 0
		for _, full := range fulls {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			st, err := os.Stat(full)
			if err != nil {
				rep.Warn("%s: %v", filepath.Base(full), err)
				continue
			}
			out, err := recompress(ctx, full, quality, maxEdge)
			if err != nil {
				rep.Warn("%s: %v", filepath.Base(full), err)
				continue
			}
			ost, _ := os.Stat(out)
			before += st.Size()
			if ost != nil {
				after += ost.Size()
			}
			done++
			rep.Detail("  %s → %s · %s → %s", filepath.Base(full), filepath.Base(out), job.Bytes(st.Size()), job.Bytes(ost.Size()))
		}
		rep.Info("%d of %d recompressed · %s → %s, %s freed", done, len(fulls), job.Bytes(before), job.Bytes(after), job.Bytes(before-after))
		s.measure(root)
		return nil
	})
	return map[string]any{"job": j.ID}, nil
}

// recompress writes a JPEG of the picture beside it and trashes the
// original, returning the JPEG's path. The JPEG keeps the original's
// name (with .jpg) and its change time.
func recompress(ctx context.Context, full string, quality, maxEdge int) (string, error) {
	st, err := os.Stat(full)
	if err != nil {
		return "", err
	}
	dir, name := filepath.Split(full)
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	out := filepath.Join(dir, stem+".jpg")
	same := strings.EqualFold(out, full)
	if !same {
		if _, err := os.Lstat(out); err == nil {
			out = filepath.Join(dir, stem+" (smaller).jpg")
			if _, err := os.Lstat(out); err == nil {
				return "", fmt.Errorf("%s is already there", filepath.Base(out))
			}
		}
	}
	tmp := filepath.Join(dir, "."+stem+".lumpy-recompress.jpg")
	defer os.Remove(tmp)
	if runtime.GOOS == "darwin" {
		if sips, err := exec.LookPath("sips"); err == nil {
			args := []string{"-s", "format", "jpeg", "-s", "formatOptions", strconv.Itoa(quality)}
			if maxEdge > 0 {
				args = append(args, "--resampleHeightWidthMax", strconv.Itoa(maxEdge))
			}
			args = append(args, full, "--out", tmp)
			if err := exec.CommandContext(ctx, sips, args...).Run(); err != nil {
				return "", fmt.Errorf("sips could not read it: %v", err)
			}
			goto place
		}
	}
	{
		f, err := os.Open(full)
		if err != nil {
			return "", err
		}
		img, _, err := image.Decode(f)
		f.Close()
		if err != nil {
			return "", fmt.Errorf("could not read it: %v", err)
		}
		b := img.Bounds()
		if maxEdge > 0 && (b.Dx() > maxEdge || b.Dy() > maxEdge) {
			w, h := b.Dx(), b.Dy()
			if w >= h {
				h = h * maxEdge / w
				w = maxEdge
			} else {
				w = w * maxEdge / h
				h = maxEdge
			}
			dst := image.NewRGBA(image.Rect(0, 0, w, h))
			draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
			img = dst
		}
		o, err := os.Create(tmp)
		if err != nil {
			return "", err
		}
		if err := jpeg.Encode(o, img, &jpeg.Options{Quality: quality}); err != nil {
			o.Close()
			return "", err
		}
		o.Close()
	}
place:
	if tst, err := os.Stat(tmp); err != nil || tst.Size() == 0 {
		return "", fmt.Errorf("nothing came out")
	} else if tst.Size() >= st.Size() {
		return "", fmt.Errorf("it would not get smaller (%s → %s)", job.Bytes(st.Size()), job.Bytes(tst.Size()))
	}
	if err := job.TrashPaths([]string{full}); err != nil {
		return "", fmt.Errorf("the original could not go to the Trash: %v", err)
	}
	if err := os.Rename(tmp, out); err != nil {
		return "", err
	}
	os.Chtimes(out, st.ModTime(), st.ModTime())
	return out, nil
}
