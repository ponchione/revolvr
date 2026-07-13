package app

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"revolvr/internal/autonomousverification"
	"revolvr/internal/runonce"
	"revolvr/internal/verification"
)

func TestVerificationCommandsPresenceMergesAgainstEveryBaseShape(t *testing.T) {
	plan := autonomousverification.Plan{
		SchemaVersion: autonomousverification.PlanSchemaVersion,
		Tiers: []autonomousverification.Tier{{
			ID:               "structural",
			Kind:             autonomousverification.TierStructural,
			RequiredForFinal: true,
			RunForFinal:      true,
			RerunPolicy:      autonomousverification.RerunNever,
			Commands:         []verification.Command{{Name: "tier-command"}},
		}},
	}
	bases := []struct {
		name string
		cfg  runonce.Config
	}{
		{name: "nil", cfg: runonce.Config{}},
		{name: "flat", cfg: runonce.Config{VerificationCommands: []verification.Command{{Name: "inherited-command"}}}},
		{name: "tiered", cfg: runonce.Config{VerificationPlan: &plan}},
	}
	inputs := []struct {
		name     string
		content  string
		inherit  bool
		commands []verification.Command
	}{
		{name: "omitted", content: "{}\n", inherit: true},
		{name: "null", content: "verification:\n  commands: null\n", inherit: true},
		{name: "empty", content: "verification:\n  commands: []\n", commands: []verification.Command{}},
		{name: "nonempty", content: "verification:\n  commands: [{name: configured-command}]\n", commands: []verification.Command{{Name: "configured-command"}}},
	}
	for _, base := range bases {
		for _, input := range inputs {
			t.Run(base.name+"/"+input.name, func(t *testing.T) {
				workDir := t.TempDir()
				writeConfigTestFile(t, workDir, input.content)
				got, err := LoadRunOnceConfig(workDir, base.cfg)
				if err != nil {
					t.Fatal(err)
				}
				if input.inherit {
					if !reflect.DeepEqual(got.VerificationCommands, base.cfg.VerificationCommands) || !reflect.DeepEqual(got.VerificationPlan, base.cfg.VerificationPlan) {
						t.Fatalf("inherited verification = commands %#v plan %#v; want %#v and %#v", got.VerificationCommands, got.VerificationPlan, base.cfg.VerificationCommands, base.cfg.VerificationPlan)
					}
					return
				}
				if !reflect.DeepEqual(got.VerificationCommands, input.commands) {
					t.Fatalf("verification commands = %#v, want %#v", got.VerificationCommands, input.commands)
				}
				if got.VerificationCommands == nil {
					t.Fatal("present commands decoded to a nil slice")
				}
				if got.VerificationPlan != nil {
					t.Fatalf("present commands retained inherited tier plan: %+v", got.VerificationPlan)
				}
			})
		}
	}
}

func TestExplicitEmptyVerificationCommandsSuppressGoDefaultAndChangeEffectiveHash(t *testing.T) {
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "go.mod"), []byte("module example.com/presence\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		content string
		count   int
	}{
		{name: "omitted", content: "{}\n", count: 1},
		{name: "null", content: "verification:\n  commands: null\n", count: 1},
		{name: "empty", content: "verification:\n  commands: []\n", count: 0},
		{name: "nonempty", content: "verification:\n  commands: [{name: custom}]\n", count: 1},
	}
	hashes := map[string]string{}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			writeConfigTestFile(t, workDir, test.content)
			result, err := CheckRunConfig(workDir)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Effective.VerificationCommands) != test.count {
				t.Fatalf("effective commands = %#v, want count %d", result.Effective.VerificationCommands, test.count)
			}
			if test.name == "empty" && result.Effective.VerificationCommands == nil {
				t.Fatal("explicit empty commands lost presence during normalization")
			}
			hashes[test.name] = result.EffectiveConfigSHA256
		})
	}
	if hashes["omitted"] != hashes["null"] {
		t.Fatalf("omitted and null effective hashes differ: %s %s", hashes["omitted"], hashes["null"])
	}
	if hashes["empty"] == hashes["omitted"] || hashes["nonempty"] == hashes["omitted"] || hashes["empty"] == hashes["nonempty"] {
		t.Fatalf("material command sets share hashes: %#v", hashes)
	}
}

func TestRunOncePreservesExplicitEmptyVerificationCommands(t *testing.T) {
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "go.mod"), []byte("module example.com/run-empty-verification\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeConfigTestFile(t, workDir, "verification:\n  missing_policy: pass\n  commands: []\n")
	called := false
	_, err := RunOnce(context.Background(), Config{WorkDir: workDir}, RunOnceInput{Runner: func(_ context.Context, cfg runonce.Config) (runonce.Result, error) {
		called = true
		if cfg.VerificationCommands == nil || len(cfg.VerificationCommands) != 0 || cfg.VerificationPlan != nil {
			t.Fatalf("configured verification = commands %#v plan %#v", cfg.VerificationCommands, cfg.VerificationPlan)
		}
		if cfg.MissingVerificationPolicy != verification.MissingCommandsPass {
			t.Fatalf("missing policy = %q", cfg.MissingVerificationPolicy)
		}
		effective, err := runonce.EffectiveConfig(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if effective.VerificationCommands == nil || len(effective.VerificationCommands) != 0 {
			t.Fatalf("run normalization synthesized commands: %#v", effective.VerificationCommands)
		}
		return runonce.Result{Outcome: runonce.OutcomeNoTask, NoTask: true}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("run once did not invoke configured runner")
	}
}
