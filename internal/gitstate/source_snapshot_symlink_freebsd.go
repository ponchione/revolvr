//go:build freebsd

package gitstate

import (
	"os"

	"golang.org/x/sys/unix"
)

// O_PATH is available on supported FreeBSD kernels but is absent from the
// x/sys version currently selected by this module.
const freeBSDOpenPath = 0x00400000

func openSourceSymlink(path string) (*os.File, error) {
	fd, err := unix.Open(path, freeBSDOpenPath|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
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
