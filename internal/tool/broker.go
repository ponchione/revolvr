package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"revolvr/internal/outputcap"
)

type Broker struct {
	policy       Policy
	handler      RuntimeHandler
	sequencer    TrajectorySequencer
	store        evidenceStore
	clock        func() time.Time
	sequenceMu   sync.Mutex
	lastSequence int64
}

func NewBroker(policy Policy, executor SandboxExecutor, store *FileStore) (*Broker, error) {
	if executor == nil {
		return nil, errors.New("tool broker requires a sandbox executor and evidence store")
	}
	return NewBrokerWithRuntime(policy, directRuntimeHandler{executor: executor}, store, &localSequencer{})
}

// NewBrokerWithRuntime is the internal compatibility boundary for future
// runtime handlers. The closed runtime kind still admits direct tools only.
func NewBrokerWithRuntime(policy Policy, handler RuntimeHandler, store *FileStore, sequencer TrajectorySequencer) (*Broker, error) {
	if handler == nil || store == nil || sequencer == nil {
		return nil, errors.New("tool broker requires a runtime handler, evidence store, and trusted trajectory sequencer")
	}
	if err := validateRuntimeKind(handler.Kind()); err != nil {
		return nil, err
	}
	if err := validatePolicy(policy, true); err != nil {
		return nil, err
	}
	policy.Authority = cloneAuthority(policy.Authority)
	policy.sandbox = cloneSandbox(policy.sandbox)
	policy.registry.Definitions = cloneDefinitions(policy.registry.Definitions)
	return &Broker{policy: policy, handler: handler, sequencer: sequencer, store: store, clock: time.Now}, nil
}

func (b *Broker) Authority() Authority { return b.policy.AuthorityCopy() }
func (b *Broker) Registry() Registry   { return b.policy.RegistryCopy() }
func (b *Broker) Scope() PolicyScope   { return b.policy.ScopeCopy() }
func (b *Broker) PolicyIdentity() (string, string) {
	return b.policy.Version, b.policy.SHA256
}

func (b *Broker) Dispatch(ctx context.Context, raw []byte) (Outcome, error) {
	return b.DispatchRuntime(ctx, RuntimeDirectToolsV1, raw)
}

// DispatchRuntime is host-only dispatch. Unknown or reserved runtime kinds and
// invalid host ordering grants are refused before journal replay or effects.
func (b *Broker) DispatchRuntime(ctx context.Context, runtimeKind RuntimeKind, raw []byte) (Outcome, error) {
	if err := validateRuntimeKind(runtimeKind); err != nil {
		return Outcome{}, err
	}
	if b.handler.Kind() != runtimeKind {
		return Outcome{}, errors.New("tool runtime handler kind does not match dispatch authority")
	}
	requestHash := digest(raw)
	sequenceRequest := SequenceRequest{RuntimeKind: runtimeKind, RunID: b.policy.Authority.RunID, RequestSHA256: requestHash}
	b.sequenceMu.Lock()
	grant, err := b.sequencer.Next(ctx, sequenceRequest)
	if err != nil {
		b.sequenceMu.Unlock()
		return Outcome{}, fmt.Errorf("assign tool trajectory sequence: %w", err)
	}
	if err := validateSequenceGrant(sequenceRequest, grant, b.lastSequence); err != nil {
		b.sequenceMu.Unlock()
		return Outcome{}, err
	}
	b.lastSequence = grant.Sequence
	b.sequenceMu.Unlock()
	sequence := grant.Sequence

	callID := minimalCallID(raw)
	if callID == "" {
		callID = "invalid-" + digest(raw)
	}
	begin, err := b.store.Begin(ctx, callID, raw)
	if err != nil {
		return Outcome{}, fmt.Errorf("begin tool evidence: %w", err)
	}
	if begin.disposition == beginConflict {
		conflictKey := callID + "-conflict-" + digest(raw)
		begin, err = b.store.Begin(ctx, conflictKey, raw)
		if err != nil {
			return Outcome{}, fmt.Errorf("begin conflicting tool evidence: %w", err)
		}
		if begin.disposition == beginReplay {
			if err := validateReplayEvidence(begin.evidence, runtimeKind, requestHash); err != nil {
				return Outcome{}, err
			}
			begin.evidence.Replayed = true
			return Outcome{Evidence: begin.evidence, TrajectorySequence: sequence, ReplayedFromSequence: begin.evidence.TrajectorySequence}, nil
		}
		return b.deny(ctx, runtimeKind, sequence, conflictKey, callID, "", raw, begin.input, "replay_identity_conflict", "call_id was previously used with different exact input", utcNow(b.clock))
	}
	if begin.disposition == beginReplay {
		if err := validateReplayEvidence(begin.evidence, runtimeKind, requestHash); err != nil {
			return Outcome{}, err
		}
		begin.evidence.Replayed = true
		stdout, _ := os.ReadFile(begin.evidence.Stdout.Path)
		stderr, _ := os.ReadFile(begin.evidence.Stderr.Path)
		return Outcome{Evidence: begin.evidence, TrajectorySequence: sequence, ReplayedFromSequence: begin.evidence.TrajectorySequence, Stdout: string(stdout), Stderr: string(stderr)}, nil
	}
	started := utcNow(b.clock)
	if begin.disposition == beginIndeterminate {
		return b.deny(ctx, runtimeKind, sequence, callID, callID, "", raw, begin.input, "indeterminate_prior_effect", "an exact prior intent has no terminal result; dispatch is refused", started)
	}
	if len(raw) == 0 || len(raw) > maximumCallBytes {
		return b.deny(ctx, runtimeKind, sequence, callID, callID, "", raw, begin.input, "malformed_call", "tool call is empty or exceeds the input cap", started)
	}
	call, operation, code, detail := b.validate(raw)
	if code != "" {
		return b.deny(ctx, runtimeKind, sequence, callID, call.CallID, call.Tool, raw, begin.input, code, detail, started)
	}

	executionCtx, cancel := context.WithTimeout(ctx, b.executionTimeout(operation))
	result, executeErr := b.handler.Execute(executionCtx, RuntimeExecutionRequest{
		RuntimeKind: runtimeKind, TrajectorySequence: sequence, RequestSHA256: requestHash,
		Call: call, Operation: operation, Sandbox: b.policy.SandboxCopy(),
		HostPolicyVersion: b.policy.Version, HostPolicySHA256: b.policy.SHA256,
	})
	executionContextErr := executionCtx.Err()
	cancel()
	finished := utcNow(b.clock)
	stdoutCap, stderrCap := b.outputLimits(operation)
	stdout, stdoutTrimmed := capBytes(result.Stdout, stdoutCap)
	stderr, stderrTrimmed := capBytes(result.Stderr, stderrCap)
	invalidTruncation := result.StdoutTruncatedBytes < 0 || result.StderrTruncatedBytes < 0
	result.Stdout, result.Stderr = stdout, stderr
	result.StdoutTruncatedBytes = max(0, result.StdoutTruncatedBytes) + stdoutTrimmed
	result.StderrTruncatedBytes = max(0, result.StderrTruncatedBytes) + stderrTrimmed
	timedOut := result.TimedOut || errors.Is(executionContextErr, context.DeadlineExceeded) || errors.Is(executeErr, context.DeadlineExceeded)
	cancelled := !timedOut && (result.Cancelled || errors.Is(ctx.Err(), context.Canceled) || errors.Is(executeErr, context.Canceled))
	result.TimedOut = timedOut
	result.Cancelled = cancelled
	evidence := Evidence{
		SchemaVersion: EvidenceSchemaVersion, RuntimeKind: runtimeKind, TrajectorySequence: sequence,
		CallID: call.CallID, Tool: call.Tool, RequestSHA256: requestHash,
		Authority: cloneAuthority(call.Authority), Input: begin.input,
		Runtime:   b.runtimeEvidence(),
		StartedAt: started, FinishedAt: finished, Duration: finished.Sub(started),
		Disposition: "completed", ExitCode: result.ExitCode, TimedOut: timedOut,
		Cancelled:            cancelled,
		Truncated:            result.StdoutTruncatedBytes > 0 || result.StderrTruncatedBytes > 0,
		StdoutTruncatedBytes: result.StdoutTruncatedBytes, StderrTruncatedBytes: result.StderrTruncatedBytes,
		SourceChanges: append([]SourceChange(nil), result.SourceChanges...), Effect: result.Effect,
	}
	if evidence.TimedOut || evidence.Cancelled {
		evidence.Cancellation = b.cancelSandbox()
		if evidence.TimedOut {
			evidence.Disposition = "timed_out"
			evidence.DenialCode = "tool_timeout"
		} else {
			evidence.Disposition = "cancelled"
			evidence.DenialCode = "tool_cancelled"
		}
	}
	if executeErr != nil && !evidence.TimedOut && !evidence.Cancelled {
		evidence.Disposition = "failed"
		evidence.Detail = boundedDetail(executeErr)
	} else if executeErr != nil {
		evidence.Detail = boundedDetail(executeErr)
	}
	if invalidTruncation || stdoutTrimmed > 0 || stderrTrimmed > 0 {
		evidence.Disposition = "failed"
		evidence.DenialCode = "output_cap_exceeded"
		evidence.Detail = "sandbox executor returned output beyond the admitted cap"
	}
	if effectErr := validateEffect(operation, result); effectErr != nil {
		evidence.Disposition = "failed"
		evidence.DenialCode = "unproven_external_effect"
		evidence.Detail = boundedDetail(effectErr)
	}
	persisted, persistErr := b.store.Complete(context.WithoutCancel(ctx), callID, raw, result.Stdout, result.Stderr, evidence)
	if persistErr != nil {
		return Outcome{Evidence: evidence, TrajectorySequence: sequence, Stdout: string(result.Stdout), Stderr: string(result.Stderr)}, fmt.Errorf("persist tool evidence: %w", persistErr)
	}
	return Outcome{Evidence: persisted, TrajectorySequence: sequence, Stdout: string(result.Stdout), Stderr: string(result.Stderr)}, executeErr
}

func (b *Broker) Cancel() CancellationEvidence { return b.cancelSandbox() }

func (b *Broker) cancelSandbox() CancellationEvidence {
	evidence := CancellationEvidence{Requested: true, StopAttempted: true}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := b.handler.Cancel(ctx, b.policy.Authority.SandboxID); err != nil {
		evidence.Error = boundedDetail(err)
		return evidence
	}
	evidence.StopSucceeded = true
	return evidence
}

func (b *Broker) deny(ctx context.Context, runtimeKind RuntimeKind, sequence int64, replayKey, callID, toolName string, raw []byte, input Artifact, code, detail string, started time.Time) (Outcome, error) {
	finished := utcNow(b.clock)
	evidence := Evidence{
		SchemaVersion: EvidenceSchemaVersion, RuntimeKind: runtimeKind, TrajectorySequence: sequence,
		CallID: callID, Tool: toolName, RequestSHA256: digest(raw), Runtime: b.runtimeEvidence(),
		Authority: b.policy.AuthorityCopy(), Input: input, StartedAt: started,
		FinishedAt: finished, Duration: finished.Sub(started), Disposition: "denied",
		DenialCode: code, Detail: detail, ExitCode: -1,
	}
	persisted, err := b.store.Complete(context.WithoutCancel(ctx), replayKey, raw, nil, nil, evidence)
	if err != nil {
		return Outcome{Evidence: evidence, TrajectorySequence: sequence}, fmt.Errorf("persist denied tool evidence: %w", err)
	}
	return Outcome{Evidence: persisted, TrajectorySequence: sequence}, nil
}

func (b *Broker) runtimeEvidence() RuntimeEvidence {
	return RuntimeEvidence{
		Image: b.policy.sandbox.Image, Profile: b.policy.sandbox.RuntimeProfile,
		Network: b.policy.sandbox.Network, Resources: b.policy.sandbox.Resources,
		SandboxSHA256: b.policy.SandboxSHA256, HostPolicyVersion: b.policy.Version,
		HostPolicySHA256: b.policy.SHA256,
	}
}

func validateReplayEvidence(evidence Evidence, runtimeKind RuntimeKind, requestHash string) error {
	if evidence.SchemaVersion != EvidenceSchemaVersion || evidence.RuntimeKind != runtimeKind || evidence.TrajectorySequence <= 0 || evidence.RequestSHA256 != requestHash || !validSHA(evidence.ResultSHA256) {
		return errors.New("tool replay evidence has stale runtime, sequence, request, or result authority")
	}
	if err := validateResultRepresentation(evidence.ResultRepresentation, evidence.ResultSHA256); err != nil {
		return fmt.Errorf("tool replay result representation: %w", err)
	}
	return nil
}

func (b *Broker) validate(raw []byte) (Call, Operation, string, string) {
	if err := validatePolicy(b.policy, true); err != nil {
		return Call{}, Operation{}, "stale_host_policy", boundedDetail(err)
	}
	if err := rejectDuplicateJSONFields(raw); err != nil {
		return Call{}, Operation{}, "malformed_call", boundedDetail(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var call Call
	if err := decoder.Decode(&call); err != nil {
		return Call{}, Operation{}, "malformed_call", boundedDetail(err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return call, Operation{}, "malformed_call", "content follows the single tool call"
	}
	if call.SchemaVersion != CallSchemaVersion || !identityToken(call.CallID) {
		return call, Operation{}, "malformed_call", "schema_version or call_id is invalid"
	}
	if !reflect.DeepEqual(call.Authority, b.policy.Authority) {
		return call, Operation{}, "stale_authority", "run, task, source, plan, step, workspace, sandbox, registry, or host-policy identity is stale"
	}
	definition, ok := registryDefinition(b.policy.registry, call.Tool)
	if !ok {
		return call, Operation{}, "unknown_tool", "tool is not present in the closed role registry"
	}
	operation, code, detail := b.validateArguments(call.Tool, call.Arguments)
	if code != "" {
		return call, Operation{}, code, detail
	}
	if definition.MutatesSource && b.policy.Role != "implementer" && b.policy.Role != "corrector" {
		return call, Operation{}, "role_capability_denied", "only mutation roles receive source-write capabilities"
	}
	return call, operation, "", ""
}

func (b *Broker) validateArguments(name string, raw []byte) (Operation, string, string) {
	if len(raw) == 0 || len(raw) > maximumCallBytes {
		return Operation{}, "malformed_arguments", "tool arguments are missing or oversized"
	}
	if err := rejectDuplicateJSONFields(raw); err != nil {
		return Operation{}, "malformed_arguments", boundedDetail(err)
	}
	switch name {
	case ToolFileRead:
		var value FileReadArguments
		if err := decodeClosed(raw, &value); err != nil {
			return Operation{}, "malformed_arguments", boundedDetail(err)
		}
		if value.Offset < 0 || value.MaxBytes <= 0 || value.MaxBytes > b.policy.MaximumReadBytes {
			return Operation{}, "resource_limit", "file-read offset or byte cap is outside policy"
		}
		if code, detail := b.validatePath(value.Path, false, false); code != "" {
			return Operation{}, code, detail
		}
		return Operation{Tool: name, FileRead: &value}, "", ""
	case ToolTextSearch:
		var value TextSearchArguments
		if err := decodeClosed(raw, &value); err != nil {
			return Operation{}, "malformed_arguments", boundedDetail(err)
		}
		if value.Query == "" || len(value.Query) > 4096 || !utf8.ValidString(value.Query) || strings.ContainsRune(value.Query, 0) || len(value.Paths) == 0 || value.MaximumResults <= 0 || value.MaximumResults > b.policy.MaximumSearchResults || value.OutputCapBytes <= 0 || value.OutputCapBytes > b.policy.MaximumStdoutBytes {
			return Operation{}, "resource_limit", "search query, paths, result limit, or output cap is outside policy"
		}
		for i, candidate := range value.Paths {
			if slices.Contains(value.Paths[:i], candidate) {
				return Operation{}, "malformed_arguments", "search paths contain a duplicate"
			}
			if code, detail := b.validatePath(candidate, false, false); code != "" {
				return Operation{}, code, detail
			}
		}
		return Operation{Tool: name, TextSearch: &value}, "", ""
	case ToolSourceEdit:
		var value SourceEditArguments
		if err := decodeClosed(raw, &value); err != nil {
			return Operation{}, "malformed_arguments", boundedDetail(err)
		}
		if len(value.Content) > int(b.policy.MaximumEditBytes) || !utf8.ValidString(value.Content) || value.ExpectedSHA256 != "absent" && !validSHA(value.ExpectedSHA256) {
			return Operation{}, "resource_limit", "edit content or expected content identity is invalid"
		}
		if code, detail := b.validatePath(value.Path, true, true); code != "" {
			return Operation{}, code, detail
		}
		return Operation{Tool: name, SourceEdit: &value}, "", ""
	case ToolCommand:
		var value CommandArguments
		if err := decodeClosed(raw, &value); err != nil {
			return Operation{}, "malformed_arguments", boundedDetail(err)
		}
		if code, detail := b.validateCommand(value); code != "" {
			return Operation{}, code, detail
		}
		return Operation{Tool: name, Command: &value}, "", ""
	default:
		return Operation{}, "unknown_tool", "tool is not in the closed registry"
	}
}

func (b *Broker) validatePath(candidate string, write, missingFinalOK bool) (string, string) {
	if !cleanRelativePath(candidate) {
		return "unsafe_path", "path is not normalized workspace-relative input"
	}
	if secretPath(candidate) {
		return "secret_or_control_path", "path is denied to the worker"
	}
	if pathCovered(candidate, b.policy.DeniedReadPaths) {
		return "secret_or_control_path", "path is denied to the worker"
	}
	if write && pathCovered(candidate, b.policy.ProtectedPaths) {
		return "protected_path", "source mutation targets a protected path"
	}
	if !pathCovered(candidate, b.policy.ExpectedPaths) && !pathCovered(candidate, b.policy.AdjacentPaths) {
		return "scope_denied", "path is outside the active plan-step scope"
	}
	resolved, err := resolveWorkspacePath(b.policy.WorkspaceRoot, candidate, missingFinalOK)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "symbolic link") {
			return "symlink_denied", boundedDetail(err)
		}
		return "unsafe_path", boundedDetail(err)
	}
	if info, statErr := os.Lstat(resolved); statErr == nil && !info.Mode().IsRegular() && !info.IsDir() {
		return "secret_or_control_path", "socket, device, pipe, and other special files are denied"
	} else if statErr != nil && !(missingFinalOK && errors.Is(statErr, os.ErrNotExist)) {
		return "unsafe_path", boundedDetail(statErr)
	}
	if write {
		parent := filepath.Dir(resolved)
		info, statErr := os.Lstat(parent)
		if statErr != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "unsafe_path", "edit parent is not an existing real directory"
		}
	}
	return "", ""
}

func (b *Broker) validateCommand(value CommandArguments) (string, string) {
	if len(value.Argv) == 0 || len(value.Argv) > 256 {
		return "malformed_arguments", "command requires bounded direct argv"
	}
	for _, argument := range value.Argv {
		if argument == "" || len(argument) > 4096 || !utf8.ValidString(argument) || strings.ContainsRune(argument, 0) {
			return "malformed_arguments", "command contains an empty, malformed, or oversized argv value"
		}
	}
	allowed := false
	for _, command := range b.policy.AllowedCommands {
		if slices.Equal(command, value.Argv) {
			allowed = true
			break
		}
	}
	if !allowed {
		return "command_denied", "direct argv is not an exact admitted command"
	}
	if !slices.Contains(b.policy.AllowedWorkingDirectories, value.WorkingDirectory) {
		return "working_directory_denied", "working directory is not an admitted container path"
	}
	if value.WorkingDirectory != "/workspace" {
		relative := strings.TrimPrefix(value.WorkingDirectory, "/workspace/")
		if code, detail := b.validatePath(relative, false, false); code != "" {
			return code, detail
		}
	}
	if value.Network != b.policy.Network || value.Network != b.policy.sandbox.Network {
		return "network_denied", "network profile differs from admitted sandbox authority"
	}
	if value.TimeoutMilliseconds <= 0 || value.TimeoutMilliseconds > b.policy.MaximumTimeoutMilliseconds || value.CPUs <= 0 || value.CPUs > b.policy.MaximumCPUs || value.MemoryBytes <= 0 || value.MemoryBytes > b.policy.MaximumMemoryBytes || value.PIDs <= 0 || value.PIDs > b.policy.MaximumPIDs || value.TmpfsBytes <= 0 || value.TmpfsBytes > b.policy.MaximumTmpfsBytes || value.StdoutCapBytes <= 0 || value.StdoutCapBytes > b.policy.MaximumStdoutBytes || value.StderrCapBytes <= 0 || value.StderrCapBytes > b.policy.MaximumStderrBytes {
		return "resource_limit", "command timeout, resources, or output caps exceed admitted policy"
	}
	for i, name := range value.EnvironmentNames {
		if i > 0 && value.EnvironmentNames[i-1] >= name || !slices.Contains(b.policy.AllowedEnvironmentNames, name) || forbiddenEnvironmentName(name) {
			return "environment_denied", "environment names must be sorted, unique, safe, and admitted"
		}
	}
	return "", ""
}

func (b *Broker) outputLimits(operation Operation) (int64, int64) {
	stdoutCap, stderrCap := int64(0), int64(0)
	switch operation.Tool {
	case ToolFileRead:
		stdoutCap, stderrCap = operation.FileRead.MaxBytes, b.policy.MaximumStderrBytes
	case ToolTextSearch:
		stdoutCap, stderrCap = operation.TextSearch.OutputCapBytes, b.policy.MaximumStderrBytes
	case ToolSourceEdit:
		stdoutCap, stderrCap = min(int64(4096), b.policy.MaximumStdoutBytes), min(int64(4096), b.policy.MaximumStderrBytes)
	case ToolCommand:
		stdoutCap, stderrCap = operation.Command.StdoutCapBytes, operation.Command.StderrCapBytes
	}
	return stdoutCap, stderrCap
}

func (b *Broker) executionTimeout(operation Operation) time.Duration {
	milliseconds := b.policy.MaximumTimeoutMilliseconds
	if operation.Tool == ToolCommand {
		milliseconds = operation.Command.TimeoutMilliseconds
	}
	return time.Duration(milliseconds) * time.Millisecond
}

func capBytes(raw []byte, limit int64) ([]byte, int64) {
	buffer := outputcap.NewBuffer(int(limit))
	_, _ = buffer.Write(raw)
	return []byte(buffer.String()), buffer.TruncatedBytes()
}

func validateEffect(operation Operation, result ExecutionResult) error {
	if operation.Tool == ToolSourceEdit && result.ExitCode == 0 && !result.Cancelled && !result.TimedOut {
		if !result.Effect.Proven || result.Effect.Kind != "source_edit" || result.Effect.Identity != operation.SourceEdit.Path || result.Effect.BeforeSHA256 != operation.SourceEdit.ExpectedSHA256 || result.Effect.AfterSHA256 != digest([]byte(operation.SourceEdit.Content)) {
			return errors.New("successful source edit lacks exact before/after effect proof")
		}
		if len(result.SourceChanges) != 1 || result.SourceChanges[0].Path != operation.SourceEdit.Path || result.SourceChanges[0].BeforeSHA256 != result.Effect.BeforeSHA256 || result.SourceChanges[0].AfterSHA256 != result.Effect.AfterSHA256 {
			return errors.New("successful source edit lacks matching source-change evidence")
		}
	}
	if len(result.SourceChanges) > 0 && !result.Effect.Proven {
		return errors.New("source-changing command lacks proven external-effect identity")
	}
	return nil
}

func registryDefinition(registry Registry, name string) (Definition, bool) {
	for _, definition := range registry.Definitions {
		if definition.Name == name {
			return definition, true
		}
	}
	return Definition{}, false
}

func decodeClosed(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("content follows tool arguments")
	}
	return nil
}

func minimalCallID(raw []byte) string {
	var value struct {
		CallID string `json:"call_id"`
	}
	if json.Unmarshal(raw, &value) != nil || !identityToken(value.CallID) {
		return ""
	}
	return value.CallID
}

func rejectDuplicateJSONFields(raw []byte) error {
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
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok || seen[key] {
					return fmt.Errorf("duplicate or malformed field %q", key)
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
