package completion

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"revolvr/internal/evidence"
)

type capsule struct {
	SchemaVersion   string   `json:"schema_version"`
	PreflightSHA256 string   `json:"preflight_sha256"`
	Snapshot        Snapshot `json:"snapshot"`
}

type manifestEntry struct {
	Role      string `json:"role"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
	MediaType string `json:"media_type"`
}

type capsuleManifest struct {
	SchemaVersion          string                       `json:"schema_version"`
	PreflightSHA256        string                       `json:"preflight_sha256"`
	CapsuleSHA256          string                       `json:"capsule_sha256"`
	ArtifactManifestSHA256 string                       `json:"artifact_manifest_sha256"`
	TrajectorySHA256       string                       `json:"trajectory_sha256"`
	HarnessAssetSetSHA256  string                       `json:"harness_asset_set_sha256"`
	Files                  []manifestEntry              `json:"files"`
	SupportingArtifacts    []evidence.ArtifactReference `json:"supporting_artifacts"`
}

type capsulePayloads struct {
	evidenceJSON []byte
	markdown     []byte
	manifest     []byte
}

func MaterializeCapsule(
	ctx context.Context,
	store *evidence.Store,
	preflight Preflight,
	provenance evidence.Provenance,
	secretSentinels []string,
	fail FailureInjector,
) (Materialized, error) {
	if store == nil {
		return Materialized{}, errorsNew("completion artifact store is nil")
	}
	if !preflight.Accepted() {
		return Materialized{}, ErrRejected
	}
	if provenance.ProducingOperationID == "" {
		return Materialized{}, errorsNew("completion artifact provenance lacks operation identity")
	}
	payloads, err := buildCapsulePayloads(preflight)
	if err != nil {
		return Materialized{}, err
	}
	if err := evidence.ScanSecrets([][]byte{payloads.evidenceJSON, payloads.markdown, payloads.manifest}, secretSentinels); err != nil {
		return Materialized{}, err
	}
	jsonArtifact, err := store.Materialize(ctx, "completion_evidence", "application/json", payloads.evidenceJSON, provenance)
	if err != nil {
		return Materialized{}, err
	}
	if err := inject(fail, FailureAfterEvidenceJSON); err != nil {
		return Materialized{}, err
	}
	markdownArtifact, err := store.Materialize(ctx, "completion_markdown", "text/markdown", payloads.markdown, provenance)
	if err != nil {
		return Materialized{}, err
	}
	if err := inject(fail, FailureAfterMarkdown); err != nil {
		return Materialized{}, err
	}
	manifestArtifact, err := store.Materialize(ctx, "completion_manifest", "application/json", payloads.manifest, provenance)
	if err != nil {
		return Materialized{}, err
	}
	return Materialized{EvidenceJSON: jsonArtifact, Markdown: markdownArtifact, Manifest: manifestArtifact}, nil
}

func buildCapsulePayloads(preflight Preflight) (capsulePayloads, error) {
	evidenceRaw, err := marshalLine(capsule{
		SchemaVersion:   evidence.CompletionEvidenceSchemaVersion,
		PreflightSHA256: preflight.SHA256,
		Snapshot:        preflight.Snapshot,
	})
	if err != nil {
		return capsulePayloads{}, err
	}
	markdownRaw := renderMarkdown(preflight)
	trajectoryHash, err := evidence.Hash(preflight.Snapshot.Trajectory)
	if err != nil {
		return capsulePayloads{}, err
	}
	manifest := capsuleManifest{
		SchemaVersion:          evidence.CompletionManifestSchemaVersion,
		PreflightSHA256:        preflight.SHA256,
		CapsuleSHA256:          evidence.HashBytes(evidenceRaw),
		ArtifactManifestSHA256: preflight.Snapshot.ArtifactManifestSHA256,
		TrajectorySHA256:       trajectoryHash,
		HarnessAssetSetSHA256:  preflight.Snapshot.HarnessAssets.ManifestSHA256,
		Files: []manifestEntry{
			{Role: "evidence_json", SHA256: evidence.HashBytes(evidenceRaw), SizeBytes: int64(len(evidenceRaw)), MediaType: "application/json"},
			{Role: "human_markdown", SHA256: evidence.HashBytes(markdownRaw), SizeBytes: int64(len(markdownRaw)), MediaType: "text/markdown"},
		},
		SupportingArtifacts: append([]evidence.ArtifactReference(nil), preflight.Snapshot.Artifacts...),
	}
	manifestRaw, err := marshalLine(manifest)
	if err != nil {
		return capsulePayloads{}, err
	}
	return capsulePayloads{evidenceJSON: evidenceRaw, markdown: markdownRaw, manifest: manifestRaw}, nil
}

func marshalLine(value any) ([]byte, error) {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func renderMarkdown(preflight Preflight) []byte {
	snapshot := preflight.Snapshot
	var builder strings.Builder
	fmt.Fprintf(&builder, "# Completion evidence: %s\n\n", snapshot.Identity.TaskID)
	fmt.Fprintf(&builder, "- Task version: `%s`\n", snapshot.Identity.TaskVersionID)
	fmt.Fprintf(&builder, "- Run: `%s`\n", snapshot.Identity.RunID)
	fmt.Fprintf(&builder, "- Preflight SHA-256: `%s`\n", preflight.SHA256)
	fmt.Fprintf(&builder, "- Source commit/tree: `%s` / `%s`\n", snapshot.Source.AfterCommit, snapshot.Source.AfterTree)
	fmt.Fprintf(&builder, "- Diff SHA-256: `%s`\n", snapshot.Source.DiffSHA256)
	fmt.Fprintf(&builder, "- Plan: `%s` (`%s`)\n", snapshot.Plan.ID, snapshot.Plan.VersionID)
	fmt.Fprintf(&builder, "- Final verification: `%s` (%s)\n", snapshot.Verification.ID, snapshot.Verification.Status)
	fmt.Fprintf(&builder, "- Independent audit: `%s` (%s)\n", snapshot.Audit.ID, snapshot.Audit.Disposition)
	fmt.Fprintf(&builder, "- Trajectory state: `%s`\n", snapshot.Trajectory.State)
	fmt.Fprintf(&builder, "- Harness asset-set SHA-256: `%s`\n", snapshot.HarnessAssets.ManifestSHA256)
	fmt.Fprintf(&builder, "- Supporting artifact manifest SHA-256: `%s`\n", snapshot.ArtifactManifestSHA256)
	builder.WriteString("\n## Acceptance criteria\n\n")
	for _, criterion := range snapshot.Criteria {
		fmt.Fprintf(&builder, "- `%s`: %s\n", criterion.ID, criterion.Status)
	}
	builder.WriteString("\n## Claims\n\n")
	for _, claim := range snapshot.Claims {
		fmt.Fprintf(&builder, "- `%s`: %s\n", claim.Key, claim.Statement)
	}
	builder.WriteString("\n## Findings\n\n")
	if len(snapshot.Findings) == 0 {
		builder.WriteString("No findings.\n")
	} else {
		for _, finding := range snapshot.Findings {
			fmt.Fprintf(&builder, "- `%s`: %s (%s)\n", finding.ID, finding.Status, finding.Significance)
		}
	}
	return []byte(builder.String())
}

func inject(fail FailureInjector, point FailurePoint) error {
	if fail == nil {
		return nil
	}
	return fail(point)
}

func errorsNew(detail string) error { return fmt.Errorf("completion: %s", detail) }
