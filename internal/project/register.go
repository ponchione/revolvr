package project

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"revolvr/internal/id"
	"revolvr/internal/runner"
	"revolvr/internal/storage/postgres"
)

const (
	gitTimeout       = 30 * time.Second
	gitOutputCap     = 1 << 20
	remoteJSONCap    = 256 << 10
	maximumRemotes   = 128
	maximumRemoteURL = 32
)

var (
	ErrAlreadyRegistered  = errors.New("project is already registered")
	ErrUnusableRepository = errors.New("unusable Git repository")
	ErrManagedConflict    = errors.New("managed repository destination conflicts with registration")
)

type DirtyState struct {
	Dirty        bool   `json:"dirty"`
	PorcelainV1Z []byte `json:"porcelain_v1_z"`
}

type Remote struct {
	Name      string   `json:"name"`
	FetchURLs []string `json:"fetch_urls"`
	PushURLs  []string `json:"push_urls"`
}

type Registration struct {
	ProjectID             string
	ProjectSourceID       string
	EventID               string
	Name                  string
	Status                string
	CanonicalSourcePath   string
	ManagedRepositoryPath string
	CurrentCommit         string
	CurrentTree           string
	CurrentBranch         *string
	DefaultBranch         *string
	DirtyState            DirtyState
	Remotes               []Remote
	CreatedAt             time.Time
}

type repositoryState struct {
	root          string
	gitCommonDir  string
	commit        string
	tree          string
	currentBranch *string
	defaultBranch *string
	dirty         DirtyState
	remotes       []Remote
}

// Register admits one local Git worktree and records its managed mirror.
func Register(ctx context.Context, pool *pgxpool.Pool, localPath, managedRoot string) (Registration, error) {
	if pool == nil {
		return Registration{}, errors.New("register project: PostgreSQL pool is required")
	}

	root, common, err := resolveWorktree(ctx, localPath)
	if err != nil {
		return Registration{}, err
	}
	queries := postgres.New(pool)
	if _, err := queries.GetProjectRegistrationByCanonicalSourcePath(ctx, root); err == nil {
		return Registration{}, fmt.Errorf("%w: %s", ErrAlreadyRegistered, root)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Registration{}, fmt.Errorf("check existing project registration: %w", err)
	}

	state, err := inspectRepository(ctx, root, common)
	if err != nil {
		return Registration{}, err
	}
	managedPath, err := managedDestination(managedRoot, state.root)
	if err != nil {
		return Registration{}, err
	}
	if err := ensureMirror(ctx, state, managedPath); err != nil {
		return Registration{}, err
	}

	projectID := newUUID()
	sourceID := newUUID()
	eventID := newUUID()
	createdAt := time.Now().UTC().Truncate(time.Microsecond)
	dirtyJSON, err := json.Marshal(state.dirty)
	if err != nil {
		return Registration{}, fmt.Errorf("encode dirty-state evidence: %w", err)
	}
	remotesJSON, err := json.Marshal(state.remotes)
	if err != nil {
		return Registration{}, fmt.Errorf("encode configured remotes: %w", err)
	}
	payload, err := json.Marshal(registeredEvent{
		ProjectID:             uuidString(projectID),
		ProjectSourceID:       uuidString(sourceID),
		Name:                  filepath.Base(state.root),
		Status:                "registered",
		CanonicalSourcePath:   state.root,
		ManagedRepositoryPath: managedPath,
		CurrentCommit:         state.commit,
		CurrentTree:           state.tree,
		CurrentBranch:         state.currentBranch,
		DefaultBranch:         state.defaultBranch,
		DirtyState:            state.dirty,
		Remotes:               state.remotes,
	})
	if err != nil {
		return Registration{}, fmt.Errorf("encode project.registered event: %w", err)
	}

	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		qtx := queries.WithTx(tx)
		if _, err := qtx.InsertProject(ctx, postgres.InsertProjectParams{
			ID: projectID, Name: filepath.Base(state.root), Status: "registered",
			CreatedAt: timestamp(createdAt), UpdatedAt: timestamp(createdAt),
		}); err != nil {
			return err
		}
		if _, err := qtx.InsertProjectSource(ctx, postgres.InsertProjectSourceParams{
			ID: sourceID, ProjectID: projectID,
			CanonicalSourcePath: state.root, ManagedRepositoryPath: managedPath,
			CurrentCommit: state.commit, CurrentTree: state.tree,
			CurrentBranch: text(state.currentBranch), DefaultBranch: text(state.defaultBranch),
			DirtyState: dirtyJSON, Remotes: remotesJSON,
		}); err != nil {
			return err
		}
		_, err := qtx.AppendEvent(ctx, postgres.AppendEventParams{
			ID: eventID, ProjectID: projectID, EventType: "project.registered",
			AggregateType: "project", AggregateID: projectID, AggregateVersion: 1,
			Payload: payload, CreatedAt: timestamp(createdAt),
		})
		return err
	})
	if err != nil {
		return Registration{}, fmt.Errorf("persist project registration: %w", err)
	}

	return Registration{
		ProjectID: uuidString(projectID), ProjectSourceID: uuidString(sourceID), EventID: uuidString(eventID),
		Name: filepath.Base(state.root), Status: "registered",
		CanonicalSourcePath: state.root, ManagedRepositoryPath: managedPath,
		CurrentCommit: state.commit, CurrentTree: state.tree,
		CurrentBranch: state.currentBranch, DefaultBranch: state.defaultBranch,
		DirtyState: state.dirty, Remotes: state.remotes, CreatedAt: createdAt,
	}, nil
}

type registeredEvent struct {
	ProjectID             string     `json:"project_id"`
	ProjectSourceID       string     `json:"project_source_id"`
	Name                  string     `json:"name"`
	Status                string     `json:"status"`
	CanonicalSourcePath   string     `json:"canonical_source_path"`
	ManagedRepositoryPath string     `json:"managed_repository_path"`
	CurrentCommit         string     `json:"current_commit"`
	CurrentTree           string     `json:"current_tree"`
	CurrentBranch         *string    `json:"current_branch"`
	DefaultBranch         *string    `json:"default_branch"`
	DirtyState            DirtyState `json:"dirty_state"`
	Remotes               []Remote   `json:"remotes"`
}

func resolveWorktree(ctx context.Context, localPath string) (string, string, error) {
	if strings.TrimSpace(localPath) == "" {
		return "", "", fmt.Errorf("%w: local path is required", ErrUnusableRepository)
	}
	abs, err := filepath.Abs(localPath)
	if err != nil {
		return "", "", fmt.Errorf("%w: resolve local path: %v", ErrUnusableRepository, err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", "", fmt.Errorf("%w: resolve local path: %v", ErrUnusableRepository, err)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", "", fmt.Errorf("%w: local path is not a directory", ErrUnusableRepository)
	}
	bare, err := gitOutput(ctx, resolved, "rev-parse", "--is-bare-repository")
	if err != nil || strings.TrimSpace(bare) != "false" {
		return "", "", fmt.Errorf("%w: bare or non-Git path", ErrUnusableRepository)
	}
	top, err := gitOutput(ctx, resolved, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", fmt.Errorf("%w: resolve worktree root: %v", ErrUnusableRepository, err)
	}
	root, err := filepath.EvalSymlinks(strings.TrimSpace(top))
	if err != nil || !within(root, resolved) {
		return "", "", fmt.Errorf("%w: invalid worktree root", ErrUnusableRepository)
	}
	common, err := gitOutput(ctx, root, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", "", fmt.Errorf("%w: resolve Git common directory: %v", ErrUnusableRepository, err)
	}
	common = strings.TrimSpace(common)
	if !filepath.IsAbs(common) {
		common = filepath.Join(root, common)
	}
	common, err = filepath.EvalSymlinks(filepath.Clean(common))
	if err != nil {
		return "", "", fmt.Errorf("%w: resolve Git common directory: %v", ErrUnusableRepository, err)
	}
	return filepath.Clean(root), common, nil
}

func inspectRepository(ctx context.Context, root, common string) (repositoryState, error) {
	commit, err := gitOutput(ctx, root, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil || !validOID(strings.TrimSpace(commit)) {
		return repositoryState{}, fmt.Errorf("%w: HEAD is unborn or invalid", ErrUnusableRepository)
	}
	tree, err := gitOutput(ctx, root, "rev-parse", "--verify", "HEAD^{tree}")
	if err != nil || !validOID(strings.TrimSpace(tree)) {
		return repositoryState{}, fmt.Errorf("%w: HEAD tree is invalid", ErrUnusableRepository)
	}
	branch, err := optionalGitOutput(ctx, root, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return repositoryState{}, fmt.Errorf("%w: identify current branch: %v", ErrUnusableRepository, err)
	}
	if branch != nil {
		value := strings.TrimSpace(*branch)
		if value == "" {
			return repositoryState{}, fmt.Errorf("%w: current branch is empty", ErrUnusableRepository)
		}
		branch = &value
	}
	remotes, err := configuredRemotes(ctx, root)
	if err != nil {
		return repositoryState{}, fmt.Errorf("%w: %v", ErrUnusableRepository, err)
	}
	defaultBranch := remoteDefaultBranch(ctx, root, remotes)
	if defaultBranch == nil {
		defaultBranch = branch
	}
	status, err := gitOutput(ctx, root, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return repositoryState{}, fmt.Errorf("%w: capture dirty state: %v", ErrUnusableRepository, err)
	}
	return repositoryState{
		root: root, gitCommonDir: common,
		commit: strings.TrimSpace(commit), tree: strings.TrimSpace(tree),
		currentBranch: branch, defaultBranch: defaultBranch,
		dirty: DirtyState{Dirty: status != "", PorcelainV1Z: []byte(status)}, remotes: remotes,
	}, nil
}

func configuredRemotes(ctx context.Context, root string) ([]Remote, error) {
	raw, err := gitOutput(ctx, root, "remote")
	if err != nil {
		return nil, err
	}
	names := strings.Fields(raw)
	if len(names) > maximumRemotes {
		return nil, fmt.Errorf("configured remotes exceed limit %d", maximumRemotes)
	}
	sort.Strings(names)
	remotes := make([]Remote, 0, len(names))
	for i, name := range names {
		if name == "" || (i > 0 && name == names[i-1]) {
			return nil, errors.New("configured remote names are malformed")
		}
		fetch, err := optionalNULValues(ctx, root, "remote."+name+".url")
		if err != nil {
			return nil, err
		}
		push, err := optionalNULValues(ctx, root, "remote."+name+".pushurl")
		if err != nil {
			return nil, err
		}
		if len(fetch) > maximumRemoteURL || len(push) > maximumRemoteURL {
			return nil, fmt.Errorf("remote %q URL count exceeds limit %d", name, maximumRemoteURL)
		}
		remotes = append(remotes, Remote{Name: name, FetchURLs: fetch, PushURLs: push})
	}
	rawJSON, err := json.Marshal(remotes)
	if err != nil || len(rawJSON) > remoteJSONCap {
		return nil, errors.New("configured remotes exceed structured-data limit")
	}
	return remotes, nil
}

func optionalNULValues(ctx context.Context, root, key string) ([]string, error) {
	raw, err := optionalGitOutput(ctx, root, "config", "--null", "--get-all", key)
	if err != nil || raw == nil {
		return nil, err
	}
	if *raw == "" || (*raw)[len(*raw)-1] != 0 {
		return nil, fmt.Errorf("Git config returned malformed values for %q", key)
	}
	return strings.Split((*raw)[:len(*raw)-1], "\x00"), nil
}

func remoteDefaultBranch(ctx context.Context, root string, remotes []Remote) *string {
	for _, remote := range remotes {
		if remote.Name != "origin" {
			continue
		}
		branch, err := optionalGitOutput(ctx, root, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")
		if err == nil && branch != nil {
			branch := strings.TrimSpace(*branch)
			value := strings.TrimPrefix(branch, "origin/")
			if value != "" && value != branch {
				return &value
			}
		}
	}
	return nil
}

func managedDestination(root, source string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", errors.New("managed repository root is required")
	}
	prospective, abs, err := prospectivePath(root)
	if err != nil {
		return "", fmt.Errorf("resolve managed repository root: %w", err)
	}
	if within(source, prospective) {
		return "", errors.New("managed repository root must be outside the operator checkout")
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return "", fmt.Errorf("create managed repository root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolve managed repository root: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() || within(source, resolved) {
		return "", errors.New("managed repository root is not a safe directory")
	}
	identity := sha256.Sum256([]byte(source))
	return filepath.Join(resolved, hex.EncodeToString(identity[:])+".git"), nil
}

func prospectivePath(path string) (string, string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", "", err
	}
	abs = filepath.Clean(abs)
	ancestor := abs
	for {
		if _, err := os.Stat(ancestor); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", "", err
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", "", errors.New("no existing managed-root ancestor")
		}
		ancestor = parent
	}
	resolved, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return "", "", err
	}
	rel, err := filepath.Rel(ancestor, abs)
	if err != nil {
		return "", "", err
	}
	return filepath.Join(resolved, rel), abs, nil
}

func ensureMirror(ctx context.Context, source repositoryState, destination string) error {
	info, err := os.Lstat(destination)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if _, err := gitOutput(ctx, filepath.Dir(destination), "clone", "--mirror", "--no-local", "--quiet", "--", source.root, destination); err != nil {
			return fmt.Errorf("create managed repository mirror: %w", err)
		}
	case err != nil:
		return fmt.Errorf("inspect managed repository destination: %w", err)
	case info.Mode()&os.ModeSymlink != 0 || !info.IsDir():
		return fmt.Errorf("%w: destination is not a real directory", ErrManagedConflict)
	}

	bare, err := gitOutput(ctx, destination, "rev-parse", "--is-bare-repository")
	if err != nil || strings.TrimSpace(bare) != "true" {
		return fmt.Errorf("%w: destination is not a bare Git repository", ErrManagedConflict)
	}
	mirror, err := gitOutput(ctx, destination, "config", "--bool", "remote.origin.mirror")
	if err != nil || strings.TrimSpace(mirror) != "true" {
		return fmt.Errorf("%w: destination is not a Revolvr mirror", ErrManagedConflict)
	}
	urls, err := optionalNULValues(ctx, destination, "remote.origin.url")
	if err != nil || len(urls) != 1 || urls[0] != source.root {
		return fmt.Errorf("%w: destination source does not match", ErrManagedConflict)
	}
	commit, err := gitOutput(ctx, destination, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil || strings.TrimSpace(commit) != source.commit {
		return fmt.Errorf("%w: destination HEAD does not match", ErrManagedConflict)
	}
	tree, err := gitOutput(ctx, destination, "rev-parse", "--verify", "HEAD^{tree}")
	if err != nil || strings.TrimSpace(tree) != source.tree {
		return fmt.Errorf("%w: destination tree does not match", ErrManagedConflict)
	}
	if _, err := gitOutput(ctx, destination, "fsck", "--connectivity-only", "--no-dangling"); err != nil {
		return fmt.Errorf("%w: destination object graph is incomplete", ErrManagedConflict)
	}
	if err := rejectSourceHardlinks(source.gitCommonDir, destination); err != nil {
		return fmt.Errorf("%w: %v", ErrManagedConflict, err)
	}
	return nil
}

func rejectSourceHardlinks(sourceGitDir, destination string) error {
	sourceObjects := filepath.Join(sourceGitDir, "objects")
	destinationObjects := filepath.Join(destination, "objects")
	return filepath.WalkDir(destinationObjects, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("managed object path is a symlink: %s", path)
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(destinationObjects, path)
		if err != nil {
			return err
		}
		sourceInfo, err := os.Stat(filepath.Join(sourceObjects, rel))
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		destinationInfo, err := os.Stat(path)
		if err != nil {
			return err
		}
		if os.SameFile(sourceInfo, destinationInfo) {
			return fmt.Errorf("managed object is hard-linked to the source: %s", rel)
		}
		return nil
	})
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	result := runGit(ctx, dir, args...)
	if err := gitError(result, args); err != nil {
		return "", err
	}
	return result.Stdout, nil
}

func optionalGitOutput(ctx context.Context, dir string, args ...string) (*string, error) {
	result := runGit(ctx, dir, args...)
	if result.Err == nil && !result.TimedOut && result.ExitCode == 1 && result.StdoutTruncatedBytes == 0 && result.StderrTruncatedBytes == 0 {
		return nil, nil
	}
	if err := gitError(result, args); err != nil {
		return nil, err
	}
	value := result.Stdout
	return &value, nil
}

func runGit(ctx context.Context, dir string, args ...string) runner.Result {
	return runner.Run(ctx, runner.Command{
		Name: "git", Args: args, Dir: dir, Env: []string{"GIT_OPTIONAL_LOCKS=0"},
		Timeout: gitTimeout, StdoutLimit: gitOutputCap, StderrLimit: gitOutputCap,
	})
}

func gitError(result runner.Result, args []string) error {
	if result.Err == nil && !result.TimedOut && result.ExitCode == 0 && result.StdoutTruncatedBytes == 0 && result.StderrTruncatedBytes == 0 {
		return nil
	}
	detail := strings.TrimSpace(result.Stderr)
	if detail == "" {
		detail = fmt.Sprintf("exit code %d", result.ExitCode)
	}
	if result.Err != nil {
		detail = result.Err.Error()
	}
	if result.StdoutTruncatedBytes != 0 || result.StderrTruncatedBytes != 0 {
		detail = "command output exceeded the configured limit"
	}
	return fmt.Errorf("git %s: %s", strings.Join(args, " "), detail)
}

func validOID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func newUUID() pgtype.UUID {
	return pgtype.UUID{Bytes: uuid.MustParse(id.New()), Valid: true}
}

func uuidString(value pgtype.UUID) string { return uuid.UUID(value.Bytes).String() }

func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func text(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}
