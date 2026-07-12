package autonomousview

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"revolvr/internal/autonomous"
)

func TestProjectDeterministicCompleteSectionsAndCallerImmutability(t *testing.T) {
	state := viewTestState(autonomous.LifecycleStateReady)
	input := Input{
		Source: Source{Kind: SourceActive, TaskID: "task-one", Title: "Task one", TaskPath: ".agent/tasks/task-one.md", TaskSHA256: strings.Repeat("a", 64), TaskByteSize: 120, Workflow: "autonomous-v1", TaskStatus: "pending", StatePath: ".revolvr/autonomous/tasks/task-one/state.json", StateSHA256: strings.Repeat("b", 64), StateByteSize: 400},
		State:  &state,
		Audits: []AuditEvidence{{
			Revision: 2, RunID: "audit-run", SourceRevision: strings.Repeat("c", 64), ArtifactPath: ".revolvr/runs/audit-run/auditor-output.canonical.json",
			Report: autonomous.AuditReport{
				TaskID: "task-one", Disposition: autonomous.AuditDispositionChangesRequired, Rationale: "One issue remains.",
				Inputs:   []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindTask, Reference: "task-one", Detail: "Task evidence."}},
				Findings: []autonomous.AuditFinding{{ID: "finding-one", Significance: autonomous.FindingSignificanceBlocking, Summary: "Broken edge", RequiredCorrection: "Fix edge.", Evidence: []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindFile, Reference: "edge.go", Detail: "Broken line."}}}},
			},
		}},
		SchedulerReadiness: "ready",
		References:         []Reference{{Kind: "task", Path: ".agent/tasks/task-one.md", Detail: "Canonical task."}},
	}
	before := cloneForTest(t, input)
	first, err := Project(input)
	if err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	second, err := Project(input)
	if err != nil {
		t.Fatalf("Project() second error = %v", err)
	}
	firstRaw, _ := Marshal(first)
	secondRaw, _ := Marshal(second)
	if !bytes.Equal(firstRaw, secondRaw) {
		t.Fatal("projection is not byte deterministic")
	}
	if !reflect.DeepEqual(input, before) {
		t.Fatal("Project mutated caller input")
	}
	if first.Summary.Plan != (Progress{Completed: 1, Total: 2}) || first.Summary.Acceptance != (Progress{Completed: 1, Total: 2}) {
		t.Fatalf("progress = %#v/%#v", first.Summary.Plan, first.Summary.Acceptance)
	}
	if len(first.Findings) != 1 || first.Findings[0].Status != "open" || first.Summary.OpenBlockingFindings != 1 {
		t.Fatalf("findings = %#v", first.Findings)
	}
	if first.Why.NextSupervisorAction != "undetermined_requires_supervisor" || first.Why.CurrentlyAdmittedAction != "none" {
		t.Fatalf("why = %#v", first.Why)
	}
	if !containsReason(first.Why.Reasons, "next_supervisor_undetermined") {
		t.Fatalf("reasons = %#v", first.Why.Reasons)
	}
}

func TestProjectLifecycleExplanations(t *testing.T) {
	tests := []struct {
		lifecycle autonomous.LifecycleState
		terminal  *autonomous.TerminalDetail
		needs     *autonomous.NeedsInputDetail
		code      string
		next      string
	}{
		{autonomous.LifecycleStatePending, nil, nil, "plan_incomplete", "undetermined_requires_supervisor"},
		{autonomous.LifecycleStateNeedsInput, nil, &autonomous.NeedsInputDetail{Reason: "Choose behavior."}, "needs_input", "undetermined_requires_supervisor"},
		{autonomous.LifecycleStateBlocked, &autonomous.TerminalDetail{Reason: "Budget stopped.", Evidence: []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindLedger, Reference: "run", Detail: "Stop."}}}, nil, "blocked", "undetermined_requires_supervisor"},
		{autonomous.LifecycleStateCancelled, &autonomous.TerminalDetail{Reason: "Operator cancelled.", Evidence: []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindLedger, Reference: "run", Detail: "Cancel."}}}, nil, "terminal", "not_applicable_terminal"},
	}
	for _, tt := range tests {
		t.Run(string(tt.lifecycle), func(t *testing.T) {
			state := viewTestState(tt.lifecycle)
			state.Terminal = tt.terminal
			state.NeedsInput = tt.needs
			view, err := Project(Input{Source: Source{Kind: SourceActive, TaskID: "task-one", Title: "Task", TaskPath: ".agent/tasks/task-one.md", TaskSHA256: strings.Repeat("a", 64), TaskByteSize: 1, Workflow: "autonomous-v1", TaskStatus: "pending", StatePath: ".revolvr/autonomous/tasks/task-one/state.json", StateSHA256: strings.Repeat("b", 64), StateByteSize: 1}, State: &state, SchedulerReadiness: "not_available"})
			if err != nil {
				t.Fatalf("Project() error = %v", err)
			}
			if !containsReason(view.Why.Reasons, tt.code) || view.Why.NextSupervisorAction != tt.next {
				t.Fatalf("why = %#v", view.Why)
			}
		})
	}
}

func TestMarshalDecodeStrictAndRedaction(t *testing.T) {
	state := viewTestState(autonomous.LifecycleStateReady)
	view, err := Project(Input{Source: Source{Kind: SourceActive, TaskID: "task-one", Title: "contains super-secret", TaskPath: ".agent/tasks/task-one.md", TaskSHA256: strings.Repeat("a", 64), TaskByteSize: 1, Workflow: "autonomous-v1", TaskStatus: "pending", StatePath: ".revolvr/autonomous/tasks/task-one/state.json", StateSHA256: strings.Repeat("b", 64), StateByteSize: 1}, State: &state, SchedulerReadiness: "ready"})
	if err != nil {
		t.Fatal(err)
	}
	redacted, err := Redact(view, func(value string) string { return strings.ReplaceAll(value, "super-secret", "[REDACTED]") })
	if err != nil {
		t.Fatal(err)
	}
	raw, err := Marshal(redacted)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("super-secret")) || !bytes.Contains(raw, []byte("[REDACTED]")) {
		t.Fatalf("redacted JSON = %s", raw)
	}
	if _, err := Decode(raw); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	malformed := bytes.Replace(raw, []byte(`"schema_version"`), []byte(`"unknown":"x","schema_version"`), 1)
	if _, err := Decode(malformed); err == nil {
		t.Fatal("Decode unknown field error = nil")
	}
}

func TestProjectTypedNeedsInputDoesNotSelectRecommendation(t *testing.T) {
	q := autonomous.NeedsInputQuestion{TaskID: "task-one", QuestionID: "product-mode", Revision: 1, Question: "Which mode?", BlockingReason: "The modes are incompatible.", Options: []autonomous.NeedsInputOption{{ID: "safe", Meaning: "Use the safe mode."}, {ID: "fast", Meaning: "Use the fast mode."}}, Recommendation: autonomous.NeedsInputRecommendation{OptionID: "safe", Rationale: "It preserves compatibility."}, Evidence: []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindTask, Reference: "task-one", Detail: "Ambiguous requirement."}}}
	identity, err := autonomous.QuestionContentSHA256(q)
	if err != nil {
		t.Fatal(err)
	}
	q.ContentSHA256 = identity
	when := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	decision := autonomous.DecisionReference{DecisionID: "decision-one", RunID: "supervisor-run", TaskID: "task-one", Action: autonomous.ActionNeedsInput, Artifact: autonomous.EvidenceReference{Kind: autonomous.EvidenceKindFile, Reference: ".revolvr/runs/supervisor-run/supervisor-decision.json", Detail: "Decision."}, CreatedAt: when}
	state := autonomous.ExecutionState{SchemaVersion: autonomous.ExecutionStateSchemaVersion, TaskID: "task-one", Lifecycle: autonomous.LifecycleStateNeedsInput, LatestDecision: &decision, Input: autonomous.InputState{TransitionSequence: 1, Questions: []autonomous.InputQuestionRecord{{Sequence: 1, TaskID: "task-one", Question: q, Decision: decision, SourceRevision: strings.Repeat("c", 64), ResumeLifecycle: autonomous.LifecycleStateReady, RecordedAt: when}}}, NeedsInput: &autonomous.NeedsInputDetail{CurrentQuestion: &autonomous.QuestionIdentity{QuestionID: q.QuestionID, Revision: q.Revision, ContentSHA256: q.ContentSHA256}}, Attempts: autonomous.AttemptState{RetryBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}, ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnlimited}, TokenBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnlimited}}}
	view, err := Project(Input{Source: Source{Kind: SourceActive, TaskID: "task-one", Title: "Task", TaskPath: ".agent/tasks/task-one.md", TaskSHA256: strings.Repeat("a", 64), TaskByteSize: 1, Workflow: "autonomous-v1", TaskStatus: "pending", StatePath: ".revolvr/autonomous/tasks/task-one/state.json", StateSHA256: strings.Repeat("b", 64), StateByteSize: 1}, State: &state, SchedulerReadiness: "needs_input"})
	if err != nil {
		t.Fatal(err)
	}
	if view.Input.State != "waiting" || view.Input.RecommendationOption != "safe" || view.Input.AnswerOptionID != "" || len(view.Input.Options) != 2 {
		t.Fatalf("operator input = %#v", view.Input)
	}
	if !containsReason(view.Why.Reasons, "needs_input") {
		t.Fatalf("why = %#v", view.Why)
	}
}

func viewTestState(lifecycle autonomous.LifecycleState) autonomous.ExecutionState {
	return autonomous.ExecutionState{SchemaVersion: autonomous.ExecutionStateSchemaVersion, TaskID: "task-one", Lifecycle: lifecycle, Plan: &autonomous.TaskPlan{TaskID: "task-one", ID: "plan-one", Revision: 1, Provenance: []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindTask, Reference: "task-one", Detail: "Task."}}, Steps: []autonomous.PlanStep{{ID: "step-one", Description: "Done", Status: autonomous.PlanStepStatusCompleted, Evidence: []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindVerification, Reference: "verify-one", Detail: "Passed."}}}, {ID: "step-two", Description: "Next", Status: autonomous.PlanStepStatusPending}}}, AcceptanceCriteria: []autonomous.AcceptanceCriterion{{ID: "criterion-one", Requirement: "Works", Status: autonomous.AcceptanceStatusSatisfied, Evidence: []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindVerification, Reference: "verify-one", Detail: "Passed."}}}, {ID: "criterion-two", Requirement: "Documented", Status: autonomous.AcceptanceStatusPending}}, FindingResolutions: []autonomous.FindingResolution{{FindingID: "finding-one", Status: autonomous.FindingResolutionStatusOpen}}, Attempts: autonomous.AttemptState{TotalAttempts: 1, RetryBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeLimited, Limit: 3, Consumed: 1}, ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnlimited}, TokenBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}}}
}

func containsReason(values []WhyReason, code string) bool {
	for _, item := range values {
		if item.Code == code {
			return true
		}
	}
	return false
}

func cloneForTest(t *testing.T, input Input) Input {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var result Input
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result
}
