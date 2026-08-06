package tool

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
)

const (
	resultMediaType          = "application/vnd.revolvr.tool-result+json"
	maximumInlineResultBytes = 16 << 10
)

func validateRuntimeKind(kind RuntimeKind) error {
	if kind != RuntimeDirectToolsV1 {
		return fmt.Errorf("tool runtime kind %q is unknown or reserved", kind)
	}
	return nil
}

type directRuntimeHandler struct {
	executor SandboxExecutor
}

func (h directRuntimeHandler) Kind() RuntimeKind { return RuntimeDirectToolsV1 }

func (h directRuntimeHandler) Execute(ctx context.Context, request RuntimeExecutionRequest) (ExecutionResult, error) {
	return h.executor.Execute(ctx, request.Sandbox, request.Operation)
}

func (h directRuntimeHandler) Cancel(ctx context.Context, sandboxID string) error {
	return h.executor.Cancel(ctx, sandboxID)
}

type localSequencer struct {
	next atomic.Int64
}

func (s *localSequencer) Next(_ context.Context, request SequenceRequest) (SequenceGrant, error) {
	return SequenceGrant{
		Sequence: s.next.Add(1), RuntimeKind: request.RuntimeKind, RunID: request.RunID,
		RequestSHA256: request.RequestSHA256, Trusted: true,
	}, nil
}

func validateSequenceGrant(request SequenceRequest, grant SequenceGrant, previous int64) error {
	if !grant.Trusted {
		return errors.New("tool trajectory sequence grant is untrusted")
	}
	if grant.RuntimeKind != request.RuntimeKind || grant.RunID != request.RunID || grant.RequestSHA256 != request.RequestSHA256 {
		return errors.New("tool trajectory sequence grant does not bind the exact request authority")
	}
	if grant.Sequence <= 0 {
		return errors.New("tool trajectory sequence is missing")
	}
	if grant.Sequence <= previous {
		return errors.New("tool trajectory sequence is duplicate or stale")
	}
	return nil
}

func representResult(raw []byte, artifact Artifact, inlineLimit int, truncatedBytes int64) ResultRepresentation {
	representation := ResultRepresentation{
		MediaType: resultMediaType, SHA256: digest(raw), SizeBytes: int64(len(raw)),
		Truncated: truncatedBytes > 0, TruncatedBytes: truncatedBytes,
	}
	if len(raw) <= inlineLimit {
		representation.Kind = ResultRepresentationInline
		representation.Resolution = "complete_inline"
		if representation.Truncated {
			representation.Resolution = "bounded_inline_truncated"
		}
		representation.Inline = &InlineResult{
			MediaType: resultMediaType, Content: string(raw), SHA256: representation.SHA256,
			SizeBytes: representation.SizeBytes,
		}
		return representation
	}
	representation.Kind = ResultRepresentationArtifact
	representation.Resolution = "immutable_content_addressed_artifact"
	if representation.Truncated {
		representation.Resolution = "immutable_artifact_with_bounded_output"
	}
	artifact.MediaType = resultMediaType
	representation.Artifacts = []ArtifactReference{{Artifact: artifact, Immutable: true}}
	return representation
}

func validateResultRepresentation(value ResultRepresentation, expectedHash string) error {
	if !validSHA(value.SHA256) || value.SHA256 != expectedHash || value.SizeBytes < 0 || value.MediaType == "" || value.TruncatedBytes < 0 || value.Truncated != (value.TruncatedBytes > 0) || value.Resolution == "" {
		return errors.New("tool result representation metadata is malformed")
	}
	switch value.Kind {
	case ResultRepresentationInline:
		if value.Inline == nil || len(value.Artifacts) != 0 || value.SizeBytes > maximumInlineResultBytes || value.Inline.MediaType != value.MediaType || value.Inline.SizeBytes != int64(len(value.Inline.Content)) || value.Inline.SizeBytes != value.SizeBytes || digest([]byte(value.Inline.Content)) != value.Inline.SHA256 || value.Inline.SHA256 != value.SHA256 {
			return errors.New("tool inline result representation is malformed or exceeds its bound")
		}
	case ResultRepresentationArtifact:
		if value.Inline != nil || len(value.Artifacts) == 0 {
			return errors.New("tool artifact result representation is not an exclusive nonempty union")
		}
		for _, reference := range value.Artifacts {
			if !reference.Immutable || reference.Artifact.MediaType == "" || !validSHA(reference.Artifact.SHA256) || reference.Artifact.SizeBytes < 0 {
				return errors.New("tool result artifact is mutable or not content-addressed")
			}
		}
		if len(value.Artifacts) == 1 && (value.Artifacts[0].Artifact.SHA256 != value.SHA256 || value.Artifacts[0].Artifact.SizeBytes != value.SizeBytes) {
			return errors.New("tool result artifact does not resolve the exact result content")
		}
	default:
		return errors.New("tool result representation kind is unknown or reserved")
	}
	return nil
}
