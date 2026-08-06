// Package correction owns one bounded finding/failure-scoped correction,
// followed by mandatory fresh final verification and independent re-audit.
package correction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"
	"time"

	"revolvr/internal/audit"
	"revolvr/internal/evidence"
	"revolvr/internal/model"
	"revolvr/internal/sandbox"
	"revolvr/internal/tool"
	"revolvr/internal/verification"
)

const (
	DossierSchemaVersion  = "revolvr-corrector-dossier-v1"
	FailureSchemaVersion  = "revolvr-failure-signature-v1"
	StrategySchemaVersion = "revolvr-correction-strategy-v1"
	OutcomeSchemaVersion  = "revolvr-strategy-outcome-v1"
	MaximumDossierBytes   = 4 << 20
)

var (
	ErrInvalidAuthority = errors.New("invalid correction authority")
	ErrRepeatedStrategy = errors.New("materially repeated failed strategy")
	ErrNoProgress       = errors.New("correction made no material progress")
	ErrBudget           = errors.New("correction budget exhausted")
)

type AuthorityKind string

const (
	AuthorityVerification AuthorityKind = "verification_failure"
	AuthorityFindings     AuthorityKind = "audit_findings"
)

type VerificationFailure struct {
	VerificationRunID string               `json:"verification_run_id"`
	CheckID           string               `json:"check_id"`
	GateID            string               `json:"gate_id"`
	Outcome           verification.Outcome `json:"outcome"`
	ExitCode          int                  `json:"exit_code"`
	FailedTestIDs     []string             `json:"failed_test_ids"`
	StableExcerpts    []string             `json:"stable_error_excerpts"`
	Component         string               `json:"affected_component"`
	AffectedFiles     []string             `json:"affected_files"`
}

type Authority struct {
	Kind         AuthorityKind        `json:"kind"`
	Source       audit.Source         `json:"source"`
	Verification *VerificationFailure `json:"verification_failure,omitempty"`
	AuditRunID   string               `json:"audit_run_id,omitempty"`
	Findings     []audit.Finding      `json:"findings"`
}

type FailureSignature struct {
	SchemaVersion       string          `json:"schema_version"`
	AuthorityKind       AuthorityKind   `json:"authority_kind"`
	Source              audit.Source    `json:"source"`
	VerificationRunID   string          `json:"verification_run_id,omitempty"`
	VerificationCheckID string          `json:"verification_check_id,omitempty"`
	AuditRunID          string          `json:"audit_run_id,omitempty"`
	FindingIDs          []string        `json:"finding_ids"`
	NormalizedMaterial  json.RawMessage `json:"normalized_material"`
	SHA256              string          `json:"sha256"`
}

type failureMaterial struct {
	Kind               AuthorityKind        `json:"kind"`
	Source             audit.Source         `json:"source"`
	GateID             string               `json:"gate_id,omitempty"`
	Outcome            verification.Outcome `json:"outcome,omitempty"`
	ExitCode           int                  `json:"exit_code,omitempty"`
	FailedTestIDs      []string             `json:"failed_test_ids"`
	StableExcerpts     []string             `json:"stable_error_excerpts"`
	Component          string               `json:"affected_component,omitempty"`
	FindingIDs         []string             `json:"finding_ids"`
	FindingDefinitions []string             `json:"finding_definitions"`
}

type Strategy struct {
	SchemaVersion    string   `json:"schema_version"`
	Approach         string   `json:"approach"`
	Techniques       []string `json:"techniques"`
	TargetFiles      []string `json:"target_files"`
	TargetSymbols    []string `json:"target_symbols"`
	ExpectedEvidence []string `json:"expected_evidence"`
}

type StrategyRecord struct {
	ID              string       `json:"id"`
	FailureSHA256   string       `json:"failure_sha256"`
	Fingerprint     string       `json:"fingerprint"`
	Outcome         string       `json:"outcome"`
	DiffSHA256      string       `json:"diff_sha256,omitempty"`
	ResultingSource audit.Source `json:"resulting_source"`
}

type RelevantTest struct {
	ID    string   `json:"id"`
	Argv  []string `json:"argv"`
	Paths []string `json:"paths"`
}

type DossierInput struct {
	Identity        audit.Identity     `json:"identity"`
	Authority       Authority          `json:"authority"`
	CurrentSource   audit.Source       `json:"current_source"`
	RelevantSource  []audit.SourceFile `json:"relevant_source"`
	RelevantTests   []RelevantTest     `json:"relevant_tests"`
	PriorStrategies []StrategyRecord   `json:"prior_strategies"`
}

type Dossier struct {
	SchemaVersion string                  `json:"schema_version"`
	Input         DossierInput            `json:"input"`
	Omissions     []audit.DossierOmission `json:"omissions"`
}

type DossierArtifact struct {
	SchemaVersion string          `json:"schema_version"`
	SHA256        string          `json:"sha256"`
	ByteSize      int             `json:"byte_size"`
	Content       json.RawMessage `json:"content"`
}

type Budget struct {
	MaximumCycles    int `json:"maximum_cycles"`
	ConsumedCycles   int `json:"consumed_cycles"`
	MaximumAttempts  int `json:"maximum_attempts"`
	ConsumedAttempts int `json:"consumed_attempts"`
}

type WorkerRequest struct {
	TaskID    string
	Dossier   DossierArtifact
	Authority Authority
	Strategy  Strategy
	Sandbox   sandbox.Specification
	Registry  tool.Registry
}

type WorkerOutcome string

const (
	WorkerSucceeded WorkerOutcome = "succeeded"
	WorkerFailed    WorkerOutcome = "failed"
	WorkerCancelled WorkerOutcome = "cancelled"
)

type WorkerResult struct {
	InvocationID       string
	Outcome            WorkerOutcome
	Strategy           Strategy
	Source             audit.Source
	ChangedFiles       []string
	ChangedSymbols     []string
	ResolvedFindingIDs []string
	Evidence           []audit.DispositionEvidence
	Error              string
}

type Worker interface {
	Run(context.Context, WorkerRequest) (WorkerResult, error)
}

type VerificationRequest struct {
	TaskID  string
	Source  audit.Source
	Purpose verification.Purpose
}

type Verifier interface {
	Verify(context.Context, VerificationRequest) (audit.VerificationEvidence, error)
}

type AuditRequest struct {
	TaskID                string
	Source                audit.Source
	Verification          audit.VerificationEvidence
	PreviousFindingIDs    []string
	CorrectorInvocationID string
}

type AuditResult struct {
	AuditID             string
	AuditorInvocationID string
	Disposition         audit.Disposition
	Source              audit.Source
	VerificationRunID   string
	Findings            []audit.Finding
	CompletedAt         time.Time
}

type Auditor interface {
	Audit(context.Context, AuditRequest) (AuditResult, error)
}

type AttemptRecord struct {
	OperationID                string
	StrategyID                 string
	Failure                    FailureSignature
	Strategy                   Strategy
	StrategyFingerprint        string
	DossierSHA256              string
	CorrectorInvocationID      string
	SandboxSpecificationSHA256 string
	StartedAt                  time.Time
}

type OutcomeRecord struct {
	ID                string
	StrategyID        string
	Outcome           string
	ResultingSource   audit.Source
	VerificationRunID string
	AuditRunID        string
	Evidence          []audit.DispositionEvidence
	CompletedAt       time.Time
}

type Store interface {
	HasFailedStrategy(context.Context, string, string, string) (bool, error)
	Begin(context.Context, audit.Identity, AttemptRecord) error
	Complete(context.Context, audit.Identity, OutcomeRecord) error
}

type Dispositioner interface {
	DispositionMany(context.Context, []audit.DispositionCommand) ([]audit.DispositionResult, error)
}

type Outcome string

const (
	OutcomeCorrectedClean     Outcome = "corrected_clean"
	OutcomeChangesRequired    Outcome = "changes_required"
	OutcomeBlocked            Outcome = "blocked"
	OutcomeRepeatedStrategy   Outcome = "repeated_strategy"
	OutcomeIdenticalDiff      Outcome = "identical_diff"
	OutcomeRepeatedFailure    Outcome = "repeated_failure"
	OutcomeNoChanges          Outcome = "no_changes"
	OutcomeNoEvidence         Outcome = "no_evidence"
	OutcomeBudgetExhausted    Outcome = "budget_exhausted"
	OutcomeCancelled          Outcome = "cancelled"
	OutcomeCorrectionFailed   Outcome = "correction_failed"
	OutcomeVerificationFailed Outcome = "verification_failed"
	OutcomeAuditFailed        Outcome = "audit_failed"
)

type Result struct {
	Outcome      Outcome
	Failure      FailureSignature
	Strategy     Strategy
	Fingerprint  string
	Worker       WorkerResult
	Verification audit.VerificationEvidence
	Audit        AuditResult
	Dispositions []audit.DispositionResult
	Reason       string
}

func BuildDossier(input DossierInput) (DossierArtifact, error) {
	if err := validateDossier(input); err != nil {
		return DossierArtifact{}, fmt.Errorf("build corrector dossier: %w", err)
	}
	input = normalizeDossier(input)
	dossier := Dossier{
		SchemaVersion: DossierSchemaVersion, Input: input,
		Omissions: []audit.DossierOmission{
			{Section: "unrelated_findings", Reason: "only exact active correction authority is included"},
			{Section: "unrelated_source", Reason: "source context is bounded to cited scope"},
			{Section: "lifecycle_authority", Reason: "corrector cannot disposition findings or complete tasks"},
		},
	}
	raw, err := json.Marshal(dossier)
	if err != nil {
		return DossierArtifact{}, err
	}
	if len(raw) > MaximumDossierBytes {
		return DossierArtifact{}, fmt.Errorf("corrector dossier exceeds %d bytes", MaximumDossierBytes)
	}
	return DossierArtifact{SchemaVersion: DossierSchemaVersion, SHA256: model.SHA256(raw), ByteSize: len(raw), Content: raw}, nil
}

func NormalizeFailure(authority Authority) (FailureSignature, error) {
	if err := validateAuthority(authority); err != nil {
		return FailureSignature{}, err
	}
	value := failureMaterial{Kind: authority.Kind, Source: authority.Source, FailedTestIDs: []string{}, StableExcerpts: []string{}, FindingIDs: []string{}, FindingDefinitions: []string{}}
	result := FailureSignature{SchemaVersion: FailureSchemaVersion, AuthorityKind: authority.Kind, Source: authority.Source, FindingIDs: []string{}}
	if authority.Kind == AuthorityVerification {
		failure := authority.Verification
		value.GateID, value.Outcome, value.ExitCode, value.Component = failure.GateID, failure.Outcome, failure.ExitCode, normalizeMeaning(failure.Component)
		value.FailedTestIDs = normalizeList(failure.FailedTestIDs)
		value.StableExcerpts = normalizeList(failure.StableExcerpts)
		result.VerificationRunID, result.VerificationCheckID = failure.VerificationRunID, failure.CheckID
	} else {
		result.AuditRunID = authority.AuditRunID
		for _, finding := range authority.Findings {
			value.FindingIDs = append(value.FindingIDs, finding.ID)
			value.FindingDefinitions = append(value.FindingDefinitions, audit.FindingDefinitionSHA256(finding))
		}
		sort.Strings(value.FindingIDs)
		sort.Strings(value.FindingDefinitions)
		result.FindingIDs = append([]string(nil), value.FindingIDs...)
	}
	raw, _ := json.Marshal(value)
	result.NormalizedMaterial = raw
	result.SHA256 = model.SHA256(raw)
	return result, nil
}

func StrategyFingerprint(strategy Strategy) (string, Strategy, error) {
	if strategy.SchemaVersion != StrategySchemaVersion || strings.TrimSpace(strategy.Approach) == "" || len(strategy.Approach) > 65536 || len(strategy.Techniques) == 0 || len(strategy.TargetFiles) == 0 || len(strategy.ExpectedEvidence) == 0 {
		return "", Strategy{}, errors.New("correction strategy is incomplete")
	}
	normalized := Strategy{
		SchemaVersion: StrategySchemaVersion,
		Approach:      normalizeMeaning(strategy.Approach),
		Techniques:    normalizeList(strategy.Techniques), TargetFiles: append([]string(nil), strategy.TargetFiles...),
		TargetSymbols: normalizeList(strategy.TargetSymbols), ExpectedEvidence: normalizeList(strategy.ExpectedEvidence),
	}
	for i, file := range normalized.TargetFiles {
		if !cleanRelative(file) {
			return "", Strategy{}, fmt.Errorf("strategy target path %q is unsafe", file)
		}
		normalized.TargetFiles[i] = file
	}
	sort.Strings(normalized.TargetFiles)
	for _, values := range [][]string{normalized.Techniques, normalized.TargetSymbols, normalized.ExpectedEvidence} {
		if slices.Contains(values, "") {
			return "", Strategy{}, errors.New("correction strategy contains empty semantic material")
		}
	}
	if duplicate(normalized.TargetFiles) || duplicate(normalized.Techniques) || duplicate(normalized.TargetSymbols) || duplicate(normalized.ExpectedEvidence) {
		return "", Strategy{}, errors.New("correction strategy contains duplicate semantic material")
	}
	raw, _ := json.Marshal(normalized)
	return model.SHA256(raw), normalized, nil
}

func validateDossier(input DossierInput) error {
	for _, value := range []string{input.Identity.ProjectID, input.Identity.TaskID, input.Identity.TaskVersionID, input.Identity.RunID, input.Identity.WorkspaceID} {
		if !correctionToken(value) {
			return errors.New("corrector dossier identity is malformed")
		}
	}
	if input.CurrentSource != input.Authority.Source || len(input.RelevantSource) == 0 || len(input.RelevantTests) == 0 {
		return errors.New("identity, current source, bounded source, or relevant tests are missing")
	}
	if err := validateAuthority(input.Authority); err != nil {
		return err
	}
	allowed := allowedFiles(input.Authority)
	seenSource := map[string]struct{}{}
	for _, file := range input.RelevantSource {
		if !slices.Contains(allowed, file.Path) || !cleanRelative(file.Path) || !correctionToken(file.ArtifactID) || !validCorrectionSHA(file.SHA256) || file.SizeBytes != int64(len(file.Content)) || file.SHA256 != model.SHA256([]byte(file.Content)) {
			return fmt.Errorf("relevant source %q is outside exact correction authority", file.Path)
		}
		if _, duplicate := seenSource[file.Path]; duplicate {
			return fmt.Errorf("relevant source %q is duplicated", file.Path)
		}
		seenSource[file.Path] = struct{}{}
	}
	seenTests := map[string]struct{}{}
	for _, test := range input.RelevantTests {
		if !correctionToken(test.ID) || len(test.Argv) == 0 {
			return errors.New("relevant test evidence is malformed")
		}
		if _, duplicate := seenTests[test.ID]; duplicate {
			return errors.New("relevant test evidence contains a duplicate")
		}
		seenTests[test.ID] = struct{}{}
		for _, argument := range test.Argv {
			if strings.TrimSpace(argument) == "" || strings.ContainsRune(argument, 0) {
				return errors.New("relevant test command is malformed")
			}
		}
		for _, testPath := range test.Paths {
			if !cleanRelative(testPath) {
				return errors.New("relevant test path is unsafe")
			}
		}
	}
	seenStrategies := map[string]struct{}{}
	for _, strategy := range input.PriorStrategies {
		if !correctionToken(strategy.ID) || !validCorrectionSHA(strategy.FailureSHA256) || !validCorrectionSHA(strategy.Fingerprint) || !correctionToken(strategy.Outcome) || strategy.DiffSHA256 != "" && !validCorrectionSHA(strategy.DiffSHA256) {
			return errors.New("prior strategy evidence is malformed")
		}
		if _, duplicate := seenStrategies[strategy.ID]; duplicate {
			return errors.New("prior strategy evidence contains a duplicate")
		}
		seenStrategies[strategy.ID] = struct{}{}
	}
	return nil
}

func validateAuthority(authority Authority) error {
	if !validCorrectionSHA(authority.Source.Revision) || !validCorrectionGitOID(authority.Source.Commit) || !validCorrectionGitOID(authority.Source.Tree) || !validCorrectionSHA(authority.Source.DiffSHA256) {
		return ErrInvalidAuthority
	}
	switch authority.Kind {
	case AuthorityVerification:
		if authority.Verification == nil || authority.AuditRunID != "" || len(authority.Findings) != 0 || !correctionToken(authority.Verification.VerificationRunID) || !correctionToken(authority.Verification.CheckID) || !correctionToken(authority.Verification.GateID) || !authority.Verification.Outcome.Failed() || len(authority.Verification.AffectedFiles) == 0 {
			return ErrInvalidAuthority
		}
		for _, file := range authority.Verification.AffectedFiles {
			if !cleanRelative(file) {
				return ErrInvalidAuthority
			}
		}
	case AuthorityFindings:
		if authority.Verification != nil || !correctionToken(authority.AuditRunID) || len(authority.Findings) == 0 {
			return ErrInvalidAuthority
		}
		seen := map[string]struct{}{}
		for _, finding := range authority.Findings {
			if finding.ID == "" || len(finding.AffectedFiles) == 0 {
				return ErrInvalidAuthority
			}
			if _, exists := seen[finding.ID]; exists {
				return ErrInvalidAuthority
			}
			seen[finding.ID] = struct{}{}
		}
	default:
		return ErrInvalidAuthority
	}
	return nil
}

func allowedFiles(authority Authority) []string {
	set := map[string]struct{}{}
	if authority.Verification != nil {
		for _, file := range authority.Verification.AffectedFiles {
			set[file] = struct{}{}
		}
	}
	for _, finding := range authority.Findings {
		for _, file := range finding.AffectedFiles {
			set[file] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for file := range set {
		result = append(result, file)
	}
	sort.Strings(result)
	return result
}

func normalizeDossier(input DossierInput) DossierInput {
	input.RelevantSource = append([]audit.SourceFile(nil), input.RelevantSource...)
	input.RelevantTests = append([]RelevantTest(nil), input.RelevantTests...)
	input.PriorStrategies = append([]StrategyRecord(nil), input.PriorStrategies...)
	sort.Slice(input.RelevantSource, func(i, j int) bool { return input.RelevantSource[i].Path < input.RelevantSource[j].Path })
	sort.Slice(input.RelevantTests, func(i, j int) bool { return input.RelevantTests[i].ID < input.RelevantTests[j].ID })
	sort.Slice(input.PriorStrategies, func(i, j int) bool { return input.PriorStrategies[i].ID < input.PriorStrategies[j].ID })
	return input
}

func normalizeMeaning(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}
func normalizeList(values []string) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = normalizeMeaning(v)
	}
	sort.Strings(out)
	return out
}
func duplicate(values []string) bool {
	for i := 1; i < len(values); i++ {
		if values[i] == values[i-1] {
			return true
		}
	}
	return false
}
func normalizedText(value string) bool {
	return value == strings.TrimSpace(value) && value != "" && len(value) <= 65536
}
func cleanRelative(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && path.Clean(value) == value && value != "." && value != ".." && !strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "../") && !strings.Contains(value, "\\")
}

func outcomeRecordHash(value OutcomeRecord) string { hash, _ := evidence.Hash(value); return hash }
