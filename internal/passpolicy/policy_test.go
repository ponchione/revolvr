package passpolicy

import (
	"reflect"
	"strings"
	"testing"

	"revolvr/internal/taskfile"
)

func TestLookupEveryMixedPassPhase(t *testing.T) {
	tests := []struct {
		name              string
		phase             string
		wantProfile       string
		wantAllowNoChange bool
		wantNextPhase     string
		wantCompletesTask bool
	}{
		{
			name:              "implement",
			phase:             taskfile.PhaseImplement,
			wantProfile:       ProfileImplementer,
			wantAllowNoChange: false,
			wantNextPhase:     taskfile.PhaseAudit,
		},
		{
			name:              "audit",
			phase:             taskfile.PhaseAudit,
			wantProfile:       ProfileAuditor,
			wantAllowNoChange: true,
			wantNextPhase:     taskfile.PhaseDocument,
		},
		{
			name:              "document",
			phase:             taskfile.PhaseDocument,
			wantProfile:       ProfileDocumentor,
			wantAllowNoChange: true,
			wantNextPhase:     taskfile.PhaseSimplify,
		},
		{
			name:              "simplify",
			phase:             taskfile.PhaseSimplify,
			wantProfile:       ProfileSimplifier,
			wantAllowNoChange: true,
			wantCompletesTask: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy, err := Lookup(taskfile.WorkflowMixedPassV1, tt.phase)
			if err != nil {
				t.Fatalf("lookup pass policy: %v", err)
			}

			if got, want := policy.Workflow, taskfile.WorkflowMixedPassV1; got != want {
				t.Fatalf("policy workflow = %q, want %q", got, want)
			}
			if got := policy.Phase; got != tt.phase {
				t.Fatalf("policy phase = %q, want %q", got, tt.phase)
			}
			if got := policy.ProfileName; got != tt.wantProfile {
				t.Fatalf("policy profile = %q, want %q", got, tt.wantProfile)
			}
			if got := policy.AllowNoChangeSuccess; got != tt.wantAllowNoChange {
				t.Fatalf("policy allow no-change success = %v, want %v", got, tt.wantAllowNoChange)
			}
			if got := policy.NextPhase; got != tt.wantNextPhase {
				t.Fatalf("policy next phase = %q, want %q", got, tt.wantNextPhase)
			}
			if got := policy.CompletesTask; got != tt.wantCompletesTask {
				t.Fatalf("policy completes task = %v, want %v", got, tt.wantCompletesTask)
			}
		})
	}
}

func TestLookupPhaseOrder(t *testing.T) {
	var got []string
	phase := taskfile.PhaseImplement

	for i := 0; i < 8; i++ {
		got = append(got, phase)
		policy, err := Lookup(taskfile.WorkflowMixedPassV1, phase)
		if err != nil {
			t.Fatalf("lookup pass policy for %q: %v", phase, err)
		}
		if policy.CompletesTask {
			got = append(got, taskfile.StatusCompleted)
			break
		}
		phase = policy.NextPhase
	}

	want := []string{
		taskfile.PhaseImplement,
		taskfile.PhaseAudit,
		taskfile.PhaseDocument,
		taskfile.PhaseSimplify,
		taskfile.StatusCompleted,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("phase order = %#v, want %#v", got, want)
	}
}

func TestLookupSimplifyIsTerminal(t *testing.T) {
	policy, err := Lookup(taskfile.WorkflowMixedPassV1, taskfile.PhaseSimplify)
	if err != nil {
		t.Fatalf("lookup pass policy: %v", err)
	}
	if !policy.CompletesTask {
		t.Fatal("simplify policy does not complete task")
	}
	if policy.NextPhase != "" {
		t.Fatalf("simplify next phase = %q, want empty", policy.NextPhase)
	}
}

func TestLookupNoChangePermissions(t *testing.T) {
	tests := []struct {
		phase string
		want  bool
	}{
		{phase: taskfile.PhaseImplement, want: false},
		{phase: taskfile.PhaseAudit, want: true},
		{phase: taskfile.PhaseDocument, want: true},
		{phase: taskfile.PhaseSimplify, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.phase, func(t *testing.T) {
			policy, err := Lookup(taskfile.WorkflowMixedPassV1, tt.phase)
			if err != nil {
				t.Fatalf("lookup pass policy: %v", err)
			}
			if got := policy.AllowNoChangeSuccess; got != tt.want {
				t.Fatalf("allow no-change success = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLookupRejectsInvalidWorkflow(t *testing.T) {
	for _, workflow := range []string{"single-pass-v1", taskfile.WorkflowAutonomousV1} {
		_, err := Lookup(workflow, taskfile.PhaseImplement)
		if err == nil {
			t.Fatalf("lookup pass policy for %q succeeded, want invalid workflow error", workflow)
		}
		if !strings.Contains(err.Error(), `unsupported workflow "`+workflow+`"`) {
			t.Fatalf("error = %v, want unsupported workflow %q", err, workflow)
		}
	}
}

func TestLookupRejectsInvalidPhase(t *testing.T) {
	_, err := Lookup(taskfile.WorkflowMixedPassV1, "review")
	if err == nil {
		t.Fatal("lookup pass policy succeeded, want invalid phase error")
	}
	if !strings.Contains(err.Error(), `unsupported phase "review" for workflow "mixed-pass-v1"`) {
		t.Fatalf("error = %v, want unsupported phase", err)
	}
}
