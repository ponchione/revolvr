package correction

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"time"

	"revolvr/internal/audit"
	"revolvr/internal/sandbox"
	"revolvr/internal/tool"
	"revolvr/internal/verification"
)

type Config struct {
	OperationID           string
	StrategyID            string
	OutcomeID             string
	Identity              audit.Identity
	DossierInput          DossierInput
	Strategy              Strategy
	Budget                Budget
	CorrectorInvocationID string
	Sandbox               sandbox.Specification
	ToolRegistry          tool.Registry
	Worker                Worker
	Verifier              Verifier
	Auditor               Auditor
	Store                 Store
	Dispositioner         Dispositioner
	DispositionID         func(string) string
	Clock                 func() time.Time
}

func Run(ctx context.Context, cfg Config) (Result, error) {
	failure, err := NormalizeFailure(cfg.DossierInput.Authority)
	if err != nil {
		return Result{Reason: err.Error()}, err
	}
	result := Result{Failure: failure}
	if cfg.Budget.MaximumCycles <= 0 || cfg.Budget.MaximumAttempts <= 0 || cfg.Budget.ConsumedCycles >= cfg.Budget.MaximumCycles || cfg.Budget.ConsumedAttempts >= cfg.Budget.MaximumAttempts {
		result.Outcome, result.Reason = OutcomeBudgetExhausted, ErrBudget.Error()
		return result, nil
	}
	dossier, err := BuildDossier(cfg.DossierInput)
	if err != nil {
		return result, err
	}
	fingerprint, normalizedStrategy, err := StrategyFingerprint(cfg.Strategy)
	if err != nil {
		return result, err
	}
	result.Strategy, result.Fingerprint = normalizedStrategy, fingerprint
	if err := validateConfig(cfg, dossier, normalizedStrategy); err != nil {
		return result, err
	}
	repeated, err := cfg.Store.HasFailedStrategy(ctx, cfg.Identity.TaskID, failure.SHA256, fingerprint)
	if err != nil {
		return result, err
	}
	if repeated {
		result.Outcome, result.Reason = OutcomeRepeatedStrategy, ErrRepeatedStrategy.Error()
		return result, nil
	}
	sandboxSHA, _ := cfg.Sandbox.SHA256()
	started := now(cfg.Clock)
	if err := cfg.Store.Begin(ctx, cfg.Identity, AttemptRecord{
		OperationID: cfg.OperationID, StrategyID: cfg.StrategyID, Failure: failure,
		Strategy: normalizedStrategy, StrategyFingerprint: fingerprint,
		DossierSHA256: dossier.SHA256, CorrectorInvocationID: cfg.CorrectorInvocationID,
		SandboxSpecificationSHA256: sandboxSHA, StartedAt: started,
	}); err != nil {
		return result, err
	}
	finish := func(outcome Outcome, stored string, source audit.Source, verificationID, auditID string, evidence []audit.DispositionEvidence, reason error) (Result, error) {
		result.Outcome = outcome
		if reason != nil {
			result.Reason = reason.Error()
		}
		if err := cfg.Store.Complete(context.WithoutCancel(ctx), cfg.Identity, OutcomeRecord{
			ID: cfg.OutcomeID, StrategyID: cfg.StrategyID, Outcome: stored,
			ResultingSource: source, VerificationRunID: verificationID, AuditRunID: auditID,
			Evidence: append([]audit.DispositionEvidence(nil), evidence...), CompletedAt: now(cfg.Clock),
		}); err != nil {
			return result, err
		}
		return result, nil
	}

	worker, workerErr := cfg.Worker.Run(ctx, WorkerRequest{
		TaskID: cfg.Identity.TaskID, Dossier: dossier, Authority: cfg.DossierInput.Authority,
		Strategy: normalizedStrategy, Sandbox: cfg.Sandbox,
		Registry: cfg.ToolRegistry,
	})
	result.Worker = worker
	if errors.Is(ctx.Err(), context.Canceled) || worker.Outcome == WorkerCancelled {
		return finish(OutcomeCancelled, "cancelled", worker.Source, "", "", worker.Evidence, context.Canceled)
	}
	if workerErr != nil || worker.Outcome != WorkerSucceeded {
		return finish(OutcomeCorrectionFailed, "failed", worker.Source, "", "", worker.Evidence, errors.Join(workerErr, errors.New(worker.Error)))
	}
	workerFingerprint, _, strategyErr := StrategyFingerprint(worker.Strategy)
	if strategyErr != nil || workerFingerprint != fingerprint || worker.InvocationID != cfg.CorrectorInvocationID {
		return finish(OutcomeCorrectionFailed, "failed", worker.Source, "", "", worker.Evidence, errors.New("corrector result changed strategy or invocation authority"))
	}
	if len(worker.ChangedFiles) == 0 || worker.Source == cfg.DossierInput.CurrentSource {
		return finish(OutcomeNoChanges, "no_progress", worker.Source, "", "", worker.Evidence, ErrNoProgress)
	}
	if err := validateChangedScope(worker.ChangedFiles, worker.ChangedSymbols, normalizedStrategy, cfg.DossierInput); err != nil {
		return finish(OutcomeCorrectionFailed, "blocked", worker.Source, "", "", worker.Evidence, err)
	}
	for _, prior := range cfg.DossierInput.PriorStrategies {
		if prior.DiffSHA256 != "" && prior.DiffSHA256 == worker.Source.DiffSHA256 {
			return finish(OutcomeIdenticalDiff, "no_progress", worker.Source, "", "", worker.Evidence, errors.New("correction reproduced an identical prior diff"))
		}
	}
	if len(worker.Evidence) == 0 {
		return finish(OutcomeNoEvidence, "no_progress", worker.Source, "", "", nil, errors.New("correction returned no exact evidence"))
	}

	verified, verifyErr := cfg.Verifier.Verify(ctx, VerificationRequest{TaskID: cfg.Identity.TaskID, Source: worker.Source, Purpose: verification.PurposeFinal})
	result.Verification = verified
	if verifyErr != nil || !freshFinalVerification(verified, worker.Source) {
		outcome := OutcomeVerificationFailed
		if sameFailure(cfg.DossierInput.Authority, verified) {
			outcome = OutcomeRepeatedFailure
		}
		return finish(outcome, "failed", worker.Source, verified.ID, "", worker.Evidence, errors.Join(verifyErr, errors.New("corrected source did not pass fresh full verification")))
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return finish(OutcomeCancelled, "cancelled", worker.Source, verified.ID, "", worker.Evidence, context.Canceled)
	}
	priorFindingIDs := failure.FindingIDs
	reaudit, auditErr := cfg.Auditor.Audit(ctx, AuditRequest{
		TaskID: cfg.Identity.TaskID, Source: worker.Source, Verification: verified,
		PreviousFindingIDs: priorFindingIDs, CorrectorInvocationID: cfg.CorrectorInvocationID,
	})
	result.Audit = reaudit
	if auditErr != nil || !validReaudit(reaudit, worker, verified) {
		return finish(OutcomeAuditFailed, "failed", worker.Source, verified.ID, reaudit.AuditID, worker.Evidence, errors.Join(auditErr, errors.New("fresh independent re-audit is invalid")))
	}
	if reaudit.Disposition == audit.DispositionBlocked {
		return finish(OutcomeBlocked, "blocked", worker.Source, verified.ID, reaudit.AuditID, worker.Evidence, nil)
	}
	if reaudit.Disposition == audit.DispositionChangesRequired {
		return finish(OutcomeChangesRequired, "failed", worker.Source, verified.ID, reaudit.AuditID, worker.Evidence, nil)
	}
	if reaudit.Disposition != audit.DispositionClean {
		return finish(OutcomeAuditFailed, "failed", worker.Source, verified.ID, reaudit.AuditID, worker.Evidence, errors.New("re-audit disposition is unknown"))
	}
	if cfg.DossierInput.Authority.Kind == AuthorityFindings {
		resolved := append([]string(nil), worker.ResolvedFindingIDs...)
		sort.Strings(resolved)
		if !slices.Equal(resolved, failure.FindingIDs) {
			return finish(OutcomeAuditFailed, "failed", worker.Source, verified.ID, reaudit.AuditID, worker.Evidence, errors.New("corrector did not resolve the exact complete cited finding set"))
		}
		commands := make([]audit.DispositionCommand, 0, len(failure.FindingIDs))
		for _, findingID := range failure.FindingIDs {
			commands = append(commands, audit.DispositionCommand{
				ID: cfg.DispositionID(findingID), OperationID: cfg.OperationID + ".resolve." + findingID,
				TaskID: cfg.Identity.TaskID, FindingID: findingID, Status: audit.FindingResolved,
				AuthorityRole: "host", AuthorityID: cfg.CorrectorInvocationID,
				ResolutionVerificationRunID: verified.ID, ResolutionAuditRunID: reaudit.AuditID,
				SourceCommit: worker.Source.Commit, SourceTree: worker.Source.Tree,
				Evidence: worker.Evidence, CreatedAt: now(cfg.Clock),
			})
		}
		values, err := cfg.Dispositioner.DispositionMany(ctx, commands)
		if err != nil {
			return finish(OutcomeAuditFailed, "failed", worker.Source, verified.ID, reaudit.AuditID, worker.Evidence, err)
		}
		result.Dispositions = append(result.Dispositions, values...)
	}
	return finish(OutcomeCorrectedClean, "succeeded", worker.Source, verified.ID, reaudit.AuditID, worker.Evidence, nil)
}

func validateConfig(cfg Config, dossier DossierArtifact, strategy Strategy) error {
	if cfg.Worker == nil || cfg.Verifier == nil || cfg.Auditor == nil || cfg.Store == nil || cfg.Dispositioner == nil || cfg.DispositionID == nil {
		return errors.New("correction requires worker, verifier, auditor, store, and disposition boundaries")
	}
	if cfg.OperationID == "" || cfg.StrategyID == "" || cfg.OutcomeID == "" || cfg.CorrectorInvocationID == "" || cfg.Identity != cfg.DossierInput.Identity {
		return errors.New("correction operation, strategy, outcome, invocation, or task identity is stale")
	}
	if cfg.Sandbox.Role != sandbox.RoleCorrector || cfg.Sandbox.ProjectID != cfg.Identity.ProjectID || cfg.Sandbox.TaskID != cfg.Identity.TaskID || cfg.Sandbox.RunID != cfg.CorrectorInvocationID {
		return errors.New("correction sandbox is not exact corrector authority")
	}
	if err := sandbox.CheckSpecification(cfg.Sandbox); err != nil {
		return fmt.Errorf("correction sandbox: %w", err)
	}
	expectedRegistry, err := tool.RegistryForRole(sandbox.RoleCorrector)
	if err != nil || !reflect.DeepEqual(cfg.ToolRegistry, expectedRegistry) {
		return errors.New("correction tool registry is not the shared corrector broker registry")
	}
	allowed := allowedFiles(cfg.DossierInput.Authority)
	for _, target := range strategy.TargetFiles {
		if !slices.Contains(allowed, target) {
			return fmt.Errorf("strategy target %q is outside cited correction scope", target)
		}
	}
	if dossier.SHA256 == "" {
		return errors.New("correction dossier identity is missing")
	}
	return nil
}

func validateChangedScope(changedFiles, changedSymbols []string, strategy Strategy, input DossierInput) error {
	seen := map[string]struct{}{}
	for _, file := range changedFiles {
		if _, duplicate := seen[file]; duplicate {
			return fmt.Errorf("corrector repeated changed path %q", file)
		}
		seen[file] = struct{}{}
		if !slices.Contains(strategy.TargetFiles, file) || !slices.Contains(allowedFiles(input.Authority), file) {
			return fmt.Errorf("corrector changed unrelated path %q", file)
		}
	}
	allowedSymbols := map[string]struct{}{}
	for _, file := range input.RelevantSource {
		for _, symbol := range file.Symbols {
			allowedSymbols[symbol] = struct{}{}
		}
	}
	seen = map[string]struct{}{}
	for _, symbol := range changedSymbols {
		if _, duplicate := seen[symbol]; duplicate {
			return fmt.Errorf("corrector repeated changed symbol %q", symbol)
		}
		seen[symbol] = struct{}{}
		if !slices.Contains(strategy.TargetSymbols, normalizeMeaning(symbol)) {
			return fmt.Errorf("corrector changed undeclared symbol %q", symbol)
		}
		if _, allowed := allowedSymbols[symbol]; !allowed {
			return fmt.Errorf("corrector changed unrelated symbol %q", symbol)
		}
	}
	return nil
}

func freshFinalVerification(value audit.VerificationEvidence, source audit.Source) bool {
	if value.ID == "" || value.Purpose != verification.PurposeFinal || value.Status != verification.RunPassed || value.Source.Commit != source.Commit || value.Source.Tree != source.Tree || value.CompletedAt.IsZero() {
		return false
	}
	for _, check := range value.Checks {
		if check.Tier == verification.TierFinal && check.Outcome == verification.OutcomePassed && check.ReusedFromCheckID == "" {
			return true
		}
	}
	return false
}

func validReaudit(value AuditResult, worker WorkerResult, verificationValue audit.VerificationEvidence) bool {
	dispositionValid := value.Disposition == audit.DispositionClean && len(value.Findings) == 0 ||
		value.Disposition == audit.DispositionChangesRequired && len(value.Findings) > 0 ||
		value.Disposition == audit.DispositionBlocked && len(value.Findings) == 0
	return dispositionValid && value.AuditID != "" && value.AuditorInvocationID != "" && value.AuditorInvocationID != worker.InvocationID && value.AuditID != verificationValue.ID && value.Source == worker.Source && value.VerificationRunID == verificationValue.ID && value.CompletedAt.After(verificationValue.CompletedAt)
}

func sameFailure(authority Authority, value audit.VerificationEvidence) bool {
	if authority.Verification == nil {
		return false
	}
	for _, check := range value.Checks {
		if check.ID == authority.Verification.CheckID && check.Outcome == authority.Verification.Outcome {
			return true
		}
	}
	return false
}

func now(clock func() time.Time) time.Time {
	if clock == nil {
		return time.Now().UTC().Truncate(time.Microsecond)
	}
	return clock().UTC().Truncate(time.Microsecond)
}
