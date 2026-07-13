package autonomousverification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"revolvr/internal/ledger"
	"revolvr/internal/pathguard"
	"revolvr/internal/runner"
	"revolvr/internal/verification"
)

type Outcome string

const (
	OutcomePassed             Outcome = "passed"
	OutcomeFailed             Outcome = "failed"
	OutcomeFlaky              Outcome = "flaky"
	OutcomeMissing            Outcome = "missing"
	OutcomeTimedOut           Outcome = "timed_out"
	OutcomeCancelled          Outcome = "cancelled"
	OutcomeRunnerError        Outcome = "runner_error"
	OutcomeConfigurationError Outcome = "configuration_error"
	OutcomeLedgerError        Outcome = "ledger_error"
	OutcomeArtifactError      Outcome = "artifact_error"
)

type RerunDecision string

const (
	RerunNotNeeded  RerunDecision = "not_needed"
	RerunDisabled   RerunDecision = "disabled"
	RerunAuthorized RerunDecision = "authorized"
	RerunIneligible RerunDecision = "ineligible"
	RerunConsumed   RerunDecision = "consumed"
)

type CommandIdentity struct {
	SHA256     string        `json:"sha256"`
	PlanSHA256 string        `json:"plan_sha256"`
	Purpose    Purpose       `json:"purpose"`
	TierID     string        `json:"tier_id"`
	TierKind   TierKind      `json:"tier_kind"`
	Position   int           `json:"position"`
	Name       string        `json:"name"`
	Args       []string      `json:"args"`
	Dir        string        `json:"dir"`
	Env        []string      `json:"env"`
	Timeout    time.Duration `json:"timeout"`
	StdoutCap  int           `json:"stdout_cap"`
	StderrCap  int           `json:"stderr_cap"`
}

type Output struct {
	Content        string `json:"content"`
	TruncatedBytes int64  `json:"truncated_bytes"`
}

type Attempt struct {
	AttemptID string          `json:"attempt_id"`
	Number    int             `json:"number"`
	Command   CommandIdentity `json:"command"`
	Outcome   Outcome         `json:"outcome"`
	Passed    bool            `json:"passed"`
	ExitCode  int             `json:"exit_code"`
	TimedOut  bool            `json:"timed_out"`
	Cancelled bool            `json:"cancelled"`
	Error     string          `json:"error,omitempty"`
	Stdout    Output          `json:"stdout"`
	Stderr    Output          `json:"stderr"`
	StartedAt time.Time       `json:"started_at"`
	EndedAt   time.Time       `json:"ended_at"`
	Duration  time.Duration   `json:"duration"`
}

type CommandResult struct {
	Identity      CommandIdentity `json:"identity"`
	Outcome       Outcome         `json:"outcome"`
	Attempts      []Attempt       `json:"attempts"`
	RerunDecision RerunDecision   `json:"rerun_decision"`
	RerunReason   string          `json:"rerun_reason"`
}

type TierResult struct {
	ID               string          `json:"id"`
	Kind             TierKind        `json:"kind"`
	RequiredForFinal bool            `json:"required_for_final"`
	Outcome          Outcome         `json:"outcome"`
	Commands         []CommandResult `json:"commands"`
	StartedAt        time.Time       `json:"started_at"`
	EndedAt          time.Time       `json:"ended_at"`
	Duration         time.Duration   `json:"duration"`
}

type GateEvidence struct {
	SchemaVersion      string       `json:"schema_version"`
	Plan               PlanIdentity `json:"plan"`
	Purpose            Purpose      `json:"purpose"`
	RequiredFinalTiers []string     `json:"required_final_tiers"`
	SelectedTiers      []string     `json:"selected_tiers"`
	ExecutedTiers      []string     `json:"executed_tiers"`
	RequiredOutcomes   []TierGate   `json:"required_outcomes"`
	MissingRequired    []string     `json:"missing_required"`
	OverallOutcome     Outcome      `json:"overall_outcome"`
	FinalSatisfied     bool         `json:"final_satisfied"`
}

type TierGate struct {
	TierID  string  `json:"tier_id"`
	Outcome Outcome `json:"outcome"`
}

type Artifact struct {
	Path     string `json:"path"`
	SHA256   string `json:"sha256"`
	ByteSize int    `json:"byte_size"`
}

type Result struct {
	SchemaVersion  string              `json:"schema_version"`
	TaskID         string              `json:"task_id"`
	RunID          string              `json:"run_id"`
	OccurrenceID   string              `json:"occurrence_id"`
	SourceRevision string              `json:"source_revision"`
	Plan           PlanIdentity        `json:"plan"`
	Purpose        Purpose             `json:"purpose"`
	Outcome        Outcome             `json:"outcome"`
	Gate           GateEvidence        `json:"gate"`
	Tiers          []TierResult        `json:"tiers"`
	StartedAt      time.Time           `json:"started_at"`
	EndedAt        time.Time           `json:"ended_at"`
	Duration       time.Duration       `json:"duration"`
	FailureStage   string              `json:"failure_stage,omitempty"`
	FailureReason  string              `json:"failure_reason,omitempty"`
	Aggregate      verification.Result `json:"aggregate"`
	Artifact       *Artifact           `json:"-"`
}

const LedgerEventSchemaVersion = "autonomous-verification-ledger-event-v1"

// CompletedLedgerEvent is the strict, self-contained verification occurrence
// recorded in the ledger and replayed by metrics.
type CompletedLedgerEvent struct {
	SchemaVersion  string                       `json:"schema_version"`
	Status         verification.Status          `json:"status"`
	Passed         bool                         `json:"passed"`
	Message        string                       `json:"message"`
	Commands       []verification.CommandResult `json:"commands"`
	TaskID         string                       `json:"task_id"`
	OccurrenceID   string                       `json:"occurrence_id"`
	SourceRevision string                       `json:"source_revision"`
	Plan           PlanIdentity                 `json:"plan"`
	Purpose        Purpose                      `json:"purpose"`
	Outcome        Outcome                      `json:"outcome"`
	Gate           GateEvidence                 `json:"gate"`
	Tiers          []TierResult                 `json:"tiers"`
	Artifact       *Artifact                    `json:"artifact,omitempty"`
}

func DecodeCompletedLedgerEvent(raw []byte) (CompletedLedgerEvent, error) {
	var event CompletedLedgerEvent
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return event, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return event, errors.New("verification ledger event contains trailing JSON")
	}
	if event.SchemaVersion != LedgerEventSchemaVersion {
		return event, fmt.Errorf("unknown verification ledger schema %q", event.SchemaVersion)
	}
	result := Result{SchemaVersion: ResultSchemaVersion, TaskID: event.TaskID, RunID: "ledger-event", OccurrenceID: event.OccurrenceID, SourceRevision: event.SourceRevision, Plan: event.Plan, Purpose: event.Purpose, Outcome: event.Outcome, Gate: event.Gate, Tiers: event.Tiers}
	if err := result.Validate(); err != nil {
		return event, fmt.Errorf("verification ledger event: %w", err)
	}
	return event, nil
}

type Ledger interface {
	AppendEvent(context.Context, string, ledger.EventType, any) (ledger.Event, error)
}

type CommandRunner func(context.Context, runner.Command) runner.Result
type ArtifactWriter func(repositoryRoot, relativePath string, content []byte) error

type Config struct {
	RepositoryRoot string
	ArtifactRoot   string
	TaskID         string
	RunID          string
	OccurrenceID   string
	SourceRevision string
	Plan           Plan
	Purpose        Purpose
	Timeout        time.Duration
	StdoutCap      int
	StderrCap      int
	Clock          func() time.Time
	AttemptID      func() string
	CommandRunner  CommandRunner
	Ledger         Ledger
	ArtifactPath   string
	ArtifactWriter ArtifactWriter
}

func Execute(ctx context.Context, cfg Config) (Result, error) {
	n, selection, identity, err := normalizeExecution(cfg)
	if err != nil {
		return Result{SchemaVersion: ResultSchemaVersion, TaskID: cfg.TaskID, RunID: cfg.RunID, OccurrenceID: cfg.OccurrenceID, SourceRevision: cfg.SourceRevision, Purpose: cfg.Purpose, Outcome: OutcomeConfigurationError, FailureStage: "configuration", FailureReason: err.Error()}, err
	}
	result := Result{SchemaVersion: ResultSchemaVersion, TaskID: n.TaskID, RunID: n.RunID, OccurrenceID: n.OccurrenceID, SourceRevision: n.SourceRevision, Plan: identity, Purpose: n.Purpose, Outcome: OutcomePassed, StartedAt: n.Clock().UTC()}
	result.Aggregate = verification.Result{Status: verification.StatusPassed, Passed: true, FailedCommandIndex: -1, StartedAt: result.StartedAt}
	result.Gate = GateEvidence{SchemaVersion: GateSchemaVersion, Plan: identity, Purpose: n.Purpose, RequiredFinalTiers: append([]string(nil), selection.RequiredFinalTiers...), SelectedTiers: append([]string(nil), selection.SelectedTierIDs...)}
	if err := appendEvent(ctx, n, ledger.EventVerificationStarted, startedEvent(result, selection)); err != nil {
		return failOperation(n, result, OutcomeLedgerError, "ledger_start", err)
	}
	runIndex := 0
	rerunConsumed := false
	for _, tier := range selection.SelectedTiers {
		if ctx.Err() != nil {
			return finishOperation(ctx, n, result, OutcomeCancelled, "cancellation", ctx.Err())
		}
		tierResult := TierResult{ID: tier.ID, Kind: tier.Kind, RequiredForFinal: tier.RequiredForFinal, Outcome: OutcomePassed, StartedAt: n.Clock().UTC()}
		result.Gate.ExecutedTiers = append(result.Gate.ExecutedTiers, tier.ID)
		if err := appendEvent(ctx, n, ledger.EventVerificationTierStarted, struct {
			SchemaVersion string     `json:"schema_version"`
			Tier          TierResult `json:"tier"`
		}{LedgerEventSchemaVersion, tierResult}); err != nil {
			result.Tiers = append(result.Tiers, tierResult)
			return failOperation(n, result, OutcomeLedgerError, "ledger_tier_start", err)
		}
		if len(tier.Commands) == 0 {
			tierResult.Outcome = OutcomeMissing
			tierResult.EndedAt = n.Clock().UTC()
			tierResult.Duration = duration(tierResult.StartedAt, tierResult.EndedAt)
			result.Tiers = append(result.Tiers, tierResult)
			if err := appendEvent(ctx, n, ledger.EventVerificationTierCompleted, struct {
				SchemaVersion string     `json:"schema_version"`
				Tier          TierResult `json:"tier"`
			}{LedgerEventSchemaVersion, tierResult}); err != nil {
				return failOperation(n, result, OutcomeLedgerError, "ledger_tier_complete", err)
			}
			return finishOperation(ctx, n, result, OutcomeMissing, "missing_commands", fmt.Errorf("selected tier %q has no commands", tier.ID))
		}
		for position, command := range tier.Commands {
			identity, identityErr := commandIdentity(n, result.Plan, tier, position, command)
			if identityErr != nil {
				return finishOperation(ctx, n, result, OutcomeConfigurationError, "command_identity", identityErr)
			}
			commandResult := CommandResult{Identity: identity, RerunDecision: RerunNotNeeded, RerunReason: "first attempt has not failed"}
			first := executeAttempt(ctx, n, identity, 1)
			commandResult.Attempts = append(commandResult.Attempts, first)
			appendAggregate(&result.Aggregate, runIndex, first)
			runIndex++
			commandResult.Outcome = first.Outcome
			if first.Outcome == OutcomeFailed {
				switch {
				case tier.RerunPolicy != RerunOnceToClassifyFlaky:
					commandResult.RerunDecision, commandResult.RerunReason = RerunDisabled, "tier policy does not authorize flaky classification"
				case rerunConsumed:
					commandResult.RerunDecision, commandResult.RerunReason = RerunConsumed, "operation rerun limit is already consumed"
				default:
					rerunConsumed = true
					commandResult.RerunDecision, commandResult.RerunReason = RerunAuthorized, "first ordinary failure is the deterministic eligible rerun"
					if err := appendEvent(ctx, n, ledger.EventVerificationRerun, rerunEvent(tier, identity)); err != nil {
						return failOperation(n, result, OutcomeLedgerError, "ledger_rerun", err)
					}
					second := executeAttempt(ctx, n, identity, 2)
					commandResult.Attempts = append(commandResult.Attempts, second)
					appendAggregate(&result.Aggregate, runIndex, second)
					runIndex++
					if second.Outcome == OutcomePassed {
						commandResult.Outcome = OutcomeFlaky
					} else {
						commandResult.Outcome = second.Outcome
					}
				}
			} else if first.Outcome != OutcomePassed {
				commandResult.RerunDecision, commandResult.RerunReason = RerunIneligible, "timeouts, cancellation, and runner errors are not rerun eligible"
			}
			tierResult.Commands = append(tierResult.Commands, commandResult)
			if commandResult.Outcome != OutcomePassed {
				tierResult.Outcome = commandResult.Outcome
				tierResult.EndedAt = n.Clock().UTC()
				tierResult.Duration = duration(tierResult.StartedAt, tierResult.EndedAt)
				result.Tiers = append(result.Tiers, tierResult)
				if err := appendEvent(ctx, n, ledger.EventVerificationTierCompleted, struct {
					SchemaVersion string     `json:"schema_version"`
					Tier          TierResult `json:"tier"`
				}{LedgerEventSchemaVersion, tierResult}); err != nil {
					return failOperation(n, result, OutcomeLedgerError, "ledger_tier_complete", err)
				}
				return finishOperation(ctx, n, result, commandResult.Outcome, "command", fmt.Errorf("tier %q command %d classified %s", tier.ID, position, commandResult.Outcome))
			}
		}
		tierResult.EndedAt = n.Clock().UTC()
		tierResult.Duration = duration(tierResult.StartedAt, tierResult.EndedAt)
		result.Tiers = append(result.Tiers, tierResult)
		if err := appendEvent(ctx, n, ledger.EventVerificationTierCompleted, struct {
			SchemaVersion string     `json:"schema_version"`
			Tier          TierResult `json:"tier"`
		}{LedgerEventSchemaVersion, tierResult}); err != nil {
			return failOperation(n, result, OutcomeLedgerError, "ledger_tier_complete", err)
		}
	}
	return finishOperation(ctx, n, result, OutcomePassed, "", nil)
}

func MarshalResult(result Result) ([]byte, error) {
	raw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal tiered verification result: %w", err)
	}
	return append(raw, '\n'), nil
}

func DecodeResult(raw []byte) (Result, error) {
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	var result Result
	if err := decoder.Decode(&result); err != nil {
		return Result{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return Result{}, errors.New("multiple JSON values are not allowed")
	} else if err.Error() != "EOF" {
		return Result{}, err
	}
	if err := result.Validate(); err != nil {
		return Result{}, err
	}
	return result, nil
}

func (r Result) Validate() error {
	if r.SchemaVersion != ResultSchemaVersion || strings.TrimSpace(r.TaskID) == "" || strings.TrimSpace(r.RunID) == "" || strings.TrimSpace(r.OccurrenceID) == "" || !validHash(r.SourceRevision) {
		return errors.New("tiered verification result has malformed schema or identity")
	}
	if !validHash(r.Plan.SHA256) || r.Plan.SchemaVersion != PlanSchemaVersion || r.Plan.ByteSize <= 0 {
		return errors.New("tiered verification result has malformed plan identity")
	}
	if err := r.Gate.Validate(); err != nil {
		return err
	}
	if r.Gate.Plan != r.Plan || r.Gate.Purpose != r.Purpose {
		return errors.New("tiered verification result gate identity mismatch")
	}
	if !validOutcome(r.Outcome) || r.Gate.OverallOutcome != r.Outcome {
		return errors.New("tiered verification result has malformed overall outcome")
	}
	if len(r.Tiers) != len(r.Gate.ExecutedTiers) {
		return errors.New("tiered verification result executed tier count mismatch")
	}
	reruns := 0
	for i, tier := range r.Tiers {
		if tier.ID != r.Gate.ExecutedTiers[i] || !validStableID(tier.ID) {
			return errors.New("tiered verification result tier execution order mismatch")
		}
		if !validOutcome(tier.Outcome) {
			return fmt.Errorf("tiered verification result tier %q has invalid outcome", tier.ID)
		}
		for _, command := range tier.Commands {
			if command.Identity.SHA256 != hashCommandIdentity(command.Identity) || command.Identity.TierID != tier.ID || command.Identity.TierKind != tier.Kind || command.Identity.PlanSHA256 != r.Plan.SHA256 || command.Identity.Purpose != r.Purpose {
				return errors.New("tiered verification result command identity mismatch")
			}
			if len(command.Attempts) < 1 || len(command.Attempts) > 2 {
				return errors.New("tiered verification result command must retain one or two attempts")
			}
			if len(command.Attempts) == 2 {
				reruns++
			}
			for j, attempt := range command.Attempts {
				if attempt.Number != j+1 || strings.TrimSpace(attempt.AttemptID) == "" || !reflect.DeepEqual(attempt.Command, command.Identity) || !validOutcome(attempt.Outcome) {
					return errors.New("tiered verification result attempt identity or ordering mismatch")
				}
			}
			if len(command.Attempts) == 2 && command.Attempts[0].Outcome != OutcomeFailed {
				return errors.New("tiered verification rerun did not follow an ordinary failure")
			}
			if command.Outcome == OutcomeFlaky && (len(command.Attempts) != 2 || command.Attempts[1].Outcome != OutcomePassed) {
				return errors.New("tiered verification flaky classification lacks fail-then-pass attempts")
			}
		}
	}
	if reruns > 1 {
		return errors.New("tiered verification result exceeds one-rerun operation limit")
	}
	return nil
}

func (g GateEvidence) Validate() error {
	if g.SchemaVersion != GateSchemaVersion || !validHash(g.Plan.SHA256) || g.Plan.SchemaVersion != PlanSchemaVersion || g.Plan.ByteSize <= 0 {
		return errors.New("verification gate evidence is malformed")
	}
	if g.Purpose != PurposeFast && g.Purpose != PurposeFinal {
		return fmt.Errorf("verification gate purpose %q is invalid", g.Purpose)
	}
	if !validOutcome(g.OverallOutcome) {
		return fmt.Errorf("verification gate overall outcome %q is invalid", g.OverallOutcome)
	}
	if hasDuplicates(g.RequiredFinalTiers) || hasDuplicates(g.SelectedTiers) || hasDuplicates(g.ExecutedTiers) || hasDuplicates(g.MissingRequired) {
		return errors.New("verification gate contains duplicate tier identities")
	}
	for _, values := range [][]string{g.RequiredFinalTiers, g.SelectedTiers, g.ExecutedTiers, g.MissingRequired} {
		for _, id := range values {
			if !validStableID(id) {
				return fmt.Errorf("verification gate tier ID %q is invalid", id)
			}
		}
	}
	selected := stringSet(g.SelectedTiers)
	executed := stringSet(g.ExecutedTiers)
	outcomes := make(map[string]Outcome, len(g.RequiredOutcomes))
	for _, item := range g.RequiredOutcomes {
		if _, ok := outcomes[item.TierID]; ok {
			return errors.New("verification gate contains duplicate required outcomes")
		}
		outcomes[item.TierID] = item.Outcome
		if !validStableID(item.TierID) || !validOutcome(item.Outcome) {
			return errors.New("verification gate required outcome is malformed")
		}
	}
	missing := []string{}
	acceptable := g.Purpose == PurposeFinal && g.OverallOutcome == OutcomePassed
	for _, id := range g.RequiredFinalTiers {
		outcome, ok := outcomes[id]
		if _, selectedOK := selected[id]; !selectedOK || !ok {
			missing = append(missing, id)
			acceptable = false
			continue
		}
		if _, executedOK := executed[id]; !executedOK || outcome != OutcomePassed {
			acceptable = false
		}
	}
	if !equalStrings(missing, g.MissingRequired) || g.FinalSatisfied != acceptable {
		return errors.New("verification gate final status does not match recomputed required-tier evidence")
	}
	return nil
}

func normalizeExecution(cfg Config) (Config, TierSelection, PlanIdentity, error) {
	root, err := filepath.Abs(strings.TrimSpace(cfg.RepositoryRoot))
	if err != nil || strings.TrimSpace(cfg.RepositoryRoot) == "" {
		return Config{}, TierSelection{}, PlanIdentity{}, errors.New("verification execution: repository root is required")
	}
	artifactRoot := root
	if strings.TrimSpace(cfg.ArtifactRoot) != "" {
		artifactRoot, err = filepath.Abs(strings.TrimSpace(cfg.ArtifactRoot))
		if err != nil {
			return Config{}, TierSelection{}, PlanIdentity{}, fmt.Errorf("verification artifact root: %w", err)
		}
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return Config{}, TierSelection{}, PlanIdentity{}, fmt.Errorf("verification execution: resolve repository root: %w", err)
	}
	for _, field := range []struct{ name, value string }{{"task_id", cfg.TaskID}, {"run_id", cfg.RunID}, {"occurrence_id", cfg.OccurrenceID}} {
		if strings.TrimSpace(field.value) == "" || field.value != strings.TrimSpace(field.value) || strings.ContainsAny(field.value, "\r\n") {
			return Config{}, TierSelection{}, PlanIdentity{}, fmt.Errorf("verification execution: %s is malformed", field.name)
		}
	}
	if !validHash(cfg.SourceRevision) {
		return Config{}, TierSelection{}, PlanIdentity{}, errors.New("verification execution: source revision must be a lower-case SHA-256")
	}
	if cfg.Timeout <= 0 || cfg.StdoutCap <= 0 || cfg.StderrCap <= 0 {
		return Config{}, TierSelection{}, PlanIdentity{}, errors.New("verification execution: positive default timeout and output caps are required")
	}
	selection, err := Select(cfg.Plan, cfg.Purpose)
	if err != nil {
		return Config{}, TierSelection{}, PlanIdentity{}, err
	}
	identity, err := Identity(cfg.Plan)
	if err != nil {
		return Config{}, TierSelection{}, PlanIdentity{}, err
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if cfg.AttemptID == nil {
		return Config{}, TierSelection{}, PlanIdentity{}, errors.New("verification execution: attempt ID generator is required")
	}
	if cfg.CommandRunner == nil {
		cfg.CommandRunner = runner.Run
	}
	if cfg.ArtifactPath != "" {
		if _, err := pathguard.Resolve(artifactRoot, cfg.ArtifactPath); err != nil {
			return Config{}, TierSelection{}, PlanIdentity{}, fmt.Errorf("verification execution: artifact path: %w", err)
		}
		if cfg.ArtifactWriter == nil {
			cfg.ArtifactWriter = writeArtifact
		}
	}
	cfg.RepositoryRoot, cfg.ArtifactRoot, cfg.Plan = root, artifactRoot, ClonePlan(cfg.Plan)
	return cfg, selection, identity, nil
}

func commandIdentity(cfg Config, plan PlanIdentity, tier Tier, position int, command verification.Command) (CommandIdentity, error) {
	dir := cfg.RepositoryRoot
	if strings.TrimSpace(command.Dir) != "" {
		var err error
		dir, err = pathguard.Resolve(cfg.RepositoryRoot, command.Dir)
		if err != nil {
			return CommandIdentity{}, err
		}
	}
	timeout := command.Timeout
	if timeout <= 0 {
		timeout = cfg.Timeout
	}
	stdout := command.StdoutCap
	if stdout <= 0 {
		stdout = cfg.StdoutCap
	}
	stderr := command.StderrCap
	if stderr <= 0 {
		stderr = cfg.StderrCap
	}
	identity := CommandIdentity{PlanSHA256: plan.SHA256, Purpose: cfg.Purpose, TierID: tier.ID, TierKind: tier.Kind, Position: position, Name: command.Name, Args: append([]string(nil), command.Args...), Dir: filepath.Clean(dir), Env: append([]string(nil), command.Env...), Timeout: timeout, StdoutCap: stdout, StderrCap: stderr}
	identity.SHA256 = hashCommandIdentity(identity)
	return identity, nil
}

func hashCommandIdentity(identity CommandIdentity) string {
	raw, _ := json.Marshal(struct {
		PlanSHA256 string        `json:"plan_sha256"`
		Purpose    Purpose       `json:"purpose"`
		TierID     string        `json:"tier_id"`
		TierKind   TierKind      `json:"tier_kind"`
		Position   int           `json:"position"`
		Name       string        `json:"name"`
		Args       []string      `json:"args"`
		Dir        string        `json:"dir"`
		Env        []string      `json:"env"`
		Timeout    time.Duration `json:"timeout"`
		StdoutCap  int           `json:"stdout_cap"`
		StderrCap  int           `json:"stderr_cap"`
	}{identity.PlanSHA256, identity.Purpose, identity.TierID, identity.TierKind, identity.Position, identity.Name, identity.Args, identity.Dir, identity.Env, identity.Timeout, identity.StdoutCap, identity.StderrCap})
	return hashBytes(raw)
}

// CommandMaterialSHA256 returns the deterministic material identity hash and
// deliberately ignores identity.SHA256 itself.
func CommandMaterialSHA256(identity CommandIdentity) string { return hashCommandIdentity(identity) }

func executeAttempt(ctx context.Context, cfg Config, identity CommandIdentity, number int) Attempt {
	attempt := Attempt{AttemptID: strings.TrimSpace(cfg.AttemptID()), Number: number, Command: identity, StartedAt: cfg.Clock().UTC()}
	if attempt.AttemptID == "" || strings.ContainsAny(attempt.AttemptID, "\r\n") {
		attempt.Outcome = OutcomeRunnerError
		attempt.Error = "attempt ID generator returned a malformed identity"
		attempt.EndedAt = cfg.Clock().UTC()
		attempt.Duration = duration(attempt.StartedAt, attempt.EndedAt)
		return attempt
	}
	if ctx.Err() != nil {
		attempt.Outcome = OutcomeCancelled
		attempt.Cancelled = true
		attempt.Error = ctx.Err().Error()
		attempt.ExitCode = -1
		attempt.EndedAt = cfg.Clock().UTC()
		attempt.Duration = duration(attempt.StartedAt, attempt.EndedAt)
		return attempt
	}
	r := cfg.CommandRunner(ctx, runner.Command{Name: identity.Name, Args: append([]string(nil), identity.Args...), Dir: identity.Dir, Env: append([]string(nil), identity.Env...), Timeout: identity.Timeout, StdoutLimit: identity.StdoutCap, StderrLimit: identity.StderrCap})
	attempt.ExitCode = r.ExitCode
	attempt.TimedOut = r.TimedOut
	attempt.Error = errorString(r.Err)
	attempt.Stdout = Output{r.Stdout, r.StdoutTruncatedBytes}
	attempt.Stderr = Output{r.Stderr, r.StderrTruncatedBytes}
	attempt.EndedAt = cfg.Clock().UTC()
	attempt.Duration = duration(attempt.StartedAt, attempt.EndedAt)
	switch {
	case r.TimedOut:
		attempt.Outcome = OutcomeTimedOut
	case errors.Is(r.Err, context.Canceled) || ctx.Err() != nil:
		attempt.Outcome = OutcomeCancelled
		attempt.Cancelled = true
	case r.Err != nil:
		attempt.Outcome = OutcomeRunnerError
	case r.ExitCode != 0:
		attempt.Outcome = OutcomeFailed
	default:
		attempt.Outcome = OutcomePassed
		attempt.Passed = true
	}
	return attempt
}

func finishOperation(ctx context.Context, cfg Config, result Result, outcome Outcome, stage string, cause error) (Result, error) {
	result.Outcome = outcome
	result.Gate.OverallOutcome = outcome
	result.FailureStage = stage
	if cause != nil {
		result.FailureReason = cause.Error()
	}
	result.EndedAt = cfg.Clock().UTC()
	result.Duration = duration(result.StartedAt, result.EndedAt)
	result.Aggregate.EndedAt = result.EndedAt
	result.Aggregate.Passed = outcome == OutcomePassed
	if result.Aggregate.Passed {
		result.Aggregate.Status = verification.StatusPassed
	} else {
		result.Aggregate.Status = verification.StatusFailed
		if result.Aggregate.Message == "" {
			result.Aggregate.Message = result.FailureReason
		}
	}
	result.Gate = computeGate(result.Gate, result.Tiers)
	if cfg.ArtifactPath != "" {
		raw, err := MarshalResult(result)
		if err != nil {
			return failOperation(cfg, result, OutcomeArtifactError, "artifact_marshal", err)
		}
		if err := cfg.ArtifactWriter(cfg.ArtifactRoot, cfg.ArtifactPath, raw); err != nil {
			return failOperation(cfg, result, OutcomeArtifactError, "artifact_write", err)
		}
		result.Artifact = &Artifact{Path: filepath.ToSlash(cfg.ArtifactPath), SHA256: hashBytes(raw), ByteSize: len(raw)}
	}
	if err := appendEvent(ctx, cfg, ledger.EventVerificationCompleted, completedEvent(result)); err != nil {
		return failOperation(cfg, result, OutcomeLedgerError, "ledger_complete", err)
	}
	if cause != nil {
		return result, cause
	}
	return result, nil
}

func failOperation(cfg Config, result Result, outcome Outcome, stage string, cause error) (Result, error) {
	result.Outcome = outcome
	result.Gate.OverallOutcome = outcome
	result.FailureStage = stage
	result.FailureReason = cause.Error()
	result.EndedAt = cfg.Clock().UTC()
	result.Duration = duration(result.StartedAt, result.EndedAt)
	result.Aggregate.Status = verification.StatusFailed
	result.Aggregate.Passed = false
	result.Aggregate.EndedAt = result.EndedAt
	result.Gate = computeGate(result.Gate, result.Tiers)
	return result, fmt.Errorf("tiered verification %s: %w", stage, cause)
}

func computeGate(g GateEvidence, tiers []TierResult) GateEvidence {
	outcomes := make(map[string]Outcome, len(tiers))
	for _, tier := range tiers {
		outcomes[tier.ID] = tier.Outcome
	}
	g.RequiredOutcomes = nil
	g.MissingRequired = nil
	acceptable := g.Purpose == PurposeFinal && g.OverallOutcome == OutcomePassed
	selected := stringSet(g.SelectedTiers)
	executed := stringSet(g.ExecutedTiers)
	for _, id := range g.RequiredFinalTiers {
		outcome, ok := outcomes[id]
		if !ok {
			outcome = OutcomeMissing
		}
		g.RequiredOutcomes = append(g.RequiredOutcomes, TierGate{TierID: id, Outcome: outcome})
		if _, s := selected[id]; !s || !ok {
			g.MissingRequired = append(g.MissingRequired, id)
			acceptable = false
			continue
		}
		if _, e := executed[id]; !e || outcome != OutcomePassed {
			acceptable = false
		}
	}
	g.FinalSatisfied = acceptable
	return g
}

func appendAggregate(result *verification.Result, index int, attempt Attempt) {
	c := verification.CommandResult{Index: index, Command: commandString(attempt.Command.Name, attempt.Command.Args), Name: attempt.Command.Name, Args: append([]string(nil), attempt.Command.Args...), Dir: attempt.Command.Dir, Status: verification.StatusFailed, Passed: attempt.Passed, ExitCode: attempt.ExitCode, TimedOut: attempt.TimedOut, Error: attempt.Error, Timeout: attempt.Command.Timeout, Stdout: verification.CappedOutput{Content: attempt.Stdout.Content, TruncatedBytes: attempt.Stdout.TruncatedBytes}, Stderr: verification.CappedOutput{Content: attempt.Stderr.Content, TruncatedBytes: attempt.Stderr.TruncatedBytes}, StartedAt: attempt.StartedAt, EndedAt: attempt.EndedAt}
	if attempt.Passed {
		c.Status = verification.StatusPassed
	}
	result.Commands = append(result.Commands, c)
	if !attempt.Passed && result.FailedCommandIndex < 0 {
		result.FailedCommandIndex = index
	}
}

func appendEvent(ctx context.Context, cfg Config, event ledger.EventType, payload any) error {
	if cfg.Ledger == nil {
		return nil
	}
	_, err := cfg.Ledger.AppendEvent(ctx, cfg.RunID, event, payload)
	return err
}
func startedEvent(r Result, s TierSelection) any {
	return struct {
		SchemaVersion                        string `json:"schema_version"`
		TaskID, OccurrenceID, SourceRevision string
		Plan                                 PlanIdentity
		Purpose                              Purpose
		Selected, Required                   []string
	}{LedgerEventSchemaVersion, r.TaskID, r.OccurrenceID, r.SourceRevision, r.Plan, r.Purpose, s.SelectedTierIDs, s.RequiredFinalTiers}
}
func completedEvent(r Result) any {
	return CompletedLedgerEvent{LedgerEventSchemaVersion, r.Aggregate.Status, r.Aggregate.Passed, r.FailureReason, r.Aggregate.Commands, r.TaskID, r.OccurrenceID, r.SourceRevision, r.Plan, r.Purpose, r.Outcome, r.Gate, r.Tiers, r.Artifact}
}
func rerunEvent(t Tier, c CommandIdentity) any {
	return struct {
		SchemaVersion string `json:"schema_version"`
		TierID        string
		Policy        RerunPolicy
		Command       CommandIdentity
	}{LedgerEventSchemaVersion, t.ID, t.RerunPolicy, c}
}
func writeArtifact(root, path string, raw []byte) error {
	abs, err := pathguard.Resolve(root, path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		return err
	}
	return os.WriteFile(abs, raw, 0644)
}
func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
func stringSet(values []string) map[string]struct{} {
	m := make(map[string]struct{}, len(values))
	for _, v := range values {
		m[v] = struct{}{}
	}
	return m
}
func hasDuplicates(values []string) bool { return len(stringSet(values)) != len(values) }
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func commandString(name string, args []string) string {
	parts := []string{name}
	for _, arg := range args {
		if strings.ContainsAny(arg, " \t\"") {
			parts = append(parts, fmt.Sprintf("%q", arg))
		} else {
			parts = append(parts, arg)
		}
	}
	return strings.Join(parts, " ")
}

func validOutcome(value Outcome) bool {
	switch value {
	case OutcomePassed, OutcomeFailed, OutcomeFlaky, OutcomeMissing, OutcomeTimedOut, OutcomeCancelled, OutcomeRunnerError, OutcomeConfigurationError, OutcomeLedgerError, OutcomeArtifactError:
		return true
	default:
		return false
	}
}
