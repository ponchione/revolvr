package autonomousdaemon

import (
	"context"
	"errors"
	"testing"
	"time"

	"revolvr/internal/autonomousqueue"
)

func TestDaemonDebouncesTransientAndRepeatedChanges(t *testing.T) {
	fingerprints := []string{"a", "a", "b", "c", "c", "d", "d"}
	index := 0
	fingerprint := func(context.Context) (string, error) {
		if index >= len(fingerprints) {
			return fingerprints[len(fingerprints)-1], nil
		}
		value := fingerprints[index]
		index++
		return value, nil
	}
	sweeps := 0
	result, err := RunDaemon(context.Background(), Config{FullyUnattended: true, PollInterval: time.Millisecond, Debounce: time.Millisecond, MaxSweeps: 2, Fingerprint: fingerprint, Wait: func(context.Context, time.Duration) error { return nil }, Sweep: func(context.Context, int64) (autonomousqueue.Result, error) {
		sweeps++
		return autonomousqueue.Result{StopReason: autonomousqueue.StopDrained}, nil
	}})
	if err != nil || result.StopReason != StopSweepLimit || sweeps != 2 || len(result.Wakes) != 1 || result.Wakes[0].Fingerprint != "d" {
		t.Fatalf("result=%+v err=%v sweeps=%d", result, err, sweeps)
	}
}

func TestDaemonAdmissionCancellationAndUnsafeChange(t *testing.T) {
	base := Config{PollInterval: time.Millisecond, Debounce: time.Millisecond, MaxSweeps: 2, Fingerprint: func(context.Context) (string, error) { return "same", nil }, Sweep: func(context.Context, int64) (autonomousqueue.Result, error) {
		return autonomousqueue.Result{StopReason: autonomousqueue.StopDrained}, nil
	}}
	if _, err := RunDaemon(context.Background(), base); err == nil {
		t.Fatal("operator-attended daemon admission succeeded")
	}
	base.FullyUnattended = true
	base.Wait = func(context.Context, time.Duration) error { return context.Canceled }
	result, err := RunDaemon(context.Background(), base)
	if !errors.Is(err, context.Canceled) || result.StopReason != StopCancelled {
		t.Fatalf("cancel result=%+v err=%v", result, err)
	}
	base.Wait = func(context.Context, time.Duration) error { return nil }
	base.Fingerprint = func(context.Context) (string, error) { return "", errors.New("malformed changed authority") }
	result, err = RunDaemon(context.Background(), base)
	if err == nil || result.StopReason != StopUnsafe {
		t.Fatalf("unsafe result=%+v err=%v", result, err)
	}
}

func TestDaemonStopsForQueueSafetyWithoutWaiting(t *testing.T) {
	waited := false
	result, err := RunDaemon(context.Background(), Config{FullyUnattended: true, PollInterval: time.Millisecond, Debounce: time.Millisecond, MaxSweeps: 3, Fingerprint: func(context.Context) (string, error) { return "x", nil }, Wait: func(context.Context, time.Duration) error { waited = true; return nil }, Sweep: func(context.Context, int64) (autonomousqueue.Result, error) {
		return autonomousqueue.Result{StopReason: autonomousqueue.StopSafety, StopDetail: "preflight"}, nil
	}})
	if err != nil || result.StopReason != StopSafety || waited {
		t.Fatalf("result=%+v err=%v waited=%v", result, err, waited)
	}
}

func TestDaemonObservesEventArrivingDuringSweep(t *testing.T) {
	authority := "old"
	sweeps := 0
	polls := 0
	result, err := RunDaemon(context.Background(), Config{FullyUnattended: true, PollInterval: time.Millisecond, Debounce: time.Millisecond, MaxSweeps: 2, Fingerprint: func(context.Context) (string, error) { return authority, nil }, Wait: func(context.Context, time.Duration) error { polls++; return nil }, Sweep: func(context.Context, int64) (autonomousqueue.Result, error) {
		sweeps++
		if sweeps == 1 {
			authority = "new"
		}
		return autonomousqueue.Result{StopReason: autonomousqueue.StopDrained}, nil
	}})
	if err != nil || sweeps != 2 || len(result.Wakes) != 1 || result.Wakes[0].Fingerprint != "new" || polls != 1 {
		t.Fatalf("result=%+v err=%v sweeps=%d waits=%d", result, err, sweeps, polls)
	}
}
