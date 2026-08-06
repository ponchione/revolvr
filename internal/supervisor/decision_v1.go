package supervisor

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"time"

	"revolvr/internal/model"
	"revolvr/internal/policy"
)

const (
	SupervisorDecisionVersion    = "revolvr-supervisor-decision-v1"
	SupervisorDecisionSchemaName = "revolvr_supervisor_decision_v1"
	SupervisorPromptVersion      = "revolvr-supervisor-prompt-v1"
	SupervisorModelPolicyVersion = "revolvr-supervisor-model-policy-v1"
)

type ModelPolicySettings struct {
	Model              string
	ReasoningEffort    string
	MaxOutputTokens    int
	Timeout            time.Duration
	MaxStreamBytes     int64
	MaxDiagnosticBytes int
	Retry              model.RetryPolicy
}

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
	Retry              model.RetryPolicy `json:"retry"`
}

type modelPolicyMaterial struct {
	Version            string            `json:"version"`
	Model              string            `json:"model"`
	ReasoningEffort    string            `json:"reasoning_effort"`
	MaxOutputTokens    int               `json:"max_output_tokens"`
	Timeout            time.Duration     `json:"timeout_ns"`
	MaxStreamBytes     int64             `json:"max_stream_bytes"`
	MaxDiagnosticBytes int               `json:"max_diagnostic_bytes"`
	ToolMode           string            `json:"tool_mode"`
	Retry              model.RetryPolicy `json:"retry"`
}

func PinSupervisorModelPolicy(settings ModelPolicySettings) (ModelPolicy, error) {
	settings.Retry.RetryableStatusCodes = append([]int(nil), settings.Retry.RetryableStatusCodes...)
	settings.Retry.RetryableStreamErrCodes = append([]string(nil), settings.Retry.RetryableStreamErrCodes...)
	sort.Ints(settings.Retry.RetryableStatusCodes)
	sort.Strings(settings.Retry.RetryableStreamErrCodes)
	policy := ModelPolicy{
		Version: SupervisorModelPolicyVersion, Model: settings.Model, ReasoningEffort: settings.ReasoningEffort,
		MaxOutputTokens: settings.MaxOutputTokens, Timeout: settings.Timeout, MaxStreamBytes: settings.MaxStreamBytes,
		MaxDiagnosticBytes: settings.MaxDiagnosticBytes, ToolMode: "none", Retry: settings.Retry,
	}
	if err := validateModelPolicy(policy, false); err != nil {
		return ModelPolicy{}, err
	}
	raw, err := json.Marshal(policy.material())
	if err != nil {
		return ModelPolicy{}, fmt.Errorf("pin supervisor model policy: %w", err)
	}
	policy.SHA256 = model.SHA256(raw)
	return policy, nil
}

func validateModelPolicy(value ModelPolicy, requireHash bool) error {
	if value.Version != SupervisorModelPolicyVersion || !stableToken(value.Model) || !stableToken(value.ReasoningEffort) {
		return errors.New("supervisor model policy has an invalid version, model, or reasoning effort")
	}
	if value.MaxOutputTokens <= 0 || value.Timeout <= 0 || value.MaxStreamBytes < 0 || value.MaxDiagnosticBytes < 0 {
		return errors.New("supervisor model policy limits must be bounded and positive where required")
	}
	if value.ToolMode != "none" {
		return errors.New("supervisor model policy must be tool-free")
	}
	if value.Retry.MaxAttempts != 1 {
		return errors.New("supervisor model policy permits exactly one fresh Responses API attempt")
	}
	if requireHash {
		raw, err := json.Marshal(value.material())
		if err != nil || value.SHA256 != model.SHA256(raw) {
			return errors.New("supervisor model policy SHA-256 does not match its exact material")
		}
	}
	return nil
}

func (value ModelPolicy) material() modelPolicyMaterial {
	return modelPolicyMaterial{
		Version: value.Version, Model: value.Model, ReasoningEffort: value.ReasoningEffort,
		MaxOutputTokens: value.MaxOutputTokens, Timeout: value.Timeout, MaxStreamBytes: value.MaxStreamBytes,
		MaxDiagnosticBytes: value.MaxDiagnosticBytes, ToolMode: value.ToolMode,
		Retry: model.RetryPolicy{MaxAttempts: value.Retry.MaxAttempts, BaseDelay: value.Retry.BaseDelay, MaxDelay: value.Retry.MaxDelay, RetryTransportErrors: value.Retry.RetryTransportErrors, RetryableStatusCodes: append([]int(nil), value.Retry.RetryableStatusCodes...), RetryableStreamErrCodes: append([]string(nil), value.Retry.RetryableStreamErrCodes...)},
	}
}

type DecisionIdentity struct {
	DecisionID         string  `json:"decision_id"`
	TaskVersionID      string  `json:"task_version_id"`
	TaskVersion        int64   `json:"task_version"`
	DossierVersion     string  `json:"dossier_version"`
	DossierSHA256      string  `json:"dossier_sha256"`
	ModelPolicyVersion string  `json:"model_policy_version"`
	ModelPolicySHA256  string  `json:"model_policy_sha256"`
	HostPolicyVersion  string  `json:"host_policy_version"`
	HostPolicySHA256   string  `json:"host_policy_sha256"`
	ContentSHA256      *string `json:"content_sha256"`
}

type DecisionStrategy struct {
	Approach   string   `json:"approach"`
	Techniques []string `json:"techniques"`
	Targets    []string `json:"targets"`
}

type BlockAdvice struct {
	Reason               string   `json:"reason"`
	EvidenceRefs         []string `json:"evidence_refs"`
	ChildTasksSuggested  bool     `json:"child_tasks_suggested"`
	OtherQueueWorkMayRun bool     `json:"other_queue_work_may_run"`
}

type QuestionOption struct {
	ID      string `json:"id"`
	Meaning string `json:"meaning"`
}

type NeedsInputAdvice struct {
	QuestionID     string           `json:"question_id"`
	Question       string           `json:"question"`
	BlockingReason string           `json:"blocking_reason"`
	Options        []QuestionOption `json:"options"`
	EvidenceRefs   []string         `json:"evidence_refs"`
}

type Decision struct {
	RevolvrIdentity model.OutputIdentity        `json:"revolvr_identity"`
	SchemaVersion   string                      `json:"schema_version"`
	Identity        DecisionIdentity            `json:"decision_identity"`
	Action          policy.Action               `json:"action"`
	Rationale       string                      `json:"rationale"`
	EvidenceRefs    []string                    `json:"evidence_refs"`
	Scope           []string                    `json:"scope"`
	Strategy        *DecisionStrategy           `json:"strategy"`
	Correction      *policy.CorrectionAuthority `json:"correction"`
	Completion      *policy.CompletionEvidence  `json:"completion"`
	Block           *BlockAdvice                `json:"block"`
	NeedsInput      *NeedsInputAdvice           `json:"needs_input"`
}

func DecisionContentSHA256(decision Decision) (string, error) {
	decision.Identity.ContentSHA256 = nil
	raw, err := json.Marshal(decision)
	if err != nil {
		return "", fmt.Errorf("hash supervisor decision: %w", err)
	}
	return model.SHA256(raw), nil
}

func ParseStructuredDecision(raw []byte) (Decision, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return Decision{}, errors.New("structured supervisor output is missing")
	}
	if err := rejectDuplicateJSONFields(raw); err != nil {
		return Decision{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var decision Decision
	if err := decoder.Decode(&decision); err != nil {
		return Decision{}, fmt.Errorf("decode exactly one supervisor decision: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return Decision{}, errors.New("multiple supervisor decisions are forbidden")
		}
		return Decision{}, fmt.Errorf("content follows supervisor decision: %w", err)
	}
	if err := validateDecisionShape(decision); err != nil {
		return Decision{}, err
	}
	want, err := DecisionContentSHA256(decision)
	if err != nil {
		return Decision{}, err
	}
	if decision.Identity.ContentSHA256 != nil && *decision.Identity.ContentSHA256 != want {
		return Decision{}, errors.New("supervisor decision content SHA-256 is stale or malformed")
	}
	decision.Identity.ContentSHA256 = &want
	return decision, nil
}

func validateDecisionShape(decision Decision) error {
	if decision.SchemaVersion != SupervisorDecisionVersion || !stableToken(decision.Identity.DecisionID) || strings.TrimSpace(decision.Rationale) == "" {
		return errors.New("supervisor decision version, identity, or rationale is invalid")
	}
	if !slices.Contains([]policy.Action{policy.ActionPlan, policy.ActionImplement, policy.ActionCorrect, policy.ActionDocument, policy.ActionSimplify, policy.ActionComplete, policy.ActionBlock, policy.ActionNeedsInput}, decision.Action) {
		return fmt.Errorf("unknown supervisor action %q", decision.Action)
	}
	if len(decision.EvidenceRefs) == 0 || duplicateOrBlank(decision.EvidenceRefs) {
		return errors.New("supervisor decision evidence references must be nonempty, normalized, and unique")
	}
	if duplicateOrBlank(decision.Scope) {
		return errors.New("supervisor decision scope must be normalized and unique")
	}
	if decision.Strategy != nil {
		if strings.TrimSpace(decision.Strategy.Approach) == "" || decision.Strategy.Techniques == nil || decision.Strategy.Targets == nil || duplicateOrBlankText(decision.Strategy.Techniques) || duplicateOrBlank(decision.Strategy.Targets) {
			return errors.New("supervisor strategy is malformed")
		}
	}
	workerAction := decision.Action == policy.ActionPlan || decision.Action == policy.ActionImplement || decision.Action == policy.ActionCorrect || decision.Action == policy.ActionDocument || decision.Action == policy.ActionSimplify
	if workerAction != (decision.Strategy != nil) {
		return fmt.Errorf("action %q has incompatible strategy presence", decision.Action)
	}
	if decision.Action == policy.ActionPlan && (len(decision.Scope) != 0 || len(decision.Strategy.Targets) != 0) {
		return errors.New("plan forbids source scope and strategy targets")
	}
	if decision.Action != policy.ActionPlan && workerAction && (len(decision.Scope) == 0 || !slices.Equal(decision.Scope, decision.Strategy.Targets)) {
		return fmt.Errorf("action %q requires strategy targets to exactly match bounded scope", decision.Action)
	}
	if (decision.Action == policy.ActionCorrect) != (decision.Correction != nil) || (decision.Action == policy.ActionComplete) != (decision.Completion != nil) || (decision.Action == policy.ActionBlock) != (decision.Block != nil) || (decision.Action == policy.ActionNeedsInput) != (decision.NeedsInput != nil) {
		return fmt.Errorf("action %q has incompatible action-specific fields", decision.Action)
	}
	if decision.Block != nil && (strings.TrimSpace(decision.Block.Reason) == "" || len(decision.Block.EvidenceRefs) == 0 || duplicateOrBlank(decision.Block.EvidenceRefs)) {
		return errors.New("block advisory is malformed")
	}
	if decision.NeedsInput != nil {
		if !stableToken(decision.NeedsInput.QuestionID) || strings.TrimSpace(decision.NeedsInput.Question) == "" || strings.TrimSpace(decision.NeedsInput.BlockingReason) == "" || len(decision.NeedsInput.Options) < 2 || len(decision.NeedsInput.EvidenceRefs) == 0 || duplicateOrBlank(decision.NeedsInput.EvidenceRefs) {
			return errors.New("needs_input advisory is malformed")
		}
		optionIDs := make([]string, len(decision.NeedsInput.Options))
		for i, option := range decision.NeedsInput.Options {
			optionIDs[i] = option.ID
			if strings.TrimSpace(option.Meaning) == "" {
				return errors.New("needs_input option meaning is required")
			}
		}
		if duplicateOrBlank(optionIDs) {
			return errors.New("needs_input option identities must be unique")
		}
	}
	return nil
}

func duplicateOrBlank(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !stableToken(value) {
			return true
		}
		if _, duplicate := seen[value]; duplicate {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
}

func duplicateOrBlankText(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value {
			return true
		}
		if _, duplicate := seen[value]; duplicate {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
}

func rejectDuplicateJSONFields(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var walk func() error
	walk = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]struct{}{}
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return errors.New("supervisor output contains a non-string object key")
				}
				if _, duplicate := seen[key]; duplicate {
					return fmt.Errorf("supervisor output contains duplicate field %q", key)
				}
				seen[key] = struct{}{}
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		case '[':
			for decoder.More() {
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		default:
			return errors.New("supervisor output contains an unexpected JSON delimiter")
		}
	}
	if err := walk(); err != nil {
		return fmt.Errorf("validate supervisor output object fields: %w", err)
	}
	return nil
}

func DecisionOutputSchemaV1() ([]byte, error) {
	nonblank := map[string]any{"type": "string", "pattern": `.*\S.*`}
	stable := map[string]any{"type": "string", "pattern": `^[^\s]+$`}
	sha := map[string]any{"type": "string", "pattern": `^[a-f0-9]{64}$`}
	stringArray := func(min int) map[string]any { return map[string]any{"type": "array", "minItems": min, "items": stable} }
	nullable := func(ref string) map[string]any {
		return map[string]any{"anyOf": []any{map[string]any{"$ref": ref}, map[string]any{"type": "null"}}}
	}
	object := func(required []string, properties map[string]any) map[string]any {
		return map[string]any{"type": "object", "additionalProperties": false, "required": required, "properties": properties}
	}

	schema := object(
		[]string{"revolvr_identity", "schema_version", "decision_identity", "action", "rationale", "evidence_refs", "scope", "strategy", "correction", "completion", "block", "needs_input"},
		map[string]any{
			"revolvr_identity":  map[string]any{"$ref": "#/$defs/revolvr_identity"},
			"schema_version":    map[string]any{"type": "string", "enum": []string{SupervisorDecisionVersion}},
			"decision_identity": map[string]any{"$ref": "#/$defs/decision_identity"},
			"action":            map[string]any{"type": "string", "enum": []string{"plan", "implement", "correct", "document", "simplify", "complete", "block", "needs_input"}},
			"rationale":         nonblank, "evidence_refs": stringArray(1), "scope": stringArray(0),
			"strategy": nullable("#/$defs/strategy"), "correction": nullable("#/$defs/correction"),
			"completion": nullable("#/$defs/completion"), "block": nullable("#/$defs/block"), "needs_input": nullable("#/$defs/needs_input"),
		},
	)
	schema["$defs"] = map[string]any{
		"revolvr_identity": object([]string{"request_id", "task_id", "run_id", "source_revision", "prompt_version", "prompt_sha256", "response_schema_version", "response_schema_sha256"}, map[string]any{
			"request_id": stable, "task_id": stable, "run_id": stable, "source_revision": stable, "prompt_version": stable, "prompt_sha256": sha, "response_schema_version": stable, "response_schema_sha256": sha,
		}),
		"decision_identity": object([]string{"decision_id", "task_version_id", "task_version", "dossier_version", "dossier_sha256", "model_policy_version", "model_policy_sha256", "host_policy_version", "host_policy_sha256", "content_sha256"}, map[string]any{
			"decision_id": stable, "task_version_id": stable, "task_version": map[string]any{"type": "integer", "minimum": 1}, "dossier_version": stable, "dossier_sha256": sha,
			"model_policy_version": stable, "model_policy_sha256": sha, "host_policy_version": stable, "host_policy_sha256": sha, "content_sha256": map[string]any{"type": []string{"string", "null"}, "pattern": `^[a-f0-9]{64}$`},
		}),
		"strategy":        object([]string{"approach", "techniques", "targets"}, map[string]any{"approach": nonblank, "techniques": stringArray(0), "targets": stringArray(0)}),
		"correction":      object([]string{"kind", "finding_ids"}, map[string]any{"kind": map[string]any{"type": "string", "enum": []string{"verification_failure", "audit_findings"}}, "finding_ids": stringArray(0)}),
		"completion":      object([]string{"plan_id", "criterion_ids", "verification_id", "audit_id", "artifact_manifest_id"}, map[string]any{"plan_id": stable, "criterion_ids": stringArray(1), "verification_id": stable, "audit_id": stable, "artifact_manifest_id": stable}),
		"block":           object([]string{"reason", "evidence_refs", "child_tasks_suggested", "other_queue_work_may_run"}, map[string]any{"reason": nonblank, "evidence_refs": stringArray(1), "child_tasks_suggested": map[string]any{"type": "boolean"}, "other_queue_work_may_run": map[string]any{"type": "boolean"}}),
		"question_option": object([]string{"id", "meaning"}, map[string]any{"id": stable, "meaning": nonblank}),
		"needs_input":     object([]string{"question_id", "question", "blocking_reason", "options", "evidence_refs"}, map[string]any{"question_id": stable, "question": nonblank, "blocking_reason": nonblank, "options": map[string]any{"type": "array", "minItems": 2, "items": map[string]any{"$ref": "#/$defs/question_option"}}, "evidence_refs": stringArray(1)}),
	}
	return json.Marshal(schema)
}
