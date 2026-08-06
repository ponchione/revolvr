// Package embedding provides the trusted Go boundary to Revolvr's dedicated
// local embedding service. Embeddings are derived data: a service failure is
// reported explicitly and never changes canonical task or run authority.
package embedding

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	ModelInfoSchemaVersion = "revolvr-embedding-model-info-v1"
	SpaceSchemaVersion     = "revolvr-embedding-space-v1"
)

type EmbeddingModelInfo struct {
	SchemaVersion  string `json:"schema_version"`
	ModelName      string `json:"model_name"`
	Revision       string `json:"revision"`
	Dimensions     int    `json:"dimensions"`
	Pooling        string `json:"pooling"`
	Normalization  string `json:"normalization"`
	Quantization   string `json:"quantization"`
	ArtifactSHA256 string `json:"artifact_sha256"`
}

func (m EmbeddingModelInfo) Validate() error {
	if m.SchemaVersion != ModelInfoSchemaVersion {
		return fmt.Errorf("embedding model metadata: schema_version must be %q", ModelInfoSchemaVersion)
	}
	fields := []struct {
		name  string
		value string
	}{
		{name: "model_name", value: m.ModelName},
		{name: "revision", value: m.Revision},
		{name: "pooling", value: m.Pooling},
		{name: "normalization", value: m.Normalization},
		{name: "quantization", value: m.Quantization},
	}
	for _, field := range fields {
		if field.value == "" || strings.TrimSpace(field.value) != field.value || strings.IndexFunc(field.value, func(r rune) bool {
			return r < 0x20 || r == 0x7f
		}) >= 0 {
			return fmt.Errorf("embedding model metadata: %s must be nonblank, normalized, and contain no control characters", field.name)
		}
	}
	if m.Dimensions <= 0 {
		return errors.New("embedding model metadata: dimensions must be positive")
	}
	decoded, err := hex.DecodeString(m.ArtifactSHA256)
	if err != nil || len(decoded) != sha256.Size || strings.ToLower(m.ArtifactSHA256) != m.ArtifactSHA256 {
		return errors.New("embedding model metadata: artifact_sha256 must be a lowercase SHA-256")
	}
	return nil
}

type EmbeddingSpaceIdentity struct {
	SchemaVersion string             `json:"schema_version"`
	Model         EmbeddingModelInfo `json:"model"`
	SHA256        string             `json:"sha256"`
}

func (m EmbeddingModelInfo) SpaceIdentity() (EmbeddingSpaceIdentity, error) {
	if err := m.Validate(); err != nil {
		return EmbeddingSpaceIdentity{}, err
	}
	material := struct {
		SchemaVersion string             `json:"schema_version"`
		Model         EmbeddingModelInfo `json:"model"`
	}{SchemaVersion: SpaceSchemaVersion, Model: m}
	raw, err := json.Marshal(material)
	if err != nil {
		return EmbeddingSpaceIdentity{}, fmt.Errorf("embedding space identity: encode metadata: %w", err)
	}
	sum := sha256.Sum256(raw)
	return EmbeddingSpaceIdentity{
		SchemaVersion: SpaceSchemaVersion,
		Model:         m,
		SHA256:        hex.EncodeToString(sum[:]),
	}, nil
}

type ServiceMode string

const (
	ServiceReady    ServiceMode = "ready"
	ServiceDegraded ServiceMode = "degraded"
	ServiceFailed   ServiceMode = "failed"
)

type ErrorKind string

const (
	ErrorInvalidInput       ErrorKind = "invalid_input"
	ErrorUnhealthy          ErrorKind = "unhealthy"
	ErrorUnavailable        ErrorKind = "unavailable"
	ErrorMalformedResponse  ErrorKind = "malformed_response"
	ErrorWrongCount         ErrorKind = "wrong_count"
	ErrorWrongDimension     ErrorKind = "wrong_dimension"
	ErrorNonFiniteVector    ErrorKind = "non_finite_vector"
	ErrorModelMetadataDrift ErrorKind = "model_metadata_drift"
	ErrorTimeout            ErrorKind = "timeout"
	ErrorCancelled          ErrorKind = "cancelled"
)

type ServiceStatus struct {
	Mode        ServiceMode         `json:"mode"`
	Kind        ErrorKind           `json:"kind,omitempty"`
	Detail      string              `json:"detail,omitempty"`
	Model       *EmbeddingModelInfo `json:"model,omitempty"`
	SpaceSHA256 string              `json:"space_sha256,omitempty"`
}

func readyStatus(model EmbeddingModelInfo, spaceSHA256 string) ServiceStatus {
	copy := model
	return ServiceStatus{Mode: ServiceReady, Model: &copy, SpaceSHA256: spaceSHA256}
}

type AdapterError struct {
	Kind       ErrorKind
	Operation  string
	StatusCode int
	Detail     string
	Mode       ServiceMode
	cause      error
}

func (e *AdapterError) Error() string {
	if e == nil {
		return ""
	}
	if e.Detail == "" {
		return fmt.Sprintf("embedding %s: %s", e.Operation, e.Kind)
	}
	return fmt.Sprintf("embedding %s: %s: %s", e.Operation, e.Kind, e.Detail)
}

func (e *AdapterError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (e *AdapterError) Status() ServiceStatus {
	if e == nil {
		return ServiceStatus{Mode: ServiceFailed, Kind: ErrorMalformedResponse, Detail: "missing adapter error"}
	}
	return ServiceStatus{Mode: e.Mode, Kind: e.Kind, Detail: e.Detail}
}

func IsKind(err error, kind ErrorKind) bool {
	var target *AdapterError
	return errors.As(err, &target) && target.Kind == kind
}

type EmbeddingBatch struct {
	Status ServiceStatus          `json:"status"`
	Space  EmbeddingSpaceIdentity `json:"space"`
	Values [][]float32            `json:"values"`
}

type Embedding struct {
	Status ServiceStatus          `json:"status"`
	Space  EmbeddingSpaceIdentity `json:"space"`
	Value  []float32              `json:"value"`
}

type Embedder interface {
	EmbedDocuments(ctx context.Context, input []string) (EmbeddingBatch, error)
	EmbedQuery(ctx context.Context, input string) (Embedding, error)
	ModelInfo(ctx context.Context) (EmbeddingModelInfo, error)
}
