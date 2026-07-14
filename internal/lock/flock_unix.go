//go:build unix

package lock

import (
	"errors"
	"os"
	"syscall"
)

func openFlockFile(path string, create bool, mode os.FileMode) (*os.File, error) {
	flags := os.O_RDWR | syscall.O_NOFOLLOW | syscall.O_NONBLOCK
	if create {
		flags |= os.O_CREATE
	}
	return os.OpenFile(path, flags, mode)
}

func tryFlock(file *os.File, mode FlockMode) error {
	operation := syscall.LOCK_NB
	if mode == FlockShared {
		operation |= syscall.LOCK_SH
	} else {
		operation |= syscall.LOCK_EX
	}
	return syscall.Flock(int(file.Fd()), operation)
}

func unlockFlock(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}

func flockWouldBlock(err error) bool {
	return errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN)
}

func noFollowSymlinkError(err error) bool { return errors.Is(err, syscall.ELOOP) }
