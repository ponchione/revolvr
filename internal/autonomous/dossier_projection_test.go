package autonomous

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestRoleDossierProjectionMatrixAndDeterminism(t *testing.T) {
	in := fullDossierInput()
	in.RepositoryMap = &RepositoryMapSource{ID: "repository-map:test", CommitSHA: strings.Repeat("a", 40), TreeSHA: strings.Repeat("b", 40), Content: []byte("- cmd/revolvr/main.go [go-source]\n"), CacheKey: strings.Repeat("c", 64), CacheManifestSHA256: strings.Repeat("d", 64), CacheResult: "hit"}
	wants := map[DossierRole]map[string]bool{
		DossierRoleSupervisor:  {"verification": true, "audit_findings": true, "recent_runs": true},
		DossierRolePlanner:     {"verification": false, "audit_findings": false, "recent_runs": true},
		DossierRoleImplementer: {"verification": true, "audit_findings": false, "recent_runs": false},
		DossierRoleAuditor:     {"verification": true, "audit_findings": true, "recent_runs": false},
		DossierRoleCorrector:   {"verification": true, "audit_findings": true, "recent_runs": false},
		DossierRoleDocumentor:  {"verification": true, "audit_findings": true, "recent_runs": false},
		DossierRoleSimplifier:  {"verification": true, "audit_findings": true, "recent_runs": false},
	}
	seenHashes := map[string]DossierRole{}
	for role, matrix := range wants {
		first, err := ProjectTaskDossier(in, role)
		if err != nil {
			t.Fatalf("%s: %v", role, err)
		}
		second, err := ProjectTaskDossier(in, role)
		if err != nil || !bytes.Equal(first.Markdown, second.Markdown) || !reflect.DeepEqual(first.Manifest, second.Manifest) {
			t.Fatalf("%s projection is nondeterministic: %v", role, err)
		}
		if prior, exists := seenHashes[first.Manifest.DossierSHA256]; exists {
			t.Fatalf("roles %s and %s unexpectedly share projection hash", prior, role)
		}
		seenHashes[first.Manifest.DossierSHA256] = role
		if first.Manifest.Projection == nil || first.Manifest.Projection.Role != role || first.Manifest.TokenEstimate == nil || first.Manifest.TokenEstimate.Estimated == 0 || first.Manifest.Cache == nil || first.Manifest.Cache.Result != "hit" {
			t.Fatalf("%s manifest facts=%+v", role, first.Manifest)
		}
		facts := map[string]DossierSectionFact{}
		for _, fact := range first.Manifest.Sections {
			facts[fact.Section] = fact
			if fact.IncludedBytes < 0 || fact.IncludedBytes > fact.TotalByteSize || fact.Estimator != DossierTokenEstimatorSchema {
				t.Fatalf("%s invalid section fact %+v", role, fact)
			}
		}
		for section, included := range matrix {
			if facts[section].Included != included {
				t.Fatalf("%s section %s included=%t want %t", role, section, facts[section].Included, included)
			}
		}
		for _, exact := range []string{"# AW-03", "criterion-satisfied", "Deterministic Repository Map", "Role Projection and Size Facts"} {
			if !strings.Contains(string(first.Markdown), exact) {
				t.Fatalf("%s dossier missing %q", role, exact)
			}
		}
	}
}

func TestRoleProjectionClonesCallerEvidence(t *testing.T) {
	in := sparseDossierInput()
	original, err := ProjectTaskDossier(in, DossierRoleSupervisor)
	if err != nil {
		t.Fatal(err)
	}
	in.TaskSpec.Content[0] = 'X'
	in.State.TaskID = "changed"
	reprojected, err := ReprojectTaskDossier(original, DossierRolePlanner)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(reprojected.Markdown, []byte("changed")) || !bytes.Contains(reprojected.Markdown, []byte("# Sparse")) {
		t.Fatalf("caller mutation leaked into projection:\n%s", reprojected.Markdown)
	}
}

func TestRoleProjectionDoesNotReuseTaskBoundEvidence(t *testing.T) {
	firstInput := sparseDossierInput()
	firstInput.RepositoryMap = &RepositoryMapSource{ID: "map", CommitSHA: strings.Repeat("a", 40), TreeSHA: strings.Repeat("b", 40), Content: []byte("same committed context\n")}
	secondInput := sparseDossierInput()
	secondInput.TaskID = "task-2"
	secondInput.TaskSpec.ID = "task-spec:task-2"
	secondInput.TaskSpec.Path = ".agent/tasks/task-2.md"
	secondInput.TaskSpec.Content = []byte("# Second Task\n\nOnly task two evidence.")
	secondInput.State.TaskID = "task-2"
	secondInput.RepositoryMap = firstInput.RepositoryMap
	first, err := ProjectTaskDossier(firstInput, DossierRoleImplementer)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ProjectTaskDossier(secondInput, DossierRoleImplementer)
	if err != nil {
		t.Fatal(err)
	}
	if first.Manifest.DossierSHA256 == second.Manifest.DossierSHA256 || bytes.Contains(second.Markdown, []byte("task-1")) {
		t.Fatalf("task-bound evidence leaked across projections:\n%s", second.Markdown)
	}
}

func TestRoleForActionFailsClosed(t *testing.T) {
	role, err := RoleForAction(ActionCorrect, WorkerProfileCorrector)
	if err != nil || role != DossierRoleCorrector {
		t.Fatalf("role=%q err=%v", role, err)
	}
	if _, err := RoleForAction(ActionCorrect, WorkerProfileImplementer); err == nil {
		t.Fatal("contradictory action/profile succeeded")
	}
	if _, err := ProjectTaskDossier(sparseDossierInput(), DossierRole("future")); err == nil {
		t.Fatal("unknown role succeeded")
	}
}
