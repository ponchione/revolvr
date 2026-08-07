package evaluation

import (
	"errors"
	"fmt"
)

var (
	ErrCrashInjected   = errors.New("evaluation: crash injected")
	ErrDivergentReplay = errors.New("evaluation: divergent replay evidence")
)

type recoveryOperation struct {
	Boundary    CrashBoundary
	MaterialSHA string
	EffectSHA   string
	Applied     bool
	Replays     int
}

// RecoveryJournal models the production planned-effect/adopted-effect rule.
// It is deliberately small and deterministic: history is authoritative and a
// changed replay can never be adopted.
type RecoveryJournal struct {
	operations map[CrashBoundary]recoveryOperation
}

func NewRecoveryJournal() *RecoveryJournal {
	return &RecoveryJournal{operations: map[CrashBoundary]recoveryOperation{}}
}

func (j *RecoveryJournal) Plan(boundary CrashBoundary, materialSHA string) error {
	if !validBoundary(boundary) || materialSHA == "" {
		return errors.New("evaluation: invalid recovery plan")
	}
	if existing, ok := j.operations[boundary]; ok {
		if existing.MaterialSHA != materialSHA {
			return ErrDivergentReplay
		}
		return nil
	}
	j.operations[boundary] = recoveryOperation{Boundary: boundary, MaterialSHA: materialSHA}
	return nil
}

func (j *RecoveryJournal) Apply(boundary CrashBoundary, materialSHA, effectSHA string, injectCrash bool) (bool, error) {
	operation, ok := j.operations[boundary]
	if !ok || operation.MaterialSHA != materialSHA || effectSHA == "" {
		return false, ErrDivergentReplay
	}
	if operation.Applied {
		if operation.EffectSHA != effectSHA {
			return false, ErrDivergentReplay
		}
		operation.Replays++
		j.operations[boundary] = operation
		return true, nil
	}
	operation.Applied = true
	operation.EffectSHA = effectSHA
	j.operations[boundary] = operation
	if injectCrash {
		return false, fmt.Errorf("%w at %s", ErrCrashInjected, boundary)
	}
	return false, nil
}

func (j *RecoveryJournal) Fact(boundary CrashBoundary) (CrashReplayFact, error) {
	operation, ok := j.operations[boundary]
	if !ok || !operation.Applied {
		return CrashReplayFact{}, errors.New("evaluation: recovery effect is incomplete")
	}
	return CrashReplayFact{
		Boundary: boundary, EffectSHA256: operation.EffectSHA, ReplayCount: operation.Replays,
		ExactReplayIdempotent: operation.Replays > 0, DivergentReplayOutcome: "unsafe_or_ambiguous",
	}, nil
}

func exerciseCrashBoundary(boundary CrashBoundary, scenarioID string) (CrashReplayFact, error) {
	journal := NewRecoveryJournal()
	material := hashBytes([]byte("material:" + scenarioID + ":" + string(boundary)))
	effect := hashBytes([]byte("effect:" + scenarioID + ":" + string(boundary)))
	if err := journal.Plan(boundary, material); err != nil {
		return CrashReplayFact{}, err
	}
	if _, err := journal.Apply(boundary, material, effect, true); !errors.Is(err, ErrCrashInjected) {
		return CrashReplayFact{}, fmt.Errorf("evaluation: boundary %s did not inject crash: %w", boundary, err)
	}
	if replayed, err := journal.Apply(boundary, material, effect, false); err != nil || !replayed {
		return CrashReplayFact{}, fmt.Errorf("evaluation: boundary %s exact replay failed: replayed=%t err=%w", boundary, replayed, err)
	}
	if _, err := journal.Apply(boundary, material, hashBytes([]byte("divergent:"+scenarioID)), false); !errors.Is(err, ErrDivergentReplay) {
		return CrashReplayFact{}, fmt.Errorf("evaluation: boundary %s accepted divergent replay", boundary)
	}
	return journal.Fact(boundary)
}

func validBoundary(boundary CrashBoundary) bool {
	for _, value := range allCrashBoundaries {
		if boundary == value {
			return true
		}
	}
	return false
}
