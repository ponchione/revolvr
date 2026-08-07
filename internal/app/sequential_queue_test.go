package app

import (
	"context"
	"errors"
	"testing"

	"revolvr/internal/queue"
)

func TestSequentialQueueRealProjectStartFailsClosedWithoutMeasuredGate(t *testing.T) {
	result, err := StartSequentialQueue(context.Background(), Config{}, SequentialQueueStartInput{
		OperationID: "019c3a64-6c00-7000-8000-000000000023",
		Limits: queue.Limits{
			MaximumTasks: 1, MaximumCyclesPerTask: 1, MaximumTotalCycles: 1,
			MaximumRemoteTokens: 1, MaximumCostMicrousd: 1,
		},
	})
	if !errors.Is(err, ErrSequentialQueueQualityGate) || result.OperationID != "" {
		t.Fatalf("result = %#v, error = %v", result, err)
	}
}
