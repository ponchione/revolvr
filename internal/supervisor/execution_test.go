package supervisor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomouspolicy"
	"revolvr/internal/codexexec"
	"revolvr/internal/gitstate"
	"revolvr/internal/ledger"
	"revolvr/internal/runner"
)

func TestRunAcceptsEverySupervisorAction(t *testing.T) {
	tests := []struct {
		name       string
		action     autonomous.Action
		profile    autonomous.WorkerProfile
		findingIDs []string
		audit      *autonomous.AuditReport
	}{
		{name: "plan", action: autonomous.ActionPlan, profile: autonomous.WorkerProfilePlanner},
		{name: "implement", action: autonomous.ActionImplement, profile: autonomous.WorkerProfileImplementer},
		{name: "audit", action: autonomous.ActionAudit, profile: autonomous.WorkerProfileAuditor},
		{name: "correct", action: autonomous.ActionCorrect, profile: autonomous.WorkerProfileCorrector, findingIDs: []string{"finding-001"}, audit: testAudit("finding-001")},
		{name: "document", action: autonomous.ActionDocument, profile: autonomous.WorkerProfileDocumentor},
		{name: "simplify", action: autonomous.ActionSimplify, profile: autonomous.WorkerProfileSimplifier},
		{name: "complete", action: autonomous.ActionComplete},
		{name: "block", action: autonomous.ActionBlock},
		{name: "needs-input", action: autonomous.ActionNeedsInput},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newPassFixture(t, "run-"+tt.name)
			defer fixture.close()
			if tt.action == autonomous.ActionPlan || tt.action == autonomous.ActionBlock || tt.action == autonomous.ActionNeedsInput {
				fixture.cfg.Lifecycle = autonomous.LifecycleStatePending
			}
			decision := testDecision(tt.action, tt.profile)
			decision.FindingIDs = tt.findingIDs
			rawJSON, err := json.Marshal(decision)
			if err != nil {
				t.Fatal(err)
			}
			raw := append([]byte(" \n"), rawJSON...)
			raw = append(raw, '\n')
			var command runner.Command
			fixture.cfg.Audit = tt.audit
			fixture.cfg.CodexCommandRunner = fakeDecisionRunner(t, raw, nil, runner.Result{ExitCode: 0}, &command)

			beforeTask := readTestFile(t, filepath.Join(fixture.root, ".agent", "tasks", "task-1.md"))
			result, err := Run(context.Background(), fixture.cfg)
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if result.Decision == nil || !reflect.DeepEqual(*result.Decision, decision) {
				t.Fatalf("decision = %#v, want %#v", result.Decision, decision)
			}
			if result.DecisionReference == nil || result.DecisionReference.TaskID != "task-1" || result.DecisionReference.RunID != fixture.cfg.RunID {
				t.Fatalf("decision reference = %#v", result.DecisionReference)
			}
			if result.LedgerRun.Status != ledger.StatusCompleted || result.LedgerRun.VerificationStatus != "not_run" || result.LedgerRun.CommitSHA != "" {
				t.Fatalf("ledger run = %+v", result.LedgerRun)
			}
			if got := readTestFile(t, filepath.Join(fixture.root, filepath.FromSlash(result.Artifacts.RawOutput.Path))); !bytes.Equal(got, raw) {
				t.Fatalf("raw output = %q, want %q", got, raw)
			}
			canonical, err := marshalIndented(decision)
			if err != nil {
				t.Fatal(err)
			}
			if got := readTestFile(t, filepath.Join(fixture.root, filepath.FromSlash(result.Artifacts.Decision.Path))); !bytes.Equal(got, canonical) {
				t.Fatalf("canonical decision = %q, want %q", got, canonical)
			}
			assertArtifactHash(t, fixture.root, result.Artifacts.Prompt)
			assertArtifactHash(t, fixture.root, result.Artifacts.Dossier)
			assertArtifactHash(t, fixture.root, result.Artifacts.DossierManifest)
			assertArtifactHash(t, fixture.root, result.Artifacts.Schema)
			assertArtifactHash(t, fixture.root, result.Artifacts.RawOutput)
			assertArtifactHash(t, fixture.root, result.Artifacts.Decision)
			assertArtifactHash(t, fixture.root, result.Artifacts.Provenance)
			assertArtifactHash(t, fixture.root, result.Artifacts.SourceEvidence)
			if result.Dossier.SHA256 != fixture.cfg.Dossier.Manifest.DossierSHA256 || result.Profile.Name != SupervisorProfileName || result.Profile.Path != ".agent/profiles/supervisor.md" {
				t.Fatalf("dossier/profile provenance = %+v / %+v", result.Dossier, result.Profile)
			}
			wantAuthority, err := autonomouspolicy.RoutingAuthorityForLifecycle(fixture.cfg.Lifecycle)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(result.RoutingAuthority, wantAuthority) || !result.RoutingAuthority.Admits(tt.action) {
				t.Fatalf("routing authority = %+v, want %+v admitting %q", result.RoutingAuthority, wantAuthority, tt.action)
			}
			var provenance supervisorProvenance
			if err := json.Unmarshal(readTestFile(t, filepath.Join(fixture.root, filepath.FromSlash(result.Artifacts.Provenance.Path))), &provenance); err != nil {
				t.Fatal(err)
			}
			if provenance.SchemaVersion != supervisorProvenanceSchemaVersion || !reflect.DeepEqual(provenance.RoutingAuthority, wantAuthority) {
				t.Fatalf("provenance lifecycle authority = %q / %+v, want %q / %+v", provenance.SchemaVersion, provenance.RoutingAuthority, supervisorProvenanceSchemaVersion, wantAuthority)
			}
			if result.Invocation.Model != codexexec.DefaultModel || result.Invocation.ReasoningEffort != codexexec.DefaultReasoningEffort || !result.Invocation.Ephemeral {
				t.Fatalf("invocation = %+v", result.Invocation)
			}
			assertInvocationFlags(t, command.Args)
			if !reflect.DeepEqual(result.Invocation.Argv, command.Args) {
				t.Fatalf("recorded argv = %#v, command argv = %#v", result.Invocation.Argv, command.Args)
			}
			if got := readTestFile(t, filepath.Join(fixture.root, ".agent", "tasks", "task-1.md")); !bytes.Equal(got, beforeTask) {
				t.Fatal("task file changed during supervisor pass")
			}
			history := runHistory(t, fixture.store, fixture.cfg.RunID)
			assertEventTypes(t, history.Events, []ledger.EventType{
				ledger.EventRunStarted,
				ledger.EventSupervisorPrepared,
				ledger.EventCodexStarted,
				ledger.EventCodexCompleted,
				ledger.EventSupervisorValidated,
				ledger.EventRunCompleted,
			})
			assertNoWorkerLifecycleEvents(t, history.Events)
			artifacts, ok := ledger.RunArtifactsFromEvents(history.Events)
			if !ok || artifacts.SupervisorPromptPath != result.Artifacts.Prompt.Path || artifacts.SupervisorDossierPath != result.Artifacts.Dossier.Path || artifacts.SupervisorDossierManifestPath != result.Artifacts.DossierManifest.Path || artifacts.SupervisorDecisionPath != result.Artifacts.Decision.Path || artifacts.ReceiptPath != "" {
				t.Fatalf("ledger artifacts = %+v, found=%t", artifacts, ok)
			}
		})
	}
}

func TestRunRejectsInvalidDecisionOutput(t *testing.T) {
	base := testDecision(autonomous.ActionImplement, autonomous.WorkerProfileImplementer)
	encode := func(mutate func(*autonomous.SupervisorDecision)) []byte {
		decision := base
		decision.Inputs = append([]autonomous.EvidenceReference(nil), base.Inputs...)
		decision.SuccessCriteria = append([]string(nil), base.SuccessCriteria...)
		mutate(&decision)
		raw, err := json.Marshal(decision)
		if err != nil {
			panic(err)
		}
		return raw
	}
	unknownField := func() []byte {
		var object map[string]any
		raw, _ := json.Marshal(base)
		_ = json.Unmarshal(raw, &object)
		object["unexpected"] = true
		result, _ := json.Marshal(object)
		return result
	}()
	tests := []struct {
		name  string
		raw   []byte
		audit *autonomous.AuditReport
	}{
		{name: "empty", raw: []byte{}},
		{name: "malformed", raw: []byte(`{"task_id":`)},
		{name: "markdown fenced", raw: []byte("```json\n{}\n```\n")},
		{name: "leading prose", raw: append([]byte("decision: "), encode(func(*autonomous.SupervisorDecision) {})...)},
		{name: "trailing prose", raw: append(encode(func(*autonomous.SupervisorDecision) {}), []byte(" done")...)},
		{name: "two objects", raw: append(append(encode(func(*autonomous.SupervisorDecision) {}), '\n'), encode(func(*autonomous.SupervisorDecision) {})...)},
		{name: "unknown field", raw: unknownField},
		{name: "unknown action", raw: encode(func(d *autonomous.SupervisorDecision) { d.Action = "review" })},
		{name: "missing worker", raw: encode(func(d *autonomous.SupervisorDecision) { d.WorkerProfile = "" })},
		{name: "incompatible worker", raw: encode(func(d *autonomous.SupervisorDecision) { d.WorkerProfile = autonomous.WorkerProfileAuditor })},
		{name: "terminal worker", raw: encode(func(d *autonomous.SupervisorDecision) { d.Action = autonomous.ActionComplete })},
		{name: "missing rationale", raw: encode(func(d *autonomous.SupervisorDecision) { d.Rationale = "" })},
		{name: "missing evidence", raw: encode(func(d *autonomous.SupervisorDecision) { d.Inputs = nil })},
		{name: "unknown evidence", raw: encode(func(d *autonomous.SupervisorDecision) { d.Inputs[0].Kind = "chat" })},
		{name: "missing success criteria", raw: encode(func(d *autonomous.SupervisorDecision) { d.SuccessCriteria = nil })},
		{name: "invalid finding id", raw: encode(func(d *autonomous.SupervisorDecision) {
			d.Action = autonomous.ActionCorrect
			d.WorkerProfile = autonomous.WorkerProfileCorrector
			d.FindingIDs = []string{"Finding_1"}
		}), audit: testAudit("finding-001")},
		{name: "duplicate finding id", raw: encode(func(d *autonomous.SupervisorDecision) {
			d.Action = autonomous.ActionCorrect
			d.WorkerProfile = autonomous.WorkerProfileCorrector
			d.FindingIDs = []string{"finding-001", "finding-001"}
		}), audit: testAudit("finding-001")},
		{name: "finding on non-correction", raw: encode(func(d *autonomous.SupervisorDecision) { d.FindingIDs = []string{"finding-001"} })},
		{name: "correct without audit", raw: encode(func(d *autonomous.SupervisorDecision) {
			d.Action = autonomous.ActionCorrect
			d.WorkerProfile = autonomous.WorkerProfileCorrector
			d.FindingIDs = []string{"finding-001"}
		})},
		{name: "correct unknown finding", raw: encode(func(d *autonomous.SupervisorDecision) {
			d.Action = autonomous.ActionCorrect
			d.WorkerProfile = autonomous.WorkerProfileCorrector
			d.FindingIDs = []string{"finding-002"}
		}), audit: testAudit("finding-001")},
		{name: "wrong task", raw: encode(func(d *autonomous.SupervisorDecision) { d.TaskID = "task-2" })},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newPassFixture(t, "run-invalid-"+strings.ReplaceAll(tt.name, " ", "-"))
			defer fixture.close()
			fixture.cfg.Audit = tt.audit
			fixture.cfg.CodexCommandRunner = fakeDecisionRunner(t, tt.raw, nil, runner.Result{ExitCode: 0}, nil)
			beforeTask := readTestFile(t, filepath.Join(fixture.root, ".agent", "tasks", "task-1.md"))
			result, err := Run(context.Background(), fixture.cfg)
			if err == nil || result.Decision != nil || result.DecisionReference != nil {
				t.Fatalf("Run() = decision %#v reference %#v error %v", result.Decision, result.DecisionReference, err)
			}
			if result.LedgerRun.Status != ledger.StatusFailed || result.LedgerRun.VerificationStatus != "not_run" || result.LedgerRun.CommitSHA != "" {
				t.Fatalf("failed ledger run = %+v", result.LedgerRun)
			}
			if _, statErr := os.Stat(filepath.Join(fixture.root, filepath.FromSlash(result.Artifacts.Diagnostics.Path))); statErr != nil {
				t.Fatalf("diagnostics missing: %v", statErr)
			}
			if _, statErr := os.Stat(filepath.Join(fixture.root, filepath.FromSlash(result.Artifacts.Decision.Path))); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("canonical decision artifact exists for rejected output: %v", statErr)
			}
			if got := readTestFile(t, filepath.Join(fixture.root, ".agent", "tasks", "task-1.md")); !bytes.Equal(got, beforeTask) {
				t.Fatal("task file changed on rejected output")
			}
			history := runHistory(t, fixture.store, fixture.cfg.RunID)
			assertEventTypes(t, history.Events, []ledger.EventType{
				ledger.EventRunStarted,
				ledger.EventSupervisorPrepared,
				ledger.EventCodexStarted,
				ledger.EventCodexCompleted,
				ledger.EventSupervisorRejected,
				ledger.EventRunFailed,
			})
			assertNoWorkerLifecycleEvents(t, history.Events)
		})
	}
}

func TestRunRejectsEverySourceMutationClassWithoutReverting(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(string)
		mutate         func(string)
		assertMutation func(*testing.T, string)
	}{
		{
			name:           "tracked modification",
			mutate:         func(root string) { mustWriteTestFile(t, filepath.Join(root, "tracked.txt"), []byte("mutated\n")) },
			assertMutation: func(t *testing.T, root string) { assertTestFile(t, filepath.Join(root, "tracked.txt"), "mutated\n") },
		},
		{
			name: "staged modification",
			mutate: func(root string) {
				mustWriteTestFile(t, filepath.Join(root, "tracked.txt"), []byte("staged\n"))
				runGit(t, root, "add", "tracked.txt")
			},
			assertMutation: func(t *testing.T, root string) {
				if status := gitOutput(t, root, "status", "--short"); !strings.Contains(status, "M  tracked.txt") {
					t.Fatalf("status = %q", status)
				}
			},
		},
		{
			name:           "new untracked file",
			mutate:         func(root string) { mustWriteTestFile(t, filepath.Join(root, "new.go"), []byte("package newfile\n")) },
			assertMutation: func(t *testing.T, root string) { assertTestFile(t, filepath.Join(root, "new.go"), "package newfile\n") },
		},
		{
			name: "deleted tracked file",
			mutate: func(root string) {
				if err := os.Remove(filepath.Join(root, "tracked.txt")); err != nil {
					t.Fatal(err)
				}
			},
			assertMutation: func(t *testing.T, root string) {
				if _, err := os.Stat(filepath.Join(root, "tracked.txt")); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("tracked file restored: %v", err)
				}
			},
		},
		{
			name: "renamed tracked file",
			mutate: func(root string) {
				if err := os.Rename(filepath.Join(root, "tracked.txt"), filepath.Join(root, "renamed.txt")); err != nil {
					t.Fatal(err)
				}
			},
			assertMutation: func(t *testing.T, root string) { assertTestFile(t, filepath.Join(root, "renamed.txt"), "baseline\n") },
		},
		{
			name:   "already dirty path changes again",
			setup:  func(root string) { mustWriteTestFile(t, filepath.Join(root, "tracked.txt"), []byte("dirty before\n")) },
			mutate: func(root string) { mustWriteTestFile(t, filepath.Join(root, "tracked.txt"), []byte("dirty after\n")) },
			assertMutation: func(t *testing.T, root string) {
				assertTestFile(t, filepath.Join(root, "tracked.txt"), "dirty after\n")
			},
		},
		{
			name: "HEAD change",
			mutate: func(root string) {
				mustWriteTestFile(t, filepath.Join(root, "tracked.txt"), []byte("committed by fake\n"))
				runGit(t, root, "add", "tracked.txt")
				runGit(t, root, "commit", "-qm", "fake mutation")
			},
			assertMutation: func(t *testing.T, root string) {
				if strings.Count(gitOutput(t, root, "log", "--oneline"), "\n") < 2 {
					t.Fatal("fake commit was reverted")
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newPassFixture(t, "run-mutation-"+strings.ReplaceAll(tt.name, " ", "-"))
			defer fixture.close()
			if tt.setup != nil {
				tt.setup(fixture.root)
			}
			decision := testDecision(autonomous.ActionImplement, autonomous.WorkerProfileImplementer)
			raw, _ := json.Marshal(decision)
			fixture.cfg.CodexCommandRunner = fakeDecisionRunner(t, raw, tt.mutate, runner.Result{ExitCode: 0}, nil)
			result, err := Run(context.Background(), fixture.cfg)
			if err == nil || result.Decision != nil || !result.SourceDifference.Changed {
				t.Fatalf("Run() = decision %#v changed=%t error=%v", result.Decision, result.SourceDifference.Changed, err)
			}
			tt.assertMutation(t, fixture.root)
			history := runHistory(t, fixture.store, fixture.cfg.RunID)
			assertEventTypes(t, history.Events, []ledger.EventType{
				ledger.EventRunStarted,
				ledger.EventSupervisorPrepared,
				ledger.EventCodexStarted,
				ledger.EventCodexCompleted,
				ledger.EventSupervisorMutation,
				ledger.EventSupervisorRejected,
				ledger.EventRunFailed,
			})
			if history.Run.Status != ledger.StatusFailed || history.Run.CommitSHA != "" {
				t.Fatalf("history run = %+v", history.Run)
			}
		})
	}
}

func TestRunRejectsSourceCaptureUncertaintyAndCodexFailureWithMutation(t *testing.T) {
	t.Run("baseline failure", func(t *testing.T) {
		fixture := newPassFixture(t, "run-baseline-failure")
		defer fixture.close()
		called := false
		fixture.cfg.CodexCommandRunner = func(context.Context, runner.Command) runner.Result { called = true; return runner.Result{} }
		fixture.cfg.SourceSnapshotter = func(context.Context, gitstate.SourceSnapshotConfig) (gitstate.SourceSnapshot, error) {
			return gitstate.SourceSnapshot{}, errors.New("source capture failed")
		}
		result, err := Run(context.Background(), fixture.cfg)
		if err == nil || result.Decision != nil || called {
			t.Fatalf("Run() decision=%#v called=%t error=%v", result.Decision, called, err)
		}
		assertEventTypes(t, runHistory(t, fixture.store, fixture.cfg.RunID).Events, []ledger.EventType{
			ledger.EventRunStarted, ledger.EventSupervisorPrepared, ledger.EventSupervisorRejected, ledger.EventRunFailed,
		})
	})

	t.Run("post capture failure", func(t *testing.T) {
		fixture := newPassFixture(t, "run-post-failure")
		defer fixture.close()
		calls := 0
		fixture.cfg.SourceSnapshotter = func(ctx context.Context, cfg gitstate.SourceSnapshotConfig) (gitstate.SourceSnapshot, error) {
			calls++
			if calls == 2 {
				return gitstate.SourceSnapshot{}, errors.New("post capture uncertain")
			}
			return gitstate.CaptureSourceSnapshot(ctx, cfg)
		}
		raw, _ := json.Marshal(testDecision(autonomous.ActionImplement, autonomous.WorkerProfileImplementer))
		fixture.cfg.CodexCommandRunner = fakeDecisionRunner(t, raw, nil, runner.Result{ExitCode: 0}, nil)
		result, err := Run(context.Background(), fixture.cfg)
		if err == nil || result.Decision != nil || calls != 2 {
			t.Fatalf("Run() decision=%#v calls=%d error=%v", result.Decision, calls, err)
		}
	})

	t.Run("truncated capture", func(t *testing.T) {
		fixture := newPassFixture(t, "run-truncated-capture")
		defer fixture.close()
		fixture.cfg.SourceSnapshotter = gitstate.CaptureSourceSnapshot
		fixture.cfg.GitCommandRunner = func(context.Context, runner.Command) runner.Result {
			return runner.Result{ExitCode: 0, Stdout: "partial", StdoutTruncatedBytes: 10}
		}
		called := false
		fixture.cfg.CodexCommandRunner = func(context.Context, runner.Command) runner.Result { called = true; return runner.Result{} }
		result, err := Run(context.Background(), fixture.cfg)
		if err == nil || result.Decision != nil || called || !strings.Contains(err.Error(), "truncated") {
			t.Fatalf("Run() decision=%#v called=%t error=%v", result.Decision, called, err)
		}
	})

	t.Run("invalid snapshot uncertainty", func(t *testing.T) {
		fixture := newPassFixture(t, "run-invalid-snapshot")
		defer fixture.close()
		fixture.cfg.SourceSnapshotter = func(context.Context, gitstate.SourceSnapshotConfig) (gitstate.SourceSnapshot, error) {
			return gitstate.SourceSnapshot{}, nil
		}
		called := false
		fixture.cfg.CodexCommandRunner = func(context.Context, runner.Command) runner.Result { called = true; return runner.Result{} }
		result, err := Run(context.Background(), fixture.cfg)
		if err == nil || result.Decision != nil || called || !strings.Contains(err.Error(), "source snapshot") {
			t.Fatalf("Run() decision=%#v called=%t error=%v", result.Decision, called, err)
		}
	})

	t.Run("mutation and Codex failure", func(t *testing.T) {
		fixture := newPassFixture(t, "run-mutation-codex-failure")
		defer fixture.close()
		raw, _ := json.Marshal(testDecision(autonomous.ActionImplement, autonomous.WorkerProfileImplementer))
		fixture.cfg.CodexCommandRunner = fakeDecisionRunner(t, raw, func(root string) {
			mustWriteTestFile(t, filepath.Join(root, "tracked.txt"), []byte("mutation with failure\n"))
		}, runner.Result{ExitCode: 2, Stderr: "failed\n"}, nil)
		result, err := Run(context.Background(), fixture.cfg)
		if err == nil || result.Decision != nil || !result.SourceDifference.Changed {
			t.Fatalf("Run() decision=%#v difference=%+v error=%v", result.Decision, result.SourceDifference, err)
		}
		if !strings.Contains(err.Error(), "changed repository source") {
			t.Fatalf("mutation should take safe-refusal precedence: %v", err)
		}
		assertTestFile(t, filepath.Join(fixture.root, "tracked.txt"), "mutation with failure\n")
	})
}

func TestRunRecordsCodexTimeoutEvidence(t *testing.T) {
	fixture := newPassFixture(t, "run-timeout")
	defer fixture.close()
	fixture.cfg.CodexCommandRunner = fakeDecisionRunner(t, nil, nil, runner.Result{ExitCode: -1, TimedOut: true, Err: context.DeadlineExceeded}, nil)
	result, err := Run(context.Background(), fixture.cfg)
	if err == nil || result.Decision != nil || !result.Codex.TimedOut {
		t.Fatalf("Run() result=%+v error=%v", result, err)
	}
	diagnostics := readTestFile(t, filepath.Join(fixture.root, filepath.FromSlash(result.Artifacts.Diagnostics.Path)))
	if !bytes.Contains(diagnostics, []byte(`"timed_out": true`)) {
		t.Fatalf("diagnostics = %s", diagnostics)
	}
}

func TestRunRecordsCodexNonzeroFailureEvidence(t *testing.T) {
	fixture := newPassFixture(t, "run-codex-failure")
	defer fixture.close()
	fixture.cfg.CodexCommandRunner = fakeDecisionRunner(t, nil, nil, runner.Result{ExitCode: 7, Stderr: "fake failure\n"}, nil)
	result, err := Run(context.Background(), fixture.cfg)
	if err == nil || result.Decision != nil || result.Codex.ExitCode != 7 || !strings.Contains(err.Error(), "exited with code 7") {
		t.Fatalf("Run() result=%+v error=%v", result, err)
	}
	history := runHistory(t, fixture.store, fixture.cfg.RunID)
	assertEventTypes(t, history.Events, []ledger.EventType{
		ledger.EventRunStarted,
		ledger.EventSupervisorPrepared,
		ledger.EventCodexStarted,
		ledger.EventCodexCompleted,
		ledger.EventSupervisorRejected,
		ledger.EventRunFailed,
	})
	if history.Run.Status != ledger.StatusFailed || history.Run.CodexExitCode == nil || *history.Run.CodexExitCode != 7 {
		t.Fatalf("failed run = %+v", history.Run)
	}
	diagnostics := readTestFile(t, filepath.Join(fixture.root, filepath.FromSlash(result.Artifacts.Diagnostics.Path)))
	if !bytes.Contains(diagnostics, []byte(`"exit_code": 7`)) {
		t.Fatalf("diagnostics = %s", diagnostics)
	}
}

func TestRunBlocksBeforeCodexOnInvalidPreparation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*passFixture)
	}{
		{name: "dossier hash", mutate: func(f *passFixture) { f.cfg.Dossier.Manifest.DossierSHA256 = strings.Repeat("0", 64) }},
		{name: "dossier task", mutate: func(f *passFixture) { f.cfg.Dossier.Manifest.TaskID = "task-2" }},
		{name: "audit task", mutate: func(f *passFixture) { f.cfg.Audit = testAudit("finding-001"); f.cfg.Audit.TaskID = "task-2" }},
		{name: "missing profile", mutate: func(f *passFixture) {
			if err := os.Remove(filepath.Join(f.root, ".agent", "profiles", "supervisor.md")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "empty version", mutate: func(f *passFixture) { f.cfg.CodexVersion = "" }},
		{name: "invalid config hash", mutate: func(f *passFixture) { f.cfg.EffectiveConfigSHA256 = "short" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newPassFixture(t, "run-preparation-"+strings.ReplaceAll(tt.name, " ", "-"))
			defer fixture.close()
			tt.mutate(&fixture)
			called := false
			fixture.cfg.CodexCommandRunner = func(context.Context, runner.Command) runner.Result {
				called = true
				return runner.Result{}
			}
			result, err := Run(context.Background(), fixture.cfg)
			if err == nil || result.Decision != nil || called {
				t.Fatalf("Run() decision=%#v called=%t error=%v", result.Decision, called, err)
			}
			if result.LedgerRun.Status != ledger.StatusFailed {
				t.Fatalf("ledger run = %+v", result.LedgerRun)
			}
		})
	}
}

func TestRunFailsClosedBeforeLedgerOrCodexWhenLifecycleAdmitsNoRouting(t *testing.T) {
	fixture := newPassFixture(t, "run-working-lifecycle")
	defer fixture.close()
	fixture.cfg.Lifecycle = autonomous.LifecycleStateWorking
	called := false
	fixture.cfg.CodexCommandRunner = func(context.Context, runner.Command) runner.Result {
		called = true
		return runner.Result{}
	}
	result, err := Run(context.Background(), fixture.cfg)
	if err == nil || !strings.Contains(err.Error(), "operation in flight") || result.RunID != "" || called {
		t.Fatalf("Run() result=%+v called=%t error=%v", result, called, err)
	}
	if _, found, err := fixture.store.GetRun(context.Background(), fixture.cfg.RunID); err != nil || found {
		t.Fatalf("closed lifecycle ledger lookup = found %t, error %v", found, err)
	}
}

func TestRunSurfacesLedgerWriteFailure(t *testing.T) {
	fixture := newPassFixture(t, "run-ledger-failure")
	defer fixture.close()
	fixture.cfg.Ledger = &eventFailingLedger{Store: fixture.store, failType: ledger.EventSupervisorPrepared}
	called := false
	fixture.cfg.CodexCommandRunner = func(context.Context, runner.Command) runner.Result {
		called = true
		return runner.Result{}
	}
	result, err := Run(context.Background(), fixture.cfg)
	if err == nil || !strings.Contains(err.Error(), "injected ledger failure") || result.Decision != nil || called {
		t.Fatalf("Run() decision=%#v called=%t error=%v", result.Decision, called, err)
	}
	if result.LedgerRun.Status != ledger.StatusFailed {
		t.Fatalf("ledger run = %+v", result.LedgerRun)
	}
}

func TestSupervisorArtifactsRemainReadableAfterLedgerReopen(t *testing.T) {
	fixture := newPassFixture(t, "run-reopen")
	decision := testDecision(autonomous.ActionPlan, autonomous.WorkerProfilePlanner)
	raw, _ := json.Marshal(decision)
	fixture.cfg.CodexCommandRunner = fakeDecisionRunner(t, raw, nil, runner.Result{ExitCode: 0}, nil)
	result, err := Run(context.Background(), fixture.cfg)
	if err != nil {
		fixture.close()
		t.Fatal(err)
	}
	ledgerPath := filepath.Join(fixture.root, ".revolvr", "ledger.sqlite")
	fixture.close()
	reopened, err := ledger.OpenLiveReadOnly(context.Background(), ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	history, ok, err := reopened.GetRunWithEvents(context.Background(), fixture.cfg.RunID)
	if err != nil || !ok {
		t.Fatalf("reopen history = found %t error %v", ok, err)
	}
	artifacts, found := ledger.RunArtifactsFromEvents(history.Events)
	if !found || artifacts.SupervisorOutputPath != result.Artifacts.RawOutput.Path || artifacts.SupervisorDecisionPath != result.Artifacts.Decision.Path {
		t.Fatalf("reopened artifacts = %+v, found=%t", artifacts, found)
	}
	for _, path := range []string{artifacts.SupervisorPromptPath, artifacts.SupervisorSchemaPath, artifacts.SupervisorOutputPath, artifacts.SupervisorDecisionPath, artifacts.SupervisorProvenancePath, artifacts.SupervisorSourcePath} {
		if _, err := os.ReadFile(filepath.Join(fixture.root, filepath.FromSlash(path))); err != nil {
			t.Fatalf("read reopened artifact %q: %v", path, err)
		}
	}
}

type passFixture struct {
	root  string
	store *ledger.Store
	cfg   Config
}

type eventFailingLedger struct {
	*ledger.Store
	failType ledger.EventType
}

func (l *eventFailingLedger) AppendEvent(ctx context.Context, runID string, eventType ledger.EventType, payload any) (ledger.Event, error) {
	if eventType == l.failType {
		l.failType = ""
		return ledger.Event{}, errors.New("injected ledger failure")
	}
	return l.Store.AppendEvent(ctx, runID, eventType, payload)
}

func newPassFixture(t *testing.T, runID string) passFixture {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init", "-q")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "Test User")
	mustWriteTestFile(t, filepath.Join(root, "tracked.txt"), []byte("baseline\n"))
	mustWriteTestFile(t, filepath.Join(root, ".gitignore"), []byte(".revolvr/\n"))
	mustWriteTestFile(t, filepath.Join(root, ".agent", "profiles", "supervisor.md"), []byte("Exact supervisor test profile.\nDecision only; do not edit source.\n"))
	mustWriteTestFile(t, filepath.Join(root, ".agent", "tasks", "task-1.md"), []byte("---\nid: task-1\nstatus: pending\n---\n# Task one\n\nDo the work.\n"))
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-qm", "baseline")
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	store, err := ledger.OpenWithClock(context.Background(), filepath.Join(root, ".revolvr", "ledger.sqlite"), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return passFixture{
		root:  root,
		store: store,
		cfg: Config{
			RepositoryRoot:        root,
			TaskID:                "task-1",
			Lifecycle:             autonomous.LifecycleStateReady,
			Dossier:               testDossier([]byte("# Validated task dossier\n\nExact evidence.\n")),
			Ledger:                store,
			RunID:                 runID,
			DecisionID:            "decision-" + strings.ToLower(strings.TrimPrefix(runID, "run-")),
			Clock:                 func() time.Time { return now },
			CodexExecutable:       "fake-codex",
			CodexModel:            codexexec.DefaultModel,
			CodexReasoningEffort:  codexexec.DefaultReasoningEffort,
			CodexEphemeral:        true,
			CodexSandbox:          "workspace-write",
			CodexApprovalPolicy:   "never",
			CodexVersion:          "codex-cli test",
			EffectiveConfigSchema: "revolvr-effective-run-config-v1",
			EffectiveConfigSHA256: strings.Repeat("a", 64),
			CodexTimeout:          2 * time.Second,
			GitTimeout:            2 * time.Second,
		},
	}
}

func (f passFixture) close() {
	_ = f.store.Close()
}

func testDecision(action autonomous.Action, profile autonomous.WorkerProfile) autonomous.SupervisorDecision {
	decision := autonomous.SupervisorDecision{
		TaskID:          "task-1",
		Action:          action,
		WorkerProfile:   profile,
		Rationale:       "Durable dossier evidence supports this next action.",
		SuccessCriteria: []string{"The selected action records concrete durable evidence."},
		Inputs: []autonomous.EvidenceReference{{
			Kind:      autonomous.EvidenceKindTask,
			Reference: ".agent/tasks/task-1.md",
			Detail:    "The validated dossier identifies the task and current evidence.",
		}},
	}
	if action == autonomous.ActionComplete || action == autonomous.ActionBlock || action == autonomous.ActionNeedsInput {
		decision.SuccessCriteria = nil
	}
	if action == autonomous.ActionNeedsInput {
		question := autonomous.NeedsInputQuestion{TaskID: "task-1", QuestionID: "product-mode", Revision: 1, Question: "Which product behavior?", BlockingReason: "The task permits incompatible behaviors.", Options: []autonomous.NeedsInputOption{{ID: "keep", Meaning: "Keep current behavior."}, {ID: "change", Meaning: "Adopt changed behavior."}}, Recommendation: autonomous.NeedsInputRecommendation{OptionID: "keep", Rationale: "Preserves compatibility."}, Evidence: append([]autonomous.EvidenceReference(nil), decision.Inputs...)}
		hash, _ := autonomous.QuestionContentSHA256(question)
		question.ContentSHA256 = hash
		decision.NeedsInput = &question
	}
	return decision
}

func testAudit(findingID string) *autonomous.AuditReport {
	return &autonomous.AuditReport{
		TaskID:      "task-1",
		Disposition: autonomous.AuditDispositionChangesRequired,
		Rationale:   "The current independent audit requires a correction.",
		Inputs:      []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindAudit, Reference: "audit-1", Detail: "The audit is current."}},
		Findings: []autonomous.AuditFinding{{
			ID:                 findingID,
			Significance:       autonomous.FindingSignificanceBlocking,
			Summary:            "A concrete defect remains.",
			Evidence:           []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindFile, Reference: "tracked.txt", Detail: "The tracked file demonstrates the defect."}},
			RequiredCorrection: "Correct the defect and add evidence.",
		}},
	}
}

func fakeDecisionRunner(t *testing.T, raw []byte, mutate func(string), result runner.Result, captured *runner.Command) codexexec.CommandRunner {
	t.Helper()
	return func(_ context.Context, command runner.Command) runner.Result {
		if captured != nil {
			*captured = command
		}
		if mutate != nil {
			mutate(command.Dir)
		}
		lastMessage := argumentAfter(command.Args, "--output-last-message")
		if lastMessage == "" {
			t.Fatal("fake Codex invocation missing --output-last-message")
		}
		if raw != nil {
			if err := os.WriteFile(lastMessage, raw, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return result
	}
}

func assertInvocationFlags(t *testing.T, args []string) {
	t.Helper()
	for _, flag := range []string{"--model", "-c", "--ephemeral", "--output-schema", "--output-last-message"} {
		if countArgument(args, flag) != 1 {
			t.Fatalf("flag %s count != 1 in %#v", flag, args)
		}
	}
	if argumentAfter(args, "--model") != codexexec.DefaultModel || argumentAfter(args, "-c") != "model_reasoning_effort="+codexexec.DefaultReasoningEffort {
		t.Fatalf("model/effort args = %#v", args)
	}
	for _, arg := range args {
		if arg == "resume" {
			t.Fatalf("invocation contains resume: %#v", args)
		}
	}
}

func runHistory(t *testing.T, store *ledger.Store, runID string) ledger.RunWithEvents {
	t.Helper()
	history, ok, err := store.GetRunWithEvents(context.Background(), runID)
	if err != nil || !ok {
		t.Fatalf("GetRunWithEvents() = found %t error %v", ok, err)
	}
	return history
}

func assertEventTypes(t *testing.T, events []ledger.Event, want []ledger.EventType) {
	t.Helper()
	got := make([]ledger.EventType, len(events))
	for i, event := range events {
		got[i] = event.Type
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("event types = %#v, want %#v", got, want)
	}
}

func assertNoWorkerLifecycleEvents(t *testing.T, events []ledger.Event) {
	t.Helper()
	for _, event := range events {
		switch event.Type {
		case ledger.EventVerificationStarted, ledger.EventVerificationCompleted, ledger.EventCommitStarted, ledger.EventCommitCreated, ledger.EventReceiptParsed, ledger.EventReceiptSynthesized:
			t.Fatalf("unexpected worker lifecycle event %q", event.Type)
		}
	}
}

func assertArtifactHash(t *testing.T, root string, artifact Artifact) {
	t.Helper()
	raw := readTestFile(t, filepath.Join(root, filepath.FromSlash(artifact.Path)))
	hash := sha256.Sum256(raw)
	if artifact.SHA256 != fmt.Sprintf("%x", hash) || artifact.ByteSize != len(raw) {
		t.Fatalf("artifact %+v does not match bytes", artifact)
	}
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	_ = gitOutput(t, root, args...)
}

func gitOutput(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
	return string(output)
}

func mustWriteTestFile(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readTestFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func assertTestFile(t *testing.T, path, want string) {
	t.Helper()
	if got := string(readTestFile(t, path)); got != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}

func argumentAfter(args []string, flag string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}

func countArgument(args []string, value string) int {
	count := 0
	for _, arg := range args {
		if arg == value {
			count++
		}
	}
	return count
}
