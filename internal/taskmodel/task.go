package taskmodel

import "time"

const (
	StatusPending   = "pending"
	StatusCompleted = "completed"
	StatusBlocked   = "blocked"
)

type Task struct {
	ID          string
	Task        string
	Status      string
	Summary     string
	Blocker     string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	CompletedAt *time.Time
	BlockedAt   *time.Time
}
