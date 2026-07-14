package autonomousnotification

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"revolvr/internal/lock"
	"revolvr/internal/redact"
	"revolvr/internal/runner"
	"revolvr/internal/runtimepath"
)

type Stage string

const (
	StageAdmitted  Stage = "admitted"
	StageRunning   Stage = "running"
	StageRetryable Stage = "retryable"
	StageSucceeded Stage = "succeeded"
	StageFailed    Stage = "failed"
	StageResumable Stage = "resumable"
)

type Intent struct {
	SchemaVersion string    `json:"schema_version"`
	DeliveryID    string    `json:"delivery_id"`
	EventID       string    `json:"event_id"`
	Event         Event     `json:"event"`
	PayloadSHA256 string    `json:"payload_sha256"`
	PayloadSize   int       `json:"payload_size"`
	Policy        Policy    `json:"policy"`
	PolicySHA256  string    `json:"policy_sha256"`
	ConfigSchema  string    `json:"config_schema"`
	ConfigSHA256  string    `json:"config_sha256"`
	AdmittedAt    time.Time `json:"admitted_at"`
}

type Attempt struct {
	Number               int           `json:"number"`
	StartedAt            time.Time     `json:"started_at"`
	CompletedAt          time.Time     `json:"completed_at"`
	Executable           string        `json:"executable"`
	Args                 []string      `json:"args,omitempty"`
	Directory            string        `json:"directory"`
	EnvironmentNames     []string      `json:"environment_names,omitempty"`
	Timeout              time.Duration `json:"timeout"`
	StdoutCap            int           `json:"stdout_cap"`
	StderrCap            int           `json:"stderr_cap"`
	ExitCode             int           `json:"exit_code"`
	TimedOut             bool          `json:"timed_out"`
	Cancelled            bool          `json:"cancelled"`
	RunnerError          bool          `json:"runner_error"`
	Retryable            bool          `json:"retryable"`
	Stdout               string        `json:"stdout,omitempty"`
	Stderr               string        `json:"stderr,omitempty"`
	Error                string        `json:"error,omitempty"`
	StdoutTruncatedBytes int64         `json:"stdout_truncated_bytes,omitempty"`
	StderrTruncatedBytes int64         `json:"stderr_truncated_bytes,omitempty"`
}

type Journal struct {
	SchemaVersion string    `json:"schema_version"`
	DeliveryID    string    `json:"delivery_id"`
	Sequence      int64     `json:"sequence"`
	Stage         Stage     `json:"stage"`
	Attempts      []Attempt `json:"attempts,omitempty"`
	Detail        string    `json:"detail,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Transition struct {
	SchemaVersion string    `json:"schema_version"`
	DeliveryID    string    `json:"delivery_id"`
	Sequence      int64     `json:"sequence"`
	Stage         Stage     `json:"stage"`
	Attempt       *Attempt  `json:"attempt,omitempty"`
	Detail        string    `json:"detail,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

type Result struct {
	SchemaVersion string `json:"schema_version"`
	DeliveryID    string `json:"delivery_id,omitempty"`
	Event         Event  `json:"event,omitempty"`
	Stage         Stage  `json:"stage,omitempty"`
	Attempts      int    `json:"attempts,omitempty"`
	Detail        string `json:"detail,omitempty"`
	Replayed      bool   `json:"replayed,omitempty"`
	Disabled      bool   `json:"disabled,omitempty"`
}

type CommandRunner func(context.Context, runner.Command) runner.Result
type LookPath func(string) (string, error)
type LookupEnv func(string) (string, bool)
type Wait func(context.Context, time.Duration) error

type DeliveryConfig struct {
	RepositoryRoot   string
	Payload          Payload
	PayloadBytes     []byte
	Policy           Policy
	RedactionNames   []string
	Clock            func() time.Time
	Runner           CommandRunner
	LookPath         LookPath
	LookupEnv        LookupEnv
	Wait             Wait
	persistenceFault persistenceFault
}

func Deliver(ctx context.Context, cfg DeliveryConfig) (Result, error) {
	policy, err := cfg.Policy.Normalize(cfg.RedactionNames)
	if err != nil {
		return Result{}, err
	}
	if !policy.Enabled || !policy.Allows(cfg.Payload.Event) {
		return Result{SchemaVersion: ResultSchemaVersion, Event: cfg.Payload.Event, Disabled: true}, nil
	}
	if err := cfg.Payload.Validate(); err != nil {
		return Result{}, err
	}
	if parsed, err := DecodePayload(cfg.PayloadBytes); err != nil || parsed.DeliveryID != cfg.Payload.DeliveryID {
		return Result{}, errors.Join(err, errors.New("notification delivery: payload bytes do not match payload authority"))
	}
	root, err := filepath.Abs(cfg.RepositoryRoot)
	if err != nil || strings.TrimSpace(cfg.RepositoryRoot) == "" {
		return Result{}, errors.Join(err, errors.New("notification delivery: repository root is required"))
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if cfg.Runner == nil {
		cfg.Runner = CommandRunner(runner.Run)
	}
	if cfg.LookPath == nil {
		cfg.LookPath = exec.LookPath
	}
	if cfg.LookupEnv == nil {
		cfg.LookupEnv = os.LookupEnv
	}
	if cfg.Wait == nil {
		cfg.Wait = waitContext
	}
	redactor, _, err := redact.New(redact.Policy{SchemaVersion: redact.PolicySchemaVersion, EnvironmentVariables: append([]string(nil), cfg.RedactionNames...)}, redact.LookupEnv(cfg.LookupEnv))
	if err != nil {
		return Result{}, err
	}
	policyRaw, _ := canonical(policy)
	if redactor.String(string(policyRaw)) != string(policyRaw) {
		return Result{}, errors.New("notification delivery: configured hook policy contains a secret value")
	}

	dir := deliveryDir(root, cfg.Payload.DeliveryID)
	unlock, err := acquireDeliveryLock(ctx, root, dir)
	if err != nil {
		return Result{}, err
	}
	defer unlock()
	policyID, _ := policy.Identity(cfg.RedactionNames)
	intent := Intent{SchemaVersion: IntentSchemaVersion, DeliveryID: cfg.Payload.DeliveryID, EventID: cfg.Payload.EventID, Event: cfg.Payload.Event, PayloadSHA256: hash(cfg.PayloadBytes), PayloadSize: len(cfg.PayloadBytes), Policy: policy, PolicySHA256: policyID, ConfigSchema: cfg.Payload.EffectiveConfigSchema, ConfigSHA256: cfg.Payload.EffectiveConfigSHA256, AdmittedAt: cfg.Payload.OccurredAt}
	journal, replayed, err := admitWithFault(dir, intent, cfg.PayloadBytes, cfg.Clock().UTC(), cfg.persistenceFault)
	if err != nil {
		return Result{}, err
	}
	if journal.Stage == StageSucceeded {
		return resultFrom(cfg.Payload.Event, journal, true), nil
	}
	if journal.Stage == StageFailed {
		return resultFrom(cfg.Payload.Event, journal, true), errors.New(journal.Detail)
	}
	if err := ctx.Err(); err != nil {
		journal, persistErr := transitionWithFault(dir, journal, StageResumable, "cancelled before hook start", nil, cfg.Clock().UTC(), cfg.persistenceFault)
		return resultFrom(cfg.Payload.Event, journal, replayed), joinCancellationPersistence(err, persistErr)
	}
	resolved, err := cfg.LookPath(policy.Executable)
	if err != nil {
		detail := redactor.String(fmt.Sprintf("notification executable %q unavailable: %v", policy.Executable, err))
		journal, persistErr := transitionWithFault(dir, journal, StageFailed, detail, nil, cfg.Clock().UTC(), cfg.persistenceFault)
		return resultFrom(cfg.Payload.Event, journal, replayed), errors.Join(errors.New(detail), persistErr)
	}
	if resolved, err = filepath.Abs(resolved); err != nil {
		return Result{}, err
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		detail := redactor.String("notification executable is not an executable regular file")
		journal, persistErr := transitionWithFault(dir, journal, StageFailed, detail, nil, cfg.Clock().UTC(), cfg.persistenceFault)
		return resultFrom(cfg.Payload.Event, journal, replayed), errors.Join(errors.New(detail), err, persistErr)
	}
	env := make([]string, 0, len(policy.EnvironmentNames))
	for _, name := range policy.EnvironmentNames {
		value, ok := cfg.LookupEnv(name)
		if !ok || value == "" {
			detail := fmt.Sprintf("notification environment variable %q is missing or empty", name)
			journal, persistErr := transitionWithFault(dir, journal, StageFailed, detail, nil, cfg.Clock().UTC(), cfg.persistenceFault)
			return resultFrom(cfg.Payload.Event, journal, replayed), errors.Join(errors.New(detail), persistErr)
		}
		env = append(env, name+"="+value)
	}
	if journal.Stage == StageRunning {
		number := len(journal.Attempts) + 1
		recovered := Attempt{Number: number, StartedAt: journal.UpdatedAt, CompletedAt: cfg.Clock().UTC(), Executable: redactor.String(resolved), Args: redactedStrings(policy.Args, redactor), Directory: redactor.String(root), EnvironmentNames: append([]string(nil), policy.EnvironmentNames...), Timeout: policy.Timeout, StdoutCap: policy.StdoutCap, StderrCap: policy.StderrCap, ExitCode: -1, RunnerError: true, Retryable: true, Error: "prior hook attempt was interrupted before local completion was durable"}
		stage, detail := StageRetryable, "interrupted hook attempt recovered for bounded retry"
		if number >= policy.MaximumAttempts {
			stage, detail = StageFailed, "interrupted hook attempt exhausted delivery attempts"
		}
		journal, err = transitionWithFault(dir, journal, stage, detail, &recovered, recovered.CompletedAt, cfg.persistenceFault)
		if err != nil {
			return resultFrom(cfg.Payload.Event, journal, replayed), err
		}
		if stage == StageFailed {
			return resultFrom(cfg.Payload.Event, journal, replayed), errors.New(detail)
		}
	}
	for len(journal.Attempts) < policy.MaximumAttempts {
		if err := ctx.Err(); err != nil {
			journal, persistErr := transitionWithFault(dir, journal, StageResumable, "cancelled before hook start", nil, cfg.Clock().UTC(), cfg.persistenceFault)
			return resultFrom(cfg.Payload.Event, journal, replayed), joinCancellationPersistence(err, persistErr)
		}
		number := len(journal.Attempts) + 1
		started := cfg.Clock().UTC()
		journal, err = transitionWithFault(dir, journal, StageRunning, fmt.Sprintf("attempt %d running", number), nil, started, cfg.persistenceFault)
		if err != nil {
			return resultFrom(cfg.Payload.Event, journal, replayed), err
		}
		commandResult := cfg.Runner(ctx, runner.Command{Name: resolved, Args: append([]string(nil), policy.Args...), Stdin: bytes.NewReader(cfg.PayloadBytes), Dir: root, Env: append([]string(nil), env...), ReplaceEnv: true, Timeout: policy.Timeout, StdoutLimit: policy.StdoutCap, StderrLimit: policy.StderrCap})
		completed := cfg.Clock().UTC()
		attempt := Attempt{Number: number, StartedAt: started, CompletedAt: completed, Executable: redactor.String(resolved), Args: redactedStrings(policy.Args, redactor), Directory: redactor.String(root), EnvironmentNames: append([]string(nil), policy.EnvironmentNames...), Timeout: policy.Timeout, StdoutCap: policy.StdoutCap, StderrCap: policy.StderrCap, ExitCode: commandResult.ExitCode, TimedOut: commandResult.TimedOut, Stdout: redactor.String(commandResult.Stdout), Stderr: redactor.String(commandResult.Stderr), StdoutTruncatedBytes: commandResult.StdoutTruncatedBytes, StderrTruncatedBytes: commandResult.StderrTruncatedBytes}
		if commandResult.Err != nil {
			attempt.Error = redactor.String(commandResult.Err.Error())
		}
		attempt.Cancelled = errors.Is(commandResult.Err, context.Canceled) || ctx.Err() != nil
		attempt.RunnerError = commandResult.Err != nil && !commandResult.TimedOut && !attempt.Cancelled
		attempt.Retryable = commandResult.TimedOut || commandResult.Err == nil && commandResult.ExitCode != 0
		success := commandResult.Err == nil && !commandResult.TimedOut && commandResult.ExitCode == 0
		stage, detail := StageSucceeded, "hook delivered"
		if !success {
			switch {
			case attempt.Cancelled:
				stage, detail = StageResumable, "hook cancelled"
			case attempt.Retryable && number < policy.MaximumAttempts:
				stage, detail = StageRetryable, "hook failed; retry admitted"
			default:
				stage, detail = StageFailed, "hook delivery failed"
			}
		}
		var cancelErr error
		if stage == StageResumable {
			cancelErr = deliveryCancellationError(ctx, commandResult.Err)
		}
		journal, err = transitionWithFault(dir, journal, stage, detail, &attempt, completed, cfg.persistenceFault)
		if err != nil {
			if stage == StageResumable {
				return resultFrom(cfg.Payload.Event, journal, replayed), joinCancellationPersistence(cancelErr, err)
			}
			return resultFrom(cfg.Payload.Event, journal, replayed), err
		}
		if success {
			return resultFrom(cfg.Payload.Event, journal, replayed), nil
		}
		if stage == StageResumable {
			return resultFrom(cfg.Payload.Event, journal, replayed), cancelErr
		}
		if stage == StageFailed {
			return resultFrom(cfg.Payload.Event, journal, replayed), errors.New(detail)
		}
		if err := cfg.Wait(ctx, policy.RetryDelay); err != nil {
			journal, persistErr := transitionWithFault(dir, journal, StageResumable, "cancelled during retry delay", nil, cfg.Clock().UTC(), cfg.persistenceFault)
			return resultFrom(cfg.Payload.Event, journal, replayed), joinCancellationPersistence(err, persistErr)
		}
	}
	return resultFrom(cfg.Payload.Event, journal, replayed), errors.New("notification delivery: attempts exhausted")
}

func deliveryCancellationError(ctx context.Context, commandErr error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if errors.Is(commandErr, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return context.Canceled
}

func joinCancellationPersistence(cancelErr, persistErr error) error {
	if persistErr == nil {
		return cancelErr
	}
	return errors.Join(cancelErr, persistErr)
}

func resultFrom(event Event, journal Journal, replayed bool) Result {
	return Result{SchemaVersion: ResultSchemaVersion, DeliveryID: journal.DeliveryID, Event: event, Stage: journal.Stage, Attempts: len(journal.Attempts), Detail: journal.Detail, Replayed: replayed}
}

func redactedStrings(values []string, redactor *redact.Redactor) []string {
	result := make([]string, len(values))
	for i, value := range values {
		result[i] = redactor.String(value)
	}
	return result
}
func waitContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func acquireDeliveryLock(ctx context.Context, root, dir string, afterOpen ...func(root, path string) error) (func(), error) {
	if err := ensureSafeDirectory(root, dir); err != nil {
		return nil, fmt.Errorf("%w: %w", runtimepath.ErrUnsafe, err)
	}
	rel, err := filepath.Rel(root, filepath.Join(dir, "delivery.lock"))
	if err != nil {
		return nil, err
	}
	var hook func(root, path string) error
	if len(afterOpen) > 0 {
		hook = afterOpen[0]
	}
	lease, err := lock.AcquireFlock(ctx, root, lock.FlockConfig{
		RelativePath: filepath.ToSlash(rel),
		Mode:         lock.FlockExclusive,
		Wait:         true,
		Create:       true,
		AfterOpen:    hook,
	})
	if err != nil {
		return nil, err
	}
	return func() { _ = lease.Close() }, nil
}
