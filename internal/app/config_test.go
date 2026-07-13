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

func TestCheckRunConfigAutonomySafetyPolicy(t *testing.T) {
	workDir := t.TempDir()
	writeConfigTestFile(t, workDir, `
autonomy:
  schema_version: revolvr-autonomous-safety-declaration-v1
  mode: fully_unattended
  external_isolation:
    expectation: container
    enforcement: external_attestation
    attestation: {authority: operator, evidence: container-record, sha256: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa}
  network:
    access: denied
    enforcement: external_attestation
    attestation: {authority: operator, evidence: firewall-record, sha256: bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb}
  hooks:
    policy: disabled
  environment:
    inherit_host: false
    allow: [PATH]
  redaction:
    schema_version: revolvr-secret-redaction-policy-v1
    environment_variables: [API_TOKEN]
  acknowledgement: revolvr-fully-unattended-v1:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc
`)
	result, err := CheckRunConfig(workDir)
	if err != nil {
		t.Fatal(err)
	}
	got := result.Effective.SafetyDeclaration
	if got.Mode != "fully_unattended" || got.ExternalIsolation.Attestation == nil || got.Network.Access != "denied" || got.Hooks.Policy != "disabled" || got.Environment.InheritHost || len(got.Redaction.EnvironmentVariables) != 1 {
		t.Fatalf("declaration = %+v", got)
	}
	first := result.EffectiveConfigSHA256
	writeConfigTestFile(t, workDir, strings.ReplaceAll(string(mustReadConfig(t, workDir)), "access: denied", "access: restricted"))
	changed, err := CheckRunConfig(workDir)
	if err != nil || changed.EffectiveConfigSHA256 == first {
		t.Fatalf("material change hash = %q, %v", changed.EffectiveConfigSHA256, err)
	}
}

func TestCheckRunConfigRejectsUnknownAutonomySafetyValues(t *testing.T) {
	for _, test := range []struct{ name, content, want string }{
		{"mode", "autonomy:\n  mode: future\n", "unknown mode"},
		{"network", "autonomy:\n  network:\n    access: future\n", "unknown access"},
		{"hooks", "autonomy:\n  hooks:\n    policy: future\n", "unknown policy"},
		{"unknown field", "autonomy:\n  permission: all\n", "field permission not found"},
		{"ambient and allow", "autonomy:\n  environment:\n    inherit_host: true\n    allow: [PATH]\n", "cannot be combined"},
	} {
		t.Run(test.name, func(t *testing.T) {
			workDir := t.TempDir()
			writeConfigTestFile(t, workDir, test.content)
			if _, err := CheckRunConfig(workDir); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestCheckRunConfigRetentionPolicyAndFingerprint(t *testing.T) {
	workDir := t.TempDir()
	writeConfigTestFile(t, workDir, "retention:\n  schema_version: revolvr-artifact-retention-policy-v1\n  mutation_enabled: true\n  recent_run_count: 3\n  compress_after_seconds: 60\n  prune_after_seconds: 120\n  minimum_compress_bytes: 10\n  prune_compressed_streams: true\n  require_verified_export: true\n  max_files_per_operation: 2\n  max_bytes_per_operation: 1000\n")
	result, err := CheckRunConfig(workDir)
	if err != nil {
		t.Fatal(err)
	}
	p := result.Effective.RetentionPolicy
	if !p.MutationEnabled || p.RecentRunCount != 3 || p.CompressAfter.Seconds() != 60 || !p.PruneCompressedStreams {
		t.Fatalf("retention=%+v", p)
	}
	first := result.EffectiveConfigSHA256
	writeConfigTestFile(t, workDir, strings.ReplaceAll(string(mustReadConfig(t, workDir)), "recent_run_count: 3", "recent_run_count: 4"))
	changed, err := CheckRunConfig(workDir)
	if err != nil || changed.EffectiveConfigSHA256 == first {
		t.Fatalf("retention fingerprint did not change: %v", err)
	}
}

func TestCheckRunConfigRejectsInvalidRetention(t *testing.T) {
	for _, test := range []struct{ name, content, want string }{{"negative", "retention:\n  compress_after_seconds: -1\n", "must be nonnegative"}, {"contradictory", "retention:\n  compress_after_seconds: 20\n  prune_after_seconds: 10\n", "cannot exceed"}, {"unsafe prune", "retention:\n  prune_compressed_streams: true\n  require_verified_export: false\n", "requires a verified"}, {"unknown", "retention:\n  delete_everything: true\n", "field delete_everything not found"}} {
		t.Run(test.name, func(t *testing.T) {
			workDir := t.TempDir()
			writeConfigTestFile(t, workDir, test.content)
			if _, err := CheckRunConfig(workDir); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestCheckRunConfigNotificationPolicyAndFingerprint(t *testing.T) {
	workDir := t.TempDir()
	writeConfigTestFile(t, workDir, `
autonomy:
  redaction:
    environment_variables: [HOOK_TOKEN]
notifications:
  schema_version: revolvr-notification-policy-v1
  enabled: true
  events: [task_completed, task_blocked, task_needs_input, safety_stop, queue_drained, daemon_failed]
  executable: notify-test
  args: [--stdin]
  directory: repository_root
  environment_names: [HOOK_TOKEN]
  timeout_seconds: 4
  stdout_cap_bytes: 100
  stderr_cap_bytes: 101
  maximum_attempts: 3
  retry_delay_seconds: 2
`)
	result, err := CheckRunConfig(workDir)
	if err != nil {
		t.Fatal(err)
	}
	p := result.Effective.NotificationPolicy
	if !p.Enabled || p.Executable != "notify-test" || len(p.Events) != 6 || p.MaximumAttempts != 3 || p.Timeout.Seconds() != 4 || p.EnvironmentNames[0] != "HOOK_TOKEN" {
		t.Fatalf("policy=%+v", p)
	}
	first := result.EffectiveConfigSHA256
	writeConfigTestFile(t, workDir, strings.ReplaceAll(string(mustReadConfig(t, workDir)), "maximum_attempts: 3", "maximum_attempts: 2"))
	changed, err := CheckRunConfig(workDir)
	if err != nil || changed.EffectiveConfigSHA256 == first {
		t.Fatalf("notification fingerprint did not change: %v", err)
	}
}

func TestCheckRunConfigQueueWorkersDefaultBoundsAndFingerprint(t *testing.T) {
	root := t.TempDir()
	base, err := CheckRunConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if base.Effective.QueuePolicy.MaximumWorkers != 1 {
		t.Fatalf("default queue policy=%+v", base.Effective.QueuePolicy)
	}
	if err := os.MkdirAll(filepath.Join(root, ".revolvr"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".revolvr", DefaultConfigFile)
	if err := os.WriteFile(path, []byte("queue:\n  schema_version: autonomous-queue-policy-v1\n  maximum_workers: 3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configured, err := CheckRunConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if configured.Effective.QueuePolicy.MaximumWorkers != 3 || configured.EffectiveConfigSHA256 == base.EffectiveConfigSHA256 {
		t.Fatalf("configured=%+v base_hash=%s", configured, base.EffectiveConfigSHA256)
	}
	for _, value := range []string{"0", "-1", "5"} {
		if err := os.WriteFile(path, []byte("queue:\n  maximum_workers: "+value+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := CheckRunConfig(root); err == nil {
			t.Fatalf("maximum_workers=%s succeeded", value)
		}
	}
	if err := os.WriteFile(path, []byte("queue:\n  unknown: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CheckRunConfig(root); err == nil {
		t.Fatal("unknown queue field succeeded")
	}
}

func TestCheckRunConfigRejectsInvalidNotificationPolicy(t *testing.T) {
	base := "autonomy:\n  redaction:\n    environment_variables: [HOOK_TOKEN]\n"
	for _, test := range []struct{ name, config, want string }{
		{"unknown field", "notifications:\n  typo: true\n", "field typo not found"},
		{"unknown event", "notifications:\n  enabled: true\n  events: [future]\n  executable: hook\n", "unknown"},
		{"duplicate event", "notifications:\n  enabled: true\n  events: [task_completed, task_completed]\n  executable: hook\n", "duplicate event"},
		{"missing executable", "notifications:\n  enabled: true\n  events: [task_completed]\n", "executable is required"},
		{"unredacted environment", "notifications:\n  enabled: true\n  events: [task_completed]\n  executable: hook\n  environment_names: [OTHER]\n  timeout_seconds: 1\n  stdout_cap_bytes: 1\n  stderr_cap_bytes: 1\n  maximum_attempts: 1\n", "not covered"},
		{"excess attempts", "notifications:\n  enabled: true\n  events: [task_completed]\n  executable: hook\n  timeout_seconds: 1\n  stdout_cap_bytes: 1\n  stderr_cap_bytes: 1\n  maximum_attempts: 6\n", "between 1 and"},
		{"disabled material", "notifications:\n  enabled: false\n  executable: hook\n", "disabled policy"},
	} {
		t.Run(test.name, func(t *testing.T) {
			workDir := t.TempDir()
			writeConfigTestFile(t, workDir, base+test.config)
			if _, err := CheckRunConfig(workDir); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v", err)
			}
		})
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
