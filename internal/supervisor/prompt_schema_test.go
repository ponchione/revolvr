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
	strategy := properties["strategy"].(map[string]any)
	if strategy["additionalProperties"] != false || !reflect.DeepEqual(strategy["required"], []any{"approach"}) {
		t.Fatalf("strategy schema = %#v", strategy)
	}
	actionEnum := properties["action"].(map[string]any)["enum"].([]any)
	wantActions := []any{"plan", "implement", "audit", "correct", "document", "simplify", "complete", "block", "needs_input"}
	if !reflect.DeepEqual(actionEnum, wantActions) {
		t.Fatalf("actions = %#v, want %#v", actionEnum, wantActions)
	}
	branches := schema["oneOf"].([]any)
	if len(branches) != 9 {
		t.Fatalf("oneOf branches = %d, want 9", len(branches))
	}
	raw := string(first)
	for action, profileName := range map[string]string{
		"plan": "planner", "implement": "implementer", "audit": "auditor", "correct": "corrector", "document": "documentor", "simplify": "simplifier",
	} {
		if !strings.Contains(raw, `"const": "`+action+`"`) || !strings.Contains(raw, `"const": "`+profileName+`"`) {
			t.Fatalf("schema missing %s -> %s", action, profileName)
		}
	}
	if !strings.Contains(raw, `"uniqueItems": true`) || !strings.Contains(raw, `"finding_ids"`) || !strings.Contains(raw, `"minItems": 1`) {
		t.Fatal("schema does not constrain correction finding IDs")
	}
	defs := schema["$defs"].(map[string]any)
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
		if fieldSchema["minLength"] != float64(1) || fieldSchema["pattern"] == "" {
			t.Fatalf("evidence %s schema = %#v", field, fieldSchema)
		}
	}
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
