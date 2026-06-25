package receipt

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseValidReceipt(t *testing.T) {
	parsed, err := Parse([]byte(validReceiptContent()))
	if err != nil {
		t.Fatalf("parse receipt: %v", err)
	}

	if got, want := parsed.SchemaVersion, SchemaVersion; got != want {
		t.Fatalf("schema version = %q, want %q", got, want)
	}
	if got, want := parsed.RunID, "run-1"; got != want {
		t.Fatalf("run id = %q, want %q", got, want)
	}
	if got, want := parsed.Verdict, VerdictCompleted; got != want {
		t.Fatalf("verdict = %q, want %q", got, want)
	}
	if got, want := parsed.Metrics.InputTokens, 11; got != want {
		t.Fatalf("input tokens = %d, want %d", got, want)
	}
	if got, want := parsed.ChangedFileClaims, []string{"internal/receipt/types.go", "internal/receipt/parser.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("changed file claims = %#v, want %#v", got, want)
	}
	if got, want := ParseVerificationCommands(parsed.RawBody), []string{"go test ./..."}; !reflect.DeepEqual(got, want) {
		t.Fatalf("verification commands = %#v, want %#v", got, want)
	}
}

func TestParseRejectsMissingRequiredField(t *testing.T) {
	content := strings.Replace(validReceiptContent(), "run_id: run-1\n", "", 1)

	_, err := Parse([]byte(content))
	if !errors.Is(err, ErrMissingField) {
		t.Fatalf("error = %v, want ErrMissingField", err)
	}
}

func TestParseRejectsInvalidVerdict(t *testing.T) {
	content := strings.Replace(validReceiptContent(), "verdict: completed\n", "verdict: done\n", 1)

	_, err := Parse([]byte(content))
	if !errors.Is(err, ErrInvalidVerdict) {
		t.Fatalf("error = %v, want ErrInvalidVerdict", err)
	}
}

func TestParseRejectsMissingRequiredSection(t *testing.T) {
	content := strings.Replace(validReceiptContent(), "\n## Concerns\nNone.\n", "", 1)

	_, err := Parse([]byte(content))
	if !errors.Is(err, ErrMissingSection) {
		t.Fatalf("error = %v, want ErrMissingSection", err)
	}
}

func TestParseChangedFiles(t *testing.T) {
	body := `## Changed Files
- ` + "`internal/receipt/types.go`" + `
- M internal/receipt/parser.go - updated parser
- internal/receipt/old.go -> internal/receipt/new.go
- None

## Verification
`

	got := ParseChangedFiles(body)
	want := []string{
		"internal/receipt/types.go",
		"internal/receipt/parser.go",
		"internal/receipt/old.go",
		"internal/receipt/new.go",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("changed files = %#v, want %#v", got, want)
	}
}

func TestParseVerificationClaims(t *testing.T) {
	body := `## Verification
- ` + "`go test ./...`" + ` (passed, exit 0)
- go vet ./... - failed, exit 1

## Concerns
`

	got := ParseVerificationClaims(body)
	want := []VerificationClaim{
		{Command: "go test ./...", ExitCode: 0, HasExitCode: true, Status: "passed"},
		{Command: "go vet ./...", ExitCode: 1, HasExitCode: true, Status: "failed"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("verification claims = %#v, want %#v", got, want)
	}
}

func TestFormatFallbackReceipt(t *testing.T) {
	timestamp := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	content, parsed := FormatFallbackReceipt(FallbackInput{
		RunID:              "run-2",
		PassID:             "pass-2",
		TaskID:             "task-2",
		Task:               "Fallback task",
		Verdict:            VerdictCodexFailed,
		Timestamp:          timestamp,
		CodexExitCode:      7,
		VerificationStatus: "not_run",
		ChangedFiles:       []string{"internal/receipt/fallback.go"},
		Verification: []VerificationEntry{
			{Command: "go test ./...", ExitCode: 0, Status: "passed"},
		},
		Metrics:   Metrics{InputTokens: 3, OutputTokens: 4, DurationSeconds: 5},
		FinalText: "Codex exited before writing a receipt.",
	})

	reparsed, err := Parse([]byte(content))
	if err != nil {
		t.Fatalf("fallback did not parse: %v\n%s", err, content)
	}
	if got, want := parsed.Verdict, VerdictCodexFailed; got != want {
		t.Fatalf("parsed fallback verdict = %q, want %q", got, want)
	}
	if got, want := reparsed.ChangedFileClaims, []string{"internal/receipt/fallback.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("fallback changed file claims = %#v, want %#v", got, want)
	}
	if got, want := ParseVerificationCommands(reparsed.RawBody), []string{"go test ./..."}; !reflect.DeepEqual(got, want) {
		t.Fatalf("fallback verification commands = %#v, want %#v", got, want)
	}
}

func TestRewriteMetricsFromCodexJSONL(t *testing.T) {
	jsonl := []byte(strings.Join([]string{
		`{"type":"turn.completed","usage":{"input_tokens":10,"output_tokens":4},"duration_ms":1500}`,
		`{"type":"turn.completed","usage":{"prompt_tokens":5,"completion_tokens":6,"duration_seconds":2}}`,
	}, "\n"))

	updated, parsed, changed, err := RewriteMetricsFromCodexJSONL([]byte(validReceiptContent()), jsonl)
	if err != nil {
		t.Fatalf("rewrite metrics: %v", err)
	}
	if !changed {
		t.Fatal("changed = false, want true")
	}
	if got, want := parsed.Metrics, (Metrics{InputTokens: 15, OutputTokens: 10, DurationSeconds: 4}); got != want {
		t.Fatalf("metrics = %#v, want %#v", got, want)
	}
	if !strings.Contains(string(updated), "input_tokens: 15") {
		t.Fatalf("updated receipt does not contain rewritten input tokens:\n%s", string(updated))
	}
	if _, err := Parse(updated); err != nil {
		t.Fatalf("updated receipt is invalid: %v", err)
	}
}

func validReceiptContent() string {
	return `---
schema_version: revolvr.receipt.v1
run_id: run-1
pass_id: pass-1
task_id: task-1
task: Add receipt package
verdict: completed
timestamp: 2026-06-25T12:00:00Z
codex_exit_code: 0
verification_status: passed
commit_sha: ""
changed_files:
  - internal/receipt/types.go
verification:
  - command: go test ./...
    exit_code: 0
    status: passed
metrics:
  input_tokens: 11
  output_tokens: 7
  duration_seconds: 3
---
## Summary
Implemented the receipt package.

## Changed Files
- ` + "`internal/receipt/types.go`" + `
- internal/receipt/parser.go - updated parser

## Verification
- ` + "`go test ./...`" + ` (passed, exit 0)

## Concerns
None.

## Next Steps
None.
`
}
