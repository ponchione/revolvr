package app

import (
	"context"
	"errors"
	"os"
	"time"

	"revolvr/internal/autonomousarchive"
	"revolvr/internal/ledger"
	"revolvr/internal/runonce"
)

type ArchiveTaskInput struct {
	TaskID          string
	OperationID     string
	ArchiveRunID    string
	Disposition     autonomousarchive.Disposition
	Reason          string
	Provenance      string
	TerminalAt      time.Time
	ArchivedAt      time.Time
	CommandRunner   autonomousarchive.CommandRunner
	FailureInjector func(autonomousarchive.FailurePoint) error
}

type ReopenArchiveInput struct {
	Selector      string
	OperationID   string
	NewTaskID     string
	Authority     string
	Reason        string
	ReopenedAt    time.Time
	CommandRunner autonomousarchive.CommandRunner
}

func ListArchives(ctx context.Context, cfg Config) ([]autonomousarchive.Entry, error) {
	paths, err := resolveStatePaths(cfg.WorkDir)
	if err != nil {
		return nil, err
	}
	return autonomousarchive.List(paths.WorkDir)
}

func ShowArchive(ctx context.Context, cfg Config, selector string) (autonomousarchive.Entry, error) {
	paths, err := resolveStatePaths(cfg.WorkDir)
	if err != nil {
		return autonomousarchive.Entry{}, err
	}
	return autonomousarchive.Show(paths.WorkDir, selector)
}

func VerifyArchive(ctx context.Context, cfg Config, selector string) (autonomousarchive.VerificationReport, error) {
	paths, err := resolveStatePaths(cfg.WorkDir)
	if err != nil {
		return autonomousarchive.VerificationReport{}, err
	}
	runCfg, err := LoadRunOnceConfig(paths.WorkDir, DefaultRunOnceConfig(paths.WorkDir))
	if err != nil {
		return autonomousarchive.VerificationReport{}, err
	}
	store, err := ledger.OpenLiveReadOnly(ctx, paths.LedgerDBPath)
	if err != nil {
		return autonomousarchive.VerificationReport{}, err
	}
	defer store.Close()
	values := archiveSecretValues(runCfg)
	return autonomousarchive.Verify(ctx, autonomousarchive.VerifyConfig{RepositoryRoot: paths.WorkDir, Ledger: store, GitExecutable: runCfg.GitExecutable, GitTimeout: runCfg.GitTimeout, ForbiddenValues: values}, selector)
}

func ArchiveTask(ctx context.Context, cfg Config, input ArchiveTaskInput) (autonomousarchive.ArchiveResult, error) {
	paths, runCfg, store, closeStore, err := archiveMutationConfig(ctx, cfg)
	if err != nil {
		return autonomousarchive.ArchiveResult{}, err
	}
	defer closeStore()
	return autonomousarchive.Archive(ctx, autonomousarchive.Config{RepositoryRoot: paths.WorkDir, Ledger: store, GitExecutable: runCfg.GitExecutable, GitTimeout: runCfg.GitTimeout, CommandRunner: input.CommandRunner, FailureInjector: input.FailureInjector, ForbiddenValues: archiveSecretValues(runCfg)}, autonomousarchive.ArchiveRequest{TaskID: input.TaskID, OperationID: input.OperationID, ArchiveRunID: input.ArchiveRunID, Authority: autonomousarchive.TerminalAuthority{SchemaVersion: autonomousarchive.AuthoritySchemaVersion, Disposition: input.Disposition, Reason: input.Reason, Provenance: input.Provenance, TerminalAt: input.TerminalAt}, ArchivedAt: input.ArchivedAt})
}

func archiveSecretValues(cfg runonce.Config) []string {
	values := []string{}
	for _, name := range cfg.SafetyDeclaration.Redaction.EnvironmentVariables {
		if value, ok := os.LookupEnv(name); ok && value != "" {
			values = append(values, value)
		}
	}
	return values
}

func ReopenArchive(ctx context.Context, cfg Config, input ReopenArchiveInput) (autonomousarchive.ReopenResult, error) {
	paths, runCfg, store, closeStore, err := archiveMutationConfig(ctx, cfg)
	if err != nil {
		return autonomousarchive.ReopenResult{}, err
	}
	defer closeStore()
	return autonomousarchive.Reopen(ctx, autonomousarchive.Config{RepositoryRoot: paths.WorkDir, Ledger: store, GitExecutable: runCfg.GitExecutable, GitTimeout: runCfg.GitTimeout, CommandRunner: input.CommandRunner}, autonomousarchive.ReopenRequest{Selector: input.Selector, OperationID: input.OperationID, NewTaskID: input.NewTaskID, Authority: input.Authority, Reason: input.Reason, ReopenedAt: input.ReopenedAt})
}

func archiveMutationConfig(ctx context.Context, cfg Config) (statePaths, runonce.Config, *ledger.Store, func(), error) {
	paths, err := resolveStatePaths(cfg.WorkDir)
	if err != nil {
		return statePaths{}, runonce.Config{}, nil, nil, err
	}
	initialized, err := ledgerInitialized(paths)
	if err != nil || !initialized {
		return statePaths{}, runonce.Config{}, nil, nil, errors.Join(err, errors.New("state is not initialized; run `revolvr init` first"))
	}
	runCfg, err := LoadRunOnceConfig(paths.WorkDir, DefaultRunOnceConfig(paths.WorkDir))
	if err != nil {
		return statePaths{}, runonce.Config{}, nil, nil, err
	}
	store, err := ledger.Open(ctx, paths.LedgerDBPath)
	if err != nil {
		return statePaths{}, runonce.Config{}, nil, nil, err
	}
	return paths, runCfg, store, func() { _ = store.Close() }, nil
}
