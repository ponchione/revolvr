package taskmodel

import (
	"time"

	"revolvr/internal/taskscheduler"
)

const (
	StatusPending   = "pending"
	StatusCompleted = "completed"
	StatusBlocked   = "blocked"
)

type Task struct {
	ID                    string
	Task                  string
	Status                string
	Summary               string
	Workflow              string
	Phase                 string
	RunProfile            string
	NextState             string
	NextRunnable          bool
	AutonomousReady       bool
	NextAutonomous        bool
	SelectedNext          bool
	Readiness             taskscheduler.Reason
	ReadinessReason       string
	WaitingDependencyIDs  []string
	DependencyIssues      []taskscheduler.DependencyIssue
	ConflictBlockers      []string
	SchedulingDiagnostics []taskscheduler.Diagnostic
	DependsOn             []string
	Tags                  []string
	Conflicts             []string
	ParentTaskID          string
	CheckpointState       string
	CheckpointReceiptPath string
	CheckpointReceiptSHA  string
	Blocker               string
	CreatedAt             time.Time
	UpdatedAt             time.Time
	CompletedAt           *time.Time
	BlockedAt             *time.Time
}

const (
	CheckpointStateAwaiting  = "awaiting"
	CheckpointStateFulfilled = "fulfilled"
	CheckpointStateInvalid   = "invalid"
)
