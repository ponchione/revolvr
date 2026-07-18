package app

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"revolvr/internal/artifactretention"
	"revolvr/internal/autonomousnotification"
	"revolvr/internal/autonomousqueue"
	"revolvr/internal/verification"
)

type numericConfigField struct {
	path   string
	render func(string) string
}

func TestConfigPositiveOnlyNumbersRejectInvalidExplicitValues(t *testing.T) {
	for _, field := range positiveNumericConfigFields() {
		field := field
		for _, value := range []string{"0", "-1", "invalid", "null"} {
			value := value
			t.Run(field.path+"/"+value, func(t *testing.T) {
				assertConfigErrorFromLoadAndCheck(t, field.render(value), field.path)
			})
		}
	}
}

func TestConfigNonNegativeNumbersAcceptZeroAndRejectInvalidValues(t *testing.T) {
	fields := []numericConfigField{
		{path: "notifications.retry_delay_seconds", render: scalarConfig("notifications", "retry_delay_seconds")},
		{path: "retention.recent_run_count", render: scalarConfig("retention", "recent_run_count")},
		{path: "retention.compress_after_seconds", render: scalarConfig("retention", "compress_after_seconds")},
		{path: "retention.prune_after_seconds", render: scalarConfig("retention", "prune_after_seconds")},
		{path: "retention.minimum_compress_bytes", render: scalarConfig("retention", "minimum_compress_bytes")},
	}
	for _, field := range fields {
		field := field
		t.Run(field.path+"/zero", func(t *testing.T) {
			assertConfigSuccessFromLoadAndCheck(t, field.render("0"))
		})
		for _, value := range []string{"-1", "invalid", "null"} {
			value := value
			t.Run(field.path+"/"+value, func(t *testing.T) {
				assertConfigErrorFromLoadAndCheck(t, field.render(value), field.path)
			})
		}
	}
}

func TestConfigDurationBoundaries(t *testing.T) {
	overflow := fmt.Sprint(maximumDurationSeconds + 1)
	fields := []struct {
		path                string
		maximumConfig       string
		maximumCheckFailure bool
		overflowConfig      string
	}{
		{path: "codex.timeout_seconds", maximumConfig: scalarConfig("codex", "timeout_seconds")(fmt.Sprint(maximumDurationSeconds)), maximumCheckFailure: true, overflowConfig: scalarConfig("codex", "timeout_seconds")(overflow)},
		{path: "git.timeout_seconds", maximumConfig: scalarConfig("git", "timeout_seconds")(fmt.Sprint(maximumDurationSeconds)), maximumCheckFailure: true, overflowConfig: scalarConfig("git", "timeout_seconds")(overflow)},
		{path: "verification.commands[0].timeout_seconds", maximumConfig: verificationCommandConfig(false, "timeout_seconds", fmt.Sprint(maximumDurationSeconds)), overflowConfig: verificationCommandConfig(false, "timeout_seconds", overflow)},
		{path: "verification.tiers[0].commands[0].timeout_seconds", maximumConfig: verificationCommandConfig(true, "timeout_seconds", fmt.Sprint(maximumDurationSeconds)), overflowConfig: verificationCommandConfig(true, "timeout_seconds", overflow)},
		{path: "commit.timeout_seconds", maximumConfig: scalarConfig("commit", "timeout_seconds")(fmt.Sprint(maximumDurationSeconds)), overflowConfig: scalarConfig("commit", "timeout_seconds")(overflow)},
		{path: "notifications.timeout_seconds", maximumConfig: validNotificationConfig("300", "0"), overflowConfig: scalarConfig("notifications", "timeout_seconds")(overflow)},
		{path: "notifications.retry_delay_seconds", maximumConfig: validNotificationConfig("1", "60"), overflowConfig: scalarConfig("notifications", "retry_delay_seconds")(overflow)},
		{path: "retention.compress_after_seconds", maximumConfig: fmt.Sprintf("retention:\n  compress_after_seconds: %d\n  prune_after_seconds: %d\n", maximumDurationSeconds, maximumDurationSeconds), overflowConfig: scalarConfig("retention", "compress_after_seconds")(overflow)},
		{path: "retention.prune_after_seconds", maximumConfig: scalarConfig("retention", "prune_after_seconds")(fmt.Sprint(maximumDurationSeconds)), overflowConfig: scalarConfig("retention", "prune_after_seconds")(overflow)},
	}
	for _, field := range fields {
		field := field
		t.Run(field.path+"/maximum", func(t *testing.T) {
			if !field.maximumCheckFailure {
				assertConfigSuccessFromLoadAndCheck(t, field.maximumConfig)
				return
			}
			root := t.TempDir()
			writeConfigTestFile(t, root, field.maximumConfig)
			if _, err := LoadRunOnceConfig(root, DefaultRunOnceConfig(root)); err != nil {
				t.Fatalf("LoadRunOnceConfig error = %v", err)
			}
			if _, err := CheckRunConfig(root); err == nil || !strings.Contains(err.Error(), "source-writer lock window overflows time.Duration") {
				t.Fatalf("CheckRunConfig error = %v, want derived source-writer overflow", err)
			}
		})
		t.Run(field.path+"/overflow", func(t *testing.T) {
			assertConfigErrorFromLoadAndCheck(t, field.overflowConfig, field.path, "overflows time.Duration")
		})
	}
}

func TestConfigNumericOmissionsPreserveBaseValues(t *testing.T) {
	root := t.TempDir()
	base := DefaultRunOnceConfig(root)
	base.CodexTimeout = 11 * time.Second
	base.GitTimeout = 12 * time.Second
	base.CommitTimeout = 13 * time.Second
	base.CodexStdoutCap = 101
	base.CodexStderrCap = 102
	base.GitStdoutCap = 103
	base.GitStderrCap = 104
	base.VerificationStdoutCap = 105
	base.VerificationStderrCap = 106
	base.CommitStdoutCap = 107
	base.CommitStderrCap = 108
	base.VerificationCommands = []verification.Command{{Name: "go", Timeout: 14 * time.Second, StdoutCap: 109, StderrCap: 110}}
	base.NotificationPolicy = autonomousnotification.Policy{
		SchemaVersion:   autonomousnotification.PolicySchemaVersion,
		Enabled:         true,
		Events:          []autonomousnotification.Event{autonomousnotification.EventTaskCompleted},
		Executable:      "hook",
		Directory:       autonomousnotification.DirectoryRepository,
		Timeout:         time.Second,
		StdoutCap:       111,
		StderrCap:       112,
		MaximumAttempts: 2,
		RetryDelay:      time.Second,
	}
	base.RetentionPolicy = artifactretention.DefaultPolicy()
	base.RetentionPolicy.RecentRunCount = 7
	base.RetentionPolicy.CompressAfter = time.Hour
	base.RetentionPolicy.PruneAfter = 2 * time.Hour
	base.RetentionPolicy.MinimumCompressBytes = 113
	base.RetentionPolicy.MaxFilesPerOperation = 8
	base.RetentionPolicy.MaxBytesPerOperation = 114
	base.RetentionPolicy.DecompressionCapBytes = 115
	base.QueuePolicy = autonomousqueue.DefaultPolicy()
	base.QueuePolicy.MaximumWorkers = 2

	writeConfigTestFile(t, root, "codex: {}\ngit: {}\nverification: {}\ncommit: {}\noutput: {}\nnotifications: {}\nretention: {}\nqueue: {}\n")
	loaded, err := LoadRunOnceConfig(root, base)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !reflect.DeepEqual(loaded, base) {
		t.Fatalf("omitted numeric fields changed base:\nloaded=%+v\nbase=%+v", loaded, base)
	}

	writeConfigTestFile(t, root, "verification:\n  commands:\n    - name: go\n")
	loaded, err = LoadRunOnceConfig(root, DefaultRunOnceConfig(root))
	if err != nil {
		t.Fatalf("load verification omission config: %v", err)
	}
	if len(loaded.VerificationCommands) != 1 || loaded.VerificationCommands[0].Timeout != 0 || loaded.VerificationCommands[0].StdoutCap != 0 || loaded.VerificationCommands[0].StderrCap != 0 {
		t.Fatalf("omitted verification command overrides = %+v", loaded.VerificationCommands)
	}
}

func TestConfigRequiresOneDocumentAndAllowsTrailingComments(t *testing.T) {
	root := t.TempDir()
	valid := `codex:
  timeout_seconds: 45
git:
  timeout_seconds: 12
verification:
  commands:
    - name: go
      timeout_seconds: 9
      stdout_cap_bytes: 123
      stderr_cap_bytes: 124
commit:
  timeout_seconds: 30
output:
  codex_stdout_cap_bytes: 101
  codex_stderr_cap_bytes: 102
  git_stdout_cap_bytes: 103
  git_stderr_cap_bytes: 104
  verification_stdout_cap_bytes: 105
  verification_stderr_cap_bytes: 106
  commit_stdout_cap_bytes: 107
  commit_stderr_cap_bytes: 108
retention:
  recent_run_count: 3
queue:
  maximum_workers: 2
`
	writeConfigTestFile(t, root, valid)
	first, err := CheckRunConfig(root)
	if err != nil {
		t.Fatalf("check valid config: %v", err)
	}
	writeConfigTestFile(t, root, "---\n"+valid+"...\n# trailing comments and whitespace are legal\n\n")
	withComments, err := CheckRunConfig(root)
	if err != nil {
		t.Fatalf("check commented config: %v", err)
	}
	if first.EffectiveConfigSHA256 != withComments.EffectiveConfigSHA256 {
		t.Fatalf("comments changed effective config: first=%s commented=%s", first.EffectiveConfigSHA256, withComments.EffectiveConfigSHA256)
	}

	for _, test := range []struct {
		name    string
		content string
	}{
		{name: "structured second document", content: valid + "---\ngit:\n  timeout_seconds: 99\n"},
		{name: "empty second document", content: valid + "---\n# second document is still present\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			writeConfigTestFile(t, root, test.content)
			assertConfigErrorAtRootFromLoadAndCheck(t, root, "exactly one document is required")
		})
	}
}

func TestConfigIntegerDecodeOverflowNamesField(t *testing.T) {
	assertConfigErrorFromLoadAndCheck(t, "output:\n  codex_stdout_cap_bytes: 9223372036854775808\n", "output.codex_stdout_cap_bytes")
	assertConfigErrorFromLoadAndCheck(t, "codex:\n  timeout_seconds: 9223372036854775808\n", "codex.timeout_seconds")
}

func positiveNumericConfigFields() []numericConfigField {
	fields := []numericConfigField{
		{path: "codex.timeout_seconds", render: scalarConfig("codex", "timeout_seconds")},
		{path: "git.timeout_seconds", render: scalarConfig("git", "timeout_seconds")},
		{path: "verification.commands[0].timeout_seconds", render: func(value string) string { return verificationCommandConfig(false, "timeout_seconds", value) }},
		{path: "verification.commands[0].stdout_cap_bytes", render: func(value string) string { return verificationCommandConfig(false, "stdout_cap_bytes", value) }},
		{path: "verification.commands[0].stderr_cap_bytes", render: func(value string) string { return verificationCommandConfig(false, "stderr_cap_bytes", value) }},
		{path: "verification.tiers[0].commands[0].timeout_seconds", render: func(value string) string { return verificationCommandConfig(true, "timeout_seconds", value) }},
		{path: "verification.tiers[0].commands[0].stdout_cap_bytes", render: func(value string) string { return verificationCommandConfig(true, "stdout_cap_bytes", value) }},
		{path: "verification.tiers[0].commands[0].stderr_cap_bytes", render: func(value string) string { return verificationCommandConfig(true, "stderr_cap_bytes", value) }},
		{path: "commit.timeout_seconds", render: scalarConfig("commit", "timeout_seconds")},
		{path: "notifications.timeout_seconds", render: scalarConfig("notifications", "timeout_seconds")},
		{path: "notifications.stdout_cap_bytes", render: scalarConfig("notifications", "stdout_cap_bytes")},
		{path: "notifications.stderr_cap_bytes", render: scalarConfig("notifications", "stderr_cap_bytes")},
		{path: "notifications.maximum_attempts", render: scalarConfig("notifications", "maximum_attempts")},
		{path: "retention.max_files_per_operation", render: scalarConfig("retention", "max_files_per_operation")},
		{path: "retention.max_bytes_per_operation", render: scalarConfig("retention", "max_bytes_per_operation")},
		{path: "retention.decompression_cap_bytes", render: scalarConfig("retention", "decompression_cap_bytes")},
		{path: "queue.maximum_workers", render: scalarConfig("queue", "maximum_workers")},
	}
	for _, key := range []string{
		"codex_stdout_cap_bytes",
		"codex_stderr_cap_bytes",
		"git_stdout_cap_bytes",
		"git_stderr_cap_bytes",
		"verification_stdout_cap_bytes",
		"verification_stderr_cap_bytes",
		"commit_stdout_cap_bytes",
		"commit_stderr_cap_bytes",
	} {
		fields = append(fields, numericConfigField{path: "output." + key, render: scalarConfig("output", key)})
	}
	return fields
}

func scalarConfig(section, field string) func(string) string {
	return func(value string) string {
		return fmt.Sprintf("%s:\n  %s: %s\n", section, field, value)
	}
}

func verificationCommandConfig(tiered bool, field, value string) string {
	if !tiered {
		return fmt.Sprintf("verification:\n  commands:\n    - name: go\n      %s: %s\n", field, value)
	}
	return fmt.Sprintf("verification:\n  tiers:\n    - id: structural\n      kind: structural\n      required_for_final: true\n      run_for_final: true\n      rerun_policy: never\n      commands:\n        - name: go\n          %s: %s\n", field, value)
}

func validNotificationConfig(timeout, retryDelay string) string {
	return fmt.Sprintf("notifications:\n  enabled: true\n  events: [task_completed]\n  executable: hook\n  timeout_seconds: %s\n  stdout_cap_bytes: 1\n  stderr_cap_bytes: 1\n  maximum_attempts: 1\n  retry_delay_seconds: %s\n", timeout, retryDelay)
}

func assertConfigSuccessFromLoadAndCheck(t *testing.T, content string) {
	t.Helper()
	root := t.TempDir()
	writeConfigTestFile(t, root, content)
	if _, err := LoadRunOnceConfig(root, DefaultRunOnceConfig(root)); err != nil {
		t.Fatalf("LoadRunOnceConfig error = %v", err)
	}
	if _, err := CheckRunConfig(root); err != nil {
		t.Fatalf("CheckRunConfig error = %v", err)
	}
}

func assertConfigErrorFromLoadAndCheck(t *testing.T, content string, want ...string) {
	t.Helper()
	root := t.TempDir()
	writeConfigTestFile(t, root, content)
	assertConfigErrorAtRootFromLoadAndCheck(t, root, want...)
}

func assertConfigErrorAtRootFromLoadAndCheck(t *testing.T, root string, want ...string) {
	t.Helper()
	assertError := func(name string, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s succeeded, want error containing %q", name, want)
		}
		for _, fragment := range want {
			if !strings.Contains(err.Error(), fragment) {
				t.Fatalf("%s error = %v, want fragment %q", name, err, fragment)
			}
		}
	}
	_, loadErr := LoadRunOnceConfig(root, DefaultRunOnceConfig(root))
	assertError("LoadRunOnceConfig", loadErr)
	_, checkErr := CheckRunConfig(root)
	assertError("CheckRunConfig", checkErr)
}
