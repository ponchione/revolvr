package planner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"revolvr/internal/model"
	"revolvr/internal/supervisor"
)

type StateReader interface {
	ReadPlannerState(context.Context, string, string) (CanonicalState, error)
}
type ModelInvoker interface {
	Invoke(context.Context, model.PreparedRequest) (model.Result, error)
}

type Config struct {
	TaskID, TaskVersionID, RunID                              string
	TaskVersionNumber                                         int64
	ProjectSourceID, SourceRevision, SourceCommit, SourceTree string
	PlanID, PlanVersionID                                     string
	ExpectedPlanAggregateVersion                              int64
	Admission                                                 supervisor.DecisionRecord
	ModelPolicy                                               ModelPolicy
	HostPolicy                                                HostPolicy
	StateReader                                               StateReader
	Model                                                     ModelInvoker
}

type TextArtifact struct {
	Version, SHA256 string
	ByteSize        int
	Content         string
}
type JSONArtifact struct {
	Version, SHA256 string
	ByteSize        int
	Content         json.RawMessage
}

type Candidate struct {
	SchemaVersion                string                               `json:"schema_version"`
	CandidateSHA256              string                               `json:"candidate_sha256"`
	PlanID                       string                               `json:"plan_id"`
	PlanVersionID                string                               `json:"plan_version_id"`
	ExpectedPlanAggregateVersion int64                                `json:"expected_plan_aggregate_version"`
	TaskID                       string                               `json:"task_id"`
	TaskVersionID                string                               `json:"task_version_id"`
	TaskVersionNumber            int64                                `json:"task_version_number"`
	RunID                        string                               `json:"run_id"`
	ProjectID                    string                               `json:"project_id"`
	ProjectSourceID              string                               `json:"project_source_id"`
	SourceRevision               string                               `json:"source_revision"`
	SourceCommit                 string                               `json:"source_commit"`
	SourceTree                   string                               `json:"source_tree"`
	SupervisorDecisionID         string                               `json:"supervisor_decision_id"`
	SupervisorDecisionSHA256     string                               `json:"supervisor_decision_sha256"`
	Dossier                      DossierArtifact                      `json:"dossier"`
	Prompt                       TextArtifact                         `json:"prompt"`
	ResponseSchema               JSONArtifact                         `json:"response_schema"`
	ModelPolicy                  ModelPolicy                          `json:"model_policy"`
	HostPolicy                   HostPolicy                           `json:"host_policy"`
	ExpectedRequest              model.RequestEvidence                `json:"expected_request"`
	ModelResult                  supervisor.ModelInvocationProvenance `json:"model_result"`
	RawOutput                    []byte                               `json:"raw_output"`
	CanonicalOutput              json.RawMessage                      `json:"canonical_output"`
	Output                       Output                               `json:"output"`
}

type Prepared struct {
	State            CanonicalState
	Dossier          DossierArtifact
	Prompt           TextArtifact
	ResponseSchema   JSONArtifact
	ExpectedOutput   model.OutputIdentity
	ExpectedRevision RevisionIdentity
	ExpectedRequest  model.RequestEvidence
	Request          model.PreparedRequest
}

func Prepare(cfg Config, state CanonicalState) (Prepared, error) {
	if err := validateConfig(cfg); err != nil {
		return Prepared{}, err
	}
	if err := validatePinnedState(cfg, state); err != nil {
		return Prepared{}, err
	}
	dossier, err := BuildDossier(state)
	if err != nil {
		return Prepared{}, err
	}
	schemaRaw, err := OutputSchema()
	if err != nil {
		return Prepared{}, err
	}
	schemaSHA := model.SHA256(schemaRaw)
	host := CurrentHostPolicy()
	if cfg.HostPolicy != host {
		return Prepared{}, errors.New("planner host policy identity is untrusted")
	}
	revision := RevisionIdentity{
		PlanID: cfg.PlanID, PlanVersionID: cfg.PlanVersionID, TaskID: cfg.TaskID,
		TaskVersionID: cfg.TaskVersionID, TaskVersionNumber: cfg.TaskVersionNumber,
		RunID: cfg.RunID, ProjectSourceID: cfg.ProjectSourceID, SourceRevision: cfg.SourceRevision,
		SupervisorDecisionID: cfg.Admission.DecisionID, SupervisorDecisionSHA256: *cfg.Admission.Decision.Identity.ContentSHA256,
		DossierVersion: dossier.Version, DossierSHA256: dossier.SHA256,
		ResponseSchemaVersion: OutputSchemaVersion, ResponseSchemaSHA256: schemaSHA,
		ModelPolicyVersion: cfg.ModelPolicy.Version, ModelPolicySHA256: cfg.ModelPolicy.SHA256,
		HostPolicyVersion: host.Version, HostPolicySHA256: host.SHA256,
	}
	if state.PriorPlan == nil {
		revision.RevisionNumber = 1
	} else {
		revision.RevisionNumber = state.PriorPlan.Revision + 1
		prior := state.PriorPlan.PlanVersionID
		revision.SupersedesPlanVersionID = &prior
	}
	promptRaw := buildPrompt(cfg, dossier, schemaSHA, revision)
	prompt := TextArtifact{Version: PromptVersion, SHA256: model.SHA256(promptRaw), ByteSize: len(promptRaw), Content: string(promptRaw)}
	revision.PromptVersion = prompt.Version
	revision.PromptSHA256 = prompt.SHA256
	requestID := cfg.RunID + ".planner." + cfg.PlanVersionID
	outputIdentity := model.OutputIdentity{RequestID: requestID, TaskID: cfg.TaskID, RunID: cfg.RunID, SourceRevision: cfg.SourceRevision, PromptVersion: prompt.Version, PromptSHA256: prompt.SHA256, ResponseSchemaVersion: OutputSchemaVersion, ResponseSchemaSHA256: schemaSHA}
	req := model.Request{RequestID: requestID, TaskID: cfg.TaskID, RunID: cfg.RunID, SourceRevision: cfg.SourceRevision, Model: cfg.ModelPolicy.Model, ReasoningEffort: cfg.ModelPolicy.ReasoningEffort, MaxOutputTokens: cfg.ModelPolicy.MaxOutputTokens, PromptVersion: prompt.Version, PromptSHA256: prompt.SHA256, Prompt: prompt.Content, ResponseSchemaVersion: OutputSchemaVersion, ResponseSchemaSHA256: schemaSHA, ResponseSchemaName: OutputSchemaName, ResponseSchema: schemaRaw, Timeout: cfg.ModelPolicy.Timeout, MaxStreamBytes: cfg.ModelPolicy.MaxStreamBytes, MaxDiagnosticBytes: cfg.ModelPolicy.MaxDiagnosticBytes, Retry: cfg.ModelPolicy.Retry}
	preparedRequest, err := model.Prepare(req)
	if err != nil {
		return Prepared{}, fmt.Errorf("prepare task-013 planner request: %w", err)
	}
	expectedRequest, err := requestEvidence(req, outputIdentity)
	if err != nil {
		return Prepared{}, err
	}
	return Prepared{State: state, Dossier: dossier, Prompt: prompt, ResponseSchema: JSONArtifact{Version: OutputSchemaVersion, SHA256: schemaSHA, ByteSize: len(schemaRaw), Content: schemaRaw}, ExpectedOutput: outputIdentity, ExpectedRevision: revision, ExpectedRequest: expectedRequest, Request: preparedRequest}, nil
}

func Generate(ctx context.Context, cfg Config) (Candidate, error) {
	if err := validateConfig(cfg); err != nil {
		return Candidate{}, err
	}
	state, err := cfg.StateReader.ReadPlannerState(ctx, cfg.TaskID, cfg.RunID)
	if err != nil {
		return Candidate{}, fmt.Errorf("read canonical planner state: %w", err)
	}
	prepared, err := Prepare(cfg, state)
	if err != nil {
		return Candidate{}, err
	}
	result, invokeErr := cfg.Model.Invoke(ctx, prepared.Request)
	if invokeErr != nil || result.Outcome != model.OutcomeSuccess {
		return Candidate{}, fmt.Errorf("%w: model outcome %q: %v", ErrRejected, result.Outcome, invokeErr)
	}
	if !reflect.DeepEqual(result.Request, prepared.ExpectedRequest) || !result.Usage.Available || result.Service.Model != cfg.ModelPolicy.Model {
		return Candidate{}, fmt.Errorf("%w: exact model provenance is stale", ErrRejected)
	}
	output, err := ParseOutput(result.StructuredOutput)
	if err != nil {
		return Candidate{}, fmt.Errorf("%w: %v", ErrRejected, err)
	}
	if err := validateOutput(output, state, prepared.ExpectedRevision, prepared.ExpectedOutput, dossierEvidence(state)); err != nil {
		return Candidate{}, fmt.Errorf("%w: %v", ErrRejected, err)
	}
	current, err := cfg.StateReader.ReadPlannerState(ctx, cfg.TaskID, cfg.RunID)
	if err != nil {
		return Candidate{}, fmt.Errorf("%w: freshness unavailable: %v", ErrRejected, err)
	}
	observed, dossierErr := BuildDossier(current)
	if dossierErr != nil || observed.SHA256 != prepared.Dossier.SHA256 || !reflect.DeepEqual(current, state) || validatePinnedState(cfg, current) != nil {
		return Candidate{}, fmt.Errorf("%w: stale task, source, supervisor decision, or dossier identity", ErrRejected)
	}
	canonical, _ := json.Marshal(output)
	candidate := Candidate{SchemaVersion: CandidateVersion, PlanID: cfg.PlanID, PlanVersionID: cfg.PlanVersionID, ExpectedPlanAggregateVersion: cfg.ExpectedPlanAggregateVersion, TaskID: cfg.TaskID, TaskVersionID: cfg.TaskVersionID, TaskVersionNumber: cfg.TaskVersionNumber, RunID: cfg.RunID, ProjectID: state.Run.ProjectID, ProjectSourceID: cfg.ProjectSourceID, SourceRevision: cfg.SourceRevision, SourceCommit: cfg.SourceCommit, SourceTree: cfg.SourceTree, SupervisorDecisionID: cfg.Admission.DecisionID, SupervisorDecisionSHA256: *cfg.Admission.Decision.Identity.ContentSHA256, Dossier: prepared.Dossier, Prompt: prepared.Prompt, ResponseSchema: prepared.ResponseSchema, ModelPolicy: cfg.ModelPolicy, HostPolicy: cfg.HostPolicy, ExpectedRequest: prepared.ExpectedRequest, ModelResult: modelProvenance(result), RawOutput: append([]byte(nil), result.StructuredOutput...), CanonicalOutput: canonical, Output: output}
	candidate.CandidateSHA256 = candidateHash(candidate)
	return candidate, nil
}

func validateConfig(cfg Config) error {
	if cfg.StateReader == nil || cfg.Model == nil {
		return errors.New("planner requires a canonical state reader and task-013 model client")
	}
	for label, value := range map[string]string{"task id": cfg.TaskID, "task version id": cfg.TaskVersionID, "run id": cfg.RunID, "project source id": cfg.ProjectSourceID, "plan id": cfg.PlanID, "plan version id": cfg.PlanVersionID} {
		if !token(value) {
			return fmt.Errorf("planner %s is malformed", label)
		}
	}
	if cfg.TaskVersionNumber <= 0 || cfg.ExpectedPlanAggregateVersion < 0 || !validSHA(cfg.SourceRevision) || !validGitOID(cfg.SourceCommit) || !validGitOID(cfg.SourceTree) {
		return errors.New("planner pinned version, aggregate, or source authority is invalid")
	}
	if err := validateModelPolicy(cfg.ModelPolicy, true); err != nil {
		return err
	}
	if cfg.HostPolicy != CurrentHostPolicy() {
		return errors.New("planner host policy identity is stale")
	}
	return validateAdmission(cfg.Admission, cfg.TaskID, cfg.TaskVersionID, cfg.RunID, cfg.SourceRevision)
}

func validatePinnedState(cfg Config, state CanonicalState) error {
	if state.Task.TaskID != cfg.TaskID || state.Task.TaskVersionID != cfg.TaskVersionID || state.Task.VersionNumber != cfg.TaskVersionNumber || state.Run.RunID != cfg.RunID || state.Run.ProjectSourceID != cfg.ProjectSourceID {
		return errors.New("canonical task/version/run/source does not match pinned planner authority")
	}
	if state.Source.Revision != cfg.SourceRevision || state.Source.Commit != cfg.SourceCommit || state.Source.Tree != cfg.SourceTree || state.SupervisorDecisionID != cfg.Admission.DecisionID || state.SupervisorDecisionSHA256 != *cfg.Admission.Decision.Identity.ContentSHA256 {
		return errors.New("canonical source or supervisor decision is stale")
	}
	if state.PriorPlan == nil && cfg.ExpectedPlanAggregateVersion != 0 {
		return errors.New("initial plan requires aggregate version zero")
	}
	if state.PriorPlan != nil && (state.PriorPlan.PlanID != cfg.PlanID || cfg.ExpectedPlanAggregateVersion <= 0) {
		return errors.New("revision does not match the current plan aggregate")
	}
	return nil
}

func buildPrompt(cfg Config, dossier DossierArtifact, schemaSHA string, revision RevisionIdentity) []byte {
	raw, _ := json.Marshal(revision)
	var out strings.Builder
	out.WriteString("# Revolvr versioned planner\n\nOne fresh stateless invocation. Use no tools, prior conversation, environment credentials, or source mutation. Return exactly one closed-schema plan proposal. The host alone accepts plans.\n\n")
	out.WriteString("Map every canonical criterion exactly once in its dossier order. Use only canonical task dependencies and verification. Steps are numbered 1..N, dependencies name earlier steps, and expected paths stay within task paths. Do not emit placeholders or speculative later work. Revisions preserve the exact existing step prefix and require explicit monotonic lineage; completed or skipped work never regresses.\n\n")
	out.WriteString("Prompt version: " + PromptVersion + "\nResponse schema: " + OutputSchemaVersion + "/" + schemaSHA + "\nModel policy: " + cfg.ModelPolicy.Version + "/" + cfg.ModelPolicy.SHA256 + " tools=none fresh=true\nHost policy: " + cfg.HostPolicy.Version + "/" + cfg.HostPolicy.SHA256 + "\nRevision identity (content_sha256 is assigned or checked by the host): " + string(raw) + "\nDossier: " + dossier.Version + "/" + dossier.SHA256 + fmt.Sprintf(" bytes=%d\n\n", dossier.ByteSize) + "## Frozen Section 13.2 dossier\n\n" + string(dossier.Content) + "\n")
	return []byte(out.String())
}

func requestEvidence(req model.Request, output model.OutputIdentity) (model.RequestEvidence, error) {
	retryRaw, err := json.Marshal(req.Retry)
	if err != nil {
		return model.RequestEvidence{}, err
	}
	maxStream := req.MaxStreamBytes
	if maxStream == 0 {
		maxStream = model.DefaultMaxStreamBytes
	}
	maxDiag := req.MaxDiagnosticBytes
	if maxDiag == 0 {
		maxDiag = model.DefaultMaxDiagnosticBytes
	}
	return model.RequestEvidence{RequestID: req.RequestID, TaskID: req.TaskID, RunID: req.RunID, SourceRevision: req.SourceRevision, Model: req.Model, ReasoningEffort: req.ReasoningEffort, MaxOutputTokens: req.MaxOutputTokens, PromptVersion: req.PromptVersion, PromptSHA256: req.PromptSHA256, ResponseSchemaVersion: req.ResponseSchemaVersion, ResponseSchemaSHA256: req.ResponseSchemaSHA256, ResponseSchemaName: req.ResponseSchemaName, Timeout: req.Timeout, MaxStreamBytes: maxStream, MaxDiagnosticBytes: maxDiag, RetryPolicySHA256: model.SHA256(retryRaw), Retry: req.Retry, OutputIdentity: output}, nil
}

func modelProvenance(value model.Result) supervisor.ModelInvocationProvenance {
	return supervisor.ModelInvocationProvenance{Outcome: value.Outcome, Request: value.Request, Attempts: append([]model.AttemptEvidence(nil), value.Attempts...), Diagnostics: append([]model.DiagnosticEvent(nil), value.Diagnostics...), CompletedResponse: append([]byte(nil), value.CompletedResponse...), StructuredOutput: append([]byte(nil), value.StructuredOutput...), Refusal: value.Refusal, Usage: value.Usage, Latency: value.Latency, Service: value.Service, Cost: value.Cost}
}

func candidateHash(value Candidate) string {
	value.CandidateSHA256 = ""
	raw, _ := json.Marshal(value)
	return model.SHA256(raw)
}
