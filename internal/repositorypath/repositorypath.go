// Package repositorypath owns the shared read-only admission check for
// canonical task and Revolvr runtime paths below a repository root.
package repositorypath

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"revolvr/internal/runtimepath"
)

const (
	AgentDir   = ".agent"
	TasksDir   = ".agent/tasks"
	RuntimeDir = ".revolvr"
	ConfigFile = ".revolvr/config.yaml"
	LedgerFile = ".revolvr/ledger.sqlite"
)

// InspectOptions contains the deterministic identity-substitution seam used
// by adversarial tests. Production callers leave AfterOpen nil.
type InspectOptions struct {
	AfterOpen func(relativePath string)
}

// Authority is one immutable repository-root identity plus the presence facts
// established by Inspect. Missing paths are represented without creating them.
type Authority struct {
	boundary       runtimepath.Boundary
	runtimePresent bool
	ledgerPresent  bool
	configPresent  bool
}

func (a Authority) Root() string { return a.boundary.Root() }

// Initialized preserves the existing status meaning: the runtime directory and
// ledger created by revolvr init are present. It does not turn preflight into a
// lease.
func (a Authority) Initialized() bool {
	return a.runtimePresent && a.ledgerPresent
}

func (a Authority) ConfigPresent() bool { return a.configPresent }

// ReadDir enumerates through the inspected repository-root identity.
func (a Authority) ReadDir(relativePath string, missingOK bool) ([]os.DirEntry, bool, error) {
	return a.boundary.ReadDir(filepath.Join(a.Root(), filepath.FromSlash(relativePath)), missingOK)
}

// ReadFile reads a protected regular file through the inspected repository-
// root identity and rechecks its name, mode, link count, and inode.
func (a Authority) ReadFile(relativePath string, missingOK bool) ([]byte, bool, error) {
	return a.boundary.ReadFile(filepath.Join(a.Root(), filepath.FromSlash(relativePath)), missingOK)
}

// Inspect validates every common path used before doctor, status, canonical
// task loading, or autonomous admission can proceed. It is read-only: missing
// paths remain presence facts and no directory, file, lock, or sidecar is made.
func Inspect(repositoryRoot string, options InspectOptions) (Authority, error) {
	boundary, err := runtimepath.Bind(repositoryRoot)
	if err != nil {
		return Authority{}, fmt.Errorf("inspect repository paths: %w", err)
	}
	authority := Authority{boundary: boundary}

	agent, found, err := openCheckedDir(boundary, AgentDir, options)
	if err != nil {
		return Authority{}, fmt.Errorf("inspect repository paths: %w", err)
	}
	if found {
		defer agent.Close()
		tasks, tasksFound, err := openCheckedChild(agent, "tasks", TasksDir, options)
		if err != nil {
			return Authority{}, fmt.Errorf("inspect repository paths: %w", err)
		}
		if tasksFound {
			defer tasks.Close()
			if err := inspectTaskFiles(tasks, options); err != nil {
				return Authority{}, fmt.Errorf("inspect repository paths: %w", err)
			}
		}
	}

	runtimeDir, found, err := openCheckedDir(boundary, RuntimeDir, options)
	if err != nil {
		return Authority{}, fmt.Errorf("inspect repository paths: %w", err)
	}
	if found {
		authority.runtimePresent = true
		defer runtimeDir.Close()
		if authority.configPresent, err = inspectOptionalFile(runtimeDir, "config.yaml", ConfigFile, options); err != nil {
			return Authority{}, fmt.Errorf("inspect repository paths: %w", err)
		}
		if authority.ledgerPresent, err = inspectOptionalFile(runtimeDir, "ledger.sqlite", LedgerFile, options); err != nil {
			return Authority{}, fmt.Errorf("inspect repository paths: %w", err)
		}
	}

	return authority, nil
}

func openCheckedDir(boundary runtimepath.Boundary, relativePath string, options InspectOptions) (*runtimepath.Directory, bool, error) {
	dir, found, err := boundary.OpenDir(filepath.Join(boundary.Root(), filepath.FromSlash(relativePath)), true)
	if err != nil || !found {
		return dir, found, err
	}
	callAfterOpen(options, relativePath)
	if err := dir.Check(); err != nil {
		_ = dir.Close()
		return nil, false, err
	}
	return dir, true, nil
}

func openCheckedChild(parent *runtimepath.Directory, name, relativePath string, options InspectOptions) (*runtimepath.Directory, bool, error) {
	dir, found, err := parent.OpenDir(name, true)
	if err != nil || !found {
		return dir, found, err
	}
	callAfterOpen(options, relativePath)
	if err := dir.Check(); err != nil {
		_ = dir.Close()
		return nil, false, err
	}
	return dir, true, nil
}

func inspectTaskFiles(tasks *runtimepath.Directory, options InspectOptions) error {
	entries, err := tasks.ReadDir()
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if filepath.Ext(name) == ".md" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		if err := inspectFile(tasks, name, filepath.ToSlash(filepath.Join(TasksDir, name)), options); err != nil {
			return err
		}
	}
	return tasks.Check()
}

func inspectOptionalFile(dir *runtimepath.Directory, name, relativePath string, options InspectOptions) (bool, error) {
	err := inspectFile(dir, name, relativePath, options)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func inspectFile(dir *runtimepath.Directory, name, relativePath string, options InspectOptions) error {
	file, err := dir.OpenFile(name, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	callAfterOpen(options, relativePath)
	if err := file.Check(); err != nil {
		return err
	}
	return nil
}

func callAfterOpen(options InspectOptions, relativePath string) {
	if options.AfterOpen != nil {
		options.AfterOpen(filepath.ToSlash(relativePath))
	}
}
