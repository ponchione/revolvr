// Package autonomousarchive owns tracked terminal-task archive transactions,
// immutable archive inspection, read-only verification, and explicit reopen
// coordination. It never runs Codex, verification, audit, or source work.
package autonomousarchive

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousfinalization"
	"revolvr/internal/taskfile"
)

const (
	ManifestSchemaVersion     = "autonomous-task-archive-manifest-v1"
	AuthoritySchemaVersion    = "autonomous-task-archive-authority-v1"
	JournalSchemaVersion      = "autonomous-task-archive-journal-v1"
	HistorySchemaVersion      = "autonomous-task-archive-transition-v1"
	ReopenRecordSchemaVersion = "autonomous-task-reopen-record-v1"
	LedgerEventSchemaVersion  = "autonomous-task-archive-ledger-event-v1"
	ArchiveRoot               = ".agent/archive"
)

type Disposition string

const (
	DispositionCompleted  Disposition = "completed"
	DispositionCancelled  Disposition = "cancelled"
	DispositionSuperseded Disposition = "superseded"
	DispositionAbandoned  Disposition = "abandoned"
)

type Artifact struct {
	Path     string `json:"path"`
	SHA256   string `json:"sha256"`
	ByteSize int    `json:"byte_size"`
}

func (a Artifact) Validate() error {
	if err := validatePath(a.Path); err != nil {
		return err
	}
	if !validHash(a.SHA256) || a.ByteSize < 0 {
		return errors.New("artifact hash or byte size is malformed")
	}
	return nil
}

type TerminalAuthority struct {
	SchemaVersion string      `json:"schema_version"`
	Disposition   Disposition `json:"disposition"`
	Reason        string      `json:"reason"`
	Provenance    string      `json:"provenance"`
	TerminalAt    time.Time   `json:"terminal_at"`
}

func (a TerminalAuthority) Validate() error {
	if a.SchemaVersion != AuthoritySchemaVersion || !a.Disposition.Valid() {
		return errors.New("terminal archive authority schema or disposition is invalid")
	}
	if err := validateText("reason", a.Reason); err != nil {
		return err
	}
	if err := validateText("provenance", a.Provenance); err != nil {
		return err
	}
	if a.TerminalAt.IsZero() || a.TerminalAt.Location() != time.UTC {
		return errors.New("terminal archive authority time must be explicit UTC")
	}
	return nil
}

func (d Disposition) Valid() bool {
	switch d {
	case DispositionCompleted, DispositionCancelled, DispositionSuperseded, DispositionAbandoned:
		return true
	default:
		return false
	}
}

type FinalizationIdentity struct {
	OperationID       string                       `json:"operation_id"`
	RunID             string                       `json:"run_id"`
	Stage             autonomous.FinalizationStage `json:"stage"`
	SourceRevision    string                       `json:"source_revision"`
	WorkspaceID       string                       `json:"workspace_id"`
	CheckpointCommit  string                       `json:"checkpoint_commit"`
	VerificationRunID string                       `json:"verification_run_id"`
	AuditRunID        string                       `json:"audit_run_id"`
	SafetyPolicySHA   string                       `json:"safety_policy_sha256"`
}

type LedgerIdentity struct {
	RunID             string `json:"run_id"`
	TerminalEventID   int64  `json:"terminal_event_id"`
	TerminalEventType string `json:"terminal_event_type"`
}

type Manifest struct {
	SchemaVersion      string                `json:"schema_version"`
	ArchiveID          string                `json:"archive_id"`
	OperationID        string                `json:"operation_id"`
	ArchiveRunID       string                `json:"archive_run_id"`
	TaskID             string                `json:"task_id"`
	Disposition        Disposition           `json:"disposition"`
	Reason             string                `json:"reason"`
	Provenance         string                `json:"provenance"`
	TerminalAt         time.Time             `json:"terminal_at"`
	ArchivedAt         time.Time             `json:"archived_at"`
	OriginalTask       Artifact              `json:"original_task"`
	ArchivedTask       Artifact              `json:"archived_task"`
	Workflow           string                `json:"workflow"`
	State              Artifact              `json:"state"`
	FrozenEvidence     *Artifact             `json:"frozen_evidence,omitempty"`
	CompletionCapsule  *Artifact             `json:"completion_capsule,omitempty"`
	CompletionManifest *Artifact             `json:"completion_manifest,omitempty"`
	Finalization       *FinalizationIdentity `json:"finalization,omitempty"`
	TerminalLedger     *LedgerIdentity       `json:"terminal_ledger,omitempty"`
	ExpectedPaths      []string              `json:"expected_paths"`
	Omissions          []string              `json:"omissions"`
}

func (m Manifest) Validate() error {
	if m.SchemaVersion != ManifestSchemaVersion || !validArchiveID(m.ArchiveID) || !validIdentity(m.OperationID) || !validIdentity(m.ArchiveRunID) || !validIdentity(m.TaskID) || !m.Disposition.Valid() {
		return errors.New("archive manifest identity is malformed")
	}
	if err := validateText("reason", m.Reason); err != nil {
		return err
	}
	if err := validateText("provenance", m.Provenance); err != nil {
		return err
	}
	if m.TerminalAt.IsZero() || m.ArchivedAt.IsZero() || m.ArchivedAt.Location() != time.UTC || m.TerminalAt.Location() != time.UTC || m.ArchivedAt.Before(m.TerminalAt) {
		return errors.New("archive manifest terminal/archive times must be monotonic explicit UTC")
	}
	for _, artifact := range []Artifact{m.OriginalTask, m.ArchivedTask, m.State} {
		if err := artifact.Validate(); err != nil {
			return err
		}
	}
	if m.OriginalTask.SHA256 != m.ArchivedTask.SHA256 || m.OriginalTask.ByteSize != m.ArchivedTask.ByteSize || m.Workflow != "autonomous-v1" {
		return errors.New("archive manifest task preservation or workflow identity is invalid")
	}
	wantBase := path.Join(ArchiveRoot, m.ArchivedAt.Format("2006"), m.ArchivedAt.Format("01"), m.TaskID)
	if m.ArchivedTask.Path != path.Join(wantBase, "task.md") {
		return errors.New("archive manifest task path does not match UTC identity")
	}
	wantPaths := []string{m.ArchivedTask.Path, path.Join(wantBase, "archive.json")}
	if m.Disposition == DispositionCompleted {
		if m.FrozenEvidence == nil || m.CompletionCapsule == nil || m.CompletionManifest == nil || m.Finalization == nil || m.TerminalLedger == nil {
			return errors.New("completed archive requires complete AW-20 identities")
		}
		if m.CompletionCapsule.Path != path.Join(wantBase, "completion.md") {
			return errors.New("completed archive capsule path is not canonical")
		}
		for _, artifact := range []*Artifact{m.FrozenEvidence, m.CompletionCapsule, m.CompletionManifest} {
			if err := artifact.Validate(); err != nil {
				return err
			}
		}
		wantPaths = append(wantPaths, m.CompletionCapsule.Path)
		if m.Finalization.Stage != autonomous.FinalizationStageLedgerCompleted || !validIdentity(m.Finalization.OperationID) || !validIdentity(m.Finalization.RunID) || !validHash(m.Finalization.SourceRevision) || !validHash(m.Finalization.SafetyPolicySHA) || !validOID(m.Finalization.CheckpointCommit) || m.TerminalLedger.RunID != m.Finalization.RunID || m.TerminalLedger.TerminalEventID <= 0 {
			return errors.New("completed archive finalization or ledger identity is malformed")
		}
	} else {
		if m.FrozenEvidence != nil || m.CompletionCapsule != nil || m.CompletionManifest != nil || m.Finalization != nil || m.TerminalLedger != nil {
			return errors.New("non-completed archive must not claim completion evidence")
		}
	}
	if !equalStrings(m.ExpectedPaths, wantPaths) {
		return errors.New("archive manifest expected paths are not canonical")
	}
	if len(m.Omissions) == 0 {
		return errors.New("archive manifest requires explicit omission facts")
	}
	for _, omission := range m.Omissions {
		if err := validateText("omission", omission); err != nil {
			return err
		}
	}
	return nil
}

type Stage string

const (
	StageAdmitted       Stage = "admitted"
	StageFilesPublished Stage = "files_published"
	StageActiveRemoved  Stage = "active_task_removed"
	StageCommitted      Stage = "commit_reconciled"
	StageLedgerComplete Stage = "ledger_completed"
)

type Journal struct {
	SchemaVersion string    `json:"schema_version"`
	ArchiveID     string    `json:"archive_id"`
	OperationID   string    `json:"operation_id"`
	TaskID        string    `json:"task_id"`
	Stage         Stage     `json:"stage"`
	Manifest      Artifact  `json:"manifest"`
	CommitSHA     string    `json:"commit_sha,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type HistoryRecord struct {
	SchemaVersion string    `json:"schema_version"`
	ArchiveID     string    `json:"archive_id"`
	OperationID   string    `json:"operation_id"`
	TaskID        string    `json:"task_id"`
	Sequence      int64     `json:"sequence"`
	Stage         Stage     `json:"stage"`
	Manifest      Artifact  `json:"manifest"`
	CommitSHA     string    `json:"commit_sha,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

type Check struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

type VerificationReport struct {
	ArchiveID string  `json:"archive_id"`
	TaskID    string  `json:"task_id"`
	Passed    bool    `json:"passed"`
	Checks    []Check `json:"checks"`
}

type Entry struct {
	Manifest      Manifest
	ManifestBytes []byte
	ManifestPath  string
	CommitSHA     string
	Verified      bool
}

// EvidenceSnapshot is the strict task/state evidence available from archive
// Show. VerifiedNow is false because this does not perform full Verify.
type EvidenceSnapshot struct {
	Entry       Entry
	Task        taskfile.Task
	State       autonomous.ExecutionState
	StateBytes  []byte
	Frozen      *autonomousfinalization.FrozenEvidence
	VerifiedNow bool
}

type ReopenRecord struct {
	SchemaVersion  string                   `json:"schema_version"`
	OperationID    string                   `json:"operation_id"`
	ArchiveID      string                   `json:"archive_id"`
	ArchivedTaskID string                   `json:"archived_task_id"`
	NewTaskID      string                   `json:"new_task_id"`
	Task           Artifact                 `json:"task"`
	State          Artifact                 `json:"state"`
	Lineage        autonomous.ReopenLineage `json:"lineage"`
	CommitSHA      string                   `json:"commit_sha"`
	CreatedAt      time.Time                `json:"created_at"`
}

func Marshal(value any) ([]byte, error) {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func ArchiveID(taskID, operationID string, disposition Disposition, archivedAt time.Time) string {
	material := strings.Join([]string{taskID, operationID, string(disposition), archivedAt.UTC().Format(time.RFC3339Nano)}, "\x00")
	sum := sha256.Sum256([]byte(material))
	return "archive-" + hex.EncodeToString(sum[:16])
}

func artifact(path string, raw []byte) Artifact {
	sum := sha256.Sum256(raw)
	return Artifact{Path: filepathSlash(path), SHA256: hex.EncodeToString(sum[:]), ByteSize: len(raw)}
}

func validHash(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == sha256.Size && value == strings.ToLower(value)
}

func validOID(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && (len(raw) == 20 || len(raw) == 32) && value == strings.ToLower(value)
}

func validIdentity(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || strings.ContainsAny(value, "\r\n/\\") || value == "." || value == ".." {
		return false
	}
	return true
}

func validArchiveID(value string) bool {
	if !strings.HasPrefix(value, "archive-") || len(value) != len("archive-")+32 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "archive-"))
	return err == nil
}

func validateText(label, value string) error {
	if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) || strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("%s is empty or malformed", label)
	}
	return nil
}

func validatePath(value string) error {
	if value == "" || value != strings.TrimSpace(value) || strings.HasPrefix(value, "/") || path.Clean(value) != value || value == "." || value == ".." || strings.HasPrefix(value, "../") {
		return fmt.Errorf("path %q is not normalized and repository-relative", value)
	}
	return nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func filepathSlash(value string) string { return strings.ReplaceAll(value, "\\", "/") }
