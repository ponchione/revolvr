//go:build !unix

package lock

import (
	"os"
)

func openFlockFile(string, bool, os.FileMode) (*os.File, error) {
	return nil, ErrFlockUnsupported
}

func tryFlock(*os.File, FlockMode) error { return ErrFlockUnsupported }
func unlockFlock(*os.File) error         { return nil }
func flockWouldBlock(error) bool         { return false }
func noFollowSymlinkError(error) bool    { return false }
