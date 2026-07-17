package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"revolvr/internal/app"
	"revolvr/internal/artifactretention"
	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousarchive"
	"revolvr/internal/autonomousdaemon"
	"revolvr/internal/autonomousmetrics"
	"revolvr/internal/autonomousmigration"
	"revolvr/internal/autonomousnotification"
	"revolvr/internal/autonomousqueue"
	"revolvr/internal/autonomoustaskrun"
	"revolvr/internal/autonomousview"
	"revolvr/internal/codexexec"
	"revolvr/internal/ledger"
	"revolvr/internal/ledgerexport"
	"revolvr/internal/receipt"
	"revolvr/internal/runonce"
	"revolvr/internal/taskfile"
	"revolvr/internal/taskmodel"
	"revolvr/internal/taskscheduler"
	tuiapp "revolvr/internal/tui"
)

const defaultVersion = "dev"

type Options struct {
	Version                string
	Out                    io.Writer
	Err                    io.Writer
	WorkDir                string
	RunOnce                RunOnceFunc
	RunTaskUntilTerminal   TaskRunFunc
	RunQueue               QueueRunFunc
	RunDaemon              DaemonRunFunc
	TUIRunner              TUIRunFunc
	DoctorCommandRunner    DoctorCommandRunner
	ExecutableLookPath     ExecutableLookPath
	ExecutableInspector    app.ExecutableInspector
	CodexIdentityInspector app.CodexIdentityInspector
	PlanArtifactGC         ArtifactGCPlanFunc
	ApplyArtifactGC        ArtifactGCApplyFunc
	ResumeArtifactGC       ArtifactGCResumeFunc
	FulfillCheckpoint      CheckpointFulfillFunc
	PlanTaskMigration      MigrationPlanFunc
	ApplyTaskMigration     MigrationApplyFunc
	RecoverTask            TaskRecoveryFunc
}

type TaskRunFunc func(context.Context, app.Config, app.TaskRunInput) (autonomoustaskrun.Result, error)
type QueueRunFunc func(context.Context, app.Config, app.QueueInput) (autonomousqueue.Result, error)
type DaemonRunFunc func(context.Context, app.Config, app.DaemonInput) (autonomousdaemon.Result, error)
type ArtifactGCPlanFunc func(context.Context, app.Config, app.GCPlanInput) (artifactretention.Plan, error)
type ArtifactGCApplyFunc func(context.Context, app.Config, app.GCApplyInput) (artifactretention.ApplyResult, error)
type ArtifactGCResumeFunc func(context.Context, app.Config, string) (artifactretention.ApplyResult, error)
type CheckpointFulfillFunc func(context.Context, app.Config, app.FulfillCheckpointInput) (app.FulfillCheckpointResult, error)
type MigrationPlanFunc func(context.Context, app.Config, app.MigrationPlanInput) (autonomousmigration.Plan, error)
type MigrationApplyFunc func(context.Context, app.Config, app.MigrationPlanInput) (autonomousmigration.ApplyResult, error)
type TaskRecoveryFunc func(context.Context, app.Config, app.RecoverAutonomousTaskInput) (app.RecoverAutonomousTaskResult, error)

type RunOnceFunc = app.RunOnceRunner
type TUIRunFunc func(context.Context, app.StatusResult, tuiapp.RunOptions) error

func NewRootCommand(opts Options) *cobra.Command {
	version := opts.Version
	if version == "" {
		version = defaultVersion
	}
	if strings.TrimSpace(opts.WorkDir) == "" {
		if workDir, err := os.Getwd(); err == nil {
			opts.WorkDir = workDir
		}
	}

	root := &cobra.Command{
		Use:           "revolvr",
		Short:         "Run bounded Codex harness passes",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	root.SetVersionTemplate("revolvr {{.Version}}\n")
	root.CompletionOptions.DisableDefaultCmd = true

	if opts.Out != nil {
		root.SetOut(opts.Out)
	}
	if opts.Err != nil {
		root.SetErr(opts.Err)
	}

	root.AddCommand(
		newInitCommand(opts),
		newTaskCommand(opts),
		newCheckpointCommand(opts),
		newArchiveCommand(opts),
		newArtifactCommand(opts),
		newLedgerCommand(opts),
		newMetricsCommand(opts),
		newNotificationCommand(opts),
		newConfigCommand(opts),
		newRunCommand(opts),
		newDoctorCommand(opts),
		newStatusCommand(opts),
		newTUICommand(opts),
		newShowCommand(opts),
		newReceiptCommand(opts),
	)

	return root
}

func newCheckpointCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "checkpoint",
		Short: "Manage pre-authored operator checkpoints",
		Args:  cobra.NoArgs,
		RunE:  runHelp,
	}
	cmd.AddCommand(newCheckpointFulfillCommand(opts))
	return cmd
}

func newCheckpointFulfillCommand(opts Options) *cobra.Command {
	runner := opts.FulfillCheckpoint
	if runner == nil {
		runner = app.FulfillCheckpoint
	}
	var receiptPath, operator string
	cmd := &cobra.Command{
		Use:   "fulfill <task-id>",
		Short: "Bind accepted operator evidence to a checkpoint",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := runner(cmd.Context(), app.Config{WorkDir: opts.WorkDir}, app.FulfillCheckpointInput{
				TaskID: args[0], ReceiptPath: receiptPath, Operator: operator,
			})
			if err != nil {
				return err
			}
			state := "fulfilled"
			if result.Replayed {
				state = "already fulfilled"
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Checkpoint %s %s: receipt=%s sha256=%s\n", result.Task.ID, state, result.ReceiptPath, result.ReceiptSHA256)
			return err
		},
	}
	cmd.Flags().StringVar(&receiptPath, "receipt", "", "canonical repository-relative receipt path")
	cmd.Flags().StringVar(&operator, "operator", "", "operator identity recorded in the receipt")
	return cmd
}

func newMetricsCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{Use: "metrics", Short: "Project autonomous-loop metrics from ledger evidence", Args: cobra.NoArgs, RunE: runHelp}
	var jsonOutput bool
	var exportID string
	show := &cobra.Command{Use: "show", Short: "Show deterministic autonomous-loop metrics", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		projection, err := app.ShowMetrics(cmd.Context(), app.Config{WorkDir: opts.WorkDir}, strings.TrimSpace(exportID))
		if err != nil {
			return err
		}
		if jsonOutput {
			raw, err := autonomousmetrics.Marshal(projection)
			if err != nil {
				return err
			}
			_, err = cmd.OutOrStdout().Write(raw)
			return err
		}
		return writeMetrics(cmd.OutOrStdout(), projection)
	}}
	show.Flags().BoolVar(&jsonOutput, "json", false, "emit canonical JSON")
	show.Flags().StringVar(&exportID, "export", "", "project one verified immutable ledger export")
	cmd.AddCommand(show)
	return cmd
}

func writeMetrics(out io.Writer, p autonomousmetrics.Projection) error {
	if _, err := fmt.Fprintf(out, "Metrics schema: %s\nSource: %s %s runs=%d events=%d high-water=%d\nTask success: %d/%d terminal=%d\n", p.SchemaVersion, p.Source.Kind, p.Source.Reference, p.Source.RunCount, p.Source.EventCount, p.Source.MaxEventID, p.TaskOutcomes.SuccessNumerator, p.TaskOutcomes.SuccessDenominator, p.TaskOutcomes.Total); err != nil {
		return err
	}
	for _, count := range p.TaskOutcomes.Counts {
		if _, err := fmt.Fprintf(out, "Outcome: %s=%d\n", count.Name, count.Value); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(out, "Attempts: admitted=%d completed=%d corrections=%d tokens=%d missing_tokens=%d attempt_duration_ns=%d task_duration_ns=%d\nAudits: performed=%d clean=%d changes_required=%d blocking_findings=%d nonblocking_findings=%d\nVerification: occurrences=%d tiers=%d commands=%d passes=%d failures=%d flaky=%d reruns=%d timeouts=%d cancellations=%d missing=%d runner_errors=%d duration_ns=%d\nArchives: completed=%d cancelled=%d superseded=%d abandoned=%d latency=%d/%d_ns\nQueues: sweeps=%d selections=%d tasks=%d drained=%d configured_workers=%d peak_workers=%d parallel=%d fallbacks=%d duration_ns=%d\nOmissions: %d\n", p.Attempts.Admitted, p.Attempts.Completed, p.Attempts.CorrectionCycles, p.Usage.RecordedTokens, p.Usage.AttemptsMissingTokens, p.Usage.AttemptDurationNanoseconds, p.Usage.TaskDurationNanoseconds, p.Audits.Performed, p.Audits.Clean, p.Audits.ChangesRequired, p.Audits.BlockingFindings, p.Audits.NonblockingFindings, p.Verification.Occurrences, p.Verification.TierAttempts, p.Verification.CommandAttempts, p.Verification.OrdinaryPasses, p.Verification.OrdinaryFailures, p.Verification.FlakyClassifications, p.Verification.Reruns, p.Verification.Timeouts, p.Verification.Cancellations, p.Verification.MissingCommands, p.Verification.RunnerErrors, p.Usage.VerificationDurationNanoseconds, p.Archives.Completed, p.Archives.Cancelled, p.Archives.Superseded, p.Archives.Abandoned, p.Archives.LatencyCount, p.Archives.LatencyNanoseconds, p.Queues.Sweeps, p.Queues.Selections, p.Queues.TasksRun, p.Queues.Drained, p.Queues.MaximumConfiguredWorkers, p.Queues.PeakActiveWorkers, p.Queues.ParallelSweeps, p.Queues.SequentialFallbacks, p.Usage.QueueDurationNanoseconds, len(p.Omissions)); err != nil {
		return err
	}
	for _, omission := range p.Omissions {
		if _, err := fmt.Fprintf(out, "Omitted: %s: %s\n", omission.Code, oneLine(omission.Detail)); err != nil {
			return err
		}
	}
	return nil
}

type notificationObservation struct {
	Result autonomousnotification.Result
	Err    error
}

func collectNotifications(target *[]notificationObservation) app.NotificationObserver {
	return func(result autonomousnotification.Result, err error) {
		*target = append(*target, notificationObservation{Result: result, Err: err})
	}
}

func writeNotificationObservations(out io.Writer, values []notificationObservation) error {
	for _, value := range values {
		if value.Err != nil {
			if _, err := fmt.Fprintf(out, "Notification warning: delivery=%s event=%s stage=%s attempts=%d detail=%s error=%s\n", value.Result.DeliveryID, value.Result.Event, value.Result.Stage, value.Result.Attempts, oneLine(value.Result.Detail), oneLine(value.Err.Error())); err != nil {
				return err
			}
			continue
		}
		if _, err := fmt.Fprintf(out, "Notification: delivery=%s event=%s stage=%s attempts=%d replayed=%t\n", value.Result.DeliveryID, value.Result.Event, value.Result.Stage, value.Result.Attempts, value.Result.Replayed); err != nil {
			return err
		}
	}
	return nil
}

func newNotificationCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{Use: "notification", Short: "Inspect durable external notification deliveries", Args: cobra.NoArgs, RunE: runHelp}
	cmd.AddCommand(
		&cobra.Command{Use: "list", Short: "List durable notification deliveries", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			values, err := app.ListNotifications(app.Config{WorkDir: opts.WorkDir})
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), "DELIVERY ID\tEVENT\tSTAGE\tATTEMPTS\tUPDATED AT"); err != nil {
				return err
			}
			for _, value := range values {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%d\t%s\n", value.DeliveryID, value.Event, value.Stage, value.Attempts, cliTime(value.UpdatedAt)); err != nil {
					return err
				}
			}
			return nil
		}},
		&cobra.Command{Use: "show <delivery-id>", Short: "Show one durable notification delivery", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			value, err := app.ShowNotification(app.Config{WorkDir: opts.WorkDir}, args[0])
			if err != nil {
				return err
			}
			p, j := value.Payload, value.Journal
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Delivery ID: %s\nEvent ID: %s\nEvent: %s\nOccurred at: %s\nSubject: %s\nOutcome: %s\nStop reason: %s\nStage: %s\nAttempts: %d\nDetail: %s\nPayload SHA-256: %s\nPolicy SHA-256: %s\nEffective config: %s sha256:%s\n", p.DeliveryID, p.EventID, p.Event, cliTime(p.OccurredAt), p.SubjectKind, p.Outcome, p.StopReason, j.Stage, len(j.Attempts), oneLine(j.Detail), value.Intent.PayloadSHA256, p.HookPolicySHA256, p.EffectiveConfigSchema, p.EffectiveConfigSHA256); err != nil {
				return err
			}
			for _, attempt := range j.Attempts {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Attempt %d: exit=%d timeout=%t cancelled=%t retryable=%t stdout_truncated=%d stderr_truncated=%d error=%s\n", attempt.Number, attempt.ExitCode, attempt.TimedOut, attempt.Cancelled, attempt.Retryable, attempt.StdoutTruncatedBytes, attempt.StderrTruncatedBytes, oneLine(attempt.Error)); err != nil {
					return err
				}
			}
			return nil
		}},
	)
	return cmd
}

func newArtifactCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{Use: "artifact", Short: "Plan, apply, and inspect artifact retention", Args: cobra.NoArgs, RunE: runHelp}
	cmd.AddCommand(newArtifactGCCommand(opts))
	return cmd
}
func newArtifactGCCommand(opts Options) *cobra.Command {
	planArtifactGC := opts.PlanArtifactGC
	if planArtifactGC == nil {
		planArtifactGC = app.PlanArtifactGC
	}
	applyArtifactGC := opts.ApplyArtifactGC
	if applyArtifactGC == nil {
		applyArtifactGC = app.ApplyArtifactGC
	}
	resumeArtifactGC := opts.ResumeArtifactGC
	if resumeArtifactGC == nil {
		resumeArtifactGC = app.ResumeArtifactGC
	}

	var operationID, plannedAtRaw, planID string
	var apply, resume bool
	cmd := &cobra.Command{Use: "gc", Short: "Plan artifact GC by default; mutation requires explicit --apply", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if resume {
			if !apply {
				return errors.New("artifact gc: --resume requires --apply")
			}
			result, err := resumeArtifactGC(cmd.Context(), app.Config{WorkDir: opts.WorkDir}, operationID)
			var writeErr error
			if result.Journal.OperationID != "" {
				writeErr = writeGCResult(cmd.OutOrStdout(), result)
			}
			return errors.Join(err, writeErr)
		}
		plannedAt, err := parseRetentionUTC("planned-at", plannedAtRaw)
		if err != nil {
			return err
		}
		plan, err := planArtifactGC(cmd.Context(), app.Config{WorkDir: opts.WorkDir}, app.GCPlanInput{OperationID: operationID, FrozenAt: plannedAt})
		if err != nil {
			return err
		}
		if err := writeGCPlan(cmd.OutOrStdout(), plan); err != nil {
			return err
		}
		if !apply {
			return nil
		}
		if strings.TrimSpace(planID) == "" || planID != plan.PlanID {
			return errors.New("artifact gc: --apply requires the exact --plan-id printed by dry-run")
		}
		result, err := applyArtifactGC(cmd.Context(), app.Config{WorkDir: opts.WorkDir}, app.GCApplyInput{Plan: plan})
		return errors.Join(err, writeGCResult(cmd.OutOrStdout(), result))
	}}
	cmd.Flags().StringVar(&operationID, "operation-id", "", "stable GC operation identity")
	cmd.Flags().StringVar(&plannedAtRaw, "planned-at", "", "frozen planning time in UTC RFC3339Nano")
	cmd.Flags().BoolVar(&apply, "apply", false, "apply the exact dry-run plan")
	cmd.Flags().StringVar(&planID, "plan-id", "", "exact plan ID required with --apply")
	cmd.Flags().BoolVar(&resume, "resume", false, "resume the admitted operation journal")
	_ = cmd.MarkFlagRequired("operation-id")
	cmd.AddCommand(&cobra.Command{Use: "inspect <operation-id>", Short: "Inspect a durable GC journal", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		journal, found, err := app.InspectArtifactGC(cmd.Context(), app.Config{WorkDir: opts.WorkDir}, args[0])
		if err != nil {
			return err
		}
		if !found {
			return errors.New("artifact gc inspect: operation not found")
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "GC operation: %s\nStage: %s\nPlan ID: %s\nCompleted actions: %d/%d\nExport ID: %s\nCancelled: %t\nSequence: %d\n", journal.OperationID, journal.Stage, journal.Plan.PlanID, len(journal.CompletedPaths), journal.Plan.Totals.Compress+journal.Plan.Totals.Prune, journal.ExportID, journal.Cancelled, journal.Sequence)
		return err
	}})
	return cmd
}

func writeGCPlan(out io.Writer, plan artifactretention.Plan) error {
	if _, err := fmt.Fprintf(out, "Artifact GC dry-run\nOperation ID: %s\nPlan ID: %s\nFrozen at: %s\nPolicy: %s mutation_enabled=%t\nLedger: high_water=%d sha256=%s\nTotals: candidates=%d pinned=%d retain=%d compress=%d prune=%d remaining=%d bytes_before=%d bytes_after=%d\nRequired verified export: %t\nActions:\n", plan.OperationID, plan.PlanID, plan.FrozenAt.Format(time.RFC3339Nano), plan.PolicySHA256, plan.Policy.MutationEnabled, plan.Ledger.HighWaterEventID, plan.Ledger.SHA256, plan.Totals.Candidates, plan.Totals.Pinned, plan.Totals.Retained, plan.Totals.Compress, plan.Totals.Prune, plan.Totals.RemainingEligible, plan.Totals.BytesBefore, plan.Totals.BytesAfter, plan.RequiredExport); err != nil {
		return err
	}
	for _, action := range plan.Actions {
		if _, err := fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%d->%d\n", action.Kind, action.Class, action.Path, action.Reason, action.Source.ByteSize, action.BytesAfter); err != nil {
			return err
		}
	}
	return nil
}
func writeGCResult(out io.Writer, result artifactretention.ApplyResult) error {
	if result.Journal.OperationID == "" {
		return nil
	}
	_, err := fmt.Fprintf(out, "GC result: operation=%s stage=%s completed=%d export=%s replayed=%t resumable=%t cancelled=%t\n", result.Journal.OperationID, result.Journal.Stage, len(result.Journal.CompletedPaths), result.Journal.ExportID, result.Replayed, result.Resumable, result.Journal.Cancelled)
	return err
}

func newLedgerCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{Use: "ledger", Short: "Export and validate immutable ledger history", Args: cobra.NoArgs, RunE: runHelp}
	cmd.AddCommand(newLedgerExportCommand(opts))
	return cmd
}
func newLedgerExportCommand(opts Options) *cobra.Command {
	var operationID, exportedAtRaw, predecessor string
	var after, through int64
	cmd := &cobra.Command{Use: "export", Short: "Create a deterministic immutable ledger export", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		exportedAt, err := parseRetentionUTC("exported-at", exportedAtRaw)
		if err != nil {
			return err
		}
		result, err := app.ExportLedger(cmd.Context(), app.Config{WorkDir: opts.WorkDir}, app.LedgerExportInput{OperationID: operationID, ExportedAt: exportedAt, Bounds: ledgerexport.Bounds{AfterEventID: after, ThroughEventID: through}, PredecessorID: predecessor})
		if err != nil {
			return err
		}
		m := result.Manifest
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Ledger export: %s\nManifest: %s\nRuns: %d\nEvents: %d\nHigh water: %d\nRecords SHA-256: %s\nLegacy payloads: %d\nReplayed: %t\n", m.ExportID, result.ManifestPath, m.RunCount, m.EventCount, m.HighWaterEventID, m.Records.SHA256, m.LegacyPayloadCount, result.Replayed)
		return err
	}}
	cmd.Flags().StringVar(&operationID, "operation-id", "", "stable export operation identity")
	cmd.Flags().StringVar(&exportedAtRaw, "exported-at", "", "frozen export time in UTC RFC3339Nano")
	cmd.Flags().Int64Var(&after, "after-event-id", 0, "exclude events through this event ID")
	cmd.Flags().Int64Var(&through, "through-event-id", 0, "include through this event ID (default snapshot high-water)")
	cmd.Flags().StringVar(&predecessor, "predecessor-id", "", "exact predecessor export ID for an incremental export")
	_ = cmd.MarkFlagRequired("operation-id")
	_ = cmd.MarkFlagRequired("exported-at")
	cmd.AddCommand(&cobra.Command{Use: "verify <export-id>", Short: "Verify an immutable ledger export", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		report, err := app.VerifyLedgerExport(cmd.Context(), app.Config{WorkDir: opts.WorkDir}, args[0])
		if err != nil {
			return err
		}
		if err := writeExportChecks(cmd.OutOrStdout(), "Ledger export verification", report.ExportID, report.Passed, report.Checks); err != nil {
			return err
		}
		if !report.Passed {
			return errors.New("ledger export verify: checks failed")
		}
		return nil
	}})
	cmd.AddCommand(&cobra.Command{Use: "replay-validate <export-id>", Short: "Reconstruct and validate exported logical history", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		report, err := app.ReplayValidateLedgerExport(cmd.Context(), app.Config{WorkDir: opts.WorkDir}, args[0])
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Replay: export=%s runs=%d events=%d terminal=%d artifacts=%d passed=%t\n", report.ExportID, report.RunCount, report.EventCount, report.TerminalRuns, report.ArtifactPaths, report.Passed); err != nil {
			return err
		}
		if !report.Passed {
			return errors.New("ledger export replay-validate: checks failed")
		}
		return nil
	}})
	return cmd
}
func writeExportChecks(out io.Writer, label, id string, passed bool, checks []ledgerexport.Check) error {
	if _, err := fmt.Fprintf(out, "%s: %t\nExport ID: %s\n", label, passed, id); err != nil {
		return err
	}
	for _, check := range checks {
		status := "PASS"
		if !check.Passed {
			status = "FAIL"
		}
		if _, err := fmt.Fprintf(out, "%s\t%s\t%s\n", status, check.Name, oneLine(check.Detail)); err != nil {
			return err
		}
	}
	return nil
}
func parseRetentionUTC(label, value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, fmt.Errorf("%s must be RFC3339Nano: %w", label, err)
	}
	_, offset := parsed.Zone()
	if offset != 0 {
		return time.Time{}, fmt.Errorf("%s must be UTC", label)
	}
	return parsed.UTC(), nil
}

func newArchiveCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{Use: "archive", Short: "Archive, inspect, verify, and reopen terminal tasks", Args: cobra.NoArgs, RunE: runHelp}
	cmd.AddCommand(newArchiveListCommand(opts), newArchiveShowCommand(opts), newArchiveVerifyCommand(opts), newArchiveCreateCommand(opts), newArchiveReopenCommand(opts))
	return cmd
}

func newArchiveListCommand(opts Options) *cobra.Command {
	return &cobra.Command{Use: "list", Short: "List tracked terminal task archives", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		entries, err := app.ListArchives(cmd.Context(), app.Config{WorkDir: opts.WorkDir})
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			_, err = fmt.Fprint(cmd.OutOrStdout(), "No archives.\n")
			return err
		}
		if _, err := fmt.Fprint(cmd.OutOrStdout(), "ARCHIVE ID\tTASK ID\tDISPOSITION\tARCHIVED AT\tPATH\n"); err != nil {
			return err
		}
		for _, entry := range entries {
			m := entry.Manifest
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\t%s\n", m.ArchiveID, m.TaskID, m.Disposition, cliTime(m.ArchivedAt), entry.ManifestPath); err != nil {
				return err
			}
		}
		return nil
	}}
}

func newArchiveShowCommand(opts Options) *cobra.Command {
	return &cobra.Command{Use: "show <archive-id-or-task-id>", Short: "Show one tracked terminal task archive", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		entry, err := app.ShowArchive(cmd.Context(), app.Config{WorkDir: opts.WorkDir}, args[0])
		if err != nil {
			return err
		}
		m := entry.Manifest
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Archive ID: %s\nTask ID: %s\nDisposition: %s\nReason: %s\nProvenance: %s\nArchived at: %s\nManifest: %s\nTask: %s\nCompletion: %s\n", m.ArchiveID, m.TaskID, m.Disposition, oneLine(m.Reason), oneLine(m.Provenance), cliTime(m.ArchivedAt), entry.ManifestPath, m.ArchivedTask.Path, optionalArchiveArtifact(m.CompletionCapsule))
		return err
	}}
}

func newArchiveVerifyCommand(opts Options) *cobra.Command {
	return &cobra.Command{Use: "verify <archive-id-or-task-id>", Short: "Verify one tracked terminal task archive without mutation", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		report, err := app.VerifyArchive(cmd.Context(), app.Config{WorkDir: opts.WorkDir}, args[0])
		if err != nil {
			return err
		}
		status := "passed"
		if !report.Passed {
			status = "failed"
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Archive verification: %s\nArchive ID: %s\nTask ID: %s\nChecks:\n", status, report.ArchiveID, report.TaskID); err != nil {
			return err
		}
		failures := 0
		for _, check := range report.Checks {
			checkStatus := "PASS"
			if !check.Passed {
				checkStatus = "FAIL"
				failures++
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", checkStatus, check.Name, oneLine(check.Detail)); err != nil {
				return err
			}
		}
		if failures > 0 {
			return archiveVerificationError{ArchiveID: report.ArchiveID, FailureCount: failures}
		}
		return nil
	}}
}

func newArchiveCreateCommand(opts Options) *cobra.Command {
	var operationID, runID, disposition, reason, provenance, terminalAtRaw, archivedAtRaw string
	cmd := &cobra.Command{Use: "create <task-id>", Short: "Move one exact terminal task into its tracked archive", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		terminalAt, err := parseExplicitUTC("terminal-at", terminalAtRaw)
		if err != nil {
			return err
		}
		archivedAt, err := parseExplicitUTC("archived-at", archivedAtRaw)
		if err != nil {
			return err
		}
		result, err := app.ArchiveTask(cmd.Context(), app.Config{WorkDir: opts.WorkDir}, app.ArchiveTaskInput{TaskID: args[0], OperationID: operationID, ArchiveRunID: runID, Disposition: autonomousarchive.Disposition(disposition), Reason: reason, Provenance: provenance, TerminalAt: terminalAt, ArchivedAt: archivedAt})
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Archived task %s.\nArchive ID: %s\nDisposition: %s\nManifest: %s\nCommit: %s\nRecovery stage: %s\nReplayed: %t\n", result.Entry.Manifest.TaskID, result.Entry.Manifest.ArchiveID, result.Entry.Manifest.Disposition, result.Entry.ManifestPath, result.CommitSHA, result.Journal.Stage, result.Replayed)
		return err
	}}
	cmd.Flags().StringVar(&operationID, "operation-id", "", "unique archive operation identity")
	cmd.Flags().StringVar(&runID, "run-id", "", "optional exact archive ledger run identity")
	cmd.Flags().StringVar(&disposition, "disposition", "", "terminal disposition: completed, cancelled, superseded, or abandoned")
	cmd.Flags().StringVar(&reason, "reason", "", "exact terminal reason")
	cmd.Flags().StringVar(&provenance, "provenance", "", "trusted operator or harness authority")
	cmd.Flags().StringVar(&terminalAtRaw, "terminal-at", "", "terminal authority time in UTC RFC3339Nano")
	cmd.Flags().StringVar(&archivedAtRaw, "archived-at", "", "frozen archive routing time in UTC RFC3339Nano")
	_ = cmd.MarkFlagRequired("operation-id")
	_ = cmd.MarkFlagRequired("disposition")
	_ = cmd.MarkFlagRequired("reason")
	_ = cmd.MarkFlagRequired("provenance")
	_ = cmd.MarkFlagRequired("terminal-at")
	_ = cmd.MarkFlagRequired("archived-at")
	return cmd
}

func newArchiveReopenCommand(opts Options) *cobra.Command {
	var operationID, newTaskID, authority, reason, reopenedAtRaw string
	cmd := &cobra.Command{Use: "reopen <archive-id-or-task-id>", Short: "Create a new pending lifecycle from a verified archive", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		reopenedAt, err := parseExplicitUTC("reopened-at", reopenedAtRaw)
		if err != nil {
			return err
		}
		result, err := app.ReopenArchive(cmd.Context(), app.Config{WorkDir: opts.WorkDir}, app.ReopenArchiveInput{Selector: args[0], OperationID: operationID, NewTaskID: newTaskID, Authority: authority, Reason: reason, ReopenedAt: reopenedAt})
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Reopened archive %s.\nNew task ID: %s\nTask: %s\nState: %s\nCommit: %s\nReplayed: %t\n", result.Record.ArchiveID, result.Record.NewTaskID, result.Record.Task.Path, result.Record.State.Path, result.Record.CommitSHA, result.Replayed)
		return err
	}}
	cmd.Flags().StringVar(&operationID, "operation-id", "", "unique reopen operation identity")
	cmd.Flags().StringVar(&newTaskID, "new-task-id", "", "new active task identity")
	cmd.Flags().StringVar(&authority, "authority", "", "trusted operator or harness authority")
	cmd.Flags().StringVar(&reason, "reason", "", "reason for the new lifecycle")
	cmd.Flags().StringVar(&reopenedAtRaw, "reopened-at", "", "reopen time in UTC RFC3339Nano")
	_ = cmd.MarkFlagRequired("operation-id")
	_ = cmd.MarkFlagRequired("new-task-id")
	_ = cmd.MarkFlagRequired("authority")
	_ = cmd.MarkFlagRequired("reason")
	_ = cmd.MarkFlagRequired("reopened-at")
	return cmd
}

func parseExplicitUTC(label, value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, fmt.Errorf("archive: %s must be RFC3339Nano: %w", label, err)
	}
	_, offset := parsed.Zone()
	if offset != 0 {
		return time.Time{}, fmt.Errorf("archive: %s must be UTC", label)
	}
	return parsed.UTC(), nil
}

func optionalArchiveArtifact(value *autonomousarchive.Artifact) string {
	if value == nil {
		return "omitted"
	}
	return value.Path
}

func newInitCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize revolvr state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			paths, err := resolveStatePaths(opts.WorkDir)
			if err != nil {
				return err
			}
			if err := initializeState(cmd.Context(), paths); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(),
				"Initialized revolvr state:\nState: %s\nTask files: %s\nLedger: %s\nRuns: %s\nReceipts: %s\nLocks: %s\n",
				paths.StateDir,
				taskfile.TasksDir,
				paths.LedgerDBPath,
				paths.RunsDir,
				paths.ReceiptsDir,
				paths.LocksDir,
			)
			return err
		},
	}
}

func newTaskCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "task",
		Short: "Manage tasks",
		Args:  cobra.NoArgs,
		RunE:  runHelp,
	}
	cmd.AddCommand(
		newTaskAddCommand(opts),
		newTaskImportCommand(opts),
		newTaskMigrateCommand(opts),
		newTaskListCommand(opts),
		newTaskShowCommand(opts),
		newTaskWhyCommand(opts),
		newTaskRecoverCommand(opts),
		newTaskRetryCommand(opts),
		newTaskUnblockCommand(opts),
	)
	return cmd
}

func newTaskRecoverCommand(opts Options) *cobra.Command {
	recoverTask := opts.RecoverTask
	if recoverTask == nil {
		recoverTask = app.RecoverAutonomousTask
	}
	var operationID, confirmOperation string
	var reconcile bool
	cmd := &cobra.Command{
		Use:   "recover <task-id>",
		Short: "Inspect or explicitly reconcile an autonomous task operation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !reconcile && cmd.Flags().Changed("confirm-operation") {
				return errors.New("task recovery: --confirm-operation requires --reconcile")
			}
			if reconcile && confirmOperation != operationID {
				return errors.New("task recovery: --confirm-operation must exactly match --operation-id")
			}
			result, err := recoverTask(cmd.Context(), app.Config{WorkDir: opts.WorkDir}, app.RecoverAutonomousTaskInput{
				TaskID: args[0], OperationID: operationID, Reconcile: reconcile, ConfirmOperation: confirmOperation,
			})
			if writeErr := writeTaskRecovery(cmd.OutOrStdout(), result); writeErr != nil {
				return errors.Join(err, writeErr)
			}
			return err
		},
	}
	cmd.Flags().StringVar(&operationID, "operation-id", "", "exact old autonomous task-operation identity")
	cmd.Flags().BoolVar(&reconcile, "reconcile", false, "create a new operation only after every authority agrees")
	cmd.Flags().StringVar(&confirmOperation, "confirm-operation", "", "repeat the exact old operation identity for reconciliation")
	_ = cmd.MarkFlagRequired("operation-id")
	return cmd
}

func writeTaskRecovery(out io.Writer, result app.RecoverAutonomousTaskResult) error {
	if result.SchemaVersion == "" {
		return nil
	}
	mode := "read-only"
	if result.Reconciled {
		mode = "reconciled"
	}
	if _, err := fmt.Fprintf(out, "Autonomous task recovery (%s)\nTask: %s\nOperation: %s\nStop reason: %s\nAuthority SHA-256: %s\nReady: %t\nReconcile eligible: %t\n", mode, result.TaskID, result.OperationID, result.StopReason, result.AuthoritySHA256, result.Ready, result.ReconcileEligible); err != nil {
		return err
	}
	if _, err := fmt.Fprint(out, "AUTHORITY\tSTATUS\tDETAIL\n"); err != nil {
		return err
	}
	for _, check := range result.Checks {
		status := "FAIL"
		if check.Passed {
			status = "PASS"
		}
		if _, err := fmt.Fprintf(out, "%s\t%s\t%s\n", check.Name, status, check.Detail); err != nil {
			return err
		}
	}
	if result.Reconciled {
		disposition := "created"
		if result.Replayed {
			disposition = "replayed"
		}
		_, err := fmt.Fprintf(out, "New operation: %s (%s)\nOld operation: %s (unchanged)\n", result.NewOperationID, disposition, result.OperationID)
		return err
	}
	return nil
}

func newTaskMigrateCommand(opts Options) *cobra.Command {
	planner := opts.PlanTaskMigration
	if planner == nil {
		planner = app.PlanTaskMigration
	}
	applier := opts.ApplyTaskMigration
	if applier == nil {
		applier = app.ApplyTaskMigration
	}
	var target string
	var all, dryRun bool
	cmd := &cobra.Command{
		Use:   "migrate [task-id...]",
		Short: "Migrate mixed-pass tasks to autonomous-v1",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			input := app.MigrationPlanInput{
				TargetWorkflow: target, TaskIDs: append([]string(nil), args...), All: all, DryRun: dryRun,
			}
			if dryRun {
				plan, err := planner(cmd.Context(), app.Config{WorkDir: opts.WorkDir}, input)
				if err != nil {
					return err
				}
				return writeMigrationPlan(cmd.OutOrStdout(), plan)
			}
			result, err := applier(cmd.Context(), app.Config{WorkDir: opts.WorkDir}, input)
			if err != nil {
				return err
			}
			return writeMigrationResult(cmd.OutOrStdout(), result)
		},
	}
	cmd.Flags().StringVar(&target, "to", "", "target workflow (autonomous-v1)")
	cmd.Flags().BoolVar(&all, "all", false, "migrate every active mixed-pass-v1 task as one batch")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show the deterministic plan without writing files")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}

func writeMigrationResult(out io.Writer, result autonomousmigration.ApplyResult) error {
	disposition := "applied"
	if result.Replayed {
		disposition = "replayed"
	}
	_, err := fmt.Fprintf(out, "Autonomous migration %s: %d task(s).\nOperation: %s\nStage: %s\n", disposition, len(result.Plan.Entries), result.OperationID, result.Stage)
	return err
}

func writeMigrationPlan(out io.Writer, plan autonomousmigration.Plan) error {
	mode := "plan-only"
	if plan.DryRun {
		mode = "dry-run"
	}
	if _, err := fmt.Fprintf(out, "Autonomous migration %s: %d task(s); no files written.\nSchema: %s\nTarget: %s\n", mode, len(plan.Entries), plan.SchemaVersion, plan.TargetWorkflow); err != nil {
		return err
	}
	if _, err := fmt.Fprint(out, "TASK ID\tSOURCE\tSOURCE SHA-256\tPROJECTED SHA-256\tSTATE PATH\tSTATE SHA-256\n"); err != nil {
		return err
	}
	for _, entry := range plan.Entries {
		if _, err := fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%s\t%s\n", entry.TaskID, entry.SourcePath, entry.SourceSHA256, entry.ProjectedSHA256, entry.AutonomousStatePath, entry.StateSHA256); err != nil {
			return err
		}
	}
	return nil
}

func newTaskShowCommand(opts Options) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "show <active-task-id-or-archive-selector>",
		Short: "Show autonomous task evidence without mutating it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			view, err := app.ShowAutonomousTask(cmd.Context(), app.Config{WorkDir: opts.WorkDir}, args[0])
			if err != nil {
				return err
			}
			if jsonOutput {
				raw, err := autonomousview.Marshal(view)
				if err != nil {
					return err
				}
				_, err = cmd.OutOrStdout().Write(raw)
				return err
			}
			return writeAutonomousTaskView(cmd.OutOrStdout(), view)
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "emit the validated deterministic projection as JSON")
	return cmd
}

func newTaskWhyCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "why <active-task-id-or-archive-selector>",
		Short: "Explain autonomous routing and readiness evidence",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			view, err := app.ShowAutonomousTask(cmd.Context(), app.Config{WorkDir: opts.WorkDir}, args[0])
			if err != nil {
				return err
			}
			return writeAutonomousWhy(cmd.OutOrStdout(), view)
		},
	}
}

func writeAutonomousTaskView(out io.Writer, view autonomousview.View) error {
	write := func(format string, args ...any) error { _, err := fmt.Fprintf(out, format, args...); return err }
	if err := write("Autonomous task\nSource: %s\nTask ID: %s\nTitle: %s\nTask path: %s\nTask identity: sha256:%s bytes:%d\nWorkflow: %s\nTask status: %s\nLifecycle: %s\nState: %s sha256:%s bytes:%d schema:%s\n", view.Identity.SourceKind, view.Identity.TaskID, displayText(view.Identity.Title), view.Identity.TaskPath, displayText(view.Identity.TaskSHA256), view.Identity.TaskByteSize, view.Identity.Workflow, view.Identity.TaskStatus, displayText(view.Identity.Lifecycle), displayText(view.Identity.StatePath), displayText(view.Identity.StateSHA256), view.Identity.StateByteSize, displayText(view.Identity.StateSchema)); err != nil {
		return err
	}
	if view.Identity.SourceKind == autonomousview.SourceArchive {
		if err := write("Archive: %s disposition:%s verification:unverified\n", view.Identity.ArchiveID, view.Identity.ArchiveDisposition); err != nil {
			return err
		}
	}
	if err := write("Summary\nPhase: %s\nPlan progress: %d/%d\nAcceptance progress: %d/%d\nOpen findings: blocking=%d non_blocking=%d\nAttempts: total=%d consecutive_failures=%d\n", displayText(view.Summary.Phase), view.Summary.Plan.Completed, view.Summary.Plan.Total, view.Summary.Acceptance.Completed, view.Summary.Acceptance.Total, view.Summary.OpenBlockingFindings, view.Summary.OpenNonBlockingFindings, view.Summary.TotalAttempts, view.Summary.ConsecutiveFailures); err != nil {
		return err
	}
	if err := writeAutonomousWhy(out, view); err != nil {
		return err
	}
	if err := write("Plan\n"); err != nil {
		return err
	}
	if view.Plan == nil {
		if err := write("none\n"); err != nil {
			return err
		}
	} else {
		if err := write("ID: %s revision:%d supersedes:%s completed:%t\n", view.Plan.ID, view.Plan.Revision, displayText(view.Plan.SupersedesPlanID), view.Plan.Completed); err != nil {
			return err
		}
		for i, step := range view.Plan.Steps {
			if err := write("%d. [%s] %s: %s", i+1, step.Status, step.ID, oneLine(step.Description)); err != nil {
				return err
			}
			if step.Rationale != "" {
				if err := write(" | rationale: %s", oneLine(step.Rationale)); err != nil {
					return err
				}
			}
			if err := write("\n"); err != nil {
				return err
			}
			if err := writeEvidence(out, step.Evidence, "   evidence"); err != nil {
				return err
			}
		}
	}
	if err := write("Acceptance\n"); err != nil {
		return err
	}
	if len(view.Acceptance) == 0 {
		if err := write("none\n"); err != nil {
			return err
		}
	}
	for _, item := range view.Acceptance {
		if err := write("[%s] %s: %s", item.Status, item.ID, oneLine(item.Description)); err != nil {
			return err
		}
		if item.Rationale != "" {
			if err := write(" | rationale: %s", oneLine(item.Rationale)); err != nil {
				return err
			}
		}
		if err := write("\n"); err != nil {
			return err
		}
		if err := writeEvidence(out, item.Evidence, "  evidence"); err != nil {
			return err
		}
	}
	if err := write("Findings\n"); err != nil {
		return err
	}
	if len(view.Findings) == 0 {
		if err := write("none\n"); err != nil {
			return err
		}
	}
	for _, finding := range view.Findings {
		if err := write("[%s/%s] %s: %s\n  correction: %s\n  introduced: audit_revision=%d run=%s\n  current: audit_revision=%d run=%s", finding.Status, finding.Significance, finding.ID, oneLine(finding.Summary), oneLine(finding.RequiredCorrection), finding.IntroducedBy.Revision, displayText(finding.IntroducedBy.RunID), finding.CurrentAudit.Revision, displayText(finding.CurrentAudit.RunID)); err != nil {
			return err
		}
		if finding.ResolutionRationale != "" {
			if err := write("\n  resolution rationale: %s", oneLine(finding.ResolutionRationale)); err != nil {
				return err
			}
		}
		if finding.SupersedingFindingID != "" {
			if err := write("\n  superseded by: %s", finding.SupersedingFindingID); err != nil {
				return err
			}
		}
		if err := write("\n"); err != nil {
			return err
		}
	}
	if err := write("Attempts and budgets\nTotal: %d\nConsecutive failures: %d\n", view.Attempts.Total, view.Attempts.ConsecutiveFailures); err != nil {
		return err
	}
	if len(view.Attempts.PerAction) == 0 {
		if err := write("Per action: none\n"); err != nil {
			return err
		}
	} else {
		for _, item := range view.Attempts.PerAction {
			if err := write("Per action: %s=%d\n", item.Action, item.Attempts); err != nil {
				return err
			}
		}
	}
	for _, budget := range view.Attempts.Budgets {
		if budget.Mode == "limited" {
			if err := write("Budget %s: limited limit=%d consumed=%d remaining=%d exhausted=%t unit=%s\n", budget.Name, budget.Limit, budget.Consumed, budget.Remaining, budget.Exhausted, budget.Unit); err != nil {
				return err
			}
		} else {
			if err := write("Budget %s: %s consumed=%d unit=%s\n", budget.Name, displayText(budget.Mode), budget.Consumed, budget.Unit); err != nil {
				return err
			}
		}
	}
	if len(view.Attempts.Stops) == 0 {
		if err := write("Stops: none\n"); err != nil {
			return err
		}
	} else {
		if err := write("Stops: %s\n", strings.Join(view.Attempts.Stops, ",")); err != nil {
			return err
		}
	}
	for _, event := range view.Attempts.Events {
		if err := write("Attempt %d: %s %s action=%s outcome=%s run=%s occurrence=%s at=%s\n", event.Sequence, event.AttemptID, event.Kind, event.Action, displayText(event.Outcome), displayText(event.RunID), displayText(event.OccurrenceID), event.CreatedAt.UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	if err := write("Operator input\nState: %s\n", view.Input.State); err != nil {
		return err
	}
	if view.Input.QuestionID != "" {
		if err := write("Question: %s revision=%d sha256=%s\n%s\nBlocking reason: %s\n", view.Input.QuestionID, view.Input.Revision, view.Input.ContentSHA256, oneLine(view.Input.Question), oneLine(view.Input.BlockingReason)); err != nil {
			return err
		}
		for _, option := range view.Input.Options {
			if err := write("Option %s: %s\n", option.ID, oneLine(option.Meaning)); err != nil {
				return err
			}
		}
		if err := write("Recommendation: %s (%s)\n", view.Input.RecommendationOption, oneLine(view.Input.RecommendationRationale)); err != nil {
			return err
		}
	}
	archivedAt := "none"
	if !view.Terminal.ArchivedAt.IsZero() {
		archivedAt = view.Terminal.ArchivedAt.UTC().Format(time.RFC3339Nano)
	}
	if err := write("Verification\nState: %s\nRun: %s occurrence:%s source:%s status:%s purpose:%s final_gate:%s\nAudit\nState: %s\nRevision: %d run:%s source:%s disposition:%s findings:%d artifact:%s\nWorkspace\nState: %s\nID: %s status:%s branch:%s source:%s checkpoint:%d/%s\nTerminal/archive\nState: %s\nReason: %s\nFinalization stage: %s\nArchive ID: %s disposition:%s archived_at:%s verified_now:%t\n", view.Verification.State, displayText(view.Verification.RunID), displayText(view.Verification.OccurrenceID), displayText(view.Verification.SourceRevision), displayText(view.Verification.Status), displayText(view.Verification.Purpose), displayText(view.Verification.FinalGate), view.Audit.State, view.Audit.Revision, displayText(view.Audit.RunID), displayText(view.Audit.SourceRevision), displayText(view.Audit.Disposition), view.Audit.FindingCount, displayText(view.Audit.ArtifactPath), view.Workspace.State, displayText(view.Workspace.WorkspaceID), displayText(view.Workspace.Status), displayText(view.Workspace.BranchRef), displayText(view.Workspace.SourceRevision), view.Workspace.CheckpointSequence, displayText(view.Workspace.CheckpointCommit), view.Terminal.State, displayText(view.Terminal.Reason), displayText(view.Terminal.FinalizationStage), displayText(view.Terminal.ArchiveID), displayText(view.Terminal.Disposition), archivedAt, view.Terminal.VerifiedNow); err != nil {
		return err
	}
	if err := write("Provenance and raw references\n"); err != nil {
		return err
	}
	if len(view.Provenance.References) == 0 {
		if err := write("none\n"); err != nil {
			return err
		}
	}
	for _, ref := range view.Provenance.References {
		if err := write("- %s path=%s run=%s sha256=%s bytes=%d | %s\n", ref.Kind, displayText(ref.Path), displayText(ref.RunID), displayText(ref.SHA256), ref.ByteSize, oneLine(ref.Detail)); err != nil {
			return err
		}
	}
	if err := write("Diagnostics\n"); err != nil {
		return err
	}
	if len(view.Diagnostics) == 0 {
		return write("none\n")
	}
	for _, item := range view.Diagnostics {
		if err := write("- %s section=%s reference=%s | %s\n", item.Code, item.Section, displayText(item.Reference), oneLine(item.Detail)); err != nil {
			return err
		}
	}
	return nil
}

func writeAutonomousWhy(out io.Writer, view autonomousview.View) error {
	if _, err := fmt.Fprintf(out, "Why and routing\nLatest decision: %s\nCurrently admitted action: %s\nScheduler readiness: %s\nNext supervisor action: %s\n", view.Why.LatestDecision, view.Why.CurrentlyAdmittedAction, view.Why.SchedulerReadiness, view.Why.NextSupervisorAction); err != nil {
		return err
	}
	if len(view.Why.Reasons) == 0 {
		_, err := fmt.Fprint(out, "Reasons: none\n")
		return err
	}
	for _, reason := range view.Why.Reasons {
		if _, err := fmt.Fprintf(out, "- %s: %s\n", reason.Code, oneLine(reason.Text)); err != nil {
			return err
		}
		if err := writeEvidence(out, reason.Evidence, "  evidence"); err != nil {
			return err
		}
	}
	return nil
}

func writeEvidence(out io.Writer, values []autonomous.EvidenceReference, prefix string) error {
	for _, item := range values {
		if _, err := fmt.Fprintf(out, "%s: %s %s | %s\n", prefix, item.Kind, item.Reference, oneLine(item.Detail)); err != nil {
			return err
		}
	}
	return nil
}

func displayText(value string) string {
	if strings.TrimSpace(value) == "" {
		return "none"
	}
	return oneLine(value)
}

func newTaskAddCommand(opts Options) *cobra.Command {
	var summary string
	cmd := &cobra.Command{
		Use:   "add <task text>",
		Short: "Add a task",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskText := strings.TrimSpace(strings.Join(args, " "))
			summaryText := strings.TrimSpace(summary)
			task, err := app.AddTask(cmd.Context(), app.Config{WorkDir: opts.WorkDir}, app.AddTaskInput{
				Task:    taskText,
				Summary: summaryText,
			})
			if err != nil {
				return err
			}
			if summaryText != "" {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Added task %s: %s (summary: %s)\n", task.ID, taskText, summaryText)
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Added task %s: %s\n", task.ID, taskText)
			return err
		},
	}
	cmd.Flags().StringVar(&summary, "summary", "", "short task summary")
	return cmd
}

func newTaskImportCommand(opts Options) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "import <path>",
		Short: "Import tasks from a Markdown file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := strings.TrimSpace(args[0])
			if path == "" {
				return fmt.Errorf("task import: path is required")
			}

			markdown, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("task import: read %s: %w", path, err)
			}

			result, err := app.ImportTasksFromMarkdown(cmd.Context(), app.Config{WorkDir: opts.WorkDir}, app.ImportTasksFromMarkdownInput{
				Markdown: markdown,
				DryRun:   dryRun,
			})
			if err != nil {
				return err
			}
			if result.DryRun {
				return writeTaskImportDryRun(cmd.OutOrStdout(), result.Tasks)
			}
			return writeTaskImportCreated(cmd.OutOrStdout(), result.Tasks)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print tasks without creating them")
	return cmd
}

func newTaskListCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List tasks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			tasks, err := app.ListTasks(cmd.Context(), app.Config{WorkDir: opts.WorkDir})
			if err != nil {
				return err
			}
			if len(tasks) == 0 {
				_, err = fmt.Fprint(cmd.OutOrStdout(), "No tasks.\n")
				return err
			}
			if _, err := fmt.Fprint(cmd.OutOrStdout(), "ID\tSTATUS\tWORKFLOW\tPHASE\tPROFILE\tNEXT\tSELECTED\tREADINESS\tWAITING_ON\tCONFLICT_BLOCKERS\tDIAGNOSTICS\tDEPENDS_ON\tTAGS\tCONFLICTS\tPARENT\tTASK\tSUMMARY\tCHECKPOINT\tCHECKPOINT_RECEIPT\n"); err != nil {
				return err
			}
			for _, task := range tasks {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					task.ID,
					task.Status,
					task.Workflow,
					task.Phase,
					task.RunProfile,
					task.NextState,
					taskSelectedWorkflows(task),
					task.ReadinessReason,
					strings.Join(task.WaitingDependencyIDs, ","),
					strings.Join(task.ConflictBlockers, ","),
					taskSchedulingDiagnostics(task),
					strings.Join(task.DependsOn, ","),
					strings.Join(task.Tags, ","),
					strings.Join(task.Conflicts, ","),
					task.ParentTaskID,
					oneLine(task.Task),
					oneLine(task.Summary),
					task.CheckpointState,
					task.CheckpointReceiptPath,
				); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func writeTaskImportDryRun(out io.Writer, tasks []app.ImportedTask) error {
	if _, err := fmt.Fprintf(out, "Dry run: %d task(s) would be imported.\n", len(tasks)); err != nil {
		return err
	}
	for i, task := range tasks {
		if _, err := fmt.Fprintf(out, "%d. %s\n", i+1, importTaskDescription(task)); err != nil {
			return err
		}
	}
	return nil
}

func writeTaskImportCreated(out io.Writer, tasks []app.ImportedTask) error {
	if _, err := fmt.Fprintf(out, "Imported %d task(s).\n", len(tasks)); err != nil {
		return err
	}
	for i, task := range tasks {
		if _, err := fmt.Fprintf(out, "%d. %s\n", i+1, task.ID); err != nil {
			return err
		}
	}
	return nil
}

func importTaskDescription(task app.ImportedTask) string {
	taskText := oneLine(task.Task)
	summary := oneLine(task.Summary)
	if summary == "" {
		return taskText
	}
	if taskText == "" {
		return summary
	}
	return fmt.Sprintf("%s - %s", summary, taskText)
}

func newTaskUnblockCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "unblock <task-id>",
		Short: "Make a blocked task pending again",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return retryBlockedTask(cmd, opts, args[0], "task unblock", "Unblocked")
		},
	}
}

func newTaskRetryCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "retry <task-id>",
		Short: "Retry a blocked task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return retryBlockedTask(cmd, opts, args[0], "task retry", "Retried")
		},
	}
}

func retryBlockedTask(cmd *cobra.Command, opts Options, rawTaskID string, operation string, successVerb string) error {
	var (
		task taskmodel.Task
		err  error
	)
	switch operation {
	case "task retry":
		task, err = app.RetryTask(cmd.Context(), app.Config{WorkDir: opts.WorkDir}, rawTaskID)
	case "task unblock":
		task, err = app.UnblockTask(cmd.Context(), app.Config{WorkDir: opts.WorkDir}, rawTaskID)
	default:
		err = fmt.Errorf("%s: unsupported task operation", operation)
	}
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s task %s.\n", successVerb, task.ID)
	return err
}

func newConfigCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect run configuration",
		Args:  cobra.NoArgs,
		RunE:  runHelp,
	}
	cmd.AddCommand(newConfigCheckCommand(opts))
	return cmd
}

func newConfigCheckCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Validate and show effective run configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			result, err := checkRunConfig(opts.WorkDir)
			if err != nil {
				return err
			}
			return writeConfigCheck(cmd.OutOrStdout(), result)
		},
	}
}

func newRunCommand(opts Options) *cobra.Command {
	runOnce := opts.RunOnce
	if runOnce == nil {
		runOnce = runonce.Run
	}
	var once bool
	var maxPasses int
	var untilTerminal, queueMode, daemonMode bool
	var taskID, operationID string
	var maxCycles, maxTasks, maxSweeps int64
	var maximumWorkers int
	var daemonPoll, daemonDebounce time.Duration
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run one harness pass",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			selected := 0
			if once {
				selected++
			}
			if cmd.Flags().Changed("max-passes") {
				selected++
			}
			if untilTerminal {
				selected++
			}
			if queueMode {
				selected++
			}
			if daemonMode {
				selected++
			}
			if selected > 1 {
				return errors.New("run: --once, --max-passes, --until-terminal, --queue, and --daemon are mutually exclusive")
			}
			autonomousMode := untilTerminal || queueMode || daemonMode
			if !autonomousMode && (cmd.Flags().Changed("task") || cmd.Flags().Changed("operation-id") || cmd.Flags().Changed("max-cycles") || cmd.Flags().Changed("max-tasks") || cmd.Flags().Changed("workers") || cmd.Flags().Changed("max-sweeps") || cmd.Flags().Changed("daemon-poll") || cmd.Flags().Changed("daemon-debounce")) {
				return errors.New("run: autonomous operation and bound flags require --until-terminal, --queue, or --daemon")
			}
			if !untilTerminal && cmd.Flags().Changed("task") {
				return errors.New("run: --task requires --until-terminal")
			}
			if !daemonMode && (cmd.Flags().Changed("max-sweeps") || cmd.Flags().Changed("daemon-poll") || cmd.Flags().Changed("daemon-debounce")) {
				return errors.New("run: daemon timing and sweep flags require --daemon")
			}
			if untilTerminal && cmd.Flags().Changed("max-tasks") {
				return errors.New("run: --max-tasks requires --queue or --daemon")
			}
			if untilTerminal && cmd.Flags().Changed("workers") {
				return errors.New("run: --workers requires --queue or --daemon")
			}
			if cmd.Flags().Changed("workers") && (maximumWorkers <= 0 || maximumWorkers > autonomousqueue.MaximumWorkerLimit) {
				return fmt.Errorf("run: --workers must be between 1 and %d", autonomousqueue.MaximumWorkerLimit)
			}
			if untilTerminal {
				if maxCycles <= 0 {
					return errors.New("run: --max-cycles must be positive")
				}
				runner := opts.RunTaskUntilTerminal
				if runner == nil {
					runner = app.RunTaskUntilTerminal
				}
				var notifications []notificationObservation
				result, err := runner(cmd.Context(), app.Config{WorkDir: opts.WorkDir}, app.TaskRunInput{OperationID: operationID, TaskID: taskID, MaxCycles: maxCycles, Notification: collectNotifications(&notifications)})
				if result.StopReason != "" {
					if writeErr := writeTaskRunSummary(cmd.OutOrStdout(), result, maxCycles); writeErr != nil {
						return writeErr
					}
				}
				if writeErr := writeNotificationObservations(cmd.OutOrStdout(), notifications); writeErr != nil {
					return writeErr
				}
				return err
			}
			if queueMode {
				if maxCycles <= 0 || maxTasks <= 0 {
					return errors.New("run: --max-cycles and --max-tasks must be positive")
				}
				runner := opts.RunQueue
				if runner == nil {
					runner = app.RunQueue
				}
				var notifications []notificationObservation
				result, err := runner(cmd.Context(), app.Config{WorkDir: opts.WorkDir}, app.QueueInput{OperationID: operationID, MaxTasks: maxTasks, MaxCycles: maxCycles, MaximumWorkers: maximumWorkers, Notification: collectNotifications(&notifications)})
				if result.StopReason != "" {
					if writeErr := writeQueueSummary(cmd.OutOrStdout(), result); writeErr != nil {
						return writeErr
					}
				}
				if writeErr := writeNotificationObservations(cmd.OutOrStdout(), notifications); writeErr != nil {
					return writeErr
				}
				return err
			}
			if daemonMode {
				if maxCycles <= 0 || maxTasks <= 0 || maxSweeps <= 0 || daemonPoll <= 0 || daemonDebounce <= 0 {
					return errors.New("run: daemon cycles, tasks, sweeps, poll, and debounce bounds must be positive")
				}
				runner := opts.RunDaemon
				if runner == nil {
					runner = app.RunDaemon
				}
				var notifications []notificationObservation
				result, err := runner(cmd.Context(), app.Config{WorkDir: opts.WorkDir}, app.DaemonInput{OperationID: operationID, MaxTasks: maxTasks, MaxCycles: maxCycles, MaximumWorkers: maximumWorkers, MaxSweeps: maxSweeps, Poll: daemonPoll, Debounce: daemonDebounce, Notification: collectNotifications(&notifications)})
				if result.StopReason != "" {
					if writeErr := writeDaemonSummary(cmd.OutOrStdout(), result); writeErr != nil {
						return writeErr
					}
				}
				if writeErr := writeNotificationObservations(cmd.OutOrStdout(), notifications); writeErr != nil {
					return writeErr
				}
				return err
			}
			if once {
				return runSinglePass(cmd, opts.WorkDir, runOnce)
			}
			if cmd.Flags().Changed("max-passes") {
				return runBoundedLoop(cmd, opts.WorkDir, runOnce, maxPasses)
			}
			return runSinglePass(cmd, opts.WorkDir, runOnce)
		},
	}
	cmd.Flags().BoolVar(&once, "once", false, "run one selected task (the default mode)")
	cmd.Flags().IntVar(&maxPasses, "max-passes", 0, "run up to N fresh passes")
	cmd.Flags().BoolVar(&untilTerminal, "until-terminal", false, "run one pinned autonomous task until a terminal stop")
	cmd.Flags().BoolVar(&queueMode, "queue", false, "run ready autonomous tasks until exhausted")
	cmd.Flags().BoolVar(&daemonMode, "daemon", false, "watch readiness authority and run bounded autonomous queue sweeps")
	cmd.Flags().StringVar(&taskID, "task", "", "exact autonomous task ID (default selects once)")
	cmd.Flags().StringVar(&operationID, "operation-id", "", "durable autonomous operation ID to start or resume")
	cmd.Flags().Int64Var(&maxCycles, "max-cycles", 50, "maximum fresh autonomous supervisor cycles")
	cmd.Flags().Int64Var(&maxTasks, "max-tasks", 100, "maximum tasks in one bounded queue sweep")
	cmd.Flags().Var(&singleIntValue{target: &maximumWorkers}, "workers", fmt.Sprintf("maximum parallel queue workers (configured default 1, cap %d)", autonomousqueue.MaximumWorkerLimit))
	cmd.Flags().Int64Var(&maxSweeps, "max-sweeps", 1000, "maximum bounded daemon queue sweeps")
	cmd.Flags().DurationVar(&daemonPoll, "daemon-poll", time.Second, "daemon readiness polling interval")
	cmd.Flags().DurationVar(&daemonDebounce, "daemon-debounce", 500*time.Millisecond, "daemon stable-change debounce interval")
	return cmd
}

type singleIntValue struct {
	target *int
	set    bool
}

func (v *singleIntValue) String() string {
	if v == nil || v.target == nil {
		return "0"
	}
	return strconv.Itoa(*v.target)
}

func (v *singleIntValue) Set(raw string) error {
	if v.set {
		return errors.New("flag may be specified only once")
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return err
	}
	*v.target, v.set = parsed, true
	return nil
}

func (*singleIntValue) Type() string { return "int" }

func writeTaskRunSummary(out io.Writer, result autonomoustaskrun.Result, maxCycles int64) error {
	actions := make([]string, 0, len(result.Statistics.Actions))
	for _, action := range result.Statistics.Actions {
		actions = append(actions, fmt.Sprintf("%s:%d", action.Action, action.Count))
	}
	_, err := fmt.Fprintf(out, "Task run: task=%s operation=%s cycles=%d/%d stop=%s replayed=%t\nLast: action=%s decision=%s run=%s\nStats: supervisors=%d/%d attempts=%d/%d verification=%d audits=%d corrections=%d optional=%d commits=%d checkpoints=%d actions=%s\nDetail: %s\n", result.TaskID, result.OperationID, result.Statistics.CyclesStarted, maxCycles, result.StopReason, result.Replayed, result.LastAction, result.LastDecisionID, result.LastRunID, result.Statistics.SupervisorCompleted, result.Statistics.SupervisorStarted, result.Statistics.AttemptsCompleted, result.Statistics.AttemptsAdmitted, result.Statistics.VerificationRuns, result.Statistics.Audits, result.Statistics.Corrections, result.Statistics.OptionalRoles, result.Statistics.SourceCommits, result.Statistics.CheckpointAdvances, strings.Join(actions, ","), oneLine(result.StopDetail))
	return err
}

func writeQueueSummary(out io.Writer, result autonomousqueue.Result) error {
	if _, err := fmt.Fprintf(out, "Queue: operation=%s mode=%s stop=%s replayed=%t tasks=%d selections=%d workers=%d peak=%d batches=%d fallbacks=%d\n", result.OperationID, result.Mode, result.StopReason, result.Replayed, len(result.Outcomes), result.Statistics.Selections, result.MaximumWorkers, result.Statistics.PeakActiveWorkers, result.Statistics.Batches, result.Statistics.SequentialFallbacks); err != nil {
		return err
	}
	for _, outcome := range result.Outcomes {
		if _, err := fmt.Fprintf(out, "Task: selection=%d batch=%d slot=%d id=%s operation=%s stop=%s replayed=%t detail=%s\n", outcome.SelectionSequence, outcome.Batch, outcome.Slot, outcome.TaskID, outcome.TaskOperationID, outcome.StopReason, outcome.Replayed, oneLine(outcome.StopDetail)); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(out, "Remaining: ready=%s waiting=%s\nDetail: %s\n", strings.Join(result.RemainingReady, ","), strings.Join(result.RemainingWaiting, ","), oneLine(result.StopDetail))
	return err
}

func writeDaemonSummary(out io.Writer, result autonomousdaemon.Result) error {
	_, err := fmt.Fprintf(out, "Daemon: stop=%s sweeps=%d wakes=%d fingerprint=%s detail=%s\n", result.StopReason, result.Sweeps, len(result.Wakes), result.LastFingerprint, oneLine(result.StopDetail))
	return err
}

func runSinglePass(cmd *cobra.Command, workDir string, runOnce RunOnceFunc) error {
	result, err := app.RunOnce(cmd.Context(), app.Config{WorkDir: workDir}, app.RunOnceInput{
		Runner:   runOnce,
		Progress: runProgress(cmd.OutOrStdout()),
	})
	if err != nil {
		return err
	}
	if _, err := fmt.Fprint(cmd.OutOrStdout(), runOnceSummary(result)); err != nil {
		return err
	}
	return app.RunOnceOutcomeError(result)
}

func runBoundedLoop(cmd *cobra.Command, workDir string, runOnce RunOnceFunc, maxPasses int) error {
	result, err := app.RunLoop(cmd.Context(), app.Config{WorkDir: workDir}, app.RunLoopInput{
		MaxPasses: maxPasses,
		Runner:    runOnce,
		Progress:  runProgress(cmd.OutOrStdout()),
		OnPass: func(result runonce.Result) error {
			_, err := fmt.Fprint(cmd.OutOrStdout(), runOnceSummary(result))
			return err
		},
	})
	if strings.TrimSpace(result.Stats.StopReason) != "" {
		if writeErr := writeRunLoopSummary(cmd.OutOrStdout(), result.Stats); writeErr != nil {
			return writeErr
		}
	}
	return err
}

func writeRunLoopSummary(out io.Writer, stats app.RunLoopStats) error {
	if strings.TrimSpace(stats.StopReason) == "" {
		stats.StopReason = "unknown"
	}
	_, err := fmt.Fprintf(out,
		"Loop summary: passes=%d/%d completed=%d failed_or_blocked=%d no_task=%t stop=%s\n",
		stats.Passes,
		stats.MaxPasses,
		stats.Completed,
		stats.FailedOrBlocked,
		stats.NoTask,
		stats.StopReason,
	)
	return err
}

func newStatusCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show harness status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			status, err := app.Status(cmd.Context(), app.Config{WorkDir: opts.WorkDir})
			if err != nil {
				return err
			}
			if !status.Initialized {
				_, err = fmt.Fprint(cmd.OutOrStdout(), "Not initialized. Run `revolvr init` first.\n")
				return err
			}
			return writeStatus(cmd.OutOrStdout(), status.Tasks, status.Schedule, status.RecentRuns, status.LatestEvents)
		},
	}
}

func newTUICommand(opts Options) *cobra.Command {
	runner := opts.TUIRunner
	if runner == nil {
		runner = tuiapp.RunStatus
	}
	return &cobra.Command{
		Use:   "tui",
		Short: "Open status TUI",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := app.Config{WorkDir: opts.WorkDir}
			ctx := cmd.Context()
			status, err := app.Status(ctx, cfg)
			if err != nil {
				return err
			}
			return runner(cmd.Context(), status, tuiapp.RunOptions{
				Input:  cmd.InOrStdin(),
				Output: cmd.OutOrStdout(),
				RefreshStatus: func() (app.StatusResult, error) {
					return app.Status(ctx, cfg)
				},
				OpenRun: func(runID string) (ledger.RunWithEvents, error) {
					return app.ShowRun(ctx, cfg, runID)
				},
				AddTask: func(input app.AddTaskInput) (taskmodel.Task, error) {
					return app.AddTask(ctx, cfg, input)
				},
				RetryTask: func(taskID string) (taskmodel.Task, error) {
					return app.RetryTask(ctx, cfg, taskID)
				},
				ValidateReceipt: func(runID string) (receipt.ValidationResult, error) {
					return app.ValidateReceipt(ctx, cfg, runID)
				},
				Preflight: func() (app.PreflightResult, error) {
					return app.Preflight(ctx, cfg, app.PreflightInput{
						CommandRunner:          opts.DoctorCommandRunner,
						LookPath:               opts.ExecutableLookPath,
						ExecutableInspector:    opts.ExecutableInspector,
						CodexIdentityInspector: opts.CodexIdentityInspector,
					})
				},
				RunOnce: func(runCtx context.Context, progress app.RunProgress) (runonce.Result, error) {
					return app.RunOnce(runCtx, cfg, app.RunOnceInput{
						Runner:   opts.RunOnce,
						Progress: progress,
					})
				},
				RunLoop: func(runCtx context.Context, maxPasses int, progress app.RunProgress, onPass app.RunPassFunc) (app.RunLoopResult, error) {
					return app.RunLoop(runCtx, cfg, app.RunLoopInput{
						MaxPasses: maxPasses,
						Runner:    opts.RunOnce,
						Progress:  progress,
						OnPass:    onPass,
					})
				},
				RunTask: func(runCtx context.Context, taskID string, maxCycles int64, progress autonomoustaskrun.Progress) (autonomoustaskrun.Result, error) {
					runner := opts.RunTaskUntilTerminal
					if runner == nil {
						runner = app.RunTaskUntilTerminal
					}
					return runner(runCtx, cfg, app.TaskRunInput{TaskID: taskID, MaxCycles: maxCycles, Progress: progress})
				},
				ListAutonomous: func() ([]app.AutonomousTaskSelector, error) {
					return app.ListAutonomousTaskSelectors(ctx, cfg)
				},
				LoadAutonomous: func(selector string) (autonomousview.View, error) {
					return app.ShowAutonomousTask(ctx, cfg, selector)
				},
				AnswerInput: func(input app.AnswerAutonomousInputRequest) (app.AnswerAutonomousInputResult, error) {
					return app.AnswerAutonomousInput(ctx, cfg, input)
				},
				RunQueue: func(runCtx context.Context, maxTasks, maxCycles int64, progress autonomousqueue.Progress) (autonomousqueue.Result, error) {
					runner := opts.RunQueue
					if runner == nil {
						runner = app.RunQueue
					}
					return runner(runCtx, cfg, app.QueueInput{MaxTasks: maxTasks, MaxCycles: maxCycles, MaximumWorkers: 1, Progress: progress})
				},
			})
		},
	}
}

func newShowCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "show <run-id>",
		Short: "Show one run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runID := strings.TrimSpace(args[0])
			if runID == "" {
				return fmt.Errorf("show: run id is required")
			}

			history, err := app.ShowRun(cmd.Context(), app.Config{WorkDir: opts.WorkDir}, runID)
			if err != nil {
				return err
			}
			return writeRun(cmd.OutOrStdout(), history)
		},
	}
}

func newReceiptCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "receipt",
		Short: "Inspect and validate receipts",
		Args:  cobra.NoArgs,
		RunE:  runHelp,
	}
	cmd.AddCommand(newReceiptValidateCommand(opts))
	return cmd
}

func newReceiptValidateCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "validate <run-id>",
		Short: "Validate one run receipt against the ledger",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := app.ValidateReceipt(cmd.Context(), app.Config{WorkDir: opts.WorkDir}, args[0])
			if err != nil {
				return err
			}
			if err := writeReceiptValidation(cmd.OutOrStdout(), result); err != nil {
				return err
			}
			if !result.Passed() {
				return receiptValidationError{RunID: result.RunID, FailureCount: len(result.Failures())}
			}
			return nil
		},
	}
}

func runHelp(cmd *cobra.Command, _ []string) error {
	return cmd.Help()
}

func runOnceSummary(result runonce.Result) string {
	if result.NoTask || result.Outcome == runonce.OutcomeNoTask {
		return "No pending runnable tasks.\n"
	}
	switch result.Outcome {
	case runonce.OutcomeCommitted:
		return fmt.Sprintf("Run %s completed task %s; commit %s.\n", result.Run.ID, result.Task.ID, result.Commit.CommitSHA)
	default:
		return fmt.Sprintf("Run %s stopped (%s): %s\n", result.Run.ID, result.Outcome, result.Message)
	}
}

func runProgress(out io.Writer) app.RunProgress {
	if out == nil {
		return nil
	}
	var mu sync.Mutex
	return func(event codexexec.ProgressEvent) {
		event.Source = strings.TrimSpace(event.Source)
		event.Message = strings.TrimSpace(event.Message)
		if event.Message == "" {
			return
		}
		if event.Source == "" {
			event.Source = "codex"
		}
		mu.Lock()
		defer mu.Unlock()
		_, _ = fmt.Fprintf(out, "%s: %s\n", event.Source, event.Message)
	}
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

type taskCounts struct {
	total     int
	pending   int
	blocked   int
	completed int
}

func writeStatus(out io.Writer, tasks []taskmodel.Task, schedule taskscheduler.Result, recentRuns []ledger.Run, latestEvents []ledger.Event) error {
	counts := countTasks(tasks)
	if _, err := fmt.Fprintf(out, "Total tasks: %d\n", counts.total); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Pending tasks: %d\n", counts.pending); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Blocked tasks: %d\n", counts.blocked); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Completed tasks: %d\n", counts.completed); err != nil {
		return err
	}
	if err := writeCheckpointStatus(out, tasks); err != nil {
		return err
	}
	if err := writeNextTaskStatus(out, tasks); err != nil {
		return err
	}
	if err := writeNextAutonomousStatus(out, tasks); err != nil {
		return err
	}
	if err := writeSchedulingStatus(out, tasks, schedule); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Recent runs: %d\n", len(recentRuns)); err != nil {
		return err
	}
	if len(recentRuns) == 0 {
		_, err := fmt.Fprint(out, "Latest run: none\n")
		return err
	}
	return writeLatestRunStatus(out, recentRuns[0], latestEvents)
}

func writeCheckpointStatus(out io.Writer, tasks []taskmodel.Task) error {
	for _, task := range tasks {
		if task.Workflow != taskfile.WorkflowOperatorCheckpointV1 {
			continue
		}
		if _, err := fmt.Fprintf(out, "Operator checkpoint: %s state=%s receipt=%s sha256=%s\n",
			optionalStatusValue(task.ID),
			optionalStatusValue(task.CheckpointState),
			optionalStatusValue(task.CheckpointReceiptPath),
			optionalStatusValue(task.CheckpointReceiptSHA),
		); err != nil {
			return err
		}
	}
	return nil
}

func writeNextAutonomousStatus(out io.Writer, tasks []taskmodel.Task) error {
	for _, task := range tasks {
		if task.NextAutonomous {
			_, err := fmt.Fprintf(out, "Next autonomous task: %s (ready)\n", statusTaskBrief(task))
			return err
		}
	}
	for _, task := range tasks {
		if task.Workflow == taskfile.WorkflowAutonomousV1 && task.Status == taskmodel.StatusPending {
			_, err := fmt.Fprintf(out, "Next autonomous task: none (%s)\n", optionalStatusValue(task.ReadinessReason))
			return err
		}
	}
	return nil
}

func writeNextTaskStatus(out io.Writer, tasks []taskmodel.Task) error {
	for _, task := range tasks {
		if task.Status == taskmodel.StatusPending && task.NextRunnable {
			return writeNextTask(out, task)
		}
	}
	_, err := fmt.Fprint(out, "Next task: none\n")
	return err
}

func writeSchedulingStatus(out io.Writer, tasks []taskmodel.Task, schedule taskscheduler.Result) error {
	for _, task := range tasks {
		if task.Status != taskmodel.StatusPending || task.ReadinessReason == "" || task.Readiness == taskscheduler.ReasonReady {
			continue
		}
		if _, err := fmt.Fprintf(out, "Task readiness: %s reason=%s waiting_on=%s conflict_blockers=%s\n",
			optionalStatusValue(task.ID),
			optionalStatusValue(task.ReadinessReason),
			optionalStatusValue(strings.Join(task.WaitingDependencyIDs, ",")),
			optionalStatusValue(strings.Join(task.ConflictBlockers, ",")),
		); err != nil {
			return err
		}
	}
	for _, diagnostic := range schedule.InvalidGraph {
		if _, err := fmt.Fprintf(out, "Scheduling diagnostic: %s: %s\n", diagnostic.Code, oneLine(diagnostic.Detail)); err != nil {
			return err
		}
	}
	return nil
}

func taskSelectedWorkflows(task taskmodel.Task) string {
	selected := make([]string, 0, 2)
	if task.NextRunnable {
		selected = append(selected, "mixed-pass-v1")
	}
	if task.NextAutonomous {
		selected = append(selected, "autonomous-v1")
	}
	return strings.Join(selected, ",")
}

func taskSchedulingDiagnostics(task taskmodel.Task) string {
	diagnostics := make([]string, 0, len(task.SchedulingDiagnostics))
	for _, diagnostic := range task.SchedulingDiagnostics {
		diagnostics = append(diagnostics, fmt.Sprintf("%s: %s", diagnostic.Code, oneLine(diagnostic.Detail)))
	}
	return strings.Join(diagnostics, "; ")
}

func writeNextTask(out io.Writer, task taskmodel.Task) error {
	if _, err := fmt.Fprintf(out, "Next task: %s\n", statusTaskBrief(task)); err != nil {
		return err
	}
	_, err := fmt.Fprintf(out, "Next pass: workflow=%s phase=%s profile=%s next=%s\n",
		optionalStatusValue(task.Workflow),
		optionalStatusValue(task.Phase),
		optionalStatusValue(task.RunProfile),
		optionalStatusValue(task.NextState),
	)
	return err
}

func statusTaskBrief(task taskmodel.Task) string {
	id := optionalStatusValue(task.ID)
	summary := oneLine(task.Summary)
	if summary == "" {
		summary = oneLine(task.Task)
	}
	if summary == "" {
		return id
	}
	return id + " - " + summary
}

func writeLatestRunStatus(out io.Writer, run ledger.Run, events []ledger.Event) error {
	if _, err := fmt.Fprintf(out, "Latest run: %s (%s)\n", run.ID, run.Status); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Latest summary: %s\n", optionalStatusValue(run.Summary)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Latest verification: %s\n", optionalStatusValue(run.VerificationStatus)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Latest commit: %s\n", optionalStatusValue(run.CommitSHA)); err != nil {
		return err
	}
	return writeLatestArtifacts(out, events)
}

func optionalStatusValue(value string) string {
	value = oneLine(value)
	if value == "" {
		return "none"
	}
	return value
}

func writeLatestArtifacts(out io.Writer, events []ledger.Event) error {
	artifacts, found := ledger.RunArtifactsFromEvents(events)
	if !found || artifacts.Empty() {
		_, err := fmt.Fprint(out, "Latest artifacts: none\n")
		return err
	}
	if _, err := fmt.Fprint(out, "Latest artifacts:\n"); err != nil {
		return err
	}
	return writeArtifactPathLines(out, artifacts)
}

func countTasks(tasks []taskmodel.Task) taskCounts {
	counts := taskCounts{total: len(tasks)}
	for _, task := range tasks {
		switch task.Status {
		case taskmodel.StatusPending:
			counts.pending++
		case taskmodel.StatusBlocked:
			counts.blocked++
		case taskmodel.StatusCompleted:
			counts.completed++
		}
	}
	return counts
}

func writeRun(out io.Writer, history ledger.RunWithEvents) error {
	run := history.Run
	if _, err := fmt.Fprintf(out, "Run ID: %s\n", run.ID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Task ID: %s\n", run.TaskID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Task: %s\n", oneLine(run.Task)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Status: %s\n", run.Status); err != nil {
		return err
	}
	if run.Summary != "" {
		if _, err := fmt.Fprintf(out, "Summary: %s\n", oneLine(run.Summary)); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(out, "Started at: %s\n", cliTime(run.StartedAt)); err != nil {
		return err
	}
	if run.CompletedAt != nil {
		if _, err := fmt.Fprintf(out, "Completed at: %s\n", cliTime(*run.CompletedAt)); err != nil {
			return err
		}
	}
	if run.CodexExitCode != nil {
		if _, err := fmt.Fprintf(out, "Codex exit code: %d\n", *run.CodexExitCode); err != nil {
			return err
		}
	}
	if run.VerificationStatus != "" {
		if _, err := fmt.Fprintf(out, "Verification status: %s\n", run.VerificationStatus); err != nil {
			return err
		}
	}
	if run.CommitSHA != "" {
		if _, err := fmt.Fprintf(out, "Commit SHA: %s\n", run.CommitSHA); err != nil {
			return err
		}
	}
	if err := writeTimeline(out, app.RunTimeline(history)); err != nil {
		return err
	}
	artifacts, artifactEvents := ledger.RunArtifactsFromEvents(history.Events)
	if artifactEvents {
		if err := writeArtifacts(out, artifacts); err != nil {
			return err
		}
	}
	if err := writeDiagnostics(out, diagnosticsFromHistory(history)); err != nil {
		return err
	}
	if _, err := fmt.Fprint(out, "Events:\n"); err != nil {
		return err
	}
	if len(history.Events) == 0 {
		_, err := fmt.Fprint(out, "No events.\n")
		return err
	}
	if _, err := fmt.Fprint(out, "ID\tTYPE\tTIMESTAMP\n"); err != nil {
		return err
	}
	for _, event := range history.Events {
		if _, err := fmt.Fprintf(out, "%d\t%s\t%s\n", event.ID, event.Type, cliTime(event.CreatedAt)); err != nil {
			return err
		}
	}
	return nil
}

func writeTimeline(out io.Writer, rows []app.RunTimelineRow) error {
	if _, err := fmt.Fprint(out, "Timeline:\n"); err != nil {
		return err
	}
	if len(rows) == 0 {
		_, err := fmt.Fprint(out, "No timeline rows.\n")
		return err
	}
	if _, err := fmt.Fprint(out, "TIMESTAMP\tPHASE\tSTATUS\tDETAIL\n"); err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", cliTimelineTime(row.Timestamp), timelineCell(row.Phase), timelineCell(row.Status), timelineCell(row.Detail)); err != nil {
			return err
		}
	}
	return nil
}

func cliTimelineTime(value time.Time) string {
	if value.IsZero() {
		return "none"
	}
	return cliTime(value)
}

func timelineCell(value string) string {
	value = oneLine(value)
	if value == "" {
		return "none"
	}
	return value
}

func writeArtifacts(out io.Writer, artifacts ledger.RunArtifacts) error {
	if _, err := fmt.Fprint(out, "Artifacts:\n"); err != nil {
		return err
	}
	if artifacts.Empty() {
		_, err := fmt.Fprint(out, "none\n")
		return err
	}
	return writeArtifactPathLines(out, artifacts)
}

func writeArtifactPathLines(out io.Writer, artifacts ledger.RunArtifacts) error {
	for _, artifact := range []struct {
		label string
		path  string
	}{
		{label: "context payload", path: artifacts.ContextPayloadPath},
		{label: "context manifest", path: artifacts.ContextManifestPath},
		{label: "codex stdout jsonl", path: artifacts.CodexStdoutJSONLPath},
		{label: "codex stderr", path: artifacts.CodexStderrPath},
		{label: "last message", path: artifacts.LastMessagePath},
		{label: "receipt", path: artifacts.ReceiptPath},
	} {
		if strings.TrimSpace(artifact.path) == "" {
			continue
		}
		if _, err := fmt.Fprintf(out, "%s: %s\n", artifact.label, artifact.path); err != nil {
			return err
		}
	}
	return nil
}

func cliTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

type receiptValidationError struct {
	RunID        string
	FailureCount int
}

type archiveVerificationError struct {
	ArchiveID    string
	FailureCount int
}

func (e archiveVerificationError) Error() string {
	return fmt.Sprintf("archive verification failed for %s (%d failed checks)", optionalStatusValue(e.ArchiveID), e.FailureCount)
}

func (e receiptValidationError) Error() string {
	return fmt.Sprintf("receipt validation failed for run %s (%d failed checks)", e.RunID, e.FailureCount)
}

func writeReceiptValidation(out io.Writer, result receipt.ValidationResult) error {
	status := "passed"
	if !result.Passed() {
		status = "failed"
	}
	if _, err := fmt.Fprintf(out, "Receipt validation: %s\n", status); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Run ID: %s\n", result.RunID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Receipt: %s\n", result.ReceiptPath); err != nil {
		return err
	}
	if _, err := fmt.Fprint(out, "Checks:\n"); err != nil {
		return err
	}
	for _, check := range result.Checks {
		if _, err := fmt.Fprintf(out, "%s: %s\n", check.Name, check.Message()); err != nil {
			return err
		}
	}
	return nil
}
