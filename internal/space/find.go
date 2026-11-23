package space

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lumpysheppard/lumpy/internal/hashing"
)

// Beyond sizes: files that are the same file twice, and files nobody
// has touched in a year. Both work from the list of big files a scan
// keeps beside its tree.

// BigFile is one file of at least BigSize, remembered by a scan.
type BigFile struct {
	Rel   string `json:"rel"`
	Size  int64  `json:"size"` // length
	Disk  int64  `json:"disk"` // on disk
	Mtime int64  `json:"mtime"`
	Atime int64  `json:"atime"`
}

// BigSize is the smallest file a scan remembers for the finders.
const BigSize = 1 << 20

// DupeGroup is one set of identical files.
type DupeGroup struct {
	Hash   string   `json:"hash"`
	Size   int64    `json:"size"`
	Wasted int64    `json:"wasted"` // all copies but one
	Paths  []string `json:"paths"`
}

// Dupes finds identical files among big: same length first, then the
// same xxh64 of the whole file. Only candidates get read. Groups come
// biggest waste first; prog counts bytes hashed.
func Dupes(ctx context.Context, root string, big []BigFile, prog *Progress) ([]DupeGroup, int64, error) {
	bySize := map[int64][]BigFile{}
	for _, f := range big {
		bySize[f.Size] = append(bySize[f.Size], f)
	}
	var groups []DupeGroup
	var wasted int64
	for size, fs := range bySize {
		if len(fs) < 2 {
			continue
		}
		byHash := map[string][]string{}
		for _, f := range fs {
			if ctx.Err() != nil {
				return nil, 0, ctx.Err()
			}
			if prog != nil {
				prog.Current.Store(f.Rel)
			}
			h, err := hashFile(ctx, filepath.Join(root, filepath.FromSlash(f.Rel)), prog)
			if err != nil {
				continue
			}
			byHash[h] = append(byHash[h], f.Rel)
		}
		for h, paths := range byHash {
			if len(paths) < 2 {
				continue
			}
			sort.Strings(paths)
			w := size * int64(len(paths)-1)
			wasted += w
			groups = append(groups, DupeGroup{Hash: h, Size: size, Wasted: w, Paths: paths})
		}
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Wasted > groups[j].Wasted })
	return groups, wasted, nil
}

func hashFile(ctx context.Context, p string, prog *Progress) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := hashing.New(hashing.XXH64)
	buf := make([]byte, 4<<20)
	for {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		n, err := f.Read(buf)
		if n > 0 {
			h.Write(buf[:n])
			if prog != nil {
				prog.Bytes.Add(int64(n))
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}
	return h.Hex(), nil
}

// Old lists the big files neither changed nor opened in the given
// time, biggest first. Opened is from the access time, which macOS
// keeps for files on its own disk.
func Old(big []BigFile, olderThan time.Duration, limit int) ([]BigFile, int64) {
	cut := time.Now().Add(-olderThan).Unix()
	var out []BigFile
	var total int64
	for _, f := range big {
		last := f.Mtime
		if f.Atime > last {
			last = f.Atime
		}
		if last > 0 && last < cut {
			out = append(out, f)
			total += f.Disk
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Disk > out[j].Disk })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, total
}

// IsUnderApp says whether a path is inside an app bundle, so the old
// list can skip an app's insides (apps are judged whole, by last use).
func IsUnderApp(rel string) bool {
	for _, part := range strings.Split(rel, "/") {
		if strings.HasSuffix(part, ".app") {
			return true
		}
	}
	return false
}
