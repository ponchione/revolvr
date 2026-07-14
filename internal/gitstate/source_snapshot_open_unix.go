//go:build linux || darwin || freebsd

package gitstate

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

const maxSourceSymlinkBytes = 1024 * 1024

func openSourceRegularFile(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("capture source snapshot: create regular-file descriptor")
	}
	return file, nil
}

func sourceFileFromDescriptor(fd int, path string) (*os.File, error) {
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("capture source snapshot: create symlink descriptor")
	}
	return file, nil
}

func readSourceSymlinkTarget(size int64, read func([]byte) (int, error)) (string, error) {
	if size < 0 || size > maxSourceSymlinkBytes {
		return "", errors.New("capture source snapshot: symlink target exceeds read limit")
	}
	bufferSize := 256
	if size >= int64(bufferSize) {
		bufferSize = int(size) + 1
	}
	for {
		buffer := make([]byte, bufferSize)
		n, err := read(buffer)
		if err != nil {
			return "", err
		}
		if n < 0 || n > len(buffer) {
			return "", errors.New("capture source snapshot: invalid symlink target size")
		}
		if n < len(buffer) {
			return string(buffer[:n]), nil
		}
		if bufferSize > maxSourceSymlinkBytes/2 {
			return "", errors.New("capture source snapshot: symlink target exceeds read limit")
		}
		bufferSize *= 2
	}
}
