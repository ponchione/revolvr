//go:build linux

package gitstate

import (
	"os"

	"golang.org/x/sys/unix"
)

func openSourceSymlink(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return sourceFileFromDescriptor(fd, path)
}

func readSourceSymlink(file *os.File, size int64) (string, error) {
	return readSourceSymlinkTarget(size, func(buffer []byte) (int, error) {
		return unix.Readlinkat(int(file.Fd()), "", buffer)
	})
}
