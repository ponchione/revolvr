package context

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"revolvr/internal/retrieval"
)

func TestCompileIsDeterministicRoleBudgetedAndAuthorityOrdered(t *testing.T) {
	revision := strings.Repeat("a", 40)
	exact := inlineCandidate("exact", retrieval.AuthorityExactSource, "internal/router.go", "RouteProvider", "func RouteProvider() {}")
	advisory := inlineCandidate("advisory", retrieval.AuthorityAdvisory, "scratch.txt", "", strings.Repeat("model guess ", 400))
	advisory.Retrieval.Score = 10_000
	request := CompileRequest{
		ProjectID: uuid.NewString(), Role: RoleImplementer, SourceRevision: revision,
		Budget:                 Budget{Bytes: 850, Tokens: 213},
		RetrievalConfiguration: retrieval.Report{ConfigurationVersion: retrieval.ConfigurationVersion, SourceRevision: revision},
		Candidates:             []Candidate{advisory, exact},
	}
	first, err := Compile(request)
	if err != nil {
		t.Fatal(err)
	}
	request.Candidates[0], request.Candidates[1] = request.Candidates[1], request.Candidates[0]
	second, err := Compile(request)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || !bytes.Equal(first.Dossier, second.Dossier) || !reflect.DeepEqual(first.Manifest, second.Manifest) {
		t.Fatalf("nondeterministic packages = %#v / %#v", first, second)
	}
	if first.Manifest.FinalBytes > first.Manifest.ByteBudget || first.Manifest.FinalTokens > first.Manifest.TokenBudget || first.Manifest.IncludedCount != 1 || first.Manifest.ExcludedCount != 1 {
		t.Fatalf("budget manifest = %#v", first.Manifest)
	}
	var dossier Dossier
	if err := json.Unmarshal(first.Dossier, &dossier); err != nil || len(dossier.Items) != 1 || dossier.Items[0].CandidateIdentity != "exact" {
		t.Fatalf("authority packing dossier = %#v, %v", dossier, err)
	}
	if first.Manifest.Items[1].CandidateIdentity != "advisory" || first.Manifest.Items[1].OmissionReason != "role_budget_exceeded" {
		t.Fatalf("excluded advisory = %#v", first.Manifest.Items)
	}
}

func TestCompileReferenceFallbackAndUnresolvedOmission(t *testing.T) {
	revision := strings.Repeat("b", 40)
	artifactBytes := strings.Repeat("artifact", 64)
	resolved := inlineCandidate("resolved", retrieval.AuthorityCanonicalEvidence, "evidence/report.json", "", artifactBytes)
	resolved.ArtifactRange = &ArtifactRange{ArtifactID: uuid.NewString(), SHA256: hash([]byte(artifactBytes)), SizeBytes: int64(len(artifactBytes)), Start: 8, End: 72, MediaType: "application/json", Resolved: true}
	unresolved := Candidate{
		Retrieval:     retrieval.Candidate{Identity: "unresolved", Authority: retrieval.AuthorityStructural, SourceKind: "artifact", SourceIdentity: "artifact:missing", SourceSHA256: strings.Repeat("c", 64)},
		ArtifactRange: &ArtifactRange{ArtifactID: uuid.NewString(), SHA256: strings.Repeat("c", 64), SizeBytes: 100, Start: 10, End: 20, MediaType: "text/plain", Resolved: false},
	}
	value, err := Compile(CompileRequest{
		ProjectID: uuid.NewString(), Role: RoleAuditor, SourceRevision: revision,
		Budget:                 Budget{Bytes: 900, Tokens: 225},
		RetrievalConfiguration: retrieval.Report{ConfigurationVersion: retrieval.ConfigurationVersion, SourceRevision: revision},
		Candidates:             []Candidate{resolved, unresolved},
	})
	if err != nil {
		t.Fatal(err)
	}
	var unresolvedManifest ManifestItem
	for _, item := range value.Manifest.Items {
		if item.CandidateIdentity == "unresolved" {
			unresolvedManifest = item
		}
	}
	if unresolvedManifest.Included || unresolvedManifest.StorageForm != StorageOmitted || unresolvedManifest.OmissionReason != "unresolved_artifact_reference" || unresolvedManifest.Retrieval.Method != "host_query.artifact_range" || unresolvedManifest.Retrieval.Start != 10 || unresolvedManifest.Retrieval.End != 20 {
		t.Fatalf("unresolved manifest = %#v", unresolvedManifest)
	}
	var dossier Dossier
	if err := json.Unmarshal(value.Dossier, &dossier); err != nil {
		t.Fatal(err)
	}
	for _, item := range dossier.Items {
		if item.CandidateIdentity == "unresolved" {
			t.Fatal("unresolved reference was silently admitted")
		}
	}
}

func TestDefaultBudgetsCompileForEveryRole(t *testing.T) {
	roles := []Role{RoleSupervisor, RolePlanner, RoleImplementer, RoleAuditor, RoleCorrector, RoleDocumentor, RoleSimplifier}
	for _, role := range roles {
		t.Run(string(role), func(t *testing.T) {
			value, err := Compile(CompileRequest{
				ProjectID: uuid.NewString(), Role: role, SourceRevision: strings.Repeat("d", 40),
				RetrievalConfiguration: retrieval.Report{ConfigurationVersion: retrieval.ConfigurationVersion, SourceRevision: strings.Repeat("d", 40)},
				Candidates:             []Candidate{inlineCandidate("source", retrieval.AuthorityExactSource, "source.go", "Source", "func Source() {}")},
			})
			if err != nil || value.Manifest.IncludedCount != 1 || value.Manifest.FinalBytes > value.Manifest.ByteBudget || value.Manifest.FinalTokens > value.Manifest.TokenBudget {
				t.Fatalf("package = %#v, %v", value, err)
			}
		})
	}
}

func inlineCandidate(identity string, authority retrieval.AuthorityClass, path, symbol, content string) Candidate {
	return Candidate{Retrieval: retrieval.Candidate{
		Identity: identity, Authority: authority, SourceKind: "code_chunk", SourceIdentity: path,
		SourceSHA256: hash([]byte(content)), Path: path, Symbol: symbol, Content: content,
	}}
}
