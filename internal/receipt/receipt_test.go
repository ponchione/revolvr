package receipt

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"revolvr/internal/ledger"
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
- ` + "`.agent/STATE.md`" + `
- M internal/receipt/parser.go - updated parser
- internal/receipt/old.go -> internal/receipt/new.go
- None

## Verification
`

	got := ParseChangedFiles(body)
	want := []string{
		"internal/receipt/types.go",
		".agent/STATE.md",
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

func TestCheckReceiptVerdictUsesTerminalOutcomeAndFinalizedVerdict(t *testing.T) {
	tests := []struct {
		name string
		run  ledger.Run
		data validationEventData
		got  Verdict
		pass bool
	}{
		{
			name: "completed run fallback",
			run:  ledger.Run{Status: ledger.StatusCompleted},
			got:  VerdictCompleted,
			pass: true,
		},
		{
			name: "failed outcome mismatch",
			run:  ledger.Run{Status: ledger.StatusFailed},
			data: validationEventData{outcome: "verification_failed"},
			got:  VerdictCompleted,
		},
		{
			name: "explicit finalized verdict",
			run:  ledger.Run{Status: ledger.StatusFailed},
			data: validationEventData{outcome: "commit_failed", receiptVerdict: VerdictBlocked},
			got:  VerdictBlocked,
			pass: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check := checkReceiptVerdict(Receipt{Verdict: tt.got}, tt.run, tt.data)
			if check.Passed != tt.pass {
				t.Fatalf("verdict check = %+v, want passed=%v", check, tt.pass)
			}
		})
	}
}

func TestRewriteHarnessFieldsRefreshesHarnessOwnedBodySections(t *testing.T) {
	timestamp := time.Date(2026, 7, 7, 17, 58, 10, 0, time.UTC)
	updated, parsed, changed, err := RewriteHarnessFields([]byte(validReceiptContent()), HarnessFields{
		Timestamp:          timestamp,
		Verdict:            VerdictCompleted,
		CodexExitCode:      0,
		VerificationStatus: "passed",
		CommitSHA:          "abc123",
		ChangedFiles:       []string{".agent/STATE.md", "README.md"},
		Verification: []VerificationEntry{{
			Command:  "go test ./...",
			ExitCode: 0,
			Status:   "passed",
		}},
		Metrics: Metrics{InputTokens: 22, OutputTokens: 9, DurationSeconds: 4},
	})
	if err != nil {
		t.Fatalf("rewrite harness fields: %v", err)
	}
	if !changed {
		t.Fatal("changed = false, want true")
	}
	body := string(updated)
	for _, want := range []string{
		"- `.agent/STATE.md`",
		"- `README.md`",
		"- `go test ./...` (passed, exit 0)",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("updated receipt missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "internal/receipt/parser.go") {
		t.Fatalf("updated receipt kept stale changed file claim:\n%s", body)
	}
	if got, want := parsed.ChangedFileClaims, []string{".agent/STATE.md", "README.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("changed file claims = %#v, want %#v", got, want)
	}
	if !parsed.Timestamp.Equal(timestamp) {
		t.Fatalf("timestamp = %s, want %s", parsed.Timestamp, timestamp)
	}
	if got, want := ParseVerificationCommands(parsed.RawBody), []string{"go test ./..."}; !reflect.DeepEqual(got, want) {
		t.Fatalf("verification commands = %#v, want %#v", got, want)
	}
}

func TestRewriteHarnessFieldsPreservesFlakyClassificationAttempts(t *testing.T) {
	entries := []VerificationEntry{{Command: "go test ./...", ExitCode: 1, Status: "failed"}, {Command: "go test ./...", ExitCode: 0, Status: "passed"}}
	updated, parsed, _, err := RewriteHarnessFields([]byte(validReceiptContent()), HarnessFields{Verdict: VerdictVerificationFailed, VerificationStatus: "failed", Verification: entries})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(parsed.Verification, entries) || parsed.VerificationStatus != "failed" || parsed.Verdict != VerdictVerificationFailed {
		t.Fatalf("receipt=%+v", parsed)
	}
	if got := strings.Count(string(updated), "`go test ./...`"); got != 2 {
		t.Fatalf("rendered attempt count=%d\n%s", got, updated)
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

func TestParseCodexUsageMetricsRecoversAfterMalformedJSONLines(t *testing.T) {
	jsonl := []byte(strings.Join([]string{
		`{"type":"item.completed","item":{"type":"command_execution","aggregated_output":"README.md`,
		`internal/runonce/runonce.go"}}`,
		`{"type":"turn.completed","usage":{"input_tokens":294455,"cached_input_tokens":239360,"output_tokens":5151,"reasoning_output_tokens":2777}}`,
	}, "\n"))

	metrics, found, err := ParseCodexUsageMetrics(jsonl)
	if !errors.Is(err, ErrMalformedCodexJSONL) {
		t.Fatalf("parse codex usage metrics error = %v, want malformed-record diagnostic", err)
	}
	if !found {
		t.Fatal("found = false, want true")
	}
	if got, want := metrics, (Metrics{InputTokens: 294455, OutputTokens: 5151, DurationSeconds: 0}); got != want {
		t.Fatalf("metrics = %#v, want %#v", got, want)
	}
	var malformed *MalformedCodexJSONLError
	if !errors.As(err, &malformed) || malformed.FirstRecord != 1 || malformed.Count != 2 {
		t.Fatalf("malformed diagnostic = %#v", err)
	}
}

func TestParseCodexUsageMetricsReportsMalformedJSONWhenNoUsageFound(t *testing.T) {
	_, found, err := ParseCodexUsageMetrics([]byte("not-json\n"))
	if err == nil {
		t.Fatal("error = nil, want malformed JSON error")
	}
	if found {
		t.Fatal("found = true, want false")
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
