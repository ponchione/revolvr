package autonomous

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"revolvr/internal/autonomousverification"
)

const DossierManifestSchemaVersion = "autonomous-task-dossier-manifest-v1"

type VerificationStatus string

const (
	VerificationStatusPassed VerificationStatus = "passed"
	VerificationStatusFailed VerificationStatus = "failed"
)

type DossierSourceKind string

const (
	DossierSourceKindTaskSpec           DossierSourceKind = "task_spec"
	DossierSourceKindExecutionState     DossierSourceKind = "execution_state"
	DossierSourceKindVerification       DossierSourceKind = "verification_summary"
	DossierSourceKindAudit              DossierSourceKind = "audit_report"
	DossierSourceKindRecentRuns         DossierSourceKind = "recent_runs"
	DossierSourceKindReceipt            DossierSourceKind = "receipt"
	DossierSourceKindGitSnapshot        DossierSourceKind = "git_snapshot"
	DossierSourceKindRepositoryGuidance DossierSourceKind = "repository_guidance"
)

type TaskSpecSource struct {
	ID      string `json:"id"`
	Path    string `json:"path,omitempty"`
	Label   string `json:"label,omitempty"`
	Content []byte `json:"-"`
}

type GuidanceSource struct {
	ID      string `json:"id"`
	Path    string `json:"path"`
	Label   string `json:"label,omitempty"`
	Content []byte `json:"-"`
}

// ReceiptSource is manifest-only provenance for exact receipt bytes that
// influenced assembled run or verification evidence.
type ReceiptSource struct {
	ID      string `json:"id"`
	Path    string `json:"path"`
	Content []byte `json:"-"`
}

// DossierSourceWindow describes the complete bounded history window supplied
// to BuildTaskDossier. HasOlderItems is true only when the collector directly
// detected at least one older selected-task item beyond Limit; it is not a
// claim about a global item total.
type DossierSourceWindow struct {
	Limit         int  `json:"limit"`
	HasOlderItems bool `json:"has_older_items"`
}

// RecentRunSummary is deliberately smaller than the ledger schema. Run
// history is rendered newest-first by StartedAt, with RunID ascending as the
// deterministic tie-breaker.
type RecentRunSummary struct {
	RunID       string              `json:"run_id"`
	TaskID      string              `json:"task_id"`
	Action      Action              `json:"action,omitempty"`
	Profile     WorkerProfile       `json:"profile,omitempty"`
	Outcome     string              `json:"outcome"`
	StartedAt   time.Time           `json:"started_at"`
	CompletedAt *time.Time          `json:"completed_at,omitempty"`
	Evidence    []EvidenceReference `json:"evidence,omitempty"`
}

type VerificationSummary struct {
	TaskID       string                         `json:"task_id"`
	Status       VerificationStatus             `json:"status"`
	Command      string                         `json:"command,omitempty"`
	Summary      string                         `json:"summary"`
	RunID        string                         `json:"run_id,omitempty"`
	OccurrenceID string                         `json:"occurrence_id,omitempty"`
	Evidence     []EvidenceReference            `json:"evidence"`
	Tiered       *autonomousverification.Result `json:"tiered,omitempty"`
}

type GitSnapshot struct {
	Head           string             `json:"head"`
	WorktreeStatus string             `json:"worktree_status"`
	DiffSummary    string             `json:"diff_summary"`
	Evidence       *EvidenceReference `json:"evidence,omitempty"`
}

type TaskDossierInput struct {
	TaskID          string
	TaskSpec        TaskSpecSource
	State           ExecutionState
	Verification    *VerificationSummary
	Audit           *AuditReport
	RecentRuns      []RecentRunSummary
	RecentRunLimit  int
	RecentRunWindow *DossierSourceWindow
	Receipts        []ReceiptSource
	Git             *GitSnapshot
	Guidance        []GuidanceSource
}

type TaskDossier struct {
	Markdown []byte
	Manifest TaskDossierManifest
}

type TaskDossierManifest struct {
	SchemaVersion   string                  `json:"schema_version"`
	TaskID          string                  `json:"task_id"`
	DossierSHA256   string                  `json:"dossier_sha256"`
	DossierByteSize int                     `json:"dossier_byte_size"`
	Sources         []DossierSourceRecord   `json:"sources"`
	ProjectionFacts []DossierProjectionFact `json:"projection_facts"`
}

type DossierSourceRecord struct {
	Kind             DossierSourceKind    `json:"kind"`
	ID               string               `json:"id"`
	Path             string               `json:"path,omitempty"`
	SHA256           string               `json:"sha256"`
	ByteSize         int                  `json:"byte_size"`
	IncludedByteSize *int                 `json:"included_byte_size,omitempty"`
	Items            *DossierItemCounts   `json:"items,omitempty"`
	SourceWindow     *DossierSourceWindow `json:"source_window,omitempty"`
	Truncated        bool                 `json:"truncated"`
}

type DossierItemCounts struct {
	Total    int `json:"total"`
	Included int `json:"included"`
	Omitted  int `json:"omitted"`
}

type DossierProjectionFact struct {
	Section  string `json:"section"`
	Reason   string `json:"reason"`
	Total    int    `json:"total_items,omitempty"`
	Included int    `json:"included_items,omitempty"`
	Omitted  int    `json:"omitted_items,omitempty"`
}

type normalizedDossierInput struct {
	input        TaskDossierInput
	sortedRuns   []RecentRunSummary
	includedRuns []RecentRunSummary
	receipts     []ReceiptSource
	guidance     []GuidanceSource
	facts        []DossierProjectionFact
}

// BuildTaskDossier validates already-supplied evidence and projects it without
// reading files, executing commands, consulting a clock, or mutating input.
func BuildTaskDossier(in TaskDossierInput) (TaskDossier, error) {
	normalized, err := validateAndNormalizeDossierInput(in)
	if err != nil {
		return TaskDossier{}, err
	}

	markdown := renderTaskDossierMarkdown(normalized)
	sources, err := buildDossierSourceRecords(normalized)
	if err != nil {
		return TaskDossier{}, err
	}
	manifest := TaskDossierManifest{
		SchemaVersion:   DossierManifestSchemaVersion,
		TaskID:          in.TaskID,
		DossierSHA256:   sha256HexBytes(markdown),
		DossierByteSize: len(markdown),
		Sources:         sources,
		ProjectionFacts: append([]DossierProjectionFact(nil), normalized.facts...),
	}
	return TaskDossier{Markdown: markdown, Manifest: manifest}, nil
}

func MarshalTaskDossierManifest(manifest TaskDossierManifest) ([]byte, error) {
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal task dossier manifest: %w", err)
	}
	return append(raw, '\n'), nil
}

func validateAndNormalizeDossierInput(in TaskDossierInput) (normalizedDossierInput, error) {
	if strings.TrimSpace(in.TaskID) == "" {
		return normalizedDossierInput{}, errors.New("build task dossier: task_id is required")
	}
	if err := validateStableText("task_id", in.TaskID); err != nil {
		return normalizedDossierInput{}, fmt.Errorf("build task dossier: %w", err)
	}
	if err := validateTaskSpecSource(in.TaskSpec); err != nil {
		return normalizedDossierInput{}, err
	}
	if err := in.State.Validate(); err != nil {
		return normalizedDossierInput{}, fmt.Errorf("build task dossier: execution_state: %w", err)
	}
	if in.TaskID != in.State.TaskID {
		return normalizedDossierInput{}, fmt.Errorf("build task dossier: task_id %q does not match execution_state task_id %q", in.TaskID, in.State.TaskID)
	}
	if in.RecentRunLimit < 0 {
		return normalizedDossierInput{}, fmt.Errorf("build task dossier: recent_run_limit cannot be negative (got %d; zero explicitly includes no runs)", in.RecentRunLimit)
	}
	if err := validateRecentRunWindow(in.RecentRunWindow, in.RecentRunLimit, len(in.RecentRuns)); err != nil {
		return normalizedDossierInput{}, err
	}
	if in.Verification != nil {
		if err := validateVerificationSummary(*in.Verification, in.TaskID); err != nil {
			return normalizedDossierInput{}, err
		}
	}
	if in.Audit != nil {
		if err := in.Audit.Validate(); err != nil {
			return normalizedDossierInput{}, fmt.Errorf("build task dossier: audit: %w", err)
		}
		if in.Audit.TaskID != in.TaskID {
			return normalizedDossierInput{}, fmt.Errorf("build task dossier: audit task_id %q does not match dossier task_id %q", in.Audit.TaskID, in.TaskID)
		}
	}
	if in.Git != nil {
		if err := validateGitSnapshot(*in.Git); err != nil {
			return normalizedDossierInput{}, err
		}
	}

	runs, err := validateAndSortRecentRuns(in.RecentRuns, in.TaskID)
	if err != nil {
		return normalizedDossierInput{}, err
	}
	receipts, err := validateAndSortReceipts(in.Receipts)
	if err != nil {
		return normalizedDossierInput{}, err
	}
	guidance, err := validateAndSortGuidance(in.Guidance)
	if err != nil {
		return normalizedDossierInput{}, err
	}

	includedCount := in.RecentRunLimit
	if includedCount > len(runs) {
		includedCount = len(runs)
	}
	included := append([]RecentRunSummary(nil), runs[:includedCount]...)
	facts := dossierProjectionFacts(in, len(runs), includedCount)
	return normalizedDossierInput{
		input:        in,
		sortedRuns:   runs,
		includedRuns: included,
		receipts:     receipts,
		guidance:     guidance,
		facts:        facts,
	}, nil
}

func validateRecentRunWindow(window *DossierSourceWindow, renderLimit int, supplied int) error {
	if window == nil {
		return nil
	}
	if window.Limit < 0 {
		return fmt.Errorf("build task dossier: recent_run_window.limit cannot be negative (got %d)", window.Limit)
	}
	if renderLimit > window.Limit {
		return fmt.Errorf("build task dossier: recent_run_limit %d exceeds recent_run_window.limit %d", renderLimit, window.Limit)
	}
	if supplied > window.Limit {
		return fmt.Errorf("build task dossier: %d recent runs supplied exceeds recent_run_window.limit %d", supplied, window.Limit)
	}
	if window.HasOlderItems && window.Limit > 0 && supplied != window.Limit {
		return fmt.Errorf("build task dossier: recent_run_window.has_older_items requires a full source window of %d items (got %d)", window.Limit, supplied)
	}
	return nil
}

func validateTaskSpecSource(source TaskSpecSource) error {
	if strings.TrimSpace(source.ID) == "" {
		return errors.New("build task dossier: task_spec.id is required")
	}
	if err := validateStableText("task_spec.id", source.ID); err != nil {
		return fmt.Errorf("build task dossier: %w", err)
	}
	if source.Path != "" {
		if err := validateRepositoryRelativePath("task_spec.path", source.Path); err != nil {
			return fmt.Errorf("build task dossier: %w", err)
		}
	}
	if source.Label != "" {
		if err := validateStableText("task_spec.label", source.Label); err != nil {
			return fmt.Errorf("build task dossier: %w", err)
		}
	}
	if len(source.Content) == 0 {
		return errors.New("build task dossier: task_spec.content is required")
	}
	if !utf8.Valid(source.Content) {
		return errors.New("build task dossier: task_spec.content is not valid UTF-8")
	}
	return nil
}

func validateVerificationSummary(summary VerificationSummary, taskID string) error {
	if strings.TrimSpace(summary.TaskID) == "" {
		return errors.New("build task dossier: verification.task_id is required")
	}
	if summary.TaskID != taskID {
		return fmt.Errorf("build task dossier: verification task_id %q does not match dossier task_id %q", summary.TaskID, taskID)
	}
	if summary.Status != VerificationStatusPassed && summary.Status != VerificationStatusFailed {
		return fmt.Errorf("build task dossier: verification.status has unknown value %q", summary.Status)
	}
	if strings.TrimSpace(summary.Summary) == "" {
		return errors.New("build task dossier: verification.summary is required")
	}
	if summary.RunID != "" {
		if err := validateStableText("verification.run_id", summary.RunID); err != nil {
			return fmt.Errorf("build task dossier: %w", err)
		}
	}
	if summary.OccurrenceID != "" {
		if err := validateStableText("verification.occurrence_id", summary.OccurrenceID); err != nil {
			return fmt.Errorf("build task dossier: %w", err)
		}
	}
	if err := validateEvidenceReferences("build task dossier: verification.evidence", summary.Evidence); err != nil {
		return err
	}
	if summary.Tiered != nil {
		if err := summary.Tiered.Validate(); err != nil {
			return fmt.Errorf("build task dossier: verification.tiered: %w", err)
		}
		if summary.Tiered.TaskID != summary.TaskID || summary.Tiered.RunID != summary.RunID || summary.Tiered.OccurrenceID != summary.OccurrenceID {
			return errors.New("build task dossier: tiered verification task/run/occurrence identity mismatch")
		}
	}
	return nil
}

func validateGitSnapshot(snapshot GitSnapshot) error {
	if strings.TrimSpace(snapshot.Head) == "" {
		return errors.New("build task dossier: git.head is required")
	}
	if strings.TrimSpace(snapshot.WorktreeStatus) == "" {
		return errors.New("build task dossier: git.worktree_status is required")
	}
	if strings.TrimSpace(snapshot.DiffSummary) == "" {
		return errors.New("build task dossier: git.diff_summary is required")
	}
	if snapshot.Evidence != nil {
		if err := validateEvidenceReferences("build task dossier: git.evidence", []EvidenceReference{*snapshot.Evidence}); err != nil {
			return err
		}
	}
	return nil
}

func validateAndSortRecentRuns(runs []RecentRunSummary, taskID string) ([]RecentRunSummary, error) {
	result := make([]RecentRunSummary, len(runs))
	copy(result, runs)
	seen := make(map[string]struct{}, len(result))
	for i, run := range result {
		prefix := fmt.Sprintf("build task dossier: recent_runs[%d]", i)
		if strings.TrimSpace(run.RunID) == "" {
			return nil, fmt.Errorf("%s.run_id is required", prefix)
		}
		if err := validateStableText(fmt.Sprintf("recent_runs[%d].run_id", i), run.RunID); err != nil {
			return nil, fmt.Errorf("build task dossier: %w", err)
		}
		if _, exists := seen[run.RunID]; exists {
			return nil, fmt.Errorf("%s.run_id duplicates run identity %q", prefix, run.RunID)
		}
		seen[run.RunID] = struct{}{}
		if strings.TrimSpace(run.TaskID) == "" {
			return nil, fmt.Errorf("%s.task_id is required", prefix)
		}
		if run.TaskID != taskID {
			return nil, fmt.Errorf("%s.task_id %q does not match dossier task_id %q", prefix, run.TaskID, taskID)
		}
		if run.Action != "" && !validAction(run.Action) {
			return nil, fmt.Errorf("%s.action has unknown value %q", prefix, run.Action)
		}
		if run.Profile != "" && !validWorkerProfile(run.Profile) {
			return nil, fmt.Errorf("%s.profile has unknown value %q", prefix, run.Profile)
		}
		if run.Action != "" && run.Profile != "" {
			expected, workerAction := workerProfileForAction(run.Action)
			if !workerAction || expected != run.Profile {
				return nil, fmt.Errorf("%s action %q is incompatible with profile %q", prefix, run.Action, run.Profile)
			}
		}
		if strings.TrimSpace(run.Outcome) == "" {
			return nil, fmt.Errorf("%s.outcome is required", prefix)
		}
		if run.StartedAt.IsZero() {
			return nil, fmt.Errorf("%s.started_at is required", prefix)
		}
		if run.CompletedAt != nil {
			if run.CompletedAt.IsZero() {
				return nil, fmt.Errorf("%s.completed_at must be non-zero when supplied", prefix)
			}
			if run.CompletedAt.Before(run.StartedAt) {
				return nil, fmt.Errorf("%s.completed_at %q precedes started_at %q", prefix, formatTime(*run.CompletedAt), formatTime(run.StartedAt))
			}
		}
		if err := validateOptionalEvidenceReferences(prefix+".evidence", run.Evidence); err != nil {
			return nil, err
		}
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].StartedAt.Equal(result[j].StartedAt) {
			return result[i].RunID < result[j].RunID
		}
		return result[i].StartedAt.After(result[j].StartedAt)
	})
	return result, nil
}

func validateAndSortGuidance(sources []GuidanceSource) ([]GuidanceSource, error) {
	result := append([]GuidanceSource(nil), sources...)
	seenIDs := make(map[string]struct{}, len(result))
	seenPaths := make(map[string]struct{}, len(result))
	for i, source := range result {
		prefix := fmt.Sprintf("build task dossier: guidance[%d]", i)
		if strings.TrimSpace(source.ID) == "" {
			return nil, fmt.Errorf("%s.id is required", prefix)
		}
		if err := validateStableText(fmt.Sprintf("guidance[%d].id", i), source.ID); err != nil {
			return nil, fmt.Errorf("build task dossier: %w", err)
		}
		if _, exists := seenIDs[source.ID]; exists {
			return nil, fmt.Errorf("%s.id duplicates guidance identity %q", prefix, source.ID)
		}
		seenIDs[source.ID] = struct{}{}
		if err := validateRepositoryRelativePath(fmt.Sprintf("guidance[%d].path", i), source.Path); err != nil {
			return nil, fmt.Errorf("build task dossier: %w", err)
		}
		if _, exists := seenPaths[source.Path]; exists {
			return nil, fmt.Errorf("%s.path duplicates guidance path %q", prefix, source.Path)
		}
		seenPaths[source.Path] = struct{}{}
		if source.Label != "" {
			if err := validateStableText(fmt.Sprintf("guidance[%d].label", i), source.Label); err != nil {
				return nil, fmt.Errorf("build task dossier: %w", err)
			}
		}
		if !utf8.Valid(source.Content) {
			return nil, fmt.Errorf("%s.content is not valid UTF-8", prefix)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Path == result[j].Path {
			return result[i].ID < result[j].ID
		}
		return result[i].Path < result[j].Path
	})
	return result, nil
}

func validateAndSortReceipts(sources []ReceiptSource) ([]ReceiptSource, error) {
	result := append([]ReceiptSource(nil), sources...)
	seenIDs := make(map[string]struct{}, len(result))
	seenPaths := make(map[string]struct{}, len(result))
	for i, source := range result {
		prefix := fmt.Sprintf("build task dossier: receipts[%d]", i)
		if strings.TrimSpace(source.ID) == "" {
			return nil, fmt.Errorf("%s.id is required", prefix)
		}
		if err := validateStableText(fmt.Sprintf("receipts[%d].id", i), source.ID); err != nil {
			return nil, fmt.Errorf("build task dossier: %w", err)
		}
		if _, exists := seenIDs[source.ID]; exists {
			return nil, fmt.Errorf("%s.id duplicates receipt identity %q", prefix, source.ID)
		}
		seenIDs[source.ID] = struct{}{}
		if err := validateRepositoryRelativePath(fmt.Sprintf("receipts[%d].path", i), source.Path); err != nil {
			return nil, fmt.Errorf("build task dossier: %w", err)
		}
		if _, exists := seenPaths[source.Path]; exists {
			return nil, fmt.Errorf("%s.path duplicates receipt path %q", prefix, source.Path)
		}
		seenPaths[source.Path] = struct{}{}
		if len(source.Content) == 0 {
			return nil, fmt.Errorf("%s.content is required", prefix)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Path == result[j].Path {
			return result[i].ID < result[j].ID
		}
		return result[i].Path < result[j].Path
	})
	return result, nil
}

func validateStableText(field, value string) error {
	if value != strings.TrimSpace(value) {
		return fmt.Errorf("%s must not contain leading or trailing whitespace", field)
	}
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("%s must be a single line", field)
	}
	return nil
}

func validateRepositoryRelativePath(field, path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("%s is required", field)
	}
	if err := validateStableText(field, path); err != nil {
		return err
	}
	if filepath.IsAbs(path) {
		return fmt.Errorf("%s %q must be repository-relative", field, path)
	}
	clean := filepath.Clean(path)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean != path {
		return fmt.Errorf("%s %q must be a clean repository-relative path", field, path)
	}
	return nil
}

func validWorkerProfile(profile WorkerProfile) bool {
	switch profile {
	case WorkerProfilePlanner, WorkerProfileImplementer, WorkerProfileAuditor, WorkerProfileCorrector, WorkerProfileDocumentor, WorkerProfileSimplifier:
		return true
	default:
		return false
	}
}

func dossierProjectionFacts(in TaskDossierInput, totalRuns, includedRuns int) []DossierProjectionFact {
	facts := make([]DossierProjectionFact, 0, 6)
	if in.State.Plan == nil {
		facts = append(facts, DossierProjectionFact{Section: "current_plan", Reason: "not_present_in_execution_state"})
	}
	if in.Verification == nil {
		facts = append(facts, DossierProjectionFact{Section: "verification", Reason: "not_supplied"})
	}
	if in.Audit == nil {
		facts = append(facts, DossierProjectionFact{Section: "audit", Reason: "not_supplied"})
	}
	if totalRuns == 0 {
		facts = append(facts, DossierProjectionFact{Section: "recent_runs", Reason: "not_supplied"})
	} else if includedRuns < totalRuns {
		facts = append(facts, DossierProjectionFact{
			Section:  "recent_runs",
			Reason:   "history_limit",
			Total:    totalRuns,
			Included: includedRuns,
			Omitted:  totalRuns - includedRuns,
		})
	}
	if in.RecentRunWindow != nil && in.RecentRunWindow.HasOlderItems {
		facts = append(facts, DossierProjectionFact{Section: "recent_runs", Reason: "collection_limit"})
	}
	if in.Git == nil {
		facts = append(facts, DossierProjectionFact{Section: "git_snapshot", Reason: "not_supplied"})
	}
	if len(in.Guidance) == 0 {
		facts = append(facts, DossierProjectionFact{Section: "repository_guidance", Reason: "not_supplied"})
	}
	return facts
}

func buildDossierSourceRecords(in normalizedDossierInput) ([]DossierSourceRecord, error) {
	records := make([]DossierSourceRecord, 0, 6+len(in.guidance))
	records = append(records, rawDossierSourceRecord(DossierSourceKindTaskSpec, in.input.TaskSpec.ID, in.input.TaskSpec.Path, in.input.TaskSpec.Content))

	stateRecord, err := typedDossierSourceRecord(DossierSourceKindExecutionState, "execution-state", "", in.input.State)
	if err != nil {
		return nil, fmt.Errorf("build task dossier: canonicalize execution_state source: %w", err)
	}
	records = append(records, stateRecord)

	if in.input.Verification != nil {
		record, err := typedDossierSourceRecord(DossierSourceKindVerification, "current-verification", "", *in.input.Verification)
		if err != nil {
			return nil, fmt.Errorf("build task dossier: canonicalize verification source: %w", err)
		}
		records = append(records, record)
	}
	if in.input.Audit != nil {
		record, err := typedDossierSourceRecord(DossierSourceKindAudit, "latest-audit", "", *in.input.Audit)
		if err != nil {
			return nil, fmt.Errorf("build task dossier: canonicalize audit source: %w", err)
		}
		records = append(records, record)
	}

	runRecord, err := typedDossierSourceRecord(DossierSourceKindRecentRuns, "recent-runs", "", in.sortedRuns)
	if err != nil {
		return nil, fmt.Errorf("build task dossier: canonicalize recent_runs source: %w", err)
	}
	runRecord.Items = &DossierItemCounts{
		Total:    len(in.sortedRuns),
		Included: len(in.includedRuns),
		Omitted:  len(in.sortedRuns) - len(in.includedRuns),
	}
	runRecord.Truncated = len(in.includedRuns) < len(in.sortedRuns)
	if in.input.RecentRunWindow != nil {
		window := *in.input.RecentRunWindow
		runRecord.SourceWindow = &window
	}
	records = append(records, runRecord)
	for _, source := range in.receipts {
		records = append(records, rawDossierSourceRecord(DossierSourceKindReceipt, source.ID, source.Path, source.Content))
	}

	if in.input.Git != nil {
		record, err := typedDossierSourceRecord(DossierSourceKindGitSnapshot, "git-snapshot", "", *in.input.Git)
		if err != nil {
			return nil, fmt.Errorf("build task dossier: canonicalize git source: %w", err)
		}
		records = append(records, record)
	}
	for _, source := range in.guidance {
		records = append(records, rawDossierSourceRecord(DossierSourceKindRepositoryGuidance, source.ID, source.Path, source.Content))
	}
	return records, nil
}

// Typed source hashes are SHA-256 over encoding/json compact JSON. All typed
// dossier sources are structs or deterministically ordered slices, never maps.
func typedDossierSourceRecord(kind DossierSourceKind, id, path string, value any) (DossierSourceRecord, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return DossierSourceRecord{}, err
	}
	return DossierSourceRecord{
		Kind:      kind,
		ID:        id,
		Path:      path,
		SHA256:    sha256HexBytes(raw),
		ByteSize:  len(raw),
		Truncated: false,
	}, nil
}

func rawDossierSourceRecord(kind DossierSourceKind, id, path string, content []byte) DossierSourceRecord {
	included := len(content)
	return DossierSourceRecord{
		Kind:             kind,
		ID:               id,
		Path:             path,
		SHA256:           sha256HexBytes(content),
		ByteSize:         len(content),
		IncludedByteSize: &included,
		Truncated:        false,
	}
}

func renderTaskDossierMarkdown(in normalizedDossierInput) []byte {
	var out bytes.Buffer
	writeDossierIdentity(&out, in)
	writeTaskSpec(&out, in.input.TaskSpec)
	writeExecutionState(&out, in.input.State)
	writePlan(&out, in.input.State.Plan)
	writeAcceptanceCriteria(&out, in.input.State.AcceptanceCriteria)
	writeVerification(&out, in.input.Verification)
	writeAuditAndResolutions(&out, in.input.Audit, in.input.State.FindingResolutions)
	writeRecentRuns(&out, in)
	writeGitSnapshot(&out, in.input.Git)
	writeGuidance(&out, in.guidance)
	writeProjectionFacts(&out, in.facts)
	return out.Bytes()
}

func writeDossierIdentity(out *bytes.Buffer, in normalizedDossierInput) {
	out.WriteString("# Autonomous Task Dossier\n\n")
	out.WriteString("## Dossier Identity\n\n")
	writeField(out, "Task ID", in.input.TaskID)
	writeField(out, "Manifest schema", DossierManifestSchemaVersion)
	out.WriteByte('\n')
}

func writeTaskSpec(out *bytes.Buffer, source TaskSpecSource) {
	out.WriteString("## Canonical Task/Spec\n\n")
	writeField(out, "Source ID", source.ID)
	writeOptionalField(out, "Path/reference", source.Path)
	if source.Label != "" {
		writeField(out, "Label", source.Label)
	}
	out.WriteByte('\n')
	writeSourceContent(out, source.Content)
}

func writeExecutionState(out *bytes.Buffer, state ExecutionState) {
	out.WriteString("## Current Autonomous State\n\n")
	writeField(out, "Execution schema", state.SchemaVersion)
	writeField(out, "Task ID", state.TaskID)
	writeField(out, "Lifecycle", string(state.Lifecycle))
	if state.Plan == nil {
		writeField(out, "Plan progress", "no current plan")
	} else {
		terminal := 0
		for _, step := range state.Plan.Steps {
			if terminalPlanStepStatus(step.Status) {
				terminal++
			}
		}
		writeField(out, "Plan progress", fmt.Sprintf("%d/%d terminal steps; revision %s (%d); completed=%t", terminal, len(state.Plan.Steps), state.Plan.ID, state.Plan.Revision, state.Plan.Completed))
	}
	writeField(out, "Acceptance progress", acceptanceProgress(state.AcceptanceCriteria))
	writeField(out, "Finding resolution progress", findingResolutionProgress(state.FindingResolutions))
	if state.NeedsInput != nil {
		writeField(out, "Needs input", state.NeedsInput.Reason)
	}
	if state.Terminal != nil {
		writeField(out, "Terminal reason", state.Terminal.Reason)
		writeEvidence(out, "Terminal evidence", state.Terminal.Evidence)
	}

	out.WriteString("\n### Latest Supervisor Decision\n\n")
	if state.LatestDecision == nil {
		out.WriteString("No supervisor decision reference recorded.\n")
	} else {
		writeDecisionReference(out, *state.LatestDecision)
	}

	out.WriteString("\n### Attempts and Budgets\n\n")
	writeField(out, "Total attempts", fmt.Sprint(state.Attempts.TotalAttempts))
	writeField(out, "Consecutive failures", fmt.Sprint(state.Attempts.ConsecutiveFailures))
	if len(state.Attempts.ActionAttempts) == 0 {
		writeField(out, "Action attempts", "none")
	} else {
		out.WriteString("- Action attempts:\n")
		for _, attempt := range state.Attempts.ActionAttempts {
			fmt.Fprintf(out, "  - %s: %d\n", attempt.Action, attempt.Attempts)
		}
	}
	writeField(out, "Retry budget", formatCountBudget(state.Attempts.RetryBudget))
	writeField(out, "Elapsed-time budget", formatDurationBudget(state.Attempts.ElapsedTimeBudget))
	writeField(out, "Token budget", formatCountBudget(state.Attempts.TokenBudget))
	if len(state.Attempts.ActionBudgets) > 0 {
		out.WriteString("- Action budgets:\n")
		for _, budget := range state.Attempts.ActionBudgets {
			fmt.Fprintf(out, "  - %s: %s\n", budget.Action, formatCountBudget(budget.Budget))
		}
	}
	if len(state.Attempts.ActionStops) > 0 {
		out.WriteString("- Exhausted actions:\n")
		for _, stop := range state.Attempts.ActionStops {
			fmt.Fprintf(out, "  - %s: %s (%s)\n", stop.Budget.Action, stop.Reason, formatCountBudget(stop.Budget.ActionBudget))
		}
	}
	if state.Attempts.RepeatedSignatureLimit > 0 {
		writeField(out, "Repeated-signature limit", fmt.Sprint(state.Attempts.RepeatedSignatureLimit))
	}
	if state.Attempts.RequiredStrategyChangeFrom != "" {
		writeField(out, "Required strategy change from", state.Attempts.RequiredStrategyChangeFrom)
	}
	if len(state.Attempts.Events) > 0 {
		out.WriteString("- Durable attempt events:\n")
		for _, event := range state.Attempts.Events {
			fmt.Fprintf(out, "  - %020d %s attempt=%s action=%s decision=%s strategy=%s", event.Sequence, event.Kind, event.AttemptID, event.Action, event.Decision.DecisionID, event.StrategySHA256)
			if event.Kind == AttemptEventCompleted {
				occurrence := event.OccurrenceID
				if occurrence == "" {
					occurrence = "none"
				}
				fmt.Fprintf(out, " outcome=%s run=%s occurrence=%s duration=%s", event.Outcome, event.RunID, occurrence, event.Duration)
				if event.Tokens == nil {
					out.WriteString(" tokens=missing")
				} else {
					fmt.Fprintf(out, " tokens=%d", *event.Tokens)
				}
				if event.SourceAfterKnown {
					fmt.Fprintf(out, " source=%s->%s", event.SourceBefore, event.SourceAfter)
				} else {
					fmt.Fprintf(out, " source=%s->unknown", event.SourceBefore)
				}
				for _, signature := range event.Signatures {
					fmt.Fprintf(out, " signature[%s]=%s", signature.Kind, signature.SHA256)
				}
			}
			out.WriteByte('\n')
		}
	}
	if state.Attempts.LastFailure == nil {
		writeField(out, "Last failure", "none")
	} else {
		writeEvidence(out, "Last failure", []EvidenceReference{*state.Attempts.LastFailure})
	}
	if state.CircuitBreaker != nil {
		out.WriteString("\n### Circuit Breaker\n\n")
		writeField(out, "Reason", string(state.CircuitBreaker.Reason))
		writeField(out, "Trigger attempts", strings.Join(state.CircuitBreaker.TriggerAttemptIDs, ", "))
		if state.CircuitBreaker.TriggerSignature != nil {
			writeField(out, "Trigger signature", fmt.Sprintf("%s:%s", state.CircuitBreaker.TriggerSignature.Kind, state.CircuitBreaker.TriggerSignature.SHA256))
		}
		if state.CircuitBreaker.RequiredStrategy != "" {
			writeField(out, "Required strategy change from", state.CircuitBreaker.RequiredStrategy)
		}
		writeField(out, "Task-attempt budget", formatCountBudget(state.CircuitBreaker.Budget.TaskAttempts))
		writeField(out, "Action-attempt budget", fmt.Sprintf("%s: %s", state.CircuitBreaker.Budget.Action, formatCountBudget(state.CircuitBreaker.Budget.ActionBudget)))
		writeField(out, "Elapsed budget", formatDurationBudget(state.CircuitBreaker.Budget.Elapsed))
		writeField(out, "Token budget", formatCountBudget(state.CircuitBreaker.Budget.Tokens))
		writeEvidence(out, "Breaker evidence", state.CircuitBreaker.Evidence)
	}
	if len(state.OptionalRoles) > 0 {
		out.WriteString("\n### Optional Role Dispositions\n\n")
		for _, occurrence := range state.OptionalRoles {
			fmt.Fprintf(out, "- %020d role=%s outcome=%s decision=%s source=%s->%s audit=%s/%d verification=%s/%s", occurrence.Sequence, occurrence.Role, occurrence.Outcome, occurrence.Decision.DecisionID, occurrence.SourceBefore, occurrence.SourceAfter, occurrence.Gate.AuditWorkerRunID, occurrence.Gate.AuditRevision, occurrence.Gate.VerificationRunID, occurrence.Gate.VerificationOccurrenceID)
			if occurrence.Worker != nil {
				fmt.Fprintf(out, " attempt=%s worker=%s", occurrence.Worker.AttemptID, occurrence.Worker.RunID)
			}
			if occurrence.CommitSHA != "" {
				fmt.Fprintf(out, " commit=%s", occurrence.CommitSHA)
			}
			out.WriteByte('\n')
			writeEvidence(out, "  Evidence", occurrence.Evidence)
		}
	}
	out.WriteByte('\n')
}

func writePlan(out *bytes.Buffer, plan *TaskPlan) {
	out.WriteString("## Current Plan\n\n")
	if plan == nil {
		out.WriteString("No current plan is present in the execution state.\n\n")
		return
	}
	writeField(out, "Plan ID", plan.ID)
	writeField(out, "Revision", fmt.Sprint(plan.Revision))
	writeOptionalField(out, "Predecessor revision", plan.SupersedesPlanID)
	writeField(out, "Completed", fmt.Sprint(plan.Completed))
	writeEvidence(out, "Provenance", plan.Provenance)
	for i, step := range plan.Steps {
		fmt.Fprintf(out, "\n### Step %d: %s\n\n", i+1, step.ID)
		writeField(out, "Status", string(step.Status))
		writeField(out, "Description", step.Description)
		writeEvidence(out, "Evidence", step.Evidence)
		if step.Rationale != "" {
			writeField(out, "Skip rationale", step.Rationale)
		}
	}
	out.WriteByte('\n')
}

func writeAcceptanceCriteria(out *bytes.Buffer, criteria []AcceptanceCriterion) {
	out.WriteString("## Acceptance Criteria\n\n")
	if len(criteria) == 0 {
		out.WriteString("No acceptance criteria are present in the execution state.\n\n")
		return
	}
	for i, criterion := range criteria {
		fmt.Fprintf(out, "### Criterion %d: %s\n\n", i+1, criterion.ID)
		writeField(out, "Requirement", criterion.Requirement)
		writeField(out, "Disposition", string(criterion.Status))
		writeEvidence(out, "Evidence", criterion.Evidence)
		if criterion.Rationale != "" {
			writeField(out, "Rationale", criterion.Rationale)
		}
		if criterion.Source == nil {
			writeField(out, "Source", "none")
		} else {
			writeEvidence(out, "Source", []EvidenceReference{*criterion.Source})
		}
		out.WriteByte('\n')
	}
}

func writeVerification(out *bytes.Buffer, summary *VerificationSummary) {
	out.WriteString("## Verification\n\n")
	if summary == nil {
		out.WriteString("No verification evidence supplied.\n\n")
		return
	}
	writeField(out, "Status", string(summary.Status))
	writeOptionalField(out, "Command/tier", summary.Command)
	writeField(out, "Summary", summary.Summary)
	writeOptionalField(out, "Run ID", summary.RunID)
	writeOptionalField(out, "Occurrence ID", summary.OccurrenceID)
	writeEvidence(out, "Evidence", summary.Evidence)
	if summary.Tiered != nil {
		writeField(out, "Purpose", string(summary.Tiered.Purpose))
		writeField(out, "Overall outcome", string(summary.Tiered.Outcome))
		writeField(out, "Final gate satisfied", fmt.Sprintf("%t", summary.Tiered.Gate.FinalSatisfied))
		for _, tier := range summary.Tiered.Tiers {
			fmt.Fprintf(out, "\n### Verification Tier: %s\n\n", tier.ID)
			writeField(out, "Kind", string(tier.Kind))
			writeField(out, "Required for final", fmt.Sprintf("%t", tier.RequiredForFinal))
			writeField(out, "Outcome", string(tier.Outcome))
			for _, command := range tier.Commands {
				writeField(out, "Command identity", command.Identity.SHA256)
				writeField(out, "Command", command.Identity.Name+" "+strings.Join(command.Identity.Args, " "))
				writeField(out, "Command outcome", string(command.Outcome))
				for _, attempt := range command.Attempts {
					fmt.Fprintf(out, "- Attempt %d (%s): %s; exit=%d; timeout=%t; cancelled=%t; stdout_truncated=%d; stderr_truncated=%d\n", attempt.Number, attempt.AttemptID, attempt.Outcome, attempt.ExitCode, attempt.TimedOut, attempt.Cancelled, attempt.Stdout.TruncatedBytes, attempt.Stderr.TruncatedBytes)
				}
			}
		}
	}
	out.WriteByte('\n')
}

func writeAuditAndResolutions(out *bytes.Buffer, report *AuditReport, resolutions []FindingResolution) {
	out.WriteString("## Audit and Finding Resolutions\n\n")
	resolutionByID := make(map[string]FindingResolution, len(resolutions))
	for _, resolution := range resolutions {
		resolutionByID[resolution.FindingID] = resolution
	}
	seen := make(map[string]struct{})
	if report == nil {
		out.WriteString("No audit report supplied.\n")
	} else {
		writeField(out, "Audit disposition", string(report.Disposition))
		writeField(out, "Audit rationale", report.Rationale)
		writeEvidence(out, "Audit inputs", report.Inputs)
		if len(report.Findings) == 0 {
			out.WriteString("\nThe latest audit is clean and contains no findings.\n")
		}
		for i, finding := range report.Findings {
			seen[finding.ID] = struct{}{}
			fmt.Fprintf(out, "\n### Finding %d: %s\n\n", i+1, finding.ID)
			writeField(out, "Significance", string(finding.Significance))
			writeField(out, "Summary", finding.Summary)
			writeEvidence(out, "Evidence", finding.Evidence)
			writeField(out, "Required correction", finding.RequiredCorrection)
			resolution, exists := resolutionByID[finding.ID]
			if !exists {
				writeField(out, "Current resolution", "not recorded")
			} else {
				writeFindingResolution(out, resolution)
			}
		}
	}

	remaining := make([]FindingResolution, 0, len(resolutions))
	for _, resolution := range resolutions {
		if _, exists := seen[resolution.FindingID]; !exists {
			remaining = append(remaining, resolution)
		}
	}
	if len(remaining) == 0 {
		if report == nil {
			out.WriteString("\nNo finding resolutions are present in the execution state.\n")
		}
	} else {
		out.WriteString("\n### Other Tracked Finding Resolutions\n\n")
		for _, resolution := range remaining {
			writeField(out, "Finding ID", resolution.FindingID)
			writeFindingResolution(out, resolution)
		}
	}
	out.WriteByte('\n')
}

func writeRecentRuns(out *bytes.Buffer, in normalizedDossierInput) {
	out.WriteString("## Recent Runs\n\n")
	fmt.Fprintf(out, "- History limit: %d (zero includes no runs)\n", in.input.RecentRunLimit)
	fmt.Fprintf(out, "- Total supplied: %d\n", len(in.sortedRuns))
	fmt.Fprintf(out, "- Included: %d\n", len(in.includedRuns))
	if in.input.RecentRunWindow != nil {
		fmt.Fprintf(out, "- Collected source window: at most %d selected-task run(s)\n", in.input.RecentRunWindow.Limit)
		if in.input.RecentRunWindow.HasOlderItems {
			out.WriteString("- Older selected-task history: detected beyond the collected source window\n")
		} else {
			out.WriteString("- Older selected-task history: not detected\n")
		}
	}
	if len(in.sortedRuns) == 0 {
		out.WriteString("\nNo recent run summaries supplied.\n")
	} else if len(in.includedRuns) == 0 {
		out.WriteString("\nNo recent runs are included because the history limit is zero.\n")
	}
	for i, run := range in.includedRuns {
		fmt.Fprintf(out, "\n### Run %d: %s\n\n", i+1, run.RunID)
		if run.Action == "" {
			writeField(out, "Action", "not supplied")
		} else {
			writeField(out, "Action", string(run.Action))
		}
		if run.Profile == "" {
			writeField(out, "Profile", "not supplied")
		} else {
			writeField(out, "Profile", string(run.Profile))
		}
		writeField(out, "Outcome", run.Outcome)
		writeField(out, "Started at", formatTime(run.StartedAt))
		if run.CompletedAt == nil {
			writeField(out, "Completed at", "not supplied")
		} else {
			writeField(out, "Completed at", formatTime(*run.CompletedAt))
		}
		writeEvidence(out, "Evidence", run.Evidence)
	}
	omitted := len(in.sortedRuns) - len(in.includedRuns)
	if omitted > 0 {
		fmt.Fprintf(out, "\nOmitted %d older run(s) due to the history limit.\n", omitted)
	}
	out.WriteByte('\n')
}

func writeGitSnapshot(out *bytes.Buffer, snapshot *GitSnapshot) {
	out.WriteString("## Git Snapshot\n\n")
	if snapshot == nil {
		out.WriteString("No Git snapshot supplied.\n\n")
		return
	}
	writeField(out, "HEAD/baseline", snapshot.Head)
	writeField(out, "Worktree status", snapshot.WorktreeStatus)
	writeField(out, "Diff summary", snapshot.DiffSummary)
	if snapshot.Evidence == nil {
		writeField(out, "Evidence", "none")
	} else {
		writeEvidence(out, "Evidence", []EvidenceReference{*snapshot.Evidence})
	}
	out.WriteByte('\n')
}

func writeGuidance(out *bytes.Buffer, sources []GuidanceSource) {
	out.WriteString("## Repository Guidance\n\n")
	if len(sources) == 0 {
		out.WriteString("No repository guidance sources supplied.\n\n")
		return
	}
	for i, source := range sources {
		fmt.Fprintf(out, "### Guidance %d: %s\n\n", i+1, source.Path)
		writeField(out, "Source ID", source.ID)
		if source.Label != "" {
			writeField(out, "Label", source.Label)
		}
		out.WriteByte('\n')
		writeSourceContent(out, source.Content)
	}
}

func writeProjectionFacts(out *bytes.Buffer, facts []DossierProjectionFact) {
	out.WriteString("## Omissions and Truncation\n\n")
	if len(facts) == 0 {
		out.WriteString("No dossier sources were omitted or truncated.\n")
		return
	}
	for _, fact := range facts {
		if fact.Reason == "history_limit" {
			fmt.Fprintf(out, "- %s: history limit retained %d of %d items and omitted %d.\n", fact.Section, fact.Included, fact.Total, fact.Omitted)
		} else if fact.Reason == "collection_limit" {
			fmt.Fprintf(out, "- %s: older selected-task items exist beyond the bounded source window.\n", fact.Section)
		} else {
			fmt.Fprintf(out, "- %s: %s.\n", fact.Section, strings.ReplaceAll(fact.Reason, "_", " "))
		}
	}
}

func writeDecisionReference(out *bytes.Buffer, reference DecisionReference) {
	writeField(out, "Decision ID", reference.DecisionID)
	writeField(out, "Run ID", reference.RunID)
	writeField(out, "Action", string(reference.Action))
	if reference.WorkerProfile == "" {
		writeField(out, "Worker profile", "none")
	} else {
		writeField(out, "Worker profile", string(reference.WorkerProfile))
	}
	writeEvidence(out, "Artifact", []EvidenceReference{reference.Artifact})
	writeField(out, "Created at", formatTime(reference.CreatedAt))
}

func writeFindingResolution(out *bytes.Buffer, resolution FindingResolution) {
	writeField(out, "Current resolution", string(resolution.Status))
	writeEvidence(out, "Resolution evidence", resolution.Evidence)
	if resolution.Rationale != "" {
		writeField(out, "Resolution rationale", resolution.Rationale)
	}
	if resolution.SupersedingFindingID != "" {
		writeField(out, "Replacement finding", resolution.SupersedingFindingID)
	}
	if resolution.Resolution == nil {
		writeField(out, "Resolution decision", "none")
	} else {
		writeField(out, "Resolution decision", fmt.Sprintf("%s (run %s, action %s, created %s)", resolution.Resolution.DecisionID, resolution.Resolution.RunID, resolution.Resolution.Action, formatTime(resolution.Resolution.CreatedAt)))
		writeEvidence(out, "Resolution decision artifact", []EvidenceReference{resolution.Resolution.Artifact})
	}
}

func writeEvidence(out *bytes.Buffer, label string, references []EvidenceReference) {
	if len(references) == 0 {
		writeField(out, label, "none")
		return
	}
	fmt.Fprintf(out, "- %s:\n", label)
	for _, reference := range references {
		fmt.Fprintf(out, "  - kind=%s; reference=%s; detail=%s\n", reference.Kind, inlineMarkdown(reference.Reference), inlineMarkdown(reference.Detail))
	}
}

func writeField(out *bytes.Buffer, label, value string) {
	fmt.Fprintf(out, "- %s: %s\n", label, inlineMarkdown(value))
}

func writeOptionalField(out *bytes.Buffer, label, value string) {
	if value == "" {
		writeField(out, label, "not supplied")
		return
	}
	writeField(out, label, value)
}

func writeSourceContent(out *bytes.Buffer, content []byte) {
	out.Write(content)
	if !bytes.HasSuffix(content, []byte("\n")) {
		out.WriteByte('\n')
	}
	out.WriteByte('\n')
}

func inlineMarkdown(value string) string {
	replacer := strings.NewReplacer("\r\n", " ↵ ", "\r", " ↵ ", "\n", " ↵ ")
	return replacer.Replace(value)
}

func acceptanceProgress(criteria []AcceptanceCriterion) string {
	counts := map[AcceptanceStatus]int{}
	for _, criterion := range criteria {
		counts[criterion.Status]++
	}
	return fmt.Sprintf("total=%d; pending=%d; satisfied=%d; waived=%d; not_applicable=%d", len(criteria), counts[AcceptanceStatusPending], counts[AcceptanceStatusSatisfied], counts[AcceptanceStatusWaived], counts[AcceptanceStatusNotApplicable])
}

func findingResolutionProgress(resolutions []FindingResolution) string {
	counts := map[FindingResolutionStatus]int{}
	for _, resolution := range resolutions {
		counts[resolution.Status]++
	}
	return fmt.Sprintf("total=%d; open=%d; resolved=%d; waived=%d; superseded=%d; invalid=%d", len(resolutions), counts[FindingResolutionStatusOpen], counts[FindingResolutionStatusResolved], counts[FindingResolutionStatusWaived], counts[FindingResolutionStatusSuperseded], counts[FindingResolutionStatusInvalid])
}

func formatCountBudget(budget CountBudget) string {
	return fmt.Sprintf("mode=%s; limit=%d; consumed=%d", budget.Mode, budget.Limit, budget.Consumed)
}

func formatDurationBudget(budget DurationBudget) string {
	return fmt.Sprintf("mode=%s; limit=%s (%d ns); consumed=%s (%d ns)", budget.Mode, budget.Limit, budget.Limit.Nanoseconds(), budget.Consumed, budget.Consumed.Nanoseconds())
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func sha256HexBytes(content []byte) string {
	sum := sha256.Sum256(content)
	return fmt.Sprintf("%x", sum)
}
