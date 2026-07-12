package autonomous

import (
	"strings"
	"testing"
	"time"
)

func TestFinalizationDetailStageValidationAndMonotonicTransition(t *testing.T) {
	previous := validExecutionState(LifecycleStateFinalizing)
	next := previous
	detail := *previous.Finalization
	detail.Stage = FinalizationStageMaterialized
	detail.Capsule = &FinalizationArtifact{Path: ".revolvr/autonomous/tasks/task-1/completion/completion.md", SHA256: strings.Repeat("a", 64), ByteSize: 20}
	detail.Manifest = &FinalizationArtifact{Path: ".revolvr/autonomous/tasks/task-1/completion/completion-manifest.json", SHA256: strings.Repeat("b", 64), ByteSize: 30}
	materialized := detail.AdmittedAt.Add(time.Second)
	detail.MaterializedAt = &materialized
	next.Finalization = &detail
	if err := ValidateExecutionStateTransition(previous, next); err != nil {
		t.Fatal(err)
	}
	if err := ValidateExecutionStateTransition(next, previous); err == nil || !strings.Contains(err.Error(), "regressed") {
		t.Fatalf("regression error = %v", err)
	}
	rewritten := next
	changed := *next.Finalization
	changed.FrozenEvidence.SHA256 = strings.Repeat("c", 64)
	rewritten.Finalization = &changed
	if err := ValidateExecutionStateTransition(next, rewritten); err == nil || !strings.Contains(err.Error(), "authority changed") {
		t.Fatalf("rewrite error = %v", err)
	}
}

func TestFinalizationDetailLifecycleCompatibility(t *testing.T) {
	state := validExecutionState(LifecycleStateFinalizing)
	state.Lifecycle = LifecycleStateWorking
	if err := state.Validate(); err == nil || !strings.Contains(err.Error(), "cannot include finalization detail") {
		t.Fatalf("error = %v", err)
	}
	state = validExecutionState(LifecycleStateFinalizing)
	state.Finalization = nil
	if err := state.Validate(); err == nil || !strings.Contains(err.Error(), "requires finalization detail") {
		t.Fatalf("error = %v", err)
	}
}
