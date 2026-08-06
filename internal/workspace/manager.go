package workspace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"revolvr/internal/gitstate"
	"revolvr/internal/id"
	"revolvr/internal/pathguard"
	"revolvr/internal/runner"
	"revolvr/internal/runtimepath"
	"revolvr/internal/storage/postgres"
)

const (
	checkoutIdentitySchema = "revolvr-checkout-identity-v1"
	defaultTimeout         = 30 * time.Second
	defaultOutputCap       = 4 << 20
	cleanupTimeout         = 30 * time.Second
)

type Manager struct {
	config        Config
	workspaceRoot runtimepath.Boundary
	artifactRoot  runtimepath.Boundary
}

func New(config Config) (*Manager, error) {
	if config.Pool == nil {
		return nil, errors.New("workspace manager: PostgreSQL pool is required")
	}
	workspaceRoot, err := bindManagedRoot(config.WorkspaceRoot, "workspace")
	if err != nil {
		return nil, err
	}
	artifactRoot, err := bindManagedRoot(config.ArtifactRoot, "artifact")
	if err != nil {
		return nil, err
	}
	if pathsOverlap(workspaceRoot.Root(), artifactRoot.Root()) {
		return nil, fmt.Errorf("%w: workspace and artifact roots overlap", ErrUnsafePath)
	}
	if strings.TrimSpace(config.GitExecutable) == "" {
		config.GitExecutable = "git"
	}
	if config.Timeout <= 0 {
		config.Timeout = defaultTimeout
	}
	if config.StdoutCap <= 0 {
		config.StdoutCap = defaultOutputCap
	}
	if config.StderrCap <= 0 {
		config.StderrCap = defaultOutputCap
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	if config.CommandRunner == nil {
		config.CommandRunner = runner.Run
	}
	config.WorkspaceRoot = workspaceRoot.Root()
	config.ArtifactRoot = artifactRoot.Root()
	return &Manager{config: config, workspaceRoot: workspaceRoot, artifactRoot: artifactRoot}, nil
}

func bindManagedRoot(path, label string) (runtimepath.Boundary, error) {
	if strings.TrimSpace(path) == "" {
		return runtimepath.Boundary{}, fmt.Errorf("workspace manager: %s root is required", label)
	}
	boundary, err := runtimepath.Bind(path)
	if err != nil {
		return runtimepath.Boundary{}, fmt.Errorf("%w: bind %s root: %v", ErrUnsafePath, label, err)
	}
	info, err := os.Lstat(boundary.Root())
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o022 != 0 {
		return runtimepath.Boundary{}, fmt.Errorf("%w: %s root is not a protected directory", ErrUnsafePath, label)
	}
	return boundary, nil
}

// Create records the planned workspace before creating its branch or
// worktree. Exact retries reconcile a planned operation; pre-existing effects
// discovered by a newly admitted operation are conflicts.
func (m *Manager) Create(ctx context.Context, request CreateRequest) (Workspace, error) {
	workspaceID, err := parseUUID("workspace id", request.WorkspaceID)
	if err != nil {
		return Workspace{}, err
	}
	runID, err := parseUUID("run id", request.RunID)
	if err != nil {
		return Workspace{}, err
	}
	if err := validateOperationID(request.OperationID, 480); err != nil {
		return Workspace{}, err
	}
	if request.SymbolicSourceID == "" {
		request.SymbolicSourceID = "workspace-" + request.WorkspaceID
	}
	if !validSymbolicID(request.SymbolicSourceID) {
		return Workspace{}, errors.New("workspace create: symbolic source id is malformed")
	}

	queries := postgres.New(m.config.Pool)
	authority, err := queries.GetWorkspaceRunAuthority(ctx, runID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Workspace{}, errors.New("workspace create: scheduler run does not exist")
	}
	if err != nil {
		return Workspace{}, fmt.Errorf("workspace create: load scheduler authority: %w", err)
	}
	if authority.RunStatus != "active" || authority.TaskStatus != "admitted" {
		return Workspace{}, fmt.Errorf("%w: run/task status is %q/%q", ErrConflict, authority.RunStatus, authority.TaskStatus)
	}
	if authority.SourceCommit != authority.CurrentCommit || authority.SourceTree != authority.CurrentTree {
		return Workspace{}, fmt.Errorf("%w: scheduler and project source identities differ", ErrWrongSourceRevision)
	}
	if err := m.validateSourceAuthority(ctx, authority); err != nil {
		return Workspace{}, err
	}

	workspacePath := filepath.Join(m.workspaceRoot.Root(), request.WorkspaceID)
	branchRef := "refs/heads/revolvr/workspaces/" + request.WorkspaceID
	before, beforeRaw, err := m.captureCheckoutIdentity(ctx, authority.CanonicalSourcePath)
	if err != nil {
		return Workspace{}, fmt.Errorf("workspace create: capture original checkout identity: %w", err)
	}
	row, replayed, err := m.admitWorkspace(ctx, postgres.InsertWorkspaceParams{
		ID: workspaceID, RunID: runID, ProjectID: authority.ProjectID,
		ProjectSourceID: authority.ProjectSourceID, TaskID: authority.TaskID,
		CreationOperationID: request.OperationID, SymbolicSourceID: request.SymbolicSourceID,
		OriginalCheckoutPath:  authority.CanonicalSourcePath,
		ManagedRepositoryPath: authority.ManagedRepositoryPath, WorkspaceRoot: m.workspaceRoot.Root(),
		WorkspacePath: workspacePath, BranchRef: branchRef,
		SourceCommit: authority.SourceCommit, SourceTree: authority.SourceTree,
		OriginalIdentityBefore: beforeRaw, CreatedAt: timestamp(m.now()),
	})
	if err != nil {
		return Workspace{}, err
	}
	if err := sameCreation(row, request, authority, workspacePath, branchRef, beforeRaw); err != nil {
		return Workspace{}, err
	}

	if row.Status == string(StatusPlanned) {
		row, err = m.advanceStatus(ctx, row, StatusCreating, "workspace.creating", request.OperationID, nil)
		if err != nil {
			return Workspace{}, err
		}
	}
	if row.Status == string(StatusCreating) {
		if err := m.ensureBranch(ctx, row, request.OperationID+":branch"); err != nil {
			return m.handleCreateFailure(row, request.OperationID, err)
		}
		device, inode, err := m.ensureWorktree(ctx, row, request.OperationID+":worktree")
		if err != nil {
			return m.handleCreateFailure(row, request.OperationID, err)
		}
		after, afterRaw, err := m.captureCheckoutIdentity(ctx, authority.CanonicalSourcePath)
		if err != nil {
			return m.handleCreateFailure(row, request.OperationID, err)
		}
		if !bytes.Equal(beforeRaw, afterRaw) {
			return m.handleCreateFailure(row, request.OperationID, &ConflictError{Effect: "original checkout", Detail: "source or filesystem identity changed during workspace creation"})
		}
		row, err = m.markReady(ctx, row, device, inode, afterRaw, request.OperationID)
		if err != nil {
			return Workspace{}, err
		}
		_ = after

		workspace, err := m.workspaceFromRow(ctx, row)
		if err != nil {
			return Workspace{}, err
		}
		workspace.OriginalBefore = before
		workspace.Replayed = replayed
		return workspace, nil
	}

	workspace, err := m.workspaceFromRow(ctx, row)
	if err != nil {
		return Workspace{}, err
	}
	if row.Status != string(StatusCleaned) {
		if err := m.revalidateWorkspace(ctx, row); err != nil {
			return Workspace{}, err
		}
	}
	current, currentRaw, err := m.captureCheckoutIdentity(ctx, authority.CanonicalSourcePath)
	if err != nil || !sameCheckoutIdentity(row.OriginalIdentityBefore, currentRaw) {
		return Workspace{}, errors.Join(err, &ConflictError{Effect: "original checkout", Detail: "identity differs from workspace admission"})
	}
	workspace.OriginalBefore = before
	workspace.OriginalAfter = &current
	workspace.Replayed = true
	return workspace, nil
}

func (m *Manager) handleCreateFailure(row postgres.CoreWorkspace, operationID string, cause error) (Workspace, error) {
	if errors.Is(cause, ErrInjectedCrash) {
		return Workspace{}, cause
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(context.Background()), cleanupTimeout)
	defer cancel()
	current, loadErr := postgres.New(m.config.Pool).GetWorkspace(ctx, row.ID)
	if loadErr == nil && current.Status != string(StatusFailed) && current.Status != string(StatusCleaned) {
		current, loadErr = m.markTerminal(ctx, current, StatusFailed, cause.Error(), operationID+":failed")
	}
	if loadErr == nil && current.Status == string(StatusFailed) {
		_, loadErr = m.cleanup(ctx, current, operationID+":cleanup")
	}
	return Workspace{}, errors.Join(cause, loadErr)
}

func (m *Manager) validateSourceAuthority(ctx context.Context, authority postgres.GetWorkspaceRunAuthorityRow) error {
	managed, err := runtimepath.Bind(authority.ManagedRepositoryPath)
	if err != nil {
		return fmt.Errorf("%w: bind managed repository: %v", ErrUnsafePath, err)
	}
	if pathsOverlap(authority.CanonicalSourcePath, managed.Root()) ||
		pathsOverlap(authority.CanonicalSourcePath, m.workspaceRoot.Root()) ||
		pathsOverlap(authority.CanonicalSourcePath, m.artifactRoot.Root()) ||
		pathsOverlap(managed.Root(), m.workspaceRoot.Root()) {
		return fmt.Errorf("%w: original, managed repository, workspace, and artifact roots must be isolated", ErrUnsafePath)
	}
	commit, err := m.git(ctx, managed.Root(), "rev-parse", "--verify", authority.SourceCommit+"^{commit}")
	if err != nil || strings.TrimSpace(commit) != authority.SourceCommit {
		return errors.Join(ErrWrongSourceRevision, err)
	}
	tree, err := m.git(ctx, managed.Root(), "rev-parse", "--verify", authority.SourceCommit+"^{tree}")
	if err != nil || strings.TrimSpace(tree) != authority.SourceTree {
		return errors.Join(ErrWrongSourceRevision, err)
	}
	return nil
}

func (m *Manager) captureCheckoutIdentity(ctx context.Context, source string) (CheckoutIdentity, []byte, error) {
	boundary, err := runtimepath.Bind(source)
	if err != nil {
		return CheckoutIdentity{}, nil, err
	}
	directory, found, err := boundary.OpenDir(boundary.Root(), false)
	if err != nil || !found {
		return CheckoutIdentity{}, nil, errors.Join(err, os.ErrNotExist)
	}
	device, inode, err := directory.Identity()
	_ = directory.Close()
	if err != nil {
		return CheckoutIdentity{}, nil, err
	}
	head, err := m.git(ctx, boundary.Root(), "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return CheckoutIdentity{}, nil, err
	}
	tree, err := m.git(ctx, boundary.Root(), "rev-parse", "--verify", "HEAD^{tree}")
	if err != nil {
		return CheckoutIdentity{}, nil, err
	}
	branch := ""
	branchResult := m.runGit(ctx, boundary.Root(), []string{"symbolic-ref", "--quiet", "--short", "HEAD"}, nil)
	if branchResult.ExitCode == 0 && branchResult.Err == nil && !branchResult.TimedOut {
		branch = strings.TrimSpace(branchResult.Stdout)
	} else if branchResult.ExitCode != 1 || branchResult.Err != nil || branchResult.TimedOut {
		return CheckoutIdentity{}, nil, gitResultError(branchResult)
	}
	status, err := m.git(ctx, boundary.Root(), "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return CheckoutIdentity{}, nil, err
	}
	snapshot, err := gitstate.CaptureSourceSnapshot(ctx, gitstate.SourceSnapshotConfig{
		WorkingDir: boundary.Root(), GitExecutable: m.config.GitExecutable,
		Timeout: m.config.Timeout, StdoutCap: m.config.StdoutCap, StderrCap: m.config.StderrCap,
		AllowHarnessRuntime: true, CommandRunner: gitstate.CommandRunner(m.safeCommandRunner),
	})
	if err != nil {
		return CheckoutIdentity{}, nil, err
	}
	statusSum := sha256.Sum256([]byte(status))
	identity := CheckoutIdentity{
		SchemaVersion: checkoutIdentitySchema, CanonicalPath: boundary.Root(), Device: device, Inode: inode,
		HeadCommit: strings.TrimSpace(head), HeadTree: strings.TrimSpace(tree), Branch: branch,
		StatusSHA256: hex.EncodeToString(statusSum[:]), StatusBytes: len(status),
		SnapshotSHA256: snapshot.SnapshotSHA256, IndexSHA256: snapshot.IndexSHA256, WorktreeSHA256: snapshot.WorktreeSHA256,
	}
	raw, err := json.Marshal(identity)
	return identity, raw, err
}

func (m *Manager) now() time.Time { return m.config.Clock().UTC().Truncate(time.Microsecond) }

func pathsOverlap(left, right string) bool {
	return pathguard.WithinRoot(left, right) || pathguard.WithinRoot(right, left)
}

func parseUUID(label, value string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("workspace: invalid %s: %w", label, err)
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}

func validateOperationID(value string, maximum int) error {
	if value == "" || value != strings.TrimSpace(value) || strings.ContainsAny(value, "\x00\r\n") || len(value) > maximum {
		return errors.New("workspace: operation id is empty, malformed, or oversized")
	}
	return nil
}

func validSymbolicID(value string) bool {
	if value == "" || len(value) > 256 {
		return false
	}
	for index, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || (index > 0 && strings.ContainsRune("._-", r)) {
			continue
		}
		return false
	}
	return true
}

func newUUID() pgtype.UUID {
	return pgtype.UUID{Bytes: uuid.MustParse(id.New()), Valid: true}
}

func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func textValue(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}

func stableHash(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func sameCheckoutIdentity(left, right []byte) bool {
	var leftIdentity, rightIdentity CheckoutIdentity
	return json.Unmarshal(left, &leftIdentity) == nil && json.Unmarshal(right, &rightIdentity) == nil && leftIdentity == rightIdentity
}

func sameJSON(left, right []byte) bool {
	var leftValue, rightValue any
	return json.Unmarshal(left, &leftValue) == nil && json.Unmarshal(right, &rightValue) == nil && reflect.DeepEqual(leftValue, rightValue)
}

func uniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
