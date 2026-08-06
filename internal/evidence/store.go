package evidence

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"revolvr/internal/id"
	"revolvr/internal/runtimepath"
)

const MaximumCompletionArtifactBytes = int64(16 << 20)

type Store struct {
	root runtimepath.Boundary
}

func NewStore(root string) (*Store, error) {
	boundary, err := runtimepath.Bind(root)
	if err != nil {
		return nil, fmt.Errorf("evidence artifact store: %w", err)
	}
	return &Store{root: boundary}, nil
}

func (s *Store) Materialize(ctx context.Context, kind, mediaType string, content []byte, provenance Provenance) (Artifact, error) {
	if err := ctx.Err(); err != nil {
		return Artifact{}, err
	}
	if kind == "" || mediaType == "" || int64(len(content)) > MaximumCompletionArtifactBytes {
		return Artifact{}, invalid("artifact kind, media type, or size is invalid")
	}
	if err := provenance.Validate(); err != nil {
		return Artifact{}, err
	}
	hash := HashBytes(content)
	parent := filepath.Join(s.root.Root(), "sha256", hash[:2], hash[2:4])
	if err := s.root.EnsureDir(parent, 0o755); err != nil {
		return Artifact{}, fmt.Errorf("materialize evidence artifact: %w", err)
	}
	directory, found, err := s.root.OpenDir(parent, false)
	if err != nil || !found {
		return Artifact{}, fmt.Errorf("materialize evidence artifact: %w", errors.Join(err, os.ErrNotExist))
	}
	defer directory.Close()
	file, err := directory.OpenFile(hash, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		if n, writeErr := file.Write(content); writeErr != nil || n != len(content) {
			_ = file.Close()
			return Artifact{}, errors.Join(writeErr, errors.New("short evidence artifact write"))
		}
		if err := file.Sync(); err != nil {
			_ = file.Close()
			return Artifact{}, err
		}
		if err := file.Chmod(0o444); err != nil {
			_ = file.Close()
			return Artifact{}, err
		}
		if err := file.Close(); err != nil {
			return Artifact{}, err
		}
		if err := directory.Sync(); err != nil {
			return Artifact{}, err
		}
	} else if errors.Is(err, os.ErrExist) {
		existing, found, readErr := directory.ReadFileLimit(hash, false, int64(len(content))+1)
		if readErr != nil || !found || !bytes.Equal(existing, content) {
			return Artifact{}, fmt.Errorf("%w: %s", ErrArtifactDivergence, hash)
		}
	} else {
		return Artifact{}, err
	}
	return Artifact{
		ID: id.New(), Kind: kind, MediaType: mediaType, SHA256: hash,
		SizeBytes: int64(len(content)), StoragePath: filepath.Join(parent, hash),
		Resolved: true, Provenance: provenance, Content: append([]byte(nil), content...),
	}, nil
}

func ScanSecrets(payloads [][]byte, sentinels []string) error {
	for _, secret := range sentinels {
		if secret == "" {
			continue
		}
		needle := []byte(secret)
		for _, payload := range payloads {
			if bytes.Contains(payload, needle) {
				return fmt.Errorf("%w: sentinel %q", ErrSecretSentinel, redact(secret))
			}
		}
	}
	return nil
}

func redact(value string) string {
	if len(value) < 4 {
		return "[redacted]"
	}
	return strings.Repeat("*", min(8, len(value)))
}
