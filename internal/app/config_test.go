package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckRunConfigCodexDefaults(t *testing.T) {
	result, err := CheckRunConfig(t.TempDir())
	if err != nil {
		t.Fatalf("check run config: %v", err)
	}
	if result.Effective.CodexExecutable != DefaultCodexExecutable ||
		result.Effective.CodexModel != DefaultCodexModel ||
		result.Effective.CodexReasoningEffort != DefaultCodexReasoningEffort ||
		!result.Effective.CodexEphemeral {
		t.Fatalf("Codex defaults = %+v", result.Effective)
	}
	if result.EffectiveConfigSchema == "" || len(result.EffectiveConfigSHA256) != 64 {
		t.Fatalf("effective config provenance = schema %q hash %q", result.EffectiveConfigSchema, result.EffectiveConfigSHA256)
	}
}

func TestCheckRunConfigCodexOverrides(t *testing.T) {
	workDir := t.TempDir()
	writeConfigTestFile(t, workDir, `
codex:
  executable: codex-custom
  model: gpt-custom
  reasoning_effort: high
  ephemeral: true
  yolo: false
`)
	result, err := CheckRunConfig(workDir)
	if err != nil {
		t.Fatalf("check run config: %v", err)
	}
	got := result.Effective
	if got.CodexExecutable != "codex-custom" || got.CodexModel != "gpt-custom" || got.CodexReasoningEffort != "high" || !got.CodexEphemeral || got.CodexBypassApprovalsAndSandbox {
		t.Fatalf("Codex overrides = %+v", got)
	}
}

func TestCheckRunConfigRejectsInvalidCodexSettings(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "empty model", content: "codex:\n  model: \"\"\n", want: "codex model must not be empty"},
		{name: "whitespace model", content: "codex:\n  model: \"   \"\n", want: "codex model must not be empty"},
		{name: "malformed model", content: "codex:\n  model: \"gpt custom\"\n", want: "whitespace and control characters are not allowed"},
		{name: "empty effort", content: "codex:\n  reasoning_effort: \"\"\n", want: "codex reasoning_effort must not be empty"},
		{name: "whitespace effort", content: "codex:\n  reasoning_effort: \"   \"\n", want: "codex reasoning_effort must not be empty"},
		{name: "unknown effort", content: "codex:\n  reasoning_effort: extreme\n", want: "invalid Codex reasoning effort"},
		{name: "persistent session", content: "codex:\n  ephemeral: false\n", want: "persistent or resumed sessions are not supported"},
		{name: "unknown field", content: "codex:\n  session: resume\n", want: "field session not found"},
		{name: "conflicting aliases", content: "codex:\n  yolo: true\n  dangerously_bypass_approvals_and_sandbox: true\n", want: "cannot both be set"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workDir := t.TempDir()
			writeConfigTestFile(t, workDir, tt.content)
			_, err := CheckRunConfig(workDir)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("check run config error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestCheckRunConfigPreservesBypassAliases(t *testing.T) {
	for _, field := range []string{"yolo", "dangerously_bypass_approvals_and_sandbox"} {
		t.Run(field, func(t *testing.T) {
			workDir := t.TempDir()
			writeConfigTestFile(t, workDir, "codex:\n  "+field+": false\n")
			result, err := CheckRunConfig(workDir)
			if err != nil {
				t.Fatalf("check run config: %v", err)
			}
			if result.Effective.CodexBypassApprovalsAndSandbox {
				t.Fatalf("%s=false did not disable bypass", field)
			}
		})
	}
}

func TestCheckRunConfigTieredVerificationAndConflict(t *testing.T) {
	workDir := t.TempDir()
	writeConfigTestFile(t, workDir, `
verification:
  tiers:
    - id: structural
      kind: structural
      required_for_final: true
      run_for_fast: true
      run_for_final: true
      rerun_policy: once_to_classify_flaky
      commands:
        - name: go
          args: [test, ./internal/app]
          dir: .
          env: [MODE=test]
          timeout_seconds: 12
          stdout_cap_bytes: 123
          stderr_cap_bytes: 124
`)
	result, err := CheckRunConfig(workDir)
	if err != nil {
		t.Fatal(err)
	}
	if result.Effective.VerificationPlan == nil || len(result.Effective.VerificationPlan.Tiers) != 1 || len(result.Effective.VerificationCommands) != 0 {
		t.Fatalf("effective=%+v", result.Effective)
	}
	command := result.Effective.VerificationPlan.Tiers[0].Commands[0]
	if command.Env[0] != "MODE=test" || command.StdoutCap != 123 || command.StderrCap != 124 {
		t.Fatalf("command=%+v", command)
	}
	firstHash := result.EffectiveConfigSHA256
	writeConfigTestFile(t, workDir, strings.ReplaceAll(string(mustReadConfig(t, workDir)), "MODE=test", "MODE=other"))
	second, err := CheckRunConfig(workDir)
	if err != nil || second.EffectiveConfigSHA256 == firstHash {
		t.Fatalf("changed tier hash=%q err=%v", second.EffectiveConfigSHA256, err)
	}

	writeConfigTestFile(t, workDir, `verification:
  commands: [{name: go}]
  tiers: [{id: structural, kind: structural, required_for_final: true, run_for_final: true, rerun_policy: never, commands: [{name: go}]}]
`)
	if _, err := CheckRunConfig(workDir); err == nil || !strings.Contains(err.Error(), "cannot both be set") {
		t.Fatalf("conflict error=%v", err)
	}
}

func mustReadConfig(t *testing.T, workDir string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(workDir, ".revolvr", DefaultConfigFile))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func writeConfigTestFile(t *testing.T, workDir string, content string) {
	t.Helper()
	path := filepath.Join(workDir, ".revolvr", DefaultConfigFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create config directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}
