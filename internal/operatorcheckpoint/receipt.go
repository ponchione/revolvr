// Package operatorcheckpoint owns the strict, dependency-free receipt
// contract used to satisfy canonical operator checkpoints. It never mutates
// task files or receipt evidence.
package operatorcheckpoint

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"revolvr/internal/runtimepath"
)

const (
	ReceiptSchemaVersion = "operator-checkpoint-receipt-v1"
	ReceiptRoot          = ".agent/checkpoints"
	maxReceiptBytes      = 1 << 20
)

type Outcome string

const OutcomeAccepted Outcome = "accepted"

type EvidenceKind string

const (
	EvidenceFile     EvidenceKind = "file"
	EvidenceSource   EvidenceKind = "source"
	EvidenceBuild    EvidenceKind = "build"
	EvidenceArtifact EvidenceKind = "artifact"
)

// EvidenceReference deliberately permits only an identity-bearing reference;
// inline evidence, secrets, private data, and arbitrary extension fields are
// outside the receipt contract.
type EvidenceReference struct {
	Kind   EvidenceKind `json:"kind"`
	Path   string       `json:"path"`
	SHA256 string       `json:"sha256"`
}

type Receipt struct {
	SchemaVersion string              `json:"schema_version"`
	TaskID        string              `json:"task_id"`
	Outcome       Outcome             `json:"outcome"`
	Operator      string              `json:"operator"`
	Provenance    string              `json:"provenance"`
	AcceptedAt    time.Time           `json:"accepted_at"`
	Subject       string              `json:"subject"`
	Decision      string              `json:"decision"`
	Evidence      []EvidenceReference `json:"evidence"`
	BuildSHA256   string              `json:"build_sha256,omitempty"`
	SourceSHA256  string              `json:"source_sha256,omitempty"`
}

type Snapshot struct {
	Receipt  Receipt
	Path     string
	SHA256   string
	ByteSize int
	Raw      []byte
}

func ExpectedReceiptPath(taskID string) string {
	return path.Join(ReceiptRoot, taskID, "receipt.json")
}

func (r Receipt) Validate(expectedTaskID string) error {
	if r.SchemaVersion != ReceiptSchemaVersion {
		return fmt.Errorf("operator checkpoint receipt: unsupported schema_version %q (want %q)", r.SchemaVersion, ReceiptSchemaVersion)
	}
	if !validIdentity(r.TaskID) {
		return fmt.Errorf("operator checkpoint receipt: invalid task_id %q", r.TaskID)
	}
	if expectedTaskID != "" && r.TaskID != expectedTaskID {
		return fmt.Errorf("operator checkpoint receipt: task_id %q does not match checkpoint %q", r.TaskID, expectedTaskID)
	}
	if r.Outcome != OutcomeAccepted {
		return fmt.Errorf("operator checkpoint receipt: unsupported accepted outcome %q", r.Outcome)
	}
	for _, value := range []struct {
		name  string
		text  string
		limit int
	}{
		{name: "operator", text: r.Operator, limit: 512},
		{name: "provenance", text: r.Provenance, limit: 1024},
		{name: "subject", text: r.Subject, limit: 4096},
		{name: "decision", text: r.Decision, limit: 4096},
	} {
		if err := validateText(value.name, value.text, value.limit); err != nil {
			return err
		}
	}
	if r.AcceptedAt.IsZero() || r.AcceptedAt.Location() != time.UTC {
		return errors.New("operator checkpoint receipt: accepted_at must be an explicit UTC timestamp")
	}
	if len(r.Evidence) == 0 {
		return errors.New("operator checkpoint receipt: at least one evidence reference is required")
	}
	seen := make(map[string]struct{}, len(r.Evidence))
	for i, reference := range r.Evidence {
		if !validEvidenceKind(reference.Kind) {
			return fmt.Errorf("operator checkpoint receipt: evidence[%d].kind has unsupported value %q", i, reference.Kind)
		}
		if err := validateReferencePath(reference.Path); err != nil {
			return fmt.Errorf("operator checkpoint receipt: evidence[%d].path: %w", i, err)
		}
		if !ValidSHA256(reference.SHA256) {
			return fmt.Errorf("operator checkpoint receipt: evidence[%d].sha256 is malformed", i)
		}
		identity := string(reference.Kind) + "\x00" + reference.Path
		if _, exists := seen[identity]; exists {
			return fmt.Errorf("operator checkpoint receipt: evidence[%d] duplicates %s reference %q", i, reference.Kind, reference.Path)
		}
		seen[identity] = struct{}{}
	}
	for _, identity := range []struct {
		name string
		sha  string
	}{{"build_sha256", r.BuildSHA256}, {"source_sha256", r.SourceSHA256}} {
		if identity.sha != "" && !ValidSHA256(identity.sha) {
			return fmt.Errorf("operator checkpoint receipt: %s is malformed", identity.name)
		}
	}
	return nil
}

func Decode(raw []byte, expectedTaskID string) (Receipt, error) {
	if len(raw) == 0 {
		return Receipt{}, errors.New("decode operator checkpoint receipt: input is empty")
	}
	if len(raw) > maxReceiptBytes {
		return Receipt{}, fmt.Errorf("decode operator checkpoint receipt: input exceeds %d bytes", maxReceiptBytes)
	}
	if err := rejectDuplicateFields(raw); err != nil {
		return Receipt{}, fmt.Errorf("decode operator checkpoint receipt: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var receipt Receipt
	if err := decoder.Decode(&receipt); err != nil {
		return Receipt{}, fmt.Errorf("decode operator checkpoint receipt: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return Receipt{}, errors.New("decode operator checkpoint receipt: multiple JSON values are not allowed")
	} else if !errors.Is(err, io.EOF) {
		return Receipt{}, fmt.Errorf("decode operator checkpoint receipt: trailing JSON: %w", err)
	}
	if err := receipt.Validate(expectedTaskID); err != nil {
		return Receipt{}, err
	}
	return cloneReceipt(receipt), nil
}

func Marshal(receipt Receipt) ([]byte, error) {
	if err := receipt.Validate(receipt.TaskID); err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(cloneReceipt(receipt), "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal operator checkpoint receipt: %w", err)
	}
	return append(raw, '\n'), nil
}

// Load validates the canonical path, reads a non-symlink regular file, and
// returns the exact receipt identity used by repository scheduling.
func Load(repositoryRoot, receiptPath, expectedTaskID string) (Snapshot, error) {
	boundary, err := runtimepath.Bind(repositoryRoot)
	if err != nil {
		return Snapshot{}, fmt.Errorf("load operator checkpoint receipt %s: bind repository root: %w", receiptPath, err)
	}
	return load(boundary, receiptPath, expectedTaskID)
}

func load(boundary runtimepath.Boundary, receiptPath, expectedTaskID string) (Snapshot, error) {
	absPath, err := validateCanonicalReceiptPath(boundary, expectedTaskID, receiptPath)
	if err != nil {
		return Snapshot{}, err
	}
	raw, found, err := boundary.ReadFileLimit(absPath, false, maxReceiptBytes)
	if errors.Is(err, runtimepath.ErrReadLimit) {
		return Snapshot{}, fmt.Errorf("load operator checkpoint receipt %s: file exceeds %d bytes", receiptPath, maxReceiptBytes)
	}
	if err != nil {
		return Snapshot{}, fmt.Errorf("load operator checkpoint receipt %s: %w", receiptPath, err)
	}
	if !found {
		return Snapshot{}, fmt.Errorf("load operator checkpoint receipt %s: file is missing", receiptPath)
	}
	receipt, err := Decode(raw, expectedTaskID)
	if err != nil {
		return Snapshot{}, fmt.Errorf("load operator checkpoint receipt %s: %w", receiptPath, err)
	}
	return Snapshot{Receipt: receipt, Path: receiptPath, SHA256: hashBytes(raw), ByteSize: len(raw), Raw: append([]byte(nil), raw...)}, nil
}

// ValidateCanonicalReceiptPath rejects alternate spellings, escapes, and any
// existing symlink component. The receipt may be absent while a checkpoint is
// still awaiting operator fulfillment.
func ValidateCanonicalReceiptPath(repositoryRoot, taskID, receiptPath string) (string, error) {
	boundary, err := runtimepath.Bind(repositoryRoot)
	if err != nil {
		return "", fmt.Errorf("resolve operator checkpoint repository root: %w", err)
	}
	return validateCanonicalReceiptPath(boundary, taskID, receiptPath)
}

func validateCanonicalReceiptPath(boundary runtimepath.Boundary, taskID, receiptPath string) (string, error) {
	if !validIdentity(taskID) {
		return "", fmt.Errorf("invalid operator checkpoint task id %q", taskID)
	}
	expected := ExpectedReceiptPath(taskID)
	if receiptPath != expected {
		return "", fmt.Errorf("invalid checkpoint_receipt_path %q for task %q: must be %q", receiptPath, taskID, expected)
	}
	absPath := filepath.Join(boundary.Root(), filepath.FromSlash(receiptPath))
	if err := boundary.CheckFile(absPath, true); err != nil {
		return "", fmt.Errorf("invalid checkpoint_receipt_path %q for task %q: %w", receiptPath, taskID, err)
	}
	return absPath, nil
}

func ValidSHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}

func validIdentity(value string) bool {
	if value == "" || value == "." || value == ".." || value != strings.TrimSpace(value) {
		return false
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

func validEvidenceKind(kind EvidenceKind) bool {
	switch kind {
	case EvidenceFile, EvidenceSource, EvidenceBuild, EvidenceArtifact:
		return true
	default:
		return false
	}
}

func validateReferencePath(value string) error {
	if value == "" || value != strings.TrimSpace(value) || strings.ContainsAny(value, "\\:\x00\r\n") || path.IsAbs(value) {
		return fmt.Errorf("reference %q must be a nonempty canonical repository-relative slash path", value)
	}
	clean := path.Clean(value)
	if clean != value || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("reference %q must be a nonempty canonical repository-relative slash path", value)
	}
	return nil
}

func rejectDuplicateFields(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var inspect func() error
	inspect = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := make(map[string]struct{})
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return errors.New("object key is not a string")
				}
				if _, exists := seen[key]; exists {
					return fmt.Errorf("duplicate field %q", key)
				}
				seen[key] = struct{}{}
				if err := inspect(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		case '[':
			for decoder.More() {
				if err := inspect(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		default:
			return fmt.Errorf("unexpected JSON delimiter %q", delim)
		}
	}
	return inspect()
}

func validateText(name, value string, limit int) error {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) || strings.ContainsAny(value, "\x00\r\n") || len(value) > limit {
		return fmt.Errorf("operator checkpoint receipt: %s must be nonempty single-line UTF-8 text of at most %d bytes", name, limit)
	}
	return nil
}

func cloneReceipt(receipt Receipt) Receipt {
	receipt.Evidence = append([]EvidenceReference(nil), receipt.Evidence...)
	return receipt
}

func hashBytes(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
