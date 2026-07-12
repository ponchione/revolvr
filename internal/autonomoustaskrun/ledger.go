package autonomoustaskrun

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

type loopEvent struct {
	SchemaVersion string     `json:"schema_version"`
	OperationID   string     `json:"operation_id"`
	TaskID        string     `json:"task_id"`
	Sequence      int64      `json:"sequence"`
	Stage         string     `json:"stage"`
	Cycle         int64      `json:"cycle"`
	Action        string     `json:"action,omitempty"`
	DecisionID    string     `json:"decision_id,omitempty"`
	RunID         string     `json:"run_id,omitempty"`
	StopReason    StopReason `json:"stop_reason,omitempty"`
	StopDetail    string     `json:"stop_detail,omitempty"`
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
	return recordLoopEvent(ctx, n, admitted, ledger.EventTaskRunAdmitted)
}

func recordLoopEvent(ctx context.Context, n normalized, op Operation, eventType ledger.EventType) error {
	if n.Ledger == nil {
		return nil
	}
	payload := loopEvent{SchemaVersion: "autonomous-task-run-event-v1", OperationID: op.OperationID, TaskID: op.TaskID, Sequence: op.Sequence, Stage: op.Stage, Cycle: op.Statistics.CyclesStarted, Action: op.LastAction, DecisionID: op.LastDecisionID, RunID: op.LastRunID, StopReason: op.StopReason, StopDetail: op.StopDetail}
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
		var prior loopEvent
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
