package implementer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"revolvr/internal/gitstate"
	"revolvr/internal/runner"
)

type HostObserver struct {
	GitExecutable string
	Timeout       time.Duration
	StdoutCap     int
	StderrCap     int
	CommandRunner func(context.Context, runner.Command) runner.Result
}

func (o HostObserver) Capture(ctx context.Context, root string) (WorkspaceObservation, error) {
	if o.GitExecutable == "" {
		o.GitExecutable = "git"
	}
	if o.Timeout <= 0 {
		o.Timeout = 30 * time.Second
	}
	if o.StdoutCap <= 0 {
		o.StdoutCap = 8 << 20
	}
	if o.StderrCap <= 0 {
		o.StderrCap = 1 << 20
	}
	if o.CommandRunner == nil {
		o.CommandRunner = runner.Run
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return WorkspaceObservation{}, err
	}
	run := func(arguments ...string) (string, error) {
		result := o.CommandRunner(ctx, runner.Command{
			Name: o.GitExecutable, Args: append(gitSafetyArguments(), arguments...), Dir: root,
			Env: safeGitEnvironment(), ReplaceEnv: true, Timeout: o.Timeout,
			StdoutLimit: o.StdoutCap, StderrLimit: o.StderrCap,
		})
		if result.Err != nil || result.TimedOut || result.ExitCode != 0 || result.StdoutTruncatedBytes != 0 || result.StderrTruncatedBytes != 0 {
			return "", fmt.Errorf("host Git observation failed: exit=%d timeout=%v truncated=%d/%d: %v: %s", result.ExitCode, result.TimedOut, result.StdoutTruncatedBytes, result.StderrTruncatedBytes, result.Err, strings.TrimSpace(result.Stderr))
		}
		return result.Stdout, nil
	}
	commandRunner := func(commandCtx context.Context, command runner.Command) runner.Result {
		command.Args = append(gitSafetyArguments(), command.Args...)
		command.Env = safeGitEnvironment()
		command.ReplaceEnv = true
		return o.CommandRunner(commandCtx, command)
	}
	snapshot, err := gitstate.CaptureSourceSnapshot(ctx, gitstate.SourceSnapshotConfig{
		WorkingDir: root, GitExecutable: o.GitExecutable, Timeout: o.Timeout,
		StdoutCap: o.StdoutCap, StderrCap: o.StderrCap, CommandRunner: commandRunner,
	})
	if err != nil {
		return WorkspaceObservation{}, err
	}
	revision, err := gitstate.PolicySourceRevision(snapshot)
	if err != nil {
		return WorkspaceObservation{}, err
	}
	tree, err := run("rev-parse", "--verify", "HEAD^{tree}")
	if err != nil {
		return WorkspaceObservation{}, err
	}
	capture, err := gitstate.CaptureChangedFiles(ctx, gitstate.Config{
		WorkingDir: root, GitExecutable: o.GitExecutable, Timeout: o.Timeout,
		StdoutCap: o.StdoutCap, StderrCap: o.StderrCap, CommandRunner: commandRunner,
	})
	if err != nil || capture.CaptureError != "" {
		return WorkspaceObservation{}, errors.Join(err, errors.New(capture.CaptureError))
	}
	manifest := make([]Change, 0, len(capture.Entries))
	for _, entry := range capture.Entries {
		manifest = append(manifest, Change{Status: entry.Status, Kind: string(entry.Kind), Path: filepath.ToSlash(entry.Path), OldPath: filepath.ToSlash(entry.OldPath)})
	}
	sort.Slice(manifest, func(i, j int) bool {
		return manifest[i].Path+"\x00"+manifest[i].OldPath+"\x00"+manifest[i].Status < manifest[j].Path+"\x00"+manifest[j].OldPath+"\x00"+manifest[j].Status
	})
	diff, err := run("diff", "--binary", "--full-index", "--no-ext-diff", "HEAD", "--")
	if err != nil {
		return WorkspaceObservation{}, err
	}
	var combined bytes.Buffer
	combined.WriteString(diff)
	for _, entry := range capture.Entries {
		if entry.Kind != gitstate.KindUntracked {
			continue
		}
		result := o.CommandRunner(ctx, runner.Command{
			Name: o.GitExecutable,
			Args: append(gitSafetyArguments(), "diff", "--no-index", "--binary", "--full-index", "--no-ext-diff", "--", "/dev/null", entry.Path),
			Dir:  root, Env: safeGitEnvironment(), ReplaceEnv: true, Timeout: o.Timeout,
			StdoutLimit: o.StdoutCap, StderrLimit: o.StderrCap,
		})
		if result.Err != nil || result.TimedOut || result.ExitCode != 1 || result.StdoutTruncatedBytes != 0 || result.StderrTruncatedBytes != 0 {
			return WorkspaceObservation{}, fmt.Errorf("capture untracked diff %q: exit=%d timeout=%v: %v: %s", entry.Path, result.ExitCode, result.TimedOut, result.Err, result.Stderr)
		}
		if combined.Len()+len(result.Stdout) > o.StdoutCap {
			return WorkspaceObservation{}, errors.New("host-observed combined source diff exceeds its output cap")
		}
		combined.WriteString(result.Stdout)
	}
	diffRaw := combined.Bytes()
	stableSnapshot, err := gitstate.CaptureSourceSnapshot(ctx, gitstate.SourceSnapshotConfig{
		WorkingDir: root, GitExecutable: o.GitExecutable, Timeout: o.Timeout,
		StdoutCap: o.StdoutCap, StderrCap: o.StderrCap, CommandRunner: commandRunner,
	})
	if err != nil {
		return WorkspaceObservation{}, err
	}
	if !reflect.DeepEqual(snapshot, stableSnapshot) {
		return WorkspaceObservation{}, errors.New("workspace source changed during host observation")
	}
	return WorkspaceObservation{
		SourceSnapshot: snapshot, SourceRevision: revision, HeadCommit: snapshot.Head,
		HeadTree: strings.TrimSpace(tree), RawStatus: []byte(capture.RawStatus),
		ChangedManifest: manifest, Diff: append([]byte(nil), diffRaw...), DiffSHA256: digestBytes(diffRaw),
	}, nil
}

func gitSafetyArguments() []string {
	return []string{"-c", "core.hooksPath=/dev/null", "-c", "credential.helper=", "-c", "protocol.file.allow=never"}
}

func safeGitEnvironment() []string {
	return []string{
		"PATH=" + os.Getenv("PATH"), "LANG=C", "LC_ALL=C", "HOME=/nonexistent",
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0",
	}
}
