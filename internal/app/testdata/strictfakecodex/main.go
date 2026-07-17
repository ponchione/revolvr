package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

const (
	contractSchema = "revolvr-strict-fake-codex-contract-v1"
	stateSchema    = "revolvr-strict-fake-codex-state-v1"
)

type material struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type invocation struct {
	Name              string         `json:"name"`
	WorkingDirectory  string         `json:"working_directory"`
	EnvironmentNames  []string       `json:"environment_names"`
	EnvironmentSHA256 string         `json:"environment_sha256"`
	Argv              []string       `json:"argv"`
	Prompt            string         `json:"prompt"`
	PromptPath        string         `json:"prompt_path,omitempty"`
	OutputSchema      *material      `json:"output_schema,omitempty"`
	LastMessage       string         `json:"last_message"`
	Substitutions     []substitution `json:"substitutions,omitempty"`
	StdoutJSONL       []string       `json:"stdout_jsonl"`
	OutputEventTypes  []string       `json:"output_event_types"`
	Receipt           *material      `json:"receipt,omitempty"`
	Writes            []material     `json:"writes,omitempty"`
}

type substitution struct {
	Token       string `json:"token"`
	Heading     string `json:"heading"`
	JSONPointer string `json:"json_pointer,omitempty"`
}

type contract struct {
	SchemaVersion          string       `json:"schema_version"`
	Version                string       `json:"version"`
	VersionWorkingDir      string       `json:"version_working_directory"`
	VersionInvocationCount int          `json:"version_invocation_count"`
	EnvironmentNames       []string     `json:"environment_names"`
	EnvironmentSHA256      string       `json:"environment_sha256"`
	OutputSequence         []string     `json:"output_sequence"`
	Invocations            []invocation `json:"invocations"`
}

type state struct {
	SchemaVersion      string   `json:"schema_version"`
	VersionInvocations int      `json:"version_invocations"`
	NextInvocation     int      `json:"next_invocation"`
	OutputSequence     []string `json:"output_sequence"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "strict fake codex:", err)
		os.Exit(64)
	}
}

func run() error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return fmt.Errorf("resolve executable identity: %w", err)
	}
	contractPath := executable + ".contract.json"
	statePath := executable + ".state.json"
	configured, err := loadContract(contractPath)
	if err != nil {
		return err
	}
	current, err := loadState(statePath)
	if err != nil {
		return err
	}
	if err := validateState(configured, current); err != nil {
		return err
	}

	args := os.Args[1:]
	if slices.Equal(args, []string{"--version"}) {
		if err := validateEnvironment(configured.EnvironmentNames, configured.EnvironmentSHA256); err != nil {
			return err
		}
		return runVersion(configured, current, statePath)
	}
	if current.NextInvocation >= len(configured.Invocations) {
		return fmt.Errorf("unexpected invocation count: got at least %d exec calls, want %d", current.NextInvocation+1, len(configured.Invocations))
	}
	expected := configured.Invocations[current.NextInvocation]
	if err := validateEnvironment(expected.EnvironmentNames, expected.EnvironmentSHA256); err != nil {
		return err
	}
	if err := runInvocation(expected, args); err != nil {
		return fmt.Errorf("invocation %d (%s): %w", current.NextInvocation+1, expected.Name, err)
	}
	current.NextInvocation++
	for _, eventType := range expected.OutputEventTypes {
		current.OutputSequence = append(current.OutputSequence, expected.Name+":"+eventType)
	}
	if err := writeState(statePath, current); err != nil {
		return err
	}
	for _, record := range expected.StdoutJSONL {
		if _, err := fmt.Fprintln(os.Stdout, record); err != nil {
			return fmt.Errorf("write JSONL output: %w", err)
		}
	}
	return nil
}

func loadContract(path string) (contract, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return contract{}, fmt.Errorf("read contract: %w", err)
	}
	var configured contract
	if err := decodeStrict(raw, &configured); err != nil {
		return contract{}, fmt.Errorf("decode contract: %w", err)
	}
	if configured.SchemaVersion != contractSchema {
		return contract{}, fmt.Errorf("unsupported contract schema %q", configured.SchemaVersion)
	}
	if strings.TrimSpace(configured.Version) == "" || strings.ContainsAny(configured.Version, "\r\n") {
		return contract{}, errors.New("contract version must be one nonempty line")
	}
	if configured.VersionInvocationCount < 0 {
		return contract{}, errors.New("version invocation count must be nonnegative")
	}
	if !sort.StringsAreSorted(configured.EnvironmentNames) {
		return contract{}, errors.New("contract environment names must be sorted")
	}
	if len(configured.EnvironmentSHA256) != sha256.Size*2 {
		return contract{}, errors.New("contract environment SHA-256 is invalid")
	}
	var outputSequence []string
	for i, expected := range configured.Invocations {
		if strings.TrimSpace(expected.Name) == "" {
			return contract{}, fmt.Errorf("invocations[%d] name is required", i)
		}
		if err := validateFreshExec(expected.Argv); err != nil {
			return contract{}, fmt.Errorf("invocations[%d] argv: %w", i, err)
		}
		if len(expected.StdoutJSONL) != len(expected.OutputEventTypes) {
			return contract{}, fmt.Errorf("invocations[%d] output sequence has %d records and %d event types", i, len(expected.StdoutJSONL), len(expected.OutputEventTypes))
		}
		for j, record := range expected.StdoutJSONL {
			var event struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal([]byte(record), &event); err != nil {
				return contract{}, fmt.Errorf("invocations[%d] stdout_jsonl[%d]: %w", i, j, err)
			}
			if event.Type != expected.OutputEventTypes[j] {
				return contract{}, fmt.Errorf("invocations[%d] output sequence[%d] type=%q, want %q", i, j, event.Type, expected.OutputEventTypes[j])
			}
			outputSequence = append(outputSequence, expected.Name+":"+event.Type)
		}
	}
	if !slices.Equal(outputSequence, configured.OutputSequence) {
		return contract{}, fmt.Errorf("output sequence=%q, want %q", outputSequence, configured.OutputSequence)
	}
	return configured, nil
}

func decodeStrict(raw []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func loadState(path string) (state, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return state{SchemaVersion: stateSchema}, nil
	}
	if err != nil {
		return state{}, fmt.Errorf("read state: %w", err)
	}
	var current state
	if err := decodeStrict(raw, &current); err != nil {
		return state{}, fmt.Errorf("decode state: %w", err)
	}
	if current.SchemaVersion != stateSchema || current.VersionInvocations < 0 || current.NextInvocation < 0 {
		return state{}, errors.New("invalid state authority")
	}
	return current, nil
}

func writeState(path string, current state) error {
	raw, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	raw = append(raw, '\n')
	temporary := fmt.Sprintf("%s.tmp-%d", path, os.Getpid())
	if err := os.WriteFile(temporary, raw, 0o600); err != nil {
		return fmt.Errorf("write state temporary: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("replace state: %w", err)
	}
	return nil
}

func validateState(configured contract, current state) error {
	if current.VersionInvocations > configured.VersionInvocationCount {
		return fmt.Errorf("version invocation count=%d exceeds contract count %d", current.VersionInvocations, configured.VersionInvocationCount)
	}
	if current.NextInvocation > len(configured.Invocations) {
		return fmt.Errorf("exec invocation count=%d exceeds contract count %d", current.NextInvocation, len(configured.Invocations))
	}
	var expectedSequence []string
	for _, completed := range configured.Invocations[:current.NextInvocation] {
		for _, eventType := range completed.OutputEventTypes {
			expectedSequence = append(expectedSequence, completed.Name+":"+eventType)
		}
	}
	if !slices.Equal(current.OutputSequence, expectedSequence) {
		return fmt.Errorf("recorded output sequence=%q, want completed prefix %q", current.OutputSequence, expectedSequence)
	}
	return nil
}

func runVersion(configured contract, current state, statePath string) error {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve version working directory: %w", err)
	}
	if workingDirectory != configured.VersionWorkingDir {
		return fmt.Errorf("version working directory=%q, want %q", workingDirectory, configured.VersionWorkingDir)
	}
	if current.VersionInvocations >= configured.VersionInvocationCount {
		return fmt.Errorf("unexpected version invocation count: got at least %d, want %d", current.VersionInvocations+1, configured.VersionInvocationCount)
	}
	current.VersionInvocations++
	if err := writeState(statePath, current); err != nil {
		return err
	}
	_, err = fmt.Fprintln(os.Stdout, configured.Version)
	return err
}

func runInvocation(expected invocation, args []string) error {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve working directory: %w", err)
	}
	if workingDirectory != expected.WorkingDirectory {
		return fmt.Errorf("working directory=%q, want %q", workingDirectory, expected.WorkingDirectory)
	}
	if err := validateFreshExec(args); err != nil {
		return err
	}
	if !slices.Equal(args, expected.Argv) {
		return fmt.Errorf("argv=%q, want %q", args, expected.Argv)
	}
	cd, err := oneFlagValue(args, "--cd")
	if err != nil {
		return err
	}
	if cd != expected.WorkingDirectory {
		return fmt.Errorf("--cd=%q, want %q", cd, expected.WorkingDirectory)
	}
	schemaPath, schemaFound, err := optionalFlagValue(args, "--output-schema")
	if err != nil {
		return err
	}
	if expected.OutputSchema == nil && schemaFound {
		return errors.New("unexpected output schema")
	}
	if expected.OutputSchema != nil {
		if !schemaFound || schemaPath != expected.OutputSchema.Path {
			return fmt.Errorf("output schema=%q, want %q", schemaPath, expected.OutputSchema.Path)
		}
		raw, err := os.ReadFile(schemaPath)
		if err != nil {
			return fmt.Errorf("read output schema: %w", err)
		}
		if string(raw) != expected.OutputSchema.Content {
			return errors.New("output schema bytes differ from contract")
		}
	}
	prompt, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read prompt: %w", err)
	}
	expectedPrompt := expected.Prompt
	if expected.PromptPath != "" {
		raw, err := os.ReadFile(expected.PromptPath)
		if err != nil {
			return fmt.Errorf("read prompt artifact: %w", err)
		}
		expectedPrompt = string(raw)
	}
	if string(prompt) != expectedPrompt {
		return errors.New("prompt bytes differ from invocation contract")
	}
	lastMessagePath, err := oneFlagValue(args, "--output-last-message")
	if err != nil {
		return err
	}
	if err := requireRestrictedRegular(lastMessagePath); err != nil {
		return fmt.Errorf("last-message output: %w", err)
	}
	if expected.Receipt != nil {
		if err := requireExistingParent(expected.Receipt.Path); err != nil {
			return fmt.Errorf("receipt output: %w", err)
		}
		if err := os.WriteFile(expected.Receipt.Path, []byte(expected.Receipt.Content), 0o600); err != nil {
			return fmt.Errorf("write receipt: %w", err)
		}
	}
	for _, write := range expected.Writes {
		path, err := resolveWritePath(expected.WorkingDirectory, write.Path)
		if err != nil {
			return fmt.Errorf("source write: %w", err)
		}
		if err := requireExistingParent(path); err != nil {
			return fmt.Errorf("source write: %w", err)
		}
		if err := os.WriteFile(path, []byte(write.Content), 0o644); err != nil {
			return fmt.Errorf("source write: %w", err)
		}
	}
	lastMessage := expected.LastMessage
	for _, replacement := range expected.Substitutions {
		value, err := promptJSONValue(expectedPrompt, replacement.Heading, replacement.JSONPointer)
		if err != nil {
			return fmt.Errorf("last-message substitution %q: %w", replacement.Token, err)
		}
		if strings.Count(lastMessage, replacement.Token) != 1 {
			return fmt.Errorf("last-message substitution token %q must occur exactly once", replacement.Token)
		}
		lastMessage = strings.Replace(lastMessage, replacement.Token, value, 1)
	}
	if err := os.WriteFile(lastMessagePath, []byte(lastMessage), 0o600); err != nil {
		return fmt.Errorf("write last message: %w", err)
	}
	return nil
}

func resolveWritePath(root, path string) (string, error) {
	if filepath.IsAbs(path) || filepath.Clean(path) != path || path == "." || path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe relative path %q", path)
	}
	resolved := filepath.Join(root, path)
	relative, err := filepath.Rel(root, resolved)
	if err != nil || relative != path {
		return "", fmt.Errorf("path %q escapes working directory", path)
	}
	return resolved, nil
}

func promptJSONValue(prompt, heading, pointer string) (string, error) {
	marker := "## " + heading
	start := strings.Index(prompt, marker)
	if start < 0 {
		return "", fmt.Errorf("heading %q is absent", heading)
	}
	block := prompt[start+len(marker):]
	open := strings.Index(block, "```json\n")
	if open < 0 {
		return "", errors.New("JSON fence is absent")
	}
	block = block[open+len("```json\n"):]
	close := strings.Index(block, "\n```")
	if close < 0 {
		return "", errors.New("JSON fence is unterminated")
	}
	var value any
	if err := json.Unmarshal([]byte(block[:close]), &value); err != nil {
		return "", fmt.Errorf("decode JSON fence: %w", err)
	}
	for _, part := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		if part == "" {
			continue
		}
		object, ok := value.(map[string]any)
		if !ok {
			return "", fmt.Errorf("JSON pointer %q does not select an object", pointer)
		}
		value, ok = object[strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")]
		if !ok {
			return "", fmt.Errorf("JSON pointer %q is absent", pointer)
		}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func validateFreshExec(args []string) error {
	if len(args) == 0 || args[0] != "exec" {
		return errors.New("fresh invocation must start with exec")
	}
	if slices.Contains(args, "resume") {
		return errors.New("resume is forbidden")
	}
	if count(args, "--ephemeral") != 1 {
		return errors.New("fresh invocation requires exactly one --ephemeral")
	}
	if count(args, "--json") != 1 {
		return errors.New("fresh invocation requires exactly one --json")
	}
	if count(args, "exec") != 1 {
		return errors.New("fresh invocation requires exactly one exec subcommand")
	}
	if count(args, "-") != 1 || args[len(args)-1] != "-" {
		return errors.New("fresh invocation requires one final stdin marker")
	}
	return nil
}

func oneFlagValue(args []string, flag string) (string, error) {
	value, found, err := optionalFlagValue(args, flag)
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("missing %s", flag)
	}
	return value, nil
}

func optionalFlagValue(args []string, flag string) (string, bool, error) {
	var value string
	found := false
	for i := 0; i < len(args); i++ {
		if args[i] != flag {
			continue
		}
		if found || i+1 >= len(args) {
			return "", false, fmt.Errorf("%s must occur exactly once with a value", flag)
		}
		found = true
		value = args[i+1]
		i++
	}
	return value, found, nil
}

func count(values []string, want string) int {
	count := 0
	for _, value := range values {
		if value == want {
			count++
		}
	}
	return count
}

func validateEnvironment(expectedNames []string, expectedSHA256 string) error {
	actual := append([]string(nil), os.Environ()...)
	sort.Strings(actual)
	actualNames := environmentNames(actual)
	if slices.Equal(actualNames, expectedNames) && environmentSHA256(actual) == expectedSHA256 {
		return nil
	}
	return fmt.Errorf("environment differs from contract (actual names=%v expected names=%v)", actualNames, expectedNames)
}

func environmentNames(environment []string) []string {
	names := make([]string, len(environment))
	for i, value := range environment {
		name, _, _ := strings.Cut(value, "=")
		names[i] = name
	}
	return names
}

func environmentSHA256(environment []string) string {
	raw, _ := json.Marshal(environment)
	digest := sha256.Sum256(raw)
	return fmt.Sprintf("%x", digest[:])
}

func requireRestrictedRegular(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return fmt.Errorf("path must be a mode-0600 regular file (mode=%s)", info.Mode())
	}
	return nil
}

func requireExistingParent(path string) error {
	parent := filepath.Dir(path)
	info, err := os.Stat(parent)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("parent is not a directory")
	}
	return nil
}
