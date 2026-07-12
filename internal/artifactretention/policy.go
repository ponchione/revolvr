// Package artifactretention owns conservative inventory, deterministic GC
// plans, stream compression, and restartable artifact mutation.
package artifactretention

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const PolicySchema = "revolvr-artifact-retention-policy-v1"

type Policy struct {
	SchemaVersion          string        `json:"schema_version"`
	MutationEnabled        bool          `json:"mutation_enabled"`
	RecentRunCount         int           `json:"recent_run_count"`
	CompressAfter          time.Duration `json:"compress_after"`
	PruneAfter             time.Duration `json:"prune_after"`
	MinimumCompressBytes   int64         `json:"minimum_compress_bytes"`
	CompressCodexJSONL     bool          `json:"compress_codex_jsonl"`
	CompressCodexStderr    bool          `json:"compress_codex_stderr"`
	PruneCompressedStreams bool          `json:"prune_compressed_streams"`
	RequireVerifiedExport  bool          `json:"require_verified_export"`
	MaxFilesPerOperation   int           `json:"max_files_per_operation"`
	MaxBytesPerOperation   int64         `json:"max_bytes_per_operation"`
	DecompressionCapBytes  int64         `json:"decompression_cap_bytes"`
}

func DefaultPolicy() Policy {
	return Policy{SchemaVersion: PolicySchema, MutationEnabled: false, RecentRunCount: 20, CompressAfter: 7 * 24 * time.Hour, PruneAfter: 90 * 24 * time.Hour, MinimumCompressBytes: 64 * 1024, CompressCodexJSONL: true, CompressCodexStderr: true, PruneCompressedStreams: false, RequireVerifiedExport: true, MaxFilesPerOperation: 100, MaxBytesPerOperation: 1 << 30, DecompressionCapBytes: 256 << 20}
}

func (p Policy) Validate() error {
	if p.SchemaVersion != PolicySchema {
		return fmt.Errorf("retention policy: unsupported schema %q", p.SchemaVersion)
	}
	if p.RecentRunCount < 0 || p.CompressAfter < 0 || p.PruneAfter < 0 || p.MinimumCompressBytes < 0 || p.MaxFilesPerOperation <= 0 || p.MaxBytesPerOperation <= 0 || p.DecompressionCapBytes <= 0 {
		return errors.New("retention policy: ages, counts, sizes, and operation bounds must be nonnegative with positive caps")
	}
	if p.PruneAfter > 0 && p.CompressAfter > p.PruneAfter {
		return errors.New("retention policy: compress_after cannot exceed prune_after")
	}
	if p.PruneCompressedStreams && !p.RequireVerifiedExport {
		return errors.New("retention policy: destructive pruning requires a verified ledger export")
	}
	if !p.CompressCodexJSONL && !p.CompressCodexStderr && p.PruneCompressedStreams {
		return errors.New("retention policy: pruning requires at least one admitted stream class")
	}
	return nil
}

func (p Policy) Fingerprint() (string, []byte, error) {
	if err := p.Validate(); err != nil {
		return "", nil, err
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return "", nil, err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), raw, nil
}

func ParseClassList(values []string) (jsonl, stderr bool, err error) {
	for _, value := range values {
		switch strings.TrimSpace(value) {
		case "codex_jsonl":
			jsonl = true
		case "codex_stderr":
			stderr = true
		case "":
		default:
			return false, false, fmt.Errorf("retention policy: unknown artifact class %q", value)
		}
	}
	return jsonl, stderr, nil
}
