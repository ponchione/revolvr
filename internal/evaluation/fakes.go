package evaluation

import (
	"context"
	"errors"
	"time"
)

type deterministicClock struct {
	started time.Time
	current time.Time
}

func newDeterministicClock() *deterministicClock {
	start := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	return &deterministicClock{started: start, current: start}
}

func (c *deterministicClock) Advance(duration time.Duration) {
	c.current = c.current.Add(duration)
}

func (c *deterministicClock) Elapsed() time.Duration {
	return c.current.Sub(c.started)
}

type deterministicModelResult struct {
	Claim      string
	TokenUsage TokenMetrics
}

type deterministicModel struct {
	calls int
}

func (m *deterministicModel) Invoke(ctx context.Context, request ExecutionRequest) (deterministicModelResult, error) {
	if err := ctx.Err(); err != nil {
		return deterministicModelResult{}, err
	}
	if request.Mode != DirectToolsV1 || request.Authority.SHA256 == "" {
		return deterministicModelResult{}, errors.New("evaluation fake model: authority is not admitted")
	}
	m.calls++
	// The fake intentionally reports no usage. Callers retain explicit
	// omissions instead of estimating token counts or cost.
	return deterministicModelResult{Claim: "scripted:" + request.Scenario.Behavior}, nil
}

type deterministicSandbox struct {
	starts  int
	removed bool
}

func (s *deterministicSandbox) Start(ctx context.Context, request ExecutionRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if request.Authority.Policy.Network != "none" {
		return errors.New("evaluation fake sandbox: network policy is not none")
	}
	s.starts++
	return nil
}

func (s *deterministicSandbox) Remove() {
	if s.starts > 0 {
		s.removed = true
	}
}

type deterministicAcceptance struct {
	checks int
}

type measuredExecution struct {
	directTools        int
	repeatedReads      int
	verificationRuns   int
	verificationReuses int
	correctionCycles   int
}

func measureScriptedExecution(scenario Scenario) (measuredExecution, error) {
	if scenario.RepeatedReadCount > scenario.DirectToolCount || scenario.DirectToolCount > 0 && scenario.RepeatedReadCount == scenario.DirectToolCount {
		return measuredExecution{}, errors.New("evaluation fakes: impossible scripted tool metrics")
	}
	seenReads := map[string]bool{}
	value := measuredExecution{}
	uniqueReads := scenario.DirectToolCount - scenario.RepeatedReadCount
	for index := 0; index < scenario.DirectToolCount; index++ {
		identity := "tool-call-a"
		if index < uniqueReads {
			identity = "tool-call-" + string(rune('a'+index))
		}
		value.directTools++
		if seenReads[identity] {
			value.repeatedReads++
		}
		seenReads[identity] = true
	}
	for range scenario.VerificationExecutions {
		value.verificationRuns++
	}
	for range scenario.VerificationReuses {
		value.verificationReuses++
	}
	for range scenario.CorrectionCycles {
		value.correctionCycles++
	}
	if value.repeatedReads != scenario.RepeatedReadCount {
		return measuredExecution{}, errors.New("evaluation fakes: repeated-read measurement differs from scenario script")
	}
	return value, nil
}

func (a *deterministicAcceptance) Evaluate(request ExecutionRequest, result Result) (bool, error) {
	a.checks++
	if len(request.Authority.Acceptance) != len(result.Criteria) || request.Authority.Expected.Outcome != result.Outcome {
		return false, errors.New("evaluation fake acceptance: immutable acceptance authority changed")
	}
	if !behaviorCompleted(request.Scenario.Behavior) {
		return false, nil
	}
	return result.Task.Status == "completed" && result.Verification.State.Status == "passed" && result.Verification.FreshFinal && result.Audit.State.Status == "clean" && result.Audit.Independent, nil
}

func workerReached(behavior string) bool {
	switch behavior {
	case "missing_dependency", "cyclic_dependency", "ambiguity":
		return false
	default:
		return true
	}
}
