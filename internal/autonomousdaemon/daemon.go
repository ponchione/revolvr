// Package autonomousdaemon owns only foreground waiting, stable fingerprint
// debounce, and requests for bounded durable queue sweeps.
package autonomousdaemon

import (
	"context"
	"errors"
	"fmt"
	"time"

	"revolvr/internal/autonomousqueue"
)

type StopReason string

const (
	StopCancelled  StopReason = "cancelled"
	StopSafety     StopReason = "safety_stop"
	StopUnsafe     StopReason = "unsafe_or_ambiguous"
	StopSweepLimit StopReason = "sweep_limit"
)

type Fingerprint func(context.Context) (string, error)
type Sweep func(context.Context, int64) (autonomousqueue.Result, error)
type Wait func(context.Context, time.Duration) error
type WakeObserver func(Wake)

type Wake struct {
	Generation  int64  `json:"generation"`
	Fingerprint string `json:"fingerprint"`
}

type Config struct {
	FullyUnattended bool
	PollInterval    time.Duration
	Debounce        time.Duration
	MaxSweeps       int64
	Fingerprint     Fingerprint
	Sweep           Sweep
	Wait            Wait
	OnWake          WakeObserver
}

type Result struct {
	StopReason      StopReason             `json:"stop_reason"`
	StopDetail      string                 `json:"stop_detail,omitempty"`
	Sweeps          int64                  `json:"sweeps"`
	Wakes           []Wake                 `json:"wakes,omitempty"`
	LastFingerprint string                 `json:"last_fingerprint,omitempty"`
	LastQueue       autonomousqueue.Result `json:"last_queue"`
}

func RunDaemon(ctx context.Context, cfg Config) (Result, error) {
	if !cfg.FullyUnattended {
		return Result{}, errors.New("autonomous daemon: explicit fully-unattended authority is required")
	}
	if cfg.PollInterval <= 0 || cfg.Debounce <= 0 || cfg.MaxSweeps <= 0 || cfg.Fingerprint == nil || cfg.Sweep == nil {
		return Result{}, errors.New("autonomous daemon: positive poll/debounce/sweep bounds and dependencies are required")
	}
	if cfg.Wait == nil {
		cfg.Wait = waitContext
	}
	var result Result
	for generation := int64(1); generation <= cfg.MaxSweeps; generation++ {
		beforeSweep, err := cfg.Fingerprint(ctx)
		if err != nil {
			result.StopReason, result.StopDetail = StopUnsafe, err.Error()
			return result, err
		}
		queue, err := cfg.Sweep(ctx, generation)
		result.Sweeps++
		result.LastQueue = queue
		if err != nil {
			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				result.StopReason, result.StopDetail = StopCancelled, err.Error()
				return result, err
			}
			result.StopReason, result.StopDetail = StopUnsafe, err.Error()
			return result, err
		}
		switch queue.StopReason {
		case autonomousqueue.StopSafety:
			result.StopReason, result.StopDetail = StopSafety, queue.StopDetail
			return result, nil
		case autonomousqueue.StopUnsafeAmbiguous:
			result.StopReason, result.StopDetail = StopUnsafe, queue.StopDetail
			return result, nil
		case autonomousqueue.StopCancelled:
			result.StopReason, result.StopDetail = StopCancelled, queue.StopDetail
			return result, context.Canceled
		case autonomousqueue.StopBudgetExhausted:
			continue
		}
		if generation == cfg.MaxSweeps {
			result.StopReason = StopSweepLimit
			result.StopDetail = fmt.Sprintf("daemon maximum sweep bound %d reached", cfg.MaxSweeps)
			return result, nil
		}

		baseline, err := cfg.Fingerprint(ctx)
		if err != nil {
			result.StopReason, result.StopDetail = StopUnsafe, err.Error()
			return result, err
		}
		result.LastFingerprint = baseline
		if baseline != beforeSweep {
			stable, stableErr := debounce(ctx, cfg, baseline)
			if stableErr != nil {
				result.StopReason, result.StopDetail = classifyWaitError(stableErr), stableErr.Error()
				return result, stableErr
			}
			result.LastFingerprint = stable
			if stable == baseline {
				wake := Wake{Generation: generation + 1, Fingerprint: stable}
				result.Wakes = append(result.Wakes, wake)
				emit(cfg.OnWake, wake)
				continue
			}
			baseline = stable
		}
		for {
			if err := cfg.Wait(ctx, cfg.PollInterval); err != nil {
				result.StopReason, result.StopDetail = StopCancelled, err.Error()
				return result, err
			}
			candidate, err := cfg.Fingerprint(ctx)
			if err != nil {
				result.StopReason, result.StopDetail = StopUnsafe, err.Error()
				return result, err
			}
			if candidate == baseline {
				continue
			}
			stable, err := debounce(ctx, cfg, candidate)
			if err != nil {
				result.StopReason, result.StopDetail = classifyWaitError(err), err.Error()
				return result, err
			}
			if stable != candidate {
				baseline = stable
				result.LastFingerprint = stable
				continue
			}
			wake := Wake{Generation: generation + 1, Fingerprint: stable}
			result.Wakes = append(result.Wakes, wake)
			result.LastFingerprint = stable
			emit(cfg.OnWake, wake)
			break
		}
	}
	result.StopReason = StopSweepLimit
	result.StopDetail = fmt.Sprintf("daemon maximum sweep bound %d reached", cfg.MaxSweeps)
	return result, nil
}

func debounce(ctx context.Context, cfg Config, candidate string) (string, error) {
	if err := cfg.Wait(ctx, cfg.Debounce); err != nil {
		return "", err
	}
	return cfg.Fingerprint(ctx)
}

func classifyWaitError(err error) StopReason {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return StopCancelled
	}
	return StopUnsafe
}

func waitContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func emit(observer WakeObserver, wake Wake) {
	if observer != nil {
		defer func() { _ = recover() }()
		observer(wake)
	}
}
