package completion

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"revolvr/internal/evidence"
)

func TestMaterializeCapsuleIsDeterministicRecoverableAndSecretFree(t *testing.T) {
	snapshot := validSnapshot(t)
	preflight, err := BuildPreflight(snapshot)
	if err != nil || !preflight.Accepted() {
		t.Fatalf("preflight = %#v, %v", preflight.Rejections, err)
	}
	store := newEvidenceStore(t)
	provenance := completionProvenance(snapshot, "materialize-test")
	crash := errors.New("fixture crash")
	_, err = MaterializeCapsule(context.Background(), store, preflight, provenance, nil, func(point FailurePoint) error {
		if point == FailureAfterMarkdown {
			return crash
		}
		return nil
	})
	if !errors.Is(err, crash) {
		t.Fatalf("crash error = %v", err)
	}
	first, err := MaterializeCapsule(context.Background(), store, preflight, provenance, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := MaterializeCapsule(context.Background(), store, preflight, provenance, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.EvidenceJSON.SHA256 != second.EvidenceJSON.SHA256 || first.Markdown.SHA256 != second.Markdown.SHA256 || first.Manifest.SHA256 != second.Manifest.SHA256 {
		t.Fatalf("materialized hashes changed: %#v / %#v", first, second)
	}
	t.Logf("deterministic capsule sha256: json=%s markdown=%s manifest=%s", first.EvidenceJSON.SHA256, first.Markdown.SHA256, first.Manifest.SHA256)
	for _, required := range []string{preflight.SHA256, evidence.TrajectoryInactive, snapshot.HarnessAssets.ManifestSHA256, snapshot.Verification.ID, snapshot.Audit.ID} {
		if !strings.Contains(string(first.EvidenceJSON.Content), required) && !strings.Contains(string(first.Manifest.Content), required) {
			t.Fatalf("capsule/manifest lacks %q", required)
		}
	}
	if !strings.Contains(string(first.Markdown.Content), snapshot.Identity.TaskID) || !strings.Contains(string(first.Markdown.Content), snapshot.Source.AfterCommit) {
		t.Fatal("human-readable capsule lacks task/source evidence")
	}
}

func TestMaterializeCapsuleRejectsSecretBeforeWriting(t *testing.T) {
	snapshot := validSnapshot(t)
	secret := "architecture-018-materialization-secret"
	snapshot.Claims[0].Statement = "claim includes " + secret
	snapshot.Claims[0].StatementSHA256 = evidence.HashBytes([]byte(snapshot.Claims[0].Statement))
	preflight, err := BuildPreflight(snapshot)
	if err != nil || !preflight.Accepted() {
		t.Fatalf("preflight = %#v, %v", preflight.Rejections, err)
	}
	root := filepath.Join(t.TempDir(), "artifacts")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := evidence.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = MaterializeCapsule(context.Background(), store, preflight, completionProvenance(snapshot, "secret-test"), []string{secret}, nil)
	if !errors.Is(err, evidence.ErrSecretSentinel) || strings.Contains(err.Error(), secret) {
		t.Fatalf("secret materialization error = %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("artifact root after secret rejection = %v, %v", entries, err)
	}
}

func TestCoordinatorCompletesAtomicallyOnceAndRejectsStalePreflight(t *testing.T) {
	snapshot := validSnapshot(t)
	key := Key{OperationID: "completion-" + uuid.NewString(), Identity: snapshot.Identity}
	memory := &memoryCompletionStore{snapshot: snapshot}
	coordinator := Coordinator{
		Reader: memory, Terminal: memory, Artifacts: newEvidenceStore(t),
		Clock: func() time.Time { return time.Date(2026, 8, 6, 13, 0, 0, 0, time.UTC) },
		NewID: func() string { return uuid.NewString() },
	}
	first, err := coordinator.Complete(context.Background(), key)
	if err != nil || first.Terminal.CompletionID == "" || first.Terminal.Replay || memory.commits != 1 {
		t.Fatalf("first completion = %#v, commits=%d, err=%v", first.Terminal, memory.commits, err)
	}
	second, err := coordinator.Complete(context.Background(), key)
	if err != nil || !second.Terminal.Replay || second.Terminal.CompletionID != first.Terminal.CompletionID || memory.commits != 1 {
		t.Fatalf("replay = %#v, commits=%d, err=%v", second.Terminal, memory.commits, err)
	}

	staleSnapshot := validSnapshot(t)
	staleKey := Key{OperationID: "completion-" + uuid.NewString(), Identity: staleSnapshot.Identity}
	stale := &memoryCompletionStore{snapshot: staleSnapshot, mutateAtRead: 2, mutate: func(snapshot *Snapshot) { snapshot.Aggregates.Task++ }}
	staleCoordinator := Coordinator{Reader: stale, Terminal: stale, Artifacts: newEvidenceStore(t)}
	if _, err := staleCoordinator.Complete(context.Background(), staleKey); !errors.Is(err, ErrStalePreflight) || stale.commits != 0 {
		t.Fatalf("stale completion error=%v commits=%d", err, stale.commits)
	}
}

func TestCoordinatorRejectsProvenanceAndArtifactDriftAtTerminalRevalidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Snapshot)
	}{
		{"trajectory", func(snapshot *Snapshot) {
			snapshot.Trajectory = evidence.TrajectoryEnvelope{
				SchemaVersion: evidence.TrajectoryEnvelopeSchemaVersion, State: evidence.TrajectoryActive,
				RuntimeKind: "programmatic_workspace_v1", Used: true, ManifestVersion: "trajectory-v1",
				ManifestSHA256: strings.Repeat("8", 64), FirstSequence: 1, LastSequence: 1, EntryCount: 1,
				Artifacts: []evidence.TrajectoryArtifact{},
			}
			snapshot.HarnessAssets.RuntimeKind = "programmatic_workspace_v1"
			snapshot.HarnessAssets.ManifestSHA256, _ = snapshot.HarnessAssets.MaterialHash()
		}},
		{"harness assets", func(snapshot *Snapshot) {
			snapshot.HarnessAssets.State = evidence.HarnessAssetsActive
			snapshot.HarnessAssets.Used = true
			snapshot.HarnessAssets.Assets = []evidence.HarnessAsset{{ID: "prompt-note", Version: "v2", SHA256: strings.Repeat("7", 64), ArtifactID: snapshot.Artifacts[0].ID, Resolved: true}}
			snapshot.HarnessAssets.ManifestSHA256, _ = snapshot.HarnessAssets.MaterialHash()
		}},
		{"artifact set", func(snapshot *Snapshot) {
			snapshot.Artifacts[0].SHA256 = strings.Repeat("0", 64)
			snapshot.ArtifactManifestSHA256, _ = evidence.ArtifactManifestHash(snapshot.Artifacts)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := validSnapshot(t)
			key := Key{OperationID: "completion-" + uuid.NewString(), Identity: snapshot.Identity}
			memory := &memoryCompletionStore{snapshot: snapshot, mutateAtRead: 2, mutate: test.mutate}
			coordinator := Coordinator{Reader: memory, Terminal: memory, Artifacts: newEvidenceStore(t)}
			if _, err := coordinator.Complete(context.Background(), key); !errors.Is(err, ErrStalePreflight) || memory.commits != 0 {
				t.Fatalf("completion error=%v commits=%d", err, memory.commits)
			}
		})
	}
}

type memoryCompletionStore struct {
	snapshot     Snapshot
	reads        int
	mutateAtRead int
	mutate       func(*Snapshot)
	commits      int
	operationID  string
	completionID string
}

func (m *memoryCompletionStore) ReadCompletionSnapshot(_ context.Context, _ Key) (Snapshot, error) {
	m.reads++
	if m.reads == m.mutateAtRead {
		m.mutate(&m.snapshot)
	}
	return m.snapshot, nil
}

func (m *memoryCompletionStore) LookupCompletion(_ context.Context, key Key) (TerminalResult, bool, error) {
	if m.operationID == "" {
		return TerminalResult{}, false, nil
	}
	if m.operationID != key.OperationID {
		return TerminalResult{}, false, ErrAlreadyCompleted
	}
	return TerminalResult{CompletionID: m.completionID, Replay: true}, true, nil
}

func (m *memoryCompletionStore) CommitCompletion(_ context.Context, command TerminalCommand) (TerminalResult, error) {
	latest, err := BuildPreflight(m.snapshot)
	if err != nil || !latest.Accepted() || latest.SHA256 != command.Preflight.SHA256 {
		return TerminalResult{}, ErrStalePreflight
	}
	m.commits++
	m.operationID, m.completionID = command.OperationID, command.CompletionID
	m.snapshot.TaskStatus, m.snapshot.RunStatus, m.snapshot.Workspace.Status = "completed", "released", "completed"
	return TerminalResult{CompletionID: command.CompletionID}, nil
}

func newEvidenceStore(t *testing.T) *evidence.Store {
	t.Helper()
	root := filepath.Join(t.TempDir(), "artifacts")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := evidence.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func completionProvenance(snapshot Snapshot, operationID string) evidence.Provenance {
	return evidence.Provenance{
		SchemaVersion: evidence.ArtifactProvenanceSchemaVersion,
		ProjectID:     snapshot.Identity.ProjectID, TaskID: snapshot.Identity.TaskID,
		TaskVersionID: snapshot.Identity.TaskVersionID, RunID: snapshot.Identity.RunID,
		WorkspaceID: snapshot.Identity.WorkspaceID, ProducerRole: "host",
		ProducingOperationID: operationID, SourceCommit: snapshot.Source.AfterCommit,
		SourceTree: snapshot.Source.AfterTree,
	}
}
