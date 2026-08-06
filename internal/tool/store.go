package tool

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"revolvr/internal/runtimepath"
)

type beginDisposition string

const (
	beginNew           beginDisposition = "new"
	beginReplay        beginDisposition = "replay"
	beginConflict      beginDisposition = "conflict"
	beginIndeterminate beginDisposition = "indeterminate"
)

type beginResult struct {
	disposition beginDisposition
	input       Artifact
	evidence    Evidence
}

type evidenceStore interface {
	Begin(context.Context, string, []byte) (beginResult, error)
	Complete(context.Context, string, []byte, []byte, []byte, Evidence) (Evidence, error)
}

type FileStore struct {
	root runtimepath.Boundary
}

func NewFileStore(root string) (*FileStore, error) {
	boundary, err := runtimepath.Bind(root)
	if err != nil {
		return nil, fmt.Errorf("tool evidence store: %w", err)
	}
	if err := boundary.CheckDir(boundary.Root(), false); err != nil {
		return nil, fmt.Errorf("tool evidence store: %w", err)
	}
	return &FileStore{root: boundary}, nil
}

func (s *FileStore) Begin(_ context.Context, callID string, raw []byte) (beginResult, error) {
	key := evidenceKey(callID)
	directoryPath := filepath.Join(s.root.Root(), "calls", key)
	if err := s.root.EnsureDir(directoryPath, 0o700); err != nil {
		return beginResult{}, err
	}
	directory, found, err := s.root.OpenDir(directoryPath, false)
	if err != nil || !found {
		return beginResult{}, errors.Join(err, os.ErrNotExist)
	}
	defer directory.Close()
	existing, inputFound, err := directory.ReadFileLimit("input.json", true, maximumCallBytes+1)
	if err != nil {
		return beginResult{}, err
	}
	if inputFound && !bytes.Equal(existing, raw) {
		return beginResult{disposition: beginConflict, input: artifact(filepath.Join(directoryPath, "input.json"), existing)}, nil
	}
	if !inputFound {
		if err := publishFile(directory, "input.json", raw); err != nil {
			return beginResult{}, err
		}
		intentRaw, _ := json.Marshal(struct {
			SchemaVersion string `json:"schema_version"`
			CallID        string `json:"call_id"`
			InputSHA256   string `json:"input_sha256"`
		}{"revolvr-tool-execution-intent-v1", callID, digest(raw)})
		if err := publishFile(directory, "intent.json", append(intentRaw, '\n')); err != nil {
			return beginResult{}, err
		}
	}
	input := artifactWithMediaType(filepath.Join(directoryPath, "input.json"), raw, "application/json")
	recordRaw, recordFound, err := directory.ReadFileLimit("record.json", true, 4<<20)
	if err != nil {
		return beginResult{}, err
	}
	if !recordFound {
		if inputFound {
			return beginResult{disposition: beginIndeterminate, input: input}, nil
		}
		return beginResult{disposition: beginNew, input: input}, nil
	}
	var evidence Evidence
	if err := json.Unmarshal(recordRaw, &evidence); err != nil || evidence.Input.SHA256 != input.SHA256 {
		return beginResult{}, errors.Join(err, errors.New("tool evidence record is corrupt"))
	}
	return beginResult{disposition: beginReplay, input: input, evidence: evidence}, nil
}

func (s *FileStore) Complete(_ context.Context, callID string, raw, stdout, stderr []byte, evidence Evidence) (Evidence, error) {
	directoryPath := filepath.Join(s.root.Root(), "calls", evidenceKey(callID))
	directory, found, err := s.root.OpenDir(directoryPath, false)
	if err != nil || !found {
		return Evidence{}, errors.Join(err, os.ErrNotExist)
	}
	defer directory.Close()
	evidence.Input = artifactWithMediaType(filepath.Join(directoryPath, "input.json"), raw, "application/json")
	evidence.RequestSHA256 = digest(raw)
	evidence.Stdout = artifactWithMediaType(filepath.Join(directoryPath, "stdout"), stdout, "application/octet-stream")
	evidence.Stderr = artifactWithMediaType(filepath.Join(directoryPath, "stderr"), stderr, "application/octet-stream")
	if err := publishFile(directory, "stdout", stdout); err != nil {
		return Evidence{}, err
	}
	if err := publishFile(directory, "stderr", stderr); err != nil {
		return Evidence{}, err
	}
	resultRaw, err := json.Marshal(evidenceResult(evidence))
	if err != nil {
		return Evidence{}, err
	}
	resultRaw = append(resultRaw, '\n')
	evidence.Result = artifactWithMediaType(filepath.Join(directoryPath, "result.json"), resultRaw, resultMediaType)
	evidence.ResultSHA256 = digest(resultRaw)
	evidence.ResultRepresentation = representResult(resultRaw, evidence.Result, maximumInlineResultBytes, evidence.StdoutTruncatedBytes+evidence.StderrTruncatedBytes)
	if err := validateResultRepresentation(evidence.ResultRepresentation, evidence.ResultSHA256); err != nil {
		return Evidence{}, err
	}
	if err := publishFile(directory, "result.json", resultRaw); err != nil {
		return Evidence{}, err
	}
	recordRaw, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return Evidence{}, err
	}
	if err := publishFile(directory, "record.json", append(recordRaw, '\n')); err != nil {
		return Evidence{}, err
	}
	return evidence, nil
}

func evidenceResult(value Evidence) any {
	return struct {
		SchemaVersion        string               `json:"schema_version"`
		RuntimeKind          RuntimeKind          `json:"runtime_kind"`
		TrajectorySequence   int64                `json:"trajectory_sequence"`
		CallID               string               `json:"call_id"`
		Tool                 string               `json:"tool,omitempty"`
		Authority            Authority            `json:"authority"`
		Runtime              RuntimeEvidence      `json:"runtime"`
		RequestSHA256        string               `json:"request_sha256"`
		Disposition          string               `json:"disposition"`
		DenialCode           string               `json:"denial_code,omitempty"`
		Detail               string               `json:"detail,omitempty"`
		ExitCode             int                  `json:"exit_code"`
		TimedOut             bool                 `json:"timed_out"`
		Cancelled            bool                 `json:"cancelled"`
		Truncated            bool                 `json:"truncated"`
		StdoutTruncatedBytes int64                `json:"stdout_truncated_bytes"`
		StderrTruncatedBytes int64                `json:"stderr_truncated_bytes"`
		SourceChanges        []SourceChange       `json:"source_changes,omitempty"`
		Effect               EffectProof          `json:"effect"`
		Cancellation         CancellationEvidence `json:"cancellation"`
	}{value.SchemaVersion, value.RuntimeKind, value.TrajectorySequence, value.CallID, value.Tool, value.Authority, value.Runtime, value.RequestSHA256, value.Disposition, value.DenialCode, value.Detail, value.ExitCode, value.TimedOut, value.Cancelled, value.Truncated, value.StdoutTruncatedBytes, value.StderrTruncatedBytes, value.SourceChanges, value.Effect, value.Cancellation}
}

func publishFile(directory *runtimepath.Directory, name string, raw []byte) error {
	existing, found, err := directory.ReadFileLimit(name, true, int64(len(raw)+1))
	if err != nil {
		return err
	}
	if found {
		if bytes.Equal(existing, raw) {
			return nil
		}
		return fmt.Errorf("tool evidence collision at %s", name)
	}
	temporary, err := directory.CreateTemp(".tool-", 0o600)
	if err != nil {
		return err
	}
	defer temporary.Close()
	if n, err := temporary.Write(raw); err != nil || n != len(raw) {
		return errors.Join(err, errors.New("short tool evidence write"))
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := directory.Link(temporary, name); err != nil {
		return err
	}
	return directory.Sync()
}

func evidenceKey(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func artifact(path string, raw []byte) Artifact {
	return artifactWithMediaType(path, raw, "application/octet-stream")
}

func artifactWithMediaType(path string, raw []byte, mediaType string) Artifact {
	return Artifact{Path: path, MediaType: mediaType, SHA256: digest(raw), SizeBytes: int64(len(raw))}
}

func digest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func boundedDetail(err error) string {
	if err == nil {
		return ""
	}
	value := err.Error()
	if len(value) > 4096 {
		value = value[:4096] + " [truncated]"
	}
	return value
}

func utcNow(clock func() time.Time) time.Time { return clock().UTC().Truncate(time.Microsecond) }
