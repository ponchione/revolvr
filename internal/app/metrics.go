package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"revolvr/internal/autonomousmetrics"
	"revolvr/internal/ledger"
	"revolvr/internal/ledgerexport"
)

// ShowMetrics loads either one coherent live-ledger snapshot or one verified
// immutable export, then invokes the same pure logical projection.
func ShowMetrics(ctx context.Context, cfg Config, exportID string) (autonomousmetrics.Projection, error) {
	paths, err := resolveStatePaths(cfg.WorkDir)
	if err != nil {
		return autonomousmetrics.Projection{}, err
	}
	var snapshot ledger.Snapshot
	runCfg, err := LoadRunOnceConfig(paths.WorkDir, DefaultRunOnceConfig(paths.WorkDir))
	if err != nil {
		return autonomousmetrics.Projection{}, err
	}
	secrets := archiveSecretValues(runCfg)
	if exportID != "" {
		snapshot, err = ledgerexport.ReplaySnapshot(ctx, paths.WorkDir, exportID, secrets)
	} else {
		store, openErr := openReadOnlyLedger(ctx, paths)
		if openErr != nil {
			return autonomousmetrics.Projection{}, openErr
		}
		snapshot, err = store.ReadSnapshot(ctx)
		closeErr := store.Close()
		if err == nil {
			err = closeErr
		}
	}
	if err != nil {
		return autonomousmetrics.Projection{}, fmt.Errorf("metrics: load logical evidence: %w", err)
	}
	projection, err := autonomousmetrics.Project(snapshot, autonomousmetrics.LogicalSource(snapshot))
	if err != nil {
		return autonomousmetrics.Projection{}, err
	}
	raw, err := autonomousmetrics.Marshal(projection)
	if err != nil {
		return autonomousmetrics.Projection{}, err
	}
	for _, secret := range secrets {
		if secret != "" && bytes.Contains(raw, []byte(secret)) {
			return autonomousmetrics.Projection{}, errors.New("metrics: configured secret detected in projection")
		}
	}
	return projection, nil
}
