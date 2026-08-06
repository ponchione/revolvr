package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

const (
	ArtifactProvenanceSchemaVersion = "revolvr-artifact-provenance-v1"
	ClaimSchemaVersion              = "revolvr-claim-v1"
	TrajectoryEnvelopeSchemaVersion = "revolvr-trajectory-provenance-envelope-v1"
	HarnessAssetSetSchemaVersion    = "revolvr-harness-asset-set-manifest-v1"
	CompletionEvidenceSchemaVersion = "revolvr-completion-evidence-v1"
	CompletionManifestSchemaVersion = "revolvr-completion-manifest-v1"
	DirectToolsRuntimeKind          = "direct_tools_v1"
	TrajectoryInactive              = "inactive_not_applicable"
	TrajectoryActive                = "active"
	HarnessAssetsInactive           = "inactive_empty"
	HarnessAssetsActive             = "active"
)

var (
	ErrInvalidEvidence    = errors.New("invalid evidence")
	ErrArtifactDivergence = errors.New("content-addressed artifact divergence")
	ErrSecretSentinel     = errors.New("completion evidence contains a configured secret")
	hexSHA256             = regexp.MustCompile(`^[0-9a-f]{64}$`)
	gitObjectID           = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)
	stableIdentifier      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
)

type Provenance struct {
	SchemaVersion        string `json:"schema_version"`
	ProjectID            string `json:"project_id"`
	TaskID               string `json:"task_id"`
	TaskVersionID        string `json:"task_version_id"`
	RunID                string `json:"run_id"`
	WorkspaceID          string `json:"workspace_id"`
	ProducerRole         string `json:"producer_role"`
	ProducingOperationID string `json:"producing_operation_id"`
	SourceCommit         string `json:"source_commit"`
	SourceTree           string `json:"source_tree"`
}

type Artifact struct {
	ID          string     `json:"artifact_id"`
	Kind        string     `json:"kind"`
	MediaType   string     `json:"media_type"`
	SHA256      string     `json:"sha256"`
	SizeBytes   int64      `json:"size_bytes"`
	StoragePath string     `json:"storage_path"`
	Resolved    bool       `json:"resolved"`
	Provenance  Provenance `json:"provenance"`
	Content     []byte     `json:"-"`
}

type EvidenceLink struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	SHA256   string `json:"sha256"`
	Resolved bool   `json:"resolved"`
}

type Claim struct {
	SchemaVersion   string         `json:"schema_version"`
	ID              string         `json:"id"`
	CriterionID     string         `json:"criterion_id,omitempty"`
	Key             string         `json:"key"`
	Statement       string         `json:"statement"`
	StatementSHA256 string         `json:"statement_sha256"`
	Evidence        []EvidenceLink `json:"evidence"`
}

type ArtifactReference struct {
	ID          string     `json:"artifact_id"`
	Kind        string     `json:"kind"`
	MediaType   string     `json:"media_type"`
	SHA256      string     `json:"sha256"`
	SizeBytes   int64      `json:"size_bytes"`
	StoragePath string     `json:"storage_path"`
	Resolved    bool       `json:"resolved"`
	Required    bool       `json:"required"`
	Provenance  Provenance `json:"provenance"`
}

type TrajectoryArtifact struct {
	ArtifactID string `json:"artifact_id"`
	SHA256     string `json:"sha256"`
	Resolved   bool   `json:"resolved"`
}

type TrajectoryEnvelope struct {
	SchemaVersion   string               `json:"schema_version"`
	State           string               `json:"state"`
	RuntimeKind     string               `json:"runtime_kind"`
	Used            bool                 `json:"used"`
	ManifestVersion string               `json:"manifest_version,omitempty"`
	ManifestSHA256  string               `json:"manifest_sha256,omitempty"`
	FirstSequence   int64                `json:"first_sequence"`
	LastSequence    int64                `json:"last_sequence"`
	EntryCount      int64                `json:"entry_count"`
	Artifacts       []TrajectoryArtifact `json:"referenced_artifacts"`
}

type HarnessAsset struct {
	ID         string `json:"id"`
	Version    string `json:"version"`
	SHA256     string `json:"sha256"`
	ArtifactID string `json:"artifact_id"`
	Resolved   bool   `json:"resolved"`
}

type HarnessAssetSet struct {
	SchemaVersion  string         `json:"schema_version"`
	State          string         `json:"state"`
	RuntimeKind    string         `json:"runtime_kind"`
	Used           bool           `json:"used"`
	Assets         []HarnessAsset `json:"assets"`
	ManifestSHA256 string         `json:"manifest_sha256"`
}

func Hash(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func HashBytes(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func NewClaim(id, criterionID, key, statement string, links []EvidenceLink) (Claim, error) {
	claim := Claim{
		SchemaVersion: ClaimSchemaVersion,
		ID:            id, CriterionID: criterionID, Key: key, Statement: statement,
		StatementSHA256: HashBytes([]byte(statement)),
		Evidence:        append([]EvidenceLink(nil), links...),
	}
	if err := claim.Validate(); err != nil {
		return Claim{}, err
	}
	return claim, nil
}

func (c Claim) Validate() error {
	if c.SchemaVersion != ClaimSchemaVersion || c.ID == "" || !stableIdentifier.MatchString(c.Key) ||
		strings.TrimSpace(c.Statement) == "" || c.StatementSHA256 != HashBytes([]byte(c.Statement)) || len(c.Evidence) == 0 {
		return invalid("claim identity, statement, or evidence is invalid")
	}
	seen := make(map[string]struct{}, len(c.Evidence))
	for _, link := range c.Evidence {
		if (link.Kind != "artifact" && link.Kind != "verification_check") || link.ID == "" ||
			!hexSHA256.MatchString(link.SHA256) || !link.Resolved {
			return invalid("claim evidence is missing, unresolved, or unsupported")
		}
		key := link.Kind + "\x00" + link.ID
		if _, ok := seen[key]; ok {
			return invalid("claim evidence contains a duplicate")
		}
		seen[key] = struct{}{}
	}
	return nil
}

func (p Provenance) Validate() error {
	roles := []string{"host", "supervisor", "planner", "implementer", "verifier", "auditor", "corrector", "operator"}
	if p.SchemaVersion != ArtifactProvenanceSchemaVersion || p.ProjectID == "" || p.TaskID == "" ||
		p.TaskVersionID == "" || p.RunID == "" || p.WorkspaceID == "" || p.ProducingOperationID == "" ||
		!slices.Contains(roles, p.ProducerRole) || !gitObjectID.MatchString(p.SourceCommit) ||
		!gitObjectID.MatchString(p.SourceTree) {
		return invalid("artifact provenance is incomplete")
	}
	return nil
}

func (a ArtifactReference) Validate() error {
	if a.ID == "" || a.Kind == "" || a.MediaType == "" || !hexSHA256.MatchString(a.SHA256) ||
		a.SizeBytes < 0 || a.StoragePath == "" || !a.Resolved {
		return invalid("artifact reference is missing, unresolved, or malformed")
	}
	return a.Provenance.Validate()
}

func DirectToolsTrajectoryEnvelope() TrajectoryEnvelope {
	return TrajectoryEnvelope{
		SchemaVersion: TrajectoryEnvelopeSchemaVersion,
		State:         TrajectoryInactive, RuntimeKind: DirectToolsRuntimeKind,
		Artifacts: []TrajectoryArtifact{},
	}
}

func (e TrajectoryEnvelope) Validate() error {
	if e.SchemaVersion != TrajectoryEnvelopeSchemaVersion || e.RuntimeKind == "" {
		return invalid("trajectory envelope identity is missing")
	}
	switch e.State {
	case TrajectoryInactive:
		if e.RuntimeKind != DirectToolsRuntimeKind || e.Used || e.ManifestVersion != "" || e.ManifestSHA256 != "" ||
			e.FirstSequence != 0 || e.LastSequence != 0 || e.EntryCount != 0 || len(e.Artifacts) != 0 {
			return invalid("inactive trajectory envelope contains fabricated or used trajectory authority")
		}
	case TrajectoryActive:
		if !e.Used || e.ManifestVersion == "" || !hexSHA256.MatchString(e.ManifestSHA256) ||
			e.FirstSequence <= 0 || e.LastSequence < e.FirstSequence || e.EntryCount <= 0 ||
			e.EntryCount > e.LastSequence-e.FirstSequence+1 {
			return invalid("active trajectory coverage or manifest is incomplete")
		}
		for _, artifact := range e.Artifacts {
			if artifact.ArtifactID == "" || !hexSHA256.MatchString(artifact.SHA256) || !artifact.Resolved {
				return invalid("active trajectory artifact is missing or unresolved")
			}
		}
	default:
		return invalid("trajectory state is unsupported")
	}
	return nil
}

func DirectToolsHarnessAssetSet() HarnessAssetSet {
	set := HarnessAssetSet{
		SchemaVersion: HarnessAssetSetSchemaVersion,
		State:         HarnessAssetsInactive, RuntimeKind: DirectToolsRuntimeKind,
		Assets: []HarnessAsset{},
	}
	set.ManifestSHA256, _ = set.MaterialHash()
	return set
}

func (s HarnessAssetSet) MaterialHash() (string, error) {
	copy := s
	copy.ManifestSHA256 = ""
	if copy.Assets == nil {
		copy.Assets = []HarnessAsset{}
	}
	return Hash(copy)
}

func (s HarnessAssetSet) Validate() error {
	if s.SchemaVersion != HarnessAssetSetSchemaVersion || s.RuntimeKind == "" || !hexSHA256.MatchString(s.ManifestSHA256) {
		return invalid("harness asset-set identity is incomplete")
	}
	want, err := s.MaterialHash()
	if err != nil || want != s.ManifestSHA256 {
		return invalid("harness asset-set manifest hash is invalid")
	}
	switch s.State {
	case HarnessAssetsInactive:
		if s.Used || len(s.Assets) != 0 {
			return invalid("inactive harness asset set contains active or fabricated assets")
		}
	case HarnessAssetsActive:
		if !s.Used || len(s.Assets) == 0 {
			return invalid("active harness asset set is empty")
		}
		seen := make(map[string]struct{}, len(s.Assets))
		for _, asset := range s.Assets {
			if !stableIdentifier.MatchString(asset.ID) || asset.Version == "" || !hexSHA256.MatchString(asset.SHA256) ||
				asset.ArtifactID == "" || !asset.Resolved {
				return invalid("active harness asset provenance is incomplete")
			}
			key := asset.ID + "\x00" + asset.Version
			if _, ok := seen[key]; ok {
				return invalid("harness asset set contains a duplicate version")
			}
			seen[key] = struct{}{}
		}
	default:
		return invalid("harness asset-set state is unsupported")
	}
	return nil
}

func ArtifactManifestHash(artifacts []ArtifactReference) (string, error) {
	copy := append([]ArtifactReference(nil), artifacts...)
	slices.SortFunc(copy, func(a, b ArtifactReference) int {
		if result := strings.Compare(a.Kind, b.Kind); result != 0 {
			return result
		}
		return strings.Compare(a.ID, b.ID)
	})
	for _, artifact := range copy {
		if err := artifact.Validate(); err != nil {
			return "", err
		}
	}
	return Hash(copy)
}

func invalid(detail string) error {
	return fmt.Errorf("%w: %s", ErrInvalidEvidence, detail)
}
