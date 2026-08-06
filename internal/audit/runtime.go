package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"revolvr/internal/model"
)

const ModelPolicyVersion = "revolvr-auditor-model-policy-v1"

type ModelPolicy struct {
	Version            string            `json:"version"`
	SHA256             string            `json:"sha256"`
	Model              string            `json:"model"`
	ReasoningEffort    string            `json:"reasoning_effort"`
	MaxOutputTokens    int               `json:"max_output_tokens"`
	Timeout            time.Duration     `json:"timeout_ns"`
	MaxStreamBytes     int64             `json:"max_stream_bytes"`
	MaxDiagnosticBytes int               `json:"max_diagnostic_bytes"`
	ToolMode           string            `json:"tool_mode"`
	FreshSession       bool              `json:"fresh_session"`
	Retry              model.RetryPolicy `json:"retry"`
}

func PinModelPolicy(modelName, reasoning string, maxTokens int, timeout time.Duration) (ModelPolicy, error) {
	value := ModelPolicy{
		Version: ModelPolicyVersion, Model: modelName, ReasoningEffort: reasoning,
		MaxOutputTokens: maxTokens, Timeout: timeout, ToolMode: "none",
		FreshSession: true, Retry: model.RetryPolicy{MaxAttempts: 1},
	}
	if err := validateModelPolicy(value, false); err != nil {
		return ModelPolicy{}, err
	}
	raw, _ := json.Marshal(modelPolicyMaterial(value))
	value.SHA256 = model.SHA256(raw)
	return value, nil
}

type ModelInvoker interface {
	Invoke(context.Context, model.PreparedRequest) (model.Result, error)
}

type StateReader interface {
	ReadAuditState(context.Context, Identity) (DossierInput, error)
}

type Config struct {
	AuditID                     string
	AuditorInvocationID         string
	SourceMutatingInvocationIDs []string
	Kind                        Kind
	Input                       DossierInput
	ModelPolicy                 ModelPolicy
	Model                       ModelInvoker
	StateReader                 StateReader
}

type Prepared struct {
	Dossier         DossierArtifact
	Prompt          string
	PromptSHA256    string
	Schema          json.RawMessage
	SchemaSHA256    string
	Authority       OutputAuthority
	OutputIdentity  model.OutputIdentity
	Request         model.PreparedRequest
	ExpectedRequest model.RequestEvidence
}

type Candidate struct {
	AuditID                     string
	AuditorInvocationID         string
	SourceMutatingInvocationIDs []string
	Kind                        Kind
	Dossier                     DossierArtifact
	PromptVersion               string
	Prompt                      string
	PromptSHA256                string
	ResponseSchema              json.RawMessage
	ResponseSchemaSHA256        string
	ModelPolicy                 ModelPolicy
	ModelRequest                model.RequestEvidence
	ModelResult                 model.Result
	RawOutput                   json.RawMessage
	CanonicalOutput             json.RawMessage
	Output                      Output
}

func Prepare(cfg Config) (Prepared, error) {
	if err := validateRuntimeConfig(cfg); err != nil {
		return Prepared{}, err
	}
	dossier, err := BuildDossier(cfg.Input, cfg.Kind)
	if err != nil {
		return Prepared{}, err
	}
	schema, err := OutputSchema()
	if err != nil {
		return Prepared{}, err
	}
	schemaSHA := schemaIdentity(schema)
	authority := OutputAuthority{
		AuditID: cfg.AuditID, AuditKind: cfg.Kind, TaskID: cfg.Input.Identity.TaskID,
		TaskVersionID: cfg.Input.Identity.TaskVersionID, RunID: cfg.Input.Identity.RunID,
		SourceRevision: cfg.Input.Source.Revision, SourceCommit: cfg.Input.Source.Commit,
		SourceTree: cfg.Input.Source.Tree, DiffSHA256: cfg.Input.Source.DiffSHA256,
		VerificationRunID:    cfg.Input.Verification.ID,
		DossierSchemaVersion: dossier.SchemaVersion, DossierSHA256: dossier.SHA256,
	}
	prompt := buildPrompt(cfg, dossier, authority, schemaSHA)
	promptSHA := model.SHA256([]byte(prompt))
	requestID := cfg.AuditorInvocationID + ".auditor"
	identity := model.OutputIdentity{
		RequestID: requestID, TaskID: cfg.Input.Identity.TaskID,
		RunID: cfg.AuditorInvocationID, SourceRevision: cfg.Input.Source.Revision,
		PromptVersion: PromptVersion, PromptSHA256: promptSHA,
		ResponseSchemaVersion: OutputSchemaVersion, ResponseSchemaSHA256: schemaSHA,
	}
	req := model.Request{
		RequestID: requestID, TaskID: cfg.Input.Identity.TaskID, RunID: cfg.AuditorInvocationID,
		SourceRevision: cfg.Input.Source.Revision, Model: cfg.ModelPolicy.Model,
		ReasoningEffort: cfg.ModelPolicy.ReasoningEffort, MaxOutputTokens: cfg.ModelPolicy.MaxOutputTokens,
		PromptVersion: PromptVersion, PromptSHA256: promptSHA, Prompt: prompt,
		ResponseSchemaVersion: OutputSchemaVersion, ResponseSchemaSHA256: schemaSHA,
		ResponseSchemaName: OutputSchemaName, ResponseSchema: schema,
		Timeout: cfg.ModelPolicy.Timeout, MaxStreamBytes: cfg.ModelPolicy.MaxStreamBytes,
		MaxDiagnosticBytes: cfg.ModelPolicy.MaxDiagnosticBytes, Retry: cfg.ModelPolicy.Retry,
	}
	prepared, err := model.Prepare(req)
	if err != nil {
		return Prepared{}, fmt.Errorf("prepare auditor model request: %w", err)
	}
	return Prepared{Dossier: dossier, Prompt: prompt, PromptSHA256: promptSHA, Schema: schema, SchemaSHA256: schemaSHA, Authority: authority, OutputIdentity: identity, Request: prepared, ExpectedRequest: prepared.Evidence()}, nil
}

func Run(ctx context.Context, cfg Config) (Candidate, error) {
	prepared, err := Prepare(cfg)
	if err != nil {
		return Candidate{}, err
	}
	result, invokeErr := cfg.Model.Invoke(ctx, prepared.Request)
	if invokeErr != nil || result.Outcome != model.OutcomeSuccess {
		return Candidate{}, fmt.Errorf("%w: auditor invocation: %v", ErrRejected, errors.Join(invokeErr, outcomeError(result.Outcome)))
	}
	if !reflect.DeepEqual(result.Request, prepared.ExpectedRequest) {
		return Candidate{}, fmt.Errorf("%w: auditor model request provenance is stale", ErrRejected)
	}
	output, err := ParseOutput(result.StructuredOutput)
	if err != nil {
		return Candidate{}, err
	}
	if err := ValidateOutput(output, prepared.Dossier, prepared.Authority, prepared.OutputIdentity); err != nil {
		return Candidate{}, err
	}
	current, err := cfg.StateReader.ReadAuditState(ctx, cfg.Input.Identity)
	if err != nil {
		return Candidate{}, fmt.Errorf("%w: audit freshness unavailable: %v", ErrRejected, err)
	}
	currentDossier, dossierErr := BuildDossier(current, cfg.Kind)
	if dossierErr != nil || currentDossier.SHA256 != prepared.Dossier.SHA256 || !reflect.DeepEqual(current, cfg.Input) {
		return Candidate{}, fmt.Errorf("%w: source, task, plan, verification, or dossier changed during audit", ErrStaleAuthority)
	}
	canonical, _ := json.Marshal(output)
	return Candidate{
		AuditID: cfg.AuditID, AuditorInvocationID: cfg.AuditorInvocationID, Kind: cfg.Kind,
		SourceMutatingInvocationIDs: append([]string(nil), cfg.SourceMutatingInvocationIDs...),
		Dossier:                     prepared.Dossier, PromptVersion: PromptVersion,
		Prompt: prepared.Prompt, PromptSHA256: prepared.PromptSHA256,
		ResponseSchema: append(json.RawMessage(nil), prepared.Schema...), ResponseSchemaSHA256: prepared.SchemaSHA256,
		ModelPolicy: cfg.ModelPolicy, ModelRequest: result.Request, ModelResult: result,
		RawOutput: append(json.RawMessage(nil), result.StructuredOutput...), CanonicalOutput: canonical, Output: output,
	}, nil
}

func validateRuntimeConfig(cfg Config) error {
	if cfg.Model == nil || cfg.StateReader == nil {
		return errors.New("audit runtime requires model and canonical state reader")
	}
	if !token(cfg.AuditID) || !token(cfg.AuditorInvocationID) || cfg.AuditorInvocationID == cfg.Input.Identity.RunID || cfg.AuditorInvocationID == cfg.Input.Verification.ID || len(cfg.SourceMutatingInvocationIDs) == 0 {
		return errors.New("audit and independent auditor invocation identities are missing or conflicting")
	}
	seenMutators := map[string]struct{}{}
	for _, invocationID := range cfg.SourceMutatingInvocationIDs {
		if !token(invocationID) || invocationID == cfg.AuditorInvocationID {
			return errors.New("auditor invocation conflicts with source-mutating invocation authority")
		}
		if _, duplicate := seenMutators[invocationID]; duplicate {
			return errors.New("source-mutating invocation authority contains a duplicate")
		}
		seenMutators[invocationID] = struct{}{}
	}
	if !validKind(cfg.Kind) {
		return fmt.Errorf("unknown audit kind %q", cfg.Kind)
	}
	if err := validateModelPolicy(cfg.ModelPolicy, true); err != nil {
		return err
	}
	return validateDossierInput(cfg.Input)
}

func validateModelPolicy(value ModelPolicy, hashRequired bool) error {
	if value.Version != ModelPolicyVersion || !token(value.Model) || !token(value.ReasoningEffort) || value.MaxOutputTokens <= 0 || value.Timeout <= 0 {
		return errors.New("auditor model policy identity or limits are invalid")
	}
	if value.ToolMode != "none" || !value.FreshSession || value.Retry.MaxAttempts != 1 {
		return errors.New("auditor must use one fresh tool-free model attempt")
	}
	if hashRequired {
		raw, _ := json.Marshal(modelPolicyMaterial(value))
		if value.SHA256 != model.SHA256(raw) {
			return errors.New("auditor model policy hash is stale")
		}
	}
	return nil
}

func modelPolicyMaterial(value ModelPolicy) ModelPolicy { value.SHA256 = ""; return value }

func buildPrompt(cfg Config, dossier DossierArtifact, authority OutputAuthority, schemaSHA string) string {
	authorityRaw, _ := json.Marshal(authority)
	var out strings.Builder
	out.WriteString("# Revolvr independent auditor\n\n")
	out.WriteString("One fresh stateless read-only invocation. You are distinct from every source-mutating worker. Use no tools, source mutation, prior conversation, environment credentials, waiver authority, or lifecycle authority. Return exactly one closed-schema result: clean, changes_required, or blocked. Every finding needs a stable ID, significance, bounded correction, exact dossier source citation, affected files/symbols, and criterion impact. Do not invent evidence.\n\n")
	out.WriteString("Prompt version: " + PromptVersion + "\n")
	out.WriteString("Response schema: " + OutputSchemaVersion + "/" + schemaSHA + "\n")
	out.WriteString("Model policy: " + cfg.ModelPolicy.Version + "/" + cfg.ModelPolicy.SHA256 + " tools=none fresh=true\n")
	mutators, _ := json.Marshal(cfg.SourceMutatingInvocationIDs)
	out.WriteString("Distinct from source-mutating invocations: " + string(mutators) + "\n")
	out.WriteString("Expected authority: " + string(authorityRaw) + "\n")
	out.WriteString("Dossier: " + dossier.SchemaVersion + "/" + dossier.SHA256 + fmt.Sprintf(" bytes=%d\n\n", dossier.ByteSize))
	out.WriteString("## Frozen Section 13.4 dossier\n\n")
	out.Write(dossier.Content)
	out.WriteByte('\n')
	return out.String()
}

func outcomeError(outcome model.Outcome) error {
	if outcome == "" {
		return nil
	}
	return fmt.Errorf("model outcome %s", outcome)
}
