package job

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/lumpysheppard/lumpy/internal/catalog"
	"github.com/lumpysheppard/lumpy/internal/volume"
)

// ArchiveOp is one piece of footage to move (Dest set, relative to the
// drive) or to put in the Trash.
type ArchiveOp struct {
	Path string
	Dest string
}

// ArchiveFiles moves unused footage aside or to the Trash and updates
// the library to match. Nothing is overwritten: a destination that
// already exists is reported and skipped.
func ArchiveFiles(ctx context.Context, db *catalog.DB, v *volume.Volume, ops []ArchiveOp, trash bool, r *Report) error {
	verb := "Moving"
	if trash {
		verb = "Moving to the Trash"
	}
	prog := r.startProgress(verb, int64(len(ops)))
	moves := map[string]string{}
	var removed []string
	var batch []ArchiveOp
	flush := func() {
		if len(batch) == 0 {
			return
		}
		paths := make([]string, len(batch))
		for i, op := range batch {
			paths[i] = filepath.Join(v.Mount, filepath.FromSlash(op.Path))
		}
		err := trashPaths(paths)
		for i, op := range batch {
			if _, statErr := os.Lstat(paths[i]); statErr != nil {
				removed = append(removed, op.Path)
				r.Detail("trashed  %s", op.Path)
			} else if err != nil {
				r.Fail("%s: %v", op.Path, err)
			} else {
				r.Fail("%s: still there after the Finder said it moved it", op.Path)
			}
			prog.Add(1)
		}
		batch = batch[:0]
	}
	for _, op := range ops {
		if ctx.Err() != nil {
			break
		}
		prog.SetCurrent(op.Path)
		abs := filepath.Join(v.Mount, filepath.FromSlash(op.Path))
		if _, err := os.Lstat(abs); err != nil {
			r.Fail("%s: %v", op.Path, err)
			prog.Add(1)
			continue
		}
		if trash {
			batch = append(batch, op)
			if len(batch) >= 50 {
				flush()
			}
			continue
		}
		dst := filepath.Join(v.Mount, filepath.FromSlash(op.Dest))
		if _, err := os.Lstat(dst); err == nil {
			r.Fail("%s: something is already at %s", op.Path, op.Dest)
			prog.Add(1)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			r.Fail("%s: %v", op.Path, err)
			prog.Add(1)
			continue
		}
		if err := os.Rename(abs, dst); err != nil {
			r.Fail("%s: %v", op.Path, err)
			prog.Add(1)
			continue
		}
		moves[op.Path] = op.Dest
		r.Detail("moved    %s → %s", op.Path, op.Dest)
		prog.Add(1)
	}
	flush()
	prog.Finish()
	if len(moves) > 0 {
		if err := db.MovePaths(v.ID, moves); err != nil {
			r.Fail("updating the library: %v", err)
		}
	}
	if len(removed) > 0 {
		if err := db.RemovePaths(v.ID, removed); err != nil {
			r.Fail("updating the library: %v", err)
		}
	}
	summary := fmt.Sprintf("%d moved", len(moves))
	if trash {
		summary = fmt.Sprintf("%d moved to the Trash", len(removed))
	}
	if n := r.Failures(); n > 0 {
		summary += fmt.Sprintf(", %d failed", n)
	}
	if ctx.Err() != nil {
		summary += ", cancelled"
	}
	db.Log(v.ID, "archive", summary, r.Failures())
	r.Info("%s: %s", v.Name, summary)
	return ctx.Err()
}

// trashPaths puts files in the Trash through the Finder, so they can be
// put back; elsewhere than a Mac it deletes them.
var trashPaths = func(paths []string) error {
	if runtime.GOOS != "darwin" {
		for _, p := range paths {
			if err := os.RemoveAll(p); err != nil {
				return err
			}
		}
		return nil
	}
	quote := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	var b strings.Builder
	b.WriteString(`tell application "Finder" to delete {`)
	for i, p := range paths {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(`POSIX file "` + quote.Replace(p) + `"`)
	}
	b.WriteString("}")
	out, err := exec.Command("osascript", "-e", b.String()).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("the Finder could not move it to the Trash: %s", msg)
	}
	return nil
}
