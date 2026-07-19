package supervisor

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"revolvr/internal/autonomous"
	"revolvr/internal/prompt"
)

func TestBuildPromptIsDeterministicAndIncludesExactInputs(t *testing.T) {
	dossier := testDossier([]byte("# Exact dossier\n\nKeep  double  spaces.\nNo rewrite marker."))
	profile := prompt.RunProfile{
		Name:        SupervisorProfileName,
		SourcePath:  ".agent/profiles/supervisor.md",
		Description: "Exact profile line one.\n\nExact profile line two.",
	}
	first, err := BuildPrompt(PromptInput{TaskID: "task-1", Dossier: dossier, Profile: profile})
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildPrompt(PromptInput{TaskID: "task-1", Dossier: dossier, Profile: profile})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("prompt rendering is not deterministic")
	}
	for _, exact := range [][]byte{[]byte(profile.Description), dossier.Markdown} {
		if !bytes.Contains(first, exact) {
			t.Fatalf("prompt does not contain exact bytes %q", exact)
		}
	}
	for _, phrase := range []string{
		"Task identity: task-1",
		"Dossier schema: " + autonomous.DossierManifestSchemaVersion,
		"Dossier SHA-256: " + dossier.Manifest.DossierSHA256,
		fmt.Sprintf("Dossier byte size: %d", len(dossier.Markdown)),
		"fresh, ephemeral, decision-only",
		"Return exactly one JSON object",
		"explicit structured strategy material",
		"Do not edit repository source",
		"Do not execute or route a worker",
		"Revolvr retains safety, verification, legal-transition, retry, commit, and terminal-state authority",
	} {
		if !strings.Contains(string(first), phrase) {
			t.Fatalf("prompt missing %q", phrase)
		}
	}
}

func TestDecisionOutputSchemaIsDeterministicAndStrict(t *testing.T) {
	first, err := DecisionOutputSchema()
	if err != nil {
		t.Fatal(err)
	}
	second, err := DecisionOutputSchema()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) || len(first) == 0 || first[len(first)-1] != '\n' {
		t.Fatal("schema bytes are not deterministic with one final newline")
	}
	var schema map[string]any
	if err := json.Unmarshal(first, &schema); err != nil {
		t.Fatal(err)
	}
	if schema["additionalProperties"] != false {
		t.Fatalf("top-level additionalProperties = %#v", schema["additionalProperties"])
	}
	properties := schema["properties"].(map[string]any)
	wantRequired := []any{"task_id", "action", "worker_profile", "rationale", "success_criteria", "inputs", "finding_ids", "verification_failure", "strategy", "needs_input", "child_tasks"}
	if !reflect.DeepEqual(schema["required"], wantRequired) {
		t.Fatalf("required = %#v, want %#v", schema["required"], wantRequired)
	}
	actionEnum := properties["action"].(map[string]any)["enum"].([]any)
	wantActions := []any{"plan", "implement", "audit", "correct", "document", "simplify", "complete", "block", "needs_input"}
	if !reflect.DeepEqual(actionEnum, wantActions) {
		t.Fatalf("actions = %#v, want %#v", actionEnum, wantActions)
	}
	profileEnum := properties["worker_profile"].(map[string]any)["enum"].([]any)
	wantProfiles := []any{"planner", "implementer", "auditor", "corrector", "documentor", "simplifier", nil}
	if !reflect.DeepEqual(profileEnum, wantProfiles) {
		t.Fatalf("profiles = %#v, want %#v", profileEnum, wantProfiles)
	}
	findingIDs := properties["finding_ids"].(map[string]any)
	if _, exists := findingIDs["uniqueItems"]; exists {
		t.Fatal("schema delegates correction finding ID uniqueness to Go validation")
	}
	defs := schema["$defs"].(map[string]any)
	strategy := defs["strategy"].(map[string]any)
	if strategy["additionalProperties"] != false || !reflect.DeepEqual(strategy["required"], []any{"approach", "techniques", "targets"}) {
		t.Fatalf("strategy schema = %#v", strategy)
	}
	evidence := defs["evidence_reference"].(map[string]any)
	if evidence["additionalProperties"] != false {
		t.Fatalf("evidence additionalProperties = %#v", evidence["additionalProperties"])
	}
	evidenceProperties := evidence["properties"].(map[string]any)
	kinds := evidenceProperties["kind"].(map[string]any)["enum"].([]any)
	if len(kinds) != 9 {
		t.Fatalf("evidence kinds = %#v", kinds)
	}
	for _, field := range []string{"reference", "detail"} {
		fieldSchema := evidenceProperties[field].(map[string]any)
		if fieldSchema["pattern"] == "" {
			t.Fatalf("evidence %s schema = %#v", field, fieldSchema)
		}
	}
}

func TestParseDecisionRequiredNullAndEmptyOptionalsPreserveSemantics(t *testing.T) {
	evidence := []any{map[string]any{"kind": "task", "reference": ".agent/tasks/task-1.md", "detail": "Exact task evidence."}}
	base := func(action string) map[string]any {
		return map[string]any{
			"task_id": "task-1", "action": action, "worker_profile": nil, "rationale": "Exact route rationale.",
			"success_criteria": []any{}, "inputs": evidence, "finding_ids": []any{}, "verification_failure": nil,
			"strategy": nil, "needs_input": nil, "child_tasks": nil,
		}
	}

	t.Run("terminal", func(t *testing.T) {
		wire := base("complete")
		decision := parseDecisionWire(t, wire)
		if decision.WorkerProfile != "" || len(decision.SuccessCriteria) != 0 || len(decision.FindingIDs) != 0 || decision.VerificationFailure != nil || decision.Strategy != nil || decision.NeedsInput != nil || decision.ChildTasks != nil {
			t.Fatalf("decoded terminal optionals = %+v", decision)
		}
	})

	t.Run("worker strategy", func(t *testing.T) {
		wire := base("implement")
		wire["worker_profile"] = "implementer"
		wire["success_criteria"] = []any{"Implement the exact requested behavior."}
		wire["strategy"] = map[string]any{"approach": "Change only the exact target.", "techniques": []any{}, "targets": []any{}}
		decision := parseDecisionWire(t, wire)
		if decision.Strategy == nil || len(decision.Strategy.Techniques) != 0 || len(decision.Strategy.Targets) != 0 {
			t.Fatalf("decoded worker strategy = %+v", decision.Strategy)
		}
	})

	t.Run("needs input computes content identity", func(t *testing.T) {
		wire := base("needs_input")
		wire["needs_input"] = map[string]any{
			"task_id": "task-1", "question_id": "product-mode", "revision": 1, "content_sha256": nil,
			"question": "Which behavior should be selected?", "blocking_reason": "The task permits incompatible behaviors.",
			"options":        []any{map[string]any{"id": "keep", "meaning": "Keep the current behavior."}, map[string]any{"id": "change", "meaning": "Change the behavior."}},
			"recommendation": map[string]any{"option_id": "keep", "rationale": "This preserves current behavior."},
			"evidence":       evidence, "independent_work": []any{},
		}
		decision := parseDecisionWire(t, wire)
		if decision.NeedsInput == nil || decision.NeedsInput.ContentSHA256 == "" || len(decision.NeedsInput.IndependentWork) != 0 {
			t.Fatalf("decoded needs-input = %+v", decision.NeedsInput)
		}
	})

	t.Run("child task optional sets", func(t *testing.T) {
		wire := base("block")
		wire["child_tasks"] = map[string]any{
			"parent_task_id": "task-1", "proposal_id": "split-work",
			"children": []any{map[string]any{
				"key": "child-one", "title": "Inspect the exact contract", "scope": "Inspect the current contract without source mutation.",
				"success_criteria": []any{"Report the exact observed contract."}, "depends_on": []any{}, "tags": []any{}, "conflicts": []any{},
				"parent_behavior": "independent", "evidence": evidence,
			}},
		}
		decision := parseDecisionWire(t, wire)
		child := decision.ChildTasks.Children[0]
		if len(child.DependsOn) != 0 || len(child.Tags) != 0 || len(child.Conflicts) != 0 {
			t.Fatalf("decoded child optional sets = %+v", child)
		}
	})

	t.Run("conditional validation remains authoritative", func(t *testing.T) {
		wire := base("complete")
		wire["worker_profile"] = "implementer"
		raw, err := json.Marshal(wire)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseDecision(raw, "task-1", nil); err == nil || !strings.Contains(err.Error(), "must not select worker_profile") {
			t.Fatalf("ParseDecision() error = %v, want terminal profile rejection", err)
		}
	})
}

func parseDecisionWire(t *testing.T, wire map[string]any) autonomous.SupervisorDecision {
	t.Helper()
	raw, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := ParseDecision(raw, "task-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	return decision
}

func TestParseDecisionAssignsAndValidatesNeedsInputContentIdentity(t *testing.T) {
	question := autonomous.NeedsInputQuestion{TaskID: "task-1", QuestionID: "product-mode", Revision: 1, Question: "Which behavior?", BlockingReason: "The task permits incompatible behaviors.", Options: []autonomous.NeedsInputOption{{ID: "keep", Meaning: "Keep behavior."}, {ID: "change", Meaning: "Change behavior."}}, Recommendation: autonomous.NeedsInputRecommendation{OptionID: "keep", Rationale: "Safer."}, Evidence: []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindTask, Reference: "task", Detail: "Ambiguous task."}}}
	decision := autonomous.SupervisorDecision{TaskID: "task-1", Action: autonomous.ActionNeedsInput, Rationale: "Input required.", Inputs: question.Evidence, NeedsInput: &question}
	raw, err := json.Marshal(decision)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseDecision(raw, "task-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.NeedsInput == nil || parsed.NeedsInput.ContentSHA256 == "" {
		t.Fatalf("parsed=%+v", parsed)
	}
	decision.NeedsInput.ContentSHA256 = strings.Repeat("f", 64)
	raw, _ = json.Marshal(decision)
	if _, err := ParseDecision(raw, "task-1", nil); err == nil || !strings.Contains(err.Error(), "deterministic question identity") {
		t.Fatalf("changed identity error=%v", err)
	}
	decision.NeedsInput.ContentSHA256 = ""
	decision.NeedsInput.Options = decision.NeedsInput.Options[:1]
	raw, _ = json.Marshal(decision)
	if _, err := ParseDecision(raw, "task-1", nil); err == nil || !strings.Contains(err.Error(), "at least two") {
		t.Fatalf("malformed options error=%v", err)
	}
}

func testDossier(markdown []byte) autonomous.TaskDossier {
	hash := sha256.Sum256(markdown)
	return autonomous.TaskDossier{
		Markdown: append([]byte(nil), markdown...),
		Manifest: autonomous.TaskDossierManifest{
			SchemaVersion:   autonomous.DossierManifestSchemaVersion,
			TaskID:          "task-1",
			DossierSHA256:   fmt.Sprintf("%x", hash),
			DossierByteSize: len(markdown),
		},
	}
}
