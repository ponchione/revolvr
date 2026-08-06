package verification

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"revolvr/internal/id"
	"revolvr/internal/runtimepath"
)

type ContentAddressedArtifactWriter struct {
	root runtimepath.Boundary
}

func NewContentAddressedArtifactWriter(root string) (*ContentAddressedArtifactWriter, error) {
	boundary, err := runtimepath.Bind(root)
	if err != nil {
		return nil, fmt.Errorf("verification artifact writer: %w", err)
	}
	return &ContentAddressedArtifactWriter{root: boundary}, nil
}

func (w *ContentAddressedArtifactWriter) Materialize(ctx context.Context, logicalKind, mediaType string, content []byte) (Artifact, error) {
	if err := ctx.Err(); err != nil {
		return Artifact{}, err
	}
	if logicalKind == "" || mediaType == "" || int64(len(content)) > MaximumCapturedStreamBytes {
		return Artifact{}, fmt.Errorf("%w: invalid kind, media type, or byte count", ErrArtifact)
	}
	hash := hashBytes(content)
	parent := filepath.Join(w.root.Root(), "sha256", hash[:2], hash[2:4])
	if err := w.root.EnsureDir(parent, 0o755); err != nil {
		return Artifact{}, fmt.Errorf("%w: create content address: %v", ErrArtifact, err)
	}
	directory, found, err := w.root.OpenDir(parent, false)
	if err != nil || !found {
		return Artifact{}, fmt.Errorf("%w: open content address: %v", ErrArtifact, errors.Join(err, os.ErrNotExist))
	}
	defer directory.Close()
	file, err := directory.OpenFile(hash, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		if n, writeErr := file.Write(content); writeErr != nil || n != len(content) {
			_ = file.Close()
			return Artifact{}, fmt.Errorf("%w: %v", ErrArtifact, errors.Join(writeErr, errors.New("short artifact write")))
		}
		if err := file.Sync(); err != nil {
			_ = file.Close()
			return Artifact{}, fmt.Errorf("%w: %v", ErrArtifact, err)
		}
		if err := file.Chmod(0o444); err != nil {
			_ = file.Close()
			return Artifact{}, fmt.Errorf("%w: %v", ErrArtifact, err)
		}
		if err := file.Close(); err != nil {
			return Artifact{}, fmt.Errorf("%w: %v", ErrArtifact, err)
		}
		if err := directory.Sync(); err != nil {
			return Artifact{}, fmt.Errorf("%w: %v", ErrArtifact, err)
		}
	} else if errors.Is(err, os.ErrExist) {
		existing, found, readErr := directory.ReadFileLimit(hash, false, int64(len(content)+1))
		if readErr != nil || !found || !bytes.Equal(existing, content) {
			return Artifact{}, fmt.Errorf("%w: content-addressed path does not contain expected bytes: %v", ErrArtifact, readErr)
		}
	} else {
		return Artifact{}, fmt.Errorf("%w: %v", ErrArtifact, err)
	}
	return Artifact{
		ID: id.New(), SHA256: hash, SizeBytes: int64(len(content)), MediaType: mediaType,
		LogicalKind: logicalKind, StoragePath: filepath.Join(parent, hash), Content: append([]byte(nil), content...),
	}, nil
}
