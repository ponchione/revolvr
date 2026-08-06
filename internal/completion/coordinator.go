package completion

import (
	"context"
	"fmt"
	"time"

	"revolvr/internal/evidence"
	"revolvr/internal/id"
)

type Coordinator struct {
	Reader          AuthorityReader
	Terminal        TerminalStore
	Artifacts       *evidence.Store
	Clock           func() time.Time
	NewID           func() string
	SecretSentinels []string
	Failure         FailureInjector
}

func (c Coordinator) Complete(ctx context.Context, key Key) (Result, error) {
	if c.Reader == nil || c.Terminal == nil || c.Artifacts == nil || key.OperationID == "" {
		return Result{}, errorsNew("coordinator dependencies or operation identity are missing")
	}
	if lookup, ok := c.Terminal.(TerminalLookup); ok {
		terminal, found, err := lookup.LookupCompletion(ctx, key)
		if err != nil {
			return Result{}, err
		}
		if found {
			return Result{Terminal: terminal}, nil
		}
	}
	clock := c.Clock
	if clock == nil {
		clock = time.Now
	}
	newID := c.NewID
	if newID == nil {
		newID = id.New
	}
	snapshot, err := c.Reader.ReadCompletionSnapshot(ctx, key)
	if err != nil {
		return Result{}, err
	}
	if snapshot.Identity != key.Identity {
		return Result{}, fmt.Errorf("%w: requested identity changed", ErrStalePreflight)
	}
	preflight, err := BuildPreflight(snapshot)
	if err != nil {
		return Result{}, err
	}
	result := Result{Preflight: preflight}
	if !preflight.Accepted() {
		return result, ErrRejected
	}
	provenance := evidence.Provenance{
		SchemaVersion: evidence.ArtifactProvenanceSchemaVersion,
		ProjectID:     snapshot.Identity.ProjectID, TaskID: snapshot.Identity.TaskID,
		TaskVersionID: snapshot.Identity.TaskVersionID, RunID: snapshot.Identity.RunID,
		WorkspaceID: snapshot.Identity.WorkspaceID, ProducerRole: "host",
		ProducingOperationID: key.OperationID, SourceCommit: snapshot.Source.AfterCommit,
		SourceTree: snapshot.Source.AfterTree,
	}
	materialized, err := MaterializeCapsule(ctx, c.Artifacts, preflight, provenance, c.SecretSentinels, c.Failure)
	if err != nil {
		return result, err
	}
	result.Materialized = materialized
	latest, err := c.Reader.ReadCompletionSnapshot(ctx, key)
	if err != nil {
		return result, err
	}
	revalidated, err := BuildPreflight(latest)
	if err != nil {
		return result, err
	}
	if !revalidated.Accepted() || revalidated.SHA256 != preflight.SHA256 {
		return result, ErrStalePreflight
	}
	if err := inject(c.Failure, FailureBeforeTerminal); err != nil {
		return result, err
	}
	terminal, err := c.Terminal.CommitCompletion(ctx, TerminalCommand{
		CompletionID: newID(), OperationID: key.OperationID, Preflight: revalidated,
		Materialized: materialized, CompletedAt: clock().UTC().Truncate(time.Microsecond),
	})
	if err != nil {
		return result, err
	}
	result.Terminal = terminal
	return result, nil
}
