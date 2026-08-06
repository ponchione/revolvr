package completion

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"revolvr/internal/evidence"
)

var (
	sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
	gitOIDPattern = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)
	imagePattern  = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

func BuildPreflight(snapshot Snapshot) (Preflight, error) {
	normalized := normalizeSnapshot(snapshot)
	hash, err := evidence.Hash(normalized)
	if err != nil {
		return Preflight{}, err
	}
	preflight := Preflight{
		SchemaVersion: PreflightSchemaVersion,
		SHA256:        hash,
		Snapshot:      normalized,
		Rejections:    evaluate(normalized),
	}
	return preflight, nil
}

func normalizeSnapshot(snapshot Snapshot) Snapshot {
	if snapshot.Criteria == nil {
		snapshot.Criteria = []Criterion{}
	}
	if snapshot.Findings == nil {
		snapshot.Findings = []Finding{}
	}
	if snapshot.Invocations == nil {
		snapshot.Invocations = []Invocation{}
	}
	if snapshot.Artifacts == nil {
		snapshot.Artifacts = []evidence.ArtifactReference{}
	}
	if snapshot.OperatorInputs == nil {
		snapshot.OperatorInputs = []OperatorInput{}
	}
	if snapshot.Claims == nil {
		snapshot.Claims = []evidence.Claim{}
	}
	if snapshot.Trajectory.Artifacts == nil {
		snapshot.Trajectory.Artifacts = []evidence.TrajectoryArtifact{}
	}
	if snapshot.HarnessAssets.Assets == nil {
		snapshot.HarnessAssets.Assets = []evidence.HarnessAsset{}
	}
	if snapshot.Plan != nil && snapshot.Plan.Steps == nil {
		snapshot.Plan.Steps = []PlanStep{}
	}
	if snapshot.Verification != nil && snapshot.Verification.Checks == nil {
		snapshot.Verification.Checks = []VerificationCheck{}
	}
	slices.SortFunc(snapshot.Criteria, func(a, b Criterion) int { return strings.Compare(a.ID, b.ID) })
	slices.SortFunc(snapshot.Findings, func(a, b Finding) int { return strings.Compare(a.ID, b.ID) })
	slices.SortFunc(snapshot.Invocations, func(a, b Invocation) int {
		if result := strings.Compare(a.Role, b.Role); result != 0 {
			return result
		}
		return strings.Compare(a.PromptSHA256, b.PromptSHA256)
	})
	slices.SortFunc(snapshot.Artifacts, func(a, b evidence.ArtifactReference) int {
		if result := strings.Compare(a.Kind, b.Kind); result != 0 {
			return result
		}
		return strings.Compare(a.ID, b.ID)
	})
	slices.SortFunc(snapshot.OperatorInputs, func(a, b OperatorInput) int { return strings.Compare(a.ID, b.ID) })
	slices.SortFunc(snapshot.Claims, func(a, b evidence.Claim) int { return strings.Compare(a.ID, b.ID) })
	slices.SortFunc(snapshot.HarnessAssets.Assets, func(a, b evidence.HarnessAsset) int {
		if result := strings.Compare(a.ID, b.ID); result != 0 {
			return result
		}
		return strings.Compare(a.Version, b.Version)
	})
	return snapshot
}

func evaluate(snapshot Snapshot) []Rejection {
	var rejected []Rejection
	reject := func(reason Reason, format string, args ...any) {
		rejected = append(rejected, Rejection{Reason: reason, Detail: fmt.Sprintf(format, args...)})
	}
	if snapshot.SchemaVersion != SnapshotSchemaVersion || snapshot.Identity.ProjectID == "" ||
		snapshot.Identity.TaskID == "" || snapshot.Identity.TaskVersionID == "" || snapshot.Identity.RunID == "" ||
		snapshot.Identity.WorkspaceID == "" || snapshot.TaskStatus != "finalizing" || snapshot.RunStatus != "active" ||
		snapshot.Aggregates.Task <= 0 || snapshot.Aggregates.Run <= 0 || snapshot.Aggregates.Workspace <= 0 ||
		snapshot.Aggregates.Plan <= 0 || snapshot.Aggregates.Lease < 0 {
		reject(ReasonTaskAuthorityChanged, "accepted task, run, workspace, or aggregate authority is not finalization-ready")
	}
	if snapshot.Plan == nil || snapshot.Plan.ID == "" || snapshot.Plan.VersionID == "" ||
		!sha256Pattern.MatchString(snapshot.Plan.SHA256) || len(snapshot.Plan.Steps) == 0 {
		reject(ReasonPlanMissing, "accepted plan or exact accepted plan version is missing")
	} else {
		for _, step := range snapshot.Plan.Steps {
			if step.Status != "completed" && step.Status != "skipped" {
				reject(ReasonPlanStepNonterminal, "plan step %q has status %q", step.ID, step.Status)
				break
			}
		}
	}
	if len(snapshot.Criteria) == 0 {
		reject(ReasonCriterionNonterminal, "accepted task has no canonical criterion outcomes")
	}
	for _, criterion := range snapshot.Criteria {
		switch criterion.Status {
		case "pending":
			reject(ReasonCriterionNonterminal, "criterion %q remains pending", criterion.ID)
		case "failed", "blocked":
			reject(ReasonCriterionUnsatisfied, "criterion %q has unsatisfied status %q", criterion.ID, criterion.Status)
		case "passed", "waived", "not_applicable":
		default:
			reject(ReasonCriterionNonterminal, "criterion %q has unsupported status %q", criterion.ID, criterion.Status)
		}
	}
	verificationFresh := validateVerification(snapshot, reject)
	validateAudit(snapshot, verificationFresh, reject)
	for _, finding := range snapshot.Findings {
		if finding.Significance == "blocking" && finding.Status == "open" {
			reject(ReasonBlockingFindingOpen, "blocking finding %q remains open", finding.ID)
			break
		}
	}
	if !gitOIDPattern.MatchString(snapshot.Source.BeforeCommit) || !gitOIDPattern.MatchString(snapshot.Source.BeforeTree) ||
		!gitOIDPattern.MatchString(snapshot.Source.AfterCommit) || !gitOIDPattern.MatchString(snapshot.Source.AfterTree) ||
		!sha256Pattern.MatchString(snapshot.Source.DiffSHA256) || snapshot.Source.FrozenAt.IsZero() {
		reject(ReasonCommitInvalid, "source commit, tree, diff, or freeze authority is invalid")
	}
	if snapshot.Source.AfterCommit != snapshot.Workspace.CandidateCommit ||
		snapshot.Source.AfterTree != snapshot.Workspace.CandidateTree || snapshot.Source.DiffSHA256 != snapshot.Workspace.DiffSHA256 {
		reject(ReasonSourceRevisionChanged, "workspace candidate source or diff does not match completion source")
	}
	budgetMaterial := snapshot.Budget
	budgetMaterial.SHA256 = ""
	budgetHash, _ := evidence.Hash(budgetMaterial)
	if snapshot.Budget.SchemaVersion == "" || snapshot.Budget.SHA256 != budgetHash || snapshot.Budget.Limit <= 0 ||
		snapshot.Budget.Consumed < 0 || snapshot.Budget.Consumed > snapshot.Budget.Limit || snapshot.Budget.InFlight != 0 {
		reject(ReasonBudgetInvalid, "budget hash, bounds, consumption, or in-flight accounting is invalid")
	}
	if snapshot.Workspace.Status != "frozen" || !snapshot.Workspace.Reconciled {
		reject(ReasonWorkspaceUnreconciled, "workspace is not frozen and reconciled")
	}
	if snapshot.Lease.Name != "global-source-mutation-v1" || !snapshot.Lease.Held || snapshot.Lease.RunID != snapshot.Identity.RunID {
		reject(ReasonLeaseUnreconciled, "global source-mutation lease is not held by the completing run")
	}
	validateInvocations(snapshot, reject)
	validateArtifacts(snapshot, reject)
	if err := snapshot.Trajectory.Validate(); err != nil {
		reject(ReasonTrajectoryProvenanceInvalid, "%v", err)
	}
	if err := snapshot.HarnessAssets.Validate(); err != nil {
		reject(ReasonHarnessAssetsInvalid, "%v", err)
	} else if snapshot.HarnessAssets.RuntimeKind != snapshot.Trajectory.RuntimeKind {
		reject(ReasonHarnessAssetsInvalid, "trajectory and harness asset-set runtime kinds diverge")
	}
	validateOperatorInputs(snapshot, reject)
	validateClaims(snapshot, reject)
	return rejected
}

func validateVerification(snapshot Snapshot, reject func(Reason, string, ...any)) bool {
	verification := snapshot.Verification
	if verification == nil || verification.ID == "" || verification.Purpose != "final" || verification.CompletedAt.IsZero() ||
		verification.SourceCommit != snapshot.Source.AfterCommit || verification.SourceTree != snapshot.Source.AfterTree ||
		verification.CompletedAt.Before(snapshot.Source.FrozenAt) || !imagePattern.MatchString(verification.ImageDigest) ||
		verification.Profile == "" || !sha256Pattern.MatchString(verification.ProfileSHA256) {
		reject(ReasonVerificationStale, "fresh source-bound final verification is missing or stale")
		return false
	}
	if verification.Status != "passed" || len(verification.Checks) == 0 {
		reject(ReasonVerificationFailed, "final verification status is %q", verification.Status)
		return false
	}
	freshTier4 := false
	for _, check := range verification.Checks {
		if !sha256Pattern.MatchString(check.ExecutionFingerprint) || !imagePattern.MatchString(check.ImageDigest) ||
			check.ImageDigest != verification.ImageDigest || check.Profile != verification.Profile ||
			check.ProfileSHA256 != verification.ProfileSHA256 ||
			(check.Outcome != "passed" && check.Outcome != "passed_reused") {
			reject(ReasonVerificationFailed, "verification check %q is invalid or nonpassing", check.ID)
			return false
		}
		if check.Tier == 4 && check.Outcome == "passed" && check.ReusedFromCheckID == "" {
			freshTier4 = true
		}
	}
	if !freshTier4 {
		reject(ReasonVerificationStale, "completion requires a freshly executed passing Tier 4 check")
		return false
	}
	return true
}

func validateAudit(snapshot Snapshot, verificationFresh bool, reject func(Reason, string, ...any)) {
	audit := snapshot.Audit
	if audit == nil || audit.ID == "" {
		reject(ReasonAuditMissing, "independent audit evidence is missing")
		return
	}
	if audit.SchemaVersion != AuditSchemaVersion || !audit.Independent || audit.Role != "auditor" ||
		audit.RunID != snapshot.Identity.RunID || audit.SourceCommit != snapshot.Source.AfterCommit ||
		audit.SourceTree != snapshot.Source.AfterTree || audit.CompletedAt.IsZero() ||
		(verificationFresh && audit.CompletedAt.Before(snapshot.Verification.CompletedAt)) ||
		audit.ReportArtifactID == "" || !sha256Pattern.MatchString(audit.ReportSHA256) {
		reject(ReasonAuditStale, "audit is not fresh, independent, source-bound, and artifact-backed")
	}
	if audit.Disposition != "clean" {
		reject(ReasonAuditChangesRequired, "audit disposition is %q", audit.Disposition)
	}
}

func validateInvocations(snapshot Snapshot, reject func(Reason, string, ...any)) {
	if len(snapshot.Invocations) == 0 {
		reject(ReasonPromptModelAuthorityMissing, "no prompt/model authority is recorded")
		return
	}
	for _, invocation := range snapshot.Invocations {
		if invocation.Role == "" || invocation.Model == "" || invocation.PromptVersion == "" ||
			!sha256Pattern.MatchString(invocation.PromptSHA256) || !sha256Pattern.MatchString(invocation.DossierSHA256) ||
			!imagePattern.MatchString(invocation.ImageDigest) || invocation.Profile == "" {
			reject(ReasonPromptModelAuthorityMissing, "role %q has incomplete prompt, model, image, or profile authority", invocation.Role)
			return
		}
	}
}

func validateArtifacts(snapshot Snapshot, reject func(Reason, string, ...any)) {
	manifestHash, err := evidence.ArtifactManifestHash(snapshot.Artifacts)
	if err != nil || manifestHash != snapshot.ArtifactManifestSHA256 || len(snapshot.Artifacts) == 0 {
		reject(ReasonArtifactManifestIncomplete, "artifact manifest is missing, unresolved, divergent, or changed")
		return
	}
	byID := make(map[string]evidence.ArtifactReference, len(snapshot.Artifacts))
	for _, artifact := range snapshot.Artifacts {
		byID[artifact.ID] = artifact
	}
	if snapshot.Audit != nil {
		artifact, ok := byID[snapshot.Audit.ReportArtifactID]
		if !ok || artifact.SHA256 != snapshot.Audit.ReportSHA256 {
			reject(ReasonArtifactManifestIncomplete, "audit report artifact is absent or changed")
		}
	}
	for _, input := range snapshot.OperatorInputs {
		if artifact, ok := byID[input.ArtifactID]; input.ArtifactID != "" && (!ok || artifact.SHA256 != input.SHA256) {
			reject(ReasonArtifactManifestIncomplete, "operator input %q artifact is absent or changed", input.ID)
			break
		}
	}
}

func validateOperatorInputs(snapshot Snapshot, reject func(Reason, string, ...any)) {
	for _, input := range snapshot.OperatorInputs {
		if input.ID == "" || input.Version <= 0 || !sha256Pattern.MatchString(input.SHA256) || input.ArtifactID == "" || !input.Resolved {
			reject(ReasonOperatorInputInvalid, "operator input %q is not exact and resolved", input.ID)
			return
		}
	}
}

func validateClaims(snapshot Snapshot, reject func(Reason, string, ...any)) {
	if len(snapshot.Claims) == 0 {
		reject(ReasonClaimEvidenceInvalid, "completion has no evidence-backed acceptance claims")
		return
	}
	artifacts := make(map[string]string, len(snapshot.Artifacts))
	for _, artifact := range snapshot.Artifacts {
		artifacts[artifact.ID] = artifact.SHA256
	}
	checks := make(map[string]string)
	if snapshot.Verification != nil {
		for _, check := range snapshot.Verification.Checks {
			checks[check.ID] = check.ExecutionFingerprint
		}
	}
	claimedCriteria := make(map[string]struct{}, len(snapshot.Claims))
	for _, claim := range snapshot.Claims {
		if err := claim.Validate(); err != nil {
			reject(ReasonClaimEvidenceInvalid, "claim %q is invalid: %v", claim.ID, err)
			return
		}
		for _, link := range claim.Evidence {
			var got string
			if link.Kind == "artifact" {
				got = artifacts[link.ID]
			} else {
				got = checks[link.ID]
			}
			if got != link.SHA256 {
				reject(ReasonClaimEvidenceInvalid, "claim %q evidence %q is absent or divergent", claim.ID, link.ID)
				return
			}
		}
		if claim.CriterionID != "" {
			claimedCriteria[claim.CriterionID] = struct{}{}
		}
	}
	for _, criterion := range snapshot.Criteria {
		if _, ok := claimedCriteria[criterion.ID]; !ok {
			reject(ReasonClaimEvidenceInvalid, "criterion %q has no exact evidence-backed claim", criterion.ID)
			return
		}
	}
}
