// Package space measures what takes up room on a disk, the way
// DaisyDisk does: every folder's size, kept as a tree that the page
// draws as rings and drills into. The tree is pruned as it is built,
// so a disk with millions of files fits in memory: a folder keeps its
// biggest children and rolls the rest into one "smaller items" entry.
package space

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
)

// Node is a file or folder and everything under it.
type Node struct {
	Name     string  `json:"name"`
	Bytes    int64   `json:"bytes"`
	Files    int64   `json:"files"` // files inside, 1 for a file
	Len      int64   `json:"-"`     // a file's length, as the library records sizes
	Dir      bool    `json:"dir,omitempty"`
	Rest     int64   `json:"rest,omitempty"` // children rolled into this node ("smaller items")
	Children []*Node `json:"children,omitempty"`
	Error    string  `json:"error,omitempty"` // a folder that couldn't be read
}

// Keep is how many children a folder keeps apart; the rest roll up.
const Keep = 60

// Progress is what a running scan reports.
type Progress struct {
	Bytes   atomic.Int64
	Files   atomic.Int64
	Current atomic.Value // string: the folder being read
}

// Scan measures root. Symlinks are not followed, other volumes mounted
// below root are skipped, and sizes are what the files take on disk
// (allocated blocks), so a sparse or cloud-only file counts as what
// it costs.
func Scan(ctx context.Context, root string, prog *Progress) (*Node, error) {
	st, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	dev := deviceOf(st)
	n := walk(ctx, root, st, dev, prog)
	n.Name = root
	return n, ctx.Err()
}

func deviceOf(st os.FileInfo) uint64 {
	if s, ok := st.Sys().(*syscall.Stat_t); ok {
		return uint64(s.Dev)
	}
	return 0
}

// onDisk is the space a file takes, from its allocated blocks when the
// system reports them, else its length.
func onDisk(st os.FileInfo) int64 {
	if s, ok := st.Sys().(*syscall.Stat_t); ok && s.Blocks >= 0 {
		b := int64(s.Blocks) * 512
		if b < st.Size() && s.Blocks == 0 {
			return st.Size()
		}
		return b
	}
	return st.Size()
}

// walk measures one folder with a bounded fan-out: the top of the tree
// reads its subfolders in parallel, deeper ones in sequence, so a
// fast SSD is kept busy without thousands of goroutines.
func walk(ctx context.Context, dir string, st os.FileInfo, dev uint64, prog *Progress) *Node {
	n := &Node{Name: st.Name(), Dir: true}
	if ctx.Err() != nil {
		return n
	}
	if prog != nil {
		prog.Current.Store(dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		n.Error = err.Error()
		return n
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := walkers
	for _, e := range entries {
		if ctx.Err() != nil {
			break
		}
		name := e.Name()
		full := filepath.Join(dir, name)
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			// The link itself is tiny; what it points at is counted
			// where it lives.
			continue
		}
		if e.IsDir() {
			if d := deviceOf(info); d != 0 && d != dev {
				continue // another volume mounted here
			}
			add := func(c *Node) {
				mu.Lock()
				n.Children = append(n.Children, c)
				n.Bytes += c.Bytes
				n.Files += c.Files
				mu.Unlock()
			}
			select {
			case sem <- struct{}{}:
				wg.Add(1)
				go func() {
					defer wg.Done()
					defer func() { <-sem }()
					add(walk(ctx, full, info, dev, prog))
				}()
			default:
				add(walk(ctx, full, info, dev, prog))
			}
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		size := onDisk(info)
		mu.Lock()
		n.Children = append(n.Children, &Node{Name: name, Bytes: size, Len: info.Size(), Files: 1})
		n.Bytes += size
		n.Files++
		mu.Unlock()
		if prog != nil {
			prog.Bytes.Add(size)
			prog.Files.Add(1)
		}
	}
	wg.Wait()
	prune(n)
	return n
}

// walkers bounds how many folders are read at once.
var walkers = make(chan struct{}, 8)

// prune keeps a folder's biggest children and rolls the rest into one
// entry, so the tree stays small however many files there are.
func prune(n *Node) {
	sort.Slice(n.Children, func(i, j int) bool { return n.Children[i].Bytes > n.Children[j].Bytes })
	if len(n.Children) <= Keep+1 {
		return
	}
	rest := &Node{Name: "", Dir: false}
	for _, c := range n.Children[Keep:] {
		rest.Bytes += c.Bytes
		rest.Files += c.Files
		rest.Rest++
	}
	n.Children = append(n.Children[:Keep:Keep], rest)
}

// At returns the node at a slash-separated path below root, or nil.
func (n *Node) At(rel string) *Node {
	if rel == "" || rel == "." {
		return n
	}
	cur := n
	for _, part := range strings.Split(rel, "/") {
		var next *Node
		for _, c := range cur.Children {
			if c.Name == part {
				next = c
				break
			}
		}
		if next == nil {
			return nil
		}
		cur = next
	}
	return cur
}

// Trim returns a copy of n cut to depth levels of children, for the
// page: the rings around the focused folder.
func (n *Node) Trim(depth int) *Node {
	c := *n
	c.Children = nil
	if depth <= 0 {
		return &c
	}
	for _, ch := range n.Children {
		c.Children = append(c.Children, ch.Trim(depth-1))
	}
	return &c
}

// Remove takes the node at rel out of the tree and subtracts its size
// all the way up, after something was moved to the Trash.
func (n *Node) Remove(rel string) bool {
	parts := strings.Split(rel, "/")
	if len(parts) == 0 || rel == "" {
		return false
	}
	parent := n.At(strings.Join(parts[:len(parts)-1], "/"))
	if parent == nil {
		return false
	}
	name := parts[len(parts)-1]
	for i, c := range parent.Children {
		if c.Name == name {
			parent.Children = append(parent.Children[:i], parent.Children[i+1:]...)
			cur := n
			for _, p := range parts[:len(parts)-1] {
				cur.Bytes -= c.Bytes
				cur.Files -= c.Files
				cur = cur.At(p)
			}
			cur.Bytes -= c.Bytes
			cur.Files -= c.Files
			return true
		}
	}
	return false
}
