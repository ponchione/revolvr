package autonomousview

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomouspolicy"
)

type Source struct {
	Kind               SourceKind
	TaskID             string
	Title              string
	TaskPath           string
	TaskSHA256         string
	TaskByteSize       int
	Workflow           string
	TaskStatus         string
	StatePath          string
	StateSHA256        string
	StateByteSize      int
	ArchiveID          string
	ArchiveDisposition string
	ArchivedAt         time.Time
}

type AuditEvidence struct {
	Revision       int64
	RunID          string
	SourceRevision string
	ArtifactPath   string
	Report         autonomous.AuditReport
	Verification   autonomouspolicy.VerificationEvidence
}

type DecisionEvidence struct {
	Reference autonomous.DecisionReference
	Decision  autonomous.SupervisorDecision
	Available bool
	Admitted  bool
}

type Input struct {
	Source             Source
	State              *autonomous.ExecutionState
	Audits             []AuditEvidence
	Decision           *DecisionEvidence
	SchedulerReadiness string
	SchedulerReasons   []WhyReason
	References         []Reference
	Diagnostics        []Diagnostic
}

func Project(input Input) (View, error) {
	in, err := cloneInput(input)
	if err != nil {
		return View{}, err
	}
	view := View{
		SchemaVersion: SchemaVersion,
		Identity:      Identity{SourceKind: in.Source.Kind, TaskID: in.Source.TaskID, Title: in.Source.Title, TaskPath: in.Source.TaskPath, TaskSHA256: in.Source.TaskSHA256, TaskByteSize: in.Source.TaskByteSize, Workflow: in.Source.Workflow, TaskStatus: in.Source.TaskStatus, StatePath: in.Source.StatePath, StateSHA256: in.Source.StateSHA256, StateByteSize: in.Source.StateByteSize, ArchiveID: in.Source.ArchiveID, ArchiveDisposition: in.Source.ArchiveDisposition},
		Why:           Why{LatestDecision: "none", CurrentlyAdmittedAction: "none", SchedulerReadiness: valueOr(in.SchedulerReadiness, "not_available"), NextSupervisorAction: "undetermined_requires_supervisor"},
		Input:         OperatorInput{State: "none"}, Verification: Verification{State: "not_available"}, Audit: Audit{State: "not_available"}, Workspace: Workspace{State: "none"}, Terminal: Terminal{State: "active"},
		Acceptance: []Acceptance{}, Findings: []Finding{}, Attempts: Attempts{PerAction: []ActionAttempts{}, Budgets: []Budget{}, Events: []AttemptReference{}, Stops: []string{}}, Provenance: Provenance{WorkerRunIDs: []string{}, VerificationRunIDs: []string{}, AuditRunIDs: []string{}, References: append([]Reference(nil), in.References...)}, Diagnostics: append([]Diagnostic(nil), in.Diagnostics...),
	}
	if in.Source.Kind == SourceArchive {
		view.Terminal = Terminal{State: "archived", ArchiveID: in.Source.ArchiveID, Disposition: in.Source.ArchiveDisposition, ArchivedAt: in.Source.ArchivedAt, VerifiedNow: false}
		view.Why.SchedulerReadiness = "not_applicable_archive"
		view.Why.NextSupervisorAction = "not_applicable_terminal"
	}
	if in.State == nil {
		view.Summary.Phase = "autonomous evidence unavailable"
		view.Why.Reasons = append(view.Why.Reasons, in.SchedulerReasons...)
		view.Why.Reasons = append(view.Why.Reasons, WhyReason{Code: "autonomous_evidence_unavailable", Text: "No canonical autonomous execution state is available for this task."})
		if err := view.Validate(); err != nil {
			return View{}, err
		}
		return view, nil
	}
	state := *in.State
	if err := state.Validate(); err != nil {
		return View{}, fmt.Errorf("project autonomous view: %w", err)
	}
	if state.TaskID != in.Source.TaskID {
		return View{}, fmt.Errorf("project autonomous view: state task %q does not match source task %q", state.TaskID, in.Source.TaskID)
	}
	view.Identity.Lifecycle = string(state.Lifecycle)
	view.Identity.StateSchema = state.SchemaVersion
	view.Summary.Phase = string(state.Lifecycle)
	projectPlan(&view, state)
	projectAcceptance(&view, state)
	projectAuditsAndFindings(&view, state, in.Audits)
	projectAttempts(&view, state)
	projectInput(&view, state)
	projectWorkspace(&view, state)
	projectTerminal(&view, state, in.Source)
	projectDecision(&view, state, in.Decision)
	projectWhy(&view, state, in.SchedulerReasons)
	if err := view.Validate(); err != nil {
		return View{}, err
	}
	return view, nil
}

func cloneInput(input Input) (Input, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return Input{}, fmt.Errorf("project autonomous view: clone input: %w", err)
	}
	var clone Input
	if err := json.Unmarshal(raw, &clone); err != nil {
		return Input{}, fmt.Errorf("project autonomous view: clone input: %w", err)
	}
	return clone, nil
}

func projectPlan(v *View, state autonomous.ExecutionState) {
	if state.Plan == nil {
		return
	}
	p := state.Plan
	v.Plan = &Plan{ID: p.ID, Revision: p.Revision, SupersedesPlanID: p.SupersedesPlanID, Completed: p.Completed, Provenance: append([]autonomous.EvidenceReference(nil), p.Provenance...), Steps: make([]PlanStep, 0, len(p.Steps))}
	v.Summary.Plan.Total = len(p.Steps)
	for _, step := range p.Steps {
		v.Plan.Steps = append(v.Plan.Steps, PlanStep{ID: step.ID, Status: string(step.Status), Description: step.Description, Rationale: step.Rationale, Evidence: append([]autonomous.EvidenceReference(nil), step.Evidence...)})
		if step.Status == autonomous.PlanStepStatusCompleted || step.Status == autonomous.PlanStepStatusSkipped {
			v.Summary.Plan.Completed++
		}
	}
}

func projectAcceptance(v *View, state autonomous.ExecutionState) {
	v.Summary.Acceptance.Total = len(state.AcceptanceCriteria)
	for _, item := range state.AcceptanceCriteria {
		var source *autonomous.EvidenceReference
		if item.Source != nil {
			copyValue := *item.Source
			source = &copyValue
		}
		v.Acceptance = append(v.Acceptance, Acceptance{ID: item.ID, Description: item.Requirement, Status: string(item.Status), Rationale: item.Rationale, Evidence: append([]autonomous.EvidenceReference(nil), item.Evidence...), Source: source})
		if item.Status != autonomous.AcceptanceStatusPending {
			v.Summary.Acceptance.Completed++
		}
	}
}

func projectAuditsAndFindings(v *View, state autonomous.ExecutionState, audits []AuditEvidence) {
	index := map[string]int{}
	for _, item := range audits {
		identity := AuditIdentity{Revision: item.Revision, RunID: item.RunID, SourceRevision: item.SourceRevision, Disposition: string(item.Report.Disposition), ArtifactPath: item.ArtifactPath}
		v.Provenance.AuditRunIDs = appendUnique(v.Provenance.AuditRunIDs, item.RunID)
		if item.Verification.Summary.RunID != "" {
			v.Provenance.VerificationRunIDs = appendUnique(v.Provenance.VerificationRunIDs, item.Verification.Summary.RunID)
		}
		for _, finding := range item.Report.Findings {
			if pos, ok := index[finding.ID]; ok {
				v.Findings[pos].CurrentAudit = identity
				v.Findings[pos].Evidence = append([]autonomous.EvidenceReference(nil), finding.Evidence...)
				continue
			}
			index[finding.ID] = len(v.Findings)
			v.Findings = append(v.Findings, Finding{ID: finding.ID, Significance: string(finding.Significance), Summary: finding.Summary, RequiredCorrection: finding.RequiredCorrection, IntroducedBy: identity, CurrentAudit: identity, Status: string(autonomous.FindingResolutionStatusOpen), Evidence: append([]autonomous.EvidenceReference(nil), finding.Evidence...)})
		}
	}
	for _, resolution := range state.FindingResolutions {
		pos, ok := index[resolution.FindingID]
		if !ok {
			pos = len(v.Findings)
			index[resolution.FindingID] = pos
			v.Findings = append(v.Findings, Finding{ID: resolution.FindingID, Significance: "not_available", Summary: "not available", RequiredCorrection: "not available", Status: string(resolution.Status)})
			v.Diagnostics = append(v.Diagnostics, Diagnostic{Code: "finding_history_unavailable", Section: "findings", Detail: "Finding resolution is present but its introducing audit history is unavailable.", Reference: resolution.FindingID})
		}
		v.Findings[pos].Status = string(resolution.Status)
		v.Findings[pos].ResolutionEvidence = append([]autonomous.EvidenceReference(nil), resolution.Evidence...)
		v.Findings[pos].ResolutionRationale = resolution.Rationale
		v.Findings[pos].SupersedingFindingID = resolution.SupersedingFindingID
		if resolution.Resolution != nil {
			copyValue := *resolution.Resolution
			v.Findings[pos].ResolutionAuthority = &copyValue
		}
	}
	for _, finding := range v.Findings {
		if finding.Status != string(autonomous.FindingResolutionStatusOpen) {
			continue
		}
		if finding.Significance == string(autonomous.FindingSignificanceBlocking) {
			v.Summary.OpenBlockingFindings++
		} else {
			v.Summary.OpenNonBlockingFindings++
		}
	}
	if len(audits) == 0 {
		return
	}
	latest := audits[len(audits)-1]
	v.Audit = Audit{State: "available", Revision: latest.Revision, RunID: latest.RunID, SourceRevision: latest.SourceRevision, Disposition: string(latest.Report.Disposition), FindingCount: len(latest.Report.Findings), ArtifactPath: latest.ArtifactPath}
	verification := latest.Verification
	v.Verification = Verification{State: "available", RunID: verification.Summary.RunID, OccurrenceID: verification.Summary.OccurrenceID, SourceRevision: verification.SourceRevision, Status: string(verification.Summary.Status), Evidence: append([]autonomous.EvidenceReference(nil), verification.Summary.Evidence...)}
	if verification.Tiered != nil {
		v.Verification.Purpose = string(verification.Tiered.Purpose)
		if verification.Tiered.FinalSatisfied {
			v.Verification.FinalGate = "satisfied"
		} else {
			v.Verification.FinalGate = "not_satisfied"
		}
	}
}

func projectAttempts(v *View, state autonomous.ExecutionState) {
	a := state.Attempts
	v.Summary.TotalAttempts, v.Summary.ConsecutiveFailures = a.TotalAttempts, a.ConsecutiveFailures
	v.Attempts.Total, v.Attempts.ConsecutiveFailures = a.TotalAttempts, a.ConsecutiveFailures
	for _, item := range a.ActionAttempts {
		v.Attempts.PerAction = append(v.Attempts.PerAction, ActionAttempts{Action: string(item.Action), Attempts: item.Attempts})
	}
	v.Attempts.Budgets = append(v.Attempts.Budgets, countBudget("retry", a.RetryBudget, "attempts"), durationBudget("elapsed", a.ElapsedTimeBudget), countBudget("tokens", a.TokenBudget, "tokens"))
	for _, item := range a.ActionBudgets {
		v.Attempts.Budgets = append(v.Attempts.Budgets, countBudget("action:"+string(item.Action), item.Budget, "attempts"))
	}
	for _, event := range a.Events {
		v.Attempts.Events = append(v.Attempts.Events, AttemptReference{Sequence: event.Sequence, AttemptID: event.AttemptID, Kind: string(event.Kind), Action: string(event.Action), Outcome: string(event.Outcome), RunID: event.RunID, OccurrenceID: event.OccurrenceID, CreatedAt: event.CreatedAt, Evidence: append([]autonomous.EvidenceReference(nil), event.Evidence...)})
		if event.RunID != "" {
			v.Provenance.WorkerRunIDs = appendUnique(v.Provenance.WorkerRunIDs, event.RunID)
		}
	}
	for _, stop := range a.ActionStops {
		v.Attempts.Stops = append(v.Attempts.Stops, string(stop.Reason))
	}
	if state.CircuitBreaker != nil {
		v.Attempts.Stops = append(v.Attempts.Stops, string(state.CircuitBreaker.Reason))
	}
}

func countBudget(name string, b autonomous.CountBudget, unit string) Budget {
	result := Budget{Name: name, Mode: string(b.Mode), Limit: b.Limit, Consumed: b.Consumed, Unit: unit}
	if b.Mode == autonomous.BudgetModeLimited {
		result.Remaining = b.Limit - b.Consumed
		if result.Remaining < 0 {
			result.Remaining = 0
		}
		result.Exhausted = b.Consumed >= b.Limit
	}
	return result
}
func durationBudget(name string, b autonomous.DurationBudget) Budget {
	return countBudget(name, autonomous.CountBudget{Mode: b.Mode, Limit: int64(b.Limit), Consumed: int64(b.Consumed)}, "nanoseconds")
}

func projectInput(v *View, state autonomous.ExecutionState) {
	if state.NeedsInput == nil {
		if len(state.Input.Answers) > 0 {
			answer := state.Input.Answers[len(state.Input.Answers)-1]
			v.Input = OperatorInput{State: "answered", AnswerID: answer.AnswerID, AnswerOptionID: answer.OptionID, AnswerActor: answer.Provenance.Actor}
		}
		return
	}
	if state.NeedsInput.CurrentQuestion == nil {
		v.Input = OperatorInput{State: "legacy_non_answerable", BlockingReason: state.NeedsInput.Reason}
		return
	}
	id := *state.NeedsInput.CurrentQuestion
	for i := len(state.Input.Questions) - 1; i >= 0; i-- {
		record := state.Input.Questions[i]
		if record.Question.QuestionID != id.QuestionID || record.Question.Revision != id.Revision || record.Question.ContentSHA256 != id.ContentSHA256 {
			continue
		}
		q := record.Question
		v.Input = OperatorInput{State: "waiting", QuestionID: q.QuestionID, Revision: q.Revision, ContentSHA256: q.ContentSHA256, Question: q.Question, BlockingReason: q.BlockingReason, RecommendationOption: q.Recommendation.OptionID, RecommendationRationale: q.Recommendation.Rationale}
		for _, option := range q.Options {
			v.Input.Options = append(v.Input.Options, InputOption{ID: option.ID, Meaning: option.Meaning})
		}
		break
	}
}

func projectWorkspace(v *View, state autonomous.ExecutionState) {
	if state.Workspace == nil {
		return
	}
	w := state.Workspace
	v.Workspace = Workspace{State: "available", WorkspaceID: w.WorkspaceID, Status: string(w.Status), ExecutionRoot: w.ExecutionRoot, BranchRef: w.BranchRef, SourceRevision: w.SourceRevision, CheckpointSequence: w.Checkpoint.Sequence, CheckpointCommit: w.Checkpoint.CommitSHA}
}

func projectTerminal(v *View, state autonomous.ExecutionState, source Source) {
	v.Summary.NeedsInput = state.Lifecycle == autonomous.LifecycleStateNeedsInput
	v.Summary.Blocked = state.Lifecycle == autonomous.LifecycleStateBlocked
	switch state.Lifecycle {
	case autonomous.LifecycleStateCompleted, autonomous.LifecycleStateCancelled, autonomous.LifecycleStateSuperseded, autonomous.LifecycleStateAbandoned:
		v.Summary.Terminal = true
	}
	if source.Kind == SourceArchive {
		v.Summary.Terminal = true
		return
	}
	if state.Terminal != nil {
		v.Terminal.State = string(state.Lifecycle)
		v.Terminal.Reason = state.Terminal.Reason
	}
	if state.Finalization != nil {
		v.Terminal.FinalizationStage = string(state.Finalization.Stage)
	}
}

func projectDecision(v *View, state autonomous.ExecutionState, evidence *DecisionEvidence) {
	if state.LatestDecision != nil {
		copyValue := *state.LatestDecision
		v.Provenance.Decision = &copyValue
		v.Why.LatestDecisionReference = &copyValue
		v.Why.LatestDecision = string(copyValue.Action)
	}
	if evidence == nil || !evidence.Available || state.LatestDecision == nil {
		if state.LatestDecision != nil {
			v.Diagnostics = append(v.Diagnostics, Diagnostic{Code: "latest_decision_payload_unavailable", Section: "why", Detail: "The accepted decision reference is available but its typed payload could not be projected.", Reference: state.LatestDecision.Artifact.Reference})
		}
		return
	}
	if evidence.Reference != *state.LatestDecision {
		v.Diagnostics = append(v.Diagnostics, Diagnostic{Code: "latest_decision_reference_mismatch", Section: "why", Detail: "The decision payload does not match the canonical latest decision reference.", Reference: evidence.Reference.Artifact.Reference})
		return
	}
	v.Why.LatestDecision = string(evidence.Decision.Action)
	if evidence.Admitted {
		v.Why.CurrentlyAdmittedAction = string(evidence.Decision.Action)
	}
}

func projectWhy(v *View, state autonomous.ExecutionState, scheduler []WhyReason) {
	reasons := append([]WhyReason(nil), scheduler...)
	add := func(code, text string, evidence ...autonomous.EvidenceReference) {
		reasons = append(reasons, WhyReason{Code: code, Text: text, Evidence: append([]autonomous.EvidenceReference(nil), evidence...)})
	}
	switch state.Lifecycle {
	case autonomous.LifecycleStateCompleted, autonomous.LifecycleStateCancelled, autonomous.LifecycleStateSuperseded, autonomous.LifecycleStateAbandoned:
		add("terminal", "The autonomous lifecycle is terminal; no new route is admitted.")
		v.Why.NextSupervisorAction = "not_applicable_terminal"
	case autonomous.LifecycleStateFinalizing:
		add("finalizing", "Terminal finalization is in progress; ordinary worker routes are not admitted.")
	case autonomous.LifecycleStateNeedsInput:
		add("needs_input", "Operator input is required before ordinary routing can resume.")
	case autonomous.LifecycleStateBlocked:
		add("blocked", "The task is durably blocked and requires an explicit recovery transition.")
	default:
		if state.CircuitBreaker != nil {
			add("budget_or_circuit_breaker", "Attempt accounting has stopped further work: "+string(state.CircuitBreaker.Reason), state.CircuitBreaker.Evidence...)
		}
		if state.Plan == nil {
			add("plan_missing", "No current durable plan is available.")
		} else if !state.Plan.Completed {
			add("plan_incomplete", "The current durable plan still has nonterminal steps.")
		}
		pending := 0
		for _, item := range state.AcceptanceCriteria {
			if item.Status == autonomous.AcceptanceStatusPending {
				pending++
			}
		}
		if pending > 0 {
			add("acceptance_pending", fmt.Sprintf("%d acceptance criterion/criteria remain pending.", pending))
		}
		if v.Summary.OpenBlockingFindings > 0 {
			add("blocking_findings_open", fmt.Sprintf("%d blocking audit finding(s) remain open.", v.Summary.OpenBlockingFindings))
		}
		if v.Verification.State != "available" {
			add("verification_unavailable", "Current verification evidence is not available in the bounded snapshot.")
		}
		if v.Audit.State != "available" {
			add("audit_unavailable", "Current independent audit evidence is not available in the bounded snapshot.")
		}
	}
	if v.Why.CurrentlyAdmittedAction == "none" && v.Why.NextSupervisorAction == "undetermined_requires_supervisor" {
		add("next_supervisor_undetermined", "Revolvr cannot predict the next supervisor decision from lifecycle or plan position; a fresh supervisor must decide.")
	}
	precedence := map[string]int{"terminal": 10, "finalizing": 20, "needs_input": 30, "blocked": 40, "budget_or_circuit_breaker": 50, "scheduler_not_ready": 60, "plan_missing": 70, "plan_incomplete": 71, "acceptance_pending": 72, "blocking_findings_open": 73, "verification_unavailable": 74, "audit_unavailable": 75, "next_supervisor_undetermined": 100}
	sort.SliceStable(reasons, func(i, j int) bool {
		left, lok := precedence[reasons[i].Code]
		right, rok := precedence[reasons[j].Code]
		if !lok {
			left = 65
		}
		if !rok {
			right = 65
		}
		if left != right {
			return left < right
		}
		return reasons[i].Code < reasons[j].Code
	})
	v.Why.Reasons = reasons
}

func appendUnique(values []string, value string) []string {
	if strings.TrimSpace(value) == "" {
		return values
	}
	for _, item := range values {
		if item == value {
			return values
		}
	}
	return append(values, value)
}
func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// Redact returns a deep-cloned projection with every string replaced through
// the caller-owned configured-secret boundary.
func Redact(view View, replace func(string) string) (View, error) {
	if replace == nil {
		return view, nil
	}
	raw, err := Marshal(view)
	if err != nil {
		return View{}, err
	}
	var result View
	if err := json.Unmarshal(raw, &result); err != nil {
		return View{}, err
	}
	redactStrings(reflect.ValueOf(&result).Elem(), replace)
	if err := result.Validate(); err != nil {
		return View{}, err
	}
	return result, nil
}

func redactStrings(value reflect.Value, replace func(string) string) {
	if !value.IsValid() {
		return
	}
	switch value.Kind() {
	case reflect.String:
		if value.CanSet() {
			value.SetString(replace(value.String()))
		}
	case reflect.Pointer:
		if !value.IsNil() {
			redactStrings(value.Elem(), replace)
		}
	case reflect.Interface:
		if !value.IsNil() {
			redactStrings(value.Elem(), replace)
		}
	case reflect.Struct:
		for i := 0; i < value.NumField(); i++ {
			redactStrings(value.Field(i), replace)
		}
	case reflect.Slice:
		for i := 0; i < value.Len(); i++ {
			redactStrings(value.Index(i), replace)
		}
	}
}
