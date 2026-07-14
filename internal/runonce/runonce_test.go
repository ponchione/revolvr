package runonce

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/codexexec"
	"revolvr/internal/commit"
	"revolvr/internal/gitstate"
	"revolvr/internal/ledger"
	"revolvr/internal/lock"
	"revolvr/internal/prompt"
	"revolvr/internal/receipt"
	"revolvr/internal/runner"
	"revolvr/internal/taskfile"
	"revolvr/internal/taskmodel"
	"revolvr/internal/taskscheduler"
	"revolvr/internal/verification"
)

func TestRunCommitsVerifiedCodexChanges(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	selected := writeRunTask(t, env, "task-1", "Implement selected task")

	state := &fakeCommandState{
		t:                 t,
		workDir:           env.workDir,
		writeReceipt:      true,
		postStatus:        " M internal/feature.go\x00",
		verificationExit:  0,
		commitSHA:         "abc123def456",
		expectedCommitAdd: []string{"--literal-pathspecs", "add", "--", "internal/feature.go"},
	}

	result, err := Run(ctx, Config{
		WorkingDir:           env.workDir,
		LedgerStore:          env.ledger,
		CodexExecutable:      "codex-test",
		GitExecutable:        "git-test",
		VerificationCommands: []verification.Command{{Name: "go", Args: []string{"test", "./..."}}},
		CommandRunner:        state.run,
		Clock:                env.clock,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}

	if result.Outcome != OutcomeCommitted {
		t.Fatalf("outcome = %s, want committed; message=%s", result.Outcome, result.Message)
	}
	if result.Commit.CommitSHA != "abc123def456" {
		t.Fatalf("commit sha = %q, want abc123def456", result.Commit.CommitSHA)
	}
	if result.Task.Status != taskmodel.StatusPending {
		t.Fatalf("task status = %q, want pending", result.Task.Status)
	}
	updatedTask := loadRunTask(t, env, selected.SourcePath)
	if got := updatedTask.Status; got != taskfile.StatusPending {
		t.Fatalf("file task status = %q, want pending", got)
	}
	if got := updatedTask.Phase; got != taskfile.PhaseAudit {
		t.Fatalf("file task phase = %q, want audit", got)
	}
	if result.Run.Status != ledger.StatusCompleted {
		t.Fatalf("run status = %q, want completed", result.Run.Status)
	}
	if result.Run.CommitSHA != "abc123def456" {
		t.Fatalf("ledger commit sha = %q, want abc123def456", result.Run.CommitSHA)
	}
	if result.ReceiptSynthesized {
		t.Fatal("receipt was synthesized, want parsed Codex receipt")
	}
	if got, want := result.Receipt.Metrics, (receipt.Metrics{InputTokens: 7, OutputTokens: 3, DurationSeconds: 1}); got != want {
		t.Fatalf("receipt metrics = %#v, want %#v", got, want)
	}
	if result.Receipt.CommitSHA != "abc123def456" {
		t.Fatalf("receipt commit sha = %q, want abc123def456", result.Receipt.CommitSHA)
	}
	if result.Receipt.VerificationStatus != "passed" {
		t.Fatalf("receipt verification status = %q, want passed", result.Receipt.VerificationStatus)
	}
	if got, want := result.Receipt.Verification, []receipt.VerificationEntry{{Command: "go test ./...", ExitCode: 0, Status: "passed"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("receipt verification entries = %#v, want %#v", got, want)
	}
	reparsedReceipt, err := receiptFromFile(result.ReceiptPath)
	if err != nil {
		t.Fatalf("parse final receipt: %v", err)
	}
	if reparsedReceipt.CommitSHA != "abc123def456" || reparsedReceipt.VerificationStatus != "passed" {
		t.Fatalf("final receipt = %+v, want commit sha and passed verification", reparsedReceipt)
	}
	if result.Run.CompletedAt == nil {
		t.Fatal("run completed at = nil, want completion time")
	}
	if !result.Receipt.Timestamp.Equal(*result.Run.CompletedAt) {
		t.Fatalf("receipt timestamp = %s, want run completed at %s", result.Receipt.Timestamp, *result.Run.CompletedAt)
	}
	if !reparsedReceipt.Timestamp.Equal(*result.Run.CompletedAt) {
		t.Fatalf("final receipt timestamp = %s, want run completed at %s", reparsedReceipt.Timestamp, *result.Run.CompletedAt)
	}
	if containsArg(state.codexArgs, "resume") {
		t.Fatalf("codex args include resume: %#v", state.codexArgs)
	}
	if got, want := state.gitCommands, [][]string{
		{"status", "--porcelain=v1", "-z", "--untracked-files=all"},
		{"status", "--porcelain=v1", "-z", "--untracked-files=all"},
		{"status", "--porcelain=v1", "-z", "--untracked-files=all"},
		{"rev-parse", "--verify", "--quiet", "HEAD"},
		{"--literal-pathspecs", "add", "--", "internal/feature.go"},
		{"commit", "-m", "Implement selected task", "-m", "Run-ID: " + result.Run.ID + "\nTask-ID: task-1\nVerification: passed"},
		{"rev-parse", "--verify", "--quiet", "HEAD"},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("git commands = %#v, want %#v", got, want)
	}
	if _, found, err := lock.ReadSourceWriter(ctx, env.workDir); err != nil || found {
		t.Fatalf("lock after successful run found=%v err=%v, want released", found, err)
	}
	history, ok, err := env.ledger.GetRunWithEvents(ctx, result.Run.ID)
	if err != nil {
		t.Fatalf("get run with events: %v", err)
	}
	if !ok {
		t.Fatal("run history not found")
	}
	artifacts, found := ledger.RunArtifactsFromEvents(history.Events)
	if !found {
		t.Fatal("run artifacts not found in ledger events")
	}
	wantArtifacts := ledger.RunArtifacts{
		ContextPayloadPath:   filepath.Join(".revolvr", "runs", result.Run.ID, "context.md"),
		ContextManifestPath:  filepath.Join(".revolvr", "runs", result.Run.ID, "context.json"),
		CodexStdoutJSONLPath: filepath.Join(".revolvr", "runs", result.Run.ID, "codex.jsonl"),
		CodexStderrPath:      filepath.Join(".revolvr", "runs", result.Run.ID, "codex.stderr"),
		LastMessagePath:      filepath.Join(".revolvr", "runs", result.Run.ID, "last-message.txt"),
		ReceiptPath:          filepath.Join(".revolvr", "receipts", result.Run.ID+".md"),
	}
	if !reflect.DeepEqual(artifacts, wantArtifacts) {
		t.Fatalf("run artifacts = %#v, want %#v", artifacts, wantArtifacts)
	}
	manifest := readContextManifestArtifact(t, env, result.Run.ID)
	var contextBuilt struct {
		Invocation codexexec.InvocationProvenance `json:"invocation"`
	}
	if !decodeTestEventPayload(t, history.Events, ledger.EventContextBuilt, &contextBuilt) {
		t.Fatal("context built event not found")
	}
	var codexStarted struct {
		Invocation codexexec.InvocationProvenance `json:"provenance"`
	}
	if !decodeTestEventPayload(t, history.Events, ledger.EventCodexStarted, &codexStarted) {
		t.Fatal("codex started event not found")
	}
	if !reflect.DeepEqual(manifest.Invocation, contextBuilt.Invocation) || !reflect.DeepEqual(manifest.Invocation, codexStarted.Invocation) {
		t.Fatalf("invocation provenance disagrees:\nmanifest=%+v\ncontext=%+v\nstarted=%+v", manifest.Invocation, contextBuilt.Invocation, codexStarted.Invocation)
	}
	if got := manifest.Invocation; got.Executable != "codex-test" || got.Version != "codex-test 1.2.3" || got.Model != codexexec.DefaultModel || got.ReasoningEffort != codexexec.DefaultReasoningEffort || !got.Ephemeral || got.SessionMode != codexexec.SessionModeEphemeral || got.EffectiveConfigSchema != EffectiveConfigSchema || got.EffectiveConfigSHA256 == "" || !reflect.DeepEqual(got.Argv, state.codexArgs) {
		t.Fatalf("invocation provenance = %+v, args=%#v", got, state.codexArgs)
	}
	assertRunEvents(t, env.ledger, result.Run.ID, []ledger.EventType{
		ledger.EventRunStarted,
		ledger.EventTaskSelected,
		ledger.EventRunArtifacts,
		ledger.EventContextBuilt,
		ledger.EventCodexStarted,
		ledger.EventCodexJSONEvent,
		ledger.EventCodexCompleted,
		ledger.EventChangedFilesCaptured,
		ledger.EventReceiptParsed,
		ledger.EventVerificationStarted,
		ledger.EventVerificationCompleted,
		ledger.EventChangedFilesCaptured,
		ledger.EventCommitStarted,
		ledger.EventCommitCreated,
		ledger.EventRunCompleted,
	})
}

func TestRunSuccessfulCommitChangedFilesIncludeAdvancedTaskFile(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	selected := writeRunTask(t, env, "task-commit-status", "Commit task status")

	var changedCaptureCalls int
	var commitChangedFiles []string
	result, err := Run(ctx, Config{
		WorkingDir:  env.workDir,
		LedgerStore: env.ledger,
		DirtyCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			return gitstate.Capture{Kind: gitstate.CaptureKindDirty}, nil
		},
		ChangedCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			changedCaptureCalls++
			switch changedCaptureCalls {
			case 1:
				if got := loadRunTask(t, env, selected.SourcePath).Status; got != taskfile.StatusPending {
					t.Fatalf("task status before verification capture = %q, want pending", got)
				}
				return gitstate.Capture{
					Kind:         gitstate.CaptureKindChanged,
					ChangedFiles: []string{"internal/feature.go"},
					Paths:        []string{"internal/feature.go"},
				}, nil
			case 2:
				updated := loadRunTask(t, env, selected.SourcePath)
				if got := updated.Status; got != taskfile.StatusPending {
					t.Fatalf("task status before commit capture = %q, want pending", got)
				}
				if got := updated.Phase; got != taskfile.PhaseAudit {
					t.Fatalf("task phase before commit capture = %q, want audit", got)
				}
				return gitstate.Capture{
					Kind:         gitstate.CaptureKindChanged,
					ChangedFiles: []string{"internal/feature.go", selected.SourcePath},
					Paths:        []string{"internal/feature.go", selected.SourcePath},
				}, nil
			default:
				t.Fatalf("changed capture call %d, want exactly 2", changedCaptureCalls)
				return gitstate.Capture{}, nil
			}
		},
		CodexRunner: func(context.Context, codexexec.Config) (codexexec.Result, error) {
			return codexexec.Result{ExitCode: 0, FinalMessage: "done"}, nil
		},
		VerificationRunner: func(context.Context, verification.Config) (verification.Result, error) {
			return passedVerificationResult("go test ./..."), nil
		},
		CommitRunner: func(_ context.Context, cfg commit.Config) (commit.Result, error) {
			commitChangedFiles = changedFiles(*cfg.PostRunChanged)
			return commit.Result{Status: commit.StatusCommitted, CommitSHA: "abc123", ChangedFiles: commitChangedFiles}, nil
		},
		Clock:                  env.clock,
		CodexVersionDiscoverer: testCodexVersionDiscoverer,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}

	if result.Outcome != OutcomeCommitted {
		t.Fatalf("outcome = %s, want committed; message=%s", result.Outcome, result.Message)
	}
	if changedCaptureCalls != 2 {
		t.Fatalf("changed capture calls = %d, want 2", changedCaptureCalls)
	}
	if got, want := commitChangedFiles, []string{"internal/feature.go", selected.SourcePath}; !reflect.DeepEqual(got, want) {
		t.Fatalf("commit changed files = %#v, want %#v", got, want)
	}
	updated := loadRunTask(t, env, selected.SourcePath)
	if got := updated.Status; got != taskfile.StatusPending {
		t.Fatalf("file task status = %q, want pending", got)
	}
	if got := updated.Phase; got != taskfile.PhaseAudit {
		t.Fatalf("file task phase = %q, want audit", got)
	}
}

func TestRunPolicyPermittedNoChangePhaseAdvancement(t *testing.T) {
	tests := []struct {
		name          string
		phase         string
		profileName   string
		profileText   string
		wantPhase     string
		wantStatus    string
		wantCompleted bool
	}{
		{
			name:        "audit advances to document",
			phase:       taskfile.PhaseAudit,
			profileName: "auditor",
			profileText: "Auditor profile for a no-change phase run.\n\nConfirm the implementation.",
			wantPhase:   taskfile.PhaseDocument,
			wantStatus:  taskfile.StatusPending,
		},
		{
			name:        "document advances to simplify",
			phase:       taskfile.PhaseDocument,
			profileName: "documentor",
			profileText: "Documentor profile for a no-change phase run.\n\nUpdate durable docs only when needed.",
			wantPhase:   taskfile.PhaseSimplify,
			wantStatus:  taskfile.StatusPending,
		},
		{
			name:          "simplify completes task",
			phase:         taskfile.PhaseSimplify,
			profileName:   "simplifier",
			profileText:   "Simplifier profile for a no-change phase run.\n\nSimplify only when worthwhile.",
			wantPhase:     taskfile.PhaseSimplify,
			wantStatus:    taskfile.StatusCompleted,
			wantCompleted: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			env := newTestEnv(t)
			writeTestRunProfile(t, env.workDir, tt.profileName, tt.profileText)
			selected := writeRunTaskWithPhase(t, env, "task-"+tt.phase+"-success", "Advance "+tt.phase+" work", tt.phase)

			var changedCaptureCalls int
			var runnerPayload string
			var commitChangedFiles []string
			result, err := Run(ctx, Config{
				WorkingDir:  env.workDir,
				LedgerStore: env.ledger,
				DirtyCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
					return gitstate.Capture{Kind: gitstate.CaptureKindDirty}, nil
				},
				ChangedCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
					changedCaptureCalls++
					switch changedCaptureCalls {
					case 1:
						before := loadRunTask(t, env, selected.SourcePath)
						if got := before.Phase; got != tt.phase {
							t.Fatalf("task phase before metadata update = %q, want %q", got, tt.phase)
						}
						return gitstate.Capture{Kind: gitstate.CaptureKindChanged}, nil
					case 2:
						after := loadRunTask(t, env, selected.SourcePath)
						if got := after.Status; got != tt.wantStatus {
							t.Fatalf("task status before commit capture = %q, want %q", got, tt.wantStatus)
						}
						if got := after.Phase; got != tt.wantPhase {
							t.Fatalf("task phase before commit capture = %q, want %q", got, tt.wantPhase)
						}
						return gitstate.Capture{
							Kind:         gitstate.CaptureKindChanged,
							ChangedFiles: []string{selected.SourcePath},
							Paths:        []string{selected.SourcePath},
						}, nil
					default:
						t.Fatalf("changed capture call %d, want exactly 2", changedCaptureCalls)
						return gitstate.Capture{}, nil
					}
				},
				CodexRunner: func(_ context.Context, cfg codexexec.Config) (codexexec.Result, error) {
					runnerPayload = cfg.Prompt
					return codexexec.Result{ExitCode: 0, FinalMessage: "done"}, nil
				},
				VerificationRunner: func(context.Context, verification.Config) (verification.Result, error) {
					return passedVerificationResult("go test ./..."), nil
				},
				CommitRunner: func(_ context.Context, cfg commit.Config) (commit.Result, error) {
					commitChangedFiles = changedFiles(*cfg.PostRunChanged)
					return commit.Result{Status: commit.StatusCommitted, CommitSHA: "abc123", ChangedFiles: commitChangedFiles}, nil
				},
				Clock:                  env.clock,
				CodexVersionDiscoverer: testCodexVersionDiscoverer,
			})
			if err != nil {
				t.Fatalf("run once: %v", err)
			}

			if result.Outcome != OutcomeCommitted {
				t.Fatalf("outcome = %s, want committed; message=%s", result.Outcome, result.Message)
			}
			if changedCaptureCalls != 2 {
				t.Fatalf("changed capture calls = %d, want 2", changedCaptureCalls)
			}
			if !strings.Contains(runnerPayload, strings.TrimSpace(tt.profileText)) {
				t.Fatalf("context payload missing selected profile:\n%s", runnerPayload)
			}
			if got, want := commitChangedFiles, []string{selected.SourcePath}; !reflect.DeepEqual(got, want) {
				t.Fatalf("commit changed files = %#v, want %#v", got, want)
			}

			updated := loadRunTask(t, env, selected.SourcePath)
			if got := updated.Status; got != tt.wantStatus {
				t.Fatalf("file task status = %q, want %q", got, tt.wantStatus)
			}
			if got := updated.Phase; got != tt.wantPhase {
				t.Fatalf("file task phase = %q, want %q", got, tt.wantPhase)
			}
			if got := result.Task.Status; got != tt.wantStatus {
				t.Fatalf("result task status = %q, want %q", got, tt.wantStatus)
			}
			if tt.wantCompleted && result.Task.CompletedAt == nil {
				t.Fatal("result task completed at = nil, want completion timestamp")
			}
			if !tt.wantCompleted && result.Task.CompletedAt != nil {
				t.Fatalf("result task completed at = %s, want nil", *result.Task.CompletedAt)
			}
			if result.Run.Status != ledger.StatusCompleted || result.Run.CommitSHA != "abc123" {
				t.Fatalf("run status/commit = %s/%s, want completed/abc123", result.Run.Status, result.Run.CommitSHA)
			}
			if result.Receipt.Verdict != receipt.VerdictCompleted {
				t.Fatalf("receipt verdict = %q, want completed", result.Receipt.Verdict)
			}
			if result.Receipt.VerificationStatus != "passed" || result.Receipt.CommitSHA != "abc123" {
				t.Fatalf("receipt verification/commit = %q/%q, want passed/abc123", result.Receipt.VerificationStatus, result.Receipt.CommitSHA)
			}
			if got, want := result.Receipt.ChangedFiles, []string{selected.SourcePath}; !reflect.DeepEqual(got, want) {
				t.Fatalf("receipt changed files = %#v, want %#v", got, want)
			}

			history, ok, err := env.ledger.GetRunWithEvents(ctx, result.Run.ID)
			if err != nil || !ok {
				t.Fatalf("get run history ok=%v err=%v", ok, err)
			}
			var terminal struct {
				Phase                  string `json:"phase"`
				NextPhase              string `json:"next_phase"`
				CompletesTask          bool   `json:"completes_task"`
				PhaseTransitionApplied bool   `json:"phase_transition_applied"`
				TaskStatus             string `json:"task_status"`
				TaskPhase              string `json:"task_phase"`
			}
			if !decodeTestEventPayload(t, history.Events, ledger.EventRunCompleted, &terminal) {
				t.Fatal("run completed event not found")
			}
			if got := terminal.Phase; got != tt.phase {
				t.Fatalf("terminal phase = %q, want %q", got, tt.phase)
			}
			if got := terminal.TaskStatus; got != tt.wantStatus {
				t.Fatalf("terminal task status = %q, want %q", got, tt.wantStatus)
			}
			if got := terminal.TaskPhase; got != tt.wantPhase {
				t.Fatalf("terminal task phase = %q, want %q", got, tt.wantPhase)
			}
			if got := terminal.CompletesTask; got != tt.wantCompleted {
				t.Fatalf("terminal completes task = %v, want %v", got, tt.wantCompleted)
			}
			if !terminal.PhaseTransitionApplied {
				t.Fatal("terminal phase_transition_applied = false, want true")
			}
		})
	}
}

func TestRunCommitFailureAfterMetadataUpdateDoesNotAdvancePhase(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	writeTestRunProfile(t, env.workDir, "auditor", "Auditor profile.\n\nReview before document.")
	selected := writeRunTaskWithPhase(t, env, "task-audit-commit-fail", "Audit commit failure", taskfile.PhaseAudit)

	var changedCaptureCalls int
	result, err := Run(ctx, Config{
		WorkingDir:  env.workDir,
		LedgerStore: env.ledger,
		DirtyCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			return gitstate.Capture{Kind: gitstate.CaptureKindDirty}, nil
		},
		ChangedCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			changedCaptureCalls++
			switch changedCaptureCalls {
			case 1:
				return gitstate.Capture{Kind: gitstate.CaptureKindChanged}, nil
			default:
				return gitstate.Capture{
					Kind:         gitstate.CaptureKindChanged,
					ChangedFiles: []string{selected.SourcePath},
					Paths:        []string{selected.SourcePath},
				}, nil
			}
		},
		CodexRunner: func(context.Context, codexexec.Config) (codexexec.Result, error) {
			return codexexec.Result{ExitCode: 0, FinalMessage: "done"}, nil
		},
		VerificationRunner: func(context.Context, verification.Config) (verification.Result, error) {
			return passedVerificationResult("go test ./..."), nil
		},
		CommitRunner: func(context.Context, commit.Config) (commit.Result, error) {
			return commit.Result{Status: commit.StatusFailed, Message: "git commit failed"}, nil
		},
		Clock:                  env.clock,
		CodexVersionDiscoverer: testCodexVersionDiscoverer,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}

	if result.Outcome != OutcomeCommitFailed {
		t.Fatalf("outcome = %s, want commit_failed; message=%s", result.Outcome, result.Message)
	}
	updated := loadRunTask(t, env, selected.SourcePath)
	if got := updated.Status; got != taskfile.StatusBlocked {
		t.Fatalf("file task status = %q, want blocked", got)
	}
	if got := updated.Phase; got != taskfile.PhaseAudit {
		t.Fatalf("file task phase = %q, want original audit", got)
	}
	if result.Task.Status != taskmodel.StatusBlocked {
		t.Fatalf("result task status = %q, want blocked", result.Task.Status)
	}
	if changedCaptureCalls != 3 {
		t.Fatalf("changed capture calls = %d, want pre-transition, post-transition, and post-rollback captures", changedCaptureCalls)
	}
	if got, want := result.Receipt.ChangedFiles, []string{selected.SourcePath}; !reflect.DeepEqual(got, want) {
		t.Fatalf("receipt changed files = %#v, want final rollback state %#v", got, want)
	}
	if result.Receipt.Verdict != receipt.VerdictBlocked {
		t.Fatalf("receipt verdict = %q, want blocked", result.Receipt.Verdict)
	}
	history, ok, err := env.ledger.GetRunWithEvents(ctx, result.Run.ID)
	if err != nil || !ok {
		t.Fatalf("get run history ok=%v err=%v", ok, err)
	}
	var terminal struct {
		TaskStatus             string          `json:"task_status"`
		TaskPhase              string          `json:"task_phase"`
		ChangedFiles           []string        `json:"changed_files"`
		ReceiptVerdict         receipt.Verdict `json:"receipt_verdict"`
		PhaseTransitionApplied bool            `json:"phase_transition_applied"`
	}
	if !decodeTestEventPayload(t, history.Events, ledger.EventRunFailed, &terminal) {
		t.Fatal("run failed event not found")
	}
	if terminal.TaskStatus != taskfile.StatusBlocked || terminal.TaskPhase != taskfile.PhaseAudit || terminal.ReceiptVerdict != receipt.VerdictBlocked || !terminal.PhaseTransitionApplied {
		t.Fatalf("terminal lifecycle payload = %+v, want blocked audit rollback", terminal)
	}
	if got, want := terminal.ChangedFiles, []string{selected.SourcePath}; !reflect.DeepEqual(got, want) {
		t.Fatalf("terminal changed files = %#v, want %#v", got, want)
	}
}

func TestRunIndeterminateCommitBlocksTransitionedPhaseForInspection(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	writeTestRunProfile(t, env.workDir, "auditor", "Auditor profile.\n\nReview before document.")
	selected := writeRunTaskWithPhase(t, env, "task-audit-commit-unknown", "Audit indeterminate commit", taskfile.PhaseAudit)

	var changedCaptureCalls int
	result, err := Run(ctx, Config{
		WorkingDir:  env.workDir,
		LedgerStore: env.ledger,
		DirtyCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			return gitstate.Capture{Kind: gitstate.CaptureKindDirty}, nil
		},
		ChangedCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			changedCaptureCalls++
			return gitstate.Capture{
				Kind:         gitstate.CaptureKindChanged,
				ChangedFiles: []string{selected.SourcePath},
				Paths:        []string{selected.SourcePath},
			}, nil
		},
		CodexRunner: func(context.Context, codexexec.Config) (codexexec.Result, error) {
			return codexexec.Result{ExitCode: 0, FinalMessage: "done"}, nil
		},
		VerificationRunner: func(context.Context, verification.Config) (verification.Result, error) {
			return passedVerificationResult("go test ./..."), nil
		},
		CommitRunner: func(context.Context, commit.Config) (commit.Result, error) {
			return commit.Result{
				Status:            commit.StatusIndeterminate,
				Message:           "git commit outcome is indeterminate: resolve HEAD after commit failed",
				PreCommitSHA:      "parent123",
				HEADLookupRetried: true,
				Commands: []commit.GitCommandResult{
					{Args: []string{"--literal-pathspecs", "add", "--", selected.SourcePath}, ExitCode: 0},
				},
			}, nil
		},
		CommandRunner: func(context.Context, runner.Command) runner.Result {
			return runner.Result{ExitCode: 0}
		},
		Clock:                  env.clock,
		CodexVersionDiscoverer: testCodexVersionDiscoverer,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}

	if result.Outcome != OutcomeCommitFailed || result.Commit.Status != commit.StatusIndeterminate {
		t.Fatalf("outcome/commit = %s/%s, want commit_failed/indeterminate", result.Outcome, result.Commit.Status)
	}
	updated := loadRunTask(t, env, selected.SourcePath)
	if updated.Status != taskfile.StatusBlocked || updated.Phase != taskfile.PhaseDocument {
		t.Fatalf("task status/phase = %s/%s, want blocked transitioned document phase", updated.Status, updated.Phase)
	}
	if changedCaptureCalls != 3 {
		t.Fatalf("changed capture calls = %d, want pre-transition, post-transition, and final captures", changedCaptureCalls)
	}
	history, ok, historyErr := env.ledger.GetRunWithEvents(ctx, result.Run.ID)
	if historyErr != nil || !ok {
		t.Fatalf("get run history ok=%v err=%v", ok, historyErr)
	}
	var terminal struct {
		TaskPhase       string        `json:"task_phase"`
		CommitStatus    commit.Status `json:"commit_status"`
		CommitPreHEAD   string        `json:"commit_pre_head"`
		CommitPostHEAD  string        `json:"commit_post_head"`
		CommitHeadRetry bool          `json:"commit_head_retry"`
		Restaged        bool          `json:"task_restage_applied"`
	}
	if !decodeTestEventPayload(t, history.Events, ledger.EventRunFailed, &terminal) {
		t.Fatal("run failed event not found")
	}
	if terminal.TaskPhase != taskfile.PhaseDocument || terminal.CommitStatus != commit.StatusIndeterminate || terminal.CommitPreHEAD != "parent123" || terminal.CommitPostHEAD != "" || !terminal.CommitHeadRetry || !terminal.Restaged {
		t.Fatalf("terminal indeterminate evidence = %+v", terminal)
	}
}

func TestRunRealGitCommitFailureRestagesBlockedOriginalPhase(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	runTestGit(t, workDir, "init", "-q")
	runTestGit(t, workDir, "config", "user.name", "Revolvr Test")
	runTestGit(t, workDir, "config", "user.email", "revolvr-test@example.invalid")
	if err := os.WriteFile(filepath.Join(workDir, ".git", "info", "exclude"), []byte("/.revolvr/\n"), 0o644); err != nil {
		t.Fatalf("write git exclude: %v", err)
	}

	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	runs, err := ledger.OpenWithClock(ctx, filepath.Join(t.TempDir(), "ledger.sqlite"), func() time.Time { return now })
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	t.Cleanup(func() { _ = runs.Close() })
	env := testEnv{workDir: workDir, ledger: runs, now: now}
	writeTestRunProfile(t, workDir, "auditor", "Auditor profile.")
	selected := writeRunTaskWithPhase(t, env, "task-real-commit-fail", "Real commit failure", taskfile.PhaseAudit)
	runTestGit(t, workDir, "add", ".")
	runTestGit(t, workDir, "commit", "-q", "-m", "Initial task")
	hookPath := filepath.Join(workDir, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write failing pre-commit hook: %v", err)
	}

	result, err := Run(ctx, Config{
		WorkingDir:  workDir,
		LedgerStore: runs,
		CodexRunner: func(context.Context, codexexec.Config) (codexexec.Result, error) {
			return codexexec.Result{ExitCode: 0, FinalMessage: "audit complete"}, nil
		},
		VerificationRunner: func(context.Context, verification.Config) (verification.Result, error) {
			return passedVerificationResult("go test ./..."), nil
		},
		Clock:                  func() time.Time { return now.Add(time.Minute) },
		CodexVersionDiscoverer: testCodexVersionDiscoverer,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if result.Outcome != OutcomeCommitFailed || result.Commit.Status != commit.StatusFailed {
		t.Fatalf("outcome/commit = %s/%s, want commit_failed/failed", result.Outcome, result.Commit.Status)
	}
	working := loadRunTask(t, env, selected.SourcePath)
	if working.Status != taskfile.StatusBlocked || working.Phase != taskfile.PhaseAudit {
		t.Fatalf("working task status/phase = %s/%s, want blocked/audit", working.Status, working.Phase)
	}
	indexCommand := exec.Command("git", "show", ":"+selected.SourcePath)
	indexCommand.Dir = workDir
	indexBytes, indexErr := indexCommand.CombinedOutput()
	if indexErr != nil {
		t.Fatalf("read staged task: %v\n%s", indexErr, indexBytes)
	}
	if !strings.Contains(string(indexBytes), "status: blocked") || !strings.Contains(string(indexBytes), "phase: audit") || strings.Contains(string(indexBytes), "phase: document") {
		t.Fatalf("staged task did not match blocked original phase:\n%s", indexBytes)
	}
	history, ok, historyErr := runs.GetRunWithEvents(ctx, result.Run.ID)
	if historyErr != nil || !ok {
		t.Fatalf("get run history ok=%v err=%v", ok, historyErr)
	}
	var terminal struct {
		TaskRestageApplied bool   `json:"task_restage_applied"`
		TaskRestageError   string `json:"task_restage_error"`
	}
	if !decodeTestEventPayload(t, history.Events, ledger.EventRunFailed, &terminal) || !terminal.TaskRestageApplied || terminal.TaskRestageError != "" {
		t.Fatalf("terminal restage evidence = %+v, want successful restage", terminal)
	}

	if _, changed, retryErr := taskfile.UpdateBlockedToPending(workDir, selected.ID); retryErr != nil || !changed {
		t.Fatalf("retry blocked task changed=%v err=%v", changed, retryErr)
	}
	statusCommand := exec.Command("git", "status", "--short", "--untracked-files=all")
	statusCommand.Dir = workDir
	statusBytes, statusErr := statusCommand.CombinedOutput()
	if statusErr != nil {
		t.Fatalf("git status after retry: %v\n%s", statusErr, statusBytes)
	}
	if !strings.Contains(string(statusBytes), selected.SourcePath) {
		t.Fatalf("git status after retry = %q, want dirty partial work to remain visible", statusBytes)
	}
}

func TestRunRejectsSelectedTaskMutationAndRestoresSnapshot(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	selected := writeRunTask(t, env, "task-mutated", "Keep durable task identity")
	mutated := `---
id: replacement-task
status: completed
workflow: mixed-pass-v1
phase: simplify
---
# Replacement Task

Mutated instructions.
`
	var captureCalls int
	commitCalled := false
	result, err := Run(ctx, Config{
		WorkingDir:  env.workDir,
		LedgerStore: env.ledger,
		DirtyCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			return gitstate.Capture{Kind: gitstate.CaptureKindDirty}, nil
		},
		ChangedCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			captureCalls++
			return gitstate.Capture{Kind: gitstate.CaptureKindChanged, ChangedFiles: []string{selected.SourcePath}, Paths: []string{selected.SourcePath}}, nil
		},
		CodexRunner: func(context.Context, codexexec.Config) (codexexec.Result, error) {
			if err := os.WriteFile(filepath.Join(env.workDir, selected.SourcePath), []byte(mutated), 0o644); err != nil {
				return codexexec.Result{}, err
			}
			return codexexec.Result{ExitCode: 0, FinalMessage: "changed task metadata"}, nil
		},
		VerificationRunner: func(context.Context, verification.Config) (verification.Result, error) {
			return passedVerificationResult("go test ./..."), nil
		},
		CommitRunner: func(context.Context, commit.Config) (commit.Result, error) {
			commitCalled = true
			return commit.Result{Status: commit.StatusCommitted, CommitSHA: "unexpected"}, nil
		},
		Clock:                  env.clock,
		CodexVersionDiscoverer: testCodexVersionDiscoverer,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if result.Outcome != OutcomeBlocked || commitCalled {
		t.Fatalf("outcome=%s commitCalled=%v, want blocked before commit", result.Outcome, commitCalled)
	}
	if !strings.Contains(result.Message, "selected task changed during the pass") {
		t.Fatalf("message = %q, want selected-task mutation", result.Message)
	}
	restored := loadRunTask(t, env, selected.SourcePath)
	if restored.ID != selected.ID || restored.Title != selected.Title || restored.Status != taskfile.StatusBlocked || restored.Phase != taskfile.PhaseImplement {
		t.Fatalf("restored task = %+v, want original identity/body at blocked implement phase", restored)
	}
	if !strings.Contains(restored.ContextBody, "Keep durable task identity") || strings.Contains(restored.ContextBody, "Mutated instructions") {
		t.Fatalf("restored task body is not the selected snapshot:\n%s", restored.ContextBody)
	}
	if result.Receipt.TaskID != selected.ID || result.Receipt.Verdict != receipt.VerdictBlocked {
		t.Fatalf("receipt identity/verdict = %q/%q, want %q/blocked", result.Receipt.TaskID, result.Receipt.Verdict, selected.ID)
	}
	if captureCalls != 2 {
		t.Fatalf("changed capture calls = %d, want post-Codex and post-restore", captureCalls)
	}
}

func TestRunImplementTaskFileOnlyChangeIsNotMeaningful(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	selected := writeRunTask(t, env, "task-file-only", "Require meaningful implementation")
	var captureCalls int
	commitCalled := false
	result, err := Run(ctx, Config{
		WorkingDir:  env.workDir,
		LedgerStore: env.ledger,
		DirtyCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			return gitstate.Capture{Kind: gitstate.CaptureKindDirty}, nil
		},
		ChangedCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			captureCalls++
			return gitstate.Capture{Kind: gitstate.CaptureKindChanged, ChangedFiles: []string{selected.SourcePath}, Paths: []string{selected.SourcePath}}, nil
		},
		CodexRunner: func(context.Context, codexexec.Config) (codexexec.Result, error) {
			return codexexec.Result{ExitCode: 0, FinalMessage: "no implementation change"}, nil
		},
		VerificationRunner: func(context.Context, verification.Config) (verification.Result, error) {
			return passedVerificationResult("go test ./..."), nil
		},
		CommitRunner: func(context.Context, commit.Config) (commit.Result, error) {
			commitCalled = true
			return commit.Result{}, nil
		},
		Clock:                  env.clock,
		CodexVersionDiscoverer: testCodexVersionDiscoverer,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if result.Outcome != OutcomeNoChanges || commitCalled {
		t.Fatalf("outcome=%s commitCalled=%v, want no_changes before commit", result.Outcome, commitCalled)
	}
	if result.Commit.Status != commit.StatusRefused || result.Commit.RefusalReason != commit.ReasonNoChanges {
		t.Fatalf("commit result = %+v, want meaningful no-changes refusal", result.Commit)
	}
	updated := loadRunTask(t, env, selected.SourcePath)
	if updated.Status != taskfile.StatusBlocked || updated.Phase != taskfile.PhaseImplement {
		t.Fatalf("task status/phase = %s/%s, want blocked/implement", updated.Status, updated.Phase)
	}
	if captureCalls != 2 {
		t.Fatalf("changed capture calls = %d, want pre-transition and final failure captures", captureCalls)
	}
}

func TestRunImplementPreExistingDirtyFileDoesNotCountAsNewMeaningfulChange(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	selected := writeRunTask(t, env, "task-preexisting-only", "Do not reuse pre-existing changes")
	commitCalled := false
	result, err := Run(ctx, Config{
		WorkingDir:            env.workDir,
		LedgerStore:           env.ledger,
		AllowPreExistingDirty: true,
		DirtyCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			return gitstate.Capture{Kind: gitstate.CaptureKindDirty, DirtyFiles: []string{"internal/already-dirty.go"}, Paths: []string{"internal/already-dirty.go"}}, nil
		},
		ChangedCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			return gitstate.Capture{Kind: gitstate.CaptureKindChanged, ChangedFiles: []string{"internal/already-dirty.go", selected.SourcePath}, Paths: []string{"internal/already-dirty.go", selected.SourcePath}}, nil
		},
		CodexRunner: func(context.Context, codexexec.Config) (codexexec.Result, error) {
			return codexexec.Result{ExitCode: 0}, nil
		},
		VerificationRunner: func(context.Context, verification.Config) (verification.Result, error) {
			return passedVerificationResult("go test ./..."), nil
		},
		CommitRunner: func(context.Context, commit.Config) (commit.Result, error) {
			commitCalled = true
			return commit.Result{}, nil
		},
		Clock:                  env.clock,
		CodexVersionDiscoverer: testCodexVersionDiscoverer,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if result.Outcome != OutcomeNoChanges || commitCalled {
		t.Fatalf("outcome=%s commitCalled=%v, want no_changes without commit", result.Outcome, commitCalled)
	}
	updated := loadRunTask(t, env, selected.SourcePath)
	if updated.Status != taskfile.StatusBlocked || updated.Phase != taskfile.PhaseImplement {
		t.Fatalf("task status/phase = %s/%s, want blocked/implement", updated.Status, updated.Phase)
	}
}

func TestRunChangedFileCaptureFailureBlocksBeforeTransitionAndCommit(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	selected := writeRunTask(t, env, "task-capture-failure", "Capture changed files")
	var captureCalls int
	commitCalled := false
	result, err := Run(ctx, Config{
		WorkingDir:  env.workDir,
		LedgerStore: env.ledger,
		DirtyCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			return gitstate.Capture{Kind: gitstate.CaptureKindDirty}, nil
		},
		ChangedCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			captureCalls++
			if captureCalls == 1 {
				return gitstate.Capture{}, errors.New("git status unavailable")
			}
			return gitstate.Capture{Kind: gitstate.CaptureKindChanged, ChangedFiles: []string{selected.SourcePath}, Paths: []string{selected.SourcePath}}, nil
		},
		CodexRunner: func(context.Context, codexexec.Config) (codexexec.Result, error) {
			return codexexec.Result{ExitCode: 0}, nil
		},
		VerificationRunner: func(context.Context, verification.Config) (verification.Result, error) {
			return passedVerificationResult("go test ./..."), nil
		},
		CommitRunner: func(context.Context, commit.Config) (commit.Result, error) {
			commitCalled = true
			return commit.Result{}, nil
		},
		Clock:                  env.clock,
		CodexVersionDiscoverer: testCodexVersionDiscoverer,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if result.Outcome != OutcomeBlocked || commitCalled {
		t.Fatalf("outcome=%s commitCalled=%v, want blocked before commit", result.Outcome, commitCalled)
	}
	if result.Commit.RefusalReason != commit.ReasonGitStateCaptureFailed || !strings.Contains(result.Message, "git status unavailable") {
		t.Fatalf("commit/message = %+v / %q, want capture refusal evidence", result.Commit, result.Message)
	}
	updated := loadRunTask(t, env, selected.SourcePath)
	if updated.Status != taskfile.StatusBlocked || updated.Phase != taskfile.PhaseImplement {
		t.Fatalf("task status/phase = %s/%s, want blocked/implement", updated.Status, updated.Phase)
	}
	history, ok, historyErr := env.ledger.GetRunWithEvents(ctx, result.Run.ID)
	if historyErr != nil || !ok {
		t.Fatalf("get run history ok=%v err=%v", ok, historyErr)
	}
	var terminal struct {
		CaptureError string `json:"capture_error"`
	}
	if !decodeTestEventPayload(t, history.Events, ledger.EventRunFailed, &terminal) || !strings.Contains(terminal.CaptureError, "git status unavailable") {
		t.Fatalf("terminal capture error = %q, want original capture failure", terminal.CaptureError)
	}
}

func TestRunCommitRunnerErrorPreservesResultAndRollsBackPhase(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	writeTestRunProfile(t, env.workDir, "auditor", "Auditor profile.")
	selected := writeRunTaskWithPhase(t, env, "task-commit-error", "Commit runner error", taskfile.PhaseAudit)
	var captureCalls int
	result, err := Run(ctx, Config{
		WorkingDir:  env.workDir,
		LedgerStore: env.ledger,
		DirtyCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			return gitstate.Capture{Kind: gitstate.CaptureKindDirty}, nil
		},
		ChangedCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			captureCalls++
			if captureCalls == 1 {
				return gitstate.Capture{Kind: gitstate.CaptureKindChanged}, nil
			}
			return gitstate.Capture{Kind: gitstate.CaptureKindChanged, ChangedFiles: []string{selected.SourcePath}, Paths: []string{selected.SourcePath}}, nil
		},
		CodexRunner: func(context.Context, codexexec.Config) (codexexec.Result, error) {
			return codexexec.Result{ExitCode: 0}, nil
		},
		VerificationRunner: func(context.Context, verification.Config) (verification.Result, error) {
			return passedVerificationResult("go test ./..."), nil
		},
		CommitRunner: func(context.Context, commit.Config) (commit.Result, error) {
			return commit.Result{Status: commit.StatusFailed, Message: "git process unavailable"}, errors.New("start git: executable not found")
		},
		Clock:                  env.clock,
		CodexVersionDiscoverer: testCodexVersionDiscoverer,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if result.Outcome != OutcomeCommitFailed || result.Commit.Status != commit.StatusFailed || result.Commit.Message != "git process unavailable" {
		t.Fatalf("outcome/commit = %s / %+v, want preserved commit runner failure", result.Outcome, result.Commit)
	}
	if !strings.Contains(result.Message, "start git: executable not found") {
		t.Fatalf("message = %q, want commit runner error", result.Message)
	}
	updated := loadRunTask(t, env, selected.SourcePath)
	if updated.Status != taskfile.StatusBlocked || updated.Phase != taskfile.PhaseAudit {
		t.Fatalf("task status/phase = %s/%s, want blocked/original audit", updated.Status, updated.Phase)
	}
	if result.Run.Status != ledger.StatusFailed || result.Receipt.Verdict != receipt.VerdictBlocked {
		t.Fatalf("run/receipt = %s/%s, want failed/blocked", result.Run.Status, result.Receipt.Verdict)
	}
}

func TestRunTaskRollbackFailureStillFinalizesLedgerEvidence(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	writeTestRunProfile(t, env.workDir, "auditor", "Auditor profile.")
	selected := writeRunTaskWithPhase(t, env, "task-rollback-error", "Rollback failure", taskfile.PhaseAudit)
	var captureCalls int
	result, err := Run(ctx, Config{
		WorkingDir:  env.workDir,
		LedgerStore: env.ledger,
		DirtyCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			return gitstate.Capture{Kind: gitstate.CaptureKindDirty}, nil
		},
		ChangedCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			captureCalls++
			return gitstate.Capture{Kind: gitstate.CaptureKindChanged}, nil
		},
		CodexRunner: func(context.Context, codexexec.Config) (codexexec.Result, error) {
			return codexexec.Result{ExitCode: 0}, nil
		},
		VerificationRunner: func(context.Context, verification.Config) (verification.Result, error) {
			return passedVerificationResult("go test ./..."), nil
		},
		CommitRunner: func(context.Context, commit.Config) (commit.Result, error) {
			if removeErr := os.RemoveAll(filepath.Join(env.workDir, taskfile.TasksDir)); removeErr != nil {
				t.Fatalf("remove tasks directory: %v", removeErr)
			}
			return commit.Result{Status: commit.StatusFailed, Message: "git commit failed"}, nil
		},
		Clock:                  env.clock,
		CodexVersionDiscoverer: testCodexVersionDiscoverer,
	})
	if err == nil || !strings.Contains(err.Error(), "update task after failed run") {
		t.Fatalf("run error = %v, want task rollback failure", err)
	}
	if result.Outcome != OutcomeCommitFailed || result.Run.Status != ledger.StatusFailed || result.Receipt.Verdict != receipt.VerdictBlocked {
		t.Fatalf("result outcome/run/receipt = %s/%s/%s, want commit_failed/failed/blocked", result.Outcome, result.Run.Status, result.Receipt.Verdict)
	}
	if _, statErr := os.Stat(filepath.Join(env.workDir, selected.SourcePath)); !os.IsNotExist(statErr) {
		t.Fatalf("removed task stat error = %v, want missing task", statErr)
	}
	history, ok, historyErr := env.ledger.GetRunWithEvents(ctx, result.Run.ID)
	if historyErr != nil || !ok {
		t.Fatalf("get run history ok=%v err=%v", ok, historyErr)
	}
	var terminal struct {
		TaskUpdateError string          `json:"task_update_error"`
		ReceiptVerdict  receipt.Verdict `json:"receipt_verdict"`
	}
	if !decodeTestEventPayload(t, history.Events, ledger.EventRunFailed, &terminal) {
		t.Fatal("run failed event not found after task rollback error")
	}
	if terminal.TaskUpdateError == "" || terminal.ReceiptVerdict != receipt.VerdictBlocked {
		t.Fatalf("terminal rollback evidence = %+v, want task error and blocked receipt", terminal)
	}
}

func TestRunSelectsLowestPriorityPendingTaskFileByPriorityThenFilename(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	writeRunTaskFile(t, env, "020-later.md", taskFileMarkdown("task-later", "Later Task", taskfile.StatusPending, ptrInt(20)))
	writeRunTaskFile(t, env, "010-beta.md", taskFileMarkdown("task-beta", "Beta Task", taskfile.StatusPending, ptrInt(10)))
	selected := writeRunTaskFile(t, env, "010-alpha.md", taskFileMarkdown("task-alpha", "Alpha Task", taskfile.StatusPending, ptrInt(10)))
	writeRunTaskFile(t, env, "001-completed.md", taskFileMarkdown("task-completed", "Completed Task", taskfile.StatusCompleted, ptrInt(1)))

	result, err := Run(ctx, Config{
		WorkingDir:     env.workDir,
		LedgerStore:    env.ledger,
		DirtyCapture:   cleanDirtyCapture,
		ChangedCapture: emptyChangedCapture,
		CodexRunner: func(context.Context, codexexec.Config) (codexexec.Result, error) {
			return codexexec.Result{ExitCode: 1}, nil
		},
		Clock:                  env.clock,
		CodexVersionDiscoverer: testCodexVersionDiscoverer,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if result.Task.ID != "task-alpha" {
		t.Fatalf("selected task id = %q, want task-alpha", result.Task.ID)
	}
	if result.FileTask.SourcePath != selected.SourcePath {
		t.Fatalf("selected task path = %q, want %q", result.FileTask.SourcePath, selected.SourcePath)
	}
	if got := loadRunTask(t, env, selected.SourcePath).Status; got != taskfile.StatusBlocked {
		t.Fatalf("selected file status = %q, want blocked", got)
	}
	if got := loadRunTask(t, env, filepath.Join(taskfile.TasksDir, "010-beta.md")).Status; got != taskfile.StatusPending {
		t.Fatalf("unselected file status = %q, want pending", got)
	}
}

func TestRunDependencySelectionUsesSharedSelectedIdentity(t *testing.T) {
	for _, tt := range []struct {
		name             string
		dependencyStatus string
		wantSelected     string
	}{
		{name: "pending prerequisite outranks preferred dependent", dependencyStatus: taskfile.StatusPending, wantSelected: "dependency"},
		{name: "completed prerequisite unlocks dependent", dependencyStatus: taskfile.StatusCompleted, wantSelected: "dependent"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestEnv(t)
			writeRunTaskFile(t, env, "020-dependency.md", taskFileMarkdownWithScheduling("dependency", "Dependency", tt.dependencyStatus, taskfile.PhaseImplement, ptrInt(50), nil))
			writeRunTaskFile(t, env, "010-dependent.md", taskFileMarkdownWithScheduling("dependent", "Dependent", taskfile.StatusPending, taskfile.PhaseImplement, ptrInt(1), []string{"dependency"}))
			codexCalls := 0
			result, err := Run(context.Background(), Config{
				WorkingDir:     env.workDir,
				LedgerStore:    env.ledger,
				DirtyCapture:   cleanDirtyCapture,
				ChangedCapture: emptyChangedCapture,
				CodexRunner: func(context.Context, codexexec.Config) (codexexec.Result, error) {
					codexCalls++
					return codexexec.Result{ExitCode: 1}, nil
				},
				Clock:                  env.clock,
				CodexVersionDiscoverer: testCodexVersionDiscoverer,
			})
			if err != nil {
				t.Fatalf("run once: %v", err)
			}
			if result.Task.ID != tt.wantSelected || result.FileTask.ID != tt.wantSelected || result.Schedule.SelectedNext == nil || result.Schedule.SelectedNext.TaskID != tt.wantSelected {
				t.Fatalf("selection result = task:%q file:%q schedule:%#v, want %q", result.Task.ID, result.FileTask.ID, result.Schedule.SelectedNext, tt.wantSelected)
			}
			if codexCalls != 1 {
				t.Fatalf("Codex calls = %d, want 1", codexCalls)
			}
		})
	}
}

func TestRunUnsatisfiedDependencyReasonsStartNoCodex(t *testing.T) {
	tests := []struct {
		name         string
		state        taskscheduler.State
		wantReason   taskscheduler.Reason
		wantTerminal bool
	}{
		{name: "running", state: taskscheduler.StateRunning, wantReason: taskscheduler.ReasonWaitingDependency},
		{name: "blocked", state: taskscheduler.StateBlocked, wantReason: taskscheduler.ReasonBlockedDependency},
		{name: "needs input", state: taskscheduler.StateNeedsInput, wantReason: taskscheduler.ReasonNeedsInputDependency},
		{name: "cancelled", state: taskscheduler.StateCancelled, wantReason: taskscheduler.ReasonTerminalUnsatisfiedDependency, wantTerminal: true},
		{name: "abandoned", state: taskscheduler.StateAbandoned, wantReason: taskscheduler.ReasonTerminalUnsatisfiedDependency, wantTerminal: true},
		{name: "superseded", state: taskscheduler.StateSuperseded, wantReason: taskscheduler.ReasonTerminalUnsatisfiedDependency, wantTerminal: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestEnv(t)
			if tt.state == taskscheduler.StateNeedsInput {
				writeAutonomousSchedulingTask(t, env, "dependency", autonomous.LifecycleStateNeedsInput)
			} else {
				writeRunTaskFile(t, env, "020-dependency.md", taskFileMarkdownWithScheduling("dependency", "Dependency", string(tt.state), taskfile.PhaseImplement, ptrInt(50), nil))
			}
			writeRunTaskFile(t, env, "010-dependent.md", taskFileMarkdownWithScheduling("dependent", "Dependent", taskfile.StatusPending, taskfile.PhaseImplement, ptrInt(1), []string{"dependency"}))
			codexCalls := 0
			result, err := Run(context.Background(), Config{
				WorkingDir:  env.workDir,
				LedgerStore: env.ledger,
				CodexRunner: func(context.Context, codexexec.Config) (codexexec.Result, error) {
					codexCalls++
					return codexexec.Result{}, nil
				},
				Clock: env.clock,
			})
			if tt.wantTerminal {
				var terminalErr TerminalDependencyError
				if !errors.As(err, &terminalErr) || result.Outcome != OutcomeBlocked || len(terminalErr.Tasks) != 1 {
					t.Fatalf("terminal result=%+v error=%v typed=%+v", result, err, terminalErr)
				}
			} else if err != nil || !result.NoTask || result.Outcome != OutcomeNoTask {
				t.Fatalf("waiting result=%+v error=%v", result, err)
			}
			if codexCalls != 0 || result.Run.ID != "" {
				t.Fatalf("Codex calls=%d run=%q, want no runner", codexCalls, result.Run.ID)
			}
			got := scheduleTaskResult(t, result.Schedule, "dependent")
			if got.Reason != tt.wantReason || !reflect.DeepEqual(got.UnmetDependencyIDs, []string{"dependency"}) {
				t.Fatalf("dependent readiness = %#v", got)
			}
		})
	}
}

func TestRunInvalidGraphStartsNoCodex(t *testing.T) {
	tests := []struct {
		name  string
		setup func(testEnv)
		code  taskscheduler.DiagnosticCode
	}{
		{name: "missing dependency", code: taskscheduler.DiagnosticMissingDependency, setup: func(env testEnv) {
			writeRunTaskFile(t, env, "task.md", taskFileMarkdownWithScheduling("task", "Task", taskfile.StatusPending, taskfile.PhaseImplement, nil, []string{"missing"}))
		}},
		{name: "cycle", code: taskscheduler.DiagnosticDependencyCycle, setup: func(env testEnv) {
			writeRunTaskFile(t, env, "a.md", taskFileMarkdownWithScheduling("a", "A", taskfile.StatusPending, taskfile.PhaseImplement, nil, []string{"b"}))
			writeRunTaskFile(t, env, "b.md", taskFileMarkdownWithScheduling("b", "B", taskfile.StatusPending, taskfile.PhaseImplement, nil, []string{"a"}))
		}},
		{name: "duplicate task id", code: taskscheduler.DiagnosticDuplicateTaskID, setup: func(env testEnv) {
			writeRunTaskFile(t, env, "a.md", taskFileMarkdownWithScheduling("duplicate", "A", taskfile.StatusPending, taskfile.PhaseImplement, nil, nil))
			writeRunTaskFile(t, env, "b.md", taskFileMarkdownWithScheduling("duplicate", "B", taskfile.StatusPending, taskfile.PhaseImplement, nil, nil))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestEnv(t)
			tt.setup(env)
			codexCalls := 0
			result, err := Run(context.Background(), Config{
				WorkingDir:  env.workDir,
				LedgerStore: env.ledger,
				CodexRunner: func(context.Context, codexexec.Config) (codexexec.Result, error) {
					codexCalls++
					return codexexec.Result{}, nil
				},
				Clock: env.clock,
			})
			var scheduleErr ScheduleError
			if !errors.As(err, &scheduleErr) || result.Outcome != OutcomeBlocked || result.Schedule.Valid() {
				t.Fatalf("result=%+v error=%v typed=%+v", result, err, scheduleErr)
			}
			found := false
			for _, diagnostic := range scheduleErr.Diagnostics {
				found = found || diagnostic.Code == tt.code
			}
			if !found {
				t.Fatalf("diagnostics = %#v, want %q", scheduleErr.Diagnostics, tt.code)
			}
			if codexCalls != 0 || result.Run.ID != "" {
				t.Fatalf("Codex calls=%d run=%q, want no runner", codexCalls, result.Run.ID)
			}
		})
	}
}

func TestRunUnsupportedCanonicalFrontmatterStartsNoCodex(t *testing.T) {
	env := newTestEnv(t)
	path := filepath.Join(env.workDir, taskfile.TasksDir, "typo.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create task directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(`---
id: typo-task
status: pending
depend_on: prerequisite
---
# Typo Task
`), 0o644); err != nil {
		t.Fatalf("write typo task: %v", err)
	}
	codexCalls := 0
	result, err := Run(context.Background(), Config{
		WorkingDir:  env.workDir,
		LedgerStore: env.ledger,
		CodexRunner: func(context.Context, codexexec.Config) (codexexec.Result, error) {
			codexCalls++
			return codexexec.Result{}, nil
		},
		Clock: env.clock,
	})
	want := `unsupported frontmatter key "depend_on" at .agent/tasks/typo.md:4`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("run error = %v, want %q", err, want)
	}
	if codexCalls != 0 || result.Run.ID != "" || result.Outcome != OutcomeBlocked {
		t.Fatalf("result=%+v codex_calls=%d, want pre-selection block with no run", result, codexCalls)
	}
}

func TestRunSelectsOnlyMixedPassTasks(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	autonomous := writeRunTaskFile(t, env, "001-autonomous.md", `---
id: task-autonomous
status: pending
workflow: autonomous-v1
autonomous_state_path: .revolvr/autonomous/tasks/task-autonomous/state.json
priority: 1
---
# Autonomous Task

Do not run through mixed-pass.
`)
	mixed := writeRunTaskFile(t, env, "010-mixed.md", `---
id: task-mixed
status: pending
workflow: mixed-pass-v1
phase: implement
priority: 2
---
# Mixed Task

Run through mixed-pass.
`)
	codexCalls := 0
	result, err := Run(ctx, Config{
		WorkingDir:     env.workDir,
		LedgerStore:    env.ledger,
		DirtyCapture:   cleanDirtyCapture,
		ChangedCapture: emptyChangedCapture,
		CodexRunner: func(context.Context, codexexec.Config) (codexexec.Result, error) {
			codexCalls++
			return codexexec.Result{ExitCode: 1}, nil
		},
		Clock:                  env.clock,
		CodexVersionDiscoverer: testCodexVersionDiscoverer,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if result.Task.ID != mixed.ID || result.FileTask.Workflow != taskfile.WorkflowMixedPassV1 || codexCalls != 1 {
		t.Fatalf("result = %+v, codex calls = %d; want selected mixed-pass task", result, codexCalls)
	}
	if got := loadRunTask(t, env, autonomous.SourcePath); got.Status != taskfile.StatusPending || got.Workflow != taskfile.WorkflowAutonomousV1 {
		t.Fatalf("autonomous task after run = %+v, want untouched pending autonomous task", got)
	}
	if got := loadRunTask(t, env, mixed.SourcePath); got.Status != taskfile.StatusBlocked {
		t.Fatalf("mixed task status = %q, want blocked after failed Codex pass", got.Status)
	}
}

func TestRunSelectionScopeDoesNotPromoteAutonomousTerminalBlocker(t *testing.T) {
	env := newTestEnv(t)
	writeRunTaskFile(t, env, "001-cancelled.md", `---
id: autonomous-cancelled
status: cancelled
workflow: autonomous-v1
autonomous_state_path: .revolvr/autonomous/tasks/autonomous-cancelled/state.json
---
# Cancelled Autonomous Task
`)
	writeRunTaskFile(t, env, "002-dependent.md", `---
id: autonomous-dependent
status: pending
workflow: autonomous-v1
autonomous_state_path: .revolvr/autonomous/tasks/autonomous-dependent/state.json
depends_on: autonomous-cancelled
---
# Autonomous Dependent
`)
	codexCalls := 0
	result, err := Run(context.Background(), Config{
		WorkingDir:  env.workDir,
		LedgerStore: env.ledger,
		CodexRunner: func(context.Context, codexexec.Config) (codexexec.Result, error) {
			codexCalls++
			return codexexec.Result{}, nil
		},
		Clock: env.clock,
	})
	if err != nil || !result.NoTask || result.Outcome != OutcomeNoTask {
		t.Fatalf("result=%+v error=%v, want ordinary empty mixed selection", result, err)
	}
	if codexCalls != 0 || len(result.Schedule.TerminalUnsatisfied) != 1 || len(result.Schedule.SelectionTerminalUnsatisfied) != 0 {
		t.Fatalf("Codex calls=%d schedule=%+v", codexCalls, result.Schedule)
	}
}

func TestRunSecondPassAfterCompletionReturnsNoTask(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	writeTestRunProfile(t, env.workDir, "simplifier", "Simplifier test profile.\n\nComplete the final phase.")
	writeRunTaskWithPhase(t, env, "task-once", "Do one task", taskfile.PhaseSimplify)

	state := &fakeCommandState{
		t:                 t,
		workDir:           env.workDir,
		writeReceipt:      true,
		postStatus:        " M internal/feature.go\x00",
		verificationExit:  0,
		commitSHA:         "abc123def456",
		expectedCommitAdd: []string{"--literal-pathspecs", "add", "--", "internal/feature.go"},
	}
	first, err := Run(ctx, Config{
		WorkingDir:           env.workDir,
		LedgerStore:          env.ledger,
		CodexExecutable:      "codex-test",
		GitExecutable:        "git-test",
		VerificationCommands: []verification.Command{{Name: "go", Args: []string{"test", "./..."}}},
		CommandRunner:        state.run,
		Clock:                env.clock,
	})
	if err != nil {
		t.Fatalf("first run once: %v", err)
	}
	if first.Outcome != OutcomeCommitted {
		t.Fatalf("first outcome = %s, want committed", first.Outcome)
	}

	secondState := &fakeCommandState{t: t, workDir: env.workDir}
	second, err := Run(ctx, Config{
		WorkingDir:    env.workDir,
		LedgerStore:   env.ledger,
		CommandRunner: secondState.run,
		Clock:         env.clock,
	})
	if err != nil {
		t.Fatalf("second run once: %v", err)
	}
	if !second.NoTask || second.Outcome != OutcomeNoTask {
		t.Fatalf("second result = %+v, want no task", second)
	}
	if len(secondState.commands) != 0 {
		t.Fatalf("second commands = %#v, want none", secondState.commands)
	}
}

func TestRunReevaluatesDependenciesAfterSuccessfulPrerequisiteCommit(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	runTestGit(t, workDir, "init", "-q")
	runTestGit(t, workDir, "config", "user.name", "Revolvr Test")
	runTestGit(t, workDir, "config", "user.email", "revolvr-test@example.invalid")
	if err := os.WriteFile(filepath.Join(workDir, ".git", "info", "exclude"), []byte("/.revolvr/\n"), 0o644); err != nil {
		t.Fatalf("write git exclude: %v", err)
	}

	now := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	ledgerPath := filepath.Join(t.TempDir(), "ledger.sqlite")
	runs, err := ledger.OpenWithClock(ctx, ledgerPath, func() time.Time { return now })
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	t.Cleanup(func() { _ = runs.Close() })
	env := testEnv{workDir: workDir, ledger: runs, now: now}

	if err := os.WriteFile(filepath.Join(workDir, "README.md"), []byte("# Test repo\n"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	writeTestRunProfile(t, workDir, prompt.DefaultRunProfileName, defaultRunProfileTemplateContent(t))
	writeTestRunProfile(t, workDir, "simplifier", "Simplifier test profile.\n\nComplete the final phase.")
	firstTask := writeRunTaskFile(t, env, "010-first.md", taskFileMarkdownWithScheduling("task-first", "First Task", taskfile.StatusPending, taskfile.PhaseSimplify, ptrInt(50), nil))
	secondTask := writeRunTaskFile(t, env, "020-second.md", taskFileMarkdownWithScheduling("task-second", "Second Task", taskfile.StatusPending, taskfile.PhaseSimplify, ptrInt(1), []string{"task-first"}))
	runTestGit(t, workDir, "add", ".")
	runTestGit(t, workDir, "commit", "-q", "-m", "Initial file tasks")

	var codexCalls int
	cfg := Config{
		WorkingDir:             workDir,
		LedgerStore:            runs,
		CodexVersionDiscoverer: testCodexVersionDiscoverer,
		CodexRunner: func(_ context.Context, cfg codexexec.Config) (codexexec.Result, error) {
			codexCalls++
			path := filepath.Join(cfg.WorkingDir, fmt.Sprintf("generated-%d.txt", codexCalls))
			if err := os.WriteFile(path, []byte(fmt.Sprintf("generated %d\n", codexCalls)), 0o644); err != nil {
				return codexexec.Result{}, err
			}
			return codexexec.Result{ExitCode: 0, FinalMessage: "done"}, nil
		},
		VerificationRunner: func(context.Context, verification.Config) (verification.Result, error) {
			return passedVerificationResult("go test ./..."), nil
		},
		Clock: func() time.Time { return now.Add(2 * time.Minute) },
	}

	first, err := Run(ctx, cfg)
	if err != nil {
		t.Fatalf("first run once: %v", err)
	}
	if first.Outcome != OutcomeCommitted {
		t.Fatalf("first outcome = %s, want committed; message=%s", first.Outcome, first.Message)
	}
	if first.Task.ID != "task-first" || first.Schedule.SelectedNext == nil || first.Schedule.SelectedNext.TaskID != "task-first" {
		t.Fatalf("first selection = task:%q schedule:%#v, want prerequisite", first.Task.ID, first.Schedule.SelectedNext)
	}
	if got := loadRunTask(t, env, firstTask.SourcePath).Status; got != taskfile.StatusCompleted {
		t.Fatalf("first file task status = %q, want completed", got)
	}
	assertGitStatusClean(t, workDir)

	second, err := Run(ctx, cfg)
	if err != nil {
		t.Fatalf("second run once: %v", err)
	}
	if second.Outcome != OutcomeCommitted {
		t.Fatalf("second outcome = %s, want committed; message=%s", second.Outcome, second.Message)
	}
	if second.Task.ID != "task-second" {
		t.Fatalf("second selected task = %q, want task-second", second.Task.ID)
	}
	if second.Schedule.SelectedNext == nil || second.Schedule.SelectedNext.TaskID != "task-second" {
		t.Fatalf("second schedule selection = %#v, want unlocked dependent", second.Schedule.SelectedNext)
	}
	if got := loadRunTask(t, env, secondTask.SourcePath).Status; got != taskfile.StatusCompleted {
		t.Fatalf("second file task status = %q, want completed", got)
	}
	assertGitStatusClean(t, workDir)
}

func TestRunWritesContextBundleWithDefaultProfile(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	profileContent := "File-backed implementer profile.\n\nUse the markdown from .agent/profiles."
	writeTestRunProfile(t, env.workDir, prompt.DefaultRunProfileName, profileContent)
	selected := writeRunTask(t, env, "task-profile", "Use the default run profile")

	var runnerPayload string
	result, err := Run(ctx, Config{
		WorkingDir:     env.workDir,
		LedgerStore:    env.ledger,
		DirtyCapture:   cleanDirtyCapture,
		ChangedCapture: emptyChangedCapture,
		CodexRunner: func(_ context.Context, cfg codexexec.Config) (codexexec.Result, error) {
			runnerPayload = cfg.Prompt
			return codexexec.Result{ExitCode: 1}, nil
		},
		Clock:                  env.clock,
		CodexVersionDiscoverer: testCodexVersionDiscoverer,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if result.Outcome != OutcomeCodexFailed {
		t.Fatalf("outcome = %s, want codex_failed", result.Outcome)
	}

	contextPayloadPath := filepath.Join(env.workDir, ".revolvr", "runs", result.Run.ID, "context.md")
	payloadBytes, err := os.ReadFile(contextPayloadPath)
	if err != nil {
		t.Fatalf("read context payload artifact: %v", err)
	}
	got := string(payloadBytes)
	if got != runnerPayload {
		t.Fatalf("context payload artifact differs from runner payload\n--- artifact ---\n%s\n--- runner ---\n%s", got, runnerPayload)
	}
	required := []string{
		"## Run Profile",
		profileContent,
		"## Selected Task",
		"Task ID: `task-profile`",
		"## Repository Rules",
		"## Artifact Paths",
		"## Required Receipt Schema",
		"## Stop Condition",
	}
	for _, want := range required {
		if !strings.Contains(got, want) {
			t.Fatalf("context payload artifact missing %q:\n%s", want, got)
		}
	}

	contextManifestPath := filepath.Join(env.workDir, ".revolvr", "runs", result.Run.ID, "context.json")
	manifestBytes, err := os.ReadFile(contextManifestPath)
	if err != nil {
		t.Fatalf("read context manifest artifact: %v", err)
	}
	var manifest prompt.ContextManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("unmarshal context manifest: %v\n%s", err, manifestBytes)
	}
	if manifest.RunID != result.Run.ID || manifest.TaskID != "task-profile" || manifest.ProfileName != prompt.DefaultRunProfileName {
		t.Fatalf("manifest identity = %+v, want run/task/profile", manifest)
	}
	payloadRel := filepath.Join(".revolvr", "runs", result.Run.ID, "context.md")
	if got, want := manifest.ContextPayloadPath, payloadRel; got != want {
		t.Fatalf("manifest payload path = %q, want %q", got, want)
	}
	if got, want := manifest.ContextPayloadSHA256, sha256HexTest(payloadBytes); got != want {
		t.Fatalf("manifest payload sha256 = %q, want %q", got, want)
	}
	if got, want := manifest.ContextPayloadByteSize, len(payloadBytes); got != want {
		t.Fatalf("manifest payload byte size = %d, want %d", got, want)
	}
	if got, want := manifest.GeneratedAt, env.clock().UTC(); !got.Equal(want) {
		t.Fatalf("manifest generated_at = %s, want %s", got, want)
	}
	selectedTask := manifestSourceByLabel(t, manifest, "selected_task")
	if got, want := selectedTask.Path, selected.SourcePath; got != want {
		t.Fatalf("selected task path = %q, want %q", got, want)
	}
	if got, want := selectedTask.SHA256, sha256HexTest(selected.SourceBytes); got != want {
		t.Fatalf("selected task sha256 = %q, want %q", got, want)
	}
	if got, want := selectedTask.ByteSize, len(selected.SourceBytes); got != want {
		t.Fatalf("selected task byte size = %d, want %d", got, want)
	}
	runProfile := manifestSourceByLabel(t, manifest, "run_profile")
	if got, want := runProfile.Path, filepath.Join(".agent", "profiles", "implementer.md"); got != want {
		t.Fatalf("run profile path = %q, want %q", got, want)
	}
	if got, want := runProfile.SHA256, sha256HexTest([]byte(profileContent)); got != want {
		t.Fatalf("run profile sha256 = %q, want %q", got, want)
	}
	if got, want := runProfile.ByteSize, len([]byte(profileContent)); got != want {
		t.Fatalf("run profile byte size = %d, want %d", got, want)
	}

	history, ok, err := env.ledger.GetRunWithEvents(ctx, result.Run.ID)
	if err != nil || !ok {
		t.Fatalf("get run history ok=%v err=%v", ok, err)
	}
	artifacts, found := ledger.RunArtifactsFromEvents(history.Events)
	if !found {
		t.Fatal("run artifacts not found")
	}
	if artifacts.ContextPayloadPath != payloadRel {
		t.Fatalf("ledger context payload path = %q, want %q", artifacts.ContextPayloadPath, payloadRel)
	}
	manifestRel := filepath.Join(".revolvr", "runs", result.Run.ID, "context.json")
	if artifacts.ContextManifestPath != manifestRel {
		t.Fatalf("ledger context manifest path = %q, want %q", artifacts.ContextManifestPath, manifestRel)
	}
}

func TestRunBlocksWhenCodexVersionDiscoveryFails(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	writeRunTask(t, env, "task-version-failure", "Require Codex version provenance")
	codexCalled := false
	var versionConfig codexexec.VersionConfig
	result, err := Run(ctx, Config{
		WorkingDir:     env.workDir,
		LedgerStore:    env.ledger,
		DirtyCapture:   cleanDirtyCapture,
		ChangedCapture: emptyChangedCapture,
		CodexVersionDiscoverer: func(_ context.Context, cfg codexexec.VersionConfig) (string, error) {
			versionConfig = cfg
			return "", errors.New("version output is malformed")
		},
		CodexRunner: func(context.Context, codexexec.Config) (codexexec.Result, error) {
			codexCalled = true
			return codexexec.Result{}, nil
		},
		Clock: env.clock,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if result.Outcome != OutcomeBlocked || codexCalled || !strings.Contains(result.Message, "discover Codex version failed") {
		t.Fatalf("result=%+v codex_called=%v", result, codexCalled)
	}
	if versionConfig.Executable != codexexec.DefaultExecutable || versionConfig.WorkingDir != env.workDir || versionConfig.Timeout != codexexec.DefaultVersionTimeout || versionConfig.StdoutCap != defaultOutputCap || versionConfig.StderrCap != defaultOutputCap {
		t.Fatalf("version config = %+v", versionConfig)
	}
	if _, statErr := os.Stat(filepath.Join(env.workDir, ".revolvr", "runs", result.Run.ID, "context.json")); !os.IsNotExist(statErr) {
		t.Fatalf("context manifest stat error = %v, want not exist", statErr)
	}
}

func TestRunLoadsProfileForSelectedTaskPhase(t *testing.T) {
	tests := []struct {
		name           string
		phase          string
		profileName    string
		profileContent string
	}{
		{
			name:           "default implement metadata",
			profileName:    prompt.DefaultRunProfileName,
			profileContent: "Implementer profile selected by default task metadata.\n\nMake the focused change.",
		},
		{
			name:           "explicit implement phase",
			phase:          taskfile.PhaseImplement,
			profileName:    prompt.DefaultRunProfileName,
			profileContent: "Implementer profile selected by explicit implement phase.\n\nMake the focused change.",
		},
		{
			name:           "audit phase",
			phase:          taskfile.PhaseAudit,
			profileName:    "auditor",
			profileContent: "Auditor profile selected by audit phase.\n\nReview the completed work.",
		},
		{
			name:           "document phase",
			phase:          taskfile.PhaseDocument,
			profileName:    "documentor",
			profileContent: "Documentor profile selected by document phase.\n\nUpdate operator-facing docs.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			env := newTestEnv(t)
			writeTestRunProfile(t, env.workDir, tt.profileName, tt.profileContent)

			var selected taskfile.Task
			if tt.phase == "" {
				selected = writeRunTask(t, env, "task-"+tt.profileName+"-default", "Use the default phase")
			} else {
				selected = writeRunTaskWithPhase(t, env, "task-"+tt.profileName+"-"+tt.phase, "Use "+tt.phase+" phase", tt.phase)
			}

			codexCalled := false
			var runnerPayload string
			result, err := Run(ctx, Config{
				WorkingDir:     env.workDir,
				LedgerStore:    env.ledger,
				DirtyCapture:   cleanDirtyCapture,
				ChangedCapture: emptyChangedCapture,
				CodexRunner: func(_ context.Context, cfg codexexec.Config) (codexexec.Result, error) {
					codexCalled = true
					runnerPayload = cfg.Prompt
					return codexexec.Result{ExitCode: 1}, nil
				},
				Clock:                  env.clock,
				CodexVersionDiscoverer: testCodexVersionDiscoverer,
			})
			if err != nil {
				t.Fatalf("run once: %v", err)
			}
			if result.Outcome != OutcomeCodexFailed {
				t.Fatalf("outcome = %s, want codex_failed", result.Outcome)
			}
			if !codexCalled {
				t.Fatal("codex runner was not called")
			}

			loadedProfileContent := strings.TrimSpace(tt.profileContent)
			if !strings.Contains(runnerPayload, loadedProfileContent) {
				t.Fatalf("context payload missing selected profile content %q:\n%s", loadedProfileContent, runnerPayload)
			}

			manifest := readContextManifestArtifact(t, env, result.Run.ID)
			if got := manifest.ProfileName; got != tt.profileName {
				t.Fatalf("manifest profile name = %q, want %q", got, tt.profileName)
			}
			runProfile := manifestSourceByLabel(t, manifest, "run_profile")
			if got, want := runProfile.Path, prompt.RunProfileSourcePath(tt.profileName); got != want {
				t.Fatalf("run profile source path = %q, want %q", got, want)
			}
			if got, want := runProfile.SHA256, sha256HexTest([]byte(loadedProfileContent)); got != want {
				t.Fatalf("run profile sha256 = %q, want %q", got, want)
			}
			if got, want := runProfile.ByteSize, len([]byte(loadedProfileContent)); got != want {
				t.Fatalf("run profile byte size = %d, want %d", got, want)
			}

			var taskSelected struct {
				Workflow    string `json:"workflow"`
				Phase       string `json:"phase"`
				ProfileName string `json:"profile_name"`
			}
			history, ok, err := env.ledger.GetRunWithEvents(ctx, result.Run.ID)
			if err != nil || !ok {
				t.Fatalf("get run history ok=%v err=%v", ok, err)
			}
			if !decodeTestEventPayload(t, history.Events, ledger.EventTaskSelected, &taskSelected) {
				t.Fatal("task selected event not found")
			}
			if got, want := taskSelected.Workflow, taskfile.WorkflowMixedPassV1; got != want {
				t.Fatalf("task selected workflow = %q, want %q", got, want)
			}
			wantPhase := tt.phase
			if wantPhase == "" {
				wantPhase = taskfile.PhaseImplement
			}
			if got := taskSelected.Phase; got != wantPhase {
				t.Fatalf("task selected phase = %q, want %q", got, wantPhase)
			}
			if got := taskSelected.ProfileName; got != tt.profileName {
				t.Fatalf("task selected profile name = %q, want %q", got, tt.profileName)
			}
			selectedTask := manifestSourceByLabel(t, manifest, "selected_task")
			if got, want := selectedTask.Path, selected.SourcePath; got != want {
				t.Fatalf("selected task source path = %q, want %q", got, want)
			}
		})
	}
}

func TestRunTaskFrontmatterProfileDoesNotOverridePhasePolicy(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	writeTestRunProfile(t, env.workDir, "auditor", "UNIQUE FRONTMATTER AUDITOR PROFILE")
	selected := writeRunTaskFile(t, env, "profile-override.md", `---
id: task-profile-override
status: pending
workflow: mixed-pass-v1
phase: implement
profile: auditor
---
# Profile Override

Use the policy-selected profile.
`)

	var payload string
	result, err := Run(ctx, Config{
		WorkingDir:     env.workDir,
		LedgerStore:    env.ledger,
		DirtyCapture:   cleanDirtyCapture,
		ChangedCapture: emptyChangedCapture,
		CodexRunner: func(_ context.Context, cfg codexexec.Config) (codexexec.Result, error) {
			payload = cfg.Prompt
			return codexexec.Result{ExitCode: 1}, nil
		},
		Clock:                  env.clock,
		CodexVersionDiscoverer: testCodexVersionDiscoverer,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if result.Outcome != OutcomeCodexFailed {
		t.Fatalf("outcome = %s, want codex_failed", result.Outcome)
	}
	if !strings.Contains(payload, "You are the implementer for this Revolvr pass.") {
		t.Fatalf("context payload missing implementer policy profile:\n%s", payload)
	}
	if strings.Contains(payload, "UNIQUE FRONTMATTER AUDITOR PROFILE") {
		t.Fatalf("context payload used task frontmatter profile:\n%s", payload)
	}
	history, ok, historyErr := env.ledger.GetRunWithEvents(ctx, result.Run.ID)
	if historyErr != nil || !ok {
		t.Fatalf("get run history ok=%v err=%v", ok, historyErr)
	}
	var taskSelected struct {
		TaskID      string `json:"task_id"`
		ProfileName string `json:"profile_name"`
	}
	if !decodeTestEventPayload(t, history.Events, ledger.EventTaskSelected, &taskSelected) {
		t.Fatal("task selected event not found")
	}
	if taskSelected.TaskID != selected.ID || taskSelected.ProfileName != "implementer" {
		t.Fatalf("task selected payload = %+v, want stable task and implementer", taskSelected)
	}
}

func TestRunSmokeScriptTaskFileManifestUsesExactPreRunBytes(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	selected := writeRunTaskFile(t, env, "001-fake-codex-smoke.md", strings.Join([]string{
		"---",
		"id: smoke-fake-codex",
		"status: pending",
		"priority: 10",
		"---",
		"# Fake Codex run smoke",
		"",
		"Run once success path through strict fake Codex",
		"",
	}, "\n"))
	preRunBytes := append([]byte(nil), selected.SourceBytes...)

	result, err := Run(ctx, Config{
		WorkingDir:     env.workDir,
		LedgerStore:    env.ledger,
		DirtyCapture:   cleanDirtyCapture,
		ChangedCapture: emptyChangedCapture,
		CodexRunner: func(context.Context, codexexec.Config) (codexexec.Result, error) {
			return codexexec.Result{ExitCode: 1}, nil
		},
		Clock:                  env.clock,
		CodexVersionDiscoverer: testCodexVersionDiscoverer,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if result.Outcome != OutcomeCodexFailed {
		t.Fatalf("outcome = %s, want codex_failed", result.Outcome)
	}
	if got := loadRunTask(t, env, selected.SourcePath).Status; got != taskfile.StatusBlocked {
		t.Fatalf("file task status = %q, want blocked", got)
	}

	contextManifestPath := filepath.Join(env.workDir, ".revolvr", "runs", result.Run.ID, "context.json")
	manifestBytes, err := os.ReadFile(contextManifestPath)
	if err != nil {
		t.Fatalf("read context manifest artifact: %v", err)
	}
	var manifest prompt.ContextManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("unmarshal context manifest: %v\n%s", err, manifestBytes)
	}
	selectedTask := manifestSourceByLabel(t, manifest, "selected_task")
	if got, want := selectedTask.Path, selected.SourcePath; got != want {
		t.Fatalf("selected task path = %q, want %q", got, want)
	}
	if got, want := selectedTask.SHA256, sha256HexTest(preRunBytes); got != want {
		t.Fatalf("selected task sha256 = %q, want pre-run %q", got, want)
	}
	if got, want := selectedTask.ByteSize, len(preRunBytes); got != want {
		t.Fatalf("selected task byte size = %d, want pre-run %d", got, want)
	}
}

func TestRunBlocksBeforeCodexWhenMappedProfileMissing(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	selected := writeRunTaskWithPhase(t, env, "task-missing-profile", "Require a phase-selected profile file", taskfile.PhaseSimplify)

	codexCalled := false
	result, err := Run(ctx, Config{
		WorkingDir:     env.workDir,
		LedgerStore:    env.ledger,
		DirtyCapture:   cleanDirtyCapture,
		ChangedCapture: emptyChangedCapture,
		CodexRunner: func(context.Context, codexexec.Config) (codexexec.Result, error) {
			codexCalled = true
			return codexexec.Result{ExitCode: 0}, nil
		},
		Clock:                  env.clock,
		CodexVersionDiscoverer: testCodexVersionDiscoverer,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}

	if result.Outcome != OutcomeBlocked {
		t.Fatalf("outcome = %s, want blocked", result.Outcome)
	}
	if codexCalled {
		t.Fatal("codex runner was called, want profile load failure before Codex starts")
	}
	for _, want := range []string{
		"load run profile failed",
		filepath.Join(".agent", "profiles", "simplifier.md"),
		"run `revolvr init` or create the profile file",
	} {
		if !strings.Contains(result.Message, want) {
			t.Fatalf("message missing %q:\n%s", want, result.Message)
		}
	}
	contextPayloadPath := filepath.Join(env.workDir, ".revolvr", "runs", result.Run.ID, "context.md")
	if _, err := os.Stat(contextPayloadPath); !os.IsNotExist(err) {
		t.Fatalf("context payload artifact stat err = %v, want not exist", err)
	}
	contextManifestPath := filepath.Join(env.workDir, ".revolvr", "runs", result.Run.ID, "context.json")
	if _, err := os.Stat(contextManifestPath); !os.IsNotExist(err) {
		t.Fatalf("context manifest artifact stat err = %v, want not exist", err)
	}
	if got := loadRunTask(t, env, selected.SourcePath).Status; got != taskfile.StatusBlocked {
		t.Fatalf("file task status = %q, want blocked", got)
	}
	if got := loadRunTask(t, env, selected.SourcePath).Phase; got != taskfile.PhaseSimplify {
		t.Fatalf("file task phase = %q, want simplify", got)
	}
}

func TestRunRecordsChangedFilesReceiptWarningWithoutBlockingCommit(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	writeRunTask(t, env, "task-warning-files", "Create a mismatched receipt")

	state := &fakeCommandState{
		t:            t,
		workDir:      env.workDir,
		writeReceipt: true,
		receiptContent: func(runID, taskID, task string) string {
			return receiptContent(runID, taskID, task, receiptOptions{ChangedFiles: []string{"internal/claimed.go"}})
		},
		postStatus:        " M internal/actual.go\x00",
		verificationExit:  0,
		commitSHA:         "abc123def456",
		expectedCommitAdd: []string{"--literal-pathspecs", "add", "--", "internal/actual.go"},
	}

	result, err := Run(ctx, Config{
		WorkingDir:           env.workDir,
		LedgerStore:          env.ledger,
		CodexExecutable:      "codex-test",
		GitExecutable:        "git-test",
		VerificationCommands: []verification.Command{{Name: "go", Args: []string{"test", "./..."}}},
		CommandRunner:        state.run,
		Clock:                env.clock,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}

	if result.Outcome != OutcomeCommitted {
		t.Fatalf("outcome = %s, want committed; message=%s", result.Outcome, result.Message)
	}
	if result.Task.Status != taskmodel.StatusPending || result.Run.Status != ledger.StatusCompleted {
		t.Fatalf("task/run status = %s/%s, want pending/completed", result.Task.Status, result.Run.Status)
	}

	warnings := receiptWarningEvents(t, env.ledger, result.Run.ID)
	if len(warnings) != 1 {
		t.Fatalf("warning count = %d, want 1", len(warnings))
	}
	var payload struct {
		WarningType string   `json:"warning_type"`
		Message     string   `json:"message"`
		ReceiptPath string   `json:"receipt_path"`
		Claimed     []string `json:"claimed"`
		Observed    []string `json:"observed"`
	}
	decodeEventPayload(t, warnings[0], &payload)
	if payload.WarningType != receiptWarningChangedFiles {
		t.Fatalf("warning type = %q, want %q", payload.WarningType, receiptWarningChangedFiles)
	}
	if payload.Message != changedFilesWarningMessage {
		t.Fatalf("warning message = %q, want %q", payload.Message, changedFilesWarningMessage)
	}
	if got, want := payload.ReceiptPath, filepath.Join(".revolvr", "receipts", result.Run.ID+".md"); got != want {
		t.Fatalf("receipt path = %q, want %q", got, want)
	}
	if got, want := payload.Claimed, []string{"internal/claimed.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("claimed files = %#v, want %#v", got, want)
	}
	if got, want := payload.Observed, []string{"internal/actual.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("observed files = %#v, want %#v", got, want)
	}
}

func TestRunRecordsVerificationReceiptWarningWithoutBlockingCommit(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	writeRunTask(t, env, "task-warning-verification", "Claim wrong verification")

	state := &fakeCommandState{
		t:            t,
		workDir:      env.workDir,
		writeReceipt: true,
		receiptContent: func(runID, taskID, task string) string {
			return receiptContent(runID, taskID, task, receiptOptions{
				ChangedFiles:       []string{"internal/feature.go"},
				VerificationStatus: "failed",
				Verification: []receipt.VerificationEntry{{
					Command:  "go test ./...",
					ExitCode: 1,
					Status:   "failed",
				}},
			})
		},
		postStatus:       " M internal/feature.go\x00",
		verificationExit: 0,
		commitSHA:        "abc123def456",
	}

	result, err := Run(ctx, Config{
		WorkingDir:           env.workDir,
		LedgerStore:          env.ledger,
		CodexExecutable:      "codex-test",
		GitExecutable:        "git-test",
		VerificationCommands: []verification.Command{{Name: "go", Args: []string{"test", "./..."}}},
		CommandRunner:        state.run,
		Clock:                env.clock,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}

	if result.Outcome != OutcomeCommitted {
		t.Fatalf("outcome = %s, want committed; message=%s", result.Outcome, result.Message)
	}
	warnings := receiptWarningEvents(t, env.ledger, result.Run.ID)
	if len(warnings) != 1 {
		t.Fatalf("warning count = %d, want 1", len(warnings))
	}
	var payload struct {
		WarningType string            `json:"warning_type"`
		Message     string            `json:"message"`
		Claimed     verificationFacts `json:"claimed"`
		Observed    verificationFacts `json:"observed"`
	}
	decodeEventPayload(t, warnings[0], &payload)
	if payload.WarningType != receiptWarningVerification {
		t.Fatalf("warning type = %q, want %q", payload.WarningType, receiptWarningVerification)
	}
	if payload.Message != verificationWarningMessage {
		t.Fatalf("warning message = %q, want %q", payload.Message, verificationWarningMessage)
	}
	if payload.Claimed.Status != "failed" || payload.Observed.Status != "passed" {
		t.Fatalf("verification payload = %+v / %+v, want failed/passed", payload.Claimed, payload.Observed)
	}
}

func TestRunReceiptWarningsDoNotChangeFailedOutcome(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	writeRunTask(t, env, "task-warning-failed", "Fail despite receipt")
	state := &fakeCommandState{
		t:            t,
		workDir:      env.workDir,
		writeReceipt: true,
		receiptContent: func(runID, taskID, task string) string {
			return receiptContent(runID, taskID, task, receiptOptions{
				ChangedFiles:       []string{"internal/feature.go"},
				VerificationStatus: "passed",
				Verification: []receipt.VerificationEntry{{
					Command:  "go test ./...",
					ExitCode: 0,
					Status:   "passed",
				}},
			})
		},
		postStatus:       " M internal/feature.go\x00",
		verificationExit: 1,
	}

	result, err := Run(ctx, Config{
		WorkingDir:           env.workDir,
		LedgerStore:          env.ledger,
		CodexExecutable:      "codex-test",
		GitExecutable:        "git-test",
		VerificationCommands: []verification.Command{{Name: "go", Args: []string{"test", "./..."}}},
		CommandRunner:        state.run,
		Clock:                env.clock,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}

	if result.Outcome != OutcomeVerificationFailed {
		t.Fatalf("outcome = %s, want verification_failed", result.Outcome)
	}
	if result.Task.Status != taskmodel.StatusBlocked || result.Run.Status != ledger.StatusFailed {
		t.Fatalf("task/run status = %s/%s, want blocked/failed", result.Task.Status, result.Run.Status)
	}
	if state.gitAddOrCommitCalls != 0 {
		t.Fatalf("git add/commit calls = %d, want 0", state.gitAddOrCommitCalls)
	}
	warnings := receiptWarningEvents(t, env.ledger, result.Run.ID)
	if len(warnings) != 2 {
		t.Fatalf("warning count = %d, want verification and verdict warnings", len(warnings))
	}
}

func TestRunDoesNotWarnWhenReceiptFactsMatchHarness(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	writeRunTask(t, env, "task-no-warning", "Match receipt facts")
	state := &fakeCommandState{
		t:            t,
		workDir:      env.workDir,
		writeReceipt: true,
		receiptContent: func(runID, taskID, task string) string {
			return receiptContent(runID, taskID, task, receiptOptions{
				ChangedFiles:       []string{"internal/feature.go"},
				VerificationStatus: "passed",
				Verification: []receipt.VerificationEntry{{
					Command:  "go test ./...",
					ExitCode: 0,
					Status:   "passed",
				}},
			})
		},
		postStatus:       " M internal/feature.go\x00",
		verificationExit: 0,
		commitSHA:        "abc123def456",
	}

	result, err := Run(ctx, Config{
		WorkingDir:           env.workDir,
		LedgerStore:          env.ledger,
		CodexExecutable:      "codex-test",
		GitExecutable:        "git-test",
		VerificationCommands: []verification.Command{{Name: "go", Args: []string{"test", "./..."}}},
		CommandRunner:        state.run,
		Clock:                env.clock,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if result.Outcome != OutcomeCommitted {
		t.Fatalf("outcome = %s, want committed", result.Outcome)
	}
	if warnings := receiptWarningEvents(t, env.ledger, result.Run.ID); len(warnings) != 0 {
		t.Fatalf("warnings = %+v, want none", warnings)
	}
}

func TestRunBlocksWhenVerificationFailsAndSkipsCommit(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	writeTestRunProfile(t, env.workDir, "auditor", "Auditor profile for verification failure.\n\nReview before document.")
	selected := writeRunTaskWithPhase(t, env, "task-verify", "Break verification", taskfile.PhaseAudit)
	state := &fakeCommandState{
		t:                t,
		workDir:          env.workDir,
		postStatus:       " M internal/feature.go\x00",
		verificationExit: 1,
	}

	result, err := Run(ctx, Config{
		WorkingDir:           env.workDir,
		LedgerStore:          env.ledger,
		CodexExecutable:      "codex-test",
		GitExecutable:        "git-test",
		VerificationCommands: []verification.Command{{Name: "go", Args: []string{"test", "./..."}}},
		CommandRunner:        state.run,
		Clock:                env.clock,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}

	if result.Outcome != OutcomeVerificationFailed {
		t.Fatalf("outcome = %s, want verification_failed", result.Outcome)
	}
	if result.Task.Status != taskmodel.StatusBlocked {
		t.Fatalf("task status = %q, want blocked", result.Task.Status)
	}
	if got := loadRunTask(t, env, selected.SourcePath).Status; got != taskfile.StatusBlocked {
		t.Fatalf("file task status = %q, want blocked", got)
	}
	if got := loadRunTask(t, env, selected.SourcePath).Phase; got != taskfile.PhaseAudit {
		t.Fatalf("file task phase = %q, want audit", got)
	}
	if result.Run.Status != ledger.StatusFailed {
		t.Fatalf("run status = %q, want failed", result.Run.Status)
	}
	if result.Commit.Status != "" {
		t.Fatalf("commit result = %+v, want zero value", result.Commit)
	}
	if state.gitAddOrCommitCalls != 0 {
		t.Fatalf("git add/commit calls = %d, want 0", state.gitAddOrCommitCalls)
	}
	if !result.ReceiptSynthesized {
		t.Fatal("receipt synthesized = false, want fallback receipt")
	}
	if result.Receipt.Verdict != receipt.VerdictVerificationFailed {
		t.Fatalf("fallback verdict = %q, want verification_failed", result.Receipt.Verdict)
	}
	if warnings := receiptWarningEvents(t, env.ledger, result.Run.ID); len(warnings) != 0 {
		t.Fatalf("fallback warnings = %+v, want none", warnings)
	}
}

func TestRunBlocksWhenNoChangesAfterSuccessfulVerification(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	selected := writeRunTask(t, env, "task-no-change", "Make no changes")
	state := &fakeCommandState{
		t:                t,
		workDir:          env.workDir,
		postStatus:       "",
		verificationExit: 0,
	}

	result, err := Run(ctx, Config{
		WorkingDir:           env.workDir,
		LedgerStore:          env.ledger,
		CodexExecutable:      "codex-test",
		GitExecutable:        "git-test",
		VerificationCommands: []verification.Command{{Name: "go", Args: []string{"test", "./..."}}},
		CommandRunner:        state.run,
		Clock:                env.clock,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}

	if result.Outcome != OutcomeNoChanges {
		t.Fatalf("outcome = %s, want no_changes", result.Outcome)
	}
	if result.Commit.Status != commit.StatusRefused || result.Commit.RefusalReason != commit.ReasonNoChanges {
		t.Fatalf("commit result = %+v, want no changes refusal", result.Commit)
	}
	if result.Task.Status != taskmodel.StatusBlocked {
		t.Fatalf("task status = %q, want blocked", result.Task.Status)
	}
	updated := loadRunTask(t, env, selected.SourcePath)
	if got := updated.Status; got != taskfile.StatusBlocked {
		t.Fatalf("file task status = %q, want blocked", got)
	}
	if got := updated.Phase; got != taskfile.PhaseImplement {
		t.Fatalf("file task phase = %q, want implement", got)
	}
	if state.gitAddOrCommitCalls != 0 {
		t.Fatalf("git add/commit calls = %d, want 0", state.gitAddOrCommitCalls)
	}
	if result.Receipt.Verdict != receipt.VerdictNoChanges {
		t.Fatalf("fallback verdict = %q, want no_changes", result.Receipt.Verdict)
	}
}

func TestRunBlocksWhenCodexFailsAndSkipsVerificationAndCommit(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	selected := writeRunTask(t, env, "task-codex", "Codex fails")
	state := &fakeCommandState{
		t:          t,
		workDir:    env.workDir,
		codexExit:  2,
		postStatus: " M internal/partial.go\x00",
	}

	result, err := Run(ctx, Config{
		WorkingDir:           env.workDir,
		LedgerStore:          env.ledger,
		CodexExecutable:      "codex-test",
		GitExecutable:        "git-test",
		VerificationCommands: []verification.Command{{Name: "go", Args: []string{"test", "./..."}}},
		CommandRunner:        state.run,
		Clock:                env.clock,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}

	if result.Outcome != OutcomeCodexFailed {
		t.Fatalf("outcome = %s, want codex_failed", result.Outcome)
	}
	if result.Task.Status != taskmodel.StatusBlocked {
		t.Fatalf("task status = %q, want blocked", result.Task.Status)
	}
	if got := loadRunTask(t, env, selected.SourcePath).Status; got != taskfile.StatusBlocked {
		t.Fatalf("file task status = %q, want blocked", got)
	}
	if state.verificationCalls != 0 {
		t.Fatalf("verification calls = %d, want 0", state.verificationCalls)
	}
	if state.gitAddOrCommitCalls != 0 {
		t.Fatalf("git add/commit calls = %d, want 0", state.gitAddOrCommitCalls)
	}
	if result.Receipt.Verdict != receipt.VerdictCodexFailed {
		t.Fatalf("fallback verdict = %q, want codex_failed", result.Receipt.Verdict)
	}
}

func TestRunBlocksPreExistingDirtyBeforeContextCodexVerificationAndCommit(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	selected := writeRunTask(t, env, "task-dirty", "Avoid dirty worktree")

	codexCalled := false
	changedCalled := false
	verificationCalled := false
	commitCalled := false
	result, err := Run(ctx, Config{
		WorkingDir:  env.workDir,
		LedgerStore: env.ledger,
		DirtyCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			return gitstate.Capture{
				Kind:       gitstate.CaptureKindDirty,
				DirtyFiles: []string{"internal/dirty.go"},
				Paths:      []string{"internal/dirty.go"},
			}, nil
		},
		ChangedCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			changedCalled = true
			return gitstate.Capture{Kind: gitstate.CaptureKindChanged}, nil
		},
		CodexRunner: func(context.Context, codexexec.Config) (codexexec.Result, error) {
			codexCalled = true
			return codexexec.Result{ExitCode: 0}, nil
		},
		VerificationRunner: func(context.Context, verification.Config) (verification.Result, error) {
			verificationCalled = true
			return verification.Result{Status: verification.StatusPassed, Passed: true}, nil
		},
		CommitRunner: func(context.Context, commit.Config) (commit.Result, error) {
			commitCalled = true
			return commit.Result{Status: commit.StatusCommitted, CommitSHA: "abc123"}, nil
		},
		Clock:                  env.clock,
		CodexVersionDiscoverer: testCodexVersionDiscoverer,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}

	if result.Outcome != OutcomeBlocked {
		t.Fatalf("outcome = %s, want blocked", result.Outcome)
	}
	if result.Message != "pre-existing dirty files are present" {
		t.Fatalf("message = %q, want dirty preflight message", result.Message)
	}
	if codexCalled || !changedCalled || verificationCalled || commitCalled {
		t.Fatalf("called codex=%v changed=%v verification=%v commit=%v, want only final changed-files capture", codexCalled, changedCalled, verificationCalled, commitCalled)
	}
	if result.Task.Status != taskmodel.StatusBlocked {
		t.Fatalf("task status = %q, want blocked", result.Task.Status)
	}
	if got := loadRunTask(t, env, selected.SourcePath).Status; got != taskfile.StatusBlocked {
		t.Fatalf("file task status = %q, want blocked", got)
	}
	if result.Run.Status != ledger.StatusFailed {
		t.Fatalf("run status = %q, want failed", result.Run.Status)
	}
	if !result.ReceiptSynthesized || result.Receipt.Verdict != receipt.VerdictBlocked {
		t.Fatalf("receipt synthesized=%v verdict=%q, want synthesized blocked", result.ReceiptSynthesized, result.Receipt.Verdict)
	}
	contextPayloadPath := filepath.Join(env.workDir, ".revolvr", "runs", result.Run.ID, "context.md")
	if _, err := os.Stat(contextPayloadPath); !os.IsNotExist(err) {
		t.Fatalf("context payload artifact stat err = %v, want not exist", err)
	}
	contextManifestPath := filepath.Join(env.workDir, ".revolvr", "runs", result.Run.ID, "context.json")
	if _, err := os.Stat(contextManifestPath); !os.IsNotExist(err) {
		t.Fatalf("context manifest artifact stat err = %v, want not exist", err)
	}
	assertRunEvents(t, env.ledger, result.Run.ID, []ledger.EventType{
		ledger.EventRunStarted,
		ledger.EventTaskSelected,
		ledger.EventRunArtifacts,
		ledger.EventChangedFilesCaptured,
		ledger.EventChangedFilesCaptured,
		ledger.EventReceiptSynthesized,
		ledger.EventRunFailed,
	})

	history, ok, err := env.ledger.GetRunWithEvents(ctx, result.Run.ID)
	if err != nil || !ok {
		t.Fatalf("get run history ok=%v err=%v", ok, err)
	}
	var payload struct {
		PreRunDirtyFiles []string `json:"pre_run_dirty_files"`
		ChangedFiles     []string `json:"changed_files"`
		CaptureError     string   `json:"capture_error"`
	}
	if !decodeTestEventPayload(t, history.Events, ledger.EventChangedFilesCaptured, &payload) {
		t.Fatal("changed-files capture event not found")
	}
	if got, want := payload.PreRunDirtyFiles, []string{"internal/dirty.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("pre-run dirty files = %#v, want %#v", got, want)
	}
	if len(payload.ChangedFiles) != 0 || payload.CaptureError != "" {
		t.Fatalf("changed-files payload = %+v, want no changed files or error", payload)
	}
	if _, found, err := lock.ReadSourceWriter(ctx, env.workDir); err != nil || found {
		t.Fatalf("lock after dirty preflight found=%v err=%v, want released", found, err)
	}
}

func TestRunBlocksWhenPreRunDirtyCaptureReportsFailure(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	selected := writeRunTask(t, env, "task-dirty-capture", "Require trustworthy dirty capture")
	codexCalled := false
	result, err := Run(ctx, Config{
		WorkingDir:  env.workDir,
		LedgerStore: env.ledger,
		DirtyCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			return gitstate.Capture{
				Kind:         gitstate.CaptureKindDirty,
				CaptureError: "git status exited with code 128",
			}, nil
		},
		ChangedCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			return gitstate.Capture{
				Kind:         gitstate.CaptureKindChanged,
				ChangedFiles: []string{selected.SourcePath},
				Paths:        []string{selected.SourcePath},
			}, nil
		},
		CodexRunner: func(context.Context, codexexec.Config) (codexexec.Result, error) {
			codexCalled = true
			return codexexec.Result{ExitCode: 0}, nil
		},
		Clock:                  env.clock,
		CodexVersionDiscoverer: testCodexVersionDiscoverer,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if codexCalled || result.Outcome != OutcomeBlocked {
		t.Fatalf("codexCalled=%v outcome=%s, want blocked before Codex", codexCalled, result.Outcome)
	}
	if !strings.Contains(result.Message, "capture pre-run dirty state failed") || !strings.Contains(result.Message, "128") {
		t.Fatalf("message = %q, want dirty capture failure", result.Message)
	}
	updated := loadRunTask(t, env, selected.SourcePath)
	if updated.Status != taskfile.StatusBlocked || updated.Phase != taskfile.PhaseImplement {
		t.Fatalf("task status/phase = %s/%s, want blocked/implement", updated.Status, updated.Phase)
	}
	history, ok, historyErr := env.ledger.GetRunWithEvents(ctx, result.Run.ID)
	if historyErr != nil || !ok {
		t.Fatalf("get run history ok=%v err=%v", ok, historyErr)
	}
	var terminal struct {
		PreRunCaptureError string `json:"pre_run_capture_error"`
	}
	if !decodeTestEventPayload(t, history.Events, ledger.EventRunFailed, &terminal) || !strings.Contains(terminal.PreRunCaptureError, "128") {
		t.Fatalf("terminal pre-run capture error = %q, want captured failure", terminal.PreRunCaptureError)
	}
}

func TestRunAllowsPreExistingDirtyWhenConfigured(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	selected := writeRunTask(t, env, "task-allow-dirty", "Proceed with dirty worktree")

	codexCalled := false
	verificationCalled := false
	commitCalled := false
	result, err := Run(ctx, Config{
		WorkingDir:                env.workDir,
		LedgerStore:               env.ledger,
		AllowPreExistingDirty:     true,
		VerificationCommands:      []verification.Command{{Name: "go", Args: []string{"test", "./..."}}},
		MissingVerificationPolicy: verification.MissingCommandsFail,
		DirtyCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			return gitstate.Capture{
				Kind:       gitstate.CaptureKindDirty,
				DirtyFiles: []string{"internal/dirty.go"},
				Paths:      []string{"internal/dirty.go"},
			}, nil
		},
		ChangedCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			return gitstate.Capture{
				Kind:         gitstate.CaptureKindChanged,
				ChangedFiles: []string{"internal/feature.go"},
				Paths:        []string{"internal/feature.go"},
			}, nil
		},
		CodexRunner: func(context.Context, codexexec.Config) (codexexec.Result, error) {
			codexCalled = true
			return codexexec.Result{ExitCode: 0}, nil
		},
		VerificationRunner: func(context.Context, verification.Config) (verification.Result, error) {
			verificationCalled = true
			return verification.Result{
				Status:             verification.StatusPassed,
				Passed:             true,
				FailedCommandIndex: -1,
				Commands: []verification.CommandResult{{
					Command:  "go test ./...",
					Name:     "go",
					Args:     []string{"test", "./..."},
					Status:   verification.StatusPassed,
					Passed:   true,
					ExitCode: 0,
				}},
			}, nil
		},
		CommitRunner: func(_ context.Context, cfg commit.Config) (commit.Result, error) {
			commitCalled = true
			if !cfg.AllowPreExistingDirty {
				t.Fatal("commit config AllowPreExistingDirty = false, want true")
			}
			return commit.Result{Status: commit.StatusCommitted, CommitSHA: "abc123", Message: "commit created"}, nil
		},
		Clock:                  env.clock,
		CodexVersionDiscoverer: testCodexVersionDiscoverer,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}

	if result.Outcome != OutcomeCommitted {
		t.Fatalf("outcome = %s, want committed; message=%s", result.Outcome, result.Message)
	}
	if !codexCalled || !verificationCalled || !commitCalled {
		t.Fatalf("called codex=%v verification=%v commit=%v, want all true", codexCalled, verificationCalled, commitCalled)
	}
	if result.Run.CommitSHA != "abc123" {
		t.Fatalf("run commit sha = %q, want abc123", result.Run.CommitSHA)
	}
	if result.Task.Status != taskmodel.StatusPending {
		t.Fatalf("task status = %q, want pending", result.Task.Status)
	}
	updated := loadRunTask(t, env, selected.SourcePath)
	if got := updated.Status; got != taskfile.StatusPending {
		t.Fatalf("file task status = %q, want pending", got)
	}
	if got := updated.Phase; got != taskfile.PhaseAudit {
		t.Fatalf("file task phase = %q, want audit", got)
	}
}

func TestRunUpdatesParsedReceiptWhenVerificationFails(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	writeRunTask(t, env, "task-receipt-verify", "Break verification after receipt")
	state := &fakeCommandState{
		t:                t,
		workDir:          env.workDir,
		writeReceipt:     true,
		postStatus:       " M internal/feature.go\x00",
		verificationExit: 1,
	}

	result, err := Run(ctx, Config{
		WorkingDir:           env.workDir,
		LedgerStore:          env.ledger,
		CodexExecutable:      "codex-test",
		GitExecutable:        "git-test",
		VerificationCommands: []verification.Command{{Name: "go", Args: []string{"test", "./..."}}},
		CommandRunner:        state.run,
		Clock:                env.clock,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}

	if result.Outcome != OutcomeVerificationFailed {
		t.Fatalf("outcome = %s, want verification_failed", result.Outcome)
	}
	if result.ReceiptSynthesized {
		t.Fatal("receipt was synthesized, want parsed receipt updated by harness")
	}
	if result.Receipt.Verdict != receipt.VerdictVerificationFailed {
		t.Fatalf("receipt verdict = %q, want verification_failed", result.Receipt.Verdict)
	}
	if result.Receipt.VerificationStatus != "failed" {
		t.Fatalf("receipt verification status = %q, want failed", result.Receipt.VerificationStatus)
	}
	if state.gitAddOrCommitCalls != 0 {
		t.Fatalf("git add/commit calls = %d, want 0", state.gitAddOrCommitCalls)
	}
}

func TestRunBlocksBeforeVerificationAndCommitWhenReceiptCannotBeSynthesized(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	selected := writeRunTask(t, env, "task-receipt-write", "Require durable receipt")
	verificationCalled := false
	commitCalled := false
	result, err := Run(ctx, Config{
		WorkingDir:  env.workDir,
		LedgerStore: env.ledger,
		DirtyCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			return gitstate.Capture{Kind: gitstate.CaptureKindDirty}, nil
		},
		ChangedCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			return gitstate.Capture{Kind: gitstate.CaptureKindChanged, ChangedFiles: []string{selected.SourcePath}, Paths: []string{selected.SourcePath}}, nil
		},
		CodexRunner: func(_ context.Context, cfg codexexec.Config) (codexexec.Result, error) {
			receiptRel := contextPayloadValue(t, cfg.Prompt, "Receipt path")
			if mkdirErr := os.MkdirAll(filepath.Join(cfg.WorkingDir, receiptRel), 0o755); mkdirErr != nil {
				return codexexec.Result{}, mkdirErr
			}
			return codexexec.Result{ExitCode: 0, FinalMessage: "done"}, nil
		},
		VerificationRunner: func(context.Context, verification.Config) (verification.Result, error) {
			verificationCalled = true
			return passedVerificationResult("go test ./..."), nil
		},
		CommitRunner: func(context.Context, commit.Config) (commit.Result, error) {
			commitCalled = true
			return commit.Result{Status: commit.StatusCommitted, CommitSHA: "unexpected"}, nil
		},
		Clock:                  env.clock,
		CodexVersionDiscoverer: testCodexVersionDiscoverer,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if result.Outcome != OutcomeBlocked || verificationCalled || commitCalled {
		t.Fatalf("outcome=%s verificationCalled=%v commitCalled=%v, want blocked before verification/commit", result.Outcome, verificationCalled, commitCalled)
	}
	if !strings.Contains(result.Message, "prepare run receipt failed") || result.ReceiptError == "" {
		t.Fatalf("message/error = %q / %q, want receipt preparation failure", result.Message, result.ReceiptError)
	}
	updated := loadRunTask(t, env, selected.SourcePath)
	if updated.Status != taskfile.StatusBlocked || updated.Phase != taskfile.PhaseImplement {
		t.Fatalf("task status/phase = %s/%s, want blocked/implement", updated.Status, updated.Phase)
	}
	if result.Run.Status != ledger.StatusFailed {
		t.Fatalf("run status = %s, want failed", result.Run.Status)
	}
	history, ok, historyErr := env.ledger.GetRunWithEvents(ctx, result.Run.ID)
	if historyErr != nil || !ok {
		t.Fatalf("get run history ok=%v err=%v", ok, historyErr)
	}
	var terminal struct {
		ReceiptError string `json:"receipt_error"`
	}
	if !decodeTestEventPayload(t, history.Events, ledger.EventRunFailed, &terminal) || terminal.ReceiptError == "" {
		t.Fatalf("terminal receipt error = %q, want finalization failure evidence", terminal.ReceiptError)
	}
}

func TestRunLedgerCompletionFailureStillBlocksTaskAndFinalizesReceipt(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	selected := writeRunTask(t, env, "task-ledger-failure", "Handle ledger failure")
	result, err := Run(ctx, Config{
		WorkingDir:     env.workDir,
		LedgerStore:    env.ledger,
		DirtyCapture:   cleanDirtyCapture,
		ChangedCapture: emptyChangedCapture,
		CodexRunner: func(context.Context, codexexec.Config) (codexexec.Result, error) {
			if closeErr := env.ledger.Close(); closeErr != nil {
				t.Fatalf("close ledger: %v", closeErr)
			}
			return codexexec.Result{ExitCode: 2, FinalMessage: "failed"}, nil
		},
		Clock:                  env.clock,
		CodexVersionDiscoverer: testCodexVersionDiscoverer,
	})
	if err == nil {
		t.Fatal("run once succeeded, want ledger completion error")
	}
	if result.Outcome != OutcomeCodexFailed || result.Run.Status != ledger.StatusRunning || result.LedgerError == nil {
		t.Fatalf("result outcome/run/ledger = %s/%s/%v, want codex_failed/running/error", result.Outcome, result.Run.Status, result.LedgerError)
	}
	updated := loadRunTask(t, env, selected.SourcePath)
	if updated.Status != taskfile.StatusBlocked || updated.Phase != taskfile.PhaseImplement {
		t.Fatalf("task status/phase = %s/%s, want blocked/implement despite ledger failure", updated.Status, updated.Phase)
	}
	if result.Receipt.Verdict != receipt.VerdictCodexFailed {
		t.Fatalf("receipt verdict = %q, want codex_failed", result.Receipt.Verdict)
	}
	if _, receiptErr := os.Stat(result.ReceiptPath); receiptErr != nil {
		t.Fatalf("final receipt stat: %v", receiptErr)
	}
}

func TestRunRefusesLiveSourceWriterLockBeforeStateMutation(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	selected := writeRunTask(t, env, "task-locked", "Do locked work")

	existing, err := lock.AcquireSourceWriter(ctx, lock.Config{
		WorkingDir: env.workDir,
		RunID:      "already-running",
		PID:        999,
		Timeout:    time.Hour,
		Clock:      env.clock,
	})
	if err != nil {
		t.Fatalf("acquire existing source-writer lock: %v", err)
	}
	defer existing.Release(ctx)

	state := &fakeCommandState{t: t, workDir: env.workDir}
	result, err := Run(ctx, Config{
		WorkingDir:          env.workDir,
		LedgerStore:         env.ledger,
		CommandRunner:       state.run,
		Clock:               env.clock,
		SourceWriterLockPID: 123,
	})
	if !errors.Is(err, lock.ErrHeld) {
		t.Fatalf("run once error = %v, want source-writer lock held", err)
	}
	if result.Run.ID != "" {
		t.Fatalf("run id = %q, want no ledger run created", result.Run.ID)
	}
	if len(state.commands) != 0 {
		t.Fatalf("commands = %#v, want none", state.commands)
	}
	runs, err := env.ledger.ListRecentRuns(ctx, 10)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("runs = %#v, want none", runs)
	}
	if got := loadRunTask(t, env, selected.SourcePath).Status; got != taskfile.StatusPending {
		t.Fatalf("file task status = %q, want pending", got)
	}
}

func TestRunRefreshesSourceWriterLockWhileCodexRuns(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	writeRunTask(t, env, "task-heartbeat", "Observe heartbeat")
	clock := &advancingClock{current: time.Date(2026, 6, 26, 14, 0, 0, 0, time.UTC), step: time.Second}

	var observed lock.Metadata
	codexRunner := func(ctx context.Context, cfg codexexec.Config) (codexexec.Result, error) {
		deadline := time.After(2 * time.Second)
		for {
			metadata, found, err := lock.ReadSourceWriter(ctx, env.workDir)
			if err != nil {
				return codexexec.Result{}, err
			}
			if found && metadata.RunID == cfg.RunID && metadata.HeartbeatAt.After(metadata.AcquiredAt) {
				observed = metadata
				return codexexec.Result{ExitCode: 2}, nil
			}
			select {
			case <-ctx.Done():
				return codexexec.Result{}, ctx.Err()
			case <-deadline:
				return codexexec.Result{}, errors.New("timed out waiting for source-writer heartbeat")
			case <-time.After(time.Millisecond):
			}
		}
	}

	result, err := Run(ctx, Config{
		WorkingDir:                        env.workDir,
		LedgerStore:                       env.ledger,
		CodexRunner:                       codexRunner,
		DirtyCapture:                      cleanDirtyCapture,
		ChangedCapture:                    emptyChangedCapture,
		Clock:                             clock.now,
		SourceWriterLockPID:               321,
		SourceWriterLockTimeout:           time.Hour,
		SourceWriterLockHeartbeatInterval: time.Millisecond,
		CodexVersionDiscoverer:            testCodexVersionDiscoverer,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if result.Outcome != OutcomeCodexFailed {
		t.Fatalf("outcome = %s, want codex_failed", result.Outcome)
	}
	if observed.RunID != result.Run.ID || observed.PID != 321 {
		t.Fatalf("observed metadata = %+v, want run %s pid 321", observed, result.Run.ID)
	}
	if !observed.HeartbeatAt.After(observed.AcquiredAt) {
		t.Fatalf("heartbeat was not refreshed: %+v", observed)
	}
	if _, found, err := lock.ReadSourceWriter(ctx, env.workDir); err != nil || found {
		t.Fatalf("lock after run found=%v err=%v, want released", found, err)
	}
}

func TestRunHeartbeatFailureCancelsCodexAndPreservesTask(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	selected := writeRunTask(t, env, "task-heartbeat-failure", "Stop on lost ownership")
	persistenceErr := fmt.Errorf("injected heartbeat write failure: %w", os.ErrPermission)
	allowFailure := make(chan struct{})
	lease := &scriptedSourceLease{heartbeatFunc: func(ctx context.Context) error {
		select {
		case <-allowFailure:
			return persistenceErr
		case <-ctx.Done():
			return ctx.Err()
		}
	}}
	verificationCalled := false
	commitCalled := false

	result, err := Run(ctx, Config{
		WorkingDir: env.workDir, LedgerStore: env.ledger, Clock: env.clock,
		DirtyCapture: cleanDirtyCapture, ChangedCapture: emptyChangedCapture,
		CodexVersionDiscoverer:            testCodexVersionDiscoverer,
		SourceWriterLockHeartbeatInterval: time.Millisecond,
		SourceLockAcquirer:                func(context.Context, lock.Config) (lock.SourceLease, error) { return lease, nil },
		CodexRunner: func(ctx context.Context, _ codexexec.Config) (codexexec.Result, error) {
			close(allowFailure)
			<-ctx.Done()
			return codexexec.Result{ExitCode: -1, Err: context.Cause(ctx)}, context.Cause(ctx)
		},
		VerificationRunner: func(context.Context, verification.Config) (verification.Result, error) {
			verificationCalled = true
			return verification.Result{}, nil
		},
		CommitRunner: func(context.Context, commit.Config) (commit.Result, error) {
			commitCalled = true
			return commit.Result{}, nil
		},
	})
	if !errors.Is(err, lock.ErrOwnershipLost) || !errors.Is(err, persistenceErr) || !errors.Is(err, os.ErrPermission) {
		t.Fatalf("Run error = %v, want ownership and persistence failures", err)
	}
	if result.Outcome != OutcomeBlocked || result.Run.Status != ledger.StatusFailed || verificationCalled || commitCalled {
		t.Fatalf("result=%+v verification=%t commit=%t", result, verificationCalled, commitCalled)
	}
	if got := loadRunTask(t, env, selected.SourcePath).Status; got != taskfile.StatusPending {
		t.Fatalf("task status = %q, want pending after ownership loss", got)
	}
	if lease.releasesCount() != 1 {
		t.Fatalf("release count = %d, want 1", lease.releasesCount())
	}
}

func TestRunSynchronousOwnershipCheckPreventsCommit(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	selected := writeRunTask(t, env, "task-boundary-lock", "Do not commit without ownership")
	state := &fakeCommandState{
		t: t, workDir: env.workDir, writeReceipt: true,
		postStatus: " M internal/feature.go\x00", verificationExit: 0, commitSHA: "must-not-commit",
	}
	ownerErr := lock.ErrHeld
	lease := &scriptedSourceLease{heartbeatErrAt: 3, heartbeatErr: ownerErr}
	result, err := Run(ctx, Config{
		WorkingDir: env.workDir, LedgerStore: env.ledger,
		CodexExecutable: "codex-test", GitExecutable: "git-test",
		VerificationCommands: []verification.Command{{Name: "go", Args: []string{"test", "./..."}}},
		CommandRunner:        state.run, Clock: env.clock,
		SourceWriterLockHeartbeatInterval: time.Hour,
		SourceLockAcquirer:                func(context.Context, lock.Config) (lock.SourceLease, error) { return lease, nil },
	})
	if !errors.Is(err, lock.ErrOwnershipLost) || !errors.Is(err, ownerErr) {
		t.Fatalf("Run error = %v, want replacement-owner failure", err)
	}
	if result.Outcome != OutcomeBlocked || result.Commit.Status == commit.StatusCommitted {
		t.Fatalf("result = %+v", result)
	}
	for _, command := range state.gitCommands {
		if len(command) > 0 && command[0] == "commit" {
			t.Fatalf("commit command ran after ownership loss: %v", state.gitCommands)
		}
	}
	if got := loadRunTask(t, env, selected.SourcePath); got.Status != taskfile.StatusPending || got.Phase != taskfile.PhaseImplement {
		t.Fatalf("task status/phase = %s/%s, want pending/implement", got.Status, got.Phase)
	}
}

func TestRunJoinsHeartbeatAndReleaseFailures(t *testing.T) {
	env := newTestEnv(t)
	writeRunTask(t, env, "task-lock-errors", "Retain both lock errors")
	heartbeatErr := errors.New("heartbeat persistence failure")
	releaseErr := errors.New("release persistence failure")
	lease := &scriptedSourceLease{heartbeatErrAt: 1, heartbeatErr: heartbeatErr, releaseErr: releaseErr}
	_, err := Run(context.Background(), Config{
		WorkingDir: env.workDir, LedgerStore: env.ledger, Clock: env.clock,
		DirtyCapture: cleanDirtyCapture, ChangedCapture: emptyChangedCapture,
		CodexVersionDiscoverer:            testCodexVersionDiscoverer,
		SourceWriterLockHeartbeatInterval: time.Millisecond,
		SourceLockAcquirer:                func(context.Context, lock.Config) (lock.SourceLease, error) { return lease, nil },
		CodexRunner: func(ctx context.Context, _ codexexec.Config) (codexexec.Result, error) {
			<-ctx.Done()
			return codexexec.Result{ExitCode: -1}, context.Cause(ctx)
		},
	})
	for _, want := range []error{lock.ErrOwnershipLost, heartbeatErr, releaseErr} {
		if !errors.Is(err, want) {
			t.Fatalf("Run error = %v, missing %v", err, want)
		}
	}
}

func TestRunPreservesCancellationRacingWithHeartbeatFailure(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	env := newTestEnv(t)
	selected := writeRunTask(t, env, "task-cancel-lock-race", "Preserve cancellation and lock failure")
	started := make(chan struct{})
	var startedOnce sync.Once
	persistenceErr := errors.New("heartbeat persistence failed during cancellation")
	lease := &scriptedSourceLease{heartbeatFunc: func(ctx context.Context) error {
		startedOnce.Do(func() { close(started) })
		<-ctx.Done()
		return errors.Join(ctx.Err(), persistenceErr)
	}}
	result, err := Run(parent, Config{
		WorkingDir: env.workDir, LedgerStore: env.ledger, Clock: env.clock,
		DirtyCapture: cleanDirtyCapture, ChangedCapture: emptyChangedCapture,
		CodexVersionDiscoverer:            testCodexVersionDiscoverer,
		SourceWriterLockHeartbeatInterval: time.Millisecond,
		SourceLockAcquirer:                func(context.Context, lock.Config) (lock.SourceLease, error) { return lease, nil },
		CodexRunner: func(ctx context.Context, _ codexexec.Config) (codexexec.Result, error) {
			select {
			case <-started:
			case <-time.After(time.Second):
				return codexexec.Result{}, errors.New("heartbeat did not start")
			}
			cancel()
			<-ctx.Done()
			return codexexec.Result{ExitCode: -1, Err: ctx.Err()}, ctx.Err()
		},
	})
	for _, want := range []error{context.Canceled, lock.ErrOwnershipLost, persistenceErr} {
		if !errors.Is(err, want) {
			t.Fatalf("Run error = %v, missing %v", err, want)
		}
	}
	if result.Outcome != OutcomeBlocked || loadRunTask(t, env, selected.SourcePath).Status != taskfile.StatusPending {
		t.Fatalf("result=%+v task=%+v", result, loadRunTask(t, env, selected.SourcePath))
	}
}

func TestRunReleasesSourceWriterLockOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	env := newTestEnv(t)
	writeRunTask(t, env, "task-cancel", "Cancel while Codex runs")

	codexRunner := func(ctx context.Context, cfg codexexec.Config) (codexexec.Result, error) {
		cancel()
		err := ctx.Err()
		if err == nil {
			err = context.Canceled
		}
		return codexexec.Result{ExitCode: -1, Err: err}, err
	}

	_, err := Run(ctx, Config{
		WorkingDir:             env.workDir,
		LedgerStore:            env.ledger,
		CodexRunner:            codexRunner,
		CodexVersionDiscoverer: testCodexVersionDiscoverer,
		DirtyCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			return gitstate.Capture{Kind: gitstate.CaptureKindDirty}, nil
		},
		ChangedCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			return gitstate.Capture{Kind: gitstate.CaptureKindChanged}, nil
		},
		Clock: env.clock,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("run once error = %v, want context canceled", err)
	}
	if _, found, readErr := lock.ReadSourceWriter(context.Background(), env.workDir); readErr != nil || found {
		t.Fatalf("lock after cancellation found=%v err=%v, want released", found, readErr)
	}
}

type scriptedSourceLease struct {
	mu             sync.Mutex
	heartbeats     int
	releases       int
	heartbeatErrAt int
	heartbeatErr   error
	heartbeatFunc  func(context.Context) error
	releaseErr     error
}

func (l *scriptedSourceLease) Heartbeat(ctx context.Context) error {
	l.mu.Lock()
	l.heartbeats++
	fn := l.heartbeatFunc
	if l.heartbeatErrAt > 0 && l.heartbeats >= l.heartbeatErrAt {
		err := l.heartbeatErr
		l.mu.Unlock()
		return err
	}
	l.mu.Unlock()
	if fn != nil {
		return fn(ctx)
	}
	return nil
}

func (l *scriptedSourceLease) Release(context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.releases++
	return l.releaseErr
}

func (l *scriptedSourceLease) releasesCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.releases
}

func TestRunPassesCodexBypassApprovalsAndSandbox(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	writeRunTask(t, env, "task-yolo", "Run without permission gates")

	codexCalled := false
	result, err := Run(ctx, Config{
		WorkingDir:                     env.workDir,
		LedgerStore:                    env.ledger,
		CodexSandbox:                   "workspace-write",
		CodexApprovalPolicy:            "never",
		CodexBypassApprovalsAndSandbox: true,
		DirtyCapture:                   cleanDirtyCapture,
		ChangedCapture:                 emptyChangedCapture,
		Clock:                          env.clock,
		CodexVersionDiscoverer:         testCodexVersionDiscoverer,
		CodexRunner: func(_ context.Context, cfg codexexec.Config) (codexexec.Result, error) {
			codexCalled = true
			if !cfg.BypassApprovalsAndSandbox {
				t.Fatal("codex bypass approvals and sandbox = false, want true")
			}
			if cfg.Sandbox != "workspace-write" || cfg.ApprovalPolicy != "never" {
				t.Fatalf("codex config sandbox/policy = %q/%q, want workspace-write/never", cfg.Sandbox, cfg.ApprovalPolicy)
			}
			return codexexec.Result{ExitCode: 1}, nil
		},
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if !codexCalled {
		t.Fatal("codex runner was not called")
	}
	if result.Outcome != OutcomeCodexFailed {
		t.Fatalf("outcome = %s, want codex_failed", result.Outcome)
	}
}

type testEnv struct {
	workDir string
	ledger  *ledger.Store
	now     time.Time
}

func newTestEnv(t *testing.T) testEnv {
	t.Helper()
	ctx := context.Background()
	workDir := t.TempDir()
	writeTestRunProfile(t, workDir, prompt.DefaultRunProfileName, defaultRunProfileTemplateContent(t))
	now := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	runs, err := ledger.OpenWithClock(ctx, filepath.Join(workDir, "ledger.sqlite"), func() time.Time { return now })
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	t.Cleanup(func() { _ = runs.Close() })
	return testEnv{workDir: workDir, ledger: runs, now: now}
}

func (e testEnv) clock() time.Time {
	return e.now.Add(2 * time.Minute)
}

func writeTestRunProfile(t *testing.T, workDir string, name string, content string) {
	t.Helper()
	path := filepath.Join(workDir, prompt.RunProfileSourcePath(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create profile dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimRight(content, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write profile file: %v", err)
	}
}

func writeRunTask(t *testing.T, env testEnv, id string, title string) taskfile.Task {
	t.Helper()
	return writeRunTaskFile(t, env, id+".md", taskFileMarkdown(id, title, taskfile.StatusPending, nil))
}

func writeRunTaskWithPhase(t *testing.T, env testEnv, id string, title string, phase string) taskfile.Task {
	t.Helper()
	return writeRunTaskFile(t, env, id+".md", taskFileMarkdownWithPhase(id, title, taskfile.StatusPending, phase))
}

func writeRunTaskFile(t *testing.T, env testEnv, name string, content string) taskfile.Task {
	t.Helper()
	path := filepath.Join(env.workDir, taskfile.TasksDir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create task dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write task file: %v", err)
	}
	task, err := taskfile.Load(env.workDir, filepath.Join(taskfile.TasksDir, name))
	if err != nil {
		t.Fatalf("load task file: %v", err)
	}
	return task
}

func loadRunTask(t *testing.T, env testEnv, sourcePath string) taskfile.Task {
	t.Helper()
	task, err := taskfile.Load(env.workDir, sourcePath)
	if err != nil {
		t.Fatalf("load task file %s: %v", sourcePath, err)
	}
	return task
}

func taskFileMarkdown(id string, title string, status string, priority *int) string {
	var out strings.Builder
	out.WriteString("---\n")
	fmt.Fprintf(&out, "id: %s\n", id)
	fmt.Fprintf(&out, "status: %s\n", status)
	if priority != nil {
		fmt.Fprintf(&out, "priority: %d\n", *priority)
	}
	out.WriteString("---\n")
	fmt.Fprintf(&out, "# %s\n\n", title)
	fmt.Fprintf(&out, "%s\n", title)
	return out.String()
}

func taskFileMarkdownWithPhase(id string, title string, status string, phase string) string {
	var out strings.Builder
	out.WriteString("---\n")
	fmt.Fprintf(&out, "id: %s\n", id)
	fmt.Fprintf(&out, "workflow: %s\n", taskfile.WorkflowMixedPassV1)
	fmt.Fprintf(&out, "phase: %s\n", phase)
	fmt.Fprintf(&out, "status: %s\n", status)
	out.WriteString("---\n")
	fmt.Fprintf(&out, "# %s\n\n", title)
	fmt.Fprintf(&out, "%s\n", title)
	return out.String()
}

func taskFileMarkdownWithScheduling(id, title, status, phase string, priority *int, dependencies []string) string {
	var out strings.Builder
	out.WriteString("---\n")
	fmt.Fprintf(&out, "id: %s\n", id)
	fmt.Fprintf(&out, "workflow: %s\n", taskfile.WorkflowMixedPassV1)
	fmt.Fprintf(&out, "phase: %s\n", phase)
	fmt.Fprintf(&out, "status: %s\n", status)
	if priority != nil {
		fmt.Fprintf(&out, "priority: %d\n", *priority)
	}
	if len(dependencies) != 0 {
		fmt.Fprintf(&out, "depends_on: %s\n", strings.Join(dependencies, ", "))
	}
	out.WriteString("---\n")
	fmt.Fprintf(&out, "# %s\n\n%s\n", title, title)
	return out.String()
}

func writeAutonomousSchedulingTask(t *testing.T, env testEnv, id string, lifecycle autonomous.LifecycleState) {
	t.Helper()
	writeRunTaskFile(t, env, "020-"+id+".md", fmt.Sprintf(`---
id: %s
status: pending
workflow: autonomous-v1
autonomous_state_path: .revolvr/autonomous/tasks/%s/state.json
priority: 50
---
# Autonomous Dependency

Wait for supervisor input.
`, id, id))
	state := autonomous.ExecutionState{
		SchemaVersion: autonomous.ExecutionStateSchemaVersion,
		TaskID:        id,
		Lifecycle:     lifecycle,
		Attempts: autonomous.AttemptState{
			RetryBudget:       autonomous.CountBudget{Mode: autonomous.BudgetModeUnset},
			ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnset},
			TokenBudget:       autonomous.CountBudget{Mode: autonomous.BudgetModeUnset},
		},
	}
	if lifecycle == autonomous.LifecycleStateNeedsInput {
		state.NeedsInput = &autonomous.NeedsInputDetail{Reason: "operator decision required"}
	}
	raw, err := autonomousstate.MarshalState(state)
	if err != nil {
		t.Fatalf("marshal autonomous scheduling state: %v", err)
	}
	statePath := filepath.Join(env.workDir, ".revolvr", "autonomous", "tasks", id, "state.json")
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatalf("create autonomous scheduling state directory: %v", err)
	}
	if err := os.WriteFile(statePath, raw, 0o644); err != nil {
		t.Fatalf("write autonomous scheduling state: %v", err)
	}
}

func scheduleTaskResult(t *testing.T, result taskscheduler.Result, taskID string) taskscheduler.TaskReadiness {
	t.Helper()
	for _, task := range result.Tasks {
		if task.TaskID == taskID {
			return task
		}
	}
	t.Fatalf("schedule task %q not found in %#v", taskID, result.Tasks)
	return taskscheduler.TaskReadiness{}
}

func ptrInt(value int) *int {
	return &value
}

func defaultRunProfileTemplateContent(t *testing.T) string {
	t.Helper()
	for _, template := range prompt.DefaultRunProfileTemplates() {
		if template.Name == prompt.DefaultRunProfileName {
			return template.Content
		}
	}
	t.Fatalf("default profile template %q not found", prompt.DefaultRunProfileName)
	return ""
}

func passedVerificationResult(command string) verification.Result {
	return verification.Result{
		Status:             verification.StatusPassed,
		Passed:             true,
		FailedCommandIndex: -1,
		Commands: []verification.CommandResult{{
			Command:  command,
			Status:   verification.StatusPassed,
			Passed:   true,
			ExitCode: 0,
		}},
	}
}

func runTestGit(t *testing.T, workDir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func assertGitStatusClean(t *testing.T, workDir string) {
	t.Helper()
	cmd := exec.Command("git", "status", "--short", "--untracked-files=all")
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git status failed: %v\n%s", err, output)
	}
	if len(output) != 0 {
		t.Fatalf("git status = %q, want clean", output)
	}
}

func cleanDirtyCapture(context.Context, gitstate.Config) (gitstate.Capture, error) {
	return gitstate.Capture{Kind: gitstate.CaptureKindDirty}, nil
}

func emptyChangedCapture(context.Context, gitstate.Config) (gitstate.Capture, error) {
	return gitstate.Capture{Kind: gitstate.CaptureKindChanged}, nil
}

func testCodexVersionDiscoverer(context.Context, codexexec.VersionConfig) (string, error) {
	return "codex-test 1.2.3", nil
}

type advancingClock struct {
	mu      sync.Mutex
	current time.Time
	step    time.Duration
}

func (c *advancingClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.current = c.current.Add(c.step)
	return c.current
}

type fakeCommandState struct {
	t                   *testing.T
	workDir             string
	writeReceipt        bool
	receiptContent      func(runID string, taskID string, task string) string
	postStatus          string
	codexExit           int
	verificationExit    int
	commitSHA           string
	expectedCommitAdd   []string
	commands            []runner.Command
	codexArgs           []string
	gitCommands         [][]string
	gitStatusCalls      int
	gitHEADCalls        int
	gitAddOrCommitCalls int
	verificationCalls   int
}

func (s *fakeCommandState) run(_ context.Context, command runner.Command) runner.Result {
	s.commands = append(s.commands, command)
	switch command.Name {
	case "codex-test", "codex":
		return s.runCodex(command)
	case "git-test", "git":
		return s.runGit(command)
	case "go":
		s.verificationCalls++
		if s.verificationExit != 0 {
			return runner.Result{ExitCode: s.verificationExit, Stderr: "verification failed\n"}
		}
		return runner.Result{ExitCode: 0, Stdout: "ok\n"}
	default:
		s.t.Fatalf("unexpected command %s %#v", command.Name, command.Args)
		return runner.Result{ExitCode: 127}
	}
}

func (s *fakeCommandState) runCodex(command runner.Command) runner.Result {
	if reflect.DeepEqual(command.Args, []string{"--version"}) {
		return runner.Result{ExitCode: 0, Stdout: "codex-test 1.2.3\n"}
	}
	s.codexArgs = append([]string(nil), command.Args...)
	contextPayload := readContextPayload(s.t, command.Stdin)
	receiptRel := contextPayloadValue(s.t, contextPayload, "Receipt path")
	runID := contextPayloadValue(s.t, contextPayload, "Run ID")
	if _, err := os.Stat(filepath.Join(command.Dir, ".revolvr", "runs", runID, "context.json")); err != nil {
		s.t.Fatalf("context manifest was not written before Codex started: %v", err)
	}
	taskID := contextPayloadValue(s.t, contextPayload, "Task ID")
	taskText := contextPayloadTaskText(s.t, contextPayload)
	if s.writeReceipt {
		content := validReceipt(runID, taskID, taskText)
		if s.receiptContent != nil {
			content = s.receiptContent(runID, taskID, taskText)
		}
		if err := writeTestFile(filepath.Join(command.Dir, receiptRel), content); err != nil {
			s.t.Fatalf("write receipt: %v", err)
		}
	}
	if lastMessagePath := argAfter(command.Args, "--output-last-message"); lastMessagePath != "" {
		if err := writeTestFile(lastMessagePath, "final message\n"); err != nil {
			s.t.Fatalf("write last message: %v", err)
		}
	}
	line := `{"type":"turn.completed","final_message":"done","usage":{"input_tokens":7,"output_tokens":3,"duration_seconds":1}}`
	if command.StdoutWriter != nil {
		if _, err := io.WriteString(command.StdoutWriter, line+"\n"); err != nil {
			s.t.Fatalf("write Codex stdout: %v", err)
		}
	}
	exitCode := s.codexExit
	return runner.Result{ExitCode: exitCode, Stdout: line + "\n"}
}

func (s *fakeCommandState) runGit(command runner.Command) runner.Result {
	s.gitCommands = append(s.gitCommands, append([]string(nil), command.Args...))
	if reflect.DeepEqual(command.Args, []string{"status", "--porcelain=v1", "-z", "--untracked-files=all"}) {
		s.gitStatusCalls++
		if s.gitStatusCalls == 1 {
			return runner.Result{ExitCode: 0}
		}
		return runner.Result{ExitCode: 0, Stdout: s.postStatus}
	}
	subcommand := gitSubcommand(command.Args)
	if subcommand == "add" || subcommand == "commit" {
		s.gitAddOrCommitCalls++
	}
	if len(s.expectedCommitAdd) > 0 && subcommand == "add" && !reflect.DeepEqual(command.Args, s.expectedCommitAdd) {
		s.t.Fatalf("git add args = %#v, want %#v", command.Args, s.expectedCommitAdd)
	}
	switch subcommand {
	case "add", "commit":
		return runner.Result{ExitCode: 0}
	case "rev-parse":
		s.gitHEADCalls++
		if s.gitHEADCalls == 1 {
			return runner.Result{ExitCode: 0, Stdout: "parent123\n"}
		}
		sha := s.commitSHA
		if sha == "" {
			sha = "abc123"
		}
		return runner.Result{ExitCode: 0, Stdout: sha + "\n"}
	default:
		s.t.Fatalf("unexpected git command %#v", command.Args)
		return runner.Result{ExitCode: 2}
	}
}

func validReceipt(runID string, taskID string, task string) string {
	return fmt.Sprintf(`---
schema_version: revolvr.receipt.v1
run_id: %s
pass_id: %s
task_id: %s
task: %q
verdict: completed
timestamp: 2026-06-26T12:00:00Z
codex_exit_code: 0
verification_status: not_run
commit_sha: ""
changed_files:
  - internal/feature.go
verification: []
metrics:
  input_tokens: 0
  output_tokens: 0
  duration_seconds: 0
---
## Summary
Implemented the selected task.

## Changed Files
- internal/feature.go

## Verification
- Not run yet.

## Concerns
None.

## Next Steps
None.
`, runID, runID, taskID, task)
}

type receiptOptions struct {
	Verdict            receipt.Verdict
	ChangedFiles       []string
	VerificationStatus string
	Verification       []receipt.VerificationEntry
}

func receiptContent(runID string, taskID string, task string, opts receiptOptions) string {
	verdict := opts.Verdict
	if verdict == "" {
		verdict = receipt.VerdictCompleted
	}
	verificationStatus := opts.VerificationStatus
	if verificationStatus == "" {
		verificationStatus = "not_run"
	}

	var out strings.Builder
	out.WriteString("---\n")
	fmt.Fprintf(&out, "schema_version: %s\n", receipt.SchemaVersion)
	fmt.Fprintf(&out, "run_id: %s\n", runID)
	fmt.Fprintf(&out, "pass_id: %s\n", runID)
	fmt.Fprintf(&out, "task_id: %s\n", taskID)
	fmt.Fprintf(&out, "task: %q\n", task)
	fmt.Fprintf(&out, "verdict: %s\n", verdict)
	out.WriteString("timestamp: 2026-06-26T12:00:00Z\n")
	out.WriteString("codex_exit_code: 0\n")
	fmt.Fprintf(&out, "verification_status: %s\n", verificationStatus)
	out.WriteString("commit_sha: \"\"\n")
	if len(opts.ChangedFiles) == 0 {
		out.WriteString("changed_files: []\n")
	} else {
		out.WriteString("changed_files:\n")
		for _, path := range opts.ChangedFiles {
			fmt.Fprintf(&out, "  - %s\n", path)
		}
	}
	if len(opts.Verification) == 0 {
		out.WriteString("verification: []\n")
	} else {
		out.WriteString("verification:\n")
		for _, entry := range opts.Verification {
			fmt.Fprintf(&out, "  - command: %s\n", entry.Command)
			fmt.Fprintf(&out, "    exit_code: %d\n", entry.ExitCode)
			fmt.Fprintf(&out, "    status: %s\n", entry.Status)
		}
	}
	out.WriteString("metrics:\n")
	out.WriteString("  input_tokens: 0\n")
	out.WriteString("  output_tokens: 0\n")
	out.WriteString("  duration_seconds: 0\n")
	out.WriteString("---\n")
	out.WriteString("## Summary\nImplemented the selected task.\n\n")
	out.WriteString("## Changed Files\n")
	if len(opts.ChangedFiles) == 0 {
		out.WriteString("None.\n")
	} else {
		for _, path := range opts.ChangedFiles {
			fmt.Fprintf(&out, "- %s\n", path)
		}
	}
	out.WriteString("\n## Verification\n")
	if len(opts.Verification) == 0 {
		out.WriteString("- Not run yet.\n")
	} else {
		for _, entry := range opts.Verification {
			fmt.Fprintf(&out, "- `%s` (%s, exit %d)\n", entry.Command, entry.Status, entry.ExitCode)
		}
	}
	out.WriteString("\n## Concerns\nNone.\n\n")
	out.WriteString("## Next Steps\nNone.\n")
	return out.String()
}

func readContextPayload(t *testing.T, reader io.Reader) string {
	t.Helper()
	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read context payload: %v", err)
	}
	return string(content)
}

func contextPayloadValue(t *testing.T, payload string, label string) string {
	t.Helper()
	prefix := "- " + label + ": `"
	for _, line := range strings.Split(payload, "\n") {
		if strings.HasPrefix(line, prefix) {
			value := strings.TrimPrefix(line, prefix)
			value = strings.TrimSuffix(value, "`")
			return value
		}
	}
	t.Fatalf("context payload missing %s:\n%s", label, payload)
	return ""
}

func contextPayloadTaskText(t *testing.T, payload string) string {
	t.Helper()
	lines := strings.Split(payload, "\n")
	for i, line := range lines {
		if line != "- Task text:" {
			continue
		}
		if i+2 >= len(lines) || lines[i+2] != "```text" {
			t.Fatalf("context payload task text block malformed:\n%s", payload)
		}
		start := i + 3
		for end := start; end < len(lines); end++ {
			if lines[end] == "```" {
				return strings.Join(lines[start:end], "\n")
			}
		}
		t.Fatalf("context payload task text block not closed:\n%s", payload)
	}
	t.Fatalf("context payload missing task text:\n%s", payload)
	return ""
}

func writeTestFile(path string, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func receiptFromFile(path string) (receipt.Receipt, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return receipt.Receipt{}, err
	}
	return receipt.Parse(content)
}

func argAfter(args []string, flag string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}

func containsArg(args []string, value string) bool {
	for _, arg := range args {
		if arg == value {
			return true
		}
	}
	return false
}

func manifestSourceByLabel(t *testing.T, manifest prompt.ContextManifest, label string) prompt.ContextSource {
	t.Helper()
	for _, source := range manifest.Sources {
		if source.Label == label {
			return source
		}
	}
	t.Fatalf("manifest source %q not found: %+v", label, manifest.Sources)
	return prompt.ContextSource{}
}

func readContextManifestArtifact(t *testing.T, env testEnv, runID string) prompt.ContextManifest {
	t.Helper()
	contextManifestPath := filepath.Join(env.workDir, ".revolvr", "runs", runID, "context.json")
	manifestBytes, err := os.ReadFile(contextManifestPath)
	if err != nil {
		t.Fatalf("read context manifest artifact: %v", err)
	}
	var manifest prompt.ContextManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("unmarshal context manifest: %v\n%s", err, manifestBytes)
	}
	return manifest
}

func sha256HexTest(content []byte) string {
	sum := sha256.Sum256(content)
	return fmt.Sprintf("%x", sum)
}

func assertRunEvents(t *testing.T, store *ledger.Store, runID string, want []ledger.EventType) {
	t.Helper()
	history, ok, err := store.GetRunWithEvents(context.Background(), runID)
	if err != nil {
		t.Fatalf("get run with events: %v", err)
	}
	if !ok {
		t.Fatal("run history not found")
	}
	got := make([]ledger.EventType, 0, len(history.Events))
	for _, event := range history.Events {
		got = append(got, event.Type)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("event types = %#v, want %#v", got, want)
	}
}

func receiptWarningEvents(t *testing.T, store *ledger.Store, runID string) []ledger.Event {
	t.Helper()
	history, ok, err := store.GetRunWithEvents(context.Background(), runID)
	if err != nil {
		t.Fatalf("get run with events: %v", err)
	}
	if !ok {
		t.Fatal("run history not found")
	}
	var warnings []ledger.Event
	for _, event := range history.Events {
		if event.Type == ledger.EventReceiptWarning {
			warnings = append(warnings, event)
		}
	}
	return warnings
}

func decodeEventPayload(t *testing.T, event ledger.Event, target any) {
	t.Helper()
	if err := json.Unmarshal(event.Payload, target); err != nil {
		t.Fatalf("decode %s payload: %v", event.Type, err)
	}
}

func decodeTestEventPayload(t *testing.T, events []ledger.Event, eventType ledger.EventType, target any) bool {
	t.Helper()
	for _, event := range events {
		if event.Type != eventType {
			continue
		}
		if err := json.Unmarshal(event.Payload, target); err != nil {
			t.Fatalf("decode %s payload: %v", eventType, err)
		}
		return true
	}
	return false
}
