package index

import (
	"errors"

	"revolvr/internal/embedding"
)

const (
	SelectedEmbeddingModelName      = "Qwen/Qwen3-Embedding-0.6B-GGUF"
	SelectedEmbeddingModelRevision  = "370f27d7550e0def9b39c1f16d3fbaa13aa67728"
	SelectedEmbeddingArtifactSHA256 = "06507c7b42688469c4e7298b0a1e16deff06caf291cf0a5b278c308249c3e439"
	SelectedEmbeddingDimensions     = 1024
	SelectedEmbeddingPooling        = "last"
	SelectedEmbeddingNormalization  = "l2"
	SelectedEmbeddingQuantization   = "Q8_0"
	SelectedEmbeddingLicense        = "Apache-2.0"
	SelectedEmbeddingSourceURI      = "https://huggingface.co/Qwen/Qwen3-Embedding-0.6B-GGUF/resolve/370f27d7550e0def9b39c1f16d3fbaa13aa67728/Qwen3-Embedding-0.6B-Q8_0.gguf"
	SelectedEmbeddingServingImage   = "sha256:8903d304f9cadf35ac881ebf0bb3537426b5b096b63088d0f17b719656b07c20"
	SelectedQueryInstruction        = "Instruct: Given a natural language query, retrieve relevant source code that answers the query\nQuery: "
	SelectedQueryInstructionSHA256  = "8a1900345dce8d58adb5671a807e86ed39eeb6da706c491f71fa845c7ed9f59a"
)

// SelectedEmbeddingEvidence is the measured Architecture 021 default. It does
// not bypass Architecture 020 metadata validation or select a fallback when
// the exact local artifact and service are unavailable.
func SelectedEmbeddingEvidence() ModelEvidence {
	model := embedding.EmbeddingModelInfo{
		SchemaVersion: embedding.ModelInfoSchemaVersion, ModelName: SelectedEmbeddingModelName,
		Revision: SelectedEmbeddingModelRevision, Dimensions: SelectedEmbeddingDimensions, Pooling: SelectedEmbeddingPooling,
		Normalization: SelectedEmbeddingNormalization, Quantization: SelectedEmbeddingQuantization, ArtifactSHA256: SelectedEmbeddingArtifactSHA256,
	}
	space, _ := model.SpaceIdentity()
	return ModelEvidence{Model: model, SpaceSHA256: space.SHA256, License: SelectedEmbeddingLicense, SourceURI: SelectedEmbeddingSourceURI, ServingImageDigest: SelectedEmbeddingServingImage}
}

// ValidateSelectedEmbeddingModel rejects compatibility with any other model
// family or representation while allowing a deliberate, atomic Qwen revision
// change to produce a new exact Architecture 020 embedding-space identity.
func ValidateSelectedEmbeddingModel(model embedding.EmbeddingModelInfo) error {
	if err := model.Validate(); err != nil {
		return err
	}
	if model.ModelName != SelectedEmbeddingModelName ||
		model.Dimensions != SelectedEmbeddingDimensions ||
		model.Pooling != SelectedEmbeddingPooling ||
		model.Normalization != SelectedEmbeddingNormalization ||
		model.Quantization != SelectedEmbeddingQuantization {
		return errors.New("code index: only the selected Qwen3-Embedding-0.6B Q8_0 representation is supported")
	}
	return nil
}
