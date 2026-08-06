package planner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"revolvr/internal/id"
	"revolvr/internal/model"
	"revolvr/internal/storage/postgres"
)

var (
	ErrConflict       = errors.New("plan optimistic concurrency conflict")
	ErrStaleAuthority = errors.New("plan task, run, or source authority is stale")
)

type PersistenceDisposition string

const (
	PersistenceCreated  PersistenceDisposition = "created"
	PersistenceAccepted PersistenceDisposition = "accepted"
	PersistenceReplayed PersistenceDisposition = "replayed"
)

type PersistenceResult struct {
	Disposition          PersistenceDisposition
	PlanAggregateVersion int64
	EventIDs             []string
}

type AcceptanceAuthority string

const TrustedHostAcceptance AcceptanceAuthority = "trusted_host"

type AcceptanceCommand struct {
	OperationID string
	AcceptedBy  string
	Authority   AcceptanceAuthority
	AcceptedAt  time.Time
}

func PersistCandidate(ctx context.Context, pool *pgxpool.Pool, candidate Candidate) (PersistenceResult, error) {
	if pool == nil {
		return PersistenceResult{}, errors.New("planner persistence requires PostgreSQL")
	}
	var result PersistenceResult
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		q := postgres.New(tx)
		plan, created, eventID, replayed, err := persistCandidate(ctx, q, candidate, time.Now().UTC().Truncate(time.Microsecond))
		if err != nil {
			return err
		}
		result.PlanAggregateVersion = plan.AggregateVersion
		if eventID != "" {
			result.EventIDs = []string{eventID}
		}
		if replayed {
			result.Disposition = PersistenceReplayed
		} else if created {
			result.Disposition = PersistenceCreated
		} else {
			result.Disposition = PersistenceCreated
		}
		return nil
	})
	if err != nil {
		return PersistenceResult{}, fmt.Errorf("persist planner candidate: %w", err)
	}
	return result, nil
}

func Accept(ctx context.Context, pool *pgxpool.Pool, candidate Candidate, command AcceptanceCommand) (PersistenceResult, error) {
	if pool == nil {
		return PersistenceResult{}, errors.New("planner acceptance requires PostgreSQL")
	}
	command.OperationID = strings.TrimSpace(command.OperationID)
	command.AcceptedBy = strings.TrimSpace(command.AcceptedBy)
	if command.Authority != TrustedHostAcceptance || !token(command.OperationID) || command.AcceptedBy == "" {
		return PersistenceResult{}, errors.New("planner acceptance requires explicit trusted host authority, operation, and actor")
	}
	if command.AcceptedAt.IsZero() {
		return PersistenceResult{}, errors.New("planner acceptance time is required")
	}
	command.AcceptedAt = command.AcceptedAt.UTC().Truncate(time.Microsecond)
	var result PersistenceResult
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		q := postgres.New(tx)
		plan, _, candidateEvent, replayed, err := persistCandidate(ctx, q, candidate, command.AcceptedAt)
		if err != nil {
			return err
		}
		if plan.AcceptedVersionID.Valid && uuidString(plan.AcceptedVersionID) == candidate.PlanVersionID && plan.AcceptedOperationID.Valid && plan.AcceptedOperationID.String == command.OperationID {
			result = PersistenceResult{Disposition: PersistenceReplayed, PlanAggregateVersion: plan.AggregateVersion}
			return nil
		}
		if replayed && plan.AcceptedVersionID.Valid { // another operation already accepted this or a competing revision
			return ErrConflict
		}
		if prior := candidate.Output.Revision.SupersedesPlanVersionID; prior == nil {
			if plan.AcceptedVersionID.Valid {
				return ErrConflict
			}
		} else if !plan.AcceptedVersionID.Valid || uuidString(plan.AcceptedVersionID) != *prior {
			return fmt.Errorf("%w: revision does not supersede the accepted plan version", ErrConflict)
		}
		planID, versionID, err := twoUUIDs(candidate.PlanID, candidate.PlanVersionID)
		if err != nil {
			return err
		}
		updated, err := q.AcceptPlanVersion(ctx, postgres.AcceptPlanVersionParams{PlanVersionID: versionID, OperationID: pgtype.Text{String: command.OperationID, Valid: true}, AcceptedBy: pgtype.Text{String: command.AcceptedBy, Valid: true}, AcceptedAt: timestamp(command.AcceptedAt), PlanID: planID, ExpectedAggregateVersion: plan.AggregateVersion})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrConflict
		}
		if err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{"schema_version": "revolvr-plan-accepted-event-v1", "plan_id": candidate.PlanID, "plan_version_id": candidate.PlanVersionID, "candidate_sha256": candidate.CandidateSHA256, "operation_id": command.OperationID, "accepted_by": command.AcceptedBy, "authority": string(command.Authority), "expected_aggregate_version": plan.AggregateVersion, "aggregate_version": updated.AggregateVersion})
		acceptedEvent, err := appendPlanEvent(ctx, q, candidate, "plan.accepted", updated.AggregateVersion, payload, command.AcceptedAt)
		if err != nil {
			return err
		}
		result = PersistenceResult{Disposition: PersistenceAccepted, PlanAggregateVersion: updated.AggregateVersion, EventIDs: []string{candidateEvent, acceptedEvent}}
		if candidateEvent == "" {
			result.EventIDs = []string{acceptedEvent}
		}
		return nil
	})
	if err != nil {
		return PersistenceResult{}, fmt.Errorf("accept planner candidate: %w", err)
	}
	return result, nil
}

func persistCandidate(ctx context.Context, q *postgres.Queries, candidate Candidate, now time.Time) (postgres.CorePlan, bool, string, bool, error) {
	if err := validateCandidateEnvelope(candidate); err != nil {
		return postgres.CorePlan{}, false, "", false, err
	}
	ids, err := candidateUUIDs(candidate)
	if err != nil {
		return postgres.CorePlan{}, false, "", false, err
	}
	authority, err := q.GetPlannerRunAuthority(ctx, ids.run)
	if errors.Is(err, pgx.ErrNoRows) {
		return postgres.CorePlan{}, false, "", false, ErrStaleAuthority
	}
	if err != nil {
		return postgres.CorePlan{}, false, "", false, err
	}
	if uuidString(authority.ProjectID) != candidate.ProjectID || uuidString(authority.TaskID) != candidate.TaskID || uuidString(authority.TaskVersionID) != candidate.TaskVersionID || uuidString(authority.ProjectSourceID) != candidate.ProjectSourceID || authority.RunStatus != "active" || (authority.TaskStatus != "admitted" && authority.TaskStatus != "planning") || !authority.AcceptedVersionID.Valid || uuidString(authority.AcceptedVersionID) != candidate.TaskVersionID || authority.SourceCommit != candidate.SourceCommit || authority.SourceTree != candidate.SourceTree || authority.CurrentCommit != candidate.SourceCommit || authority.CurrentTree != candidate.SourceTree {
		return postgres.CorePlan{}, false, "", false, ErrStaleAuthority
	}

	plan, err := q.GetPlanForUpdate(ctx, ids.plan)
	created := false
	if errors.Is(err, pgx.ErrNoRows) {
		if candidate.ExpectedPlanAggregateVersion != 0 || candidate.Output.Revision.RevisionNumber != 1 {
			return postgres.CorePlan{}, false, "", false, ErrConflict
		}
		plan, err = q.InsertPlan(ctx, postgres.InsertPlanParams{ID: ids.plan, ProjectID: ids.project, TaskID: ids.task, TaskVersionID: ids.taskVersion, RunID: ids.run, ProjectSourceID: ids.source, SourceRevision: candidate.SourceRevision, SourceCommit: candidate.SourceCommit, SourceTree: candidate.SourceTree, CreatedAt: timestamp(now)})
		created = true
	}
	if err != nil {
		return postgres.CorePlan{}, false, "", false, err
	}
	if uuidString(plan.TaskID) != candidate.TaskID || uuidString(plan.TaskVersionID) != candidate.TaskVersionID || uuidString(plan.RunID) != candidate.RunID || uuidString(plan.ProjectSourceID) != candidate.ProjectSourceID || plan.SourceRevision != candidate.SourceRevision || plan.SourceCommit != candidate.SourceCommit || plan.SourceTree != candidate.SourceTree {
		return postgres.CorePlan{}, false, "", false, ErrStaleAuthority
	}

	existing, err := q.GetPlanVersion(ctx, ids.planVersion)
	if err == nil {
		if existing.PlanID != ids.plan || existing.CandidateSha256 != candidate.CandidateSHA256 || existing.ContentSha256 != *candidate.Output.Revision.ContentSHA256 {
			return postgres.CorePlan{}, false, "", false, ErrConflict
		}
		if plan.AggregateVersion != candidate.ExpectedPlanAggregateVersion+1 && !(plan.AcceptedVersionID.Valid && uuidString(plan.AcceptedVersionID) == candidate.PlanVersionID) {
			return postgres.CorePlan{}, false, "", false, ErrConflict
		}
		return plan, created, "", true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return postgres.CorePlan{}, false, "", false, err
	}
	if plan.AggregateVersion != candidate.ExpectedPlanAggregateVersion {
		return postgres.CorePlan{}, false, "", false, ErrConflict
	}
	if err := insertVersion(ctx, q, candidate, ids, now); err != nil {
		return postgres.CorePlan{}, false, "", false, err
	}
	updated, err := q.AdvancePlanCandidate(ctx, postgres.AdvancePlanCandidateParams{UpdatedAt: timestamp(now), PlanID: ids.plan, ExpectedAggregateVersion: plan.AggregateVersion})
	if errors.Is(err, pgx.ErrNoRows) {
		return postgres.CorePlan{}, false, "", false, ErrConflict
	}
	if err != nil {
		return postgres.CorePlan{}, false, "", false, err
	}
	payload, _ := json.Marshal(map[string]any{"schema_version": "revolvr-plan-candidate-event-v1", "plan_id": candidate.PlanID, "plan_version_id": candidate.PlanVersionID, "revision_number": candidate.Output.Revision.RevisionNumber, "candidate_sha256": candidate.CandidateSHA256, "content_sha256": *candidate.Output.Revision.ContentSHA256, "supervisor_decision_id": candidate.SupervisorDecisionID, "supervisor_decision_sha256": candidate.SupervisorDecisionSHA256, "dossier_sha256": candidate.Dossier.SHA256, "expected_aggregate_version": plan.AggregateVersion, "aggregate_version": updated.AggregateVersion})
	eventID, err := appendPlanEvent(ctx, q, candidate, "plan.candidate_recorded", updated.AggregateVersion, payload, now)
	if err != nil {
		return postgres.CorePlan{}, false, "", false, err
	}
	return updated, created, eventID, false, nil
}

type parsedIDs struct{ plan, planVersion, project, task, taskVersion, run, source pgtype.UUID }

func candidateUUIDs(c Candidate) (parsedIDs, error) {
	values := []string{c.PlanID, c.PlanVersionID, c.ProjectID, c.TaskID, c.TaskVersionID, c.RunID, c.ProjectSourceID}
	out := make([]pgtype.UUID, len(values))
	for i, value := range values {
		parsed, err := uuid.Parse(value)
		if err != nil {
			return parsedIDs{}, fmt.Errorf("planner persistence identity %q is not UUID: %w", value, err)
		}
		out[i] = pgtype.UUID{Bytes: parsed, Valid: true}
	}
	return parsedIDs{out[0], out[1], out[2], out[3], out[4], out[5], out[6]}, nil
}
func twoUUIDs(a, b string) (pgtype.UUID, pgtype.UUID, error) {
	x, err := uuid.Parse(a)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, err
	}
	y, err := uuid.Parse(b)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: x, Valid: true}, pgtype.UUID{Bytes: y, Valid: true}, nil
}

func insertVersion(ctx context.Context, q *postgres.Queries, c Candidate, ids parsedIDs, now time.Time) error {
	modelPolicy, _ := json.Marshal(c.ModelPolicy)
	hostPolicy, _ := json.Marshal(c.HostPolicy)
	request, _ := json.Marshal(c.ExpectedRequest)
	modelResult, _ := json.Marshal(c.ModelResult)
	var supersedes pgtype.UUID
	if c.Output.Revision.SupersedesPlanVersionID != nil {
		parsed, err := uuid.Parse(*c.Output.Revision.SupersedesPlanVersionID)
		if err != nil {
			return err
		}
		supersedes = pgtype.UUID{Bytes: parsed, Valid: true}
	}
	_, err := q.InsertPlanVersion(ctx, postgres.InsertPlanVersionParams{ID: ids.planVersion, PlanID: ids.plan, TaskID: ids.task, TaskVersionID: ids.taskVersion, RunID: ids.run, ProjectSourceID: ids.source, RevisionNumber: int32(c.Output.Revision.RevisionNumber), SupersedesVersionID: supersedes, CandidateSha256: c.CandidateSHA256, ContentSha256: *c.Output.Revision.ContentSHA256, ChangeExplanation: c.Output.ChangeExplanation, SourceRevision: c.SourceRevision, SupervisorDecisionID: c.SupervisorDecisionID, SupervisorDecisionSha256: c.SupervisorDecisionSHA256, DossierVersion: c.Dossier.Version, DossierSha256: c.Dossier.SHA256, DossierContent: c.Dossier.Content, PromptVersion: c.Prompt.Version, PromptSha256: c.Prompt.SHA256, PromptContent: []byte(c.Prompt.Content), ResponseSchemaVersion: c.ResponseSchema.Version, ResponseSchemaSha256: c.ResponseSchema.SHA256, ResponseSchema: c.ResponseSchema.Content, ModelPolicyVersion: c.ModelPolicy.Version, ModelPolicySha256: c.ModelPolicy.SHA256, ModelPolicy: modelPolicy, HostPolicyVersion: c.HostPolicy.Version, HostPolicySha256: c.HostPolicy.SHA256, HostPolicy: hostPolicy, ExpectedRequest: request, ModelResult: modelResult, RawOutput: c.RawOutput, CanonicalOutput: c.CanonicalOutput, CreatedAt: timestamp(now)})
	if err != nil {
		return err
	}
	for _, step := range c.Output.Steps {
		criterion, _ := json.Marshal(step.CriterionIDs)
		depends, _ := json.Marshal(step.DependsOnStepIDs)
		paths, _ := json.Marshal(step.ExpectedPaths)
		components, _ := json.Marshal(step.Components)
		tests, _ := json.Marshal(step.TestStrategy)
		risks, _ := json.Marshal(step.Risks)
		assumptions, _ := json.Marshal(step.Assumptions)
		evidence, _ := json.Marshal(step.EvidenceRefs)
		var lineage []byte
		if step.Lineage != nil {
			lineage, _ = json.Marshal(step.Lineage)
		}
		if _, err := q.InsertPlanStep(ctx, postgres.InsertPlanStepParams{PlanVersionID: ids.planVersion, PlanID: ids.plan, StepID: step.ID, Ordinal: int32(step.Ordinal), Status: step.Status, Description: step.Description, CriterionIds: criterion, DependsOnStepIds: depends, ExpectedPaths: paths, Components: components, TestStrategy: tests, Risks: risks, Assumptions: assumptions, EvidenceRefs: evidence, Lineage: lineage}); err != nil {
			return err
		}
	}
	return nil
}

func validateCandidateEnvelope(c Candidate) error {
	if c.SchemaVersion != CandidateVersion || c.CandidateSHA256 == "" || c.CandidateSHA256 != candidateHash(c) {
		return errors.New("planner candidate envelope hash or version is invalid")
	}
	if c.PlanID != c.Output.Revision.PlanID || c.PlanVersionID != c.Output.Revision.PlanVersionID || c.TaskID != c.Output.Revision.TaskID || c.TaskVersionID != c.Output.Revision.TaskVersionID || c.RunID != c.Output.Revision.RunID || c.ProjectSourceID != c.Output.Revision.ProjectSourceID || c.SourceRevision != c.Output.Revision.SourceRevision || c.SupervisorDecisionID != c.Output.Revision.SupervisorDecisionID || c.SupervisorDecisionSHA256 != c.Output.Revision.SupervisorDecisionSHA256 {
		return errors.New("planner candidate identities are inconsistent")
	}
	if c.Output.Revision.ContentSHA256 == nil || *c.Output.Revision.ContentSHA256 != contentSHA256(c.Output) || c.Dossier.SHA256 != c.Output.Revision.DossierSHA256 || c.Prompt.SHA256 != c.Output.Revision.PromptSHA256 || c.ResponseSchema.SHA256 != c.Output.Revision.ResponseSchemaSHA256 || c.ModelPolicy.SHA256 != c.Output.Revision.ModelPolicySHA256 || c.HostPolicy.SHA256 != c.Output.Revision.HostPolicySHA256 {
		return errors.New("planner candidate provenance identities are inconsistent")
	}
	if c.Dossier.Version != DossierVersion || c.Dossier.ByteSize != len(c.Dossier.Content) || c.Dossier.SHA256 != model.SHA256(c.Dossier.Content) ||
		c.Prompt.Version != PromptVersion || c.Prompt.ByteSize != len(c.Prompt.Content) || c.Prompt.SHA256 != model.SHA256([]byte(c.Prompt.Content)) {
		return errors.New("planner candidate dossier or prompt bytes do not match their identities")
	}
	wantSchema, err := OutputSchema()
	if err != nil || c.ResponseSchema.Version != OutputSchemaVersion || c.ResponseSchema.ByteSize != len(c.ResponseSchema.Content) ||
		c.ResponseSchema.SHA256 != model.SHA256(c.ResponseSchema.Content) || !bytes.Equal(c.ResponseSchema.Content, wantSchema) {
		return errors.New("planner candidate response schema bytes do not match the canonical schema")
	}
	if err := validateModelPolicy(c.ModelPolicy, true); err != nil || c.HostPolicy != CurrentHostPolicy() {
		return errors.New("planner candidate model or host policy is untrusted")
	}
	var dossier Dossier
	if err := json.Unmarshal(c.Dossier.Content, &dossier); err != nil || dossier.SchemaVersion != DossierVersion {
		return errors.New("planner candidate dossier content is malformed")
	}
	state := CanonicalState{Task: dossier.Task, Run: dossier.Run, Source: dossier.Source, SupervisorDecisionID: dossier.SupervisorDecisionID, SupervisorDecisionSHA256: dossier.SupervisorDecisionSHA256, ArchitectureConstraints: dossier.ArchitectureConstraints, ProjectMap: dossier.ProjectMap, ModuleRelationships: dossier.ModuleRelationships, Conventions: dossier.Conventions, PriorDecisions: dossier.PriorDecisions, PriorPlan: dossier.PriorPlan}
	expectedRevision := c.Output.Revision
	expectedRevision.ContentSHA256 = nil
	if err := validateOutput(c.Output, state, expectedRevision, c.ExpectedRequest.OutputIdentity, dossierEvidence(state)); err != nil {
		return fmt.Errorf("planner candidate output is invalid: %w", err)
	}
	parsed, err := ParseOutput(c.RawOutput)
	if err != nil || !reflect.DeepEqual(parsed, c.Output) {
		return errors.New("planner candidate raw output does not reproduce the validated output")
	}
	canonical, _ := json.Marshal(c.Output)
	if !bytes.Equal(c.CanonicalOutput, canonical) || c.ModelResult.Outcome != model.OutcomeSuccess ||
		!reflect.DeepEqual(c.ModelResult.Request, c.ExpectedRequest) || !bytes.Equal(c.ModelResult.StructuredOutput, c.RawOutput) ||
		!c.ModelResult.Usage.Available || c.ModelResult.Service.Model != c.ModelPolicy.Model {
		return errors.New("planner candidate canonical output or model provenance is inconsistent")
	}
	return nil
}

func appendPlanEvent(ctx context.Context, q *postgres.Queries, c Candidate, eventType string, version int64, payload []byte, at time.Time) (string, error) {
	event := pgtype.UUID{Bytes: uuid.MustParse(id.New()), Valid: true}
	_, err := q.AppendEvent(ctx, postgres.AppendEventParams{ID: event, ProjectID: mustUUID(c.ProjectID), TaskID: mustUUID(c.TaskID), RunID: mustUUID(c.RunID), EventType: eventType, AggregateType: "plan", AggregateID: mustUUID(c.PlanID), AggregateVersion: version, Payload: payload, CreatedAt: timestamp(at)})
	return uuidString(event), err
}
func mustUUID(value string) pgtype.UUID {
	return pgtype.UUID{Bytes: uuid.MustParse(value), Valid: true}
}
func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}
func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}
