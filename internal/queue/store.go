package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"revolvr/internal/scheduler"
	"revolvr/internal/storage/postgres"
)

var (
	ErrNotFound          = errors.New("sequential queue operation not found")
	ErrOperationActive   = errors.New("another sequential queue operation is active")
	ErrOperationConflict = errors.New("sequential queue operation authority conflict")
	ErrOperationBusy     = errors.New("sequential queue operation is already owned")
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) (*Store, error) {
	if pool == nil {
		return nil, errors.New("sequential queue: PostgreSQL pool is required")
	}
	return &Store{pool: pool}, nil
}

func (s *Store) acquire(ctx context.Context, operationID string) (func(), error) {
	connection, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	var acquired bool
	if err := connection.QueryRow(ctx,
		"SELECT pg_try_advisory_lock(hashtextextended($1, 0))", "revolvr-queue:"+operationID,
	).Scan(&acquired); err != nil {
		connection.Release()
		return nil, err
	}
	if !acquired {
		connection.Release()
		return nil, ErrOperationBusy
	}
	return func() {
		var released bool
		_ = connection.QueryRow(context.Background(),
			"SELECT pg_advisory_unlock(hashtextextended($1, 0))", "revolvr-queue:"+operationID,
		).Scan(&released)
		connection.Release()
	}, nil
}

func (s *Store) createOrLoad(ctx context.Context, operationID string, configuration PinnedConfiguration, configSHA string, raw []byte, now time.Time) (postgres.CoreQueueOperation, bool, error) {
	operationUUID, err := parseUUID(operationID)
	if err != nil {
		return postgres.CoreQueueOperation{}, false, err
	}
	existing, err := postgres.New(s.pool).GetQueueOperation(ctx, operationUUID)
	if err == nil {
		if err := compatibleOperation(existing, configuration, configSHA, raw); err != nil {
			return postgres.CoreQueueOperation{}, false, err
		}
		return existing, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return postgres.CoreQueueOperation{}, false, err
	}

	var created postgres.CoreQueueOperation
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		queries := postgres.New(tx)
		created, err = queries.InsertQueueOperation(ctx, postgres.InsertQueueOperationParams{
			ID: operationUUID, SchemaVersion: OperationSchemaVersion,
			WorkerMode: WorkerModeDirectTools, MaximumWorkers: MaximumWorkers,
			QualityGateStatus: string(configuration.QualityGateStatus),
			ConfigSchema:      configuration.SchemaVersion, ConfigSha256: configSHA,
			Configuration: raw, MaxTasks: configuration.Limits.MaximumTasks,
			MaxCyclesPerTask:        configuration.Limits.MaximumCyclesPerTask,
			MaxTotalCycles:          configuration.Limits.MaximumTotalCycles,
			MaxRemoteTokens:         configuration.Limits.MaximumRemoteTokens,
			MaxCostMicrousd:         configuration.Limits.MaximumCostMicrousd,
			MaxDurationMilliseconds: configuration.Limits.MaximumDurationMillis,
			StartedAt:               timestamp(now), DeadlineAt: timestamp(now.Add(configuration.Limits.MaximumDuration)),
			UpdatedAt: timestamp(now),
		})
		if err != nil {
			return err
		}
		return appendQueueEvent(ctx, queries, created, "queue.operation.started", map[string]any{
			"operation_id": operationID, "configuration_sha256": configSHA,
			"worker_mode": WorkerModeDirectTools, "maximum_workers": MaximumWorkers,
			"quality_gate_status": configuration.QualityGateStatus,
		})
	})
	if uniqueViolation(err) {
		return postgres.CoreQueueOperation{}, false, ErrOperationActive
	}
	if err != nil {
		return postgres.CoreQueueOperation{}, false, err
	}
	return created, false, nil
}

func compatibleOperation(operation postgres.CoreQueueOperation, configuration PinnedConfiguration, configSHA string, raw []byte) error {
	var stored PinnedConfiguration
	storedRaw, storedSHA, err := canonicalJSON(stored)
	if unmarshalErr := decodeStrict(operation.Configuration, &stored); unmarshalErr == nil {
		storedRaw, storedSHA, err = canonicalJSON(stored)
	}
	if operation.SchemaVersion != OperationSchemaVersion || operation.WorkerMode != WorkerModeDirectTools ||
		operation.MaximumWorkers != MaximumWorkers || operation.ConfigSchema != configuration.SchemaVersion ||
		err != nil || operation.ConfigSha256 != configSHA || storedSHA != configSHA || string(storedRaw) != string(raw) ||
		operation.QualityGateStatus != string(configuration.QualityGateStatus) {
		return ErrOperationConflict
	}
	return nil
}

func (s *Store) getOperation(ctx context.Context, operationID string) (postgres.CoreQueueOperation, error) {
	id, err := parseUUID(operationID)
	if err != nil {
		return postgres.CoreQueueOperation{}, err
	}
	operation, err := postgres.New(s.pool).GetQueueOperation(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return postgres.CoreQueueOperation{}, ErrNotFound
	}
	return operation, err
}

func (s *Store) openOccurrence(ctx context.Context, operationID string) (postgres.CoreQueueTaskOccurrence, bool, error) {
	id, err := parseUUID(operationID)
	if err != nil {
		return postgres.CoreQueueTaskOccurrence{}, false, err
	}
	var occurrence postgres.CoreQueueTaskOccurrence
	err = s.pool.QueryRow(ctx, `SELECT `+occurrenceColumns+`
		FROM core.queue_task_occurrences
		WHERE queue_operation_id = $1 AND state <> 'checkpointed'
		ORDER BY occurrence_sequence LIMIT 1`, id).Scan(occurrenceDestinations(&occurrence)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return postgres.CoreQueueTaskOccurrence{}, false, nil
	}
	return occurrence, err == nil, err
}

type selectionIntent struct {
	OccurrenceID   string
	SchedulerRunID string
	Sequence       int64
	Replayed       bool
}

func (s *Store) beginSelection(ctx context.Context, operationID, occurrenceID, schedulerRunID string, now time.Time) (selectionIntent, error) {
	opID, err := parseUUID(operationID)
	if err != nil {
		return selectionIntent{}, err
	}
	occID, err := parseUUID(occurrenceID)
	if err != nil {
		return selectionIntent{}, err
	}
	runID, err := parseUUID(schedulerRunID)
	if err != nil {
		return selectionIntent{}, err
	}
	intent := selectionIntent{OccurrenceID: occurrenceID, SchedulerRunID: schedulerRunID}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var operation postgres.CoreQueueOperation
		if err := tx.QueryRow(ctx, `SELECT `+operationColumns+` FROM core.queue_operations WHERE id=$1 FOR UPDATE`, opID).Scan(operationDestinations(&operation)...); err != nil {
			return err
		}
		if operation.Status != "active" || operation.ActiveOccurrenceID.Valid {
			return ErrOperationConflict
		}
		if operation.SelectionIntentID.Valid {
			intent = selectionIntent{
				OccurrenceID:   uuidString(operation.SelectionIntentID),
				SchedulerRunID: uuidString(operation.SelectionSchedulerRunID),
				Sequence:       operation.SelectionIntentSequence.Int64, Replayed: true,
			}
			return nil
		}
		intent.Sequence = operation.NextOccurrenceSequence
		operation.SelectionIntentID, operation.SelectionSchedulerRunID = occID, runID
		operation.SelectionIntentSequence = int8Value(intent.Sequence)
		operation.AggregateVersion++
		operation.UpdatedAt = timestamp(now)
		if _, err := tx.Exec(ctx, `UPDATE core.queue_operations SET selection_intent_id=$2,
			selection_scheduler_run_id=$3, selection_intent_sequence=$4,
			aggregate_version=$5, updated_at=$6 WHERE id=$1`, opID, occID, runID,
			intent.Sequence, operation.AggregateVersion, now); err != nil {
			return err
		}
		return appendQueueEvent(ctx, postgres.New(tx), operation, "queue.task.selection_intent", map[string]any{
			"operation_id": operationID, "occurrence_id": occurrenceID,
			"occurrence_sequence": intent.Sequence, "scheduler_run_id": schedulerRunID,
		})
	})
	return intent, err
}

func (s *Store) clearSelection(ctx context.Context, operationID string, intent selectionIntent, reason string, now time.Time) error {
	opID, err := parseUUID(operationID)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var operation postgres.CoreQueueOperation
		if err := tx.QueryRow(ctx, `SELECT `+operationColumns+` FROM core.queue_operations WHERE id=$1 FOR UPDATE`, opID).Scan(operationDestinations(&operation)...); err != nil {
			return err
		}
		if uuidString(operation.SelectionIntentID) != intent.OccurrenceID ||
			uuidString(operation.SelectionSchedulerRunID) != intent.SchedulerRunID ||
			operation.SelectionIntentSequence.Int64 != intent.Sequence {
			return ErrOperationConflict
		}
		operation.SelectionIntentID, operation.SelectionSchedulerRunID = pgtype.UUID{}, pgtype.UUID{}
		operation.SelectionIntentSequence = pgtype.Int8{}
		operation.AggregateVersion++
		operation.UpdatedAt = timestamp(now)
		if _, err := tx.Exec(ctx, `UPDATE core.queue_operations SET selection_intent_id=NULL,
			selection_scheduler_run_id=NULL, selection_intent_sequence=NULL,
			aggregate_version=$2, updated_at=$3 WHERE id=$1`, opID, operation.AggregateVersion, now); err != nil {
			return err
		}
		return appendQueueEvent(ctx, postgres.New(tx), operation, "queue.task.selection_empty", map[string]any{
			"operation_id": operationID, "selection_intent_id": intent.OccurrenceID,
			"occurrence_sequence": intent.Sequence, "reason": reason,
		})
	})
}

func (s *Store) persistSelectedOccurrence(ctx context.Context, operationID, coordinator string, intent selectionIntent, candidate scheduler.Candidate, now time.Time) (postgres.CoreQueueTaskOccurrence, error) {
	opID, err := parseUUID(operationID)
	if err != nil {
		return postgres.CoreQueueTaskOccurrence{}, err
	}
	raw, selectionSHA, err := canonicalJSON(candidate)
	if err != nil {
		return postgres.CoreQueueTaskOccurrence{}, err
	}
	projectID, err := parseUUID(candidate.ProjectID)
	if err != nil {
		return postgres.CoreQueueTaskOccurrence{}, err
	}
	sourceID, err := parseUUID(candidate.ProjectSourceID)
	if err != nil {
		return postgres.CoreQueueTaskOccurrence{}, err
	}
	taskID, err := parseUUID(candidate.TaskID)
	if err != nil {
		return postgres.CoreQueueTaskOccurrence{}, err
	}
	versionID, err := parseUUID(candidate.TaskVersionID)
	if err != nil {
		return postgres.CoreQueueTaskOccurrence{}, err
	}
	occID, err := parseUUID(intent.OccurrenceID)
	if err != nil {
		return postgres.CoreQueueTaskOccurrence{}, err
	}
	runID, err := parseUUID(intent.SchedulerRunID)
	if err != nil {
		return postgres.CoreQueueTaskOccurrence{}, err
	}
	var occurrence postgres.CoreQueueTaskOccurrence
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var operation postgres.CoreQueueOperation
		if err := tx.QueryRow(ctx, `SELECT `+operationColumns+` FROM core.queue_operations WHERE id=$1 FOR UPDATE`, opID).Scan(operationDestinations(&operation)...); err != nil {
			return err
		}
		if operation.Status != "active" || operation.ActiveOccurrenceID.Valid ||
			uuidString(operation.SelectionIntentID) != intent.OccurrenceID ||
			uuidString(operation.SelectionSchedulerRunID) != intent.SchedulerRunID ||
			operation.SelectionIntentSequence.Int64 != intent.Sequence {
			return ErrOperationConflict
		}
		occurrence = postgres.CoreQueueTaskOccurrence{
			ID: occID, QueueOperationID: operation.ID, OccurrenceSequence: intent.Sequence,
			State: "selected", SchedulerRunID: runID, CoordinatorIdentity: coordinator,
			ProjectID: projectID, ProjectSourceID: sourceID, TaskID: taskID, TaskVersionID: versionID,
			ExternalTaskID:               text(candidate.ExternalTaskID),
			ExpectedTaskAggregateVersion: int8Value(candidate.ExpectedAggregateVersion),
			TaskPriority:                 int4Value(candidate.Priority), TaskCreatedAt: timestamp(candidate.CreatedAt),
			SourceCommit: text(candidate.SourceCommit), SourceTree: text(candidate.SourceTree),
			Selection: raw, SelectionSha256: text(selectionSHA), SelectedAt: timestamp(now),
			CreatedAt: timestamp(now), UpdatedAt: timestamp(now),
		}
		_, err := tx.Exec(ctx, `INSERT INTO core.queue_task_occurrences (
			id,queue_operation_id,occurrence_sequence,state,scheduler_run_id,
			coordinator_identity,project_id,project_source_id,task_id,task_version_id,
			external_task_id,expected_task_aggregate_version,task_priority,task_created_at,
			source_commit,source_tree,selection,selection_sha256,selected_at,created_at,updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)`,
			occurrence.ID, occurrence.QueueOperationID, occurrence.OccurrenceSequence,
			occurrence.State, occurrence.SchedulerRunID, occurrence.CoordinatorIdentity,
			occurrence.ProjectID, occurrence.ProjectSourceID, occurrence.TaskID,
			occurrence.TaskVersionID, occurrence.ExternalTaskID,
			occurrence.ExpectedTaskAggregateVersion, occurrence.TaskPriority,
			occurrence.TaskCreatedAt, occurrence.SourceCommit, occurrence.SourceTree,
			occurrence.Selection, occurrence.SelectionSha256, occurrence.SelectedAt,
			occurrence.CreatedAt, occurrence.UpdatedAt)
		if err != nil {
			return err
		}
		operation.SelectionIntentID, operation.SelectionSchedulerRunID = pgtype.UUID{}, pgtype.UUID{}
		operation.SelectionIntentSequence = pgtype.Int8{}
		operation.ActiveOccurrenceID = occurrence.ID
		operation.NextOccurrenceSequence++
		operation.AggregateVersion++
		operation.UpdatedAt = timestamp(now)
		if _, err := tx.Exec(ctx, `UPDATE core.queue_operations SET selection_intent_id=NULL,
			selection_scheduler_run_id=NULL, selection_intent_sequence=NULL,
			active_occurrence_id=$2,next_occurrence_sequence=$3,aggregate_version=$4,
			updated_at=$5 WHERE id=$1`, operation.ID, occurrence.ID,
			operation.NextOccurrenceSequence, operation.AggregateVersion, now); err != nil {
			return err
		}
		return appendQueueEvent(ctx, postgres.New(tx), operation, "queue.task.selected", map[string]any{
			"operation_id": operationID, "occurrence_id": intent.OccurrenceID,
			"occurrence_sequence": intent.Sequence, "task_id": candidate.TaskID,
			"task_version_id": candidate.TaskVersionID, "scheduler_run_id": intent.SchedulerRunID,
			"selection_sha256": selectionSHA,
		})
	})
	return occurrence, err
}

func (s *Store) markAdmitted(ctx context.Context, occurrenceID string, now time.Time) (postgres.CoreQueueTaskOccurrence, error) {
	id, err := parseUUID(occurrenceID)
	if err != nil {
		return postgres.CoreQueueTaskOccurrence{}, err
	}
	var occurrence postgres.CoreQueueTaskOccurrence
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		current, operation, err := lockOccurrenceAndOperation(ctx, tx, id)
		if err != nil {
			return err
		}
		if current.State == "admitted" || current.State == "runner_terminal" || current.State == "checkpointed" {
			occurrence = current
			return nil
		}
		if current.State != "selected" {
			return ErrOperationConflict
		}
		if _, err := tx.Exec(ctx, `UPDATE core.queue_task_occurrences SET state='admitted', admitted_at=$2, updated_at=$2 WHERE id=$1`, id, now); err != nil {
			return err
		}
		operation.AggregateVersion++
		operation.UpdatedAt = timestamp(now)
		operation.PeakSourceMutatingWorkers = 1
		if _, err := tx.Exec(ctx, `UPDATE core.queue_operations SET aggregate_version=$2,
			updated_at=$3, peak_source_mutating_workers=1 WHERE id=$1`, operation.ID, operation.AggregateVersion, now); err != nil {
			return err
		}
		if err := appendQueueEvent(ctx, postgres.New(tx), operation, "queue.task.admitted", map[string]any{
			"operation_id": uuidString(operation.ID), "occurrence_id": uuidString(id),
			"scheduler_run_id": uuidString(current.SchedulerRunID), "maximum_workers": MaximumWorkers,
		}); err != nil {
			return err
		}
		current.State, current.AdmittedAt, current.UpdatedAt = "admitted", timestamp(now), timestamp(now)
		occurrence = current
		return nil
	})
	return occurrence, err
}

func (s *Store) persistEffectIntent(ctx context.Context, operationID, occurrenceID string, kind EffectKind, identity, material string, now time.Time) (EffectAdmission, error) {
	if !kind.valid() {
		return EffectAdmission{}, errors.New("sequential queue: invalid effect kind")
	}
	if err := validateEffectIdentity(identity, material); err != nil {
		return EffectAdmission{}, err
	}
	opID, _ := parseUUID(operationID)
	occID, _ := parseUUID(occurrenceID)
	var result EffectAdmission
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var state string
		if err := tx.QueryRow(ctx, `SELECT state FROM core.queue_task_occurrences
			WHERE id=$1 AND queue_operation_id=$2 FOR UPDATE`, occID, opID).Scan(&state); err != nil {
			return err
		}
		if state != "admitted" {
			return ErrOperationConflict
		}
		var row postgres.CoreQueueTaskEffect
		err := tx.QueryRow(ctx, `SELECT id, queue_operation_id, task_occurrence_id,
			effect_sequence, effect_kind, effect_identity, material_sha256, status,
			evidence_sha256, intended_at, completed_at FROM core.queue_task_effects
			WHERE task_occurrence_id=$1 AND effect_identity=$2`, occID, identity).Scan(
			&row.ID, &row.QueueOperationID, &row.TaskOccurrenceID, &row.EffectSequence,
			&row.EffectKind, &row.EffectIdentity, &row.MaterialSha256, &row.Status,
			&row.EvidenceSha256, &row.IntendedAt, &row.CompletedAt)
		if err == nil {
			if row.EffectKind != string(kind) || row.MaterialSha256 != material {
				return ErrOperationConflict
			}
			result = EffectAdmission{Sequence: row.EffectSequence, Completed: row.Status == "completed", Replayed: true, Evidence: row.EvidenceSha256.String}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var sequence int64
		if err := tx.QueryRow(ctx, `SELECT COALESCE(max(effect_sequence),0)+1
			FROM core.queue_task_effects WHERE task_occurrence_id=$1`, occID).Scan(&sequence); err != nil {
			return err
		}
		effectID := pgtype.UUID{Bytes: uuid.Must(uuid.NewV7()), Valid: true}
		if _, err := tx.Exec(ctx, `INSERT INTO core.queue_task_effects (
			id, queue_operation_id, task_occurrence_id, effect_sequence, effect_kind,
			effect_identity, material_sha256, status, intended_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,'intent',$8)`, effectID, opID, occID,
			sequence, string(kind), identity, material, now); err != nil {
			return err
		}
		operation, err := advanceOperation(ctx, tx, opID, now)
		if err != nil {
			return err
		}
		if err := appendQueueEvent(ctx, postgres.New(tx), operation, "queue.task.effect_intent", map[string]any{
			"operation_id": operationID, "occurrence_id": occurrenceID,
			"effect_sequence": sequence, "effect_kind": kind,
			"effect_identity": identity, "material_sha256": material,
		}); err != nil {
			return err
		}
		result = EffectAdmission{Sequence: sequence}
		return nil
	})
	return result, err
}

func (s *Store) persistEffectCompletion(ctx context.Context, operationID, occurrenceID string, kind EffectKind, identity, material, evidence string, now time.Time) (EffectAdmission, error) {
	if err := validateEffectIdentity(identity, material); err != nil || !validHash(evidence) {
		if err != nil {
			return EffectAdmission{}, err
		}
		return EffectAdmission{}, errors.New("sequential queue: invalid effect evidence SHA-256")
	}
	opID, _ := parseUUID(operationID)
	occID, _ := parseUUID(occurrenceID)
	var result EffectAdmission
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var row postgres.CoreQueueTaskEffect
		if err := tx.QueryRow(ctx, `SELECT id, queue_operation_id, task_occurrence_id,
			effect_sequence, effect_kind, effect_identity, material_sha256, status,
			evidence_sha256, intended_at, completed_at FROM core.queue_task_effects
			WHERE task_occurrence_id=$1 AND effect_identity=$2 FOR UPDATE`, occID, identity).Scan(
			&row.ID, &row.QueueOperationID, &row.TaskOccurrenceID, &row.EffectSequence,
			&row.EffectKind, &row.EffectIdentity, &row.MaterialSha256, &row.Status,
			&row.EvidenceSha256, &row.IntendedAt, &row.CompletedAt); err != nil {
			return err
		}
		if row.QueueOperationID != opID || row.EffectKind != string(kind) || row.MaterialSha256 != material {
			return ErrOperationConflict
		}
		if row.Status == "completed" {
			if row.EvidenceSha256.String != evidence {
				return ErrOperationConflict
			}
			result = EffectAdmission{Sequence: row.EffectSequence, Completed: true, Replayed: true, Evidence: evidence}
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE core.queue_task_effects SET status='completed', evidence_sha256=$2, completed_at=$3 WHERE id=$1`, row.ID, evidence, now); err != nil {
			return err
		}
		operation, err := advanceOperation(ctx, tx, opID, now)
		if err != nil {
			return err
		}
		if err := appendQueueEvent(ctx, postgres.New(tx), operation, "queue.task.effect_completed", map[string]any{
			"operation_id": operationID, "occurrence_id": occurrenceID,
			"effect_sequence": row.EffectSequence, "effect_kind": kind,
			"effect_identity": identity, "material_sha256": material,
			"evidence_sha256": evidence,
		}); err != nil {
			return err
		}
		result = EffectAdmission{Sequence: row.EffectSequence, Completed: true, Evidence: evidence}
		return nil
	})
	return result, err
}

func (s *Store) listEffects(ctx context.Context, occurrenceID pgtype.UUID) ([]postgres.CoreQueueTaskEffect, error) {
	return postgres.New(s.pool).ListQueueTaskEffects(ctx, occurrenceID)
}

func (s *Store) persistRunnerTerminal(ctx context.Context, occurrenceID string, result TaskResult, now time.Time) (postgres.CoreQueueTaskOccurrence, bool, error) {
	id, err := parseUUID(occurrenceID)
	if err != nil {
		return postgres.CoreQueueTaskOccurrence{}, false, err
	}
	raw, resultSHA, err := canonicalJSON(result)
	if err != nil {
		return postgres.CoreQueueTaskOccurrence{}, false, err
	}
	effects, err := s.listEffects(ctx, id)
	if err != nil {
		return postgres.CoreQueueTaskOccurrence{}, false, err
	}
	effectChainSHA, err := effectChainHash(effects)
	if err != nil {
		return postgres.CoreQueueTaskOccurrence{}, false, err
	}
	var occurrence postgres.CoreQueueTaskOccurrence
	replayed := false
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		current, operation, err := lockOccurrenceAndOperation(ctx, tx, id)
		if err != nil {
			return err
		}
		if current.State == "runner_terminal" || current.State == "checkpointed" {
			var stored TaskResult
			storedRaw, storedSHA, marshalErr := canonicalJSON(stored)
			if unmarshalErr := decodeStrict(current.Result, &stored); unmarshalErr == nil {
				storedRaw, storedSHA, marshalErr = canonicalJSON(stored)
			}
			if marshalErr != nil || current.ResultSha256.String != resultSHA || storedSHA != resultSHA ||
				string(storedRaw) != string(raw) || current.EffectChainSha256.String != effectChainSHA {
				return ErrOperationConflict
			}
			occurrence, replayed = current, true
			return nil
		}
		if current.State != "admitted" {
			return ErrOperationConflict
		}
		if _, err := tx.Exec(ctx, `UPDATE core.queue_task_occurrences SET state='runner_terminal',
			outcome=$2, outcome_detail=$3, result=$4, result_sha256=$5, effect_chain_sha256=$6,
			cycles_consumed=$7, remote_tokens_consumed=$8, cost_microusd_consumed=$9,
			workspace_reconciled=$10, evidence_reconciled=$11,
			runner_terminal_at=$12, updated_at=$12 WHERE id=$1`, id, string(result.Outcome),
			result.Detail, raw, resultSHA, effectChainSHA, result.CyclesConsumed,
			result.RemoteTokensConsumed, result.CostMicrousdConsumed,
			result.Reconciliation.Workspace, result.Reconciliation.Evidence, now); err != nil {
			return err
		}
		operation.AggregateVersion++
		operation.UpdatedAt = timestamp(now)
		if _, err := tx.Exec(ctx, `UPDATE core.queue_operations SET aggregate_version=$2, updated_at=$3 WHERE id=$1`, operation.ID, operation.AggregateVersion, now); err != nil {
			return err
		}
		if err := appendQueueEvent(ctx, postgres.New(tx), operation, "queue.task.runner_terminal", map[string]any{
			"operation_id": uuidString(operation.ID), "occurrence_id": occurrenceID,
			"outcome": result.Outcome, "result_sha256": resultSHA,
		}); err != nil {
			return err
		}
		current.State, current.Outcome, current.OutcomeDetail = "runner_terminal", text(string(result.Outcome)), text(result.Detail)
		current.Result, current.ResultSha256, current.EffectChainSha256 = raw, text(resultSHA), text(effectChainSHA)
		current.CyclesConsumed, current.RemoteTokensConsumed = int8Value(result.CyclesConsumed), int8Value(result.RemoteTokensConsumed)
		current.CostMicrousdConsumed = int8Value(result.CostMicrousdConsumed)
		current.WorkspaceReconciled, current.EvidenceReconciled = boolValue(result.Reconciliation.Workspace), boolValue(result.Reconciliation.Evidence)
		current.RunnerTerminalAt, current.UpdatedAt = timestamp(now), timestamp(now)
		occurrence = current
		return nil
	})
	return occurrence, replayed, err
}

func (s *Store) checkpoint(ctx context.Context, occurrenceID string, now time.Time) (postgres.CoreQueueOperation, bool, error) {
	id, err := parseUUID(occurrenceID)
	if err != nil {
		return postgres.CoreQueueOperation{}, false, err
	}
	var operation postgres.CoreQueueOperation
	replayed := false
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		occurrence, current, err := lockOccurrenceAndOperation(ctx, tx, id)
		if err != nil {
			return err
		}
		if occurrence.State == "checkpointed" {
			operation, replayed = current, true
			return nil
		}
		if occurrence.State != "runner_terminal" {
			return ErrOperationConflict
		}
		cycles := occurrence.CyclesConsumed.Int64
		tokens := occurrence.RemoteTokensConsumed.Int64
		cost := occurrence.CostMicrousdConsumed.Int64
		if current.TasksStarted+1 > current.MaxTasks || current.CyclesConsumed+cycles > current.MaxTotalCycles ||
			current.RemoteTokensConsumed+tokens > current.MaxRemoteTokens || current.CostMicrousdConsumed+cost > current.MaxCostMicrousd {
			return ErrOperationConflict
		}
		if _, err := tx.Exec(ctx, `UPDATE core.queue_task_occurrences SET state='checkpointed',
			lease_reconciled=true, checkpointed_at=$2, updated_at=$2 WHERE id=$1`, id, now); err != nil {
			return err
		}
		current.TasksStarted++
		current.CyclesConsumed += cycles
		current.RemoteTokensConsumed += tokens
		current.CostMicrousdConsumed += cost
		current.ActiveOccurrenceID = pgtype.UUID{}
		current.AggregateVersion++
		current.UpdatedAt = timestamp(now)
		if _, err := tx.Exec(ctx, `UPDATE core.queue_operations SET tasks_started=$2,
			cycles_consumed=$3, remote_tokens_consumed=$4, cost_microusd_consumed=$5,
			active_occurrence_id=NULL, aggregate_version=$6, updated_at=$7 WHERE id=$1`,
			current.ID, current.TasksStarted, current.CyclesConsumed,
			current.RemoteTokensConsumed, current.CostMicrousdConsumed,
			current.AggregateVersion, now); err != nil {
			return err
		}
		if err := appendQueueEvent(ctx, postgres.New(tx), current, "queue.task.checkpointed", map[string]any{
			"operation_id": uuidString(current.ID), "occurrence_id": occurrenceID,
			"occurrence_sequence": occurrence.OccurrenceSequence,
			"outcome":             occurrence.Outcome.String, "result_sha256": occurrence.ResultSha256.String,
			"tasks_started": current.TasksStarted, "cycles_consumed": current.CyclesConsumed,
			"remote_tokens_consumed": current.RemoteTokensConsumed,
			"cost_microusd_consumed": current.CostMicrousdConsumed,
		}); err != nil {
			return err
		}
		operation = current
		return nil
	})
	return operation, replayed, err
}

func (s *Store) terminate(ctx context.Context, operationID string, reason StopReason, detail string, now time.Time) (postgres.CoreQueueOperation, bool, error) {
	if !reason.valid() || len(detail) > 8192 {
		return postgres.CoreQueueOperation{}, false, errors.New("sequential queue: invalid terminal result")
	}
	id, err := parseUUID(operationID)
	if err != nil {
		return postgres.CoreQueueOperation{}, false, err
	}
	var operation postgres.CoreQueueOperation
	replayed := false
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT `+operationColumns+` FROM core.queue_operations WHERE id=$1 FOR UPDATE`, id).Scan(operationDestinations(&operation)...); err != nil {
			return err
		}
		if operation.Status == "terminal" {
			if operation.StopReason.String != string(reason) || operation.StopDetail.String != detail {
				return ErrOperationConflict
			}
			replayed = true
			return nil
		}
		if operation.ActiveOccurrenceID.Valid {
			return ErrOperationConflict
		}
		occurrences, err := listOccurrencesTx(ctx, tx, id)
		if err != nil {
			return err
		}
		marker := terminalMarker{
			SchemaVersion: OperationSchemaVersion, OperationID: operationID,
			ConfigSHA256: operation.ConfigSha256, StopReason: reason, StopDetail: detail,
			TasksStarted: operation.TasksStarted, CyclesConsumed: operation.CyclesConsumed,
			RemoteTokensConsumed:      operation.RemoteTokensConsumed,
			CostMicrousdConsumed:      operation.CostMicrousdConsumed,
			PeakSourceMutatingWorkers: int(operation.PeakSourceMutatingWorkers),
			TerminalAt:                now.UTC(), Occurrences: make([]terminalOccurrence, 0, len(occurrences)),
		}
		for _, occurrence := range occurrences {
			if occurrence.State != "checkpointed" {
				return ErrOperationConflict
			}
			marker.Occurrences = append(marker.Occurrences, terminalOccurrence{
				ID: uuidString(occurrence.ID), Sequence: occurrence.OccurrenceSequence,
				TaskID: uuidString(occurrence.TaskID), ResultSHA256: occurrence.ResultSha256.String,
			})
		}
		_, terminalSHA, err := canonicalJSON(marker)
		if err != nil {
			return err
		}
		operation.Status = "terminal"
		operation.StopReason, operation.StopDetail = text(string(reason)), text(detail)
		operation.TerminalMarkerSha256 = text(terminalSHA)
		operation.TerminalAt, operation.UpdatedAt = timestamp(now), timestamp(now)
		operation.AggregateVersion++
		if _, err := tx.Exec(ctx, `UPDATE core.queue_operations SET status='terminal',
			stop_reason=$2, stop_detail=$3, terminal_marker_sha256=$4, terminal_at=$5,
			updated_at=$5, aggregate_version=$6 WHERE id=$1`, id, string(reason), detail,
			terminalSHA, now, operation.AggregateVersion); err != nil {
			return err
		}
		return appendQueueEvent(ctx, postgres.New(tx), operation, "queue.operation.terminal", map[string]any{
			"operation_id": operationID, "stop_reason": reason, "stop_detail": detail,
			"terminal_marker_sha256": terminalSHA,
		})
	})
	return operation, replayed, err
}

func (s *Store) requestCancel(ctx context.Context, operationID string, now time.Time) (postgres.CoreQueueOperation, bool, error) {
	id, err := parseUUID(operationID)
	if err != nil {
		return postgres.CoreQueueOperation{}, false, err
	}
	var operation postgres.CoreQueueOperation
	replayed := false
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT `+operationColumns+` FROM core.queue_operations WHERE id=$1 FOR UPDATE`, id).Scan(operationDestinations(&operation)...); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if operation.Status == "terminal" || operation.CancelRequestedAt.Valid {
			replayed = true
			return nil
		}
		operation.CancelRequestedAt = timestamp(now)
		operation.UpdatedAt = timestamp(now)
		operation.AggregateVersion++
		if _, err := tx.Exec(ctx, `UPDATE core.queue_operations SET cancel_requested_at=$2,
			updated_at=$2, aggregate_version=$3 WHERE id=$1`, id, now, operation.AggregateVersion); err != nil {
			return err
		}
		return appendQueueEvent(ctx, postgres.New(tx), operation, "queue.operation.cancel_requested", map[string]any{
			"operation_id": operationID, "requested_at": now.UTC(),
		})
	})
	return operation, replayed, err
}

func (s *Store) cancellationRequested(ctx context.Context, operationID string) (bool, error) {
	operation, err := s.getOperation(ctx, operationID)
	return operation.CancelRequestedAt.Valid, err
}

func (s *Store) status(ctx context.Context, operationID string) (Status, error) {
	operation, err := s.getOperation(ctx, operationID)
	if err != nil {
		return Status{}, err
	}
	var configuration PinnedConfiguration
	if err := decodeStrict(operation.Configuration, &configuration); err != nil {
		return Status{}, ErrOperationConflict
	}
	configuration.Limits, err = configuration.Limits.normalized()
	if err != nil {
		return Status{}, ErrOperationConflict
	}
	_, storedConfigSHA, err := canonicalJSON(configuration)
	if err != nil || storedConfigSHA != operation.ConfigSha256 {
		return Status{}, ErrOperationConflict
	}
	occurrences, err := postgres.New(s.pool).ListQueueTaskOccurrences(ctx, operation.ID)
	if err != nil {
		return Status{}, err
	}
	status := Status{
		OperationID: operationID, Status: operation.Status, Configuration: configuration,
		ConfigSHA256: operation.ConfigSha256, StartedAt: operation.StartedAt.Time,
		DeadlineAt: operation.DeadlineAt.Time, UpdatedAt: operation.UpdatedAt.Time,
		StopReason: StopReason(operation.StopReason.String), StopDetail: operation.StopDetail.String,
		TerminalMarkerSHA256: operation.TerminalMarkerSha256.String,
		TasksStarted:         operation.TasksStarted, CyclesConsumed: operation.CyclesConsumed,
		RemoteTokensConsumed:      operation.RemoteTokensConsumed,
		CostMicrousdConsumed:      operation.CostMicrousdConsumed,
		PeakSourceMutatingWorkers: int(operation.PeakSourceMutatingWorkers),
		Outcomes:                  make([]Outcome, 0, len(occurrences)),
	}
	if operation.TerminalAt.Valid {
		terminal := operation.TerminalAt.Time
		status.TerminalAt = &terminal
	}
	if operation.CancelRequestedAt.Valid {
		requested := operation.CancelRequestedAt.Time
		status.CancelRequestedAt = &requested
	}
	for _, occurrence := range occurrences {
		if occurrence.State != "checkpointed" {
			continue
		}
		var result TaskResult
		if err := decodeStrict(occurrence.Result, &result); err != nil {
			return Status{}, ErrOperationConflict
		}
		_, resultSHA, err := canonicalJSON(result)
		if err != nil || resultSHA != occurrence.ResultSha256.String ||
			result.Outcome != TaskOutcome(occurrence.Outcome.String) ||
			result.CyclesConsumed != occurrence.CyclesConsumed.Int64 ||
			result.RemoteTokensConsumed != occurrence.RemoteTokensConsumed.Int64 ||
			result.CostMicrousdConsumed != occurrence.CostMicrousdConsumed.Int64 ||
			result.Reconciliation.Workspace != occurrence.WorkspaceReconciled.Bool ||
			result.Reconciliation.Evidence != occurrence.EvidenceReconciled.Bool {
			return Status{}, ErrOperationConflict
		}
		effects, err := postgres.New(s.pool).ListQueueTaskEffects(ctx, occurrence.ID)
		effectChainSHA, chainErr := effectChainHash(effects)
		if err != nil || chainErr != nil || effectChainSHA != occurrence.EffectChainSha256.String ||
			validateEffectHistory(effects, result.Outcome) != nil {
			return Status{}, ErrOperationConflict
		}
		status.Outcomes = append(status.Outcomes, outcomeFromOccurrence(occurrence))
	}
	if err := s.validateEventChain(ctx, operation); err != nil {
		return Status{}, err
	}
	if operation.Status == "terminal" {
		if !operation.TerminalAt.Valid || len(status.Outcomes) != int(operation.TasksStarted) {
			return Status{}, ErrOperationConflict
		}
		marker := terminalMarker{
			SchemaVersion: OperationSchemaVersion, OperationID: operationID,
			ConfigSHA256: operation.ConfigSha256, StopReason: StopReason(operation.StopReason.String),
			StopDetail: operation.StopDetail.String, TasksStarted: operation.TasksStarted,
			CyclesConsumed:            operation.CyclesConsumed,
			RemoteTokensConsumed:      operation.RemoteTokensConsumed,
			CostMicrousdConsumed:      operation.CostMicrousdConsumed,
			PeakSourceMutatingWorkers: int(operation.PeakSourceMutatingWorkers),
			TerminalAt:                operation.TerminalAt.Time.UTC(),
			Occurrences:               make([]terminalOccurrence, 0, len(occurrences)),
		}
		for _, occurrence := range occurrences {
			if occurrence.State != "checkpointed" {
				return Status{}, ErrOperationConflict
			}
			marker.Occurrences = append(marker.Occurrences, terminalOccurrence{
				ID: uuidString(occurrence.ID), Sequence: occurrence.OccurrenceSequence,
				TaskID: uuidString(occurrence.TaskID), ResultSHA256: occurrence.ResultSha256.String,
			})
		}
		_, markerSHA, err := canonicalJSON(marker)
		if err != nil || markerSHA != operation.TerminalMarkerSha256.String {
			return Status{}, ErrOperationConflict
		}
	}
	return status, nil
}

func (s *Store) validateEventChain(ctx context.Context, operation postgres.CoreQueueOperation) error {
	rows, err := s.pool.Query(ctx, `SELECT aggregate_version,event_type FROM core.events
		WHERE aggregate_type='queue_operation' AND aggregate_id=$1 ORDER BY aggregate_version`, operation.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	var version int64
	lastType := ""
	for rows.Next() {
		var current int64
		if err := rows.Scan(&current, &lastType); err != nil {
			return err
		}
		version++
		if current != version {
			return ErrOperationConflict
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if version != operation.AggregateVersion || operation.Status == "terminal" && lastType != "queue.operation.terminal" {
		return ErrOperationConflict
	}
	return nil
}

func lockOccurrenceAndOperation(ctx context.Context, tx pgx.Tx, occurrenceID pgtype.UUID) (postgres.CoreQueueTaskOccurrence, postgres.CoreQueueOperation, error) {
	var occurrence postgres.CoreQueueTaskOccurrence
	if err := tx.QueryRow(ctx, `SELECT `+occurrenceColumns+` FROM core.queue_task_occurrences WHERE id=$1 FOR UPDATE`, occurrenceID).Scan(occurrenceDestinations(&occurrence)...); err != nil {
		return occurrence, postgres.CoreQueueOperation{}, err
	}
	var operation postgres.CoreQueueOperation
	if err := tx.QueryRow(ctx, `SELECT `+operationColumns+` FROM core.queue_operations WHERE id=$1 FOR UPDATE`, occurrence.QueueOperationID).Scan(operationDestinations(&operation)...); err != nil {
		return occurrence, operation, err
	}
	if operation.Status != "active" || operation.ActiveOccurrenceID != occurrence.ID {
		if occurrence.State == "checkpointed" && !operation.ActiveOccurrenceID.Valid {
			return occurrence, operation, nil
		}
		return occurrence, operation, ErrOperationConflict
	}
	return occurrence, operation, nil
}

func advanceOperation(ctx context.Context, tx pgx.Tx, operationID pgtype.UUID, now time.Time) (postgres.CoreQueueOperation, error) {
	var operation postgres.CoreQueueOperation
	if err := tx.QueryRow(ctx, `UPDATE core.queue_operations SET aggregate_version=aggregate_version+1,
		updated_at=$2 WHERE id=$1 AND status='active' RETURNING `+operationColumns, operationID, now).Scan(operationDestinations(&operation)...); err != nil {
		return operation, err
	}
	return operation, nil
}

func appendQueueEvent(ctx context.Context, queries *postgres.Queries, operation postgres.CoreQueueOperation, eventType string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = queries.AppendEvent(ctx, postgres.AppendEventParams{
		ID: pgtype.UUID{Bytes: uuid.Must(uuid.NewV7()), Valid: true}, EventType: eventType,
		AggregateType: "queue_operation", AggregateID: operation.ID,
		AggregateVersion: operation.AggregateVersion, Payload: raw, CreatedAt: operation.UpdatedAt,
	})
	return err
}

func listOccurrencesTx(ctx context.Context, tx pgx.Tx, operationID pgtype.UUID) ([]postgres.CoreQueueTaskOccurrence, error) {
	rows, err := tx.Query(ctx, `SELECT `+occurrenceColumns+` FROM core.queue_task_occurrences
		WHERE queue_operation_id=$1 ORDER BY occurrence_sequence`, operationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var occurrences []postgres.CoreQueueTaskOccurrence
	for rows.Next() {
		var occurrence postgres.CoreQueueTaskOccurrence
		if err := rows.Scan(occurrenceDestinations(&occurrence)...); err != nil {
			return nil, err
		}
		occurrences = append(occurrences, occurrence)
	}
	return occurrences, rows.Err()
}

func outcomeFromOccurrence(occurrence postgres.CoreQueueTaskOccurrence) Outcome {
	return Outcome{
		OccurrenceID: uuidString(occurrence.ID), OccurrenceSequence: occurrence.OccurrenceSequence,
		TaskID: uuidString(occurrence.TaskID), ExternalTaskID: occurrence.ExternalTaskID.String,
		SchedulerRunID: uuidString(occurrence.SchedulerRunID), Outcome: TaskOutcome(occurrence.Outcome.String),
		Detail: occurrence.OutcomeDetail.String, CyclesConsumed: occurrence.CyclesConsumed.Int64,
		RemoteTokensConsumed: occurrence.RemoteTokensConsumed.Int64,
		CostMicrousdConsumed: occurrence.CostMicrousdConsumed.Int64,
		Reconciliation:       Reconciliation{Workspace: occurrence.WorkspaceReconciled.Bool, Evidence: occurrence.EvidenceReconciled.Bool},
		LeaseReconciled:      occurrence.LeaseReconciled, ResultSHA256: occurrence.ResultSha256.String,
	}
}

type terminalMarker struct {
	SchemaVersion             string               `json:"schema_version"`
	OperationID               string               `json:"operation_id"`
	ConfigSHA256              string               `json:"config_sha256"`
	StopReason                StopReason           `json:"stop_reason"`
	StopDetail                string               `json:"stop_detail"`
	TasksStarted              int64                `json:"tasks_started"`
	CyclesConsumed            int64                `json:"cycles_consumed"`
	RemoteTokensConsumed      int64                `json:"remote_tokens_consumed"`
	CostMicrousdConsumed      int64                `json:"cost_microusd_consumed"`
	PeakSourceMutatingWorkers int                  `json:"peak_source_mutating_workers"`
	TerminalAt                time.Time            `json:"terminal_at"`
	Occurrences               []terminalOccurrence `json:"occurrences"`
}

type terminalOccurrence struct {
	ID           string `json:"id"`
	Sequence     int64  `json:"sequence"`
	TaskID       string `json:"task_id"`
	ResultSHA256 string `json:"result_sha256"`
}

type effectChainItem struct {
	Sequence       int64      `json:"sequence"`
	Kind           EffectKind `json:"kind"`
	Identity       string     `json:"identity"`
	MaterialSHA256 string     `json:"material_sha256"`
	EvidenceSHA256 string     `json:"evidence_sha256"`
}

func effectChainHash(effects []postgres.CoreQueueTaskEffect) (string, error) {
	items := make([]effectChainItem, 0, len(effects))
	for i, effect := range effects {
		if effect.EffectSequence != int64(i+1) || effect.Status != "completed" ||
			!EffectKind(effect.EffectKind).valid() || !effect.EvidenceSha256.Valid {
			return "", ErrOperationConflict
		}
		items = append(items, effectChainItem{
			Sequence: effect.EffectSequence, Kind: EffectKind(effect.EffectKind),
			Identity: effect.EffectIdentity, MaterialSHA256: effect.MaterialSha256,
			EvidenceSHA256: effect.EvidenceSha256.String,
		})
	}
	_, digest, err := canonicalJSON(items)
	return digest, err
}

const operationColumns = `id,schema_version,status,worker_mode,maximum_workers,
	quality_gate_status,config_schema,config_sha256,configuration,max_tasks,
	max_cycles_per_task,max_total_cycles,max_remote_tokens,max_cost_microusd,
	max_duration_milliseconds,tasks_started,cycles_consumed,remote_tokens_consumed,
	cost_microusd_consumed,peak_source_mutating_workers,next_occurrence_sequence,
	selection_intent_id,selection_scheduler_run_id,selection_intent_sequence,
	active_occurrence_id,cancel_requested_at,started_at,deadline_at,updated_at,
	terminal_at,stop_reason,stop_detail,terminal_marker_sha256,aggregate_version`

func operationDestinations(o *postgres.CoreQueueOperation) []any {
	return []any{&o.ID, &o.SchemaVersion, &o.Status, &o.WorkerMode, &o.MaximumWorkers,
		&o.QualityGateStatus, &o.ConfigSchema, &o.ConfigSha256, &o.Configuration,
		&o.MaxTasks, &o.MaxCyclesPerTask, &o.MaxTotalCycles, &o.MaxRemoteTokens,
		&o.MaxCostMicrousd, &o.MaxDurationMilliseconds, &o.TasksStarted,
		&o.CyclesConsumed, &o.RemoteTokensConsumed, &o.CostMicrousdConsumed,
		&o.PeakSourceMutatingWorkers, &o.NextOccurrenceSequence, &o.SelectionIntentID,
		&o.SelectionSchedulerRunID, &o.SelectionIntentSequence, &o.ActiveOccurrenceID,
		&o.CancelRequestedAt, &o.StartedAt, &o.DeadlineAt, &o.UpdatedAt,
		&o.TerminalAt, &o.StopReason, &o.StopDetail, &o.TerminalMarkerSha256,
		&o.AggregateVersion}
}

const occurrenceColumns = `id,queue_operation_id,occurrence_sequence,state,
	scheduler_run_id,coordinator_identity,project_id,project_source_id,task_id,
	task_version_id,external_task_id,expected_task_aggregate_version,task_priority,
	task_created_at,source_commit,source_tree,selection,selection_sha256,outcome,
	outcome_detail,result,result_sha256,effect_chain_sha256,cycles_consumed,remote_tokens_consumed,
	cost_microusd_consumed,workspace_reconciled,evidence_reconciled,
	lease_reconciled,selected_at,admitted_at,runner_terminal_at,checkpointed_at,
	created_at,updated_at`

func occurrenceDestinations(o *postgres.CoreQueueTaskOccurrence) []any {
	return []any{&o.ID, &o.QueueOperationID, &o.OccurrenceSequence, &o.State,
		&o.SchedulerRunID, &o.CoordinatorIdentity, &o.ProjectID, &o.ProjectSourceID,
		&o.TaskID, &o.TaskVersionID, &o.ExternalTaskID,
		&o.ExpectedTaskAggregateVersion, &o.TaskPriority, &o.TaskCreatedAt,
		&o.SourceCommit, &o.SourceTree, &o.Selection, &o.SelectionSha256,
		&o.Outcome, &o.OutcomeDetail, &o.Result, &o.ResultSha256, &o.EffectChainSha256,
		&o.CyclesConsumed, &o.RemoteTokensConsumed, &o.CostMicrousdConsumed,
		&o.WorkspaceReconciled, &o.EvidenceReconciled, &o.LeaseReconciled,
		&o.SelectedAt, &o.AdmittedAt, &o.RunnerTerminalAt, &o.CheckpointedAt,
		&o.CreatedAt, &o.UpdatedAt}
}

func parseUUID(value string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("sequential queue: invalid UUID %q: %w", value, err)
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}

func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC().Truncate(time.Microsecond), Valid: true}
}

func text(value string) pgtype.Text     { return pgtype.Text{String: value, Valid: true} }
func int8Value(value int64) pgtype.Int8 { return pgtype.Int8{Int64: value, Valid: true} }
func int4Value(value int32) pgtype.Int4 { return pgtype.Int4{Int32: value, Valid: true} }
func boolValue(value bool) pgtype.Bool  { return pgtype.Bool{Bool: value, Valid: true} }

func uniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
