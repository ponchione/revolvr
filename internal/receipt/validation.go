package receipt

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"revolvr/internal/ledger"
)

const (
	ValidationCheckIdentity            = "identity"
	ValidationCheckCompletionTime      = "completion_time"
	ValidationCheckCommitSHA           = "commit_sha"
	ValidationCheckChangedFiles        = "changed_files"
	ValidationCheckVerificationResults = "verification_results"
	ValidationCheckArtifacts           = "artifacts"
)

type ValidationInput struct {
	WorkDir     string
	History     ledger.RunWithEvents
	ReceiptPath string
}

type ValidationResult struct {
	RunID       string
	ReceiptPath string
	Checks      []ValidationCheck
}

type ValidationCheck struct {
	Name    string
	Passed  bool
	Details []string
}

func ValidateRunReceipt(input ValidationInput) (ValidationResult, error) {
	run := input.History.Run
	runID := strings.TrimSpace(run.ID)
	if runID == "" {
		return ValidationResult{}, errors.New("validate receipt: run id is required")
	}

	workDir, err := validationWorkDir(input.WorkDir)
	if err != nil {
		return ValidationResult{}, err
	}

	artifacts, _ := ledger.RunArtifactsFromEvents(input.History.Events)
	receiptPath := strings.TrimSpace(input.ReceiptPath)
	if receiptPath == "" {
		receiptPath = strings.TrimSpace(artifacts.ReceiptPath)
	}
	if receiptPath == "" {
		receiptPath = filepath.Join(".revolvr", "receipts", runID+".md")
	}
	receiptAbsPath := resolveValidationPath(workDir, receiptPath)

	content, err := os.ReadFile(receiptAbsPath)
	if err != nil {
		return ValidationResult{}, fmt.Errorf("validate receipt: read %s: %w", receiptPath, err)
	}
	parsed, err := Parse(content)
	if err != nil {
		return ValidationResult{}, fmt.Errorf("validate receipt: parse %s: %w", receiptPath, err)
	}

	eventData := validationEventDataFromEvents(input.History.Events)
	result := ValidationResult{
		RunID:       runID,
		ReceiptPath: receiptPath,
	}
	result.add(checkReceiptIdentity(parsed, run))
	result.add(checkReceiptCompletionTime(parsed, run))
	result.add(checkReceiptCommitSHA(parsed, run, eventData))
	result.add(checkReceiptChangedFiles(parsed, eventData))
	result.add(checkReceiptVerification(parsed, run, eventData))
	result.add(checkReceiptArtifacts(workDir, receiptPath, receiptAbsPath, input.History.Events))
	return result, nil
}

func (r ValidationResult) Passed() bool {
	for _, check := range r.Checks {
		if !check.Passed {
			return false
		}
	}
	return true
}

func (r ValidationResult) Failures() []ValidationCheck {
	failures := make([]ValidationCheck, 0)
	for _, check := range r.Checks {
		if !check.Passed {
			failures = append(failures, check)
		}
	}
	return failures
}

func (c ValidationCheck) Message() string {
	if c.Passed {
		return "ok"
	}
	if len(c.Details) == 0 {
		return "failed"
	}
	return "failed - " + strings.Join(c.Details, "; ")
}

func (r *ValidationResult) add(check ValidationCheck) {
	r.Checks = append(r.Checks, check)
}

func validationWorkDir(workDir string) (string, error) {
	workDir = strings.TrimSpace(workDir)
	if workDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("validate receipt: resolve working directory: %w", err)
		}
		workDir = wd
	}
	abs, err := filepath.Abs(workDir)
	if err != nil {
		return "", fmt.Errorf("validate receipt: resolve working directory: %w", err)
	}
	return abs, nil
}

func checkReceiptIdentity(r Receipt, run ledger.Run) ValidationCheck {
	details := []string{}
	if r.RunID != run.ID {
		details = append(details, fmt.Sprintf("receipt run_id %q does not match ledger run id %q", r.RunID, run.ID))
	}
	if r.PassID != run.ID {
		details = append(details, fmt.Sprintf("receipt pass_id %q does not match ledger run id %q", r.PassID, run.ID))
	}
	if r.TaskID != run.TaskID {
		details = append(details, fmt.Sprintf("receipt task_id %q does not match ledger task_id %q", r.TaskID, run.TaskID))
	}
	if r.Task != run.Task {
		details = append(details, fmt.Sprintf("receipt task %q does not match ledger task %q", r.Task, run.Task))
	}
	return validationCheck(ValidationCheckIdentity, details)
}

func checkReceiptCompletionTime(r Receipt, run ledger.Run) ValidationCheck {
	details := []string{}
	if run.CompletedAt == nil {
		details = append(details, "ledger run has no completion time")
		return validationCheck(ValidationCheckCompletionTime, details)
	}
	expected := run.CompletedAt.UTC()
	if !r.Timestamp.UTC().Equal(expected) {
		details = append(details, fmt.Sprintf("receipt timestamp %s does not match ledger completed_at %s", validationTime(r.Timestamp), validationTime(expected)))
	}
	return validationCheck(ValidationCheckCompletionTime, details)
}

func checkReceiptCommitSHA(r Receipt, run ledger.Run, data validationEventData) ValidationCheck {
	details := []string{}
	expected := strings.TrimSpace(run.CommitSHA)
	if r.CommitSHA != expected {
		details = append(details, fmt.Sprintf("receipt commit_sha %q does not match ledger commit_sha %q", r.CommitSHA, expected))
	}
	if data.commitFound && data.commitSHA != expected {
		details = append(details, fmt.Sprintf("commit_created event commit_sha %q does not match ledger commit_sha %q", data.commitSHA, expected))
	}
	return validationCheck(ValidationCheckCommitSHA, details)
}

func checkReceiptChangedFiles(r Receipt, data validationEventData) ValidationCheck {
	details := []string{}
	if !data.changedFilesFound {
		details = append(details, "ledger changed_files_captured event is missing")
		return validationCheck(ValidationCheckChangedFiles, details)
	}
	expected := normalizeValidationStrings(data.changedFiles)
	frontmatter := normalizeValidationStrings(r.ChangedFiles)
	if !equalStringSets(frontmatter, expected) {
		details = append(details, fmt.Sprintf("frontmatter changed_files got %s, want %s", validationStringList(frontmatter), validationStringList(expected)))
	}
	body := normalizeValidationStrings(r.ChangedFileClaims)
	if !equalStringSets(body, expected) {
		details = append(details, fmt.Sprintf("body changed files got %s, want %s", validationStringList(body), validationStringList(expected)))
	}
	return validationCheck(ValidationCheckChangedFiles, details)
}

func checkReceiptVerification(r Receipt, run ledger.Run, data validationEventData) ValidationCheck {
	details := []string{}
	expectedStatus := strings.TrimSpace(run.VerificationStatus)
	if r.VerificationStatus != expectedStatus {
		details = append(details, fmt.Sprintf("receipt verification_status %q does not match ledger verification_status %q", r.VerificationStatus, expectedStatus))
	}

	expectedEntries := []VerificationEntry{}
	if data.verificationFound {
		expectedEntries = data.verification
	} else if expectedStatus != "" && expectedStatus != "not_run" {
		details = append(details, "ledger verification_completed event is missing")
	}

	details = append(details, verificationEntryMismatches("frontmatter verification", r.Verification, expectedEntries)...)
	details = append(details, verificationClaimMismatches("body verification", r.VerificationClaims, expectedEntries)...)
	return validationCheck(ValidationCheckVerificationResults, details)
}

func checkReceiptArtifacts(workDir string, receiptPath string, receiptAbsPath string, events []ledger.Event) ValidationCheck {
	details := []string{}
	artifacts, found := ledger.RunArtifactsFromEvents(events)
	if !found || artifacts.Empty() {
		details = append(details, "ledger run artifact paths are missing")
		return validationCheck(ValidationCheckArtifacts, details)
	}

	for _, artifact := range []struct {
		label string
		path  string
	}{
		{label: "context payload", path: artifacts.ContextPayloadPath},
		{label: "context manifest", path: artifacts.ContextManifestPath},
		{label: "codex stdout jsonl", path: artifacts.CodexStdoutJSONLPath},
		{label: "codex stderr", path: artifacts.CodexStderrPath},
		{label: "last message", path: artifacts.LastMessagePath},
		{label: "receipt", path: artifacts.ReceiptPath},
	} {
		path := strings.TrimSpace(artifact.path)
		if path == "" {
			details = append(details, fmt.Sprintf("%s artifact path is missing", artifact.label))
			continue
		}
		absPath := resolveValidationPath(workDir, path)
		info, err := os.Stat(absPath)
		if os.IsNotExist(err) {
			details = append(details, fmt.Sprintf("%s artifact does not exist: %s", artifact.label, path))
			continue
		}
		if err != nil {
			details = append(details, fmt.Sprintf("inspect %s artifact %s: %v", artifact.label, path, err))
			continue
		}
		if info.IsDir() {
			details = append(details, fmt.Sprintf("%s artifact is a directory: %s", artifact.label, path))
		}
	}

	if strings.TrimSpace(artifacts.ReceiptPath) != "" {
		artifactReceiptAbs := resolveValidationPath(workDir, artifacts.ReceiptPath)
		if filepath.Clean(artifactReceiptAbs) != filepath.Clean(receiptAbsPath) {
			details = append(details, fmt.Sprintf("validated receipt %s does not match ledger receipt artifact %s", receiptPath, artifacts.ReceiptPath))
		}
	}
	return validationCheck(ValidationCheckArtifacts, details)
}

func validationCheck(name string, details []string) ValidationCheck {
	cleaned := normalizeValidationStrings(details)
	return ValidationCheck{Name: name, Passed: len(cleaned) == 0, Details: cleaned}
}

type validationEventData struct {
	changedFilesFound bool
	changedFiles      []string
	verificationFound bool
	verification      []VerificationEntry
	commitFound       bool
	commitSHA         string
}

func validationEventDataFromEvents(events []ledger.Event) validationEventData {
	var data validationEventData
	for _, event := range events {
		switch event.Type {
		case ledger.EventChangedFilesCaptured:
			var payload struct {
				ChangedFiles []string `json:"changed_files"`
			}
			if decodeValidationPayload(event, &payload) {
				data.changedFilesFound = true
				data.changedFiles = normalizeValidationStrings(payload.ChangedFiles)
			}
		case ledger.EventVerificationCompleted:
			var payload struct {
				Commands []struct {
					Command  string `json:"command"`
					Status   string `json:"status"`
					Passed   bool   `json:"passed"`
					ExitCode int    `json:"exit_code"`
				} `json:"commands"`
			}
			if decodeValidationPayload(event, &payload) {
				data.verificationFound = true
				data.verification = verificationEntriesFromEvent(payload.Commands)
			}
		case ledger.EventCommitCreated:
			var payload struct {
				CommitSHA string `json:"commit_sha"`
			}
			if decodeValidationPayload(event, &payload) {
				data.commitFound = true
				data.commitSHA = strings.TrimSpace(payload.CommitSHA)
			}
		}
	}
	return data
}

func verificationEntriesFromEvent(commands []struct {
	Command  string `json:"command"`
	Status   string `json:"status"`
	Passed   bool   `json:"passed"`
	ExitCode int    `json:"exit_code"`
}) []VerificationEntry {
	entries := make([]VerificationEntry, 0, len(commands))
	for _, command := range commands {
		name := strings.TrimSpace(command.Command)
		if name == "" {
			continue
		}
		status := strings.TrimSpace(command.Status)
		if status == "" && command.Passed {
			status = "passed"
		}
		entries = append(entries, VerificationEntry{
			Command:  name,
			ExitCode: command.ExitCode,
			Status:   status,
		})
	}
	return entries
}

func verificationEntryMismatches(label string, got []VerificationEntry, want []VerificationEntry) []string {
	got = normalizeVerificationEntries(got)
	want = normalizeVerificationEntries(want)
	if len(got) != len(want) {
		return []string{fmt.Sprintf("%s got %s, want %s", label, validationVerificationList(got), validationVerificationList(want))}
	}
	details := []string{}
	for i := range want {
		if got[i] != want[i] {
			details = append(details, fmt.Sprintf("%s[%d] got %s, want %s", label, i, validationVerificationEntry(got[i]), validationVerificationEntry(want[i])))
		}
	}
	return details
}

func verificationClaimMismatches(label string, got []VerificationClaim, want []VerificationEntry) []string {
	want = normalizeVerificationEntries(want)
	if len(got) != len(want) {
		return []string{fmt.Sprintf("%s got %s, want %s", label, validationVerificationClaims(got), validationVerificationList(want))}
	}
	details := []string{}
	for i := range want {
		claim := got[i]
		expected := want[i]
		if strings.TrimSpace(claim.Command) != expected.Command {
			details = append(details, fmt.Sprintf("%s[%d] command got %q, want %q", label, i, strings.TrimSpace(claim.Command), expected.Command))
		}
		if strings.TrimSpace(claim.Status) != expected.Status {
			details = append(details, fmt.Sprintf("%s[%d] status got %q, want %q", label, i, strings.TrimSpace(claim.Status), expected.Status))
		}
		if !claim.HasExitCode {
			details = append(details, fmt.Sprintf("%s[%d] exit_code is missing, want %d", label, i, expected.ExitCode))
			continue
		}
		if claim.ExitCode != expected.ExitCode {
			details = append(details, fmt.Sprintf("%s[%d] exit_code got %d, want %d", label, i, claim.ExitCode, expected.ExitCode))
		}
	}
	return details
}

func normalizeVerificationEntries(entries []VerificationEntry) []VerificationEntry {
	out := make([]VerificationEntry, 0, len(entries))
	for _, entry := range entries {
		entry.Command = strings.TrimSpace(entry.Command)
		entry.Status = strings.TrimSpace(entry.Status)
		if entry.Command == "" {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func decodeValidationPayload(event ledger.Event, target any) bool {
	if len(event.Payload) == 0 {
		return false
	}
	return json.Unmarshal(event.Payload, target) == nil
}

func resolveValidationPath(workDir string, path string) string {
	path = strings.TrimSpace(path)
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(workDir, path)
}

func equalStringSets(got []string, want []string) bool {
	got = normalizeValidationStrings(got)
	want = normalizeValidationStrings(want)
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func normalizeValidationStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func validationStringList(values []string) string {
	values = normalizeValidationStrings(values)
	if len(values) == 0 {
		return "[]"
	}
	return "[" + strings.Join(values, ", ") + "]"
}

func validationVerificationList(entries []VerificationEntry) string {
	entries = normalizeVerificationEntries(entries)
	if len(entries) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(entries))
	for _, entry := range entries {
		parts = append(parts, validationVerificationEntry(entry))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func validationVerificationEntry(entry VerificationEntry) string {
	return fmt.Sprintf("%s (%s, exit %d)", entry.Command, entry.Status, entry.ExitCode)
}

func validationVerificationClaims(claims []VerificationClaim) string {
	if len(claims) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(claims))
	for _, claim := range claims {
		status := strings.TrimSpace(claim.Status)
		exit := "missing"
		if claim.HasExitCode {
			exit = fmt.Sprintf("%d", claim.ExitCode)
		}
		parts = append(parts, fmt.Sprintf("%s (%s, exit %s)", strings.TrimSpace(claim.Command), status, exit))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func validationTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}
