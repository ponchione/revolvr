package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/codexexec"
	"revolvr/internal/receipt"
	"revolvr/internal/runner"
	"revolvr/internal/supervisor"
)

const (
	strictFakeCodexContractSchema = "revolvr-strict-fake-codex-contract-v1"
	strictFakeCodexStateSchema    = "revolvr-strict-fake-codex-state-v1"
	strictFakeCodexVersion        = "codex-cli 0.0.0"
)

type strictFakeCodexMaterial struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type strictFakeCodexInvocation struct {
	Name              string                    `json:"name"`
	WorkingDirectory  string                    `json:"working_directory"`
	EnvironmentNames  []string                  `json:"environment_names"`
	EnvironmentSHA256 string                    `json:"environment_sha256"`
	Argv              []string                  `json:"argv"`
	Prompt            string                    `json:"prompt"`
	PromptPath        string                    `json:"prompt_path,omitempty"`
	OutputSchema      *strictFakeCodexMaterial  `json:"output_schema,omitempty"`
	LastMessage       string                    `json:"last_message"`
	Substitutions     []strictFakeSubstitution  `json:"substitutions,omitempty"`
	StdoutJSONL       []string                  `json:"stdout_jsonl"`
	OutputEventTypes  []string                  `json:"output_event_types"`
	Receipt           *strictFakeCodexMaterial  `json:"receipt,omitempty"`
	Writes            []strictFakeCodexMaterial `json:"writes,omitempty"`
}

type strictFakeSubstitution struct {
	Token       string `json:"token"`
	Heading     string `json:"heading"`
	JSONPointer string `json:"json_pointer,omitempty"`
}

type strictFakeCodexContract struct {
	SchemaVersion          string                      `json:"schema_version"`
	Version                string                      `json:"version"`
	VersionWorkingDir      string                      `json:"version_working_directory"`
	VersionInvocationCount int                         `json:"version_invocation_count"`
	EnvironmentNames       []string                    `json:"environment_names"`
	EnvironmentSHA256      string                      `json:"environment_sha256"`
	OutputSequence         []string                    `json:"output_sequence"`
	Invocations            []strictFakeCodexInvocation `json:"invocations"`
}

type strictFakeCodexState struct {
	SchemaVersion      string   `json:"schema_version"`
	VersionInvocations int      `json:"version_invocations"`
	NextInvocation     int      `json:"next_invocation"`
	OutputSequence     []string `json:"output_sequence"`
}

type strictFakeCodexFixture struct {
	Executable   string
	ContractPath string
	StatePath    string
}

func TestStrictFakeCodexContract(t *testing.T) {
	root := t.TempDir()
	executable := buildStrictFakeCodex(t)
	schema, err := supervisor.DecisionOutputSchema()
	if err != nil {
		t.Fatal(err)
	}
	schemaPath := filepath.Join(root, "supervisor-output-schema.json")
	if err := os.WriteFile(schemaPath, schema, 0o644); err != nil {
		t.Fatal(err)
	}
	receiptPath := filepath.Join(root, "worker", "receipt.md")
	if err := os.MkdirAll(filepath.Dir(receiptPath), 0o755); err != nil {
		t.Fatal(err)
	}

	decision := autonomous.SupervisorDecision{
		TaskID:          "strict-contract-task",
		Action:          autonomous.ActionImplement,
		WorkerProfile:   autonomous.WorkerProfileImplementer,
		Rationale:       "The strict contract requests one deterministic worker invocation.",
		SuccessCriteria: []string{"Emit deterministic worker and receipt material."},
		Inputs:          []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindTask, Reference: ".agent/tasks/strict-contract-task.md", Detail: "Exact strict fixture task."}},
	}
	if err := decision.Validate(); err != nil {
		t.Fatal(err)
	}
	decisionJSON, err := json.Marshal(decision)
	if err != nil {
		t.Fatal(err)
	}
	decisionJSON = append(decisionJSON, '\n')
	workerJSON := "{\"summary\":\"strict fake worker completed\",\"changed_files\":[\"fixture.txt\"]}\n"
	receiptContent, _ := receipt.FormatFallbackReceipt(receipt.FallbackInput{
		RunID:              "strict-worker-run",
		PassID:             "strict-worker-run",
		TaskID:             "strict-contract-task",
		Task:               "Exercise the strict reusable fake Codex contract.",
		Verdict:            receipt.VerdictCompleted,
		Timestamp:          time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC),
		CodexExitCode:      0,
		VerificationStatus: "passed",
		CommitSHA:          "",
		ChangedFiles:       []string{"fixture.txt"},
		Verification:       []receipt.VerificationEntry{{Command: "test -f fixture.txt", ExitCode: 0, Status: "passed"}},
		Metrics:            receipt.Metrics{InputTokens: 17, OutputTokens: 9, DurationSeconds: 1},
		FinalText:          "Strict fake Codex emitted deterministic worker material.",
	})

	supervisorPrompt := "# Strict fake supervisor prompt\n\nTask: strict-contract-task\n"
	workerPrompt := "# Strict fake worker prompt\n\nReceipt: " + receiptPath + "\n"
	supervisorConfig := strictFakeRunConfig(root, executable, supervisorPrompt, "supervisor", filepath.Base(schemaPath))
	workerConfig := strictFakeRunConfig(root, executable, workerPrompt, "worker", "")
	supervisorProvenance := prepareStrictFakeInvocation(t, supervisorConfig)
	workerProvenance := prepareStrictFakeInvocation(t, workerConfig)
	supervisorEvents := []string{
		`{"type":"thread.started","thread_id":"strict-supervisor-thread"}`,
		`{"type":"turn.completed","final_message":"strict supervisor completed","usage":{"input_tokens":11,"output_tokens":5,"duration_seconds":1}}`,
	}
	workerEvents := []string{
		`{"type":"thread.started","thread_id":"strict-worker-thread"}`,
		`{"type":"turn.completed","final_message":"strict worker completed","usage":{"input_tokens":17,"output_tokens":9,"duration_seconds":1}}`,
	}
	contract := strictFakeCodexContract{
		VersionInvocationCount: 1,
		OutputSequence: []string{
			"supervisor:thread.started",
			"supervisor:turn.completed",
			"worker:thread.started",
			"worker:turn.completed",
		},
		Invocations: []strictFakeCodexInvocation{
			{
				Name:             "supervisor",
				WorkingDirectory: root,
				Argv:             supervisorProvenance.Argv,
				Prompt:           supervisorPrompt,
				OutputSchema:     &strictFakeCodexMaterial{Path: schemaPath, Content: string(schema)},
				LastMessage:      string(decisionJSON),
				StdoutJSONL:      supervisorEvents,
				OutputEventTypes: []string{"thread.started", "turn.completed"},
			},
			{
				Name:             "worker",
				WorkingDirectory: root,
				Argv:             workerProvenance.Argv,
				Prompt:           workerPrompt,
				LastMessage:      workerJSON,
				StdoutJSONL:      workerEvents,
				OutputEventTypes: []string{"thread.started", "turn.completed"},
				Receipt:          &strictFakeCodexMaterial{Path: receiptPath, Content: receiptContent},
			},
		},
	}
	fixture := configureStrictFakeCodex(t, executable, root, contract)

	version, err := codexexec.DiscoverVersion(context.Background(), codexexec.VersionConfig{Executable: executable, WorkingDir: root})
	if err != nil {
		t.Fatal(err)
	}
	if version != strictFakeCodexVersion {
		t.Fatalf("version = %q, want %q", version, strictFakeCodexVersion)
	}

	supervisorResult, err := codexexec.Run(context.Background(), supervisorConfig)
	if err != nil {
		t.Fatal(err)
	}
	workerResult, err := codexexec.Run(context.Background(), workerConfig)
	if err != nil {
		t.Fatal(err)
	}
	for name, result := range map[string]codexexec.Result{"supervisor": supervisorResult, "worker": workerResult} {
		if result.Err != nil || result.ExitCode != 0 || result.JSONEvents != 2 || len(result.JSONParseErrors) != 0 || !result.UsageFound {
			t.Fatalf("%s result = %+v", name, result)
		}
	}
	if supervisorResult.FinalMessage != strings.TrimSpace(string(decisionJSON)) {
		t.Fatalf("supervisor last message = %q", supervisorResult.FinalMessage)
	}
	var parsedDecision autonomous.SupervisorDecision
	if err := json.Unmarshal([]byte(supervisorResult.FinalMessage), &parsedDecision); err != nil {
		t.Fatal(err)
	}
	if err := parsedDecision.Validate(); err != nil {
		t.Fatal(err)
	}
	if workerResult.FinalMessage != strings.TrimSpace(workerJSON) {
		t.Fatalf("worker last message = %q", workerResult.FinalMessage)
	}
	var parsedWorker map[string]any
	if err := json.Unmarshal([]byte(workerResult.FinalMessage), &parsedWorker); err != nil {
		t.Fatalf("parse worker JSON: %v", err)
	}
	assertStrictFakeArtifact(t, supervisorResult.Artifacts.StdoutJSONL, strings.Join(supervisorEvents, "\n")+"\n")
	assertStrictFakeArtifact(t, workerResult.Artifacts.StdoutJSONL, strings.Join(workerEvents, "\n")+"\n")
	assertStrictFakeArtifact(t, receiptPath, receiptContent)
	if _, err := receipt.Parse([]byte(receiptContent)); err != nil {
		t.Fatalf("parse fixture receipt: %v", err)
	}
	state := fixture.loadState(t)
	wantState := strictFakeCodexState{SchemaVersion: strictFakeCodexStateSchema, VersionInvocations: 1, NextInvocation: 2, OutputSequence: append([]string(nil), contract.OutputSequence...)}
	if !reflect.DeepEqual(state, wantState) {
		t.Fatalf("fixture state = %+v, want %+v", state, wantState)
	}

	extra := runStrictFakeCommand(root, executable, supervisorProvenance.Argv, supervisorPrompt, nil, false)
	assertStrictFakeRefusal(t, extra, "unexpected invocation count")

	t.Run("rejects contract drift", func(t *testing.T) {
		base := contract
		base.VersionInvocationCount = 0
		base.OutputSequence = []string{"supervisor:thread.started", "supervisor:turn.completed"}
		base.Invocations = append([]strictFakeCodexInvocation(nil), contract.Invocations[:1]...)
		for _, test := range []struct {
			name               string
			configure          func(strictFakeCodexContract) strictFakeCodexContract
			args               func([]string) []string
			dir                string
			env                []string
			replaceEnvironment bool
			mutate             func(*testing.T)
			want               string
		}{
			{name: "argv", args: func(args []string) []string {
				return append(append(append([]string(nil), args[:len(args)-1]...), "--unexpected"), args[len(args)-1])
			}, want: "argv="},
			{name: "working directory", dir: t.TempDir(), env: strictFakeEnvironment(root), replaceEnvironment: true, want: "working directory="},
			{name: "schema", mutate: func(t *testing.T) {
				t.Helper()
				if err := os.WriteFile(schemaPath, []byte("{}\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}, want: "output schema bytes differ"},
			{name: "environment", env: []string{"REVOLVR_STRICT_FAKE_UNEXPECTED=1"}, want: "environment differs"},
			{name: "invocation count", configure: func(value strictFakeCodexContract) strictFakeCodexContract {
				value.Invocations = nil
				value.OutputSequence = nil
				return value
			}, want: "unexpected invocation count"},
			{name: "output sequence", configure: func(value strictFakeCodexContract) strictFakeCodexContract {
				value.OutputSequence = []string{"worker:turn.completed"}
				return value
			}, want: "output sequence="},
			{name: "non-ephemeral", args: func(args []string) []string { return removeStrictFakeArg(args, "--ephemeral") }, want: "exactly one --ephemeral"},
		} {
			t.Run(test.name, func(t *testing.T) {
				configured := base
				if test.configure != nil {
					configured = test.configure(configured)
				}
				configureStrictFakeCodex(t, executable, root, configured)
				if test.mutate != nil {
					test.mutate(t)
					defer func() {
						if err := os.WriteFile(schemaPath, schema, 0o644); err != nil {
							t.Fatal(err)
						}
					}()
				}
				args := append([]string(nil), supervisorProvenance.Argv...)
				if test.args != nil {
					args = test.args(args)
				}
				dir := root
				if test.dir != "" {
					dir = test.dir
				}
				result := runStrictFakeCommand(dir, executable, args, supervisorPrompt, test.env, test.replaceEnvironment)
				assertStrictFakeRefusal(t, result, test.want)
			})
		}
	})
}

func strictFakeRunConfig(root, executable, prompt, name, outputSchema string) codexexec.Config {
	ephemeral := true
	return codexexec.Config{
		Executable:                executable,
		WorkingDir:                root,
		Prompt:                    prompt,
		Model:                     codexexec.DefaultModel,
		ReasoningEffort:           codexexec.DefaultReasoningEffort,
		Ephemeral:                 &ephemeral,
		Timeout:                   10 * time.Second,
		StdoutCap:                 64 * 1024,
		StderrCap:                 64 * 1024,
		BypassApprovalsAndSandbox: true,
		Artifacts: codexexec.ArtifactPaths{
			StdoutJSONL: filepath.Join("artifacts", name, "stdout.jsonl"),
			Stderr:      filepath.Join("artifacts", name, "stderr.log"),
			LastMessage: filepath.Join("artifacts", name, "last-message.txt"),
		},
		OutputSchema: outputSchema,
	}
}

func prepareStrictFakeInvocation(t *testing.T, cfg codexexec.Config) codexexec.InvocationProvenance {
	t.Helper()
	prepared, _, err := codexexec.PrepareInvocation(codexexec.InvocationConfig{
		Executable:             cfg.Executable,
		WorkingDir:             cfg.WorkingDir,
		ArtifactRoot:           cfg.ArtifactRoot,
		Model:                  cfg.Model,
		ReasoningEffort:        cfg.ReasoningEffort,
		Ephemeral:              cfg.Ephemeral != nil && *cfg.Ephemeral,
		Sandbox:                cfg.Sandbox,
		ApprovalPolicy:         cfg.ApprovalPolicy,
		BypassApprovalsSandbox: cfg.BypassApprovalsAndSandbox,
		Artifacts:              cfg.Artifacts,
		OutputSchema:           cfg.OutputSchema,
	})
	if err != nil {
		t.Fatal(err)
	}
	return prepared
}

func buildStrictFakeCodex(t *testing.T) string {
	t.Helper()
	executable := filepath.Join(t.TempDir(), "strict-fake-codex")
	command := exec.Command("go", "build", "-o", executable, "./testdata/strictfakecodex")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("build strict fake Codex: %v\n%s", err, output)
	}
	return executable
}

func configureStrictFakeCodex(t *testing.T, executable, root string, contract strictFakeCodexContract) strictFakeCodexFixture {
	t.Helper()
	contract.SchemaVersion = strictFakeCodexContractSchema
	contract.Version = strictFakeCodexVersion
	contract.VersionWorkingDir = root
	environment := strictFakeEnvironment(root)
	contract.EnvironmentNames = strictFakeEnvironmentNames(environment)
	contract.EnvironmentSHA256 = strictFakeEnvironmentSHA256(environment)
	for i := range contract.Invocations {
		environment := strictFakeEnvironment(contract.Invocations[i].WorkingDirectory)
		contract.Invocations[i].EnvironmentNames = strictFakeEnvironmentNames(environment)
		contract.Invocations[i].EnvironmentSHA256 = strictFakeEnvironmentSHA256(environment)
	}
	raw, err := json.MarshalIndent(contract, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	fixture := strictFakeCodexFixture{Executable: executable, ContractPath: executable + ".contract.json", StatePath: executable + ".state.json"}
	if err := os.WriteFile(fixture.ContractPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(fixture.StatePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return fixture
}

func strictFakeEnvironment(workingDirectory string) []string {
	environment := make([]string, 0, len(os.Environ())+1)
	for _, value := range os.Environ() {
		if strings.HasPrefix(value, "PWD=") {
			continue
		}
		environment = append(environment, value)
	}
	environment = append(environment, "PWD="+workingDirectory)
	sort.Strings(environment)
	return environment
}

func strictFakeEnvironmentNames(environment []string) []string {
	names := make([]string, len(environment))
	for i, value := range environment {
		name, _, _ := strings.Cut(value, "=")
		names[i] = name
	}
	return names
}

func strictFakeEnvironmentSHA256(environment []string) string {
	raw, err := json.Marshal(environment)
	if err != nil {
		panic(err)
	}
	digest := sha256.Sum256(raw)
	return strings.ToLower(fmt.Sprintf("%x", digest[:]))
}

func (fixture strictFakeCodexFixture) loadState(t *testing.T) strictFakeCodexState {
	t.Helper()
	raw, err := os.ReadFile(fixture.StatePath)
	if err != nil {
		t.Fatal(err)
	}
	var state strictFakeCodexState
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	return state
}

func runStrictFakeCommand(dir, executable string, args []string, prompt string, environment []string, replaceEnvironment bool) runner.Result {
	return runner.Run(context.Background(), runner.Command{
		Name:        executable,
		Args:        args,
		Stdin:       strings.NewReader(prompt),
		Dir:         dir,
		Env:         environment,
		ReplaceEnv:  replaceEnvironment,
		Timeout:     10 * time.Second,
		StdoutLimit: 64 * 1024,
		StderrLimit: 64 * 1024,
	})
}

func assertStrictFakeRefusal(t *testing.T, result runner.Result, want string) {
	t.Helper()
	if result.Err != nil || result.ExitCode != 64 || !strings.Contains(result.Stderr, want) {
		t.Fatalf("strict fake refusal = %+v, want exit 64 containing %q", result, want)
	}
}

func assertStrictFakeArtifact(t *testing.T, path, want string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != want {
		t.Fatalf("artifact %s = %q, want %q", path, raw, want)
	}
}

func removeStrictFakeArg(args []string, remove string) []string {
	result := make([]string, 0, len(args))
	removed := false
	for _, arg := range args {
		if !removed && arg == remove {
			removed = true
			continue
		}
		result = append(result, arg)
	}
	return result
}
