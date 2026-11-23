// Package space measures what takes room on a drive or in a folder, for
// the Space tab: a tree of folders and their largest files, with the
// bytes each one occupies on the disk.
package space

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
)

// KeepFiles is how many files a folder keeps by name; the rest of its
// files are summed into one "smaller files" entry, so a folder of a
// million small files doesn't cost a million entries.
const KeepFiles = 100

// Node is a folder, a file, or a folder's smaller files summed up (Rest).
type Node struct {
	Name       string
	Size       int64 // bytes on the disk, so sparse and cloud-only files count what they really use
	Files      int64
	Dir        bool
	Rest       bool    // the folder's files beyond KeepFiles, summed
	Unreadable bool    // the folder could not be listed (no permission)
	Kids       []*Node // largest first
}

// Progress is how far a scan has got. It is safe to read while the scan runs.
type Progress struct {
	Files      atomic.Int64
	Bytes      atomic.Int64
	Unreadable atomic.Int64
	current    atomic.Value // string: the folder being listed
}

// Current is the folder being listed.
func (p *Progress) Current() string {
	s, _ := p.current.Load().(string)
	return s
}

// Scan measures everything under root. It stays on root's volume (a drive
// mounted inside it is left out), doesn't follow symbolic links, and
// counts a file with several hard links once. Folders it may not read are
// marked Unreadable and counted in p.Unreadable; the scan goes on.
func Scan(ctx context.Context, root string, p *Progress) (*Node, error) {
	if p == nil {
		p = &Progress{}
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, &os.PathError{Op: "scan", Path: root, Err: syscall.ENOTDIR}
	}
	dev, _, _ := ids(info)
	s := &scanner{ctx: ctx, dev: dev, p: p, sem: make(chan struct{}, 32), seen: map[[2]uint64]bool{}}
	n := &Node{Name: filepath.Base(root), Dir: true}
	s.dir(root, n)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return n, nil
}

type scanner struct {
	ctx  context.Context
	dev  uint64
	p    *Progress
	sem  chan struct{}
	mu   sync.Mutex
	seen map[[2]uint64]bool // device and inode of files with more than one link
}

func (s *scanner) dir(path string, n *Node) {
	if s.ctx.Err() != nil {
		return
	}
	s.p.current.Store(path)
	entries, err := os.ReadDir(path)
	if err != nil && len(entries) == 0 {
		n.Unreadable = true
		s.p.Unreadable.Add(1)
		return
	}
	var wg sync.WaitGroup
	var files []*Node
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue // gone since the listing
		}
		size := onDisk(info)
		if info.IsDir() {
			if dev, _, _ := ids(info); dev != s.dev {
				continue // another volume is mounted here
			}
			c := &Node{Name: e.Name(), Dir: true, Size: size}
			n.Kids = append(n.Kids, c)
			p := filepath.Join(path, e.Name())
			select {
			case s.sem <- struct{}{}:
				wg.Add(1)
				go func() {
					defer func() { <-s.sem; wg.Done() }()
					s.dir(p, c)
				}()
			default:
				s.dir(p, c)
			}
			continue
		}
		if dev, ino, links := ids(info); links > 1 {
			s.mu.Lock()
			dup := s.seen[[2]uint64{dev, ino}]
			s.seen[[2]uint64{dev, ino}] = true
			s.mu.Unlock()
			if dup {
				size = 0
			}
		}
		files = append(files, &Node{Name: e.Name(), Size: size, Files: 1})
		s.p.Files.Add(1)
		s.p.Bytes.Add(size)
	}
	wg.Wait()
	sort.Slice(files, func(i, j int) bool { return files[i].Size > files[j].Size })
	if len(files) > KeepFiles {
		rest := &Node{Rest: true, Name: ""}
		for _, f := range files[KeepFiles:] {
			rest.Size += f.Size
			rest.Files++
		}
		files = append(files[:KeepFiles], rest)
	}
	n.Kids = append(n.Kids, files...)
	for _, c := range n.Kids {
		n.Size += c.Size
		n.Files += c.Files
	}
	sortKids(n)
}

func sortKids(n *Node) {
	sort.SliceStable(n.Kids, func(i, j int) bool { return n.Kids[i].Size > n.Kids[j].Size })
}

// onDisk is the space a file takes on its drive: its allocated blocks,
// which is less than its length for a sparse file or one iCloud keeps
// only in the cloud.
func onDisk(info os.FileInfo) int64 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return int64(st.Blocks) * 512
	}
	return info.Size()
}

func ids(info os.FileInfo) (dev, ino, links uint64) {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Dev), uint64(st.Ino), uint64(st.Nlink)
	}
	return 0, 0, 1
}

// Find returns the node at rel, a slash-separated path under the scanned
// root ("" is the root), and the nodes above it, root first.
func Find(root *Node, rel string) (*Node, []*Node) {
	n := root
	var up []*Node
	for _, name := range strings.Split(rel, "/") {
		if name == "" {
			continue
		}
		var next *Node
		for _, c := range n.Kids {
			if !c.Rest && c.Name == name {
				next = c
				break
			}
		}
		if next == nil {
			return nil, nil
		}
		up = append(up, n)
		n = next
	}
	return n, up
}

// Remove takes the node at rel out of the tree, for something that was
// moved to the Trash, and takes its size off every folder above it.
func Remove(root *Node, rel string) bool {
	n, up := Find(root, rel)
	if n == nil || len(up) == 0 {
		return false
	}
	for _, a := range up {
		a.Size -= n.Size
		a.Files -= n.Files
	}
	parent := up[len(up)-1]
	for i, c := range parent.Kids {
		if c == n {
			parent.Kids = append(parent.Kids[:i], parent.Kids[i+1:]...)
			break
		}
	}
	for _, a := range up {
		sortKids(a)
	}
	return true
}

// View is a node as the app draws it: its children down to Depth levels,
// leaving out any smaller than a fraction of the node shown.
type View struct {
	Name       string  `json:"name"`
	Path       string  `json:"path"` // slash-separated, under the scanned root
	Size       int64   `json:"size"`
	Files      int64   `json:"files"`
	Dir        bool    `json:"dir,omitempty"`
	Rest       bool    `json:"rest,omitempty"`
	Unreadable bool    `json:"unreadable,omitempty"`
	Kids       []*View `json:"kids,omitempty"`
	More       int     `json:"more,omitempty"` // children left out for being too small
}

// ViewOf returns n (found at rel) with depth levels below it, keeping
// children of at least minSize bytes and at most limit per folder.
func ViewOf(n *Node, rel string, depth int, minSize int64, limit int) *View {
	v := &View{Name: n.Name, Path: rel, Size: n.Size, Files: n.Files, Dir: n.Dir, Rest: n.Rest, Unreadable: n.Unreadable}
	if depth == 0 {
		return v
	}
	for _, c := range n.Kids {
		if c.Size < minSize || len(v.Kids) >= limit {
			v.More++
			continue
		}
		p := c.Name
		if rel != "" {
			p = rel + "/" + c.Name
		}
		if c.Rest {
			p = ""
		}
		v.Kids = append(v.Kids, ViewOf(c, p, depth-1, minSize, limit))
	}
	return v
}

// Volume is the size of the drive holding a folder.
type Volume struct {
	Total int64 `json:"total"`
	Free  int64 `json:"free"`
	Root  bool  `json:"root"` // the folder is the drive's top level, so used minus scanned is hidden space
}

// VolumeOf describes the drive that holds path.
func VolumeOf(path string) (Volume, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return Volume{}, err
	}
	v := Volume{Total: int64(st.Blocks) * int64(st.Bsize), Free: int64(st.Bavail) * int64(st.Bsize)}
	clean := filepath.Clean(path)
	if clean == "/" {
		v.Root = true
	} else if a, err := os.Stat(clean); err == nil {
		if b, err := os.Stat(filepath.Dir(clean)); err == nil {
			da, _, _ := ids(a)
			db, _, _ := ids(b)
			v.Root = da != db
		}
	}
	return v, nil
}
