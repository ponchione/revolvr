package taskmodel

import "time"

const (
	StatusPending   = "pending"
	StatusCompleted = "completed"
	StatusBlocked   = "blocked"
)

type Task struct {
	ID              string
	Task            string
	Status          string
	Summary         string
	Workflow        string
	Phase           string
	RunProfile      string
	NextState       string
	NextRunnable    bool
	AutonomousReady bool
	NextAutonomous  bool
	ReadinessReason string
	DependsOn       []string
	Tags            []string
	Conflicts       []string
	ParentTaskID    string
	Blocker         string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	CompletedAt     *time.Time
	BlockedAt       *time.Time
}
