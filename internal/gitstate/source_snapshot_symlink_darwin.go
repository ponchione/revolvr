//go:build darwin

package gitstate

import (
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// freadlink was added as Darwin syscall 551 in macOS 13. It reads the target
// from an O_SYMLINK descriptor instead of resolving the pathname again.
const darwinFreadlink = 551

func openSourceSymlink(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_SYMLINK|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return sourceFileFromDescriptor(fd, path)
}

func readSourceSymlink(file *os.File, size int64) (string, error) {
	return readSourceSymlinkTarget(size, func(buffer []byte) (int, error) {
		result, _, errno := syscall.Syscall(
			darwinFreadlink,
			file.Fd(),
			uintptr(unsafe.Pointer(&buffer[0])),
			uintptr(len(buffer)),
		)
		if errno != 0 {
			return 0, errno
		}
		return int(result), nil
	})
}
