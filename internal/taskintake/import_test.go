package taskintake

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"revolvr/internal/storage/postgres"
)

func TestImportArbitrarySourcePreservesExactBytes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool := taskIntakeTestPool(t, ctx)
	projectID := taskIntakeTestProject(t, ctx, pool)
	root := taskIntakeArtifactRoot(t)
	source := append([]byte{0x00, 0xff, 0x01}, []byte(" arbitrary source\r\n")...)

	result, err := Import(ctx, pool, uuidString(projectID), root, "request.bundle", "application/octet-stream", source)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "needs_compilation" || result.TaskID != "" || result.Replayed {
		t.Fatalf("result = %#v, want needs_compilation without task", result)
	}
	if got, err := os.ReadFile(result.ArtifactPath); err != nil || !bytes.Equal(got, source) {
		t.Fatalf("artifact bytes = %x, err = %v, want %x", got, err, source)
	}

	queries := postgres.New(pool)
	stored, err := queries.GetTaskImport(ctx, mustUUID(t, result.ImportID))
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != "needs_compilation" || stored.TaskID.Valid || stored.SourceSha256 != result.SourceSHA256 {
		t.Fatalf("stored import = %#v", stored)
	}
	artifact, err := queries.GetArtifactBySHA256(ctx, result.SourceSHA256)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.SizeBytes != int64(len(source)) || artifact.MediaType != "application/octet-stream" ||
		artifact.LogicalKind != "task_source" || artifact.StoragePath != result.ArtifactPath {
		t.Fatalf("stored artifact = %#v", artifact)
	}
	counts := projectCounts(t, ctx, pool, projectID)
	if counts.imports != 1 || counts.tasks != 0 || counts.versions != 0 || counts.events != 1 {
		t.Fatalf("project counts = %#v", counts)
	}
}

func TestImportRejectsEmptyAndOversizedSources(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool := taskIntakeTestPool(t, ctx)
	projectID := taskIntakeTestProject(t, ctx, pool)
	root := taskIntakeArtifactRoot(t)
	for _, test := range []struct {
		name   string
		source []byte
	}{
		{"empty", nil},
		{"oversized", make([]byte, MaximumSourceBytes+1)},
	} {
		if _, err := Import(ctx, pool, uuidString(projectID), root, test.name, "application/octet-stream", test.source); err == nil {
			t.Fatalf("Import(%s) succeeded, want size error", test.name)
		}
	}
	if counts := projectCounts(t, ctx, pool, projectID); counts != (intakeCounts{}) {
		t.Fatalf("counts after rejected sizes = %#v", counts)
	}
}

func TestImportCanonicalTaskStoresNormalizedDraftContract(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool := taskIntakeTestPool(t, ctx)
	projectID := taskIntakeTestProject(t, ctx, pool)
	dependencyID := insertDraftTask(t, ctx, pool, projectID, "dependency-task")
	conflictID := insertDraftTask(t, ctx, pool, projectID, "conflicting-task")
	root := taskIntakeArtifactRoot(t)
	source := canonicalTask("import-task", "[dependency-task]", "[conflicting-task]", validCriteria())

	result, err := Import(ctx, pool, uuidString(projectID), root, "import-task.md", "text/markdown; charset=utf-8", source)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "draft" || result.TaskID == "" || result.Replayed {
		t.Fatalf("result = %#v, want new draft task", result)
	}
	if got, err := os.ReadFile(result.ArtifactPath); err != nil || !bytes.Equal(got, source) {
		t.Fatalf("artifact bytes differ: err = %v", err)
	}

	queries := postgres.New(pool)
	selected, err := queries.GetTaskWithSelectedVersionByExternalID(ctx, postgres.GetTaskWithSelectedVersionByExternalIDParams{
		ProjectID: projectID, ExternalTaskID: "import-task",
	})
	if err != nil {
		t.Fatal(err)
	}
	if selected.Status != "draft" || selected.AcceptedVersionID.Valid || selected.SelectedVersionID.Valid {
		t.Fatalf("selected task = %#v, want draft without accepted version", selected)
	}

	var (
		versionID, sourceArtifactID pgtype.UUID
		versionNumber               int32
		title, goal                 string
		risk, mutation, network     string
		priority                    int32
		readOnly                    bool
		scope, excluded, plan       []byte
		budget, secrets, paths      []byte
		checkpoints                 []byte
	)
	err = pool.QueryRow(ctx, `SELECT id, version_number, source_artifact_id, title, goal,
        risk_class, mutation_class, network_profile, priority, read_only_investigation,
        scope, excluded_scope, verification_plan, budget, secret_requirements,
        expected_paths, operator_checkpoints
        FROM core.task_versions WHERE task_id = $1`, result.TaskID).Scan(
		&versionID, &versionNumber, &sourceArtifactID, &title, &goal, &risk, &mutation,
		&network, &priority, &readOnly, &scope, &excluded, &plan, &budget, &secrets, &paths, &checkpoints,
	)
	if err != nil {
		t.Fatal(err)
	}
	if versionNumber != 1 || title != "Import canonical tasks" || goal != "Store one validated task contract." ||
		risk != "medium" || mutation != "bounded_source" || network != "none" || priority != 100 || readOnly {
		t.Fatalf("task version scalar data = version %d title %q goal %q risk %q mutation %q network %q priority %d readonly %v",
			versionNumber, title, goal, risk, mutation, network, priority, readOnly)
	}
	assertJSON(t, scope, []any{"Parser.", "PostgreSQL persistence."})
	assertJSON(t, excluded, []any{"Lifecycle approval."})
	assertJSON(t, budget, map[string]any{"max_cycles": float64(8), "max_model_tokens": float64(500000), "max_wall_time": "2h0m0s"})
	assertJSON(t, secrets, []any{"TEST_TOKEN"})
	assertJSON(t, paths, []any{"internal/taskintake/**", "db/**"})
	var planEntries []map[string]any
	if err := json.Unmarshal(plan, &planEntries); err != nil || len(planEntries) != 2 ||
		planEntries[0]["method"] != "command" || planEntries[1]["method"] != "operator_checkpoint" {
		t.Fatalf("verification plan = %s, err = %v", plan, err)
	}
	var checkpointEntries []map[string]string
	if err := json.Unmarshal(checkpoints, &checkpointEntries); err != nil || len(checkpointEntries) != 1 || checkpointEntries[0]["criterion_id"] != "AC-2" {
		t.Fatalf("operator checkpoints = %s, err = %v", checkpoints, err)
	}

	var gotDependency, gotConflict pgtype.UUID
	if err := pool.QueryRow(ctx, "SELECT dependency_task_id FROM core.task_dependencies WHERE task_version_id = $1", versionID).Scan(&gotDependency); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT conflicting_task_id FROM core.task_conflicts WHERE task_version_id = $1", versionID).Scan(&gotConflict); err != nil {
		t.Fatal(err)
	}
	if gotDependency != dependencyID || gotConflict != conflictID {
		t.Fatalf("graph = dependency %s conflict %s", uuidString(gotDependency), uuidString(gotConflict))
	}

	rows, err := pool.Query(ctx, `SELECT c.external_criterion_id, c.status, v.version_number,
        v.requirement, v.verification_method, v.verification_reference, v.operator_checkpoint
        FROM core.task_acceptance_criteria AS c
        JOIN core.task_acceptance_versions AS v ON v.criterion_id = c.id
        WHERE c.task_id = $1 ORDER BY c.external_criterion_id`, result.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type storedCriterion struct {
		id, status, requirement, method string
		version                         int32
		reference                       pgtype.Text
		checkpoint                      []byte
	}
	var criteria []storedCriterion
	for rows.Next() {
		var criterion storedCriterion
		if err := rows.Scan(&criterion.id, &criterion.status, &criterion.version, &criterion.requirement, &criterion.method, &criterion.reference, &criterion.checkpoint); err != nil {
			t.Fatal(err)
		}
		criteria = append(criteria, criterion)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(criteria) != 2 || criteria[0].id != "AC-1" || criteria[0].status != "pending" || criteria[0].version != 1 ||
		criteria[0].requirement != "The task is stored as a draft." || criteria[0].method != "command" ||
		!criteria[0].reference.Valid || criteria[0].reference.String != "go test ./internal/taskintake" ||
		criteria[1].method != "operator_checkpoint" || criteria[1].reference.Valid {
		t.Fatalf("criteria = %#v", criteria)
	}
	assertJSON(t, criteria[1].checkpoint, map[string]any{"description": "Confirm the imported scope is correct."})

	artifact, err := queries.GetArtifactBySHA256(ctx, result.SourceSHA256)
	if err != nil || artifact.ID != sourceArtifactID || artifact.StoragePath != result.ArtifactPath {
		t.Fatalf("artifact = %#v, err = %v", artifact, err)
	}
	var eventCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM core.events
        WHERE project_id = $1 AND task_id = $2 AND event_type = 'task_import.created'`, projectID, result.TaskID).Scan(&eventCount); err != nil || eventCount != 1 {
		t.Fatalf("event count = %d, err = %v", eventCount, err)
	}
}

func TestImportCanonicalValidationFailsWithoutPartialState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool := taskIntakeTestPool(t, ctx)
	projectID := taskIntakeTestProject(t, ctx, pool)
	root := taskIntakeArtifactRoot(t)
	before := projectCounts(t, ctx, pool, projectID)

	tests := []struct {
		name   string
		source []byte
	}{
		{"unknown frontmatter key", bytes.Replace(canonicalTask("bad-task", "[]", "[]", validCriteria()), []byte("priority: 100"), []byte("priority: 100\nprioroty: 100"), 1)},
		{"missing goal", bytes.Replace(canonicalTask("bad-task", "[]", "[]", validCriteria()), []byte("## Goal\n\nStore one validated task contract.\n\n"), nil, 1)},
		{"unsupported mutation", bytes.Replace(canonicalTask("bad-task", "[]", "[]", validCriteria()), []byte("mutation_class: bounded_source"), []byte("mutation_class: magic"), 1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := strings.ReplaceAll(tt.name, " ", "-") + ".md"
			if _, err := Import(ctx, pool, uuidString(projectID), root, name, "text/markdown", tt.source); err == nil {
				t.Fatal("Import() succeeded, want validation error")
			}
			if after := projectCounts(t, ctx, pool, projectID); after != before {
				t.Fatalf("project counts after rejection = %#v, want %#v", after, before)
			}
			hash := fmt.Sprintf("%x", sha256.Sum256(tt.source))
			if _, err := postgres.New(pool).GetArtifactBySHA256(ctx, hash); !errors.Is(err, pgx.ErrNoRows) {
				t.Fatalf("artifact metadata after rejection error = %v, want no row", err)
			}
		})
	}
}

func TestImportRejectsInvalidGraphAndDuplicateCriteria(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool := taskIntakeTestPool(t, ctx)
	projectID := taskIntakeTestProject(t, ctx, pool)
	insertDraftTask(t, ctx, pool, projectID, "dependency-task")
	insertDraftTask(t, ctx, pool, projectID, "conflicting-task")
	root := taskIntakeArtifactRoot(t)
	before := projectCounts(t, ctx, pool, projectID)

	tests := []struct {
		name, dependencies, conflicts string
		criteria                      string
	}{
		{"duplicate dependency", "[dependency-task, dependency-task]", "[]", validCriteria()},
		{"self dependency", "[graph-task]", "[]", validCriteria()},
		{"missing dependency", "[missing-task]", "[]", validCriteria()},
		{"duplicate conflict", "[]", "[conflicting-task, conflicting-task]", validCriteria()},
		{"self conflict", "[]", "[graph-task]", validCriteria()},
		{"missing conflict", "[]", "[missing-task]", validCriteria()},
		{"duplicate criteria", "[]", "[]", validCriteria() + "\n### AC-1\n\nDuplicate.\n\nVerification:\n\n`true`\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := canonicalTask("graph-task", tt.dependencies, tt.conflicts, tt.criteria)
			if _, err := Import(ctx, pool, uuidString(projectID), root, strings.ReplaceAll(tt.name, " ", "-")+".md", "text/markdown", source); err == nil {
				t.Fatal("Import() succeeded, want validation error")
			}
			if after := projectCounts(t, ctx, pool, projectID); after != before {
				t.Fatalf("project counts after rejection = %#v, want %#v", after, before)
			}
		})
	}
}

func TestImportRetryAdoptsMatchingArtifactAndReturnsExistingImport(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool := taskIntakeTestPool(t, ctx)
	projectID := taskIntakeTestProject(t, ctx, pool)
	root := taskIntakeArtifactRoot(t)
	source := []byte("retryable arbitrary source " + uuidString(projectID))
	hash := fmt.Sprintf("%x", sha256.Sum256(source))
	path := artifactTestPath(root, hash)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, source, 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	first, err := Import(ctx, pool, uuidString(projectID), root, "retry.txt", "text/plain", source)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) || after.Mode().Perm() != 0o444 {
		t.Fatalf("adopted artifact info = mode %o same %v", after.Mode().Perm(), os.SameFile(before, after))
	}
	second, err := Import(ctx, pool, uuidString(projectID), root, "retry.txt", "text/plain", source)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Replayed || second.ImportID != first.ImportID || second.ArtifactID != first.ArtifactID {
		t.Fatalf("replay = %#v, first = %#v", second, first)
	}
	if _, err := Import(ctx, pool, uuidString(projectID), root, "retry.txt", "text/plain", append(source, '!')); !errors.Is(err, ErrImportConflict) {
		t.Fatalf("changed replay error = %v, want %v", err, ErrImportConflict)
	}
	counts := projectCounts(t, ctx, pool, projectID)
	if counts.imports != 1 || counts.events != 1 {
		t.Fatalf("counts after replay = %#v", counts)
	}
}

func TestImportRejectsMismatchedAndSymlinkedArtifacts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool := taskIntakeTestPool(t, ctx)
	projectID := taskIntakeTestProject(t, ctx, pool)
	root := taskIntakeArtifactRoot(t)

	mismatchedSource := []byte("mismatched destination " + uuidString(projectID))
	mismatchedHash := fmt.Sprintf("%x", sha256.Sum256(mismatchedSource))
	mismatchedPath := artifactTestPath(root, mismatchedHash)
	if err := os.MkdirAll(filepath.Dir(mismatchedPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mismatchedPath, []byte("wrong bytes"), 0o444); err != nil {
		t.Fatal(err)
	}
	if _, err := Import(ctx, pool, uuidString(projectID), root, "mismatch.txt", "text/plain", mismatchedSource); err == nil || !strings.Contains(err.Error(), "mismatched bytes") {
		t.Fatalf("mismatched artifact error = %v", err)
	}

	symlinkSource := []byte("symlink destination " + uuidString(projectID))
	symlinkHash := fmt.Sprintf("%x", sha256.Sum256(symlinkSource))
	symlinkPath := artifactTestPath(root, symlinkHash)
	if err := os.MkdirAll(filepath.Dir(symlinkPath), 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, symlinkSource, 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, symlinkPath); err != nil {
		t.Fatal(err)
	}
	if _, err := Import(ctx, pool, uuidString(projectID), root, "symlink.txt", "text/plain", symlinkSource); !errors.Is(err, ErrUnsafeArtifact) {
		t.Fatalf("symlink artifact error = %v, want %v", err, ErrUnsafeArtifact)
	}
	if counts := projectCounts(t, ctx, pool, projectID); counts.imports != 0 || counts.tasks != 0 || counts.events != 0 {
		t.Fatalf("counts after unsafe artifacts = %#v", counts)
	}
}

func TestImportTransactionRollbackAndArtifactRecovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool := taskIntakeTestPool(t, ctx)
	projectID := taskIntakeTestProject(t, ctx, pool)
	insertDraftTask(t, ctx, pool, projectID, "dependency-task")
	insertDraftTask(t, ctx, pool, projectID, "conflicting-task")
	root := taskIntakeArtifactRoot(t)
	source := canonicalTask("rollback-task", "[dependency-task]", "[conflicting-task]", validCriteria())
	hash := fmt.Sprintf("%x", sha256.Sum256(source))
	path := artifactTestPath(root, hash)
	before := projectCounts(t, ctx, pool, projectID)
	enableTaskImportFailure(t, ctx, pool)

	if _, err := Import(ctx, pool, uuidString(projectID), root, "rollback.md", "text/markdown", source); err == nil {
		t.Fatal("Import() succeeded with forced event failure")
	}
	if after := projectCounts(t, ctx, pool, projectID); after != before {
		t.Fatalf("counts after rollback = %#v, want %#v", after, before)
	}
	if _, err := postgres.New(pool).GetArtifactBySHA256(ctx, hash); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("artifact metadata after rollback error = %v, want no row", err)
	}
	firstInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("materialized artifact after rollback: %v", err)
	}

	disableTaskImportFailure(t, ctx, pool)
	result, err := Import(ctx, pool, uuidString(projectID), root, "rollback.md", "text/markdown", source)
	if err != nil {
		t.Fatal(err)
	}
	secondInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if result.Replayed || !os.SameFile(firstInfo, secondInfo) {
		t.Fatalf("recovery result = %#v, artifact replaced = %v", result, !os.SameFile(firstInfo, secondInfo))
	}
	after := projectCounts(t, ctx, pool, projectID)
	if after.imports != before.imports+1 || after.tasks != before.tasks+1 || after.versions != before.versions+1 ||
		after.dependencies != before.dependencies+1 || after.conflicts != before.conflicts+1 ||
		after.criteria != before.criteria+2 || after.acceptanceVersions != before.acceptanceVersions+2 || after.events != before.events+1 {
		t.Fatalf("counts after recovery = %#v, before %#v", after, before)
	}
}

func canonicalTask(id, dependencies, conflicts, criteria string) []byte {
	return []byte(fmt.Sprintf(`---
schema: revolvr-task-v1
id: %s
priority: 100
mutation_class: bounded_source
risk: medium
network: none
depends_on: %s
conflicts: %s
expected_paths:
  - internal/taskintake/**
  - db/**
budget:
  max_cycles: 8
  max_model_tokens: 500000
  max_wall_time: 120m
secret_requirements:
  - TEST_TOKEN
---

# Import canonical tasks

## Goal

Store one validated task contract.

## Scope

- Parser.
- PostgreSQL persistence.

## Excluded Scope

- Lifecycle approval.

## Acceptance

%s`, id, dependencies, conflicts, criteria))
}

func validCriteria() string {
	return `### AC-1

The task is stored as a draft.

Verification:

` + "```text\n" + `go test ./internal/taskintake
` + "```" + `

### AC-2

The operator can inspect imported scope.

Operator Checkpoint:

Confirm the imported scope is correct.
`
}

type intakeCounts struct {
	imports, tasks, versions, dependencies, conflicts, criteria, acceptanceVersions, events int64
}

func projectCounts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, projectID pgtype.UUID) intakeCounts {
	t.Helper()
	var counts intakeCounts
	err := pool.QueryRow(ctx, `SELECT
        (SELECT count(*) FROM core.task_imports WHERE project_id = $1),
        (SELECT count(*) FROM core.tasks WHERE project_id = $1),
        (SELECT count(*) FROM core.task_versions AS v JOIN core.tasks AS t ON t.id = v.task_id WHERE t.project_id = $1),
        (SELECT count(*) FROM core.task_dependencies WHERE project_id = $1),
        (SELECT count(*) FROM core.task_conflicts WHERE project_id = $1),
        (SELECT count(*) FROM core.task_acceptance_criteria AS c JOIN core.tasks AS t ON t.id = c.task_id WHERE t.project_id = $1),
        (SELECT count(*) FROM core.task_acceptance_versions AS v JOIN core.tasks AS t ON t.id = v.task_id WHERE t.project_id = $1),
        (SELECT count(*) FROM core.events WHERE project_id = $1)`, projectID).Scan(
		&counts.imports, &counts.tasks, &counts.versions, &counts.dependencies,
		&counts.conflicts, &counts.criteria, &counts.acceptanceVersions, &counts.events,
	)
	if err != nil {
		t.Fatal(err)
	}
	return counts
}

func taskIntakeTestPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
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

func taskIntakeTestProject(t *testing.T, ctx context.Context, pool *pgxpool.Pool) pgtype.UUID {
	t.Helper()
	projectID := newUUID()
	now := timestamp(time.Now().UTC().Truncate(time.Microsecond))
	if _, err := postgres.New(pool).InsertProject(ctx, postgres.InsertProjectParams{
		ID: projectID, Name: "task-intake-" + uuidString(projectID), Status: "registered",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupTaskIntakeProject(t, pool, projectID) })
	return projectID
}

func insertDraftTask(t *testing.T, ctx context.Context, pool *pgxpool.Pool, projectID pgtype.UUID, externalID string) pgtype.UUID {
	t.Helper()
	taskID := newUUID()
	now := timestamp(time.Now().UTC().Truncate(time.Microsecond))
	if _, err := postgres.New(pool).InsertTask(ctx, postgres.InsertTaskParams{
		ID: taskID, ProjectID: projectID, ExternalTaskID: externalID, Status: "draft",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	return taskID
}

func taskIntakeArtifactRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "data", "artifacts")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func artifactTestPath(root, hash string) string {
	return filepath.Join(root, "sha256", hash[:2], hash[2:4], hash)
}

func mustUUID(t *testing.T, value string) pgtype.UUID {
	t.Helper()
	parsed, err := parseUUID(value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func assertJSON(t *testing.T, raw []byte, want any) {
	t.Helper()
	var got any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode JSON %s: %v", raw, err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotJSON, wantJSON) {
		t.Fatalf("JSON = %s, want %s", gotJSON, wantJSON)
	}
}

func enableTaskImportFailure(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	disableTaskImportFailure(t, ctx, pool)
	if _, err := pool.Exec(ctx, `CREATE FUNCTION core.fail_task_import_test() RETURNS trigger
        LANGUAGE plpgsql AS $$
        BEGIN
            IF NEW.event_type = 'task_import.created' THEN
                RAISE EXCEPTION 'forced task import failure';
            END IF;
            RETURN NEW;
        END
        $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE TRIGGER fail_task_import_test
        BEFORE INSERT ON core.events FOR EACH ROW EXECUTE FUNCTION core.fail_task_import_test()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { disableTaskImportFailure(t, context.Background(), pool) })
}

func disableTaskImportFailure(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, "DROP TRIGGER IF EXISTS fail_task_import_test ON core.events"); err != nil {
		t.Error(err)
	}
	if _, err := pool.Exec(ctx, "DROP FUNCTION IF EXISTS core.fail_task_import_test()"); err != nil {
		t.Error(err)
	}
}

func cleanupTaskIntakeProject(t *testing.T, pool *pgxpool.Pool, projectID pgtype.UUID) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := pool.Query(ctx, `SELECT source_artifact_id FROM core.task_imports WHERE project_id = $1
        UNION SELECT v.source_artifact_id FROM core.task_versions AS v
        JOIN core.tasks AS t ON t.id = v.task_id WHERE t.project_id = $1`, projectID)
	if err != nil {
		t.Error(err)
		return
	}
	var artifacts []pgtype.UUID
	for rows.Next() {
		var artifactID pgtype.UUID
		if err := rows.Scan(&artifactID); err != nil {
			t.Error(err)
			rows.Close()
			return
		}
		artifacts = append(artifacts, artifactID)
	}
	rows.Close()
	statements := []string{
		"DELETE FROM core.task_imports WHERE project_id = $1",
		"DELETE FROM core.task_acceptance_versions WHERE task_id IN (SELECT id FROM core.tasks WHERE project_id = $1)",
		"DELETE FROM core.task_acceptance_criteria WHERE task_id IN (SELECT id FROM core.tasks WHERE project_id = $1)",
		"DELETE FROM core.task_conflicts WHERE project_id = $1",
		"DELETE FROM core.task_dependencies WHERE project_id = $1",
		"UPDATE core.tasks SET accepted_version_id = NULL WHERE project_id = $1",
		"DELETE FROM core.task_versions WHERE task_id IN (SELECT id FROM core.tasks WHERE project_id = $1)",
		"DELETE FROM core.tasks WHERE project_id = $1",
		"DELETE FROM core.events WHERE project_id = $1",
		"DELETE FROM core.projects WHERE id = $1",
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement, projectID); err != nil {
			t.Error(err)
			return
		}
	}
	for _, artifactID := range artifacts {
		if _, err := pool.Exec(ctx, "DELETE FROM core.artifacts WHERE id = $1", artifactID); err != nil {
			t.Error(err)
		}
	}
}
