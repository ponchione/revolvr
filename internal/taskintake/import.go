package taskintake

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"revolvr/internal/id"
	"revolvr/internal/storage/postgres"
)

var (
	ErrImportConflict = errors.New("task import conflicts with existing admitted identity")
	ErrUnsafeArtifact = errors.New("unsafe artifact destination")
)

type Result struct {
	ImportID     string
	ArtifactID   string
	TaskID       string
	Status       string
	SourceSHA256 string
	ArtifactPath string
	Replayed     bool
}

// Import retains at most MaximumSourceBytes bytes beneath an existing,
// non-symlink artifact root and atomically records their canonical state.
func Import(ctx context.Context, pool *pgxpool.Pool, projectID, artifactRoot, sourceName, mediaType string, source []byte) (Result, error) {
	if pool == nil {
		return Result{}, errors.New("task intake: PostgreSQL pool is required")
	}
	if len(source) == 0 {
		return Result{}, errors.New("task intake: source is empty")
	}
	if len(source) > MaximumSourceBytes {
		return Result{}, fmt.Errorf("task intake: source is %d bytes; maximum is %d", len(source), MaximumSourceBytes)
	}
	if strings.TrimSpace(artifactRoot) == "" {
		return Result{}, errors.New("task intake: artifact root is required")
	}
	sourceName = strings.TrimSpace(sourceName)
	if sourceName == "" || len(sourceName) > 1024 || strings.ContainsRune(sourceName, 0) {
		return Result{}, errors.New("task intake: source name must contain at most 1024 bytes")
	}
	mediaType, err := normalizeMediaType(mediaType)
	if err != nil {
		return Result{}, err
	}
	projectUUID, err := parseUUID(projectID)
	if err != nil {
		return Result{}, fmt.Errorf("task intake: invalid project id: %w", err)
	}

	queries := postgres.New(pool)
	project, err := queries.GetProjectByID(ctx, projectUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Result{}, errors.New("task intake: project is not registered")
		}
		return Result{}, fmt.Errorf("task intake: get project: %w", err)
	}
	if project.Status != "registered" {
		return Result{}, fmt.Errorf("task intake: project status %q is not registered", project.Status)
	}

	hash := sha256.Sum256(source)
	hashText := hex.EncodeToString(hash[:])
	identity := postgres.GetTaskImportBySourceIdentityParams{ProjectID: projectUUID, SourceName: sourceName}
	if existing, err := queries.GetTaskImportBySourceIdentity(ctx, identity); err == nil {
		return replay(artifactRoot, mediaType, hashText, source, existing)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Result{}, fmt.Errorf("task intake: get existing import: %w", err)
	}

	contract, err := parseSource(source)
	if err != nil {
		return Result{}, fmt.Errorf("task intake: %w", err)
	}
	graph, err := validateGraph(ctx, queries, projectUUID, contract)
	if err != nil {
		return Result{}, err
	}
	artifactPath, err := materializeArtifact(artifactRoot, hashText, source)
	if err != nil {
		return Result{}, fmt.Errorf("task intake: materialize source artifact: %w", err)
	}

	createdAt := time.Now().UTC().Truncate(time.Microsecond)
	importID := newUUID()
	artifactID := newUUID()
	eventID := newUUID()
	var taskID, versionID pgtype.UUID
	status := "needs_compilation"
	if contract != nil {
		taskID, versionID, status = newUUID(), newUUID(), "draft"
	}

	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		qtx := queries.WithTx(tx)
		artifact, err := qtx.GetArtifactBySHA256(ctx, hashText)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			artifact, err = qtx.InsertArtifact(ctx, postgres.InsertArtifactParams{
				ID: artifactID, Sha256: hashText, SizeBytes: int64(len(source)),
				MediaType: mediaType, LogicalKind: "task_source", StoragePath: artifactPath,
				CreatedAt: timestamp(createdAt),
			})
		case err == nil:
			if artifact.SizeBytes != int64(len(source)) || artifact.MediaType != mediaType ||
				artifact.LogicalKind != "task_source" || artifact.StoragePath != artifactPath {
				return fmt.Errorf("%w: artifact metadata for sha256:%s", ErrImportConflict, hashText)
			}
		default:
			return err
		}
		if err != nil {
			return err
		}
		artifactID = artifact.ID

		if contract != nil {
			if err := insertContract(ctx, qtx, projectUUID, artifactID, taskID, versionID, createdAt, *contract, graph); err != nil {
				return err
			}
		}
		if _, err := qtx.InsertTaskImport(ctx, postgres.InsertTaskImportParams{
			ID: importID, ProjectID: projectUUID, SourceArtifactID: artifactID, TaskID: taskID,
			SourceName: sourceName, SourceSha256: hashText, MediaType: mediaType, Status: status,
			CreatedAt: timestamp(createdAt), UpdatedAt: timestamp(createdAt),
		}); err != nil {
			return err
		}
		payload, err := json.Marshal(map[string]any{
			"artifact_id": uuidString(artifactID), "project_id": uuidString(projectUUID),
			"source_name": sourceName, "source_sha256": hashText,
			"status": status, "task_id": nullableUUIDString(taskID),
		})
		if err != nil {
			return err
		}
		_, err = qtx.AppendEvent(ctx, postgres.AppendEventParams{
			ID: eventID, ProjectID: projectUUID, TaskID: taskID,
			EventType: "task_import.created", AggregateType: "task_import",
			AggregateID: importID, AggregateVersion: 1, Payload: payload,
			CreatedAt: timestamp(createdAt),
		})
		return err
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			if existing, getErr := queries.GetTaskImportBySourceIdentity(ctx, identity); getErr == nil {
				return replay(artifactRoot, mediaType, hashText, source, existing)
			}
		}
		return Result{}, fmt.Errorf("task intake: persist import: %w", err)
	}

	return Result{
		ImportID: uuidString(importID), ArtifactID: uuidString(artifactID),
		TaskID: nullableUUIDString(taskID), Status: status, SourceSHA256: hashText,
		ArtifactPath: artifactPath,
	}, nil
}

type graphReferences struct {
	dependencies []pgtype.UUID
	conflicts    []pgtype.UUID
}

func validateGraph(ctx context.Context, queries *postgres.Queries, projectID pgtype.UUID, contract *Contract) (graphReferences, error) {
	if contract == nil {
		return graphReferences{}, nil
	}
	lookup := func(externalID string) (pgtype.UUID, error) {
		row, err := queries.GetTaskWithSelectedVersionByExternalID(ctx, postgres.GetTaskWithSelectedVersionByExternalIDParams{
			ProjectID: projectID, ExternalTaskID: externalID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, fmt.Errorf("task intake: graph reference %q is missing from the project", externalID)
		}
		if err != nil {
			return pgtype.UUID{}, fmt.Errorf("task intake: resolve graph reference %q: %w", externalID, err)
		}
		return row.ID, nil
	}
	if _, err := queries.GetTaskWithSelectedVersionByExternalID(ctx, postgres.GetTaskWithSelectedVersionByExternalIDParams{
		ProjectID: projectID, ExternalTaskID: contract.ExternalTaskID,
	}); err == nil {
		return graphReferences{}, fmt.Errorf("%w: task %q already exists", ErrImportConflict, contract.ExternalTaskID)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return graphReferences{}, fmt.Errorf("task intake: check external task identity: %w", err)
	}

	graph := graphReferences{
		dependencies: make([]pgtype.UUID, 0, len(contract.Dependencies)),
		conflicts:    make([]pgtype.UUID, 0, len(contract.Conflicts)),
	}
	for _, externalID := range contract.Dependencies {
		id, err := lookup(externalID)
		if err != nil {
			return graphReferences{}, err
		}
		graph.dependencies = append(graph.dependencies, id)
	}
	for _, externalID := range contract.Conflicts {
		id, err := lookup(externalID)
		if err != nil {
			return graphReferences{}, err
		}
		graph.conflicts = append(graph.conflicts, id)
	}
	return graph, nil
}

func insertContract(ctx context.Context, queries *postgres.Queries, projectID, artifactID, taskID, versionID pgtype.UUID, createdAt time.Time, contract Contract, graph graphReferences) error {
	jsonValue := func(value any) ([]byte, error) { return json.Marshal(value) }
	scope, err := jsonValue(contract.Scope)
	if err != nil {
		return err
	}
	excludedScope, err := jsonValue(contract.ExcludedScope)
	if err != nil {
		return err
	}
	verificationPlan, err := jsonValue(contract.verificationPlan())
	if err != nil {
		return err
	}
	budget, err := jsonValue(contract.Budget)
	if err != nil {
		return err
	}
	secrets, err := jsonValue(contract.SecretRequirements)
	if err != nil {
		return err
	}
	expectedPaths, err := jsonValue(contract.ExpectedPaths)
	if err != nil {
		return err
	}
	checkpoints := make([]map[string]string, 0)
	for _, criterion := range contract.Criteria {
		if criterion.OperatorCheckpointText != "" {
			checkpoints = append(checkpoints, map[string]string{
				"criterion_id": criterion.ID, "description": criterion.OperatorCheckpointText,
			})
		}
	}
	operatorCheckpoints, err := jsonValue(checkpoints)
	if err != nil {
		return err
	}

	if _, err := queries.InsertTask(ctx, postgres.InsertTaskParams{
		ID: taskID, ProjectID: projectID, ExternalTaskID: contract.ExternalTaskID,
		Status: "draft", CreatedAt: timestamp(createdAt), UpdatedAt: timestamp(createdAt),
	}); err != nil {
		return err
	}
	if _, err := queries.InsertTaskVersion(ctx, postgres.InsertTaskVersionParams{
		ID: versionID, TaskID: taskID, VersionNumber: 1, SourceArtifactID: artifactID,
		Title: contract.Title, Goal: contract.Goal, RiskClass: contract.RiskClass,
		MutationClass: contract.MutationClass, NetworkProfile: contract.NetworkProfile,
		Priority: contract.Priority, ReadOnlyInvestigation: contract.ReadOnlyInvestigation,
		Scope: scope, ExcludedScope: excludedScope, VerificationPlan: verificationPlan,
		Budget: budget, SecretRequirements: secrets, ExpectedPaths: expectedPaths,
		OperatorCheckpoints: operatorCheckpoints, CreatedAt: timestamp(createdAt),
	}); err != nil {
		return err
	}
	for _, dependencyID := range graph.dependencies {
		if _, err := queries.InsertTaskDependency(ctx, postgres.InsertTaskDependencyParams{
			TaskVersionID: versionID, TaskID: taskID, ProjectID: projectID,
			DependencyTaskID: dependencyID, DependencyType: "requires", CreatedAt: timestamp(createdAt),
		}); err != nil {
			return err
		}
	}
	for _, conflictID := range graph.conflicts {
		if _, err := queries.InsertTaskConflict(ctx, postgres.InsertTaskConflictParams{
			TaskVersionID: versionID, TaskID: taskID, ProjectID: projectID,
			ConflictingTaskID: conflictID, CreatedAt: timestamp(createdAt),
		}); err != nil {
			return err
		}
	}
	for _, criterion := range contract.Criteria {
		criterionID := newUUID()
		if _, err := queries.InsertTaskAcceptanceCriterion(ctx, postgres.InsertTaskAcceptanceCriterionParams{
			ID: criterionID, TaskID: taskID, ExternalCriterionID: criterion.ID,
			Status: "pending", CreatedAt: timestamp(createdAt), UpdatedAt: timestamp(createdAt),
		}); err != nil {
			return err
		}
		var reference pgtype.Text
		var checkpoint []byte
		if criterion.VerificationMethod == "command" {
			reference = pgtype.Text{String: criterion.VerificationReference, Valid: true}
		} else {
			checkpoint, err = json.Marshal(map[string]string{"description": criterion.OperatorCheckpointText})
			if err != nil {
				return err
			}
		}
		if _, err := queries.InsertTaskAcceptanceVersion(ctx, postgres.InsertTaskAcceptanceVersionParams{
			ID: newUUID(), CriterionID: criterionID, TaskID: taskID, TaskVersionID: versionID,
			VersionNumber: 1, Requirement: criterion.Requirement,
			VerificationMethod: criterion.VerificationMethod, VerificationReference: reference,
			OperatorCheckpoint: checkpoint, CreatedAt: timestamp(createdAt),
		}); err != nil {
			return err
		}
	}
	return nil
}

func replay(artifactRoot, mediaType, hash string, source []byte, existing postgres.GetTaskImportBySourceIdentityRow) (Result, error) {
	if existing.SourceSha256 != hash || existing.MediaType != mediaType || existing.ArtifactSizeBytes != int64(len(source)) {
		return Result{}, fmt.Errorf("%w: source %q has changed", ErrImportConflict, existing.SourceName)
	}
	expectedPath, err := artifactPath(artifactRoot, hash)
	if err != nil {
		return Result{}, err
	}
	if existing.ArtifactStoragePath != expectedPath {
		return Result{}, fmt.Errorf("%w: artifact root changed for source %q", ErrImportConflict, existing.SourceName)
	}
	path, err := materializeArtifact(artifactRoot, hash, source)
	if err != nil {
		return Result{}, fmt.Errorf("task intake: validate replay artifact: %w", err)
	}
	return Result{
		ImportID: uuidString(existing.ID), ArtifactID: uuidString(existing.SourceArtifactID),
		TaskID: nullableUUIDString(existing.TaskID), Status: existing.Status,
		SourceSHA256: hash, ArtifactPath: path, Replayed: true,
	}, nil
}

func materializeArtifact(artifactRoot, hash string, source []byte) (string, error) {
	if len(hash) != 64 {
		return "", errors.New("invalid artifact hash")
	}
	absRoot, err := filepath.Abs(artifactRoot)
	if err != nil {
		return "", err
	}
	path := filepath.Join(absRoot, "sha256", hash[:2], hash[2:4], hash)
	rootInfo, err := os.Lstat(absRoot)
	if err != nil {
		return "", err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return "", fmt.Errorf("%w: artifact root is not a real directory", ErrUnsafeArtifact)
	}
	root, err := os.OpenRoot(absRoot)
	if err != nil {
		return "", err
	}
	openedInfo, err := root.Stat(".")
	if err != nil || !os.SameFile(rootInfo, openedInfo) {
		root.Close()
		return "", fmt.Errorf("%w: artifact root changed while opening", ErrUnsafeArtifact)
	}

	current := root
	for _, component := range []string{"sha256", hash[:2], hash[2:4]} {
		next, err := openArtifactDirectory(current, component)
		if current != root {
			_ = current.Close()
		}
		if err != nil {
			_ = root.Close()
			return "", err
		}
		current = next
	}
	defer root.Close()
	defer current.Close()

	file, err := current.OpenFile(hash, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		created := true
		defer func() {
			if created {
				_ = current.Remove(hash)
			}
		}()
		if n, writeErr := io.Copy(file, bytes.NewReader(source)); writeErr != nil || n != int64(len(source)) {
			_ = file.Close()
			return "", fmt.Errorf("write artifact: wrote %d bytes: %w", n, writeErr)
		}
		if err := file.Sync(); err != nil {
			_ = file.Close()
			return "", err
		}
		if err := file.Chmod(0o444); err != nil {
			_ = file.Close()
			return "", err
		}
		createdInfo, err := file.Stat()
		if err != nil {
			_ = file.Close()
			return "", err
		}
		if err := file.Close(); err != nil {
			return "", err
		}
		pathInfo, err := current.Lstat(hash)
		if err != nil || pathInfo.Mode()&os.ModeSymlink != 0 || !pathInfo.Mode().IsRegular() || !os.SameFile(createdInfo, pathInfo) {
			return "", fmt.Errorf("%w: created artifact path changed", ErrUnsafeArtifact)
		}
		directory, err := current.Open(".")
		if err != nil {
			return "", err
		}
		if err := directory.Sync(); err != nil {
			_ = directory.Close()
			return "", err
		}
		if err := directory.Close(); err != nil {
			return "", err
		}
		created = false
	} else if errors.Is(err, fs.ErrExist) {
		if err := adoptArtifact(current, hash, source); err != nil {
			return "", err
		}
	} else {
		return "", err
	}
	return path, nil
}

func artifactPath(artifactRoot, hash string) (string, error) {
	if len(hash) != 64 {
		return "", errors.New("invalid artifact hash")
	}
	absRoot, err := filepath.Abs(artifactRoot)
	if err != nil {
		return "", err
	}
	return filepath.Join(absRoot, "sha256", hash[:2], hash[2:4], hash), nil
}

func openArtifactDirectory(parent *os.Root, name string) (*os.Root, error) {
	if err := parent.Mkdir(name, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
		return nil, err
	}
	before, err := parent.Lstat(name)
	if err != nil || before.Mode()&os.ModeSymlink != 0 || !before.IsDir() {
		return nil, fmt.Errorf("%w: artifact directory %q is not a real directory", ErrUnsafeArtifact, name)
	}
	child, err := parent.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	opened, openErr := child.Stat(".")
	after, afterErr := parent.Lstat(name)
	if openErr != nil || afterErr != nil || after.Mode()&os.ModeSymlink != 0 ||
		!after.IsDir() || !os.SameFile(before, after) || !os.SameFile(after, opened) {
		child.Close()
		return nil, fmt.Errorf("%w: artifact directory %q changed while opening", ErrUnsafeArtifact, name)
	}
	return child, nil
}

func adoptArtifact(root *os.Root, name string, source []byte) error {
	before, err := root.Lstat(name)
	if err != nil || before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() {
		return fmt.Errorf("%w: existing artifact is not a regular file", ErrUnsafeArtifact)
	}
	file, err := root.Open(name)
	if err != nil {
		return err
	}
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		file.Close()
		return fmt.Errorf("%w: artifact changed while opening", ErrUnsafeArtifact)
	}
	raw, err := io.ReadAll(io.LimitReader(file, MaximumSourceBytes+1))
	if err != nil {
		file.Close()
		return err
	}
	after, statErr := root.Lstat(name)
	if statErr != nil || after.Mode()&os.ModeSymlink != 0 || !os.SameFile(before, after) {
		file.Close()
		return fmt.Errorf("%w: artifact changed while reading", ErrUnsafeArtifact)
	}
	if !bytes.Equal(raw, source) {
		file.Close()
		return fmt.Errorf("existing artifact sha256 path contains mismatched bytes")
	}
	if err := file.Chmod(0o444); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	final, err := root.Lstat(name)
	if err != nil || final.Mode()&os.ModeSymlink != 0 || !final.Mode().IsRegular() || !os.SameFile(before, final) {
		file.Close()
		return fmt.Errorf("%w: artifact changed while sealing", ErrUnsafeArtifact)
	}
	return file.Close()
}

func normalizeMediaType(value string) (string, error) {
	base, parameters, err := mime.ParseMediaType(strings.TrimSpace(value))
	if err != nil || base == "" {
		return "", fmt.Errorf("task intake: invalid media type %q", value)
	}
	normalized := mime.FormatMediaType(strings.ToLower(base), parameters)
	if normalized == "" || len(normalized) > 512 {
		return "", fmt.Errorf("task intake: invalid media type %q", value)
	}
	return normalized, nil
}

func parseUUID(value string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}

func newUUID() pgtype.UUID {
	return pgtype.UUID{Bytes: uuid.MustParse(id.New()), Valid: true}
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}

func nullableUUIDString(value pgtype.UUID) string { return uuidString(value) }

func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}
