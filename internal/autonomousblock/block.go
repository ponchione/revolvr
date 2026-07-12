// Package autonomousblock owns the one-way durable transition for an exact
// supervisor block decision. It runs no model, worker, verification, or Git
// operation.
package autonomousblock

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomouspolicy"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/taskfile"
)

type Config struct {
	RepositoryRoot string
	TaskID         string
	OperationID    string
	Expected       autonomousstate.ExpectedState
	Decision       autonomous.SupervisorDecision
	Reference      autonomous.DecisionReference
	Source         autonomouspolicy.SourceEvidence
	Verification   *autonomouspolicy.VerificationEvidence
	Audit          *autonomouspolicy.AuditEvidence
	CreatedAt      time.Time
	Store          *autonomousstate.Store
}

type Result struct {
	Disposition autonomousstate.CommitDisposition
	Current     autonomousstate.Snapshot
	History     autonomousstate.BlockHistorySnapshot
}

func Apply(ctx context.Context, cfg Config) (Result, error) {
	if cfg.Store == nil || strings.TrimSpace(cfg.RepositoryRoot) == "" || strings.TrimSpace(cfg.TaskID) == "" || strings.TrimSpace(cfg.OperationID) == "" || cfg.CreatedAt.IsZero() {
		return Result{}, errors.New("apply explicit block: store, identities, and time are required")
	}
	if err := cfg.Expected.Validate(); err != nil || !cfg.Expected.Exists {
		return Result{}, errors.New("apply explicit block: exact existing state expectation is required")
	}
	application, err := applicationSHA(cfg)
	if err != nil {
		return Result{}, err
	}
	if history, found, loadErr := cfg.Store.LoadBlockOperation(ctx, cfg.TaskID, cfg.OperationID); loadErr != nil {
		return Result{}, loadErr
	} else if found {
		if history.Record.ApplicationSHA256 != application {
			return Result{}, autonomousstate.ErrOperationConflict
		}
		current, stateFound, stateErr := cfg.Store.Load(ctx, cfg.TaskID)
		if stateErr != nil || !stateFound {
			return Result{}, errors.Join(stateErr, autonomousstate.ErrStateMissing)
		}
		if current.SHA256 != history.Record.ResultingState.SHA256 || current.ByteSize != history.Record.ResultingState.ByteSize {
			return Result{}, autonomousstate.ErrStaleWrite
		}
		if err := publishBlockedTask(cfg.RepositoryRoot, cfg.TaskID); err != nil {
			return Result{}, err
		}
		return Result{Disposition: autonomousstate.CommitReplayed, Current: current, History: history}, nil
	}
	snapshot, found, err := cfg.Store.Load(ctx, cfg.TaskID)
	if err != nil || !found {
		return Result{}, errors.Join(err, autonomousstate.ErrStateMissing)
	}
	if snapshot.SHA256 != cfg.Expected.SHA256 || snapshot.ByteSize != cfg.Expected.ByteSize {
		return Result{}, autonomousstate.ErrStaleWrite
	}
	route, err := autonomouspolicy.Evaluate(autonomouspolicy.Input{TaskID: cfg.TaskID, Decision: cfg.Decision, Reference: cfg.Reference, State: snapshot.State, Source: cfg.Source, Verification: cfg.Verification, Audit: cfg.Audit})
	if err != nil || route.Kind != autonomouspolicy.RouteKindBlock {
		return Result{}, errors.Join(err, errors.New("apply explicit block: policy did not authorize the exact block decision"))
	}
	evidence := append([]autonomous.EvidenceReference(nil), cfg.Decision.Inputs...)
	evidence = append(evidence, cfg.Reference.Artifact)
	next, err := cloneState(snapshot.State)
	if err != nil {
		return Result{}, err
	}
	next.Lifecycle = autonomous.LifecycleStateBlocked
	next.LatestDecision = &cfg.Reference
	next.Terminal = &autonomous.TerminalDetail{Reason: cfg.Decision.Rationale, Evidence: evidence}
	if err := next.Validate(); err != nil {
		return Result{}, err
	}
	previousIdentity, err := autonomousstate.StateIdentityFor(snapshot.SourcePath, true, snapshot.State)
	if err != nil {
		return Result{}, err
	}
	resultingIdentity, err := autonomousstate.StateIdentityFor(snapshot.SourcePath, true, next)
	if err != nil {
		return Result{}, err
	}
	history := autonomousstate.BlockHistoryRecord{SchemaVersion: autonomousstate.BlockHistorySchemaVersion, TaskID: cfg.TaskID, OperationID: cfg.OperationID, ApplicationSHA256: application, Decision: cfg.Reference, Reason: cfg.Decision.Rationale, Evidence: evidence, SourceRevision: cfg.Source.Revision, CreatedAt: cfg.CreatedAt.UTC(), PreviousState: previousIdentity, ResultingState: resultingIdentity}
	committed, err := cfg.Store.CommitBlock(context.WithoutCancel(ctx), autonomousstate.BlockCommitRequest{TaskID: cfg.TaskID, Expected: cfg.Expected, PreviousState: snapshot.State, NextState: next, History: history})
	if err != nil {
		return Result{}, err
	}
	if err := publishBlockedTask(cfg.RepositoryRoot, cfg.TaskID); err != nil {
		return Result{}, err
	}
	return Result{Disposition: committed.Disposition, Current: committed.Current, History: committed.History}, nil
}

func publishBlockedTask(root, taskID string) error {
	task, found, err := taskfile.FindByID(root, taskID)
	if err != nil || !found {
		return errors.Join(err, autonomousstate.ErrTaskMissing)
	}
	if task.Status == taskfile.StatusBlocked {
		return nil
	}
	if task.Status != taskfile.StatusPending {
		return fmt.Errorf("apply explicit block: canonical task status %q cannot become blocked", task.Status)
	}
	_, err = taskfile.UpdateStatus(root, task.SourcePath, taskfile.StatusBlocked)
	return err
}

func applicationSHA(cfg Config) (string, error) {
	raw, err := json.Marshal(struct {
		TaskID, OperationID string
		Expected            autonomousstate.ExpectedState
		Decision            autonomous.SupervisorDecision
		Reference           autonomous.DecisionReference
		Source              autonomouspolicy.SourceEvidence
	}{cfg.TaskID, cfg.OperationID, cfg.Expected, cfg.Decision, cfg.Reference, cfg.Source})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func cloneState(state autonomous.ExecutionState) (autonomous.ExecutionState, error) {
	raw, err := json.Marshal(state)
	if err != nil {
		return autonomous.ExecutionState{}, err
	}
	var clone autonomous.ExecutionState
	if err := json.Unmarshal(raw, &clone); err != nil {
		return autonomous.ExecutionState{}, err
	}
	return clone, nil
}
