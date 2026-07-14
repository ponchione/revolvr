//go:build !unix

package runner

import (
	"os/exec"
	"runtime"
)

func prepareProcessTree(*exec.Cmd) error {
	return &unsupportedProcessTreeError{platform: runtime.GOOS}
}

func signalProcessTree(int, bool) error {
	return &unsupportedProcessTreeError{platform: runtime.GOOS}
}

func processTreeRunning(int) (bool, error) {
	return false, &unsupportedProcessTreeError{platform: runtime.GOOS}
}

func processTreeIdentityReused(int) (bool, error) {
	return false, &unsupportedProcessTreeError{platform: runtime.GOOS}
}

type unsupportedProcessTreeError struct {
	platform string
}

func (e *unsupportedProcessTreeError) Error() string {
	return ErrProcessTreeUnsupported.Error() + ": " + e.platform
}

func (e *unsupportedProcessTreeError) Unwrap() error {
	return ErrProcessTreeUnsupported
}
