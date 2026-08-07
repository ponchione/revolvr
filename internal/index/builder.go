package index

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"revolvr/internal/embedding"
)

type ExistingFile struct {
	Parsed      ParsedFile
	SpaceSHA256 string
	Vectors     map[string][]float32
}

type PrepareRequest struct {
	OperationID    string
	Kind           BuildKind
	Snapshot       Snapshot
	Limits         Limits
	Existing       map[string]ExistingFile
	EmbeddingSpace *ModelEvidence
	Embedder       embedding.Embedder
	Now            time.Time
}

type BuildError struct {
	Code   string
	Status embedding.ServiceStatus
	Err    error
}

func (e *BuildError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("code index build %s: %v", e.Code, e.Err)
}
func (e *BuildError) Unwrap() error { return e.Err }

func Prepare(ctx context.Context, request PrepareRequest) (PreparedBuild, error) {
	if err := ctx.Err(); err != nil {
		return PreparedBuild{}, err
	}
	limits, err := request.Limits.normalized()
	if err != nil {
		return PreparedBuild{}, err
	}
	if strings.TrimSpace(request.OperationID) == "" || strings.TrimSpace(request.OperationID) != request.OperationID {
		return PreparedBuild{}, errors.New("code index: stable operation ID is required")
	}
	switch request.Kind {
	case BuildFull, BuildIncremental, BuildRebuild, BuildSpaceSwitch:
	default:
		return PreparedBuild{}, errors.New("code index: unsupported build kind")
	}
	if err := request.Snapshot.Validate(limits); err != nil {
		return PreparedBuild{}, err
	}
	if request.EmbeddingSpace == nil && request.Embedder != nil {
		return PreparedBuild{}, errors.New("code index: embedder requires exact embedding-space evidence")
	}
	if request.EmbeddingSpace != nil {
		if request.Embedder == nil {
			return PreparedBuild{}, errors.New("code index: embedding space requires an embedder")
		}
		if err := request.EmbeddingSpace.Validate(); err != nil {
			return PreparedBuild{}, err
		}
		info, err := request.Embedder.ModelInfo(ctx)
		if err != nil {
			return PreparedBuild{}, embeddingBuildError("embedding_metadata", err)
		}
		if info != request.EmbeddingSpace.Model {
			return PreparedBuild{}, &BuildError{Code: "embedding_space_drift", Status: embedding.ServiceStatus{Mode: embedding.ServiceDegraded, Kind: embedding.ErrorModelMetadataDrift, Detail: "model metadata differs from requested build space"}, Err: errors.New("embedding metadata drift")}
		}
	}

	build := PreparedBuild{
		ID:          DeterministicID("index-build", request.Snapshot.ProjectID, request.OperationID),
		OperationID: request.OperationID, Kind: request.Kind, Snapshot: request.Snapshot,
		EmbeddingSpace: request.EmbeddingSpace, Vectors: make(map[string][]float32), PreparedAt: request.Now.UTC().Truncate(time.Microsecond),
	}
	if build.PreparedAt.IsZero() {
		build.PreparedAt = time.Now().UTC().Truncate(time.Microsecond)
	}

	for _, file := range request.Snapshot.Files {
		if err := ctx.Err(); err != nil {
			return PreparedBuild{}, err
		}
		contentSHA := SHA256(file.Content)
		existing, found := request.Existing[file.Path]
		canReuse := request.Kind == BuildIncremental && found && existing.Parsed.ContentSHA256 == contentSHA
		var parsed ParsedFile
		if canReuse {
			parsed = cloneParsed(existing.Parsed)
			parsed.Reused = true
		} else {
			parsed, err = ParseFile(request.Snapshot.ProjectID, file, limits)
			if err != nil {
				return PreparedBuild{}, fmt.Errorf("code index: parse %s: %w", file.Path, err)
			}
		}
		build.Files = append(build.Files, parsed)
	}
	build.Files = sortedFiles(build.Files)

	if request.EmbeddingSpace != nil {
		space := request.EmbeddingSpace.SpaceSHA256
		var pending []Chunk
		for _, file := range build.Files {
			existing := request.Existing[file.Path]
			for _, chunk := range file.Chunks {
				if file.Reused && existing.SpaceSHA256 == space {
					if value, ok := existing.Vectors[chunk.ID]; ok {
						build.Vectors[chunk.ID] = append([]float32(nil), value...)
						continue
					}
				}
				pending = append(pending, chunk)
			}
		}
		if err := embedChunks(ctx, request.Embedder, *request.EmbeddingSpace, pending, build.Vectors); err != nil {
			return PreparedBuild{}, err
		}
		chunkCount := 0
		for _, file := range build.Files {
			chunkCount += len(file.Chunks)
		}
		if len(build.Vectors) != chunkCount {
			return PreparedBuild{}, &BuildError{Code: "incomplete_vector_set", Status: embedding.ServiceStatus{Mode: embedding.ServiceDegraded, Kind: embedding.ErrorWrongCount}, Err: errors.New("embedding build did not produce one exact vector per chunk")}
		}
	}
	if err := build.finalizeManifest(); err != nil {
		return PreparedBuild{}, err
	}
	return build, nil
}

func embedChunks(ctx context.Context, embedder embedding.Embedder, evidence ModelEvidence, chunks []Chunk, output map[string][]float32) error {
	const maxInputs = 32
	const maxBytes = 768 << 10
	for start := 0; start < len(chunks); {
		end, bytes := start, 0
		var input []string
		for end < len(chunks) && end-start < maxInputs {
			text := chunks[end].EmbeddingText()
			if len(input) > 0 && bytes+len(text) > maxBytes {
				break
			}
			input = append(input, text)
			bytes += len(text)
			end++
		}
		batch, err := embedder.EmbedDocuments(ctx, input)
		if err != nil {
			return embeddingBuildError("embedding_unavailable", err)
		}
		if batch.Status.Mode != embedding.ServiceReady || batch.Space.SHA256 != evidence.SpaceSHA256 || batch.Space.Model != evidence.Model || len(batch.Values) != len(input) {
			return &BuildError{Code: "embedding_space_drift", Status: batch.Status, Err: errors.New("embedding batch space or count is divergent")}
		}
		for i, value := range batch.Values {
			if len(value) != evidence.Model.Dimensions {
				return &BuildError{Code: "embedding_dimension", Status: embedding.ServiceStatus{Mode: embedding.ServiceDegraded, Kind: embedding.ErrorWrongDimension}, Err: errors.New("embedding dimension is divergent")}
			}
			output[chunks[start+i].ID] = append([]float32(nil), value...)
		}
		start = end
	}
	return nil
}

func embeddingBuildError(code string, err error) error {
	status := embedding.ServiceStatus{Mode: embedding.ServiceDegraded, Kind: embedding.ErrorUnavailable, Detail: err.Error()}
	var adapter *embedding.AdapterError
	if errors.As(err, &adapter) {
		status = adapter.Status()
	}
	return &BuildError{Code: code, Status: status, Err: err}
}

func cloneParsed(value ParsedFile) ParsedFile {
	copyValue := value
	copyValue.Chunks = append([]Chunk(nil), value.Chunks...)
	copyValue.Symbols = append([]Symbol(nil), value.Symbols...)
	copyValue.Edges = append([]Edge(nil), value.Edges...)
	return copyValue
}

func vectorText(value []float32) string {
	parts := make([]string, len(value))
	for i, number := range value {
		parts[i] = fmt.Sprintf("%.9g", number)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func sortedVectorIDs(vectors map[string][]float32) []string {
	ids := make([]string, 0, len(vectors))
	for id := range vectors {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
