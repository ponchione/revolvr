package runonce

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"revolvr/internal/autonomousverification"
	"revolvr/internal/codexexec"
	"revolvr/internal/ledger"
	"revolvr/internal/verification"
)

func TestFingerprintEffectiveConfigIsDeterministicAndCopiesSlices(t *testing.T) {
	args := []string{"test", "./..."}
	commands := []verification.Command{{Name: "go", Args: args, Dir: "internal", Timeout: time.Minute}}
	cfg := Config{WorkingDir: t.TempDir(), VerificationCommands: commands}

	first, err := FingerprintEffectiveConfig(cfg)
	if err != nil {
		t.Fatalf("fingerprint effective config: %v", err)
	}
	second, err := FingerprintEffectiveConfig(cfg)
	if err != nil {
		t.Fatalf("fingerprint effective config again: %v", err)
	}
	if first.Schema != EffectiveConfigSchema || first.SHA256 != second.SHA256 || !reflect.DeepEqual(first.JSON, second.JSON) || !reflect.DeepEqual(first.Projection, second.Projection) {
		t.Fatalf("fingerprints differ:\nfirst=%+v\nsecond=%+v", first, second)
	}
	first.Projection.Verification.Commands[0].Args[0] = "changed"
	first.JSON[0] = 'x'
	if args[0] != "test" || commands[0].Args[0] != "test" {
		t.Fatalf("caller slices mutated: args=%#v commands=%#v", args, commands)
	}
	third, err := FingerprintEffectiveConfig(cfg)
	if err != nil {
		t.Fatalf("fingerprint effective config after result mutation: %v", err)
	}
	if third.SHA256 != second.SHA256 || !reflect.DeepEqual(third.JSON, second.JSON) {
		t.Fatalf("result mutation affected repeated fingerprint")
	}
}

func TestFingerprintEffectiveConfigChangesForMaterialSettings(t *testing.T) {
	base := Config{
		WorkingDir:           t.TempDir(),
		CodexModel:           codexexec.DefaultModel,
		CodexReasoningEffort: codexexec.DefaultReasoningEffort,
		CodexEphemeral:       true,
		VerificationCommands: []verification.Command{{Name: "go", Args: []string{"test", "./..."}}},
	}
	baseline, err := FingerprintEffectiveConfig(base)
	if err != nil {
		t.Fatalf("baseline fingerprint: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{name: "model", mutate: func(cfg *Config) { cfg.CodexModel = "gpt-custom" }},
		{name: "effort", mutate: func(cfg *Config) { cfg.CodexReasoningEffort = "high" }},
		{name: "verification", mutate: func(cfg *Config) { cfg.VerificationCommands[0].Args = []string{"test", "./internal/..."} }},
		{name: "sandbox", mutate: func(cfg *Config) { cfg.CodexSandbox = "read-only" }},
		{name: "git", mutate: func(cfg *Config) { cfg.GitTimeout = time.Second }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base
			cfg.VerificationCommands = append([]verification.Command(nil), base.VerificationCommands...)
			cfg.VerificationCommands[0].Args = append([]string(nil), base.VerificationCommands[0].Args...)
			tt.mutate(&cfg)
			got, err := FingerprintEffectiveConfig(cfg)
			if err != nil {
				t.Fatalf("fingerprint: %v", err)
			}
			if got.SHA256 == baseline.SHA256 {
				t.Fatalf("%s change did not change hash %s", tt.name, got.SHA256)
			}
		})
	}

	projection := baseline.Projection
	projection.Codex.Ephemeral = false
	_, sessionHash, err := fingerprintProjection(projection)
	if err != nil {
		t.Fatalf("fingerprint session projection: %v", err)
	}
	if sessionHash == baseline.SHA256 {
		t.Fatal("session change did not change hash")
	}
}

func TestFingerprintEffectiveConfigRejectsRemovedDirtyWorktreeOption(t *testing.T) {
	_, err := FingerprintEffectiveConfig(Config{
		WorkingDir:            t.TempDir(),
		AllowPreExistingDirty: true,
	})
	if err == nil || !strings.Contains(err.Error(), "allow_pre_existing_dirty is unsupported") {
		t.Fatalf("FingerprintEffectiveConfig error = %v, want removed option error", err)
	}
}

func TestFingerprintEffectiveConfigIgnoresProcessLocalIdentity(t *testing.T) {
	workDir := t.TempDir()
	base := Config{WorkingDir: workDir, VerificationCommands: []verification.Command{}}
	first := base
	first.Clock = func() time.Time { return time.Unix(1, 0) }
	first.CodexRunner = func(context.Context, codexexec.Config) (codexexec.Result, error) { return codexexec.Result{}, nil }
	first.LedgerStore = &ledger.Store{}
	second := base
	second.Clock = func() time.Time { return time.Unix(2, 0) }
	second.CodexRunner = func(context.Context, codexexec.Config) (codexexec.Result, error) {
		return codexexec.Result{ExitCode: 9}, nil
	}
	second.LedgerStore = nil

	firstFingerprint, err := FingerprintEffectiveConfig(first)
	if err != nil {
		t.Fatalf("first fingerprint: %v", err)
	}
	secondFingerprint, err := FingerprintEffectiveConfig(second)
	if err != nil {
		t.Fatalf("second fingerprint: %v", err)
	}
	if firstFingerprint.SHA256 != secondFingerprint.SHA256 || !reflect.DeepEqual(firstFingerprint.JSON, secondFingerprint.JSON) {
		t.Fatalf("process-local identities changed fingerprint")
	}
}

func TestFingerprintEffectiveConfigIncludesTierPlanAndCopiesIt(t *testing.T) {
	args := []string{"test", "./..."}
	plan := autonomousverification.Plan{SchemaVersion: autonomousverification.PlanSchemaVersion, Tiers: []autonomousverification.Tier{{ID: "structural", Kind: autonomousverification.TierStructural, RequiredForFinal: true, RunForFast: true, RunForFinal: true, Commands: []verification.Command{{Name: "go", Args: args, Env: []string{"MODE=test"}, StdoutCap: 10, StderrCap: 11}}, RerunPolicy: autonomousverification.RerunOnceToClassifyFlaky}}}
	cfg := Config{WorkingDir: t.TempDir(), VerificationPlan: &plan, VerificationCommands: []verification.Command{}}
	first, err := FingerprintEffectiveConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	first.Projection.Verification.Plan.Tiers[0].Commands[0].Args[0] = "changed"
	if args[0] != "test" {
		t.Fatal("fingerprint retained caller args")
	}
	changed := autonomousverification.ClonePlan(plan)
	changed.Tiers[0].RerunPolicy = autonomousverification.RerunNever
	cfg.VerificationPlan = &changed
	second, err := FingerprintEffectiveConfig(cfg)
	if err != nil || second.SHA256 == first.SHA256 {
		t.Fatalf("rerun policy hash unchanged: %v", err)
	}
}
