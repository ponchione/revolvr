package evidence

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirectToolsProvenanceIsExplicitAndCanonical(t *testing.T) {
	trajectory := DirectToolsTrajectoryEnvelope()
	if err := trajectory.Validate(); err != nil {
		t.Fatal(err)
	}
	assets := DirectToolsHarnessAssetSet()
	if err := assets.Validate(); err != nil {
		t.Fatal(err)
	}
	if trajectory.State != TrajectoryInactive || trajectory.Used || trajectory.Artifacts == nil ||
		assets.State != HarnessAssetsInactive || assets.Used || assets.Assets == nil || assets.ManifestSHA256 == "" {
		t.Fatalf("trajectory/assets = %#v / %#v", trajectory, assets)
	}
	other := DirectToolsHarnessAssetSet()
	if assets.ManifestSHA256 != other.ManifestSHA256 {
		t.Fatal("equal empty asset sets have different hashes")
	}
}

func TestUsedTrajectoryAndHarnessAssetsRequireExactProvenance(t *testing.T) {
	trajectory := DirectToolsTrajectoryEnvelope()
	trajectory.State = TrajectoryActive
	trajectory.Used = true
	if err := trajectory.Validate(); err == nil {
		t.Fatal("active trajectory without manifest was accepted")
	}
	trajectory.ManifestVersion = "trajectory-v1"
	trajectory.ManifestSHA256 = strings.Repeat("1", 64)
	trajectory.FirstSequence, trajectory.LastSequence, trajectory.EntryCount = 1, 2, 2
	trajectory.Artifacts = []TrajectoryArtifact{{ArtifactID: "artifact", SHA256: strings.Repeat("2", 64)}}
	if err := trajectory.Validate(); err == nil {
		t.Fatal("unresolved trajectory artifact was accepted")
	}
	trajectory.Artifacts[0].Resolved = true
	if err := trajectory.Validate(); err != nil {
		t.Fatal(err)
	}

	assets := HarnessAssetSet{
		SchemaVersion: HarnessAssetSetSchemaVersion, State: HarnessAssetsActive,
		RuntimeKind: "programmatic_workspace_v1", Used: true,
		Assets: []HarnessAsset{{ID: "skill", Version: "v1", SHA256: strings.Repeat("3", 64), ArtifactID: "asset"}},
	}
	assets.ManifestSHA256, _ = assets.MaterialHash()
	if err := assets.Validate(); err == nil {
		t.Fatal("unresolved harness asset was accepted")
	}
	assets.Assets[0].Resolved = true
	assets.ManifestSHA256, _ = assets.MaterialHash()
	if err := assets.Validate(); err != nil {
		t.Fatal(err)
	}
	assets.Assets[0].Version = "v2"
	if err := assets.Validate(); err == nil {
		t.Fatal("changed harness asset with stale manifest hash was accepted")
	}
}

func TestContentAddressedStoreReusesExactBytesAndRejectsDivergence(t *testing.T) {
	root := filepath.Join(t.TempDir(), "artifacts")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	provenance := fixtureProvenance()
	content := []byte("immutable completion bytes")
	first, err := store.Materialize(context.Background(), "completion_evidence", "application/json", content, provenance)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Materialize(context.Background(), "completion_evidence", "application/json", content, provenance)
	if err != nil {
		t.Fatal(err)
	}
	if first.SHA256 != second.SHA256 || first.StoragePath != second.StoragePath {
		t.Fatalf("reused artifacts = %#v / %#v", first, second)
	}
	if err := os.Chmod(first.StoragePath, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first.StoragePath, []byte("divergent completion bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Materialize(context.Background(), "completion_evidence", "application/json", content, provenance); !errors.Is(err, ErrArtifactDivergence) {
		t.Fatalf("divergent materialization error = %v", err)
	}
}

func TestSecretSentinelScan(t *testing.T) {
	secret := "architecture-018-secret-sentinel"
	if err := ScanSecrets([][]byte{[]byte("safe")}, []string{secret}); err != nil {
		t.Fatal(err)
	}
	if err := ScanSecrets([][]byte{[]byte("contains " + secret)}, []string{secret}); !errors.Is(err, ErrSecretSentinel) || strings.Contains(err.Error(), secret) {
		t.Fatalf("secret scan error = %v", err)
	}
}

func fixtureProvenance() Provenance {
	return Provenance{
		SchemaVersion: ArtifactProvenanceSchemaVersion,
		ProjectID:     "project", TaskID: "task", TaskVersionID: "version", RunID: "run", WorkspaceID: "workspace",
		ProducerRole: "host", ProducingOperationID: "operation",
		SourceCommit: strings.Repeat("a", 40), SourceTree: strings.Repeat("b", 40),
	}
}
