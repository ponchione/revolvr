package app

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"revolvr/internal/artifactretention"
	"revolvr/internal/ledgerexport"
	"revolvr/internal/runonce"
)

type GCPlanInput struct {
	OperationID string
	FrozenAt    time.Time
}
type GCApplyInput struct{ Plan artifactretention.Plan }
type LedgerExportInput struct {
	OperationID   string
	ExportedAt    time.Time
	Bounds        ledgerexport.Bounds
	PredecessorID string
}

func PlanArtifactGC(ctx context.Context, cfg Config, in GCPlanInput) (artifactretention.Plan, error) {
	paths, runCfg, fingerprint, err := retentionAuthority(cfg.WorkDir)
	if err != nil {
		return artifactretention.Plan{}, err
	}
	return artifactretention.PlanGC(ctx, artifactretention.PlanInput{RepositoryRoot: paths.WorkDir, LedgerPath: paths.LedgerDBPath, OperationID: in.OperationID, FrozenAt: in.FrozenAt, Policy: runCfg.RetentionPolicy, EffectiveConfigSHA256: fingerprint.SHA256})
}

func ApplyArtifactGC(ctx context.Context, cfg Config, in GCApplyInput) (artifactretention.ApplyResult, error) {
	paths, runCfg, fingerprint, err := retentionAuthority(cfg.WorkDir)
	if err != nil {
		return artifactretention.ApplyResult{}, err
	}
	if fingerprint.SHA256 != in.Plan.EffectiveConfigSHA256 || runCfg.RetentionPolicy != in.Plan.Policy {
		return artifactretention.ApplyResult{}, errors.New("artifact GC apply: effective configuration differs from plan")
	}
	return artifactretention.ApplyGC(ctx, artifactretention.ApplyInput{RepositoryRoot: paths.WorkDir, LedgerPath: paths.LedgerDBPath, Plan: in.Plan, Secrets: configuredSecrets(runCfg)})
}

func ResumeArtifactGC(ctx context.Context, cfg Config, operationID string) (artifactretention.ApplyResult, error) {
	paths, runCfg, _, err := retentionAuthority(cfg.WorkDir)
	if err != nil {
		return artifactretention.ApplyResult{}, err
	}
	return artifactretention.ResumeGC(ctx, paths.WorkDir, operationID, paths.LedgerDBPath, configuredSecrets(runCfg))
}
func InspectArtifactGC(ctx context.Context, cfg Config, operationID string) (artifactretention.Journal, bool, error) {
	paths, err := resolveStatePaths(cfg.WorkDir)
	if err != nil {
		return artifactretention.Journal{}, false, err
	}
	return artifactretention.InspectGC(paths.WorkDir, operationID)
}

func ExportLedger(ctx context.Context, cfg Config, in LedgerExportInput) (ledgerexport.Result, error) {
	paths, runCfg, _, err := retentionAuthority(cfg.WorkDir)
	if err != nil {
		return ledgerexport.Result{}, err
	}
	policyHash, _, _ := runCfg.RetentionPolicy.Fingerprint()
	return ledgerexport.Export(ctx, ledgerexport.ExportInput{RepositoryRoot: paths.WorkDir, LedgerPath: paths.LedgerDBPath, OperationID: in.OperationID, ExportedAt: in.ExportedAt, PolicySHA256: policyHash, Bounds: in.Bounds, PredecessorID: in.PredecessorID, Secrets: configuredSecrets(runCfg)})
}
func VerifyLedgerExport(ctx context.Context, cfg Config, exportID string) (ledgerexport.VerifyReport, error) {
	paths, runCfg, _, err := retentionAuthority(cfg.WorkDir)
	if err != nil {
		return ledgerexport.VerifyReport{}, err
	}
	return ledgerexport.Verify(ctx, paths.WorkDir, exportID, configuredSecrets(runCfg))
}
func ReplayValidateLedgerExport(ctx context.Context, cfg Config, exportID string) (ledgerexport.ReplayReport, error) {
	paths, runCfg, _, err := retentionAuthority(cfg.WorkDir)
	if err != nil {
		return ledgerexport.ReplayReport{}, err
	}
	return ledgerexport.ReplayValidate(ctx, paths.WorkDir, exportID, configuredSecrets(runCfg))
}

func retentionAuthority(workDir string) (statePaths, runonce.Config, runonce.EffectiveConfigFingerprint, error) {
	paths, err := resolveStatePaths(workDir)
	if err != nil {
		return statePaths{}, runonce.Config{}, runonce.EffectiveConfigFingerprint{}, err
	}
	cfg, err := LoadRunOnceConfig(paths.WorkDir, DefaultRunOnceConfig(paths.WorkDir))
	if err != nil {
		return statePaths{}, runonce.Config{}, runonce.EffectiveConfigFingerprint{}, err
	}
	effective, err := runonce.EffectiveConfig(cfg)
	if err != nil {
		return statePaths{}, runonce.Config{}, runonce.EffectiveConfigFingerprint{}, err
	}
	fingerprint, err := runonce.FingerprintEffectiveConfig(effective)
	return paths, effective, fingerprint, err
}
func configuredSecrets(cfg runonce.Config) []string {
	var out []string
	for _, name := range cfg.SafetyDeclaration.Redaction.EnvironmentVariables {
		if value, ok := os.LookupEnv(strings.TrimSpace(name)); ok && value != "" {
			out = append(out, value)
		}
	}
	return out
}
