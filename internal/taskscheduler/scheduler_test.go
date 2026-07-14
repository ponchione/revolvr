package taskscheduler

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestDependencyStateSemantics(t *testing.T) {
	tests := []struct {
		name              string
		workflow          Workflow
		state             State
		wantReason        Reason
		wantSelected      string
		wantCategoryCount int
	}{
		{name: "pending prerequisite", workflow: WorkflowMixedPassV1, state: StatePending, wantReason: ReasonWaitingDependency, wantSelected: "dependency", wantCategoryCount: 1},
		{name: "completed prerequisite", workflow: WorkflowMixedPassV1, state: StateCompleted, wantReason: ReasonReady, wantSelected: "dependent", wantCategoryCount: 0},
		{name: "running prerequisite", workflow: WorkflowMixedPassV1, state: StateRunning, wantReason: ReasonWaitingDependency, wantCategoryCount: 1},
		{name: "blocked prerequisite", workflow: WorkflowMixedPassV1, state: StateBlocked, wantReason: ReasonBlockedDependency, wantCategoryCount: 1},
		{name: "needs input prerequisite", workflow: WorkflowAutonomousV1, state: StateNeedsInput, wantReason: ReasonNeedsInputDependency, wantCategoryCount: 1},
		{name: "cancelled prerequisite", workflow: WorkflowMixedPassV1, state: StateCancelled, wantReason: ReasonTerminalUnsatisfiedDependency, wantCategoryCount: 1},
		{name: "abandoned prerequisite", workflow: WorkflowMixedPassV1, state: StateAbandoned, wantReason: ReasonTerminalUnsatisfiedDependency, wantCategoryCount: 1},
		{name: "superseded prerequisite", workflow: WorkflowMixedPassV1, state: StateSuperseded, wantReason: ReasonTerminalUnsatisfiedDependency, wantCategoryCount: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dependency := scheduledTask("dependency", tt.workflow, tt.state, 99)
			dependent := scheduledTask("dependent", WorkflowMixedPassV1, StatePending, 1)
			dependent.DependsOn = []string{dependency.ID}
			result := Evaluate(Input{Tasks: []Task{dependent, dependency}})
			if !result.Valid() {
				t.Fatalf("invalid result: %#v", result.InvalidGraph)
			}
			got := taskResult(t, result, dependent.ID)
			if got.Reason != tt.wantReason {
				t.Fatalf("reason = %q, want %q", got.Reason, tt.wantReason)
			}
			if tt.state == StateCompleted {
				if len(got.UnmetDependencyIDs) != 0 || len(got.DependencyIssues) != 0 {
					t.Fatalf("completed dependency remained unmet: %#v", got)
				}
			} else if !reflect.DeepEqual(got.UnmetDependencyIDs, []string{"dependency"}) {
				t.Fatalf("unmet dependencies = %#v", got.UnmetDependencyIDs)
			}
			selected := ""
			if result.SelectedNext != nil {
				selected = result.SelectedNext.TaskID
			}
			if selected != tt.wantSelected {
				t.Fatalf("selected = %q, want %q", selected, tt.wantSelected)
			}
			switch tt.wantReason {
			case ReasonWaitingDependency:
				if len(result.Waiting) != tt.wantCategoryCount {
					t.Fatalf("waiting = %#v", result.Waiting)
				}
			case ReasonBlockedDependency, ReasonNeedsInputDependency:
				if len(result.DependencyBlocked) != tt.wantCategoryCount {
					t.Fatalf("blocked = %#v", result.DependencyBlocked)
				}
			case ReasonTerminalUnsatisfiedDependency:
				if len(result.TerminalUnsatisfied) != tt.wantCategoryCount {
					t.Fatalf("terminal unsatisfied = %#v", result.TerminalUnsatisfied)
				}
			}
		})
	}
}

func TestDependencyReasonPrecedenceAndExactUnmetIDs(t *testing.T) {
	pending := scheduledTask("pending", WorkflowMixedPassV1, StatePending, 1)
	blocked := scheduledTask("blocked", WorkflowMixedPassV1, StateBlocked, 1)
	needsInput := scheduledTask("input", WorkflowAutonomousV1, StateNeedsInput, 1)
	cancelled := scheduledTask("cancelled", WorkflowMixedPassV1, StateCancelled, 1)
	completed := scheduledTask("completed", WorkflowMixedPassV1, StateCompleted, 1)
	dependent := scheduledTask("dependent", WorkflowMixedPassV1, StatePending, 1)
	dependent.DependsOn = []string{"pending", "completed", "blocked", "input", "cancelled"}

	result := Evaluate(Input{Tasks: []Task{dependent, completed, cancelled, needsInput, blocked, pending}})
	got := taskResult(t, result, dependent.ID)
	if got.Reason != ReasonTerminalUnsatisfiedDependency {
		t.Fatalf("mixed terminal precedence = %q", got.Reason)
	}
	if want := []string{"blocked", "cancelled", "input", "pending"}; !reflect.DeepEqual(got.UnmetDependencyIDs, want) {
		t.Fatalf("unmet = %#v, want %#v", got.UnmetDependencyIDs, want)
	}

	dependent.DependsOn = []string{"pending", "blocked", "input"}
	result = Evaluate(Input{Tasks: []Task{dependent, needsInput, blocked, pending}})
	if got = taskResult(t, result, dependent.ID); got.Reason != ReasonNeedsInputDependency {
		t.Fatalf("needs-input precedence = %q", got.Reason)
	}

	dependent.DependsOn = []string{"pending", "blocked"}
	result = Evaluate(Input{Tasks: []Task{dependent, blocked, pending}})
	if got = taskResult(t, result, dependent.ID); got.Reason != ReasonBlockedDependency {
		t.Fatalf("blocked precedence = %q", got.Reason)
	}
}

func TestGraphValidationDiagnosticsFailClosed(t *testing.T) {
	validArchive := Archive{TaskID: "archived", ArchiveID: "archive-1", Disposition: StateCompleted, Verified: true, Reconciled: true}
	tests := []struct {
		name     string
		tasks    []Task
		archives []Archive
		wantCode DiagnosticCode
	}{
		{name: "missing dependency", tasks: withDependency(scheduledTask("task", WorkflowMixedPassV1, StatePending, 1), "missing"), wantCode: DiagnosticMissingDependency},
		{name: "self dependency", tasks: withDependency(scheduledTask("task", WorkflowMixedPassV1, StatePending, 1), "task"), wantCode: DiagnosticSelfDependency},
		{name: "duplicate edge", tasks: withDependency(scheduledTask("task", WorkflowMixedPassV1, StatePending, 1), "dependency", "dependency"), archives: []Archive{validArchiveFor("dependency")}, wantCode: DiagnosticDuplicateDependencyEdge},
		{name: "duplicate task id", tasks: []Task{scheduledTaskAt("task", StatePending, "tasks/a.md"), scheduledTaskAt("task", StatePending, "tasks/b.md")}, wantCode: DiagnosticDuplicateTaskID},
		{name: "active archive ambiguity", tasks: []Task{scheduledTask("archived", WorkflowMixedPassV1, StatePending, 1)}, archives: []Archive{validArchive}, wantCode: DiagnosticActiveArchiveAmbiguity},
		{name: "unverified archive", tasks: withDependency(scheduledTask("task", WorkflowMixedPassV1, StatePending, 1), "archived"), archives: []Archive{{TaskID: "archived", ArchiveID: "archive-1", Disposition: StateCompleted, Verified: false, Reconciled: true}}, wantCode: DiagnosticMalformedArchive},
		{name: "unreconciled archive", tasks: withDependency(scheduledTask("task", WorkflowMixedPassV1, StatePending, 1), "archived"), archives: []Archive{{TaskID: "archived", ArchiveID: "archive-1", Disposition: StateCompleted, Verified: true, Reconciled: false}}, wantCode: DiagnosticMalformedArchive},
		{name: "unsupported workflow", tasks: []Task{{ID: "task", Workflow: "unknown", State: StatePending, SourcePath: "tasks/task.md"}}, wantCode: DiagnosticInvalidTask},
		{name: "unsupported state", tasks: []Task{{ID: "task", Workflow: WorkflowMixedPassV1, State: StateNeedsInput, SourcePath: "tasks/task.md"}}, wantCode: DiagnosticInvalidTask},
		{name: "two node cycle", tasks: []Task{taskWithDependencies("a", "b"), taskWithDependencies("b", "a")}, wantCode: DiagnosticDependencyCycle},
		{name: "longer cycle", tasks: []Task{taskWithDependencies("a", "b"), taskWithDependencies("b", "c"), taskWithDependencies("c", "a")}, wantCode: DiagnosticDependencyCycle},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Evaluate(Input{Tasks: tt.tasks, Archives: tt.archives})
			if result.Valid() {
				t.Fatal("result unexpectedly valid")
			}
			if !hasDiagnostic(result.InvalidGraph, tt.wantCode) {
				t.Fatalf("diagnostics = %#v, want %q", result.InvalidGraph, tt.wantCode)
			}
			if result.SelectedNext != nil || len(result.Ready) != 0 {
				t.Fatalf("invalid graph selected work: %#v", result)
			}
			for _, task := range result.Tasks {
				if task.Reason != ReasonInvalidGraph {
					t.Fatalf("invalid task projection = %#v", task)
				}
			}
		})
	}
}

func TestCyclesAreDeterministicClosedPaths(t *testing.T) {
	result := Evaluate(Input{Tasks: []Task{
		taskWithDependencies("z", "y"),
		taskWithDependencies("b", "a"),
		taskWithDependencies("a", "b"),
		taskWithDependencies("y", "x"),
		taskWithDependencies("x", "z"),
	}})
	var cycles [][]string
	for _, diagnostic := range result.InvalidGraph {
		if diagnostic.Code == DiagnosticDependencyCycle {
			cycles = append(cycles, diagnostic.Cycle)
		}
	}
	want := [][]string{{"a", "b", "a"}, {"x", "z", "y", "x"}}
	if !reflect.DeepEqual(cycles, want) {
		t.Fatalf("cycles = %#v, want %#v", cycles, want)
	}
}

func TestUnambiguousCycleIsReportedBesideDuplicateIdentity(t *testing.T) {
	result := Evaluate(Input{Tasks: []Task{
		scheduledTaskAt("duplicate", StatePending, "tasks/duplicate-a.md"),
		scheduledTaskAt("duplicate", StatePending, "tasks/duplicate-b.md"),
		taskWithDependencies("a", "b"),
		taskWithDependencies("b", "a"),
	}})
	if !hasDiagnostic(result.InvalidGraph, DiagnosticDuplicateTaskID) || !hasDiagnostic(result.InvalidGraph, DiagnosticDependencyCycle) {
		t.Fatalf("diagnostics = %#v", result.InvalidGraph)
	}
}

func TestArchiveDependencySemantics(t *testing.T) {
	dependent := withDependency(scheduledTask("dependent", WorkflowMixedPassV1, StatePending, 1), "archived")[0]
	completed := validArchiveFor("archived")
	result := Evaluate(Input{Tasks: []Task{dependent}, Archives: []Archive{completed}})
	if got := taskResult(t, result, dependent.ID); got.Reason != ReasonReady {
		t.Fatalf("completed archive did not satisfy edge: %#v", got)
	}

	for _, disposition := range []State{StateCancelled, StateAbandoned, StateSuperseded} {
		t.Run(string(disposition), func(t *testing.T) {
			archive := validArchiveFor("archived")
			archive.Disposition = disposition
			archive.Reason = "operator recorded " + string(disposition)
			result := Evaluate(Input{Tasks: []Task{dependent}, Archives: []Archive{archive}})
			got := taskResult(t, result, dependent.ID)
			if got.Reason != ReasonTerminalUnsatisfiedDependency || len(result.TerminalUnsatisfied) != 1 {
				t.Fatalf("classification = %#v", got)
			}
			wantIssue := DependencyIssue{DependencyID: "archived", State: disposition, Reason: ReasonTerminalUnsatisfiedDependency, Archived: true, ArchiveID: "archive-archived", Detail: archive.Reason}
			if !reflect.DeepEqual(got.DependencyIssues, []DependencyIssue{wantIssue}) {
				t.Fatalf("issues = %#v, want %#v", got.DependencyIssues, wantIssue)
			}
		})
	}
}

func TestNonCompletedArchiveRequiresExplicitReason(t *testing.T) {
	archive := validArchiveFor("archived")
	archive.Disposition = StateCancelled
	result := Evaluate(Input{Archives: []Archive{archive}})
	if !hasDiagnostic(result.InvalidGraph, DiagnosticMalformedArchive) {
		t.Fatalf("diagnostics = %#v", result.InvalidGraph)
	}
}

func TestReadyOrderingPriorityPathAndID(t *testing.T) {
	tasks := []Task{
		unprioritizedTask("u-z", "tasks/z.md"),
		scheduledTaskAt("p-id-b", StatePending, "tasks/b.md"),
		prioritizedTask("p-low", -1, "tasks/z.md"),
		unprioritizedTask("u-a", "tasks/a.md"),
		prioritizedTask("p-id-a", 2, "tasks/b.md"),
		prioritizedTask("p-a", 2, `tasks\a.md`),
	}
	tasks[1].Priority = 2
	result := Evaluate(Input{Tasks: tasks})
	if !result.Valid() {
		t.Fatalf("invalid result: %#v", result.InvalidGraph)
	}
	want := []string{"p-low", "p-a", "p-id-a", "p-id-b", "u-a", "u-z"}
	if got := resultIDs(result.Ready); !reflect.DeepEqual(got, want) {
		t.Fatalf("ready order = %#v, want %#v", got, want)
	}
	if result.SelectedNext == nil || result.SelectedNext.TaskID != "p-low" {
		t.Fatalf("selected = %#v", result.SelectedNext)
	}
	if got := taskResult(t, result, "p-a").SourcePath; got != "tasks/a.md" {
		t.Fatalf("normalized source path = %q", got)
	}
}

func TestPriorityOrdersOnlyReadyTasks(t *testing.T) {
	running := scheduledTask("running", WorkflowMixedPassV1, StateRunning, 1)
	waitingZ := prioritizedTask("waiting-z", -100, "tasks/z.md")
	waitingZ.DependsOn = []string{running.ID}
	waitingA := unprioritizedTask("waiting-a", "tasks/a.md")
	waitingA.DependsOn = []string{running.ID}
	result := Evaluate(Input{Tasks: []Task{waitingZ, running, waitingA}})
	if got, want := resultIDs(result.Waiting), []string{"waiting-a", "waiting-z"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("waiting order = %#v, want path/id order %#v", got, want)
	}
}

func TestCrossWorkflowDependenciesAndOperatorCheckpoint(t *testing.T) {
	mixedCompleted := scheduledTask("mixed-completed", WorkflowMixedPassV1, StateCompleted, 1)
	autonomous := scheduledTask("autonomous", WorkflowAutonomousV1, StatePending, 1)
	autonomous.DependsOn = []string{mixedCompleted.ID}
	autonomousCompleted := scheduledTask("autonomous-completed", WorkflowAutonomousV1, StateCompleted, 1)
	mixed := scheduledTask("mixed", WorkflowMixedPassV1, StatePending, 1)
	mixed.DependsOn = []string{autonomousCompleted.ID}
	checkpoint := scheduledTask("license", WorkflowOperatorCheckpointV1, StateAwaitingOperator, 0)
	checkpoint.Checkpoint = awaitingCheckpointAuthority(checkpoint.ID)
	checkpointDependent := scheduledTask("asset-build", WorkflowMixedPassV1, StatePending, 0)
	checkpointDependent.DependsOn = []string{checkpoint.ID}

	result := Evaluate(Input{Tasks: []Task{checkpointDependent, checkpoint, mixed, autonomousCompleted, autonomous, mixedCompleted}})
	if got := taskResult(t, result, autonomous.ID); got.Reason != ReasonReady {
		t.Fatalf("autonomous cross-workflow readiness = %#v", got)
	}
	if got := taskResult(t, result, mixed.ID); got.Reason != ReasonReady {
		t.Fatalf("mixed cross-workflow readiness = %#v", got)
	}
	if got := taskResult(t, result, checkpoint.ID); got.Reason != ReasonAwaitingOperator {
		t.Fatalf("checkpoint = %#v", got)
	}
	got := taskResult(t, result, checkpointDependent.ID)
	if got.Reason != ReasonWaitingDependency || got.DependencyIssues[0].Reason != ReasonAwaitingOperatorDependency {
		t.Fatalf("checkpoint dependent = %#v", got)
	}
	if len(result.OperatorInput) != 1 || result.OperatorInput[0].TaskID != checkpoint.ID {
		t.Fatalf("operator input = %#v", result.OperatorInput)
	}
	for _, ready := range result.Ready {
		if ready.TaskID == checkpoint.ID {
			t.Fatal("operator checkpoint was runnable")
		}
	}
}

func TestOperatorCheckpointReceiptAuthorityAndDependencyUnlock(t *testing.T) {
	awaiting := scheduledTask("manual-acceptance", WorkflowOperatorCheckpointV1, StateAwaitingOperator, 0)
	awaiting.Checkpoint = awaitingCheckpointAuthority(awaiting.ID)
	dependent := scheduledTask("release", WorkflowMixedPassV1, StatePending, 1)
	dependent.DependsOn = []string{awaiting.ID}

	result := Evaluate(Input{Tasks: []Task{dependent, awaiting}, SelectionWorkflow: WorkflowMixedPassV1})
	if !result.Valid() || result.SelectedNext != nil {
		t.Fatalf("awaiting result = %#v", result)
	}
	if got := taskResult(t, result, awaiting.ID); got.State != StateAwaitingOperator || got.Reason != ReasonAwaitingOperator {
		t.Fatalf("awaiting checkpoint = %#v", got)
	}
	if got := taskResult(t, result, dependent.ID); got.Reason != ReasonWaitingDependency || got.DependencyIssues[0].Reason != ReasonAwaitingOperatorDependency {
		t.Fatalf("dependent = %#v", got)
	}

	completed := awaiting
	completed.State = StateCompleted
	completed.Checkpoint = completedCheckpointAuthority(completed.ID)
	result = Evaluate(Input{Tasks: []Task{dependent, completed}, SelectionWorkflow: WorkflowMixedPassV1})
	if !result.Valid() || result.SelectedNext == nil || result.SelectedNext.TaskID != dependent.ID {
		t.Fatalf("fulfilled result = %#v", result)
	}
	if got := taskResult(t, result, completed.ID); got.Reason != ReasonCompleted {
		t.Fatalf("completed checkpoint = %#v", got)
	}
	for _, ready := range result.Ready {
		if ready.TaskID == completed.ID {
			t.Fatal("completed checkpoint entered executable ready set")
		}
	}
}

func TestOperatorCheckpointInvalidAuthorityFailsGraphClosed(t *testing.T) {
	tests := []struct {
		name   string
		task   Task
		detail string
	}{
		{name: "missing authority", task: scheduledTask("checkpoint", WorkflowOperatorCheckpointV1, StateAwaitingOperator, 0), detail: "no receipt authority"},
		{name: "missing path", task: Task{ID: "checkpoint", Workflow: WorkflowOperatorCheckpointV1, State: StateAwaitingOperator, SourcePath: "tasks/checkpoint.md", Checkpoint: &CheckpointAuthority{}}, detail: "is not canonical"},
		{name: "alternate path", task: Task{ID: "checkpoint", Workflow: WorkflowOperatorCheckpointV1, State: StateAwaitingOperator, SourcePath: "tasks/checkpoint.md", Checkpoint: &CheckpointAuthority{ReceiptPath: ".agent/checkpoints/other/receipt.json"}}, detail: "is not canonical"},
		{name: "awaiting claims identity", task: Task{ID: "checkpoint", Workflow: WorkflowOperatorCheckpointV1, State: StateAwaitingOperator, SourcePath: "tasks/checkpoint.md", Checkpoint: &CheckpointAuthority{ReceiptPath: ".agent/checkpoints/checkpoint/receipt.json", ReceiptSHA256: strings.Repeat("a", 64)}}, detail: "must not claim"},
		{name: "completed malformed identity", task: Task{ID: "checkpoint", Workflow: WorkflowOperatorCheckpointV1, State: StateCompleted, SourcePath: "tasks/checkpoint.md", Checkpoint: &CheckpointAuthority{ReceiptPath: ".agent/checkpoints/checkpoint/receipt.json", ReceiptSHA256: "bad", Verified: true}}, detail: "malformed receipt identity"},
		{name: "completed unverified missing", task: Task{ID: "checkpoint", Workflow: WorkflowOperatorCheckpointV1, State: StateCompleted, SourcePath: "tasks/checkpoint.md", Checkpoint: &CheckpointAuthority{ReceiptPath: ".agent/checkpoints/checkpoint/receipt.json", ReceiptSHA256: strings.Repeat("a", 64), Detail: "receipt does not exist"}}, detail: "receipt does not exist"},
		{name: "completed changed identity", task: Task{ID: "checkpoint", Workflow: WorkflowOperatorCheckpointV1, State: StateCompleted, SourcePath: "tasks/checkpoint.md", Checkpoint: &CheckpointAuthority{ReceiptPath: ".agent/checkpoints/checkpoint/receipt.json", ReceiptSHA256: strings.Repeat("a", 64), Detail: "bound receipt identity changed"}}, detail: "identity changed"},
		{name: "mixed task claims authority", task: Task{ID: "task", Workflow: WorkflowMixedPassV1, State: StatePending, SourcePath: "tasks/task.md", Checkpoint: awaitingCheckpointAuthority("task")}, detail: "has operator checkpoint authority"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := Evaluate(Input{Tasks: []Task{test.task}})
			if result.Valid() || result.SelectedNext != nil || len(result.InvalidGraph) != 1 || result.InvalidGraph[0].Code != DiagnosticInvalidTask || !strings.Contains(result.InvalidGraph[0].Detail, test.detail) {
				t.Fatalf("invalid result = %#v, want detail %q", result, test.detail)
			}
		})
	}
}

func TestSelectionWorkflowUsesSchedulerOwnedReadyOrdering(t *testing.T) {
	autonomous := scheduledTask("autonomous", WorkflowAutonomousV1, StatePending, -10)
	mixedFirst := scheduledTask("mixed-first", WorkflowMixedPassV1, StatePending, 1)
	mixedSecond := scheduledTask("mixed-second", WorkflowMixedPassV1, StatePending, 2)
	result := Evaluate(Input{Tasks: []Task{mixedSecond, autonomous, mixedFirst}, SelectionWorkflow: WorkflowMixedPassV1})
	if result.SelectedNext == nil || result.SelectedNext.TaskID != mixedFirst.ID {
		t.Fatalf("selected = %#v, want first ready mixed task", result.SelectedNext)
	}
	if got, want := resultIDs(result.Ready), []string{"autonomous", "mixed-first", "mixed-second"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("complete ready set = %#v, want %#v", got, want)
	}
	if got, ok := result.SelectedForWorkflow(WorkflowAutonomousV1); !ok || got.TaskID != autonomous.ID {
		t.Fatalf("autonomous workflow selection = %#v found=%v", got, ok)
	}
	if got, ok := result.SelectedForWorkflow(WorkflowMixedPassV1); !ok || got.TaskID != mixedFirst.ID {
		t.Fatalf("mixed workflow selection = %#v found=%v", got, ok)
	}

	invalid := Evaluate(Input{Tasks: []Task{mixedFirst}, SelectionWorkflow: WorkflowOperatorCheckpointV1})
	if invalid.Valid() || invalid.SelectedNext != nil || !hasDiagnostic(invalid.InvalidGraph, DiagnosticInvalidSelection) {
		t.Fatalf("invalid selection workflow result = %#v", invalid)
	}
}

func TestSelectionTerminalUnsatisfiedIsWorkflowScoped(t *testing.T) {
	cancelled := scheduledTask("cancelled", WorkflowAutonomousV1, StateCancelled, 1)
	autonomous := scheduledTask("autonomous", WorkflowAutonomousV1, StatePending, 1)
	autonomous.DependsOn = []string{cancelled.ID}
	mixedRunning := scheduledTask("mixed-running", WorkflowMixedPassV1, StateRunning, 1)
	result := Evaluate(Input{Tasks: []Task{autonomous, cancelled, mixedRunning}, SelectionWorkflow: WorkflowMixedPassV1})
	if len(result.TerminalUnsatisfied) != 1 || len(result.SelectionTerminalUnsatisfied) != 0 || result.SelectedNext != nil {
		t.Fatalf("workflow-scoped terminal result = %#v", result)
	}
	if result.SelectionWorkflow != WorkflowMixedPassV1 {
		t.Fatalf("selection workflow = %q", result.SelectionWorkflow)
	}
}

func TestConflictClassificationIsSymmetricAndKeyAware(t *testing.T) {
	a := scheduledTask("a", WorkflowMixedPassV1, StatePending, 1)
	a.Conflicts = []string{"gpu"}
	b := scheduledTask("b", WorkflowMixedPassV1, StatePending, 1)
	b.Conflicts = []string{"gpu"}
	c := scheduledTask("c", WorkflowMixedPassV1, StatePending, 1)
	c.Conflicts = []string{"a"}
	result := Evaluate(Input{Tasks: []Task{c, b, a}, Occupied: []string{"a"}})
	for _, id := range []string{"a", "b", "c"} {
		if got := taskResult(t, result, id); got.Reason != ReasonConflictBlocked || !reflect.DeepEqual(got.ConflictingTaskOrKeys, []string{"a"}) {
			t.Fatalf("%s conflict = %#v", id, got)
		}
	}
	if result.SelectedNext != nil || len(result.ConflictBlocked) != 3 {
		t.Fatalf("conflict result = %#v", result)
	}

	keyResult := Evaluate(Input{Tasks: []Task{a}, Occupied: []string{"gpu"}})
	if got := taskResult(t, keyResult, "a"); got.Reason != ReasonConflictBlocked {
		t.Fatalf("key conflict = %#v", got)
	}
}

func TestNoReadyTasksIsValidAndSelectsNothing(t *testing.T) {
	running := scheduledTask("running", WorkflowMixedPassV1, StateRunning, 1)
	result := Evaluate(Input{Tasks: []Task{running}})
	if !result.Valid() || result.SelectedNext != nil || len(result.Ready) != 0 {
		t.Fatalf("result = %#v", result)
	}
	if got := taskResult(t, result, running.ID); got.Reason != ReasonRunning {
		t.Fatalf("running projection = %#v", got)
	}
}

func TestSerializedEvaluationIsDeterministic(t *testing.T) {
	a := scheduledTask("a", WorkflowMixedPassV1, StatePending, 1)
	a.DependsOn = []string{"archived", "blocked"}
	a.Conflicts = []string{"gpu", "workspace"}
	blocked := scheduledTask("blocked", WorkflowMixedPassV1, StateBlocked, 2)
	z := unprioritizedTask("z", "tasks/z.md")
	archive := validArchiveFor("archived")

	left := Evaluate(Input{Tasks: []Task{z, a, blocked}, Archives: []Archive{archive}, Occupied: []string{"workspace", "gpu"}})
	a.DependsOn = []string{"blocked", "archived"}
	a.Conflicts = []string{"workspace", "gpu"}
	right := Evaluate(Input{Tasks: []Task{blocked, a, z}, Archives: []Archive{archive}, Occupied: []string{"gpu", "workspace", "gpu"}})
	leftJSON, err := json.Marshal(left)
	if err != nil {
		t.Fatal(err)
	}
	rightJSON, err := json.Marshal(right)
	if err != nil {
		t.Fatal(err)
	}
	if string(leftJSON) != string(rightJSON) {
		t.Fatalf("nondeterministic result:\nleft  %s\nright %s", leftJSON, rightJSON)
	}
	repeatedJSON, err := json.Marshal(Evaluate(Input{Tasks: []Task{z, a, blocked}, Archives: []Archive{archive}, Occupied: []string{"workspace", "gpu"}}))
	if err != nil {
		t.Fatal(err)
	}
	if string(rightJSON) != string(repeatedJSON) {
		t.Fatalf("repeated evaluation differs:\nfirst  %s\nsecond %s", rightJSON, repeatedJSON)
	}
}

func TestSerializedInvalidDiagnosticsAreDeterministic(t *testing.T) {
	a := taskWithDependencies("a", "missing", "b")
	b := taskWithDependencies("b", "a")
	duplicateA := scheduledTaskAt("duplicate", StatePending, "tasks/duplicate-a.md")
	duplicateB := scheduledTaskAt("duplicate", StatePending, "tasks/duplicate-b.md")
	left, err := json.Marshal(Evaluate(Input{Tasks: []Task{duplicateB, a, duplicateA, b}}))
	if err != nil {
		t.Fatal(err)
	}
	a.DependsOn = []string{"b", "missing"}
	right, err := json.Marshal(Evaluate(Input{Tasks: []Task{b, duplicateA, a, duplicateB}}))
	if err != nil {
		t.Fatal(err)
	}
	if string(left) != string(right) {
		t.Fatalf("nondeterministic invalid result:\nleft  %s\nright %s", left, right)
	}
}

func scheduledTask(id string, workflow Workflow, state State, priority int) Task {
	return Task{ID: id, Workflow: workflow, State: state, SourcePath: "tasks/" + id + ".md", HasPriority: true, Priority: priority}
}

func scheduledTaskAt(id string, state State, sourcePath string) Task {
	return Task{ID: id, Workflow: WorkflowMixedPassV1, State: state, SourcePath: sourcePath, HasPriority: true, Priority: 2}
}

func prioritizedTask(id string, priority int, sourcePath string) Task {
	return Task{ID: id, Workflow: WorkflowMixedPassV1, State: StatePending, SourcePath: sourcePath, HasPriority: true, Priority: priority}
}

func unprioritizedTask(id, sourcePath string) Task {
	return Task{ID: id, Workflow: WorkflowMixedPassV1, State: StatePending, SourcePath: sourcePath}
}

func taskWithDependencies(id string, dependencies ...string) Task {
	task := scheduledTask(id, WorkflowMixedPassV1, StatePending, 1)
	task.DependsOn = dependencies
	return task
}

func withDependency(task Task, dependencies ...string) []Task {
	task.DependsOn = dependencies
	return []Task{task}
}

func validArchiveFor(taskID string) Archive {
	return Archive{TaskID: taskID, ArchiveID: "archive-" + taskID, Disposition: StateCompleted, Verified: true, Reconciled: true}
}

func awaitingCheckpointAuthority(taskID string) *CheckpointAuthority {
	return &CheckpointAuthority{ReceiptPath: ".agent/checkpoints/" + taskID + "/receipt.json"}
}

func completedCheckpointAuthority(taskID string) *CheckpointAuthority {
	return &CheckpointAuthority{ReceiptPath: ".agent/checkpoints/" + taskID + "/receipt.json", ReceiptSHA256: strings.Repeat("a", 64), Verified: true}
}

func taskResult(t *testing.T, result Result, taskID string) TaskReadiness {
	t.Helper()
	for _, task := range result.Tasks {
		if task.TaskID == taskID {
			return task
		}
	}
	t.Fatalf("task %q not found in %#v", taskID, result.Tasks)
	return TaskReadiness{}
}

func resultIDs(tasks []TaskReadiness) []string {
	result := make([]string, len(tasks))
	for i, task := range tasks {
		result[i] = task.TaskID
	}
	return result
}

func hasDiagnostic(diagnostics []Diagnostic, code DiagnosticCode) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}
