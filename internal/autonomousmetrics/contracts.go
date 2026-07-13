// Package autonomousmetrics projects deterministic autonomous-loop metrics
// from validated logical ledger evidence. It performs no I/O or workflow work.
package autonomousmetrics

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

const SchemaVersion = "autonomous-loop-metrics-v1"

type Source struct {
	Kind       string `json:"kind"`
	Reference  string `json:"reference"`
	RunCount   int    `json:"run_count"`
	EventCount int    `json:"event_count"`
	MaxEventID int64  `json:"max_event_id"`
}

type Reference struct {
	RunID      string `json:"run_id"`
	EventID    int64  `json:"event_id"`
	TaskID     string `json:"task_id,omitempty"`
	Operation  string `json:"operation_id,omitempty"`
	Occurrence string `json:"occurrence_id,omitempty"`
}

type Count struct {
	Name  string `json:"name"`
	Value int64  `json:"value"`
}

type OutcomeFact struct {
	TaskID      string    `json:"task_id"`
	OperationID string    `json:"operation_id"`
	Reason      string    `json:"reason"`
	CompletedAt time.Time `json:"completed_at"`
	Reference   Reference `json:"reference"`
}

type TaskOutcomes struct {
	Total              int64         `json:"total"`
	Counts             []Count       `json:"counts"`
	SuccessNumerator   int64         `json:"success_numerator"`
	SuccessDenominator int64         `json:"success_denominator"`
	Facts              []OutcomeFact `json:"facts"`
}

type AttemptFact struct {
	TaskID              string    `json:"task_id"`
	AttemptID           string    `json:"attempt_id"`
	Sequence            int64     `json:"sequence"`
	Kind                string    `json:"kind"`
	Action              string    `json:"action"`
	RunID               string    `json:"run_id,omitempty"`
	OccurrenceID        string    `json:"occurrence_id,omitempty"`
	Outcome             string    `json:"outcome,omitempty"`
	DurationNanoseconds int64     `json:"duration_nanoseconds,omitempty"`
	Tokens              *int64    `json:"tokens,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	Reference           Reference `json:"reference"`
}

type CorrectionFact struct {
	TaskID      string    `json:"task_id"`
	OperationID string    `json:"operation_id"`
	Cycle       int64     `json:"cycle"`
	Kind        string    `json:"kind"`
	RunID       string    `json:"run_id,omitempty"`
	Reference   Reference `json:"reference"`
}

type Attempts struct {
	Admitted         int64            `json:"admitted"`
	Completed        int64            `json:"completed"`
	CorrectionCycles int64            `json:"correction_cycles"`
	Corrections      []Count          `json:"correction_kinds"`
	Facts            []AttemptFact    `json:"facts"`
	CorrectionFacts  []CorrectionFact `json:"correction_facts"`
}

type AuditFact struct {
	TaskID              string    `json:"task_id"`
	AuditRunID          string    `json:"audit_run_id"`
	Disposition         string    `json:"disposition"`
	BlockingFindings    int64     `json:"blocking_findings"`
	NonblockingFindings int64     `json:"nonblocking_findings"`
	Reference           Reference `json:"reference"`
}

type FindingFact struct {
	TaskID                 string    `json:"task_id"`
	FindingID              string    `json:"finding_id"`
	Significance           string    `json:"significance"`
	IntroducedByAuditRunID string    `json:"introduced_by_audit_run_id"`
	Disposition            string    `json:"disposition"`
	Reference              Reference `json:"reference"`
}

type Audits struct {
	Performed           int64         `json:"performed"`
	Clean               int64         `json:"clean"`
	ChangesRequired     int64         `json:"changes_required"`
	BlockingFindings    int64         `json:"blocking_findings"`
	NonblockingFindings int64         `json:"nonblocking_findings"`
	Resolutions         []Count       `json:"resolution_dispositions"`
	Facts               []AuditFact   `json:"facts"`
	Findings            []FindingFact `json:"findings"`
}

type VerificationFact struct {
	TaskID              string    `json:"task_id"`
	RunID               string    `json:"run_id"`
	OccurrenceID        string    `json:"occurrence_id"`
	Purpose             string    `json:"purpose"`
	Outcome             string    `json:"outcome"`
	TierAttempts        int64     `json:"tier_attempts"`
	CommandAttempts     int64     `json:"command_attempts"`
	Reruns              int64     `json:"reruns"`
	Flaky               int64     `json:"flaky"`
	DurationNanoseconds int64     `json:"duration_nanoseconds"`
	Reference           Reference `json:"reference"`
}

type Verification struct {
	Occurrences          int64              `json:"occurrences"`
	TierAttempts         int64              `json:"tier_attempts"`
	CommandAttempts      int64              `json:"command_attempts"`
	OrdinaryPasses       int64              `json:"ordinary_passes"`
	OrdinaryFailures     int64              `json:"ordinary_failures"`
	FlakyClassifications int64              `json:"flaky_classifications"`
	Reruns               int64              `json:"reruns"`
	Timeouts             int64              `json:"timeouts"`
	Cancellations        int64              `json:"cancellations"`
	MissingCommands      int64              `json:"missing_commands"`
	RunnerErrors         int64              `json:"runner_errors"`
	Facts                []VerificationFact `json:"facts"`
}

type StopFact struct {
	TaskID      string    `json:"task_id"`
	OperationID string    `json:"operation_id"`
	Class       string    `json:"class"`
	Reason      string    `json:"reason"`
	Reference   Reference `json:"reference"`
}

type Usage struct {
	RecordedTokens                  int64 `json:"recorded_tokens"`
	AttemptsWithTokens              int64 `json:"attempts_with_tokens"`
	AttemptsMissingTokens           int64 `json:"attempts_missing_tokens"`
	AttemptDurationNanoseconds      int64 `json:"attempt_duration_nanoseconds"`
	RecordedRunDurationNanoseconds  int64 `json:"recorded_run_duration_nanoseconds"`
	TaskDurationNanoseconds         int64 `json:"task_duration_nanoseconds"`
	VerificationDurationNanoseconds int64 `json:"verification_duration_nanoseconds"`
	QueueDurationNanoseconds        int64 `json:"queue_duration_nanoseconds"`
}

type ArchiveFact struct {
	TaskID             string    `json:"task_id"`
	ArchiveID          string    `json:"archive_id"`
	Disposition        string    `json:"disposition"`
	TerminalAt         time.Time `json:"terminal_at"`
	ArchivedAt         time.Time `json:"archived_at"`
	LatencyNanoseconds int64     `json:"latency_nanoseconds"`
	Reference          Reference `json:"reference"`
}

type CompletionFact struct {
	TaskID      string    `json:"task_id"`
	OperationID string    `json:"operation_id"`
	TerminalAt  time.Time `json:"terminal_at"`
	Reference   Reference `json:"reference"`
}

type Archives struct {
	Completed           int64            `json:"completed"`
	Cancelled           int64            `json:"cancelled"`
	Superseded          int64            `json:"superseded"`
	Abandoned           int64            `json:"abandoned"`
	LatencyCount        int64            `json:"latency_count"`
	LatencyNanoseconds  int64            `json:"latency_nanoseconds"`
	TerminalCompletions []CompletionFact `json:"terminal_completions"`
	Facts               []ArchiveFact    `json:"facts"`
}

type QueueFact struct {
	OperationID                      string    `json:"operation_id"`
	Mode                             string    `json:"mode"`
	Sweep                            int64     `json:"sweep"`
	Selections                       int64     `json:"selections"`
	TasksRun                         int64     `json:"tasks_run"`
	MaximumWorkers                   int       `json:"maximum_workers"`
	PeakActiveWorkers                int       `json:"peak_active_workers"`
	Batches                          int64     `json:"batches"`
	SequentialFallbacks              int64     `json:"sequential_fallbacks"`
	Drained                          int64     `json:"drained"`
	StopReason                       string    `json:"stop_reason"`
	DurationNanoseconds              int64     `json:"duration_nanoseconds"`
	ThroughputNumerator              int64     `json:"throughput_numerator"`
	ThroughputDenominatorNanoseconds int64     `json:"throughput_denominator_nanoseconds"`
	Reference                        Reference `json:"reference"`
}

type Queues struct {
	Sweeps                   int64       `json:"sweeps"`
	Selections               int64       `json:"selections"`
	TasksRun                 int64       `json:"tasks_run"`
	Drained                  int64       `json:"drained"`
	MaximumConfiguredWorkers int         `json:"maximum_configured_workers"`
	PeakActiveWorkers        int         `json:"peak_active_workers"`
	ParallelSweeps           int64       `json:"parallel_sweeps"`
	SequentialFallbacks      int64       `json:"sequential_fallbacks"`
	StopReasons              []Count     `json:"stop_reasons"`
	Facts                    []QueueFact `json:"facts"`
}

type Omission struct {
	Code      string     `json:"code"`
	Detail    string     `json:"detail"`
	Reference *Reference `json:"reference,omitempty"`
}

type Projection struct {
	SchemaVersion string       `json:"schema_version"`
	Source        Source       `json:"source"`
	TaskOutcomes  TaskOutcomes `json:"task_outcomes"`
	Attempts      Attempts     `json:"attempts"`
	Audits        Audits       `json:"audits"`
	Verification  Verification `json:"verification"`
	Stops         []StopFact   `json:"stops"`
	Usage         Usage        `json:"usage"`
	Archives      Archives     `json:"archives"`
	Queues        Queues       `json:"queues"`
	Omissions     []Omission   `json:"omissions"`
}

func (p Projection) Validate() error {
	if p.SchemaVersion != SchemaVersion {
		return fmt.Errorf("metrics: unknown schema %q", p.SchemaVersion)
	}
	if p.Source.Kind != "logical_ledger" && p.Source.Kind != "fixture" {
		return fmt.Errorf("metrics: unknown source kind %q", p.Source.Kind)
	}
	if p.Source.Reference == "" || p.Source.RunCount < 0 || p.Source.EventCount < 0 || p.Source.MaxEventID < 0 {
		return errors.New("metrics: malformed source")
	}
	if p.TaskOutcomes.Total != p.TaskOutcomes.SuccessDenominator || p.TaskOutcomes.SuccessNumerator < 0 || p.TaskOutcomes.SuccessNumerator > p.TaskOutcomes.SuccessDenominator {
		return errors.New("metrics: invalid success denominator")
	}
	return nil
}

func Marshal(p Projection) ([]byte, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func Decode(raw []byte) (Projection, error) {
	var p Projection
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&p); err != nil {
		return p, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return p, errors.New("metrics: expected exactly one JSON value")
	}
	if err := p.Validate(); err != nil {
		return p, err
	}
	canonical, err := Marshal(p)
	if err != nil {
		return p, err
	}
	if !bytes.Equal(raw, canonical) {
		return p, errors.New("metrics: non-canonical JSON")
	}
	return p, nil
}
