package project

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"revolvr/internal/storage/postgres"
)

func TestRegisterStoresSourceAndPreservesOperatorCheckout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := projectTestPool(t, ctx)
	repo := projectTestRepository(t)
	runProjectGit(t, repo, "remote", "add", "origin", "https://example.invalid/revolvr-test.git")
	runProjectGit(t, repo, "remote", "set-url", "--add", "--push", "origin", "ssh://git@example.invalid/revolvr-test.git")

	committed := []byte("committed bytes\n")
	dirty := []byte("operator dirty bytes\n")
	untracked := []byte("operator untracked bytes\n")
	writeProjectFile(t, filepath.Join(repo, "tracked.txt"), dirty)
	writeProjectFile(t, filepath.Join(repo, "untracked.txt"), untracked)

	canonical, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	headBefore := string(runProjectGit(t, repo, "rev-parse", "HEAD"))
	treeBefore := string(runProjectGit(t, repo, "rev-parse", "HEAD^{tree}"))
	statusBefore := runProjectGit(t, repo, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	trackedBefore := readProjectFile(t, filepath.Join(repo, "tracked.txt"))
	untrackedBefore := readProjectFile(t, filepath.Join(repo, "untracked.txt"))

	registration, err := Register(ctx, pool, filepath.Join(repo, "nested"), filepath.Join(t.TempDir(), "managed"))
	if err != nil {
		t.Fatal(err)
	}
	cleanupRegistration(t, pool, registration)
	if registration.CanonicalSourcePath != canonical || registration.CurrentCommit != trimGit(headBefore) || registration.CurrentTree != trimGit(treeBefore) {
		t.Fatalf("registration identity = %#v, want source %q commit %q tree %q", registration, canonical, trimGit(headBefore), trimGit(treeBefore))
	}

	queries := postgres.New(pool)
	stored, err := queries.GetProjectRegistrationByCanonicalSourcePath(ctx, canonical)
	if err != nil {
		t.Fatal(err)
	}
	if uuidString(stored.ProjectID) != registration.ProjectID || uuidString(stored.ProjectSourceID) != registration.ProjectSourceID {
		t.Fatalf("stored IDs = %s/%s, want %s/%s", uuidString(stored.ProjectID), uuidString(stored.ProjectSourceID), registration.ProjectID, registration.ProjectSourceID)
	}
	if stored.Name != filepath.Base(canonical) || stored.Status != "registered" ||
		stored.CanonicalSourcePath != canonical || stored.ManagedRepositoryPath != registration.ManagedRepositoryPath ||
		stored.CurrentCommit != registration.CurrentCommit || stored.CurrentTree != registration.CurrentTree {
		t.Fatalf("stored registration = %#v, want %#v", stored, registration)
	}
	if !stored.CurrentBranch.Valid || stored.CurrentBranch.String != "main" || !stored.DefaultBranch.Valid || stored.DefaultBranch.String != "main" {
		t.Fatalf("stored branches = %#v/%#v, want main/main", stored.CurrentBranch, stored.DefaultBranch)
	}
	if !stored.CreatedAt.Time.Equal(registration.CreatedAt) || !stored.UpdatedAt.Time.Equal(registration.CreatedAt) {
		t.Fatalf("stored timestamps = %s/%s, want %s", stored.CreatedAt.Time, stored.UpdatedAt.Time, registration.CreatedAt)
	}
	var storedDirty DirtyState
	if err := json.Unmarshal(stored.DirtyState, &storedDirty); err != nil {
		t.Fatal(err)
	}
	if !storedDirty.Dirty || !bytes.Equal(storedDirty.PorcelainV1Z, statusBefore) {
		t.Fatalf("stored dirty state = %#v, want exact status %q", storedDirty, statusBefore)
	}
	wantRemotes := []Remote{{
		Name: "origin", FetchURLs: []string{"https://example.invalid/revolvr-test.git"},
		PushURLs: []string{"ssh://git@example.invalid/revolvr-test.git"},
	}}
	var storedRemotes []Remote
	if err := json.Unmarshal(stored.Remotes, &storedRemotes); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(storedRemotes, wantRemotes) {
		t.Fatalf("stored remotes = %#v, want %#v", storedRemotes, wantRemotes)
	}

	event, err := queries.GetEvent(ctx, projectUUID(registration.EventID))
	if err != nil {
		t.Fatal(err)
	}
	if event.EventType != "project.registered" || event.AggregateType != "project" || event.AggregateVersion != 1 ||
		uuidString(event.ProjectID) != registration.ProjectID || uuidString(event.AggregateID) != registration.ProjectID {
		t.Fatalf("stored event = %#v", event)
	}
	var payload registeredEvent
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ProjectID != registration.ProjectID || payload.ProjectSourceID != registration.ProjectSourceID ||
		payload.CanonicalSourcePath != canonical || payload.ManagedRepositoryPath != registration.ManagedRepositoryPath ||
		payload.CurrentCommit != registration.CurrentCommit || payload.CurrentTree != registration.CurrentTree ||
		!reflect.DeepEqual(payload.DirtyState, storedDirty) || !reflect.DeepEqual(payload.Remotes, wantRemotes) {
		t.Fatalf("project.registered payload = %#v, want exact registration metadata", payload)
	}

	if got := trimGit(string(runProjectGit(t, registration.ManagedRepositoryPath, "rev-parse", "HEAD^{commit}"))); got != registration.CurrentCommit {
		t.Fatalf("managed HEAD = %q, want %q", got, registration.CurrentCommit)
	}
	if got := trimGit(string(runProjectGit(t, registration.ManagedRepositoryPath, "rev-parse", "HEAD^{tree}"))); got != registration.CurrentTree {
		t.Fatalf("managed tree = %q, want %q", got, registration.CurrentTree)
	}
	if got := runProjectGit(t, registration.ManagedRepositoryPath, "show", "HEAD:tracked.txt"); !bytes.Equal(got, committed) {
		t.Fatalf("managed tracked bytes = %q, want committed bytes %q", got, committed)
	}
	projectGitMustFail(t, registration.ManagedRepositoryPath, "cat-file", "-e", "HEAD:untracked.txt")

	if got := string(runProjectGit(t, repo, "rev-parse", "HEAD")); got != headBefore {
		t.Fatalf("operator HEAD changed from %q to %q", headBefore, got)
	}
	if got := runProjectGit(t, repo, "status", "--porcelain=v1", "-z", "--untracked-files=all"); !bytes.Equal(got, statusBefore) {
		t.Fatalf("operator status changed from %q to %q", statusBefore, got)
	}
	if got := readProjectFile(t, filepath.Join(repo, "tracked.txt")); !bytes.Equal(got, trackedBefore) {
		t.Fatalf("operator tracked bytes changed from %q to %q", trackedBefore, got)
	}
	if got := readProjectFile(t, filepath.Join(repo, "untracked.txt")); !bytes.Equal(got, untrackedBefore) {
		t.Fatalf("operator untracked bytes changed from %q to %q", untrackedBefore, got)
	}

	alias := filepath.Join(t.TempDir(), "source-alias")
	if err := os.Symlink(repo, alias); err != nil {
		t.Fatal(err)
	}
	for _, duplicate := range []string{filepath.Join(repo, "nested"), alias} {
		if _, err := Register(ctx, pool, duplicate, filepath.Join(t.TempDir(), "managed")); !errors.Is(err, ErrAlreadyRegistered) {
			t.Fatalf("Register(%q) error = %v, want %v", duplicate, err, ErrAlreadyRegistered)
		}
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM core.project_sources WHERE canonical_source_path = $1", canonical).Scan(&count); err != nil || count != 1 {
		t.Fatalf("registered source count = %d, err = %v, want 1", count, err)
	}
}

func TestRegisterRetryAdoptsMirrorAfterTransactionRollback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := projectTestPool(t, ctx)
	repo := projectTestRepository(t)
	managedRoot := filepath.Join(t.TempDir(), "managed")
	canonical, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	destination, err := managedDestination(managedRoot, canonical)
	if err != nil {
		t.Fatal(err)
	}
	before := registrationCounts(t, ctx, pool)
	enableRegistrationFailure(t, ctx, pool)

	if _, err := Register(ctx, pool, repo, managedRoot); err == nil {
		t.Fatal("Register() succeeded with forced event failure")
	}
	if after := registrationCounts(t, ctx, pool); after != before {
		t.Fatalf("row counts after rollback = %#v, want %#v", after, before)
	}
	first, err := os.Stat(destination)
	if err != nil || !first.IsDir() {
		t.Fatalf("managed mirror after rollback: info = %#v, err = %v", first, err)
	}
	if got := trimGit(string(runProjectGit(t, destination, "rev-parse", "HEAD^{commit}"))); got != trimGit(string(runProjectGit(t, repo, "rev-parse", "HEAD^{commit}"))) {
		t.Fatalf("managed mirror HEAD after rollback = %q", got)
	}

	disableRegistrationFailure(t, ctx, pool)
	registration, err := Register(ctx, pool, repo, managedRoot)
	if err != nil {
		t.Fatal(err)
	}
	cleanupRegistration(t, pool, registration)
	second, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(first, second) {
		t.Fatal("retry replaced the complete matching managed mirror instead of adopting it")
	}
}

func TestRegisterRejectsUnusableRepositoriesAndUnsafeDestination(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := projectTestPool(t, ctx)

	nonGit := t.TempDir()
	unborn := filepath.Join(t.TempDir(), "unborn")
	runProjectGit(t, "", "init", "-q", unborn)
	bare := filepath.Join(t.TempDir(), "bare.git")
	runProjectGit(t, "", "init", "-q", "--bare", bare)
	for _, path := range []string{nonGit, unborn, bare} {
		if _, err := Register(ctx, pool, path, filepath.Join(t.TempDir(), "managed")); !errors.Is(err, ErrUnusableRepository) {
			t.Fatalf("Register(%q) error = %v, want %v", path, err, ErrUnusableRepository)
		}
	}

	repo := projectTestRepository(t)
	canonical, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	managedRoot := filepath.Join(t.TempDir(), "managed")
	destination, err := managedDestination(managedRoot, canonical)
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, destination); err != nil {
		t.Fatal(err)
	}
	if _, err := Register(ctx, pool, repo, managedRoot); !errors.Is(err, ErrManagedConflict) {
		t.Fatalf("Register() unsafe-destination error = %v, want %v", err, ErrManagedConflict)
	}
	if _, err := postgres.New(pool).GetProjectRegistrationByCanonicalSourcePath(ctx, canonical); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("registration after unsafe destination error = %v, want no rows", err)
	}
}

type rowCounts struct {
	projects int64
	sources  int64
	events   int64
}

func registrationCounts(t *testing.T, ctx context.Context, pool *pgxpool.Pool) rowCounts {
	t.Helper()
	var counts rowCounts
	err := pool.QueryRow(ctx, `SELECT
        (SELECT count(*) FROM core.projects),
        (SELECT count(*) FROM core.project_sources),
        (SELECT count(*) FROM core.events WHERE event_type = 'project.registered')`).Scan(&counts.projects, &counts.sources, &counts.events)
	if err != nil {
		t.Fatal(err)
	}
	return counts
}

func enableRegistrationFailure(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	disableRegistrationFailure(t, ctx, pool)
	_, err := pool.Exec(ctx, `CREATE FUNCTION core.fail_project_registration_test() RETURNS trigger
        LANGUAGE plpgsql AS $$
        BEGIN
            IF NEW.event_type = 'project.registered' THEN
                RAISE EXCEPTION 'forced project registration failure';
            END IF;
            RETURN NEW;
        END
        $$`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE TRIGGER fail_project_registration_test
        BEFORE INSERT ON core.events
        FOR EACH ROW EXECUTE FUNCTION core.fail_project_registration_test()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { disableRegistrationFailure(t, context.Background(), pool) })
}

func disableRegistrationFailure(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, "DROP TRIGGER IF EXISTS fail_project_registration_test ON core.events"); err != nil {
		t.Error(err)
	}
	if _, err := pool.Exec(ctx, "DROP FUNCTION IF EXISTS core.fail_project_registration_test()"); err != nil {
		t.Error(err)
	}
}

func projectTestPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("REVOLVR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("REVOLVR_TEST_DATABASE_URL is not set")
	}
	pool, err := postgres.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func projectTestRepository(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "source")
	runProjectGit(t, "", "init", "-q", "-b", "main", repo)
	runProjectGit(t, repo, "config", "user.name", "Revolvr Test")
	runProjectGit(t, repo, "config", "user.email", "revolvr@example.invalid")
	writeProjectFile(t, filepath.Join(repo, "tracked.txt"), []byte("committed bytes\n"))
	writeProjectFile(t, filepath.Join(repo, "nested", ".keep"), []byte("nested\n"))
	runProjectGit(t, repo, "add", "tracked.txt", "nested/.keep")
	runProjectGit(t, repo, "commit", "-q", "-m", "initial")
	return repo
}

func cleanupRegistration(t *testing.T, pool *pgxpool.Pool, registration Registration) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := pool.Exec(ctx, "DELETE FROM core.events WHERE id = $1", registration.EventID); err != nil {
			t.Error(err)
		}
		if _, err := pool.Exec(ctx, "DELETE FROM core.project_sources WHERE id = $1", registration.ProjectSourceID); err != nil {
			t.Error(err)
		}
		if _, err := pool.Exec(ctx, "DELETE FROM core.projects WHERE id = $1", registration.ProjectID); err != nil {
			t.Error(err)
		}
	})
}

func runProjectGit(t *testing.T, dir string, args ...string) []byte {
	t.Helper()
	commandArgs := args
	if dir != "" {
		commandArgs = append([]string{"-C", dir}, args...)
	}
	cmd := exec.Command("git", commandArgs...)
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", commandArgs, err)
	}
	return out
}

func projectGitMustFail(t *testing.T, dir string, args ...string) {
	t.Helper()
	commandArgs := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", commandArgs...)
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	if err := cmd.Run(); err == nil {
		t.Fatalf("git %v unexpectedly succeeded", commandArgs)
	}
}

func writeProjectFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readProjectFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func projectUUID(value string) pgtype.UUID {
	return pgtype.UUID{Bytes: uuid.MustParse(value), Valid: true}
}

func trimGit(value string) string {
	return string(bytes.TrimSpace([]byte(value)))
}
