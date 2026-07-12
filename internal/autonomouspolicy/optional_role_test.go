package autonomouspolicy

import (
	"testing"
	"time"

	"revolvr/internal/autonomous"
)

func TestCompletionIgnoresOrderOfExplicitOptionalRoleSkips(t *testing.T) {
	for _, roles := range [][]autonomous.WorkerProfile{{autonomous.WorkerProfileDocumentor, autonomous.WorkerProfileSimplifier}, {autonomous.WorkerProfileSimplifier, autonomous.WorkerProfileDocumentor}} {
		in := validInput(autonomous.ActionComplete)
		for i, role := range roles {
			in.State.OptionalRoles = append(in.State.OptionalRoles, policySkip(role, int64(i+1), in))
		}
		if _, err := Evaluate(in); err != nil {
			t.Fatalf("order %v: %v", roles, err)
		}
	}
}

func policySkip(role autonomous.WorkerProfile, sequence int64, in Input) autonomous.OptionalRoleOccurrence {
	action := autonomous.ActionDocument
	if role == autonomous.WorkerProfileSimplifier {
		action = autonomous.ActionSimplify
	}
	decision := decisionReference("optional-"+string(role), "supervisor-"+string(role), action, role)
	return autonomous.OptionalRoleOccurrence{SchemaVersion: autonomous.OptionalRoleOccurrenceSchemaVersion, Sequence: sequence, TaskID: taskID, Role: role, Outcome: autonomous.OptionalRoleOutcomeNotApplicable, Decision: decision, AssessmentSHA256: currentRevision, SourceBefore: in.Source.Revision, SourceAfter: in.Source.Revision, Gate: autonomous.OptionalRoleGate{SourceRevision: in.Source.Revision, VerificationRunID: in.Verification.Summary.RunID, VerificationOccurrenceID: in.Verification.Summary.OccurrenceID, AuditSupervisorRunID: "audit-supervisor-" + string(role), AuditWorkerRunID: "audit-worker-" + string(role), AuditRevision: sequence}, Evidence: []autonomous.EvidenceReference{evidence(autonomous.EvidenceKindTask, ".agent/tasks/task-1.md")}, Rationale: "Exact structured evidence found no relevant optional-role work.", CreatedAt: time.Date(2026, 7, 11, 14, int(sequence), 0, 0, time.UTC)}
}
