//go:build !darwin

package volume

import "os"

// identify names a volume by its mount point name and size. Only macOS
// gives Lumpy a volume UUID.
func identify(mount string) (*Volume, error) {
	if _, err := os.Stat(mount); err != nil {
		return nil, err
	}
	return fallback(mount), nil
}

// mounted finds no drives: plugged-in drives are only tracked on macOS.
func mounted() ([]*Volume, error) { return nil, nil }
