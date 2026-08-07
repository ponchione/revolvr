package app

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"revolvr/internal/queue"
	storage "revolvr/internal/storage/postgres"
)

var ErrSequentialQueueQualityGate = errors.New("sequential queue: Section 23.3 real-project quality gate has no approved measured threshold")

type SequentialQueueStartInput struct {
	DatabaseURL         string
	OperationID         string
	CoordinatorIdentity string
	Limits              queue.Limits
	QualityGateStatus   queue.QualityGateStatus
	Executor            queue.TaskExecutor
	Clock               func() time.Time
	FailureInjector     queue.FailureInjector
	CancellationPoll    time.Duration
}

func StartSequentialQueue(ctx context.Context, _ Config, input SequentialQueueStartInput) (queue.Result, error) {
	// Architecture 022 measured a deterministic baseline but deliberately set
	// no Section 23.3 real-project threshold. The production CLI therefore may
	// not manufacture approval or silently substitute the legacy queue.
	if input.Executor == nil {
		return queue.Result{}, ErrSequentialQueueQualityGate
	}
	if input.QualityGateStatus == "" {
		input.QualityGateStatus = queue.QualityGateDeterministicOnly
	}
	pool, err := openSequentialQueueDatabase(ctx, input.DatabaseURL)
	if err != nil {
		return queue.Result{}, err
	}
	defer pool.Close()
	return queue.Run(ctx, pool, queue.Config{
		OperationID: input.OperationID, CoordinatorIdentity: input.CoordinatorIdentity,
		Limits: input.Limits, QualityGateStatus: input.QualityGateStatus,
		Clock: input.Clock, Executor: input.Executor, FailureInjector: input.FailureInjector,
		CancellationPoll: input.CancellationPoll,
	})
}

func SequentialQueueStatus(ctx context.Context, _ Config, databaseURL, operationID string) (queue.Status, error) {
	pool, err := openSequentialQueueDatabase(ctx, databaseURL)
	if err != nil {
		return queue.Status{}, err
	}
	defer pool.Close()
	return queue.StatusOperation(ctx, pool, operationID)
}

func CancelSequentialQueue(ctx context.Context, _ Config, databaseURL, operationID string) (queue.Status, bool, error) {
	pool, err := openSequentialQueueDatabase(ctx, databaseURL)
	if err != nil {
		return queue.Status{}, false, err
	}
	defer pool.Close()
	return queue.Cancel(ctx, pool, operationID, nil)
}

func openSequentialQueueDatabase(ctx context.Context, configured string) (*pgxpool.Pool, error) {
	databaseURL := strings.TrimSpace(configured)
	if databaseURL == "" {
		databaseURL = strings.TrimSpace(os.Getenv("REVOLVR_DATABASE_URL"))
	}
	if databaseURL == "" {
		return nil, errors.New("sequential queue: REVOLVR_DATABASE_URL is required")
	}
	return storage.Open(ctx, databaseURL)
}
