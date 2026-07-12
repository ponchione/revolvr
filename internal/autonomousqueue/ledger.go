package autonomousqueue

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"revolvr/internal/ledger"
)

type queueEvent struct {
	SchemaVersion         string     `json:"schema_version"`
	OperationID           string     `json:"operation_id"`
	Mode                  Mode       `json:"mode"`
	Sequence              int64      `json:"sequence"`
	Sweep                 int64      `json:"sweep"`
	Stage                 string     `json:"stage"`
	TaskID                string     `json:"task_id,omitempty"`
	TaskOperationID       string     `json:"task_operation_id,omitempty"`
	TaskStop              string     `json:"task_stop,omitempty"`
	StopReason            StopReason `json:"stop_reason,omitempty"`
	StopDetail            string     `json:"stop_detail,omitempty"`
	DaemonWakeCount       int64      `json:"daemon_wake_count,omitempty"`
	DaemonWakeFingerprint string     `json:"daemon_wake_fingerprint,omitempty"`
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
	admitted.StopReason, admitted.StopDetail = "", ""
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
	payload := queueEvent{SchemaVersion: "autonomous-queue-event-v1", OperationID: op.OperationID, Mode: op.Mode, Sequence: op.Sequence, Sweep: op.Sweep, Stage: op.Stage, StopReason: op.StopReason, StopDetail: op.StopDetail, DaemonWakeCount: op.DaemonWakeCount, DaemonWakeFingerprint: op.DaemonWakeFingerprint}
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
		var prior queueEvent
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
