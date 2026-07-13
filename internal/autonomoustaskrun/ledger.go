package autonomoustaskrun

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"revolvr/internal/ledger"
)

const LedgerEventSchemaVersion = "autonomous-task-run-event-v2"

// LedgerEvent is the strict logical task-operation evidence consumed by
// read-only projections and immutable export replay.
type LedgerEvent struct {
	SchemaVersion string           `json:"schema_version"`
	OperationID   string           `json:"operation_id"`
	TaskID        string           `json:"task_id"`
	Sequence      int64            `json:"sequence"`
	Stage         string           `json:"stage"`
	Cycle         int64            `json:"cycle"`
	Action        string           `json:"action,omitempty"`
	DecisionID    string           `json:"decision_id,omitempty"`
	RunID         string           `json:"run_id,omitempty"`
	StopReason    StopReason       `json:"stop_reason,omitempty"`
	StopDetail    string           `json:"stop_detail,omitempty"`
	StartedAt     time.Time        `json:"started_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
	CompletedAt   *time.Time       `json:"completed_at,omitempty"`
	Statistics    Statistics       `json:"statistics"`
	Metrics       *MetricsEvidence `json:"metrics_evidence,omitempty"`
	Verification  json.RawMessage  `json:"verification,omitempty"`
	Audit         json.RawMessage  `json:"audit,omitempty"`
}

func loopLedgerRunID(operationID string) string {
	sum := sha256.Sum256([]byte("autonomous-task-run-ledger-v1\x00" + operationID))
	return "task-run-" + hex.EncodeToString(sum[:12])
}

func admitLoopLedger(ctx context.Context, n normalized, op Operation) error {
	if n.Ledger == nil {
		return nil
	}
	runID := loopLedgerRunID(op.OperationID)
	existing, found, err := n.Ledger.GetRunWithEvents(ctx, runID)
	if err != nil {
		return err
	}
	if !found {
		if _, err := n.Ledger.CreateRun(ctx, ledger.RunSpec{ID: runID, TaskID: op.TaskID, Task: "run exact autonomous task until terminal", StartedAt: op.StartedAt}); err != nil {
			return err
		}
	} else if existing.Run.TaskID != op.TaskID || existing.Run.Task != "run exact autonomous task until terminal" || !existing.Run.StartedAt.Equal(op.StartedAt) {
		return errors.New("task run: loop ledger run identity conflicts with durable operation")
	}
	admitted := op
	admitted.Sequence = 0
	admitted.Stage = "admitted"
	admitted.InFlight = false
	admitted.Statistics = Statistics{}
	admitted.LastAction, admitted.LastDecisionID, admitted.LastRunID = "", "", ""
	admitted.StopReason, admitted.StopDetail, admitted.CompletedAt = "", "", nil
	admitted.UpdatedAt = admitted.StartedAt
	admitted.Metrics, admitted.Verification, admitted.Audit = nil, nil, nil
	return recordLoopEvent(ctx, n, admitted, ledger.EventTaskRunAdmitted)
}

func recordLoopEvent(ctx context.Context, n normalized, op Operation, eventType ledger.EventType) error {
	if n.Ledger == nil {
		return nil
	}
	payload := LedgerEvent{SchemaVersion: LedgerEventSchemaVersion, OperationID: op.OperationID, TaskID: op.TaskID, Sequence: op.Sequence, Stage: op.Stage, Cycle: op.Statistics.CyclesStarted, Action: op.LastAction, DecisionID: op.LastDecisionID, RunID: op.LastRunID, StopReason: op.StopReason, StopDetail: op.StopDetail, StartedAt: op.StartedAt, UpdatedAt: op.UpdatedAt, CompletedAt: op.CompletedAt, Statistics: op.Statistics, Metrics: op.Metrics}
	if op.Verification != nil {
		payload.Verification, _ = json.Marshal(op.Verification)
	}
	if op.Audit != nil {
		payload.Audit, _ = json.Marshal(op.Audit)
	}
	want, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	runID := loopLedgerRunID(op.OperationID)
	existing, found, err := n.Ledger.GetRunWithEvents(ctx, runID)
	if err != nil {
		return err
	}
	if !found {
		return errors.New("task run: loop ledger run disappeared")
	}
	for _, event := range existing.Events {
		if event.Type != eventType {
			continue
		}
		var prior LedgerEvent
		if json.Unmarshal(event.Payload, &prior) != nil || prior.OperationID != op.OperationID || prior.Sequence != op.Sequence || prior.Stage != op.Stage {
			continue
		}
		priorRaw, _ := json.Marshal(prior)
		if !bytes.Equal(priorRaw, want) {
			return fmt.Errorf("task run: loop ledger event %q conflicts at sequence %d", eventType, op.Sequence)
		}
		return nil
	}
	_, err = n.Ledger.AppendEvent(ctx, runID, eventType, payload)
	return err
}

// DecodeLedgerEvent rejects unknown fields and schemas. Legacy v1 events are
// intentionally handled by metrics as explicit omissions, not upgraded here.
func DecodeLedgerEvent(raw []byte) (LedgerEvent, error) {
	var event LedgerEvent
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return event, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return event, errors.New("task run ledger event contains trailing JSON")
	}
	if event.SchemaVersion != LedgerEventSchemaVersion {
		return event, fmt.Errorf("unknown task run ledger schema %q", event.SchemaVersion)
	}
	if event.OperationID == "" || event.TaskID == "" || event.Sequence < 0 || event.StartedAt.IsZero() || event.UpdatedAt.Before(event.StartedAt) {
		return event, errors.New("task run ledger event identity or time is malformed")
	}
	if event.StopReason != "" && !event.StopReason.Valid() {
		return event, fmt.Errorf("task run ledger event stop reason %q is invalid", event.StopReason)
	}
	if event.StopReason != "" && (event.CompletedAt == nil || event.CompletedAt.Before(event.StartedAt)) {
		return event, errors.New("task run ledger terminal time is missing or malformed")
	}
	return event, nil
}

func completeLoopLedger(ctx context.Context, n normalized, op Operation) error {
	if n.Ledger == nil {
		return nil
	}
	if err := recordLoopEvent(ctx, n, op, ledger.EventTaskRunStopped); err != nil {
		return err
	}
	runID := loopLedgerRunID(op.OperationID)
	existing, found, err := n.Ledger.GetRunWithEvents(ctx, runID)
	if err != nil || !found {
		return errors.Join(err, errors.New("task run: loop ledger run disappeared before completion"))
	}
	summary := fmt.Sprintf("task %s stopped: %s", op.TaskID, op.StopReason)
	if detail := strings.TrimSpace(op.StopDetail); detail != "" {
		summary += ": " + detail
	}
	if existing.Run.Status != ledger.StatusRunning {
		if existing.Run.Status != ledger.StatusCompleted || existing.Run.Summary != summary || existing.Run.CompletedAt == nil || op.CompletedAt == nil || !existing.Run.CompletedAt.Equal(*op.CompletedAt) {
			return errors.New("task run: completed loop ledger summary conflicts with durable operation")
		}
		return nil
	}
	updated, ok, err := n.Ledger.CompleteRun(ctx, runID, ledger.RunCompletion{Status: ledger.StatusCompleted, Summary: summary, CompletedAt: *op.CompletedAt})
	if err != nil || !ok || updated.Status != ledger.StatusCompleted {
		return errors.Join(err, errors.New("task run: loop ledger completion was not durable"))
	}
	return nil
}
