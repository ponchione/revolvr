package autonomousscheduler

import (
	"context"

	"revolvr/internal/taskschedule"
)

type ActiveTask = taskschedule.ActiveTask

// LoadActive is retained as an autonomous admission adapter while canonical
// repository/state loading is shared with mixed and read-side scheduling.
func LoadActive(ctx context.Context, repositoryRoot string) ([]ActiveTask, error) {
	return taskschedule.LoadActive(ctx, repositoryRoot)
}

func LoadActiveStrict(ctx context.Context, repositoryRoot string) ([]ActiveTask, error) {
	return taskschedule.LoadActiveStrict(ctx, repositoryRoot)
}
