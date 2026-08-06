package implementer

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"revolvr/internal/gitoid"
	"revolvr/internal/gitstate"
	"revolvr/internal/model"
	"revolvr/internal/planner"
	"revolvr/internal/runtimepath"
	"revolvr/internal/tool"
)

var (
	ErrIndeterminateReplay = errors.New("implementer invocation has prior intent without a terminal result")
	ErrReplaySourceDrift   = errors.New("implementer replay source differs from the terminal host observation")
)

const (
	maximumActiveStepBytes      = 2 << 20
	maximumModelArtifactBytes   = 32 << 20
	maximumModelToolOutputBytes = 64 << 10
)

func PinModelPolicy(modelName string, maximumIterations, maximumToolCalls, maximumFinalBytes int) (ModelPolicy, error) {
	value := ModelPolicy{
		Version: ModelPolicyVersion, Model: modelName, MaximumIterations: maximumIterations,
		MaximumToolCalls: maximumToolCalls, MaximumFinalBytes: maximumFinalBytes,
		FreshSession: true, HiddenState: false,
	}
	if err := validateModelPolicy(value, false); err != nil {
		return ModelPolicy{}, err
	}
	raw, _ := json.Marshal(modelPolicyMaterial(value))
	value.SHA256 = model.SHA256(raw)
	return value, nil
}

func Run(ctx context.Context, cfg Config) (Result, error) {
	prepared, err := prepare(cfg)
	if err != nil {
		return Result{}, err
	}
	store, err := newRunStore(cfg.EvidenceRoot)
	if err != nil {
		return Result{}, err
	}
	intentRaw, _ := json.Marshal(struct {
		SchemaVersion string      `json:"schema_version"`
		InvocationID  string      `json:"invocation_id"`
		Admission     Admission   `json:"admission"`
		ModelPolicy   ModelPolicy `json:"model_policy"`
		PromptSHA256  string      `json:"prompt_sha256"`
	}{InvocationVersion, cfg.Admission.RunID + ".implementer", cloneAdmission(cfg.Admission), cfg.ModelPolicy, prepared.promptSHA})
	intentRaw = append(intentRaw, '\n')
	disposition, replay, err := store.begin(intentRaw)
	if err != nil {
		return Result{}, err
	}
	switch disposition {
	case "replay":
		if replay.Before.SourceRevision != "" {
			current, captureErr := cfg.Observer.Capture(ctx, cfg.Admission.WorkspaceRoot)
			if captureErr != nil {
				return Result{}, fmt.Errorf("validate implementer replay source: %w", captureErr)
			}
			if !reflect.DeepEqual(current.SourceSnapshot, replay.After.SourceSnapshot) || current.DiffSHA256 != replay.After.DiffSHA256 || !reflect.DeepEqual(current.ChangedManifest, replay.After.ChangedManifest) {
				return Result{}, ErrReplaySourceDrift
			}
		}
		return *replay, nil
	case "conflict":
		return Result{}, errors.New("implementer replay authority conflicts with existing exact intent")
	case "indeterminate":
		return Result{}, ErrIndeterminateReplay
	}

	result := Result{
		SchemaVersion: EvidenceVersion, InvocationID: cfg.Admission.RunID + ".implementer",
		Disposition: "failed", Admission: cloneAdmission(cfg.Admission), PromptVersion: PromptVersion,
		PromptSHA256: prepared.promptSHA, SummarySchemaVersion: SummarySchemaVersion,
		SummarySchemaSHA256: prepared.schemaSHA, ModelPolicy: cfg.ModelPolicy,
	}
	finish := func(runErr error) (Result, error) {
		if runErr != nil {
			result.Error = bounded(runErr)
		}
		captureCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if result.Before.SourceRevision != "" {
			after, captureErr := cfg.Observer.Capture(captureCtx, cfg.Admission.WorkspaceRoot)
			if captureErr == nil {
				result.After = after
			} else {
				runErr = errors.Join(runErr, fmt.Errorf("capture final workspace evidence: %w", captureErr))
				result.Error = bounded(runErr)
			}
		}
		if sourceErr := persistSourceEvidence(store, &result); sourceErr != nil {
			runErr = errors.Join(runErr, sourceErr)
			result.Error = bounded(runErr)
		}
		result.Signals = reconcile(cfg.Admission, result.Summary, result.ToolExecutions, result.Before, result.After, result.Disposition)
		if err := store.complete(result); err != nil {
			return result, errors.Join(runErr, err)
		}
		return result, runErr
	}

	before, err := cfg.Observer.Capture(ctx, cfg.Admission.WorkspaceRoot)
	if err != nil {
		return finish(fmt.Errorf("capture admitted workspace: %w", err))
	}
	result.Before = before
	if before.SourceRevision != cfg.Admission.SourceRevision || before.HeadCommit != cfg.Admission.SourceCommit || before.HeadTree != cfg.Admission.SourceTree || len(before.ChangedManifest) != 0 {
		return finish(errors.New("admitted workspace source identity is stale or initially dirty"))
	}

	history := []HistoryItem{}
	toolCalls := 0
	for iteration := 1; iteration <= cfg.ModelPolicy.MaximumIterations; iteration++ {
		if err := ctx.Err(); err != nil {
			result.Disposition = "cancelled"
			result.Cancellation = cfg.Broker.Cancel()
			return finish(err)
		}
		request := ModelRequest{
			SchemaVersion: InvocationVersion, InvocationID: result.InvocationID, Iteration: iteration,
			FreshSession: true, PromptVersion: PromptVersion, PromptSHA256: prepared.promptSHA,
			Prompt: prepared.prompt, SummarySchema: append(json.RawMessage(nil), prepared.schema...),
			Registry: cfg.Broker.Registry(), Authority: cfg.Broker.Authority(), History: cloneHistory(history),
		}
		requestRaw, _ := json.Marshal(request)
		if len(requestRaw) > maximumModelArtifactBytes {
			return finish(errors.New("implementer model request exceeds the invocation artifact cap"))
		}
		requestArtifact, writeErr := store.write(fmt.Sprintf("model-%03d-request.json", iteration), append(requestRaw, '\n'))
		if writeErr != nil {
			return finish(writeErr)
		}
		started := now(cfg.Clock)
		turn, modelErr := cfg.Model.Next(ctx, request)
		finished := now(cfg.Clock)
		turnRaw, _ := json.Marshal(turn)
		if len(turnRaw) > maximumModelArtifactBytes {
			return finish(errors.New("implementer model response exceeds the invocation artifact cap"))
		}
		responseArtifact, responseErr := store.write(fmt.Sprintf("model-%03d-response.json", iteration), append(turnRaw, '\n'))
		iterationEvidence := ModelIterationEvidence{Iteration: iteration, StartedAt: started, FinishedAt: finished, Duration: finished.Sub(started), Request: requestArtifact, Response: responseArtifact, Usage: turn.Usage}
		if modelErr != nil {
			iterationEvidence.Error = bounded(modelErr)
		}
		result.ModelIterations = append(result.ModelIterations, iterationEvidence)
		if responseErr != nil {
			return finish(responseErr)
		}
		if modelErr != nil {
			if errors.Is(ctx.Err(), context.Canceled) || errors.Is(modelErr, context.Canceled) {
				result.Disposition = "cancelled"
				result.Cancellation = cfg.Broker.Cancel()
			}
			return finish(modelErr)
		}
		if err := validateTurn(turn); err != nil {
			return finish(err)
		}
		if turn.Refusal != "" {
			result.Disposition = "refused"
			return finish(errors.New("implementer model refusal: " + boundedString(turn.Refusal)))
		}
		if len(turn.FinalOutput) != 0 {
			if len(turn.FinalOutput) > cfg.ModelPolicy.MaximumFinalBytes {
				return finish(errors.New("implementer final output exceeds the admitted byte cap"))
			}
			summaryArtifact, writeErr := store.write("summary.json", append(append([]byte(nil), turn.FinalOutput...), '\n'))
			if writeErr != nil {
				return finish(writeErr)
			}
			result.RawSummary = summaryArtifact
			summary, parseErr := parseSummary(turn.FinalOutput, prepared.summaryIdentity, cfg.Admission, result.ToolExecutions)
			if parseErr != nil {
				return finish(parseErr)
			}
			result.Summary = &summary
			result.Disposition = "completed"
			if summary.Partial {
				result.Disposition = "partial"
			}
			return finish(nil)
		}
		for _, rawCall := range turn.ToolCalls {
			toolCalls++
			if toolCalls > cfg.ModelPolicy.MaximumToolCalls {
				return finish(errors.New("implementer tool-call budget exhausted"))
			}
			outcome, dispatchErr := cfg.Broker.Dispatch(ctx, rawCall)
			result.ToolExecutions = append(result.ToolExecutions, outcome.Evidence)
			copyOutcome := modelVisibleOutcome(outcome)
			history = append(history, HistoryItem{Kind: "tool", Iteration: iteration, ToolCall: append(json.RawMessage(nil), rawCall...), ToolOutcome: &copyOutcome})
			if dispatchErr != nil {
				if errors.Is(ctx.Err(), context.Canceled) {
					result.Disposition = "cancelled"
					result.Cancellation = outcome.Evidence.Cancellation
				}
				return finish(dispatchErr)
			}
		}
	}
	result.Disposition = "budget_exhausted"
	return finish(errors.New("implementer iteration budget exhausted"))
}

type preparedInvocation struct {
	prompt          string
	promptSHA       string
	schema          json.RawMessage
	schemaSHA       string
	summaryIdentity SummaryIdentity
}

func prepare(cfg Config) (preparedInvocation, error) {
	if cfg.Model == nil || cfg.Broker == nil || cfg.Observer == nil {
		return preparedInvocation{}, errors.New("implementer requires model, broker, and host observer boundaries")
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if strings.TrimSpace(cfg.EvidenceRoot) == "" {
		return preparedInvocation{}, errors.New("implementer evidence root is required")
	}
	if err := validateModelPolicy(cfg.ModelPolicy, true); err != nil {
		return preparedInvocation{}, err
	}
	if err := validateAdmission(cfg.Admission, cfg.Broker, cfg.ModelPolicy); err != nil {
		return preparedInvocation{}, err
	}
	schema, err := SummarySchema()
	if err != nil {
		return preparedInvocation{}, err
	}
	schemaSHA := model.SHA256(schema)
	prompt := buildPrompt(cfg.Admission, cfg.ModelPolicy, cfg.Broker.Registry(), schemaSHA)
	promptSHA := model.SHA256([]byte(prompt))
	identity := expectedSummaryIdentity(cfg.Admission, cfg.ModelPolicy, promptSHA, schemaSHA)
	return preparedInvocation{prompt: prompt, promptSHA: promptSHA, schema: schema, schemaSHA: schemaSHA, summaryIdentity: identity}, nil
}

func validateAdmission(value Admission, broker *tool.Broker, modelPolicy ModelPolicy) error {
	if value.SchemaVersion != AdmissionSchemaVersion || !value.Accepted || !value.PlanAccepted || !token(value.AcceptanceID) || !token(value.PlanAcceptanceID) {
		return errors.New("implementer requires an accepted run and accepted plan-step authority")
	}
	for label, candidate := range map[string]string{
		"project": value.ProjectID, "task": value.TaskID, "task version": value.TaskVersionID, "run": value.RunID,
		"project source": value.ProjectSourceID, "plan": value.PlanID, "plan version": value.PlanVersionID,
		"workspace": value.WorkspaceID, "sandbox": value.SandboxID,
	} {
		if !token(candidate) {
			return fmt.Errorf("implementer %s identity is malformed", label)
		}
	}
	if !validSHA(value.SourceRevision) || !gitoid.Valid(value.SourceCommit) || !gitoid.Valid(value.SourceTree) || value.PlanRevision <= 0 {
		return errors.New("implementer source or plan revision identity is malformed")
	}
	if value.WorkspaceStatus != "active" || value.WorkspaceRoot == "" || value.WorkspaceDevice == 0 || value.WorkspaceInode == 0 {
		return errors.New("implementer requires an active identity-pinned managed workspace")
	}
	boundary, err := runtimepath.Bind(value.WorkspaceRoot)
	if err != nil {
		return fmt.Errorf("implementer workspace boundary: %w", err)
	}
	directory, found, err := boundary.OpenDir(boundary.Root(), false)
	if err != nil || !found {
		return errors.Join(err, errors.New("implementer workspace is unavailable"))
	}
	device, inode, identityErr := directory.Identity()
	closeErr := directory.Close()
	if identityErr != nil || closeErr != nil || device != value.WorkspaceDevice || inode != value.WorkspaceInode {
		return errors.Join(identityErr, closeErr, errors.New("implementer workspace filesystem identity is stale"))
	}
	if len(value.ActiveSteps) == 0 || len(value.ActiveSteps) > MaximumBatchSteps {
		return errors.New("implementer requires one active step or a bounded adjacent batch")
	}
	stepIDs := make([]string, len(value.ActiveSteps))
	stepPaths := []string{}
	for i, step := range value.ActiveSteps {
		if !token(step.ID) || step.Ordinal <= 0 || i > 0 && step.Ordinal != value.ActiveSteps[i-1].Ordinal+1 || step.Status != "pending" && step.Status != "in_progress" {
			return errors.New("implementer active plan steps are malformed, non-adjacent, or terminal")
		}
		stepIDs[i] = step.ID
		stepPaths = append(stepPaths, step.ExpectedPaths...)
	}
	stepRaw, err := json.Marshal(value.ActiveSteps)
	if err != nil || len(stepRaw) > maximumActiveStepBytes || value.StepBatchSHA256 != model.SHA256(stepRaw) {
		return errors.Join(err, errors.New("implementer active plan-step batch identity is stale"))
	}
	stepPaths = compactSorted(stepPaths)
	if !slices.Equal(stepPaths, compactSorted(value.ExpectedPaths)) {
		return errors.New("implementer expected paths differ from the exact active plan-step batch")
	}
	authority := broker.Authority()
	want := tool.Authority{
		ProjectID: value.ProjectID, TaskID: value.TaskID, TaskVersionID: value.TaskVersionID, RunID: value.RunID,
		SourceRevision: value.SourceRevision, SourceCommit: value.SourceCommit, SourceTree: value.SourceTree,
		PlanID: value.PlanID, PlanVersionID: value.PlanVersionID, PlanRevision: value.PlanRevision,
		StepBatchSHA256: value.StepBatchSHA256, StepIDs: stepIDs,
		WorkspaceID: value.WorkspaceID, SandboxID: value.SandboxID, SandboxSHA256: value.SandboxSHA256,
		RegistryVersion: value.RegistryVersion, RegistrySHA256: value.RegistrySHA256,
		HostPolicyVersion: value.HostPolicyVersion, HostPolicySHA256: value.HostPolicySHA256,
	}
	if !reflect.DeepEqual(authority, want) {
		return errors.New("implementer run/source/plan/workspace/sandbox/registry/host-policy authority is stale")
	}
	scope := broker.Scope()
	if scope.Role != "implementer" || !slices.Equal(compactSorted(scope.ExpectedPaths), compactSorted(value.ExpectedPaths)) ||
		!slices.Equal(compactSorted(scope.AdjacentPaths), compactSorted(value.AdjacentPaths)) ||
		!slices.Equal(compactSorted(scope.ProtectedPaths), compactSorted(value.ProtectedPaths)) ||
		!slices.Equal(compactSorted(scope.DependencyPaths), compactSorted(value.DependencyPaths)) ||
		!slices.Equal(compactSorted(scope.VerificationAuthorityPaths), compactSorted(value.VerificationPaths)) {
		return errors.New("implementer plan paths differ from the exact role-scoped broker policy")
	}
	if value.ModelPolicyVersion != modelPolicy.Version || value.ModelPolicySHA256 != modelPolicy.SHA256 {
		return errors.New("implementer model policy authority is stale")
	}
	return nil
}

func validateModelPolicy(value ModelPolicy, requireHash bool) error {
	if value.Version != ModelPolicyVersion || !token(value.Model) || value.MaximumIterations <= 0 || value.MaximumIterations > 16 || value.MaximumToolCalls <= 0 || value.MaximumToolCalls > 64 || value.MaximumFinalBytes <= 0 || value.MaximumFinalBytes > 4<<20 || !value.FreshSession || value.HiddenState {
		return errors.New("implementer model policy must be fresh, stateless, and bounded")
	}
	if requireHash {
		raw, _ := json.Marshal(modelPolicyMaterial(value))
		if value.SHA256 != model.SHA256(raw) {
			return errors.New("implementer model policy hash is stale")
		}
	}
	return nil
}

func modelPolicyMaterial(value ModelPolicy) ModelPolicy { value.SHA256 = ""; return value }

func buildPrompt(admission Admission, modelPolicy ModelPolicy, registry tool.Registry, schemaSHA string) string {
	steps, _ := json.Marshal(admission.ActiveSteps)
	return "# Revolvr bounded implementer\n\n" +
		"This is one fresh stateless invocation. Use only the closed brokered tools below. No host paths, credentials, database, runtime socket, raw container controls, hooks, ambient configuration, canonical-state writes, task broadening, verification authority, or lifecycle authority are available. Work only on the exact active accepted plan-step batch. Final claims are advisory and must match host-observed source evidence.\n\n" +
		fmt.Sprintf("Prompt=%s SummarySchema=%s/%s ModelPolicy=%s/%s Registry=%s/%s HostPolicy=%s/%s Sandbox=%s/%s Workspace=%s StepBatch=%s container_path=/workspace\nActiveSteps=%s\nExpectedPaths=%s\nProtectedPaths=%s\n",
			PromptVersion, SummarySchemaVersion, schemaSHA, modelPolicy.Version, modelPolicy.SHA256,
			registry.Version, registry.SHA256, admission.HostPolicyVersion, admission.HostPolicySHA256,
			admission.SandboxID, admission.SandboxSHA256, admission.WorkspaceID, admission.StepBatchSHA256, steps,
			strings.Join(admission.ExpectedPaths, ","), strings.Join(admission.ProtectedPaths, ","))
}

func expectedSummaryIdentity(admission Admission, policy ModelPolicy, promptSHA, schemaSHA string) SummaryIdentity {
	return SummaryIdentity{
		ProjectID: admission.ProjectID, TaskID: admission.TaskID, TaskVersionID: admission.TaskVersionID,
		RunID: admission.RunID, SourceRevision: admission.SourceRevision, SourceCommit: admission.SourceCommit,
		SourceTree: admission.SourceTree, PlanID: admission.PlanID, PlanVersionID: admission.PlanVersionID,
		PlanRevision: admission.PlanRevision, StepBatchSHA256: admission.StepBatchSHA256,
		WorkspaceID: admission.WorkspaceID, SandboxID: admission.SandboxID,
		SandboxSHA256: admission.SandboxSHA256, PromptVersion: PromptVersion, PromptSHA256: promptSHA,
		SummarySchemaVersion: SummarySchemaVersion, SummarySchemaSHA256: schemaSHA,
		RegistryVersion: admission.RegistryVersion, RegistrySHA256: admission.RegistrySHA256,
		HostPolicyVersion: admission.HostPolicyVersion, HostPolicySHA256: admission.HostPolicySHA256,
		ModelPolicyVersion: policy.Version, ModelPolicySHA256: policy.SHA256,
	}
}

func validateTurn(turn ModelTurn) error {
	choices := 0
	if len(turn.ToolCalls) > 0 {
		choices++
	}
	if len(turn.FinalOutput) > 0 {
		choices++
	}
	if turn.Refusal != "" {
		choices++
	}
	if choices != 1 || len(turn.ToolCalls) > 8 || len(turn.Refusal) > 4096 {
		return errors.New("implementer model turn must contain exactly one bounded tool-call batch, final output, or refusal")
	}
	for _, call := range turn.ToolCalls {
		if len(call) == 0 || len(call) > 2<<20 {
			return errors.New("implementer model emitted an empty or oversized tool call")
		}
	}
	usage := turn.Usage
	for _, value := range []int64{usage.InputTokens, usage.CachedTokens, usage.CacheWriteTokens, usage.OutputTokens, usage.ReasoningTokens, usage.TotalTokens} {
		if value < 0 {
			return errors.New("implementer model usage contains a negative value")
		}
	}
	if !usage.Available || usage.CachedTokens > usage.InputTokens || usage.ReasoningTokens > usage.OutputTokens || usage.TotalTokens != usage.InputTokens+usage.OutputTokens {
		return errors.New("implementer model usage is unavailable or inconsistent")
	}
	return nil
}

func modelVisibleOutcome(value tool.Outcome) tool.Outcome {
	value.Evidence.Input.Path = ""
	value.Evidence.Result.Path = ""
	value.Evidence.Stdout.Path = ""
	value.Evidence.Stderr.Path = ""
	value.Evidence.Authority.StepIDs = append([]string(nil), value.Evidence.Authority.StepIDs...)
	value.Evidence.SourceChanges = append([]tool.SourceChange(nil), value.Evidence.SourceChanges...)
	value.Evidence.ResultRepresentation.Artifacts = append([]tool.ArtifactReference(nil), value.Evidence.ResultRepresentation.Artifacts...)
	for index := range value.Evidence.ResultRepresentation.Artifacts {
		value.Evidence.ResultRepresentation.Artifacts[index].Artifact.Path = ""
	}
	value.Stdout, value.Evidence.StdoutTruncatedBytes = modelVisibleOutput(value.Stdout, value.Evidence.StdoutTruncatedBytes)
	value.Stderr, value.Evidence.StderrTruncatedBytes = modelVisibleOutput(value.Stderr, value.Evidence.StderrTruncatedBytes)
	value.Evidence.Truncated = value.Evidence.StdoutTruncatedBytes > 0 || value.Evidence.StderrTruncatedBytes > 0
	return value
}

func modelVisibleOutput(value string, alreadyTruncated int64) (string, int64) {
	if len(value) <= maximumModelToolOutputBytes {
		return value, alreadyTruncated
	}
	dropped := int64(len(value) - maximumModelToolOutputBytes)
	prefix := strings.ToValidUTF8(value[:maximumModelToolOutputBytes], "�")
	return prefix + "\n[model context truncated; use artifact identity for host evidence]\n", alreadyTruncated + dropped
}

func parseSummary(raw []byte, expected SummaryIdentity, admission Admission, executions []tool.Evidence) (Summary, error) {
	if err := rejectDuplicateFields(raw); err != nil {
		return Summary{}, err
	}
	if err := validateSummaryJSON(raw); err != nil {
		return Summary{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var summary Summary
	if err := decoder.Decode(&summary); err != nil {
		return Summary{}, fmt.Errorf("decode closed implementer summary: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Summary{}, errors.New("content follows implementer summary")
	}
	if summary.SchemaVersion != SummarySchemaVersion || summary.Identity != expected {
		return Summary{}, errors.New("implementer summary schema or exact run/source/plan/workspace/sandbox/policy identity is stale")
	}
	if err := boundedText("summary", summary.Summary, true); err != nil {
		return Summary{}, err
	}
	if err := validateTextList("claimed files", summary.ClaimedFiles, false, true); err != nil {
		return Summary{}, err
	}
	if err := validateTextList("concerns", summary.Concerns, false, false); err != nil {
		return Summary{}, err
	}
	if err := validateTextList("follow-up work", summary.CandidateFollowUpWork, false, false); err != nil {
		return Summary{}, err
	}
	executionByID := map[string]tool.Evidence{}
	for _, execution := range executions {
		executionByID[execution.CallID] = execution
	}
	for _, test := range summary.VoluntaryTests {
		if !token(test.ToolCallID) || test.Outcome != "passed" && test.Outcome != "failed" && test.Outcome != "cancelled" || executionByID[test.ToolCallID].Tool != tool.ToolCommand {
			return Summary{}, errors.New("implementer voluntary test does not cite an exact command execution")
		}
	}
	active := map[string]bool{}
	for _, step := range admission.ActiveSteps {
		active[step.ID] = true
	}
	seenSteps := map[string]bool{}
	for _, progress := range summary.CandidatePlanProgress {
		if !active[progress.StepID] || seenSteps[progress.StepID] || progress.Status != "candidate_completed" && progress.Status != "candidate_partial" && progress.Status != "unchanged" {
			return Summary{}, errors.New("implementer candidate plan progress is outside the active accepted batch")
		}
		seenSteps[progress.StepID] = true
		for i, callID := range progress.EvidenceCallIDs {
			if !token(callID) || slices.Contains(progress.EvidenceCallIDs[:i], callID) {
				return Summary{}, errors.New("implementer plan-progress evidence is malformed or duplicated")
			}
			if _, ok := executionByID[callID]; !ok {
				return Summary{}, errors.New("implementer plan progress cites an unknown tool execution")
			}
		}
	}
	if len(seenSteps) != len(active) {
		return Summary{}, errors.New("implementer candidate plan progress does not cover the exact active batch")
	}
	return summary, nil
}

func persistSourceEvidence(store *runStore, result *Result) error {
	if result.Before.SourceRevision == "" {
		return nil
	}
	beforeRaw, _ := json.Marshal(result.Before.SourceSnapshot)
	afterRaw, _ := json.Marshal(result.After.SourceSnapshot)
	manifestRaw, _ := json.Marshal(result.After.ChangedManifest)
	var err error
	if result.Source.BeforeSnapshot, err = store.write("source-before.json", append(beforeRaw, '\n')); err != nil {
		return err
	}
	if result.Source.AfterSnapshot, err = store.write("source-after.json", append(afterRaw, '\n')); err != nil {
		return err
	}
	if result.Source.Status, err = store.write("source-status", result.After.RawStatus); err != nil {
		return err
	}
	if result.Source.Manifest, err = store.write("source-manifest.json", append(manifestRaw, '\n')); err != nil {
		return err
	}
	if result.Source.Diff, err = store.write("source.diff", result.After.Diff); err != nil {
		return err
	}
	result.Source.DiffSHA256 = result.After.DiffSHA256
	if result.Source.Diff.SHA256 != result.After.DiffSHA256 {
		return errors.New("host-observed diff artifact identity is inconsistent")
	}
	return nil
}

func reconcile(admission Admission, summary *Summary, executions []tool.Evidence, before, after WorkspaceObservation, disposition string) []PolicySignal {
	paths := manifestPaths(after.ChangedManifest)
	signals := []PolicySignal{}
	add := func(kind SignalKind, selected []string, detail string) {
		selected = compactSorted(selected)
		for _, existing := range signals {
			if existing.Kind == kind && slices.Equal(existing.Paths, selected) {
				return
			}
		}
		signals = append(signals, PolicySignal{Kind: kind, Paths: selected, Detail: detail})
	}
	if len(paths) == 0 {
		add(SignalNoSourceChange, nil, "host observation found no source change")
	}
	adjacent, unexpected, protected, dependency, verification := []string{}, []string{}, []string{}, []string{}, []string{}
	for _, candidate := range paths {
		if !covered(candidate, admission.ExpectedPaths) && covered(candidate, admission.AdjacentPaths) {
			adjacent = append(adjacent, candidate)
		} else if !covered(candidate, admission.ExpectedPaths) && !covered(candidate, admission.AdjacentPaths) {
			unexpected = append(unexpected, candidate)
		}
		if covered(candidate, admission.ProtectedPaths) || candidate == ".git" || strings.HasPrefix(candidate, ".git/") || candidate == ".revolvr" || strings.HasPrefix(candidate, ".revolvr/") || candidate == ".agent" || strings.HasPrefix(candidate, ".agent/") {
			protected = append(protected, candidate)
		}
		if covered(candidate, admission.DependencyPaths) {
			dependency = append(dependency, candidate)
		}
		if covered(candidate, admission.VerificationPaths) {
			verification = append(verification, candidate)
		}
	}
	for _, execution := range executions {
		if execution.DenialCode == "protected_path" || execution.DenialCode == "secret_or_control_path" {
			protected = append(protected, execution.Tool+":"+execution.CallID)
		}
	}
	if len(adjacent) > 0 {
		add(SignalAdjacentChange, adjacent, "host manifest contains admitted adjacent-scope changes for later policy review")
	}
	if len(unexpected) > 0 {
		add(SignalUnexpectedChange, unexpected, "host manifest contains changes outside expected or adjacent plan scope")
	}
	if len(protected) > 0 {
		add(SignalProtectedChange, protected, "host evidence or a denied tool call identifies a protected change")
	}
	if len(dependency) > 0 {
		add(SignalDependencyChange, dependency, "dependency authority changed")
	}
	if len(verification) > 0 {
		add(SignalVerificationAuthorityMutation, verification, "pre-admitted verification authority changed")
	}
	claimed := []string{}
	if summary != nil {
		claimed = compactSorted(summary.ClaimedFiles)
	}
	if summary != nil && !slices.Equal(claimed, paths) {
		add(SignalClaimedActualMismatch, union(claimed, paths), "implementer claimed files differ from the host-observed manifest")
	}
	toolPaths := []string{}
	toolEvidenceMismatch := []string{}
	for _, execution := range executions {
		for _, change := range execution.SourceChanges {
			toolPaths = append(toolPaths, change.Path)
			if change.BeforeSHA256 != sourceEntryIdentity(before.SourceSnapshot, change.Path) || change.AfterSHA256 != sourceEntryIdentity(after.SourceSnapshot, change.Path) {
				toolEvidenceMismatch = append(toolEvidenceMismatch, change.Path)
			}
		}
	}
	toolPaths = compactSorted(toolPaths)
	if !slices.Equal(toolPaths, paths) || len(toolEvidenceMismatch) > 0 {
		add(SignalToolActualMismatch, union(union(toolPaths, paths), toolEvidenceMismatch), "recorded tool source paths or content identities differ from the host-observed source snapshot")
	}
	if disposition != "completed" || summary != nil && summary.Partial {
		add(SignalPartialWork, paths, "implementer work is partial or lacks a validated final summary")
	}
	if disposition == "cancelled" {
		add(SignalCancellation, paths, "model or tool cancellation terminated sandbox work")
	}
	if before.HeadCommit != "" && after.HeadCommit != "" && before.HeadCommit != after.HeadCommit {
		add(SignalUnexpectedChange, paths, "worker changed Git HEAD without host authority")
	}
	sort.Slice(signals, func(i, j int) bool { return signals[i].Kind < signals[j].Kind })
	return signals
}

func cloneAdmission(value Admission) Admission {
	steps := make([]planner.Step, len(value.ActiveSteps))
	for i, step := range value.ActiveSteps {
		step.CriterionIDs = append([]string(nil), step.CriterionIDs...)
		step.DependsOnStepIDs = append([]string(nil), step.DependsOnStepIDs...)
		step.ExpectedPaths = append([]string(nil), step.ExpectedPaths...)
		step.Components = append([]string(nil), step.Components...)
		step.TestStrategy = append([]planner.TestStrategy(nil), step.TestStrategy...)
		step.Risks = append([]string(nil), step.Risks...)
		step.Assumptions = append([]string(nil), step.Assumptions...)
		step.EvidenceRefs = append([]string(nil), step.EvidenceRefs...)
		if step.Lineage != nil {
			lineage := *step.Lineage
			step.Lineage = &lineage
		}
		steps[i] = step
	}
	value.ActiveSteps = steps
	value.ExpectedPaths = append([]string(nil), value.ExpectedPaths...)
	value.AdjacentPaths = append([]string(nil), value.AdjacentPaths...)
	value.ProtectedPaths = append([]string(nil), value.ProtectedPaths...)
	value.DependencyPaths = append([]string(nil), value.DependencyPaths...)
	value.VerificationPaths = append([]string(nil), value.VerificationPaths...)
	return value
}

func cloneHistory(values []HistoryItem) []HistoryItem {
	result := make([]HistoryItem, len(values))
	copy(result, values)
	for i := range result {
		result[i].ToolCall = append(json.RawMessage(nil), values[i].ToolCall...)
		if values[i].ToolOutcome != nil {
			outcome := modelVisibleOutcome(*values[i].ToolOutcome)
			result[i].ToolOutcome = &outcome
		}
	}
	return result
}

func manifestPaths(manifest []Change) []string {
	values := []string{}
	for _, change := range manifest {
		values = append(values, change.Path)
		if change.OldPath != "" {
			values = append(values, change.OldPath)
		}
	}
	return compactSorted(values)
}

func sourceEntryIdentity(snapshot gitstate.SourceSnapshot, candidate string) string {
	for _, entry := range snapshot.Entries {
		if entry.Path == candidate && entry.FileType != "missing" {
			return entry.SHA256
		}
	}
	return "absent"
}

func compactSorted(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return slices.Compact(out)
}
func union(left, right []string) []string {
	return compactSorted(append(append([]string(nil), left...), right...))
}
func covered(candidate string, roots []string) bool {
	for _, root := range roots {
		if candidate == root || strings.HasPrefix(candidate, root+"/") {
			return true
		}
	}
	return false
}
func token(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && len(value) <= 512 && !strings.ContainsAny(value, " \t\r\n\x00")
}
func validSHA(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == 32 && value == strings.ToLower(value)
}
func bounded(err error) string {
	if err == nil {
		return ""
	}
	return boundedString(err.Error())
}
func boundedString(value string) string {
	if len(value) > 4096 {
		return value[:4096] + " [truncated]"
	}
	return value
}
func now(clock func() time.Time) time.Time {
	if clock == nil {
		clock = time.Now
	}
	return clock().UTC().Truncate(time.Microsecond)
}
func boundedText(label, value string, required bool) error {
	if required && strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) || len(value) > 16384 || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
		return fmt.Errorf("implementer %s is blank or malformed", label)
	}
	return nil
}
func validateTextList(label string, values []string, required, paths bool) error {
	if required && len(values) == 0 {
		return fmt.Errorf("implementer %s is empty", label)
	}
	for i, value := range values {
		if boundedText(label, value, true) != nil || slices.Contains(values[:i], value) {
			return fmt.Errorf("implementer %s is malformed or duplicated", label)
		}
		if paths && (strings.HasPrefix(value, "/") || strings.Contains(value, "\\") || path.Clean(value) != value || value == "." || value == ".." || strings.HasPrefix(value, "../") || strings.ContainsAny(value, "\r\n")) {
			return fmt.Errorf("implementer %s contains unsafe path", label)
		}
	}
	return nil
}

func rejectDuplicateFields(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var walk func() error
	walk = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			seen := map[string]bool{}
			for decoder.More() {
				keyValue, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyValue.(string)
				if !ok || seen[key] {
					return fmt.Errorf("duplicate or malformed JSON field %q", key)
				}
				seen[key] = true
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
			return errors.New("unexpected JSON delimiter")
		}
	}
	return walk()
}
