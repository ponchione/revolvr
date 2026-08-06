package supervisor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"revolvr/internal/model"
	"revolvr/internal/policy"
)

const SupervisorDecisionRecordVersion = "revolvr-supervisor-decision-record-v1"

var ErrDecisionRejected = errors.New("supervisor decision rejected")

type StateReader interface {
	ReadSupervisorState(context.Context, string, string) (CanonicalState, error)
}

type ModelInvoker interface {
	Invoke(context.Context, model.PreparedRequest) (model.Result, error)
}

type DecisionRecorder interface {
	PersistSupervisorDecision(context.Context, DecisionRecord) error
}

var _ ModelInvoker = (*model.Client)(nil)

type DecisionConfig struct {
	TaskID         string
	TaskVersionID  string
	TaskVersion    int64
	RunID          string
	SourceRevision string
	SourceCommit   string
	SourceTree     string
	DecisionID     string
	ModelPolicy    ModelPolicy
	HostPolicy     policy.Identity
	StateReader    StateReader
	Model          ModelInvoker
	Recorder       DecisionRecorder
}

type ByteArtifact struct {
	Version  string          `json:"version"`
	SHA256   string          `json:"sha256"`
	ByteSize int             `json:"byte_size"`
	Content  json.RawMessage `json:"content"`
}

type TextArtifact struct {
	Version  string `json:"version"`
	SHA256   string `json:"sha256"`
	ByteSize int    `json:"byte_size"`
	Content  string `json:"content"`
}

type DecisionDisposition string

const (
	DecisionAccepted DecisionDisposition = "accepted"
	DecisionRejected DecisionDisposition = "rejected"
)

// ModelInvocationProvenance retains the complete task-013 result while
// treating model-controlled response bytes as opaque persistence data. A
// rejected malformed response must not make the enclosing decision record
// impossible to encode.
type ModelInvocationProvenance struct {
	Outcome           model.Outcome           `json:"outcome"`
	Request           model.RequestEvidence   `json:"request"`
	Attempts          []model.AttemptEvidence `json:"attempts"`
	Diagnostics       []model.DiagnosticEvent `json:"diagnostics,omitempty"`
	CompletedResponse []byte                  `json:"completed_response,omitempty"`
	StructuredOutput  []byte                  `json:"structured_output,omitempty"`
	Refusal           string                  `json:"refusal,omitempty"`
	Usage             model.UsageEvidence     `json:"usage"`
	Latency           model.LatencyEvidence   `json:"latency"`
	Service           model.ServiceEvidence   `json:"service"`
	Cost              model.CostEvidence      `json:"cost"`
}

type DecisionRecord struct {
	SchemaVersion     string                    `json:"schema_version"`
	Disposition       DecisionDisposition       `json:"disposition"`
	ReasonCode        string                    `json:"reason_code"`
	ReasonDetail      string                    `json:"reason_detail"`
	DecisionID        string                    `json:"decision_id"`
	TaskID            string                    `json:"task_id"`
	TaskVersionID     string                    `json:"task_version_id"`
	TaskVersion       int64                     `json:"task_version"`
	RunID             string                    `json:"run_id"`
	SourceRevision    string                    `json:"source_revision"`
	SourceCommit      string                    `json:"source_commit"`
	SourceTree        string                    `json:"source_tree"`
	Dossier           DossierArtifact           `json:"dossier"`
	ObservedDossier   *DossierArtifact          `json:"observed_dossier"`
	Prompt            TextArtifact              `json:"prompt"`
	ResponseSchema    ByteArtifact              `json:"response_schema"`
	ModelPolicy       ModelPolicy               `json:"model_policy"`
	HostPolicy        policy.Identity           `json:"host_policy"`
	ExpectedRequest   model.RequestEvidence     `json:"expected_request"`
	ModelResult       ModelInvocationProvenance `json:"model_result"`
	RawOutput         []byte                    `json:"raw_output"`
	Decision          *Decision                 `json:"decision"`
	CanonicalDecision json.RawMessage           `json:"canonical_decision"`
	Route             *policy.Route             `json:"route"`
}

type PreparedDecision struct {
	State            CanonicalState
	Dossier          DossierArtifact
	Prompt           TextArtifact
	ResponseSchema   ByteArtifact
	ExpectedOutput   model.OutputIdentity
	ExpectedDecision DecisionIdentity
	ExpectedRequest  model.RequestEvidence
	Request          model.PreparedRequest
}

type DecisionResult struct {
	Disposition DecisionDisposition
	Decision    *Decision
	Route       *policy.Route
	Record      DecisionRecord
}

func PrepareSupervisorDecision(cfg DecisionConfig, state CanonicalState) (PreparedDecision, error) {
	if err := validateDecisionConfig(cfg); err != nil {
		return PreparedDecision{}, err
	}
	if err := validatePinnedState(cfg, state); err != nil {
		return PreparedDecision{}, err
	}
	dossier, err := BuildSupervisorDossier(state)
	if err != nil {
		return PreparedDecision{}, err
	}
	schemaRaw, err := DecisionOutputSchemaV1()
	if err != nil {
		return PreparedDecision{}, fmt.Errorf("prepare supervisor decision schema: %w", err)
	}
	schemaSHA := model.SHA256(schemaRaw)
	expectedDecision := DecisionIdentity{
		DecisionID: cfg.DecisionID, TaskVersionID: cfg.TaskVersionID, TaskVersion: cfg.TaskVersion,
		DossierVersion: dossier.Version, DossierSHA256: dossier.SHA256,
		ModelPolicyVersion: cfg.ModelPolicy.Version, ModelPolicySHA256: cfg.ModelPolicy.SHA256,
		HostPolicyVersion: cfg.HostPolicy.Version, HostPolicySHA256: cfg.HostPolicy.SHA256,
	}
	promptRaw := buildSupervisorPrompt(cfg, dossier, schemaSHA, expectedDecision)
	prompt := TextArtifact{Version: SupervisorPromptVersion, SHA256: model.SHA256(promptRaw), ByteSize: len(promptRaw), Content: string(promptRaw)}
	requestID := cfg.RunID + ".supervisor." + cfg.DecisionID
	expectedOutput := model.OutputIdentity{
		RequestID: requestID, TaskID: cfg.TaskID, RunID: cfg.RunID, SourceRevision: cfg.SourceRevision,
		PromptVersion: SupervisorPromptVersion, PromptSHA256: prompt.SHA256,
		ResponseSchemaVersion: SupervisorDecisionVersion, ResponseSchemaSHA256: schemaSHA,
	}
	request := model.Request{
		RequestID: requestID, TaskID: cfg.TaskID, RunID: cfg.RunID, SourceRevision: cfg.SourceRevision,
		Model: cfg.ModelPolicy.Model, ReasoningEffort: cfg.ModelPolicy.ReasoningEffort, MaxOutputTokens: cfg.ModelPolicy.MaxOutputTokens,
		PromptVersion: SupervisorPromptVersion, PromptSHA256: prompt.SHA256, Prompt: prompt.Content,
		ResponseSchemaVersion: SupervisorDecisionVersion, ResponseSchemaSHA256: schemaSHA, ResponseSchemaName: SupervisorDecisionSchemaName, ResponseSchema: schemaRaw,
		Timeout: cfg.ModelPolicy.Timeout, MaxStreamBytes: cfg.ModelPolicy.MaxStreamBytes, MaxDiagnosticBytes: cfg.ModelPolicy.MaxDiagnosticBytes, Retry: cfg.ModelPolicy.Retry,
	}
	preparedRequest, err := model.Prepare(request)
	if err != nil {
		return PreparedDecision{}, fmt.Errorf("prepare task-013 model request: %w", err)
	}
	expectedRequest, err := expectedRequestEvidence(request, expectedOutput)
	if err != nil {
		return PreparedDecision{}, err
	}
	return PreparedDecision{
		State: state, Dossier: dossier, Prompt: prompt,
		ResponseSchema: ByteArtifact{Version: SupervisorDecisionVersion, SHA256: schemaSHA, ByteSize: len(schemaRaw), Content: append(json.RawMessage(nil), schemaRaw...)},
		ExpectedOutput: expectedOutput, ExpectedDecision: expectedDecision, ExpectedRequest: expectedRequest, Request: preparedRequest,
	}, nil
}

func Decide(ctx context.Context, cfg DecisionConfig) (DecisionResult, error) {
	if err := validateDecisionConfig(cfg); err != nil {
		return DecisionResult{}, err
	}
	state, err := cfg.StateReader.ReadSupervisorState(ctx, cfg.TaskID, cfg.RunID)
	if err != nil {
		return DecisionResult{}, fmt.Errorf("read canonical supervisor state: %w", err)
	}
	prepared, err := PrepareSupervisorDecision(cfg, state)
	if err != nil {
		return DecisionResult{}, err
	}
	if state.Budget.ModelCallsRemaining <= 0 || state.Budget.TokensRemaining <= 0 {
		return DecisionResult{}, errors.New("supervisor model budget is exhausted before invocation")
	}

	modelResult, invokeErr := cfg.Model.Invoke(ctx, prepared.Request)
	record := baseDecisionRecord(cfg, prepared, modelResult)
	if invokeErr != nil || modelResult.Outcome != model.OutcomeSuccess {
		code := "model_" + string(modelResult.Outcome)
		if modelResult.Outcome == "" {
			code = "model_invocation_failure"
		}
		detail := errorDetail(invokeErr)
		if detail == "" {
			detail = "model invocation did not produce an accepted structured output"
		}
		return persistRejected(ctx, cfg.Recorder, record, code, detail)
	}
	if err := validateModelResult(prepared, cfg.ModelPolicy, modelResult); err != nil {
		return persistRejected(ctx, cfg.Recorder, record, "stale_model_identity", err.Error())
	}
	decision, err := ParseStructuredDecision(modelResult.StructuredOutput)
	if err != nil {
		return persistRejected(ctx, cfg.Recorder, record, "malformed_decision", err.Error())
	}
	record.Decision = &decision
	record.CanonicalDecision, err = canonicalDecision(decision)
	if err != nil {
		return persistRejected(ctx, cfg.Recorder, record, "malformed_decision", err.Error())
	}
	if err := validateDecisionFreshness(prepared, decision); err != nil {
		return persistRejected(ctx, cfg.Recorder, record, "stale_decision_identity", err.Error())
	}
	if err := validateDecisionEvidence(prepared.State, decision); err != nil {
		return persistRejected(ctx, cfg.Recorder, record, "untrusted_decision_evidence", err.Error())
	}

	current, err := cfg.StateReader.ReadSupervisorState(ctx, cfg.TaskID, cfg.RunID)
	if err != nil {
		return persistRejected(ctx, cfg.Recorder, record, "freshness_unavailable", err.Error())
	}
	observedDossier, dossierErr := BuildSupervisorDossier(current)
	if dossierErr != nil {
		return persistRejected(ctx, cfg.Recorder, record, "freshness_unavailable", dossierErr.Error())
	}
	record.ObservedDossier = &observedDossier
	if err := validatePinnedState(cfg, current); err != nil || observedDossier.SHA256 != prepared.Dossier.SHA256 || !reflect.DeepEqual(current, prepared.State) {
		detail := "canonical task, source, or dossier identity changed during the supervisor invocation"
		if err != nil {
			detail = err.Error()
		}
		return persistRejected(ctx, cfg.Recorder, record, "stale_canonical_state", detail)
	}

	route, err := policy.RouteSupervisor(policyInput(current, decision, modelResult.Usage.TotalTokens))
	if err != nil {
		return persistRejected(ctx, cfg.Recorder, record, "host_policy_denied", err.Error())
	}
	record.Disposition = DecisionAccepted
	record.ReasonCode = "accepted"
	record.ReasonDetail = "trusted host policy admitted one advisory route"
	record.Route = &route
	if err := persistRecord(ctx, cfg.Recorder, record); err != nil {
		return DecisionResult{}, err
	}
	return DecisionResult{Disposition: DecisionAccepted, Decision: &decision, Route: &route, Record: record}, nil
}

func validateDecisionConfig(cfg DecisionConfig) error {
	if cfg.StateReader == nil || cfg.Model == nil || cfg.Recorder == nil {
		return errors.New("supervisor decision requires state reader, task-013 model client, and decision recorder")
	}
	for _, value := range []struct{ label, text string }{{"task_id", cfg.TaskID}, {"task_version_id", cfg.TaskVersionID}, {"run_id", cfg.RunID}, {"decision_id", cfg.DecisionID}} {
		if !stableToken(value.text) {
			return fmt.Errorf("supervisor %s is missing or not normalized", value.label)
		}
	}
	if cfg.TaskVersion <= 0 || !validSHA256(cfg.SourceRevision) || !validGitOID(cfg.SourceCommit) || !validGitOID(cfg.SourceTree) {
		return errors.New("supervisor pinned task version or source identity is invalid")
	}
	if err := validateModelPolicy(cfg.ModelPolicy, true); err != nil {
		return err
	}
	if err := policy.ValidateIdentity(cfg.HostPolicy); err != nil {
		return err
	}
	return nil
}

func validatePinnedState(cfg DecisionConfig, state CanonicalState) error {
	if state.Task.TaskID != cfg.TaskID || state.Task.TaskVersionID != cfg.TaskVersionID || state.Task.Version != cfg.TaskVersion || state.Run.RunID != cfg.RunID {
		return errors.New("canonical task version or run does not match scheduler-pinned authority")
	}
	if state.Source.Revision != cfg.SourceRevision || state.Source.Commit != cfg.SourceCommit || state.Source.Tree != cfg.SourceTree {
		return errors.New("canonical source does not match scheduler-pinned authority")
	}
	return nil
}

func buildSupervisorPrompt(cfg DecisionConfig, dossier DossierArtifact, schemaSHA string, decisionIdentity DecisionIdentity) []byte {
	identityRaw, _ := json.Marshal(decisionIdentity)
	var out strings.Builder
	out.WriteString("# Revolvr supervisor decision\n\n")
	out.WriteString("This is one fresh, stateless, decision-only invocation. Use no tools and no prior conversation. Return exactly one object matching the supplied closed schema. Do not execute work, mutate lifecycle or PostgreSQL state, answer an operator question, broaden scope, verify, audit, correct, or finalize.\n\n")
	out.WriteString("Admitted actions: plan, implement, correct, document, simplify, complete, block, needs_input. The host validates lifecycle, scope, budgets, evidence, and policy. Complete is only a proposal to completion preflight. Block and needs_input are advisory data only.\n\n")
	out.WriteString("Prompt version: " + SupervisorPromptVersion + "\n")
	out.WriteString("Response schema version: " + SupervisorDecisionVersion + "\n")
	out.WriteString("Response schema SHA-256: " + schemaSHA + "\n")
	out.WriteString("Model policy: " + cfg.ModelPolicy.Version + "/" + cfg.ModelPolicy.SHA256 + " model=" + cfg.ModelPolicy.Model + " reasoning=" + cfg.ModelPolicy.ReasoningEffort + " tools=none\n")
	out.WriteString("Host policy: " + cfg.HostPolicy.Version + "/" + cfg.HostPolicy.SHA256 + "\n")
	out.WriteString("Decision identity material (content_sha256 is assigned or checked by the host): " + string(identityRaw) + "\n")
	out.WriteString("Dossier: " + dossier.Version + "/" + dossier.SHA256 + " bytes=" + fmt.Sprint(dossier.ByteSize) + "\n\n")
	out.WriteString("## Frozen canonical dossier\n\n")
	out.Write(dossier.Content)
	out.WriteString("\n")
	return []byte(out.String())
}

func expectedRequestEvidence(req model.Request, output model.OutputIdentity) (model.RequestEvidence, error) {
	retryRaw, err := json.Marshal(req.Retry)
	if err != nil {
		return model.RequestEvidence{}, err
	}
	maxStream := req.MaxStreamBytes
	if maxStream == 0 {
		maxStream = model.DefaultMaxStreamBytes
	}
	maxDiagnostic := req.MaxDiagnosticBytes
	if maxDiagnostic == 0 {
		maxDiagnostic = model.DefaultMaxDiagnosticBytes
	}
	return model.RequestEvidence{
		RequestID: req.RequestID, TaskID: req.TaskID, RunID: req.RunID, SourceRevision: req.SourceRevision,
		Model: req.Model, ReasoningEffort: req.ReasoningEffort, MaxOutputTokens: req.MaxOutputTokens,
		PromptVersion: req.PromptVersion, PromptSHA256: req.PromptSHA256, ResponseSchemaVersion: req.ResponseSchemaVersion,
		ResponseSchemaSHA256: req.ResponseSchemaSHA256, ResponseSchemaName: req.ResponseSchemaName, Timeout: req.Timeout,
		MaxStreamBytes: maxStream, MaxDiagnosticBytes: maxDiagnostic, RetryPolicySHA256: model.SHA256(retryRaw), Retry: req.Retry, OutputIdentity: output,
	}, nil
}

func validateModelResult(prepared PreparedDecision, modelPolicy ModelPolicy, result model.Result) error {
	if !reflect.DeepEqual(result.Request, prepared.ExpectedRequest) {
		return errors.New("task-013 request provenance does not match the exact prepared supervisor invocation")
	}
	if !result.Usage.Available {
		return errors.New("successful supervisor result is missing usage evidence")
	}
	if result.Service.Model != modelPolicy.Model {
		return fmt.Errorf("responding model %q does not match pinned model %q", result.Service.Model, modelPolicy.Model)
	}
	return nil
}

func validateDecisionFreshness(prepared PreparedDecision, decision Decision) error {
	if decision.RevolvrIdentity != prepared.ExpectedOutput {
		return errors.New("decision request, task, run, source, prompt, or schema identity is stale")
	}
	got := decision.Identity
	content := got.ContentSHA256
	got.ContentSHA256 = nil
	want := prepared.ExpectedDecision
	if !reflect.DeepEqual(got, want) || content == nil || !validSHA256(*content) {
		return errors.New("decision task-version, dossier, model-policy, host-policy, or decision identity is stale")
	}
	return nil
}

func validateDecisionEvidence(state CanonicalState, decision Decision) error {
	allowed := evidenceIDs(state)
	for _, reference := range decision.EvidenceRefs {
		if _, ok := allowed[reference]; !ok {
			return fmt.Errorf("decision cites evidence %q outside the frozen dossier", reference)
		}
	}
	for _, references := range [][]string{func() []string {
		if decision.Block == nil {
			return nil
		}
		return decision.Block.EvidenceRefs
	}(), func() []string {
		if decision.NeedsInput == nil {
			return nil
		}
		return decision.NeedsInput.EvidenceRefs
	}()} {
		for _, reference := range references {
			if !slicesContains(decision.EvidenceRefs, reference) {
				return fmt.Errorf("action-specific evidence %q is absent from decision evidence", reference)
			}
		}
	}
	return nil
}

func policyInput(state CanonicalState, decision Decision, usage int64) policy.Input {
	criteria := make([]policy.CriterionGate, len(state.Criteria))
	for i, value := range state.Criteria {
		criteria[i] = policy.CriterionGate{ID: value.ID, Status: value.Status}
	}
	findings := make([]policy.FindingGate, len(state.Findings))
	for i, value := range state.Findings {
		findings[i] = policy.FindingGate{ID: value.ID, Open: value.Open}
	}
	var plan *policy.PlanGate
	if state.Plan != nil {
		plan = &policy.PlanGate{ID: state.Plan.ID, Completed: state.Plan.Completed, Steps: append([]policy.PlanStepGate(nil), state.Plan.Steps...)}
	}
	var verification *policy.VerificationGate
	if state.LatestVerification != nil {
		value := state.LatestVerification
		verification = &policy.VerificationGate{ID: value.ID, Status: value.Status, SourceRevision: value.SourceRevision, Final: value.Final, EvidenceComplete: value.EvidenceComplete}
	}
	var audit *policy.AuditGate
	if state.LatestAudit != nil {
		value := state.LatestAudit
		audit = &policy.AuditGate{ID: value.ID, Status: value.Status, SourceRevision: value.SourceRevision, VerificationID: value.VerificationID, Independent: value.Independent, EvidenceComplete: value.EvidenceComplete}
	}
	return policy.Input{
		TaskID: state.Task.TaskID, Lifecycle: state.Lifecycle, SourceRevision: state.Source.Revision, SourceSafe: state.Source.Safe,
		Budget: state.Budget, SupervisorUsageTokens: usage, Scope: policy.Scope{AllowedPaths: append([]string(nil), state.Task.AllowedPaths...), ExcludedPaths: append([]string(nil), state.Task.ExcludedPaths...)},
		Plan: plan, Criteria: criteria, Verification: verification, Audit: audit, Findings: findings,
		WorkspaceReconciled: state.WorkspaceReconciled, ArtifactManifestID: state.ArtifactManifest.ID, ArtifactManifestComplete: state.ArtifactManifest.Complete,
		Proposal: policy.Proposal{DecisionID: decision.Identity.DecisionID, Action: decision.Action, Scope: append([]string(nil), decision.Scope...), Correction: decision.Correction, Completion: decision.Completion},
	}
}

func baseDecisionRecord(cfg DecisionConfig, prepared PreparedDecision, result model.Result) DecisionRecord {
	return DecisionRecord{
		SchemaVersion: SupervisorDecisionRecordVersion, Disposition: DecisionRejected,
		DecisionID: cfg.DecisionID, TaskID: cfg.TaskID, TaskVersionID: cfg.TaskVersionID, TaskVersion: cfg.TaskVersion,
		RunID: cfg.RunID, SourceRevision: cfg.SourceRevision, SourceCommit: cfg.SourceCommit, SourceTree: cfg.SourceTree,
		Dossier: prepared.Dossier, Prompt: prepared.Prompt, ResponseSchema: prepared.ResponseSchema,
		ModelPolicy: cfg.ModelPolicy, HostPolicy: cfg.HostPolicy, ExpectedRequest: prepared.ExpectedRequest,
		ModelResult: modelInvocationProvenance(result), RawOutput: append([]byte(nil), result.StructuredOutput...),
	}
}

func modelInvocationProvenance(result model.Result) ModelInvocationProvenance {
	return ModelInvocationProvenance{
		Outcome: result.Outcome, Request: result.Request,
		Attempts:          append([]model.AttemptEvidence(nil), result.Attempts...),
		Diagnostics:       append([]model.DiagnosticEvent(nil), result.Diagnostics...),
		CompletedResponse: append([]byte(nil), result.CompletedResponse...),
		StructuredOutput:  append([]byte(nil), result.StructuredOutput...),
		Refusal:           result.Refusal, Usage: result.Usage, Latency: result.Latency,
		Service: result.Service, Cost: result.Cost,
	}
}

func persistRejected(ctx context.Context, recorder DecisionRecorder, record DecisionRecord, code, detail string) (DecisionResult, error) {
	record.Disposition = DecisionRejected
	record.ReasonCode = code
	record.ReasonDetail = detail
	if err := persistRecord(ctx, recorder, record); err != nil {
		return DecisionResult{}, err
	}
	return DecisionResult{Disposition: DecisionRejected, Decision: record.Decision, Record: record}, fmt.Errorf("%w: %s: %s", ErrDecisionRejected, code, detail)
}

func persistRecord(ctx context.Context, recorder DecisionRecorder, record DecisionRecord) error {
	if err := recorder.PersistSupervisorDecision(ctx, cloneDecisionRecord(record)); err != nil {
		return fmt.Errorf("persist supervisor decision provenance: %w", err)
	}
	return nil
}

func cloneDecisionRecord(record DecisionRecord) DecisionRecord {
	cloned := record
	cloned.Dossier.Content = append(json.RawMessage(nil), record.Dossier.Content...)
	cloned.ResponseSchema.Content = append(json.RawMessage(nil), record.ResponseSchema.Content...)
	cloned.ExpectedRequest.Retry.RetryableStatusCodes = append([]int(nil), record.ExpectedRequest.Retry.RetryableStatusCodes...)
	cloned.ExpectedRequest.Retry.RetryableStreamErrCodes = append([]string(nil), record.ExpectedRequest.Retry.RetryableStreamErrCodes...)
	cloned.ModelResult.Request.Retry.RetryableStatusCodes = append([]int(nil), record.ModelResult.Request.Retry.RetryableStatusCodes...)
	cloned.ModelResult.Request.Retry.RetryableStreamErrCodes = append([]string(nil), record.ModelResult.Request.Retry.RetryableStreamErrCodes...)
	cloned.ModelResult.Attempts = append([]model.AttemptEvidence(nil), record.ModelResult.Attempts...)
	cloned.ModelResult.Diagnostics = append([]model.DiagnosticEvent(nil), record.ModelResult.Diagnostics...)
	cloned.ModelResult.CompletedResponse = append([]byte(nil), record.ModelResult.CompletedResponse...)
	cloned.ModelResult.StructuredOutput = append([]byte(nil), record.ModelResult.StructuredOutput...)
	cloned.RawOutput = append([]byte(nil), record.RawOutput...)
	cloned.CanonicalDecision = append(json.RawMessage(nil), record.CanonicalDecision...)
	if record.ObservedDossier != nil {
		value := *record.ObservedDossier
		value.Content = append(json.RawMessage(nil), record.ObservedDossier.Content...)
		cloned.ObservedDossier = &value
	}
	if record.Decision != nil {
		value := *record.Decision
		value.EvidenceRefs = append([]string(nil), value.EvidenceRefs...)
		value.Scope = append([]string(nil), value.Scope...)
		cloned.Decision = &value
	}
	if record.Route != nil {
		value := *record.Route
		cloned.Route = &value
	}
	return cloned
}

func canonicalDecision(decision Decision) (json.RawMessage, error) {
	raw, err := json.Marshal(decision)
	if err != nil {
		return nil, fmt.Errorf("encode canonical supervisor decision: %w", err)
	}
	return append(json.RawMessage(nil), raw...), nil
}

func slicesContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func errorDetail(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
