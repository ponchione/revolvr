//go:build linux || darwin || freebsd

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"revolvr/internal/sandbox"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	runtimeDirectory := os.Getenv("XDG_RUNTIME_DIR")
	if !filepath.IsAbs(runtimeDirectory) {
		runtimeDirectory = filepath.Join("/run/user", strconv.Itoa(os.Getuid()))
	}
	stateDirectory := os.Getenv("XDG_STATE_HOME")
	if !filepath.IsAbs(stateDirectory) {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		stateDirectory = filepath.Join(home, ".local", "state")
	}
	defaultState := filepath.Join(stateDirectory, "revolvr", "sandboxd")
	defaultSocket := filepath.Join(runtimeDirectory, "revolvr", "sandboxd.sock")

	flags := flag.NewFlagSet("revolvr-sandboxd", flag.ContinueOnError)
	socketPath := flags.String("socket", defaultSocket, "operator-owned Unix socket path")
	statePath := flags.String("state", defaultState, "private lifecycle evidence directory")
	dockerExecutable := flags.String("docker", "docker", "Docker client executable")
	dockerHost := flags.String("docker-host", "unix://"+filepath.Join(runtimeDirectory, "docker.sock"), "rootless Docker Unix socket")
	dependencyNetwork := flags.String("dependency-network", "", "approved Docker network for the dependencies profile")
	openNetwork := flags.String("open-network", "", "approved Docker network for attended diagnostic use")
	if err := flags.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("revolvr-sandboxd accepts flags only")
	}
	for _, directory := range []string{*statePath, filepath.Dir(*socketPath)} {
		if err := sandbox.PreparePrivateDirectory(directory); err != nil {
			return err
		}
	}
	stateRoot, err := filepath.Abs(*statePath)
	if err != nil {
		return err
	}
	stateRoot, err = filepath.EvalSymlinks(stateRoot)
	if err != nil {
		return err
	}
	ownerBytes := sha256.Sum256([]byte(strconv.Itoa(os.Getuid()) + "\x00" + stateRoot))
	runtime, err := sandbox.NewDockerRuntime(*dockerExecutable, *dockerHost, hex.EncodeToString(ownerBytes[:]))
	if err != nil {
		return err
	}
	runtime.DependencyNetwork = *dependencyNetwork
	runtime.OpenNetwork = *openNetwork

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	removed, err := runtime.Reconcile(ctx)
	if err != nil {
		return fmt.Errorf("reconcile sandbox orphans: %w", err)
	}
	for _, id := range removed {
		fmt.Fprintf(os.Stderr, "reconciled orphan sandbox %s\n", id)
	}
	manager, err := sandbox.NewManager(runtime, stateRoot)
	if err != nil {
		return err
	}
	listener, err := sandbox.ListenUnix(*socketPath)
	if err != nil {
		return err
	}
	defer listener.Close()
	defer os.Remove(*socketPath)
	return (&sandbox.Server{Manager: manager}).Serve(ctx, listener)
}
