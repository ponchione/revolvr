package autonomous

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestOptionalRoleAssessmentRequiresStructuredCurrentRelevance(t *testing.T) {
	for _, test := range []struct {
		name        string
		role        WorkerProfile
		kind        OptionalRoleEvidenceKind
		path        string
		disposition OptionalRoleDisposition
		want        string
	}{
		{"documentation target runs", WorkerProfileDocumentor, OptionalRoleEvidenceTaskDocumentation, "README.md", OptionalRoleDispositionRun, ""},
		{"simplification target runs", WorkerProfileSimplifier, OptionalRoleEvidenceComplexityTarget, "internal/parser.go", OptionalRoleDispositionRun, ""},
		{"documentation absence skips", WorkerProfileDocumentor, OptionalRoleEvidenceNoRelevantWork, "", OptionalRoleDispositionNotApplicable, ""},
		{"simplification absence skips", WorkerProfileSimplifier, OptionalRoleEvidenceNoRelevantWork, "", OptionalRoleDispositionNotApplicable, ""},
		{"rationale alone cannot run", WorkerProfileDocumentor, OptionalRoleEvidenceNoRelevantWork, "", OptionalRoleDispositionRun, "concrete selected role target"},
		{"generic cleanup cannot run", WorkerProfileSimplifier, OptionalRoleEvidenceUserFacingChange, "README.md", OptionalRoleDispositionRun, "concrete selected role target"},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := optionalAssessment(test.role, test.disposition, test.kind, test.path)
			err := a.Validate()
			if test.want == "" && err != nil {
				t.Fatal(err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("error=%v want %q", err, test.want)
			}
		})
	}
}

func TestOptionalRoleAssessmentCannotWaiveDocumentationObligation(t *testing.T) {
	a := optionalAssessment(WorkerProfileDocumentor, OptionalRoleDispositionNotApplicable, OptionalRoleEvidenceNoRelevantWork, "")
	required := OptionalRoleEvidence{ID: "docs-required", Role: WorkerProfileDocumentor, Kind: OptionalRoleEvidenceAcceptanceDocumentation, Reference: optionalEvidence("acceptance:docs"), SourceRevision: optionalSHA("source"), TargetPath: "docs/usage.md"}
	a.Evidence = append(a.Evidence, required)
	if err := a.Validate(); err == nil || !strings.Contains(err.Error(), "cannot waive") {
		t.Fatalf("error=%v", err)
	}

	stale := optionalAssessment(WorkerProfileDocumentor, OptionalRoleDispositionRun, OptionalRoleEvidenceTaskDocumentation, "README.md")
	stale.Evidence[0].SourceRevision = optionalSHA("old")
	if err := stale.Validate(); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale error=%v", err)
	}
}

func TestOptionalRoleAssessmentIdentityIgnoresSupervisorIncidentalProse(t *testing.T) {
	first := optionalAssessment(WorkerProfileDocumentor, OptionalRoleDispositionRun, OptionalRoleEvidenceTaskDocumentation, "README.md")
	second := first
	second.Decision.Rationale = "Formatting-only replacement rationale."
	second.DecisionReference.DecisionID = "decision-reformatted"
	second.DecisionReference.RunID = "supervisor-reformatted"
	second.DecisionReference.CreatedAt = second.DecisionReference.CreatedAt.Add(time.Hour)
	firstSHA, err := first.Identity()
	if err != nil {
		t.Fatal(err)
	}
	secondSHA, err := second.Identity()
	if err != nil {
		t.Fatal(err)
	}
	if firstSHA != secondSHA {
		t.Fatalf("incidental supervisor metadata changed relevance identity: %s != %s", firstSHA, secondSHA)
	}
	second.Evidence[0].Reference.Reference = "different-exact-evidence"
	second.Decision.Inputs[0] = second.Evidence[0].Reference
	thirdSHA, err := second.Identity()
	if err != nil {
		t.Fatal(err)
	}
	if thirdSHA == firstSHA {
		t.Fatal("material evidence change retained relevance identity")
	}
}

func TestOptionalRoleOccurrenceDistinguishesSkipNoopAndSourceChange(t *testing.T) {
	for _, outcome := range []OptionalRoleOutcome{OptionalRoleOutcomeNotApplicable, OptionalRoleOutcomeNoChange, OptionalRoleOutcomeSourceChanged} {
		t.Run(string(outcome), func(t *testing.T) {
			o := optionalOccurrence(outcome, WorkerProfileDocumentor)
			if err := o.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
	simplify := optionalOccurrence(OptionalRoleOutcomeSourceChanged, WorkerProfileSimplifier)
	simplify.BehaviorPreservation = nil
	if err := simplify.Validate(); err == nil || !strings.Contains(err.Error(), "behavior-preservation") {
		t.Fatalf("error=%v", err)
	}
	noChange := optionalOccurrence(OptionalRoleOutcomeNoChange, WorkerProfileDocumentor)
	noChange.SourceAfter = optionalSHA("changed")
	noChange.Gate.SourceRevision = noChange.SourceAfter
	if err := noChange.Validate(); err == nil || !strings.Contains(err.Error(), "unchanged source") {
		t.Fatalf("error=%v", err)
	}
}

func optionalAssessment(role WorkerProfile, disposition OptionalRoleDisposition, kind OptionalRoleEvidenceKind, path string) OptionalRoleAssessment {
	action := ActionDocument
	if role == WorkerProfileSimplifier {
		action = ActionSimplify
	}
	ref := optionalEvidence("evidence-one")
	decision := SupervisorDecision{TaskID: "task-optional", Action: action, WorkerProfile: role, Rationale: "Use exact structured evidence.", SuccessCriteria: []string{"Record the exact conditional outcome."}, Inputs: []EvidenceReference{ref}, Strategy: &Strategy{Approach: "work only on the selected exact target", Targets: []EvidenceReference{ref}}}
	return OptionalRoleAssessment{SchemaVersion: OptionalRoleAssessmentSchemaVersion, TaskID: "task-optional", Role: role, Disposition: disposition, Decision: decision, DecisionReference: DecisionReference{DecisionID: "decision-optional", RunID: "supervisor-optional", TaskID: "task-optional", Action: action, WorkerProfile: role, Artifact: optionalEvidence("decision-artifact"), CreatedAt: time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)}, TaskSource: EvidenceReference{Kind: EvidenceKindTask, Reference: ".agent/tasks/task-optional.md", Detail: "Exact canonical task bytes."}, StateSHA256: optionalSHA("state"), SourceRevision: optionalSHA("source"), VerificationRunID: "verification-run", VerificationID: "verification-occurrence", AuditRunID: "audit-run", AuditSourceRevision: optionalSHA("source"), Evidence: []OptionalRoleEvidence{{ID: "evidence-one", Role: role, Kind: kind, Reference: ref, SourceRevision: optionalSHA("source"), TargetPath: path}}, SelectedEvidenceIDs: []string{"evidence-one"}, Rationale: "The exact evidence determines this disposition."}
}

func optionalOccurrence(outcome OptionalRoleOutcome, role WorkerProfile) OptionalRoleOccurrence {
	a := optionalAssessment(role, OptionalRoleDispositionRun, OptionalRoleEvidenceTaskDocumentation, "README.md")
	if role == WorkerProfileSimplifier {
		a = optionalAssessment(role, OptionalRoleDispositionRun, OptionalRoleEvidenceComplexityTarget, "internal/parser.go")
	}
	assessmentSHA, _ := a.Identity()
	sourceAfter := optionalSHA("source")
	var worker *OptionalRoleWorkerEvidence
	var paths []string
	commit := ""
	var behavior []EvidenceReference
	if outcome != OptionalRoleOutcomeNotApplicable {
		worker = &OptionalRoleWorkerEvidence{AttemptID: "attempt-one", RunID: "worker-run", DossierSHA256: optionalSHA("dossier"), DossierByteSize: 10, ProfilePath: ".agent/profiles/" + string(role) + ".md", ProfileSHA256: optionalSHA("profile"), ProfileByteSize: 20, Receipt: EvidenceReference{Kind: EvidenceKindReceipt, Reference: "receipt.json", Detail: "Exact receipt."}, Ledger: optionalEvidence("worker-ledger")}
	}
	if outcome == OptionalRoleOutcomeSourceChanged {
		sourceAfter = optionalSHA("changed")
		paths, commit = []string{"README.md"}, "commit-sha"
		behavior = []EvidenceReference{optionalEvidence("behavior-tests")}
	}
	return OptionalRoleOccurrence{SchemaVersion: OptionalRoleOccurrenceSchemaVersion, Sequence: 1, TaskID: "task-optional", Role: role, Outcome: outcome, Decision: a.DecisionReference, AssessmentSHA256: assessmentSHA, SourceBefore: optionalSHA("source"), SourceAfter: sourceAfter, Gate: OptionalRoleGate{SourceRevision: sourceAfter, VerificationRunID: "verification-run", VerificationOccurrenceID: "verification-occurrence", AuditSupervisorRunID: "audit-supervisor", AuditWorkerRunID: "audit-worker", AuditRevision: 1}, Worker: worker, ChangedPaths: paths, CommitSHA: commit, BehaviorPreservation: behavior, Evidence: []EvidenceReference{optionalEvidence("occurrence")}, Rationale: "Exact optional-role outcome.", CreatedAt: time.Date(2026, 7, 11, 13, 0, 0, 0, time.UTC)}
}

func optionalEvidence(reference string) EvidenceReference {
	return EvidenceReference{Kind: EvidenceKindLedger, Reference: reference, Detail: "Exact durable evidence."}
}

func optionalSHA(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	return fmt.Sprintf("%x", sum)
}
