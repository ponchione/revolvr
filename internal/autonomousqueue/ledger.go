package autonomousqueue

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

const (
	LedgerEventSchemaVersion       = "autonomous-queue-event-v3"
	LegacyLedgerEventSchemaVersion = "autonomous-queue-event-v2"
)

// LedgerEvent is the complete bounded queue occurrence evidence used by
// deterministic metrics and immutable export replay.
type LedgerEvent struct {
	SchemaVersion         string        `json:"schema_version"`
	OperationID           string        `json:"operation_id"`
	Mode                  Mode          `json:"mode"`
	Sequence              int64         `json:"sequence"`
	Sweep                 int64         `json:"sweep"`
	Stage                 string        `json:"stage"`
	TaskID                string        `json:"task_id,omitempty"`
	TaskOperationID       string        `json:"task_operation_id,omitempty"`
	TaskStop              string        `json:"task_stop,omitempty"`
	StopReason            StopReason    `json:"stop_reason,omitempty"`
	StopDetail            string        `json:"stop_detail,omitempty"`
	DaemonWakeCount       int64         `json:"daemon_wake_count,omitempty"`
	DaemonWakeFingerprint string        `json:"daemon_wake_fingerprint,omitempty"`
	StartedAt             time.Time     `json:"started_at"`
	UpdatedAt             time.Time     `json:"updated_at"`
	CompletedAt           *time.Time    `json:"completed_at,omitempty"`
	MaximumWorkers        int           `json:"maximum_workers,omitempty"`
	SequentialFallback    string        `json:"sequential_fallback,omitempty"`
	Slots                 []WorkerSlot  `json:"slots,omitempty"`
	Statistics            Statistics    `json:"statistics"`
	Outcomes              []TaskOutcome `json:"outcomes,omitempty"`
}

func queueLedgerRunID(operationID string) string {
	sum := sha256.Sum256([]byte("autonomous-queue-ledger-v1\x00" + operationID))
	return "queue-" + hex.EncodeToString(sum[:12])
}

func admitQueueLedger(ctx context.Context, n normalized, op Operation) error {
	if n.Ledger == nil {
		return nil
	}
	runID := queueLedgerRunID(op.OperationID)
	history, found, err := n.Ledger.GetRunWithEvents(ctx, runID)
	if err != nil {
		return err
	}
	if !found {
		if _, err := n.Ledger.CreateRun(ctx, ledger.RunSpec{ID: runID, TaskID: op.OperationID, Task: "run autonomous queue until exhausted", StartedAt: op.StartedAt}); err != nil {
			return err
		}
	} else if history.Run.TaskID != op.OperationID || history.Run.Task != "run autonomous queue until exhausted" || !history.Run.StartedAt.Equal(op.StartedAt) {
		return errors.New("autonomous queue: ledger run identity conflict")
	}
	admitted := op
	admitted.Sequence, admitted.Stage, admitted.InFlight = 0, "admitted", nil
	admitted.Slots, admitted.SequentialFallback = nil, ""
	admitted.StopReason, admitted.StopDetail = "", ""
	admitted.UpdatedAt, admitted.CompletedAt = admitted.StartedAt, nil
	admitted.Statistics, admitted.Outcomes = Statistics{}, nil
	if err := recordQueueEvent(ctx, n, admitted, ledger.EventQueueAdmitted); err != nil {
		return err
	}
	if admitted.Mode == ModeDaemon && admitted.DaemonWakeCount > 0 {
		return recordQueueEvent(ctx, n, admitted, ledger.EventQueueDaemonWake)
	}
	return nil
}

func recordQueueEvent(ctx context.Context, n normalized, op Operation, kind ledger.EventType) error {
	if n.Ledger == nil {
		return nil
	}
	schema := LedgerEventSchemaVersion
	if op.SchemaVersion == LegacyOperationSchemaVersion {
		schema = LegacyLedgerEventSchemaVersion
	}
	payload := LedgerEvent{SchemaVersion: schema, OperationID: op.OperationID, Mode: op.Mode, Sequence: op.Sequence, Sweep: op.Sweep, Stage: op.Stage, StopReason: op.StopReason, StopDetail: op.StopDetail, DaemonWakeCount: op.DaemonWakeCount, DaemonWakeFingerprint: op.DaemonWakeFingerprint, StartedAt: op.StartedAt, UpdatedAt: op.UpdatedAt, CompletedAt: op.CompletedAt, Statistics: op.Statistics, Outcomes: append([]TaskOutcome(nil), op.Outcomes...)}
	if schema == LedgerEventSchemaVersion {
		payload.MaximumWorkers, payload.SequentialFallback, payload.Slots = op.MaximumWorkers, op.SequentialFallback, append([]WorkerSlot(nil), op.Slots...)
	}
	if op.InFlight != nil {
		payload.TaskID, payload.TaskOperationID = op.InFlight.TaskID, op.InFlight.TaskOperationID
	} else if len(op.Outcomes) > 0 && kind == ledger.EventQueueTaskStopped {
		last := op.Outcomes[len(op.Outcomes)-1]
		payload.TaskID, payload.TaskOperationID, payload.TaskStop = last.TaskID, last.TaskOperationID, string(last.StopReason)
	}
	want, _ := json.Marshal(payload)
	history, found, err := n.Ledger.GetRunWithEvents(ctx, queueLedgerRunID(op.OperationID))
	if err != nil || !found {
		return errors.Join(err, errors.New("autonomous queue: ledger run disappeared"))
	}
	for _, event := range history.Events {
		if event.Type != kind {
			continue
		}
		var prior LedgerEvent
		if json.Unmarshal(event.Payload, &prior) != nil || prior.OperationID != op.OperationID || prior.Sequence != op.Sequence || prior.Stage != op.Stage {
			continue
		}
		raw, _ := json.Marshal(prior)
		if !bytes.Equal(raw, want) {
			return fmt.Errorf("autonomous queue: ledger event conflict at sequence %d", op.Sequence)
		}
		return nil
	}
	_, err = n.Ledger.AppendEvent(ctx, queueLedgerRunID(op.OperationID), kind, payload)
	return err
}

func DecodeLedgerEvent(raw []byte) (LedgerEvent, error) {
	var event LedgerEvent
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return event, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return event, errors.New("queue ledger event contains trailing JSON")
	}
	if event.SchemaVersion != LedgerEventSchemaVersion && event.SchemaVersion != LegacyLedgerEventSchemaVersion {
		return event, fmt.Errorf("unknown queue ledger schema %q", event.SchemaVersion)
	}
	if event.SchemaVersion == LedgerEventSchemaVersion && (event.MaximumWorkers <= 0 || event.MaximumWorkers > MaximumWorkerLimit) {
		return event, errors.New("queue ledger worker bound is malformed")
	}
	if event.SequentialFallback != "" && event.SequentialFallback != "overlap_authority_unavailable" && event.SequentialFallback != "no_additional_safe_candidate" {
		return event, errors.New("queue ledger sequential fallback is malformed")
	}
	if event.OperationID == "" || event.Sequence < 0 || event.Sweep <= 0 || event.StartedAt.IsZero() || event.UpdatedAt.Before(event.StartedAt) {
		return event, errors.New("queue ledger event identity or time is malformed")
	}
	if event.StopReason != "" && !event.StopReason.Valid() {
		return event, fmt.Errorf("queue ledger stop reason %q is invalid", event.StopReason)
	}
	if event.StopReason != "" && (event.CompletedAt == nil || event.CompletedAt.Before(event.StartedAt)) {
		return event, errors.New("queue ledger terminal time is missing or malformed")
	}
	return event, nil
}

func completeQueueLedger(ctx context.Context, n normalized, op Operation) error {
	if n.Ledger == nil {
		return nil
	}
	if err := recordQueueEvent(ctx, n, op, ledger.EventQueueStopped); err != nil {
		return err
	}
	runID := queueLedgerRunID(op.OperationID)
	history, found, err := n.Ledger.GetRunWithEvents(ctx, runID)
	if err != nil || !found || op.CompletedAt == nil {
		return errors.Join(err, errors.New("autonomous queue: terminal ledger authority is incomplete"))
	}
	summary := fmt.Sprintf("queue %s stopped: %s", op.OperationID, op.StopReason)
	if strings.TrimSpace(op.StopDetail) != "" {
		summary += ": " + op.StopDetail
	}
	if history.Run.Status != ledger.StatusRunning {
		if history.Run.Status != ledger.StatusCompleted || history.Run.Summary != summary || history.Run.CompletedAt == nil || !history.Run.CompletedAt.Equal(*op.CompletedAt) {
			return errors.New("autonomous queue: terminal ledger summary conflict")
		}
		return nil
	}
	updated, ok, err := n.Ledger.CompleteRun(ctx, runID, ledger.RunCompletion{Status: ledger.StatusCompleted, Summary: summary, CompletedAt: *op.CompletedAt})
	if err != nil || !ok || updated.Status != ledger.StatusCompleted {
		return errors.Join(err, errors.New("autonomous queue: ledger completion was not durable"))
	}
	return nil
}
