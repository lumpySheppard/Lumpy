//go:build darwin || linux

package job

import "golang.org/x/sys/unix"

func setXattr(path, name, value string) error {
	return unix.Setxattr(path, name, []byte(value), 0)
}

func getXattr(path, name string) (string, error) {
	buf := make([]byte, 256)
	n, err := unix.Getxattr(path, name, buf)
	if err != nil {
		return "", err
	}
	return string(buf[:n]), nil
}
