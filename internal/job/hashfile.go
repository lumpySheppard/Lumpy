package job

import (
	"context"
	"io"

	"github.com/lumpysheppard/lumpy/internal/fsx"
	"github.com/lumpysheppard/lumpy/internal/hashing"
)

const chunkSize = 8 << 20

// hashFile reads path from the drive (not from cache) and returns its
// checksum in format f and its length.
func hashFile(ctx context.Context, path string, f hashing.Format, prog *Progress) (string, int64, error) {
	file, err := fsx.OpenRead(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	fsx.DropCache(file)
	h := hashing.New(f)
	buf := make([]byte, chunkSize)
	var n int64
	for {
		if err := ctx.Err(); err != nil {
			return "", n, err
		}
		m, err := file.Read(buf)
		if m > 0 {
			h.Write(buf[:m])
			n += int64(m)
			if prog != nil {
				prog.Add(m)
			}
			pace(m)
		}
		if err == io.EOF {
			return h.Hex(), n, nil
		}
		if err != nil {
			return "", n, err
		}
	}
}
