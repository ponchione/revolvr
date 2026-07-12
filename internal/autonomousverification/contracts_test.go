package autonomousverification

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"revolvr/internal/verification"
)

func TestPlanKindsOrderSelectionAndIdentity(t *testing.T) {
	plan := allTierPlan()
	before, _ := json.Marshal(plan)
	if err := plan.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	first, err := MarshalPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := MarshalPlan(plan)
	if !reflect.DeepEqual(first, second) || first[len(first)-1] != '\n' {
		t.Fatal("plan encoding is not deterministic with one final newline")
	}
	id1, _ := Identity(plan)
	id2, _ := Identity(plan)
	if id1 != id2 || id1.ByteSize != len(first) {
		t.Fatalf("identities differ: %+v %+v", id1, id2)
	}
	fast, err := Select(plan, PurposeFast)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := fast.SelectedTierIDs, []string{"structural", "focused", "task-acceptance"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("fast=%v want %v", got, want)
	}
	final, err := Select(plan, PurposeFinal)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := final.SelectedTierIDs, []string{"structural", "focused", "task-acceptance", "full-suite", "race", "integration", "security"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("final=%v want %v", got, want)
	}
	if got, want := final.RequiredFinalTiers, []string{"structural", "focused", "task-acceptance", "full-suite"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("required=%v want %v", got, want)
	}
	after, _ := json.Marshal(plan)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("selection mutated caller plan")
	}
}

func TestPlanStrictValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Plan)
		want   string
	}{
		{"schema", func(p *Plan) { p.SchemaVersion = "v2" }, "unsupported schema"},
		{"duplicate id", func(p *Plan) { p.Tiers[1].ID = p.Tiers[0].ID }, "duplicates tier ID"},
		{"duplicate kind", func(p *Plan) { p.Tiers[1].Kind = p.Tiers[0].Kind }, "duplicates tier kind"},
		{"invalid id", func(p *Plan) { p.Tiers[0].ID = "Bad_ID" }, "lower-case kebab-case"},
		{"order", func(p *Plan) { p.Tiers[0], p.Tiers[1] = p.Tiers[1], p.Tiers[0] }, "canonical tier order"},
		{"unknown kind", func(p *Plan) { p.Tiers[0].Kind = "smoke" }, "unknown value"},
		{"required not selected", func(p *Plan) { p.Tiers[0].RunForFinal = false }, "run_for_final is false"},
		{"expensive fast", func(p *Plan) { p.Tiers[3].RunForFast = true }, "cannot be selected for fast"},
		{"never selected", func(p *Plan) { p.Tiers[4].RunForFinal = false }, "never selected"},
		{"unknown rerun", func(p *Plan) { p.Tiers[0].RerunPolicy = "twice" }, "unknown value"},
		{"unsafe dir", func(p *Plan) { p.Tiers[0].Commands[0].Dir = "../outside" }, "unsafe command directory"},
		{"bad env", func(p *Plan) { p.Tiers[0].Commands[0].Env = []string{"BROKEN"} }, "NAME=value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := allTierPlan()
			tt.mutate(&p)
			if err := p.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error=%v want %q", err, tt.want)
			}
		})
	}
}

func TestDecodePlanRejectsUnknownAndMultipleJSON(t *testing.T) {
	for _, raw := range []string{
		`{"schema_version":"autonomous-verification-plan-v1","tiers":[],"unknown":true}`,
		`{"schema_version":"autonomous-verification-plan-v1","tiers":[]} {}`,
	} {
		if _, err := DecodePlan([]byte(raw)); err == nil {
			t.Fatalf("DecodePlan(%s) succeeded", raw)
		}
	}
}

func TestLegacyAdapterPreservesCommandsAndMissingEvidence(t *testing.T) {
	args := []string{"test", "./..."}
	commands := []verification.Command{{Name: "go", Args: args, Env: []string{"MODE=test"}, Timeout: time.Minute, StdoutCap: 10, StderrCap: 11}}
	plan := AdaptLegacy(commands)
	if err := plan.Validate(); err != nil {
		t.Fatal(err)
	}
	if plan.Tiers[0].ID != "legacy-flat" || plan.Tiers[0].Kind != TierFullSuite || !plan.Tiers[0].RequiredForFinal {
		t.Fatalf("legacy plan=%+v", plan)
	}
	plan.Tiers[0].Commands[0].Args[0] = "changed"
	if args[0] != "test" || commands[0].Args[0] != "test" {
		t.Fatal("adapter retained caller slice")
	}
	missing := AdaptLegacy(nil)
	if err := missing.Validate(); err != nil {
		t.Fatalf("missing legacy plan must remain executable evidence: %v", err)
	}
}

func allTierPlan() Plan {
	kinds := []TierKind{TierStructural, TierFocused, TierTaskAcceptance, TierFullSuite, TierRace, TierIntegration, TierSecurity}
	ids := []string{"structural", "focused", "task-acceptance", "full-suite", "race", "integration", "security"}
	tiers := make([]Tier, len(kinds))
	for i := range kinds {
		tiers[i] = Tier{ID: ids[i], Kind: kinds[i], RequiredForFinal: i < 4, RunForFast: i < 3, RunForFinal: true, Commands: []verification.Command{{Name: "check", Args: []string{ids[i]}, Dir: "checks", Env: []string{"TIER=" + ids[i]}}}, RerunPolicy: RerunNever}
	}
	return Plan{SchemaVersion: PlanSchemaVersion, Tiers: tiers}
}
