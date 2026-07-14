package autonomousnotification

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"revolvr/internal/runner"
	"revolvr/internal/runtimepath"
)

var fixedTime = time.Date(2026, 7, 12, 16, 0, 0, 123456789, time.UTC)

func enabledPolicy(executable string) Policy {
	return Policy{SchemaVersion: PolicySchemaVersion, Enabled: true, Events: append([]Event(nil), eventOrder...), Executable: executable, Args: []string{"--receive"}, Directory: DirectoryRepository, EnvironmentNames: []string{"HOOK_SECRET"}, Timeout: 2 * time.Second, StdoutCap: 64, StderrCap: 32, MaximumAttempts: 2, RetryDelay: time.Second}
}

func payloadInput(event Event, policy Policy) EventInput {
	refs := References{Task: Reference{Applicable: true, ID: "task-1", Path: ".agent/tasks/task-1.md", SHA256: strings.Repeat("a", 64), ByteSize: 42}, TaskRun: Reference{Applicable: true, ID: "operation-1"}, Terminal: Reference{Applicable: true, ID: "terminal-1"}}
	return EventInput{Event: event, SourceIdentity: "source-1", OccurredAt: fixedTime, RepositoryRoot: "/repo", EffectiveConfigSchema: "revolvr-effective-run-config-v4", EffectiveConfigSHA256: strings.Repeat("b", 64), HookPolicy: policy, RedactionNames: []string{"HOOK_SECRET"}, SubjectKind: "task", Outcome: string(event), StopReason: "completed", Detail: "detail secret-value", References: refs, Omissions: []string{"archive reference not applicable", "question reference not applicable"}}
}

func TestPayloadAllEventsDeterministicStrictAndRedacted(t *testing.T) {
	policy := enabledPolicy("hook")
	originalEvents := append([]Event(nil), policy.Events...)
	golden := map[Event]string{
		EventTaskCompleted:  "688f918e03ceaf3126bd2f3039142076eeaf96713888e283606c493f2eee97a1",
		EventTaskBlocked:    "d9c1c4b1adf9b9b6c56a9ed6fbad3dd35c1a770b8d92266fa5b1cb71dce679cf",
		EventTaskNeedsInput: "cc53c014a79f6d8ab6053ddae3c3caa4a4a7c9607dc169206f53730c0e955360",
		EventSafetyStop:     "e9d4af698f4b1a3911e574ff012ced54e7abe0b9dedeb7068e9650f1c064a256",
		EventQueueDrained:   "f48b6f5a6fa75f29d527531e542cfa4ec45ccc1ec30b9547ca73e0ceb30ae443",
		EventDaemonFailed:   "0ef68c86beb58c0441235f73da1b09914781cd293210491eec0e744b10a155ed",
	}
	for _, event := range eventOrder {
		t.Run(string(event), func(t *testing.T) {
			in := payloadInput(event, policy)
			first, raw, err := BuildPayload(in, func(value string) string { return strings.ReplaceAll(value, "secret-value", "[REDACTED]") })
			if err != nil {
				t.Fatal(err)
			}
			second, raw2, err := BuildPayload(in, func(value string) string { return strings.ReplaceAll(value, "secret-value", "[REDACTED]") })
			if err != nil || !reflect.DeepEqual(first, second) || !bytes.Equal(raw, raw2) {
				t.Fatalf("payload is not deterministic: %v", err)
			}
			if !bytes.HasSuffix(raw, []byte("\n")) || bytes.Contains(raw, []byte("secret-value")) || !bytes.Contains(raw, []byte("[REDACTED]")) {
				t.Fatalf("payload bytes = %s", raw)
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(raw)); got != golden[event] {
				t.Fatalf("payload golden hash = %s, want %s\n%s", got, golden[event], raw)
			}
			decoded, err := DecodePayload(raw)
			if err != nil || decoded.Event != event {
				t.Fatalf("decode=%+v err=%v", decoded, err)
			}
		})
	}
	if !reflect.DeepEqual(policy.Events, originalEvents) {
		t.Fatalf("caller event slice mutated: %v", policy.Events)
	}
	_, raw, _ := BuildPayload(payloadInput(EventTaskCompleted, policy), nil)
	unknown := bytes.Replace(raw, []byte(`"event":`), []byte(`"future":1,"event":`), 1)
	if _, err := DecodePayload(unknown); err == nil {
		t.Fatal("unknown field accepted")
	}
	unknown = bytes.Replace(raw, []byte(`"task_completed"`), []byte(`"future_event"`), 1)
	if _, err := DecodePayload(unknown); err == nil {
		t.Fatal("unknown event accepted")
	}
}

func TestPolicyValidationAndDisabledCompatibility(t *testing.T) {
	if got, err := DefaultPolicy().Normalize(nil); err != nil || got.Enabled {
		t.Fatalf("default=%+v err=%v", got, err)
	}
	cases := []Policy{
		{SchemaVersion: PolicySchemaVersion, Enabled: true},
		{SchemaVersion: PolicySchemaVersion, Enabled: true, Events: []Event{"future"}, Executable: "hook"},
		{SchemaVersion: PolicySchemaVersion, Enabled: true, Events: []Event{EventTaskCompleted, EventTaskCompleted}, Executable: "hook"},
		enabledPolicy(" hook "),
	}
	for i, policy := range cases {
		if _, err := policy.Normalize([]string{"HOOK_SECRET"}); err == nil {
			t.Fatalf("case %d accepted", i)
		}
	}
	bad := enabledPolicy("hook")
	bad.EnvironmentNames = []string{"UNREDACTED"}
	if _, err := bad.Normalize([]string{"HOOK_SECRET"}); err == nil {
		t.Fatal("unredacted environment accepted")
	}
	bad = enabledPolicy("hook")
	bad.MaximumAttempts = MaxAttempts + 1
	if _, err := bad.Normalize([]string{"HOOK_SECRET"}); err == nil {
		t.Fatal("excess attempts accepted")
	}
}

func TestDeliveryRetryReplayExactInputEnvironmentAndRedaction(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "hook")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	policy := enabledPolicy(executable)
	payload, raw, err := BuildPayload(payloadInput(EventTaskCompleted, policy), func(v string) string { return strings.ReplaceAll(v, "secret-value", "[REDACTED]") })
	if err != nil {
		t.Fatal(err)
	}
	now := fixedTime
	calls := 0
	waits := 0
	var inputs [][]byte
	run := func(_ context.Context, command runner.Command) runner.Result {
		calls++
		read, _ := io.ReadAll(command.Stdin)
		inputs = append(inputs, read)
		if command.Name != executable || !reflect.DeepEqual(command.Args, []string{"--receive"}) || command.Dir != root || !command.ReplaceEnv || !reflect.DeepEqual(command.Env, []string{"HOOK_SECRET=secret-value"}) {
			t.Fatalf("command=%+v", command)
		}
		if _, err := os.Stat(filepath.Join(root, ".revolvr", "autonomous", "notifications", payload.DeliveryID, "payload.json")); err != nil {
			t.Fatalf("payload not durable before invocation: %v", err)
		}
		if calls == 1 {
			return runner.Result{ExitCode: 7, Stdout: "secret-value stdout", Stderr: "secret-value stderr"}
		}
		return runner.Result{ExitCode: 0}
	}
	cfg := DeliveryConfig{RepositoryRoot: root, Payload: payload, PayloadBytes: raw, Policy: policy, RedactionNames: []string{"HOOK_SECRET"}, Clock: func() time.Time { now = now.Add(time.Second); return now }, Runner: run, LookPath: func(string) (string, error) { return executable, nil }, LookupEnv: func(name string) (string, bool) { return "secret-value", name == "HOOK_SECRET" }, Wait: func(context.Context, time.Duration) error { waits++; return nil }}
	result, err := Deliver(context.Background(), cfg)
	if err != nil || result.Stage != StageSucceeded || result.Attempts != 2 || calls != 2 || waits != 1 {
		t.Fatalf("result=%+v calls=%d waits=%d err=%v", result, calls, waits, err)
	}
	if !bytes.Equal(inputs[0], inputs[1]) || !bytes.Equal(inputs[0], raw) {
		t.Fatal("retry payload changed")
	}
	_, _, journal, found, err := Inspect(root, payload.DeliveryID)
	if err != nil || !found || strings.Contains(journal.Attempts[0].Stdout+journal.Attempts[0].Stderr, "secret-value") {
		t.Fatalf("journal=%+v found=%v err=%v", journal, found, err)
	}
	summaries, err := List(root)
	if err != nil || len(summaries) != 1 || summaries[0].DeliveryID != payload.DeliveryID || summaries[0].Stage != StageSucceeded || summaries[0].Attempts != 2 {
		t.Fatalf("summaries=%+v err=%v", summaries, err)
	}
	replay, err := Deliver(context.Background(), cfg)
	if err != nil || !replay.Replayed || calls != 2 {
		t.Fatalf("replay=%+v calls=%d err=%v", replay, calls, err)
	}
}

func TestDeliveryTimeoutExhaustionCancellationAndRestart(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "hook")
	if err := os.WriteFile(executable, []byte("x"), 0o700); err != nil {
		t.Fatal(err)
	}
	policy := enabledPolicy(executable)
	payload, raw, err := BuildPayload(payloadInput(EventSafetyStop, policy), nil)
	if err != nil {
		t.Fatal(err)
	}
	now := fixedTime
	cfg := DeliveryConfig{RepositoryRoot: root, Payload: payload, PayloadBytes: raw, Policy: policy, RedactionNames: []string{"HOOK_SECRET"}, Clock: func() time.Time { now = now.Add(time.Second); return now }, LookPath: func(string) (string, error) { return executable, nil }, LookupEnv: func(string) (string, bool) { return "secret-value", true }, Wait: func(context.Context, time.Duration) error { return nil }, Runner: func(context.Context, runner.Command) runner.Result {
		return runner.Result{ExitCode: -1, Err: context.DeadlineExceeded, TimedOut: true}
	}}
	result, err := Deliver(context.Background(), cfg)
	if err == nil || result.Stage != StageFailed || result.Attempts != 2 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	other := payloadInput(EventQueueDrained, policy)
	other.SourceIdentity = "source-cancel"
	cancelPayload, cancelRaw, _ := BuildPayload(other, nil)
	cfg.Payload, cfg.PayloadBytes = cancelPayload, cancelRaw
	result, err = Deliver(cancelled, cfg)
	if !errors.Is(err, context.Canceled) || result.Stage != StageResumable || result.Attempts != 0 {
		t.Fatalf("cancel=%+v err=%v", result, err)
	}
	result, err = Deliver(context.Background(), cfg)
	if err == nil || result.Attempts != 2 {
		t.Fatalf("restart=%+v err=%v", result, err)
	}
}

func TestDeliveryCancellationPersistenceFaultsPreserveDurableAuthority(t *testing.T) {
	type faultCase struct {
		name              string
		timing            string
		point             persistencePoint
		pathMatches       func(string, string) bool
		wantStage         Stage
		wantSequence      int64
		wantAttempts      int
		wantCheckpointSeq int64
	}
	cases := []faultCase{
		{
			name: "history write before dispatch", timing: "before_dispatch", point: persistenceHistoryWrite,
			pathMatches: func(_ string, path string) bool { return filepath.Base(path) == "00000000000000000002-resumable.json" },
			wantStage:   StageAdmitted, wantSequence: 1, wantCheckpointSeq: 1,
		},
		{
			name: "history file sync before dispatch", timing: "before_dispatch", point: persistenceFileSync,
			pathMatches: func(_ string, path string) bool { return filepath.Base(path) == "00000000000000000002-resumable.json" },
			wantStage:   StageAdmitted, wantSequence: 1, wantCheckpointSeq: 1,
		},
		{
			name: "history directory sync before dispatch", timing: "before_dispatch", point: persistenceDirectorySync,
			pathMatches: func(dir, path string) bool { return path == filepath.Join(dir, "history") },
			wantStage:   StageResumable, wantSequence: 2, wantCheckpointSeq: 1,
		},
		{
			name: "journal replacement during terminal transition", timing: "during_delivery", point: persistenceJournalReplace,
			pathMatches: func(_ string, path string) bool { return filepath.Base(path) == "journal.json" },
			wantStage:   StageResumable, wantSequence: 3, wantAttempts: 1, wantCheckpointSeq: 2,
		},
		{
			name: "journal file sync during terminal transition", timing: "during_delivery", point: persistenceFileSync,
			pathMatches: func(_ string, path string) bool { return filepath.Base(path) == "journal.json" },
			wantStage:   StageResumable, wantSequence: 3, wantAttempts: 1, wantCheckpointSeq: 2,
		},
		{
			name: "journal directory sync during terminal transition", timing: "during_delivery", point: persistenceDirectorySync,
			pathMatches: func(dir, path string) bool { return path == dir },
			wantStage:   StageResumable, wantSequence: 3, wantAttempts: 1, wantCheckpointSeq: 3,
		},
		{
			name: "history write during retry cancellation", timing: "retry_delay", point: persistenceHistoryWrite,
			pathMatches: func(_ string, path string) bool { return filepath.Base(path) == "00000000000000000004-resumable.json" },
			wantStage:   StageRetryable, wantSequence: 3, wantAttempts: 1, wantCheckpointSeq: 3,
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			root, payload, cfg, admitted := admittedDeliveryFixture(t, EventQueueDrained)
			if admitted.Sequence != 1 || admitted.Stage != StageAdmitted {
				t.Fatalf("admitted journal = %+v", admitted)
			}
			dir := deliveryDir(root, payload.DeliveryID)
			injected := errors.New("injected " + test.name)
			armed := test.timing == "before_dispatch"
			injections := 0
			cfg.persistenceFault = func(point persistencePoint, path string) error {
				if armed && injections == 0 && point == test.point && test.pathMatches(dir, path) {
					injections++
					return injected
				}
				return nil
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch test.timing {
			case "before_dispatch":
				cancel()
			case "during_delivery":
				cfg.Runner = func(context.Context, runner.Command) runner.Result {
					armed = true
					cancel()
					return runner.Result{ExitCode: -1, Err: context.Canceled}
				}
			case "retry_delay":
				cfg.Runner = func(context.Context, runner.Command) runner.Result {
					return runner.Result{ExitCode: 7}
				}
				cfg.Wait = func(context.Context, time.Duration) error {
					armed = true
					cancel()
					return context.Canceled
				}
			default:
				t.Fatalf("unknown timing %q", test.timing)
			}

			result, err := Deliver(ctx, cfg)
			if !errors.Is(err, context.Canceled) || !errors.Is(err, injected) {
				t.Fatalf("delivery error = %v, want cancellation joined with %v", err, injected)
			}
			if injections != 1 {
				t.Fatalf("fault injections = %d, want 1", injections)
			}
			if result.DeliveryID != payload.DeliveryID || result.Stage != test.wantStage || result.Attempts != test.wantAttempts {
				t.Fatalf("result = %+v, want delivery %q stage %q attempts %d", result, payload.DeliveryID, test.wantStage, test.wantAttempts)
			}

			_, _, reopened, found, reopenErr := Inspect(root, payload.DeliveryID)
			if reopenErr != nil || !found {
				t.Fatalf("reopen found=%t error=%v", found, reopenErr)
			}
			if reopened.Sequence != test.wantSequence || reopened.Stage != test.wantStage || len(reopened.Attempts) != test.wantAttempts {
				t.Fatalf("reopened journal = %+v, want sequence %d stage %q attempts %d", reopened, test.wantSequence, test.wantStage, test.wantAttempts)
			}
			checkpoint := readNotificationTestJournal(t, filepath.Join(dir, "journal.json"))
			if checkpoint.Sequence != test.wantCheckpointSeq {
				t.Fatalf("checkpoint = %+v, want sequence %d", checkpoint, test.wantCheckpointSeq)
			}
			history, readErr := os.ReadDir(filepath.Join(dir, "history"))
			if readErr != nil || len(history) != int(test.wantSequence) {
				t.Fatalf("history entries = %d, want %d; error=%v", len(history), test.wantSequence, readErr)
			}
			entries, readErr := os.ReadDir(dir)
			if readErr != nil {
				t.Fatal(readErr)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".journal-") {
					t.Fatalf("temporary checkpoint survived failure: %s", entry.Name())
				}
			}

			restart := cfg
			restart.persistenceFault = nil
			restart.Runner = func(context.Context, runner.Command) runner.Result { return runner.Result{ExitCode: 0} }
			restart.Wait = func(context.Context, time.Duration) error { return nil }
			restarted, restartErr := Deliver(context.Background(), restart)
			if restartErr != nil || restarted.Stage != StageSucceeded {
				t.Fatalf("restart result = %+v error=%v", restarted, restartErr)
			}
		})
	}
}

func TestDeliveryCancellationTimingsRemainResumable(t *testing.T) {
	for _, test := range []struct {
		name         string
		timing       string
		wantAttempts int
	}{
		{name: "before dispatch", timing: "before_dispatch"},
		{name: "after executable lookup", timing: "after_lookup"},
		{name: "during delivery", timing: "during_delivery", wantAttempts: 1},
		{name: "during retry delay", timing: "retry_delay", wantAttempts: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, payload, cfg, _ := admittedDeliveryFixture(t, EventSafetyStop)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch test.timing {
			case "before_dispatch":
				cancel()
			case "after_lookup":
				lookPath := cfg.LookPath
				cfg.LookPath = func(name string) (string, error) {
					resolved, err := lookPath(name)
					cancel()
					return resolved, err
				}
			case "during_delivery":
				cfg.Runner = func(context.Context, runner.Command) runner.Result {
					cancel()
					return runner.Result{ExitCode: -1, Err: context.Canceled}
				}
			case "retry_delay":
				cfg.Runner = func(context.Context, runner.Command) runner.Result { return runner.Result{ExitCode: 7} }
				cfg.Wait = func(context.Context, time.Duration) error {
					cancel()
					return context.Canceled
				}
			}
			result, err := Deliver(ctx, cfg)
			if err != context.Canceled || result.Stage != StageResumable || result.Attempts != test.wantAttempts {
				t.Fatalf("result = %+v error=%v", result, err)
			}
			_, _, reopened, found, reopenErr := Inspect(root, payload.DeliveryID)
			if reopenErr != nil || !found || reopened.Stage != StageResumable || len(reopened.Attempts) != test.wantAttempts {
				t.Fatalf("reopened = %+v found=%t error=%v", reopened, found, reopenErr)
			}
		})
	}
}

func TestDisabledDeliveryStartsNoWorkOrWrites(t *testing.T) {
	root := t.TempDir()
	policy := DefaultPolicy()
	result, err := Deliver(context.Background(), DeliveryConfig{RepositoryRoot: root, Policy: policy, Payload: Payload{Event: EventTaskCompleted}, Runner: func(context.Context, runner.Command) runner.Result { t.Fatal("runner called"); return runner.Result{} }, LookPath: func(string) (string, error) { t.Fatal("lookup called"); return "", nil }})
	if err != nil || !result.Disabled {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".revolvr")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("disabled delivery wrote state: %v", err)
	}
}

func TestDeliveryRejectsSymlinkedRuntimeNamespace(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, ".revolvr")); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(root, "hook")
	if err := os.WriteFile(executable, []byte("x"), 0o700); err != nil {
		t.Fatal(err)
	}
	policy := enabledPolicy(executable)
	payload, raw, err := BuildPayload(payloadInput(EventSafetyStop, policy), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Deliver(context.Background(), DeliveryConfig{RepositoryRoot: root, Payload: payload, PayloadBytes: raw, Policy: policy, RedactionNames: []string{"HOOK_SECRET"}, LookPath: func(string) (string, error) { return executable, nil }, LookupEnv: func(string) (string, bool) { return "secret-value", true }})
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("error=%v", err)
	}
	entries, _ := os.ReadDir(outside)
	if len(entries) != 0 {
		t.Fatalf("outside namespace mutated: %v", entries)
	}
}

func TestDeliveryLockRejectsUnsafePathsAndOpenSubstitution(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T, root, outside, sentinel, dir, lockPath string)
		afterOpen func(root, path string) error
	}{
		{
			name: "final symlink",
			setup: func(t *testing.T, _, _, sentinel, dir, lockPath string) {
				t.Helper()
				if err := os.MkdirAll(dir, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(sentinel, lockPath); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "hard link alias",
			setup: func(t *testing.T, _, _, sentinel, dir, lockPath string) {
				t.Helper()
				if err := os.MkdirAll(dir, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Link(sentinel, lockPath); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "symlinked ancestor",
			setup: func(t *testing.T, root, outside, _, _, _ string) {
				t.Helper()
				if err := os.Mkdir(filepath.Join(root, ".revolvr"), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, filepath.Join(root, ".revolvr", "autonomous")); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "substitution after open",
			afterOpen: func(_, path string) error {
				if err := os.Rename(path, path+".opened"); err != nil {
					return err
				}
				return os.WriteFile(path, []byte("replacement\n"), 0o600)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			sentinel := filepath.Join(outside, "sentinel.txt")
			const sentinelBytes = "outside sentinel\n"
			if err := os.WriteFile(sentinel, []byte(sentinelBytes), 0o600); err != nil {
				t.Fatal(err)
			}
			dir := deliveryDir(root, "delivery-lock-test")
			lockPath := filepath.Join(dir, "delivery.lock")
			if tt.setup != nil {
				tt.setup(t, root, outside, sentinel, dir, lockPath)
			}
			boundary, bindErr := runtimepath.Bind(root)
			if bindErr != nil {
				t.Fatal(bindErr)
			}
			lease, err := acquireDeliveryLock(context.Background(), boundary, dir, tt.afterOpen)
			if err == nil {
				_ = lease.Close()
				t.Fatal("unsafe delivery lock path was acquired")
			}
			if !errors.Is(err, runtimepath.ErrUnsafe) {
				t.Fatalf("acquire error = %v, want runtimepath.ErrUnsafe", err)
			}
			raw, readErr := os.ReadFile(sentinel)
			if readErr != nil || string(raw) != sentinelBytes {
				t.Fatalf("outside sentinel changed: err=%v bytes=%q", readErr, raw)
			}
			entries, readErr := os.ReadDir(outside)
			if readErr != nil || len(entries) != 1 || entries[0].Name() != "sentinel.txt" {
				t.Fatalf("outside directory changed: err=%v entries=%v", readErr, entries)
			}
			if tt.afterOpen != nil {
				if raw, readErr := os.ReadFile(lockPath); readErr != nil || string(raw) != "replacement\n" {
					t.Fatalf("replacement changed: err=%v bytes=%q", readErr, raw)
				}
			}
		})
	}
}

func TestNotificationEvidenceRejectsUnsafeFinalComponents(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, dir, outside, sentinel string)
	}{
		{
			name: "intent symlink",
			setup: func(t *testing.T, dir, _, sentinel string) {
				t.Helper()
				path := filepath.Join(dir, "intent.json")
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(sentinel, path); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "payload hard link",
			setup: func(t *testing.T, dir, _, sentinel string) {
				t.Helper()
				path := filepath.Join(dir, "payload.json")
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Link(sentinel, path); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "journal unsafe mode",
			setup: func(t *testing.T, dir, _, _ string) {
				t.Helper()
				if err := os.Chmod(filepath.Join(dir, "journal.json"), 0o666); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "history symlink",
			setup: func(t *testing.T, dir, _, sentinel string) {
				t.Helper()
				path := filepath.Join(dir, "history", "00000000000000000001-admitted.json")
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(sentinel, path); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "delivery directory unsafe mode",
			setup: func(t *testing.T, dir, _, _ string) {
				t.Helper()
				if err := os.Chmod(dir, 0o777); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, payload, _, _ := admittedDeliveryFixture(t, EventQueueDrained)
			dir := deliveryDir(root, payload.DeliveryID)
			outside := t.TempDir()
			sentinel := filepath.Join(outside, "sentinel")
			if err := os.WriteFile(sentinel, []byte("outside authority\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			test.setup(t, dir, outside, sentinel)
			before := notificationTreeSnapshot(t, outside)
			if _, _, _, _, err := Inspect(root, payload.DeliveryID); !errors.Is(err, runtimepath.ErrUnsafe) {
				t.Fatalf("Inspect error = %v, want runtimepath.ErrUnsafe", err)
			}
			if after := notificationTreeSnapshot(t, outside); !reflect.DeepEqual(after, before) {
				t.Fatalf("outside tree changed\nbefore: %v\nafter:  %v", before, after)
			}
		})
	}
}

func TestDeliveryPersistenceRejectsAncestorSubstitutionWithoutOutsideMutation(t *testing.T) {
	tests := []struct {
		name       string
		point      persistencePoint
		pathMatch  func(string) bool
		failBefore bool
	}{
		{name: "intent before open", point: persistenceBeforeOpen, pathMatch: func(path string) bool { return filepath.Base(path) == "intent.json" }},
		{name: "payload after open", point: persistenceAfterOpen, pathMatch: func(path string) bool { return filepath.Base(path) == "payload.json" }},
		{name: "history before publication", point: persistenceBeforePublication, pathMatch: func(path string) bool { return filepath.Base(path) == "00000000000000000001-admitted.json" }},
		{name: "journal before publication", point: persistenceBeforePublication, pathMatch: func(path string) bool { return filepath.Base(path) == "journal.json" }},
		{name: "journal after publication", point: persistenceAfterPublication, pathMatch: func(path string) bool { return filepath.Base(path) == "journal.json" }},
		{name: "history directory sync", point: persistenceDirectorySync, pathMatch: func(path string) bool { return filepath.Base(path) == "history" }},
		{name: "history cleanup", point: persistenceCleanup, pathMatch: func(path string) bool { return filepath.Base(path) == "00000000000000000001-admitted.json" }, failBefore: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			payload, cfg := notificationTestDelivery(t, root, EventSafetyStop)
			dir := deliveryDir(root, payload.DeliveryID)
			outside := t.TempDir()
			if err := os.WriteFile(filepath.Join(outside, "sentinel"), []byte("outside authority\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			injectedFailure := errors.New("injected history failure")
			injected := false
			var outsideBefore []string
			cfg.persistenceFault = func(point persistencePoint, path string) error {
				if test.failBefore && point == persistenceHistoryWrite && test.pathMatch(path) {
					return injectedFailure
				}
				if injected || point != test.point || !test.pathMatch(path) {
					return nil
				}
				injected = true
				var err error
				outsideBefore, err = substituteNotificationDirectory(t, dir, outside)
				return err
			}

			if _, err := Deliver(context.Background(), cfg); !errors.Is(err, runtimepath.ErrUnsafe) {
				t.Fatalf("Deliver error = %v, want runtimepath.ErrUnsafe", err)
			} else if test.failBefore && !errors.Is(err, injectedFailure) {
				t.Fatalf("Deliver error = %v, want injected history failure", err)
			}
			if !injected || outsideBefore == nil {
				t.Fatal("substitution hook did not run")
			}
			if after := notificationTreeSnapshot(t, outside); !reflect.DeepEqual(after, outsideBefore) {
				t.Fatalf("outside tree changed\nbefore: %v\nafter:  %v", outsideBefore, after)
			}
		})
	}
}

func TestDeliveryPersistenceChecksHeldLeaseBeforeJournalReplacement(t *testing.T) {
	root := t.TempDir()
	payload, cfg := notificationTestDelivery(t, root, EventDaemonFailed)
	dir := deliveryDir(root, payload.DeliveryID)
	injected := false
	cfg.persistenceFault = func(point persistencePoint, path string) error {
		if injected || point != persistenceBeforePublication || filepath.Base(path) != "journal.json" {
			return nil
		}
		injected = true
		lockPath := filepath.Join(dir, "delivery.lock")
		if err := os.Rename(lockPath, lockPath+".held"); err != nil {
			return err
		}
		return os.WriteFile(lockPath, []byte("replacement lock\n"), 0o600)
	}

	if _, err := Deliver(context.Background(), cfg); !errors.Is(err, runtimepath.ErrUnsafe) {
		t.Fatalf("Deliver error = %v, want runtimepath.ErrUnsafe", err)
	}
	if !injected {
		t.Fatal("lease substitution hook did not run")
	}
	if _, err := os.Lstat(filepath.Join(dir, "journal.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("journal was replaced after lease substitution: %v", err)
	}
	_, _, journal, found, err := Inspect(root, payload.DeliveryID)
	if err != nil || !found || journal.Sequence != 1 || journal.Stage != StageAdmitted {
		t.Fatalf("recovered journal = %+v found=%t error=%v", journal, found, err)
	}
}

func TestDeliveryRestartRecoversRunningAttemptAndHistoryAhead(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "hook")
	if err := os.WriteFile(executable, []byte("x"), 0o700); err != nil {
		t.Fatal(err)
	}
	policy := enabledPolicy(executable)
	payload, raw, err := BuildPayload(payloadInput(EventDaemonFailed, policy), nil)
	if err != nil {
		t.Fatal(err)
	}
	store, closeStore := openNotificationTestStore(t, root, payload.DeliveryID)
	policyID, _ := policy.Identity([]string{"HOOK_SECRET"})
	intent := Intent{SchemaVersion: IntentSchemaVersion, DeliveryID: payload.DeliveryID, EventID: payload.EventID, Event: payload.Event, PayloadSHA256: hash(raw), PayloadSize: len(raw), Policy: policy, PolicySHA256: policyID, ConfigSchema: payload.EffectiveConfigSchema, ConfigSHA256: payload.EffectiveConfigSHA256, AdmittedAt: fixedTime}
	journal, _, err := store.admit(intent, raw, fixedTime)
	if err != nil {
		t.Fatal(err)
	}
	journal, err = store.transition(journal, StageRunning, "attempt 1 running", nil, fixedTime.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	closeStore()
	if err := os.Remove(filepath.Join(deliveryDir(root, payload.DeliveryID), "journal.json")); err != nil {
		t.Fatal(err)
	}
	calls := 0
	now := fixedTime.Add(time.Second)
	result, err := Deliver(context.Background(), DeliveryConfig{RepositoryRoot: root, Payload: payload, PayloadBytes: raw, Policy: policy, RedactionNames: []string{"HOOK_SECRET"}, Clock: func() time.Time { now = now.Add(time.Second); return now }, LookPath: func(string) (string, error) { return executable, nil }, LookupEnv: func(string) (string, bool) { return "secret-value", true }, Wait: func(context.Context, time.Duration) error { return nil }, Runner: func(context.Context, runner.Command) runner.Result { calls++; return runner.Result{ExitCode: 0} }})
	if err != nil || result.Stage != StageSucceeded || result.Attempts != 2 || calls != 1 {
		t.Fatalf("result=%+v calls=%d err=%v", result, calls, err)
	}
	_, _, recovered, found, err := Inspect(root, payload.DeliveryID)
	if err != nil || !found || len(recovered.Attempts) != 2 || !recovered.Attempts[0].RunnerError || !recovered.Attempts[0].Retryable {
		t.Fatalf("journal=%+v found=%v err=%v", recovered, found, err)
	}
}

func admittedDeliveryFixture(t *testing.T, event Event) (string, Payload, DeliveryConfig, Journal) {
	t.Helper()
	root := t.TempDir()
	executable := filepath.Join(root, "hook")
	if err := os.WriteFile(executable, []byte("x"), 0o700); err != nil {
		t.Fatal(err)
	}
	policy := enabledPolicy(executable)
	payload, raw, err := BuildPayload(payloadInput(event, policy), nil)
	if err != nil {
		t.Fatal(err)
	}
	store, closeStore := openNotificationTestStore(t, root, payload.DeliveryID)
	policyID, err := policy.Identity([]string{"HOOK_SECRET"})
	if err != nil {
		t.Fatal(err)
	}
	intent := Intent{SchemaVersion: IntentSchemaVersion, DeliveryID: payload.DeliveryID, EventID: payload.EventID, Event: payload.Event, PayloadSHA256: hash(raw), PayloadSize: len(raw), Policy: policy, PolicySHA256: policyID, ConfigSchema: payload.EffectiveConfigSchema, ConfigSHA256: payload.EffectiveConfigSHA256, AdmittedAt: payload.OccurredAt}
	journal, _, err := store.admit(intent, raw, fixedTime)
	if err != nil {
		t.Fatal(err)
	}
	closeStore()
	now := fixedTime
	cfg := DeliveryConfig{
		RepositoryRoot: root,
		Payload:        payload,
		PayloadBytes:   raw,
		Policy:         policy,
		RedactionNames: []string{"HOOK_SECRET"},
		Clock:          func() time.Time { now = now.Add(time.Second); return now },
		Runner:         func(context.Context, runner.Command) runner.Result { return runner.Result{ExitCode: 0} },
		LookPath:       func(string) (string, error) { return executable, nil },
		LookupEnv:      func(string) (string, bool) { return "secret-value", true },
		Wait:           func(context.Context, time.Duration) error { return nil },
	}
	return root, payload, cfg, journal
}

func readNotificationTestJournal(t *testing.T, path string) Journal {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var journal Journal
	if err := decodeCanonical(raw, &journal); err != nil {
		t.Fatal(err)
	}
	return journal
}

func openNotificationTestStore(t *testing.T, root, deliveryID string) (*deliveryStore, func()) {
	t.Helper()
	boundary, err := runtimepath.Bind(root)
	if err != nil {
		t.Fatal(err)
	}
	dir := deliveryDir(boundary.Root(), deliveryID)
	lease, err := acquireDeliveryLock(context.Background(), boundary, dir)
	if err != nil {
		t.Fatal(err)
	}
	store, found, err := openDeliveryStore(boundary, deliveryID, lease, nil)
	if err != nil || !found {
		_ = lease.Close()
		t.Fatalf("open delivery store: found=%t error=%v", found, err)
	}
	return store, func() {
		t.Helper()
		if err := errors.Join(store.Close(), lease.Close()); err != nil {
			t.Fatal(err)
		}
	}
}

func notificationTestDelivery(t *testing.T, root string, event Event) (Payload, DeliveryConfig) {
	t.Helper()
	executable := filepath.Join(root, "hook")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	policy := enabledPolicy(executable)
	payload, raw, err := BuildPayload(payloadInput(event, policy), nil)
	if err != nil {
		t.Fatal(err)
	}
	now := fixedTime
	return payload, DeliveryConfig{
		RepositoryRoot: root,
		Payload:        payload,
		PayloadBytes:   raw,
		Policy:         policy,
		RedactionNames: []string{"HOOK_SECRET"},
		Clock:          func() time.Time { now = now.Add(time.Second); return now },
		Runner: func(context.Context, runner.Command) runner.Result {
			t.Fatal("notification runner reached after persistence substitution")
			return runner.Result{}
		},
		LookPath:  func(string) (string, error) { return executable, nil },
		LookupEnv: func(string) (string, bool) { return "secret-value", true },
		Wait:      func(context.Context, time.Duration) error { return nil },
	}
}

func substituteNotificationDirectory(t *testing.T, dir, outside string) ([]string, error) {
	t.Helper()
	moved := dir + ".moved"
	if err := os.Rename(dir, moved); err != nil {
		return nil, err
	}
	if err := filepath.WalkDir(moved, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), ".immutable-") && !strings.HasPrefix(entry.Name(), ".journal-") {
			return nil
		}
		rel, err := filepath.Rel(moved, path)
		if err != nil {
			return err
		}
		attackerPath := filepath.Join(outside, rel)
		if err := os.MkdirAll(filepath.Dir(attackerPath), 0o700); err != nil {
			return err
		}
		return os.WriteFile(attackerPath, []byte("attacker temporary\n"), 0o600)
	}); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(outside, "delivery.lock"), []byte("attacker lock\n"), 0o600); err != nil {
		return nil, err
	}
	if err := os.Symlink(outside, dir); err != nil {
		return nil, err
	}
	return notificationTreeSnapshot(t, outside), nil
}

func notificationTreeSnapshot(t *testing.T, root string) []string {
	t.Helper()
	var result []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		nlink := uint64(0)
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			nlink = uint64(stat.Nlink)
		}
		value := fmt.Sprintf("%s|%s|%04o|%d", filepath.ToSlash(rel), info.Mode().Type(), info.Mode().Perm(), nlink)
		if info.Mode().IsRegular() {
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			value += "|" + string(raw)
		} else if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			value += "|" + target
		}
		result = append(result, value)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
