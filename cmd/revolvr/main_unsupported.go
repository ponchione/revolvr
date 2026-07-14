//go:build !linux && !darwin && !freebsd

package main

import (
	"fmt"
	"os"
	"runtime"
)

func main() {
	fmt.Fprintf(
		os.Stderr,
		"revolvr: unsupported operating system %q; supported operating systems are Linux, macOS, and FreeBSD\n",
		runtime.GOOS,
	)
	os.Exit(1)
}
