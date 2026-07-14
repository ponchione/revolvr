//go:build linux || darwin || freebsd

package main

import (
	"fmt"
	"os"

	"revolvr/internal/cli"
)

var version = "dev"

func main() {
	root := cli.NewRootCommand(cli.Options{
		Version: version,
		Out:     os.Stdout,
		Err:     os.Stderr,
	})
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
