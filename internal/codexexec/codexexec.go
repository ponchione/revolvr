package codexexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"revolvr/internal/ledger"
	"revolvr/internal/pathguard"
	"revolvr/internal/receipt"
	"revolvr/internal/runner"
)

const (
	maxSummaryText = 500
)

type CommandRunner func(context.Context, runner.Command) runner.Result

type Ledger interface {
	AppendEvent(context.Context, string, ledger.EventType, any) (ledger.Event, error)
}

type ArtifactPaths struct {
	StdoutJSONL string `json:"stdout_jsonl"`
	Stderr      string `json:"stderr"`
	LastMessage string `json:"last_message"`
}

type ProgressEvent struct {
	Source  string
	Message string
}

type Config struct {
	Executable                string
	WorkingDir                string
	Prompt                    string
	Model                     string
	ReasoningEffort           string
	Ephemeral                 *bool
	Timeout                   time.Duration
	StdoutCap                 int
	StderrCap                 int
	Sandbox                   string
	ApprovalPolicy            string
	BypassApprovalsAndSandbox bool
	Artifacts                 ArtifactPaths
	OutputSchema              string
	RunID                     string
	Ledger                    Ledger
	CommandRunner             CommandRunner
	OnProgress                func(ProgressEvent)
	Provenance                InvocationProvenance
}

type CappedOutput struct {
	Content        string
	TruncatedBytes int64
	Path           string
}

type Result struct {
	ExitCode        int
	TimedOut        bool
	Err             error
	FinalMessage    string
	Usage           receipt.Metrics
	UsageFound      bool
	Artifacts       ArtifactPaths
	Stdout          CappedOutput
	Stderr          CappedOutput
	JSONEvents      int
	JSONParseErrors []string
	ParseError      error
	ArtifactError   error
	LedgerError     error
}

func Run(ctx context.Context, cfg Config) (Result, error) {
	cfg, workDir, err := normalizeConfig(cfg)
	if err != nil {
		return Result{}, err
	}
	provenance, artifacts, err := PrepareInvocation(InvocationConfig{
		Executable:             cfg.Executable,
		WorkingDir:             workDir,
		Model:                  cfg.Model,
		ReasoningEffort:        cfg.ReasoningEffort,
		Ephemeral:              *cfg.Ephemeral,
		Sandbox:                cfg.Sandbox,
		ApprovalPolicy:         cfg.ApprovalPolicy,
		BypassApprovalsSandbox: cfg.BypassApprovalsAndSandbox,
		Artifacts:              cfg.Artifacts,
		OutputSchema:           cfg.OutputSchema,
		CodexVersion:           cfg.Provenance.Version,
		EffectiveConfigSchema:  cfg.Provenance.EffectiveConfigSchema,
		EffectiveConfigSHA256:  cfg.Provenance.EffectiveConfigSHA256,
	})
	if err != nil {
		return Result{}, err
	}
	if len(cfg.Provenance.Argv) > 0 {
		if err := cfg.Provenance.Validate(); err != nil {
			return Result{}, err
		}
		if !sameInvocation(cfg.Provenance, provenance) {
			return Result{}, errors.New("run codex exec: supplied invocation provenance does not match effective invocation")
		}
		provenance = cfg.Provenance
	}

	stdoutFile, err := createArtifact(artifacts.StdoutJSONL)
	if err != nil {
		return Result{}, err
	}
	var stderrFile *os.File
	if artifacts.Stderr != "" {
		stderrFile, err = createArtifact(artifacts.Stderr)
		if err != nil {
			_ = stdoutFile.Close()
			return Result{}, err
		}
	}
	if artifacts.LastMessage != "" {
		if err := ensureParent(artifacts.LastMessage); err != nil {
			_ = stdoutFile.Close()
			if stderrFile != nil {
				_ = stderrFile.Close()
			}
			return Result{}, err
		}
		if err := os.Remove(artifacts.LastMessage); err != nil && !errors.Is(err, os.ErrNotExist) {
			_ = stdoutFile.Close()
			if stderrFile != nil {
				_ = stderrFile.Close()
			}
			return Result{}, fmt.Errorf("prepare last-message artifact: %w", err)
		}
	}

	state := &executionState{}
	appendLedger := func(eventType ledger.EventType, payload any) {
		if cfg.Ledger == nil {
			return
		}
		if _, err := cfg.Ledger.AppendEvent(ctx, cfg.RunID, eventType, payload); err != nil {
			state.setLedgerError(err)
		}
	}

	args := append([]string(nil), provenance.Argv...)
	appendLedger(ledger.EventCodexStarted, map[string]any{
		"executable":  cfg.Executable,
		"args":        args,
		"working_dir": workDir,
		"artifacts":   artifacts,
		"provenance":  provenance,
	})

	runResult := cfg.CommandRunner(ctx, runner.Command{
		Name:        cfg.Executable,
		Args:        args,
		Stdin:       strings.NewReader(cfg.Prompt),
		Dir:         workDir,
		Timeout:     cfg.Timeout,
		StdoutLimit: cfg.StdoutCap,
		StderrLimit: cfg.StderrCap,
		OnStdoutLine: func(line string) {
			lineNumber := state.nextStdoutLine()
			if _, err := fmt.Fprintln(stdoutFile, line); err != nil {
				state.setArtifactError(fmt.Errorf("write stdout JSONL artifact: %w", err))
			}
			event, ok := state.recordJSONLine(lineNumber, line)
			if !ok {
				return
			}
			if message := finalMessageFromEvent(event); message != "" {
				state.setFinalMessage(message)
			}
			if cfg.OnProgress != nil {
				if message := progressMessageFromEvent(event); message != "" {
					cfg.OnProgress(ProgressEvent{Source: "codex", Message: message})
				}
			}
			appendLedger(ledger.EventCodexJSONEvent, summarizeEvent(lineNumber, event))
		},
		OnStderrLine: func(line string) {
			if stderrFile == nil {
				if cfg.OnProgress != nil {
					cfg.OnProgress(ProgressEvent{Source: "codex stderr", Message: strings.TrimSpace(line)})
				}
				return
			}
			if _, err := fmt.Fprintln(stderrFile, line); err != nil {
				state.setArtifactError(fmt.Errorf("write stderr artifact: %w", err))
			}
			if cfg.OnProgress != nil {
				cfg.OnProgress(ProgressEvent{Source: "codex stderr", Message: strings.TrimSpace(line)})
			}
		},
	})

	if err := stdoutFile.Close(); err != nil {
		state.setArtifactError(fmt.Errorf("close stdout JSONL artifact: %w", err))
	}
	if stderrFile != nil {
		if err := stderrFile.Close(); err != nil {
			state.setArtifactError(fmt.Errorf("close stderr artifact: %w", err))
		}
	}

	result := Result{
		ExitCode:  runResult.ExitCode,
		TimedOut:  runResult.TimedOut,
		Err:       runResult.Err,
		Artifacts: artifacts,
		Stdout: CappedOutput{
			Content:        runResult.Stdout,
			TruncatedBytes: runResult.StdoutTruncatedBytes,
			Path:           artifacts.StdoutJSONL,
		},
		Stderr: CappedOutput{
			Content:        runResult.Stderr,
			TruncatedBytes: runResult.StderrTruncatedBytes,
			Path:           artifacts.Stderr,
		},
	}

	if artifacts.LastMessage != "" {
		if message, err := readLastMessage(artifacts.LastMessage); err == nil && message != "" {
			state.setFinalMessage(message)
		} else if err != nil {
			state.setArtifactError(err)
		}
	}

	if raw, err := os.ReadFile(artifacts.StdoutJSONL); err == nil {
		usage, found, parseErr := receipt.ParseCodexUsageMetrics(raw)
		if parseErr != nil {
			result.ParseError = parseErr
		} else {
			result.Usage = usage
			result.UsageFound = found
		}
	} else {
		state.setArtifactError(fmt.Errorf("read stdout JSONL artifact: %w", err))
	}

	result.applyState(state)
	appendLedger(ledger.EventCodexCompleted, map[string]any{
		"exit_code":              result.ExitCode,
		"timed_out":              result.TimedOut,
		"error":                  errorString(result.Err),
		"final_message_present":  result.FinalMessage != "",
		"usage":                  result.Usage,
		"usage_found":            result.UsageFound,
		"artifacts":              result.Artifacts,
		"stdout_truncated_bytes": result.Stdout.TruncatedBytes,
		"stderr_truncated_bytes": result.Stderr.TruncatedBytes,
		"json_events":            result.JSONEvents,
		"json_parse_errors":      result.JSONParseErrors,
		"parse_error":            errorString(result.ParseError),
		"artifact_error":         errorString(result.ArtifactError),
	})
	result.applyState(state)

	return result, nil
}

func normalizeConfig(cfg Config) (Config, string, error) {
	cfg.Executable = strings.TrimSpace(cfg.Executable)
	if cfg.Executable == "" {
		cfg.Executable = DefaultExecutable
	}
	if strings.TrimSpace(cfg.WorkingDir) == "" {
		return Config{}, "", errors.New("run codex exec: working directory is required")
	}
	if strings.TrimSpace(cfg.Prompt) == "" {
		return Config{}, "", errors.New("run codex exec: prompt is required")
	}
	if strings.TrimSpace(cfg.Artifacts.StdoutJSONL) == "" {
		return Config{}, "", errors.New("run codex exec: stdout JSONL artifact path is required")
	}
	if cfg.Ledger != nil && strings.TrimSpace(cfg.RunID) == "" {
		return Config{}, "", errors.New("run codex exec: run id is required when ledger is configured")
	}
	if cfg.CommandRunner == nil {
		cfg.CommandRunner = runner.Run
	}
	if strings.TrimSpace(cfg.Model) == "" {
		cfg.Model = DefaultModel
	}
	if strings.TrimSpace(cfg.ReasoningEffort) == "" {
		cfg.ReasoningEffort = DefaultReasoningEffort
	}
	if cfg.Ephemeral == nil {
		ephemeral := true
		cfg.Ephemeral = &ephemeral
	} else if !*cfg.Ephemeral {
		return Config{}, "", errors.New("run codex exec: only ephemeral sessions are supported")
	}
	model, err := NormalizeModel(cfg.Model)
	if err != nil {
		return Config{}, "", fmt.Errorf("run codex exec: %w", err)
	}
	effort, err := NormalizeReasoningEffort(cfg.ReasoningEffort)
	if err != nil {
		return Config{}, "", fmt.Errorf("run codex exec: %w", err)
	}
	cfg.Model = model
	cfg.ReasoningEffort = effort
	cfg.Sandbox = strings.TrimSpace(cfg.Sandbox)
	cfg.ApprovalPolicy = strings.TrimSpace(cfg.ApprovalPolicy)
	cfg.RunID = strings.TrimSpace(cfg.RunID)

	workDir, err := filepath.Abs(cfg.WorkingDir)
	if err != nil {
		return Config{}, "", fmt.Errorf("resolve working directory: %w", err)
	}
	return cfg, workDir, nil
}

func buildArgs(workDir string, cfg Config, artifacts ArtifactPaths, outputSchema string) []string {
	args := make([]string, 0, 20)
	if !cfg.BypassApprovalsAndSandbox && cfg.ApprovalPolicy != "" {
		args = append(args, "--ask-for-approval", cfg.ApprovalPolicy)
	}
	args = append(args,
		"exec",
		"--json",
		"--model", cfg.Model,
		"-c", "model_reasoning_effort="+cfg.ReasoningEffort,
	)
	if cfg.Ephemeral != nil && *cfg.Ephemeral {
		args = append(args, "--ephemeral")
	}
	if cfg.BypassApprovalsAndSandbox {
		args = append(args, "--dangerously-bypass-approvals-and-sandbox")
	} else if cfg.Sandbox != "" {
		args = append(args, "--sandbox", cfg.Sandbox)
	}
	args = append(args, "--cd", workDir)
	if outputSchema != "" {
		args = append(args, "--output-schema", outputSchema)
	}
	if artifacts.LastMessage != "" {
		args = append(args, "--output-last-message", artifacts.LastMessage)
	}
	args = append(args, "-")
	return args
}

func resolveArtifacts(workDir string, artifacts ArtifactPaths) (ArtifactPaths, error) {
	stdout, err := resolveArtifactPath(workDir, artifacts.StdoutJSONL)
	if err != nil {
		return ArtifactPaths{}, fmt.Errorf("resolve stdout JSONL artifact: %w", err)
	}
	stderr, err := resolveOptionalArtifactPath(workDir, artifacts.Stderr)
	if err != nil {
		return ArtifactPaths{}, fmt.Errorf("resolve stderr artifact: %w", err)
	}
	lastMessage, err := resolveOptionalArtifactPath(workDir, artifacts.LastMessage)
	if err != nil {
		return ArtifactPaths{}, fmt.Errorf("resolve last-message artifact: %w", err)
	}
	return ArtifactPaths{StdoutJSONL: stdout, Stderr: stderr, LastMessage: lastMessage}, nil
}

func resolveOptionalArtifactPath(workDir, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	return resolveArtifactPath(workDir, path)
}

func resolveArtifactPath(workDir, path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("path is required")
	}
	resolved, err := pathguard.Resolve(workDir, path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func createArtifact(path string) (*os.File, error) {
	if err := ensureParent(path); err != nil {
		return nil, err
	}
	file, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create artifact %s: %w", path, err)
	}
	return file, nil
}

func ensureParent(path string) error {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create artifact directory %s: %w", dir, err)
	}
	return nil
}

func readLastMessage(path string) (string, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read last-message artifact: %w", err)
	}
	return strings.TrimSpace(string(content)), nil
}

type executionState struct {
	mu              sync.Mutex
	stdoutLines     int
	jsonEvents      int
	jsonParseErrors []string
	finalMessage    string
	artifactError   error
	ledgerError     error
}

func (s *executionState) nextStdoutLine() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stdoutLines++
	return s.stdoutLines
}

func (s *executionState) recordJSONLine(lineNumber int, line string) (map[string]any, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, false
	}
	var event map[string]any
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		s.mu.Lock()
		s.jsonParseErrors = append(s.jsonParseErrors, fmt.Sprintf("line %d: %v", lineNumber, err))
		s.mu.Unlock()
		return nil, false
	}
	s.mu.Lock()
	s.jsonEvents++
	s.mu.Unlock()
	return event, true
}

func (s *executionState) setFinalMessage(message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.finalMessage = message
}

func (s *executionState) setArtifactError(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.artifactError == nil {
		s.artifactError = err
	}
}

func (s *executionState) setLedgerError(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ledgerError == nil {
		s.ledgerError = err
	}
}

func (r *Result) applyState(state *executionState) {
	state.mu.Lock()
	defer state.mu.Unlock()
	r.FinalMessage = state.finalMessage
	r.JSONEvents = state.jsonEvents
	r.JSONParseErrors = append([]string(nil), state.jsonParseErrors...)
	r.ArtifactError = state.artifactError
	r.LedgerError = state.ledgerError
}

func summarizeEvent(lineNumber int, event map[string]any) map[string]any {
	out := map[string]any{"line": lineNumber}
	copyScalar(out, "type", event["type"])
	copyScalar(out, "id", event["id"])
	copyScalar(out, "thread_id", event["thread_id"])
	copyScalar(out, "turn_id", event["turn_id"])
	copyScalar(out, "item_id", event["item_id"])
	copyScalar(out, "session_id", event["session_id"])

	if item, ok := mapValue(event["item"]); ok {
		copyScalar(out, "item_type", item["type"])
		copyScalar(out, "item_id", item["id"])
		copyScalar(out, "role", item["role"])
	}
	if message := finalMessageFromEvent(event); message != "" {
		out["message"] = truncateText(message, maxSummaryText)
	}
	if errorText := errorFromEvent(event); errorText != "" {
		out["error"] = truncateText(errorText, maxSummaryText)
	}
	if usage, found := usageFromEvent(event); found {
		out["usage"] = usage
	}
	return out
}

func usageFromEvent(event map[string]any) (receipt.Metrics, bool) {
	content, err := json.Marshal(event)
	if err != nil {
		return receipt.Metrics{}, false
	}
	usage, found, err := receipt.ParseCodexUsageMetrics(append(content, '\n'))
	if err != nil {
		return receipt.Metrics{}, false
	}
	return usage, found
}

func progressMessageFromEvent(event map[string]any) string {
	eventType := strings.TrimSpace(textFromValue(event["type"]))
	if eventType == "" {
		return ""
	}
	if errorText := errorFromEvent(event); errorText != "" {
		return "error: " + truncateText(errorText, maxSummaryText)
	}
	if item, ok := mapValue(event["item"]); ok {
		if message := progressMessageFromItem(eventType, item); message != "" {
			return message
		}
	}
	if message := finalMessageFromEvent(event); message != "" {
		return "message: " + truncateText(message, maxSummaryText)
	}
	if usage, found := usageFromEvent(event); found {
		return fmt.Sprintf("turn completed: input_tokens=%d output_tokens=%d", usage.InputTokens, usage.OutputTokens)
	}
	switch eventType {
	case "thread.started":
		if threadID := textFromValue(event["thread_id"]); threadID != "" {
			return "thread started: " + threadID
		}
		return "thread started"
	case "turn.started":
		return "turn started"
	case "turn.completed":
		return "turn completed"
	default:
		return ""
	}
}

func progressMessageFromItem(eventType string, item map[string]any) string {
	itemType := strings.TrimSpace(textFromValue(item["type"]))
	switch itemType {
	case "agent_message", "assistant_message", "message":
		if eventType != "item.completed" {
			return ""
		}
		if text := textFromValue(item["text"]); text != "" {
			return "message: " + truncateText(text, maxSummaryText)
		}
		if text := textFromValue(item["content"]); text != "" {
			return "message: " + truncateText(text, maxSummaryText)
		}
	case "command_execution":
		command := truncateText(textFromValue(item["command"]), 240)
		switch eventType {
		case "item.started":
			if command == "" {
				return "command started"
			}
			return "command started: " + command
		case "item.completed":
			status := textFromValue(item["status"])
			exitCode := scalarIntText(item["exit_code"])
			details := compactProgressDetails(status, exitCode)
			if command == "" {
				return "command completed" + details
			}
			return "command completed" + details + ": " + command
		}
	}
	return ""
}

func compactProgressDetails(values ...string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			parts = append(parts, value)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

func scalarIntText(value any) string {
	switch typed := value.(type) {
	case float64:
		return fmt.Sprintf("exit %.0f", typed)
	case int:
		return fmt.Sprintf("exit %d", typed)
	case int64:
		return fmt.Sprintf("exit %d", typed)
	default:
		return ""
	}
}

func copyScalar(out map[string]any, key string, value any) {
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) != "" {
			out[key] = typed
		}
	case float64, bool:
		out[key] = typed
	}
}

func finalMessageFromEvent(event map[string]any) string {
	for _, key := range []string{"final_message", "last_message"} {
		if message := textFromValue(event[key]); message != "" {
			return message
		}
	}

	eventType := strings.ToLower(textFromValue(event["type"]))
	if strings.Contains(eventType, "final") || strings.Contains(eventType, "completed") || strings.Contains(eventType, "message") {
		for _, key := range []string{"message", "content", "text", "output"} {
			if message := textFromValue(event[key]); message != "" {
				return message
			}
		}
	}

	for _, key := range []string{"item", "response", "result"} {
		child, ok := mapValue(event[key])
		if !ok {
			continue
		}
		childType := strings.ToLower(textFromValue(child["type"]))
		if strings.Contains(childType, "message") || strings.Contains(childType, "final") || strings.Contains(eventType, "completed") {
			for _, textKey := range []string{"message", "content", "text", "output"} {
				if message := textFromValue(child[textKey]); message != "" {
					return message
				}
			}
		}
	}
	return ""
}

func errorFromEvent(event map[string]any) string {
	if errorText := textFromValue(event["error_message"]); errorText != "" {
		return errorText
	}
	if errorText := textFromValue(event["error"]); errorText != "" {
		return errorText
	}
	eventType := strings.ToLower(textFromValue(event["type"]))
	if strings.Contains(eventType, "error") {
		for _, key := range []string{"message", "content", "text"} {
			if errorText := textFromValue(event[key]); errorText != "" {
				return errorText
			}
		}
	}
	return ""
}

func textFromValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []any:
		var parts []string
		for _, child := range typed {
			if text := textFromValue(child); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	case map[string]any:
		for _, key := range []string{"text", "content", "message", "output_text", "value"} {
			if text := textFromValue(typed[key]); text != "" {
				return text
			}
		}
		return ""
	default:
		return ""
	}
}

func mapValue(value any) (map[string]any, bool) {
	typed, ok := value.(map[string]any)
	return typed, ok
}

func truncateText(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes]) + "..."
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
