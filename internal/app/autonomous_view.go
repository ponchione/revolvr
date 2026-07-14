package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousarchive"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/autonomousview"
	"revolvr/internal/pathguard"
	"revolvr/internal/redact"
	"revolvr/internal/taskfile"
	"revolvr/internal/taskschedule"
	"revolvr/internal/taskscheduler"
)

const maxAutonomousViewArtifactBytes = 4 << 20

// ShowAutonomousTask loads one exact active task or archive selector and
// returns a read-only, deterministic operator projection.
func ShowAutonomousTask(ctx context.Context, cfg Config, rawSelector string) (autonomousview.View, error) {
	selector := strings.TrimSpace(rawSelector)
	if selector == "" {
		return autonomousview.View{}, errors.New("task show: active task id or archive selector is required")
	}
	paths, err := resolveStatePaths(cfg.WorkDir)
	if err != nil {
		return autonomousview.View{}, err
	}
	runCfg, err := LoadRunOnceConfig(paths.WorkDir, DefaultRunOnceConfig(paths.WorkDir))
	if err != nil {
		return autonomousview.View{}, err
	}
	redactor, _, err := redact.New(runCfg.SafetyDeclaration.Redaction, os.LookupEnv)
	if err != nil {
		return autonomousview.View{}, err
	}
	view, err := loadAutonomousTaskView(ctx, paths.WorkDir, selector)
	if err != nil {
		if ctx.Err() != nil {
			return autonomousview.View{}, ctx.Err()
		}
		return autonomousview.View{}, redactor.Error(err)
	}
	view, err = autonomousview.Redact(view, redactor.String)
	if err != nil {
		return autonomousview.View{}, redactor.Error(err)
	}
	return view, nil
}

func loadAutonomousTaskView(ctx context.Context, root, selector string) (autonomousview.View, error) {
	tasks, err := taskfile.List(root)
	if err != nil {
		return autonomousview.View{}, fmt.Errorf("task show: load active tasks: %w", err)
	}
	archives, err := autonomousarchive.List(root)
	if err != nil {
		return autonomousview.View{}, fmt.Errorf("task show: load archives: %w", err)
	}
	var active *taskfile.Task
	for i := range tasks {
		if tasks[i].ID == selector {
			copyValue := tasks[i]
			active = &copyValue
		}
	}
	archiveMatches := 0
	for _, entry := range archives {
		if entry.Manifest.TaskID == selector || entry.Manifest.ArchiveID == selector {
			archiveMatches++
		}
	}
	if active != nil && archiveMatches != 0 {
		return autonomousview.View{}, fmt.Errorf("task show: selector %q is ambiguous between active task %s and archived authority", selector, active.SourcePath)
	}
	if archiveMatches > 1 {
		return autonomousview.View{}, fmt.Errorf("task show: archive selector %q is ambiguous across %d archives", selector, archiveMatches)
	}
	if active == nil && archiveMatches == 0 {
		return autonomousview.View{}, fmt.Errorf("task show: %q not found as an active task or archive selector", selector)
	}
	if active != nil {
		return loadActiveView(ctx, root, *active)
	}
	return loadArchiveView(ctx, root, selector)
}

func loadActiveView(ctx context.Context, root string, task taskfile.Task) (autonomousview.View, error) {
	input := autonomousview.Input{Source: autonomousview.Source{Kind: autonomousview.SourceActive, TaskID: task.ID, Title: task.Title, TaskPath: task.SourcePath, TaskSHA256: task.SourceSHA256(), TaskByteSize: task.SourceByteSize(), Workflow: task.Workflow, TaskStatus: task.Status, StatePath: task.AutonomousStatePath}, References: []autonomousview.Reference{{Kind: "task", Path: task.SourcePath, SHA256: task.SourceSHA256(), ByteSize: task.SourceByteSize(), Detail: "Canonical active task bytes."}}}
	if task.Workflow != taskfile.WorkflowAutonomousV1 {
		input.Diagnostics = append(input.Diagnostics, autonomousview.Diagnostic{Code: "wrong_workflow", Section: "identity", Detail: "Autonomous evidence is unavailable for mixed-pass tasks.", Reference: task.SourcePath})
		input.SchedulerReadiness = "not_applicable_wrong_workflow"
		return autonomousview.Project(input)
	}
	store, err := autonomousstate.New(autonomousstate.Config{RepositoryRoot: root})
	if err != nil {
		return autonomousview.View{}, err
	}
	snapshot, found, err := store.Load(ctx, task.ID)
	if err != nil {
		return autonomousview.View{}, fmt.Errorf("task show: load canonical state: %w", err)
	}
	if !found {
		return autonomousview.View{}, fmt.Errorf("task show: canonical state %s is missing", task.AutonomousStatePath)
	}
	input.State = &snapshot.State
	input.Source.StateSHA256, input.Source.StateByteSize = snapshot.SHA256, snapshot.ByteSize
	input.References = append(input.References, autonomousview.Reference{Kind: "state", Path: snapshot.SourcePath, SHA256: snapshot.SHA256, ByteSize: snapshot.ByteSize, Detail: "Canonical active autonomous state."})
	input.SchedulerReadiness, input.SchedulerReasons, input.Diagnostics, err = projectReadiness(ctx, root, task, input.Diagnostics)
	if err != nil {
		return autonomousview.View{}, err
	}
	audits, auditErr := store.LoadCommittedAuditHistory(ctx, task.ID)
	if auditErr != nil {
		input.Diagnostics = append(input.Diagnostics, autonomousview.Diagnostic{Code: "audit_history_unavailable", Section: "audit", Detail: "Committed audit history could not be reconstructed safely.", Reference: task.AutonomousStatePath})
	} else {
		for _, item := range audits {
			input.Audits = append(input.Audits, autonomousview.AuditEvidence{Revision: item.Record.AuditRevision, RunID: item.Record.WorkerRunID, SourceRevision: item.Record.SourceRevision, ArtifactPath: item.Record.CanonicalOutput.Path, Report: item.Record.Report, Verification: item.Record.Verification})
			input.References = append(input.References, autonomousview.Reference{Kind: "audit", Path: item.Record.CanonicalOutput.Path, RunID: item.Record.WorkerRunID, SHA256: item.Record.CanonicalOutput.SHA256, ByteSize: item.Record.CanonicalOutput.ByteSize, Detail: "Canonical committed audit output."}, autonomousview.Reference{Kind: "audit_history", Path: item.SourcePath, SHA256: item.SHA256, ByteSize: item.ByteSize, Detail: "Immutable committed audit transition."})
		}
	}
	if snapshot.State.LatestDecision != nil {
		decision, decisionRaw, decisionErr := loadDecision(root, *snapshot.State.LatestDecision)
		if decisionErr != nil {
			input.Diagnostics = append(input.Diagnostics, autonomousview.Diagnostic{Code: "latest_decision_malformed", Section: "why", Detail: "The latest accepted decision payload is unavailable or malformed; current route is unknown.", Reference: snapshot.State.LatestDecision.Artifact.Reference})
			input.Decision = &autonomousview.DecisionEvidence{Reference: *snapshot.State.LatestDecision}
		} else {
			input.Decision = &autonomousview.DecisionEvidence{Reference: *snapshot.State.LatestDecision, Decision: decision, Available: true, Admitted: lifecycleAdmits(snapshot.State.Lifecycle, decision.Action)}
			input.References = append(input.References, autonomousview.Reference{Kind: "decision", Path: snapshot.State.LatestDecision.Artifact.Reference, RunID: snapshot.State.LatestDecision.RunID, SHA256: hashBytes(decisionRaw), ByteSize: len(decisionRaw), Detail: "Latest accepted supervisor decision payload."})
		}
	}
	view, err := autonomousview.Project(input)
	if err != nil {
		return autonomousview.View{}, err
	}
	// Re-read both canonical authorities after optional evidence collection so
	// the view never mixes task/state snapshots across a concurrent transition.
	currentTask, ok, err := taskfile.FindByID(root, task.ID)
	if err != nil || !ok {
		return autonomousview.View{}, errors.Join(err, errors.New("task show: active task changed or disappeared during read"))
	}
	currentState, found, err := store.Load(ctx, task.ID)
	if err != nil || !found || currentTask.SourceSHA256() != task.SourceSHA256() || currentState.SHA256 != snapshot.SHA256 || currentState.ByteSize != snapshot.ByteSize {
		return autonomousview.View{}, errors.Join(err, fmt.Errorf("task show: active snapshot changed during read (task %s -> %s, state %s/%d -> %s/%d)", task.SourceSHA256(), currentTask.SourceSHA256(), snapshot.SHA256, snapshot.ByteSize, currentState.SHA256, currentState.ByteSize))
	}
	if auditErr == nil {
		currentAudits, err := store.LoadCommittedAuditHistory(ctx, task.ID)
		if err != nil || auditHistoryIdentity(currentAudits) != auditHistoryIdentity(audits) {
			return autonomousview.View{}, errors.Join(err, errors.New("task show: committed audit history changed during read"))
		}
	}
	if input.Decision != nil && input.Decision.Available {
		currentRaw, err := readBoundedRegular(root, input.Decision.Reference.Artifact.Reference, maxAutonomousViewArtifactBytes)
		if err != nil || hashBytes(currentRaw) != findReferenceHash(input.References, "decision") {
			return autonomousview.View{}, errors.Join(err, errors.New("task show: latest decision artifact changed during read"))
		}
	}
	return view, nil
}

func loadArchiveView(ctx context.Context, root, selector string) (autonomousview.View, error) {
	if err := ctx.Err(); err != nil {
		return autonomousview.View{}, err
	}
	snapshot, err := autonomousarchive.LoadEvidence(root, selector)
	if err != nil {
		return autonomousview.View{}, err
	}
	m := snapshot.Entry.Manifest
	input := autonomousview.Input{Source: autonomousview.Source{Kind: autonomousview.SourceArchive, TaskID: m.TaskID, Title: snapshot.Task.Title, TaskPath: m.ArchivedTask.Path, TaskSHA256: m.ArchivedTask.SHA256, TaskByteSize: m.ArchivedTask.ByteSize, Workflow: m.Workflow, TaskStatus: snapshot.Task.Status, StatePath: m.State.Path, StateSHA256: m.State.SHA256, StateByteSize: m.State.ByteSize, ArchiveID: m.ArchiveID, ArchiveDisposition: string(m.Disposition), ArchivedAt: m.ArchivedAt}, State: &snapshot.State, SchedulerReadiness: "not_applicable_archive", References: []autonomousview.Reference{{Kind: "archive_manifest", Path: snapshot.Entry.ManifestPath, SHA256: hashBytes(snapshot.Entry.ManifestBytes), ByteSize: len(snapshot.Entry.ManifestBytes), Detail: "Strict archive Show manifest; full verification was not run."}, {Kind: "task", Path: m.ArchivedTask.Path, SHA256: m.ArchivedTask.SHA256, ByteSize: m.ArchivedTask.ByteSize, Detail: "Tracked archived task bytes."}, {Kind: "state", Path: m.State.Path, SHA256: m.State.SHA256, ByteSize: m.State.ByteSize, Detail: "Canonical terminal autonomous state."}}, Diagnostics: []autonomousview.Diagnostic{{Code: "archive_verification_not_run", Section: "terminal", Detail: "Strict archive Show checks passed; full archive verification was not run.", Reference: snapshot.Entry.ManifestPath}}}
	if snapshot.Frozen != nil {
		frozen := snapshot.Frozen
		input.Audits = append(input.Audits, autonomousview.AuditEvidence{RunID: frozen.Audit.RunID, SourceRevision: frozen.Audit.SourceRevision, Report: frozen.Audit.Report, Verification: frozen.Verification})
		input.Decision = &autonomousview.DecisionEvidence{Reference: frozen.DecisionReference, Decision: frozen.Decision, Available: true, Admitted: true}
		input.References = append(input.References, autonomousview.Reference{Kind: "completion_evidence", Path: m.FrozenEvidence.Path, SHA256: m.FrozenEvidence.SHA256, ByteSize: m.FrozenEvidence.ByteSize, Detail: "Frozen completion authority loaded by strict identity."})
	} else {
		input.Diagnostics = append(input.Diagnostics, autonomousview.Diagnostic{Code: "completion_evidence_not_applicable", Section: "terminal", Detail: "This archive disposition does not require AW-20 completion evidence.", Reference: snapshot.Entry.ManifestPath})
	}
	view, err := autonomousview.Project(input)
	if err != nil {
		return autonomousview.View{}, err
	}
	again, err := autonomousarchive.LoadEvidence(root, selector)
	if err != nil || !bytes.Equal(again.Entry.ManifestBytes, snapshot.Entry.ManifestBytes) || !bytes.Equal(again.StateBytes, snapshot.StateBytes) || again.Task.SourceSHA256() != snapshot.Task.SourceSHA256() {
		return autonomousview.View{}, errors.Join(err, errors.New("task show: archive snapshot changed during read"))
	}
	return view, nil
}

func projectReadiness(ctx context.Context, root string, task taskfile.Task, diagnostics []autonomousview.Diagnostic) (string, []autonomousview.WhyReason, []autonomousview.Diagnostic, error) {
	runCfg, err := LoadRunOnceConfig(root, DefaultRunOnceConfig(root))
	if err != nil {
		return "", nil, diagnostics, err
	}
	paths, err := resolveStatePaths(root)
	if err != nil {
		return "", nil, diagnostics, err
	}
	runs, closeRuns, err := openSchedulingLedger(ctx, paths)
	if err != nil {
		return "", nil, diagnostics, err
	}
	defer closeRuns()
	snapshot, err := taskschedule.Load(ctx, taskschedule.Config{
		RepositoryRoot:    root,
		SelectionWorkflow: taskscheduler.WorkflowAutonomousV1,
		Ledger:            runs,
		GitExecutable:     runCfg.GitExecutable,
		GitTimeout:        runCfg.GitTimeout,
		ForbiddenValues:   archiveSecretValues(runCfg),
	})
	if err != nil {
		return "", nil, diagnostics, fmt.Errorf("task show: load shared scheduling authority: %w", err)
	}
	return projectScheduledReadiness(task, snapshot.Result, diagnostics)
}

func projectScheduledReadiness(task taskfile.Task, result taskscheduler.Result, diagnostics []autonomousview.Diagnostic) (string, []autonomousview.WhyReason, []autonomousview.Diagnostic, error) {
	for _, diagnostic := range result.InvalidGraph {
		reference := diagnostic.SourcePath
		if reference == "" {
			reference = diagnostic.DependencyID
		}
		if reference == "" {
			reference = diagnostic.TaskID
		}
		diagnostics = append(diagnostics, autonomousview.Diagnostic{Code: string(diagnostic.Code), Section: "why", Detail: diagnostic.Detail, Reference: reference})
	}
	var readiness taskscheduler.TaskReadiness
	found := false
	for _, candidate := range result.Tasks {
		if candidate.TaskID == task.ID && candidate.SourcePath == filepath.ToSlash(task.SourcePath) {
			readiness, found = candidate, true
			break
		}
	}
	if !found {
		return "", nil, diagnostics, fmt.Errorf("task show: shared schedule omitted task %q", task.ID)
	}
	reasons := make([]autonomousview.WhyReason, 0, len(readiness.DependencyIssues)+1)
	for _, issue := range readiness.DependencyIssues {
		text := fmt.Sprintf("Dependency %s is %s (state=%s", issue.DependencyID, issue.Reason, issue.State)
		if issue.Archived {
			text += ", archive=" + issue.ArchiveID
		}
		text += ")."
		if issue.Detail != "" {
			text += " " + issue.Detail
		}
		reasons = append(reasons, autonomousview.WhyReason{Code: string(issue.Reason), Text: text})
	}
	if readiness.Reason == taskscheduler.ReasonReady {
		selected, selectedFound := result.SelectedForWorkflow(taskscheduler.WorkflowAutonomousV1)
		if selectedFound && selected.TaskID == readiness.TaskID && selected.SourcePath == readiness.SourcePath {
			reasons = append(reasons, autonomousview.WhyReason{Code: "scheduler_selected_next", Text: "Shared ready ordering selects this task as the next autonomous task."})
		} else if selectedFound {
			reasons = append(reasons, autonomousview.WhyReason{Code: "scheduler_ready_not_selected", Text: fmt.Sprintf("This task is ready, but shared ordering selects %s first.", selected.TaskID)})
		}
	} else {
		text := "Scheduler readiness is " + string(readiness.Reason) + "."
		if len(readiness.UnmetDependencyIDs) != 0 {
			text = "Scheduler readiness is " + string(readiness.Reason) + " on dependencies: " + strings.Join(readiness.UnmetDependencyIDs, ", ") + "."
		} else if len(readiness.ConflictingTaskOrKeys) != 0 {
			text = "Scheduler readiness is conflict_blocked by: " + strings.Join(readiness.ConflictingTaskOrKeys, ", ") + "."
		}
		reasons = append(reasons, autonomousview.WhyReason{Code: "scheduler_not_ready", Text: text})
	}
	return string(readiness.Reason), reasons, diagnostics, nil
}

func loadDecision(root string, reference autonomous.DecisionReference) (autonomous.SupervisorDecision, []byte, error) {
	if err := reference.Validate(); err != nil {
		return autonomous.SupervisorDecision{}, nil, err
	}
	raw, err := readBoundedRegular(root, reference.Artifact.Reference, maxAutonomousViewArtifactBytes)
	if err != nil {
		return autonomous.SupervisorDecision{}, nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var decision autonomous.SupervisorDecision
	if err := decoder.Decode(&decision); err != nil {
		return decision, nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return decision, nil, errors.New("decision artifact contains multiple JSON values")
	}
	if err := decision.Validate(); err != nil {
		return decision, nil, err
	}
	if decision.TaskID != reference.TaskID || decision.Action != reference.Action || decision.WorkerProfile != reference.WorkerProfile {
		return decision, nil, errors.New("decision artifact disagrees with canonical reference")
	}
	return decision, raw, nil
}

func readBoundedRegular(root, rel string, limit int64) ([]byte, error) {
	abs, err := pathguard.Resolve(root, filepath.FromSlash(rel))
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o022 != 0 || info.Size() > limit {
		return nil, errors.New("evidence path is not a bounded safe regular file")
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok && stat.Nlink != 1 {
		return nil, errors.New("evidence path has an unsafe hard-link count")
	}
	return os.ReadFile(abs)
}

func lifecycleAdmits(lifecycle autonomous.LifecycleState, action autonomous.Action) bool {
	switch lifecycle {
	case autonomous.LifecycleStatePlanning:
		return action == autonomous.ActionPlan
	case autonomous.LifecycleStateWorking:
		return action == autonomous.ActionImplement || action == autonomous.ActionDocument || action == autonomous.ActionSimplify
	case autonomous.LifecycleStateAuditing:
		return action == autonomous.ActionAudit
	case autonomous.LifecycleStateCorrecting:
		return action == autonomous.ActionCorrect
	case autonomous.LifecycleStateNeedsInput:
		return action == autonomous.ActionNeedsInput
	case autonomous.LifecycleStateBlocked:
		return action == autonomous.ActionBlock
	case autonomous.LifecycleStateFinalizing, autonomous.LifecycleStateCompleted:
		return action == autonomous.ActionComplete
	default:
		return false
	}
}

func hashBytes(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }

func auditHistoryIdentity(values []autonomousstate.AuditHistorySnapshot) string {
	var out strings.Builder
	for _, item := range values {
		fmt.Fprintf(&out, "%s:%s:%d\n", item.SourcePath, item.SHA256, item.ByteSize)
	}
	return out.String()
}

func findReferenceHash(values []autonomousview.Reference, kind string) string {
	for _, item := range values {
		if item.Kind == kind {
			return item.SHA256
		}
	}
	return ""
}
