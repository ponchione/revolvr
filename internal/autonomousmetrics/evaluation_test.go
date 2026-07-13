package autonomousmetrics

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousqueue"
	"revolvr/internal/autonomoustaskrun"
	"revolvr/internal/ledger"
)

// TestDeterministicEvaluationScenarios is the no-model source-of-truth suite.
// The fake steps are owner-typed ledger occurrences with fixed UTC times and
// IDs; they exercise projection without Codex, network, Git, notifications,
// archive/retention mutation, a daemon, or parallel workers.
func TestDeterministicEvaluationScenarios(t *testing.T) {
	tests := []struct {
		name  string
		steps []struct {
			action string
			stop   autonomoustaskrun.StopReason
		}
		want        autonomoustaskrun.StopReason
		corrections int64
		wantSuccess int64
	}{
		{"straight success", []struct {
			action string
			stop   autonomoustaskrun.StopReason
		}{{"plan", ""}, {"implement", ""}, {"audit", ""}, {"complete", autonomoustaskrun.StopCompleted}}, autonomoustaskrun.StopCompleted, 0, 1},
		{"verification or finding correction", []struct {
			action string
			stop   autonomoustaskrun.StopReason
		}{{"implement", ""}, {"correct", ""}, {"audit", ""}, {"complete", autonomoustaskrun.StopCompleted}}, autonomoustaskrun.StopCompleted, 1, 1},
		{"clean re-audit", []struct {
			action string
			stop   autonomoustaskrun.StopReason
		}{{"audit", ""}, {"correct", ""}, {"audit", ""}, {"complete", autonomoustaskrun.StopCompleted}}, autonomoustaskrun.StopCompleted, 1, 1},
		{"conditional skips", []struct {
			action string
			stop   autonomoustaskrun.StopReason
		}{{"document_not_applicable", ""}, {"simplify_not_applicable", ""}, {"complete", autonomoustaskrun.StopCompleted}}, autonomoustaskrun.StopCompleted, 0, 1},
		{"no progress", []struct {
			action string
			stop   autonomoustaskrun.StopReason
		}{{"implement", ""}, {"implement", autonomoustaskrun.StopNoProgress}}, autonomoustaskrun.StopNoProgress, 0, 0},
		{"needs input", []struct {
			action string
			stop   autonomoustaskrun.StopReason
		}{{"needs_input", autonomoustaskrun.StopNeedsInput}}, autonomoustaskrun.StopNeedsInput, 0, 0},
		{"blocked skip", []struct {
			action string
			stop   autonomoustaskrun.StopReason
		}{{"block", autonomoustaskrun.StopBlocked}}, autonomoustaskrun.StopBlocked, 0, 0},
		{"crash finalization", []struct {
			action string
			stop   autonomoustaskrun.StopReason
		}{{"complete", autonomoustaskrun.StopCompleted}}, autonomoustaskrun.StopCompleted, 0, 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var events []ledger.Event
			stats := autonomoustaskrun.Statistics{}
			opID := "eval-" + strings.ReplaceAll(test.name, " ", "-")
			nextEventID := int64(1)
			for i, step := range test.steps {
				stats.CyclesStarted++
				stats.CyclesCompleted++
				if step.action == "correct" {
					stats.Corrections++
				}
				event := taskEvent(t, "task-eval", opID, int64(i+1), step.action, step.stop)
				event.Statistics = stats
				if test.name == "no progress" {
					event.Metrics = &autonomoustaskrun.MetricsEvidence{CircuitBreaker: &autonomous.CircuitBreakerDetail{Reason: autonomous.BreakerRepeatedSignature, TriggerSignature: &autonomous.CanonicalSignature{Kind: autonomous.SignatureKindOperationFailure, SHA256: hash("repeated-signature")}}}
					event.StopDetail = string(autonomous.BreakerRepeatedSignature)
				}
				if test.name == "needs input" {
					event.StopDetail = "Which exact storage format should be authoritative?"
				}
				if step.action == "correct" {
					if test.name != "clean re-audit" {
						event.Audit = auditRawRun(t, "task-eval", "audit-findings", false)
					}
					event.Metrics = &autonomoustaskrun.MetricsEvidence{FindingResolutions: []autonomous.FindingResolution{{FindingID: "finding-one", Status: autonomous.FindingResolutionStatusResolved, Evidence: []autonomous.EvidenceReference{evidence("correction-evidence")}}}}
				}
				if step.action == "audit" {
					clean := true
					if test.name == "clean re-audit" && i == 0 {
						clean = false
					}
					event.Audit = auditRawRun(t, "task-eval", "audit-"+fmt.Sprint(i+1), clean)
				}
				if step.stop == autonomoustaskrun.StopCompleted {
					verificationRaw, _ := json.Marshal(verificationEventTask("task-eval"))
					events = append(events, ledger.Event{ID: nextEventID, RunID: "run-eval", Type: ledger.EventVerificationCompleted, Payload: verificationRaw, CreatedAt: event.UpdatedAt.Add(-time.Millisecond)})
					nextEventID++
				}
				raw, _ := jsonMarshal(event)
				kind := ledger.EventTaskRunCycleCompleted
				if step.stop != "" {
					kind = ledger.EventTaskRunStopped
				}
				events = append(events, ledger.Event{ID: nextEventID, RunID: "run-eval", Type: kind, Payload: raw, CreatedAt: event.UpdatedAt})
				nextEventID++
			}
			done := fixed.Add(10)
			snapshot := ledger.Snapshot{Runs: []ledger.RunWithEvents{{Run: ledger.Run{ID: "run-eval", TaskID: "task-eval", Task: "fake Codex evaluation", Status: ledger.StatusCompleted, StartedAt: fixed, CompletedAt: &done}, Events: events}}, MaxEventID: nextEventID - 1}
			var actions []string
			for _, recorded := range events {
				if recorded.Type == ledger.EventTaskRunCycleCompleted || recorded.Type == ledger.EventTaskRunStopped {
					decoded, err := autonomoustaskrun.DecodeLedgerEvent(recorded.Payload)
					if err != nil {
						t.Fatal(err)
					}
					actions = append(actions, decoded.Action)
				}
			}
			var expectedActions []string
			for _, step := range test.steps {
				expectedActions = append(expectedActions, step.action)
			}
			if !reflect.DeepEqual(actions, expectedActions) {
				t.Fatalf("event order=%v want=%v", actions, expectedActions)
			}
			p, err := Project(snapshot, LogicalSource(snapshot))
			if err != nil {
				t.Fatal(err)
			}
			if len(p.TaskOutcomes.Facts) != 1 || p.TaskOutcomes.Facts[0].Reason != string(test.want) || p.TaskOutcomes.SuccessNumerator != test.wantSuccess || p.Attempts.CorrectionCycles != test.corrections {
				t.Fatalf("terminal=%+v corrections=%d", p.TaskOutcomes, p.Attempts.CorrectionCycles)
			}
			if test.wantSuccess == 1 && p.Verification.Occurrences != 1 {
				t.Fatalf("final verification metrics=%+v", p.Verification)
			}
			if test.name == "needs input" && p.TaskOutcomes.Facts[0].Reason != "needs_input" {
				t.Fatal("typed input terminal evidence missing")
			}
			if (test.name == "clean re-audit" || test.name == "verification or finding correction") && (p.Audits.Clean != 1 || p.Audits.ChangesRequired != 1 || p.Audits.Findings[0].Disposition != "resolved") {
				t.Fatalf("re-audit evidence=%+v", p.Audits)
			}
			if test.name == "crash finalization" {
				duplicate := snapshot.Runs[0].Events[len(events)-1]
				snapshot.Runs[0].Events = append(snapshot.Runs[0].Events, duplicate)
				if _, err := Project(snapshot, LogicalSource(snapshot)); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestEvaluationBlockedQueueYieldAllowsUnrelatedTask(t *testing.T) {
	done := fixed.Add(8 * time.Second)
	item := autonomousqueue.LedgerEvent{SchemaVersion: autonomousqueue.LedgerEventSchemaVersion, OperationID: "queue-blocked-yield", Mode: autonomousqueue.ModeUntilExhausted, Sequence: 4, Sweep: 1, Stage: "terminal", StopReason: autonomousqueue.StopDrained, StartedAt: fixed, UpdatedAt: done, CompletedAt: &done, MaximumWorkers: 1, Statistics: autonomousqueue.Statistics{Selections: 2, TasksRun: 2, Batches: 2, PeakActiveWorkers: 1}, Outcomes: []autonomousqueue.TaskOutcome{{TaskID: "blocked-task", TaskOperationID: "blocked-operation", StopReason: autonomoustaskrun.StopBlocked, BeforeFingerprint: hash("blocked-before"), AfterFingerprint: hash("blocked-after"), Authority: hash("blocked-authority")}, {TaskID: "ready-task", TaskOperationID: "ready-operation", StopReason: autonomoustaskrun.StopCompleted, BeforeFingerprint: hash("ready-before"), AfterFingerprint: hash("ready-after"), Authority: hash("ready-authority")}}}
	raw, _ := json.Marshal(item)
	snapshot := ledger.Snapshot{Runs: []ledger.RunWithEvents{{Run: ledger.Run{ID: "queue-run", TaskID: "queue-blocked-yield", Task: "fake sequential queue", Status: ledger.StatusCompleted, StartedAt: fixed, CompletedAt: &done, DurationSeconds: 8}, Events: []ledger.Event{{ID: 1, RunID: "queue-run", Type: ledger.EventQueueStopped, Payload: raw, CreatedAt: done}}}}, MaxEventID: 1}
	p, err := Project(snapshot, LogicalSource(snapshot))
	if err != nil {
		t.Fatal(err)
	}
	if p.Queues.TasksRun != 2 || p.Queues.Selections != 2 || p.Queues.Drained != 1 || item.Outcomes[0].TaskID != "blocked-task" || item.Outcomes[1].TaskID != "ready-task" {
		t.Fatalf("queue metrics=%+v outcomes=%+v", p.Queues, item.Outcomes)
	}
}

func jsonMarshal(value any) ([]byte, error) { return json.Marshal(value) }
