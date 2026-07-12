// Package autonomouscycle orchestrates exactly one supervisor-directed
// autonomous worker cycle. It deliberately owns no durable task or execution
// state transitions.
package autonomouscycle

import (
	"context"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousassembly"
	"revolvr/internal/autonomouspolicy"
	"revolvr/internal/autonomoussafety"
	"revolvr/internal/autonomousverification"
	"revolvr/internal/codexexec"
	"revolvr/internal/commit"
	"revolvr/internal/gitstate"
	"revolvr/internal/ledger"
	"revolvr/internal/lock"
	"revolvr/internal/prompt"
	"revolvr/internal/receipt"
	"revolvr/internal/runner"
	"revolvr/internal/supervisor"
	"revolvr/internal/taskfile"
	"revolvr/internal/verification"
)

type Outcome string

const (
	OutcomeInvalidConfiguration       Outcome = "invalid_configuration"
	OutcomeSafetyPreflightFailed      Outcome = "safety_preflight_failed"
	OutcomeNoTaskState                Outcome = "no_autonomous_task_or_state"
	OutcomeDossierFailed              Outcome = "dossier_failed"
	OutcomeSourceChangedDuringDossier Outcome = "source_changed_during_dossier_assembly"
	OutcomeSupervisorFailed           Outcome = "supervisor_failed"
	OutcomeSourceChanged              Outcome = "source_changed_before_worker_admission"
	OutcomePolicyRejected             Outcome = "policy_rejected"
	OutcomeCompleteAuthorized         Outcome = "complete_authorized"
	OutcomeBlockAuthorized            Outcome = "block_authorized"
	OutcomeNeedsInputAuthorized       Outcome = "needs_input_authorized"
	OutcomeWorkerFailed               Outcome = "worker_failed"
	OutcomeReadOnlyMutation           Outcome = "unauthorized_read_only_worker_mutation"
	OutcomeWorkerNoChanges            Outcome = "worker_completed_no_source_changes"
	OutcomeReadOnlyCompleted          Outcome = "read_only_worker_evidence_completed"
	OutcomeChangedCaptureFailed       Outcome = "changed_files_capture_failed"
	OutcomeVerificationFailed         Outcome = "verification_failed"
	OutcomeCommitFailed               Outcome = "commit_failed"
	OutcomeVerifiedChangesCommitted   Outcome = "verified_changes_committed"
)

type Ledger interface {
	CreateRun(context.Context, ledger.RunSpec) (ledger.Run, error)
	AppendEvent(context.Context, string, ledger.EventType, any) (ledger.Event, error)
	CompleteRun(context.Context, string, ledger.RunCompletion) (ledger.Run, bool, error)
	RecordCommitSHA(context.Context, string, string) error
}

type SourceLock interface {
	Heartbeat(context.Context) error
	Release(context.Context) error
}

type TaskLoader func(string, string) (taskfile.Task, bool, error)
type DossierAssembler func(context.Context, autonomousassembly.Input) (autonomous.TaskDossier, error)
type SupervisorRunner func(context.Context, supervisor.Config) (supervisor.Result, error)
type PolicyEvaluator func(autonomouspolicy.Input) (autonomouspolicy.Route, error)
type ProfileLoader func(string, string) (prompt.RunProfile, error)
type CodexRunner func(context.Context, codexexec.Config) (codexexec.Result, error)
type SourceSnapshotter func(context.Context, gitstate.SourceSnapshotConfig) (gitstate.SourceSnapshot, error)
type DirtyCapture func(context.Context, gitstate.Config) (gitstate.Capture, error)
type ChangedCapture func(context.Context, gitstate.Config) (gitstate.Capture, error)
type VerificationRunner func(context.Context, verification.Config) (verification.Result, error)
type TieredVerificationRunner func(context.Context, autonomousverification.Config) (autonomousverification.Result, error)
type CommitRunner func(context.Context, commit.Config) (commit.Result, error)
type LockAcquirer func(context.Context, lock.Config) (SourceLock, error)
type CommandRunner func(context.Context, runner.Command) runner.Result
type SafetyPreflightRunner func(context.Context, autonomoussafety.Input) (autonomoussafety.Output, error)

// WorkerAdmission runs after the supervisor decision and policy route have
// been validated against the admission source, but before any worker starts.
// It is the composition seam used by the task runner to durably charge AW-15.
type WorkerAdmission func(context.Context, WorkerAdmissionInput) error

type WorkerAdmissionInput struct {
	TaskID         string
	State          autonomous.ExecutionState
	Decision       autonomous.SupervisorDecision
	Reference      autonomous.DecisionReference
	Route          autonomouspolicy.Route
	SourceRevision string
}

type Config struct {
	// RepositoryRoot is the canonical control root. Workspace, when supplied,
	// authorizes a distinct execution worktree for source mutation.
	RepositoryRoot    string
	Workspace         *autonomous.TaskWorkspace
	TaskID            string
	State             autonomous.ExecutionState
	SafetyDeclaration autonomoussafety.Declaration

	SourceSafety      autonomouspolicy.SourceSafety
	LatestMutation    *autonomouspolicy.SourceMutation
	Verification      *autonomouspolicy.VerificationEvidence
	Audit             *autonomouspolicy.AuditEvidence
	CorrectionFailure *autonomous.VerificationFailureTarget

	HistoryPolicy       autonomousassembly.HistoryPolicy
	HistoryReader       autonomousassembly.HistoryReader
	LedgerPath          string
	GuidancePolicy      autonomousassembly.GuidancePolicy
	RepositoryMapPolicy autonomousassembly.RepositoryMapPolicy
	Ledger              Ledger

	CodexExecutable                string
	CodexModel                     string
	CodexReasoningEffort           string
	CodexEphemeral                 bool
	CodexSandbox                   string
	CodexApprovalPolicy            string
	CodexBypassApprovalsAndSandbox bool
	CodexVersion                   string
	EffectiveConfigSchema          string
	EffectiveConfigSHA256          string
	CodexTimeout                   time.Duration
	CodexStdoutCap                 int
	CodexStderrCap                 int

	GitExecutable string
	GitTimeout    time.Duration
	GitStdoutCap  int
	GitStderrCap  int

	VerificationCommands      []verification.Command
	VerificationPlan          *autonomousverification.Plan
	MissingVerificationPolicy verification.MissingCommandsPolicy
	VerificationTimeout       time.Duration
	VerificationStdoutCap     int
	VerificationStderrCap     int

	AllowPreExistingDirty bool
	CommitTimeout         time.Duration
	CommitStdoutCap       int
	CommitStderrCap       int

	SourceWriterLockTimeout           time.Duration
	SourceWriterLockHeartbeatInterval time.Duration
	SourceWriterLockPID               int

	IDGenerator func() string
	Clock       func() time.Time

	TaskLoader                 TaskLoader
	DossierAssembler           DossierAssembler
	SupervisorRunner           SupervisorRunner
	PolicyEvaluator            PolicyEvaluator
	ProfileLoader              ProfileLoader
	CodexRunner                CodexRunner
	SourceSnapshotter          SourceSnapshotter
	DirtyCapture               DirtyCapture
	ChangedCapture             ChangedCapture
	VerificationRunner         VerificationRunner
	TieredVerificationRunner   TieredVerificationRunner
	VerificationArtifactWriter autonomousverification.ArtifactWriter
	CommitRunner               CommitRunner
	LockAcquirer               LockAcquirer
	CommandRunner              CommandRunner
	SafetyPreflightRunner      SafetyPreflightRunner
	SafetyLookPath             func(string) (string, error)
	SafetyLookupEnv            func(string) (string, bool)
	BeforeWorker               WorkerAdmission
}

type Artifact struct {
	Path     string `json:"path"`
	SHA256   string `json:"sha256,omitempty"`
	ByteSize int    `json:"byte_size,omitempty"`
}

type WorkerArtifacts struct {
	Dossier         Artifact  `json:"dossier"`
	DossierManifest Artifact  `json:"dossier_manifest"`
	Prompt          Artifact  `json:"prompt"`
	Provenance      Artifact  `json:"provenance"`
	OutputSchema    *Artifact `json:"output_schema,omitempty"`
	Output          Artifact  `json:"output"`
	SourceEvidence  Artifact  `json:"source_evidence"`
	CodexStdout     Artifact  `json:"codex_stdout"`
	CodexStderr     Artifact  `json:"codex_stderr"`
	Receipt         Artifact  `json:"receipt"`
	Verification    *Artifact `json:"verification,omitempty"`
}

type ProfileEvidence struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	SHA256   string `json:"sha256"`
	ByteSize int    `json:"byte_size"`
}

type SourceEvidence struct {
	DossierBefore          *gitstate.SourceSnapshot
	DossierAfter           *gitstate.SourceSnapshot
	DossierDifference      gitstate.SourceDifference
	Admission              *gitstate.SourceSnapshot
	AdmissionDifference    gitstate.SourceDifference
	WorkerAfter            *gitstate.SourceSnapshot
	WorkerDifference       gitstate.SourceDifference
	VerificationAfter      *gitstate.SourceSnapshot
	VerificationDifference gitstate.SourceDifference
	Final                  *gitstate.SourceSnapshot
	DossierRevision        string
	AdmissionRevision      string
	WorkerRevision         string
	FinalRevision          string
	ChangedFiles           []string
}

type ReceiptWarning struct {
	Kind     string   `json:"kind"`
	Message  string   `json:"message"`
	Claimed  []string `json:"claimed,omitempty"`
	Observed []string `json:"observed,omitempty"`
}

type ReceiptEvidence struct {
	Path        string
	Receipt     receipt.Receipt
	Synthesized bool
	ParseError  string
	Warnings    []ReceiptWarning
}

type VerificationEvidence struct {
	OccurrenceID   string
	SourceRevision string
	Result         verification.Result
	Policy         *autonomouspolicy.VerificationEvidence
	Tiered         *autonomousverification.Result
}

type WorkerEvidence struct {
	Started      bool
	RunID        string
	Run          ledger.Run
	Action       autonomous.Action
	Profile      ProfileEvidence
	Invocation   codexexec.InvocationProvenance
	Artifacts    WorkerArtifacts
	Codex        codexexec.Result
	RawOutput    []byte
	Receipt      ReceiptEvidence
	Verification VerificationEvidence
	Commit       commit.Result
	LedgerError  error
}

type Failure struct {
	Stage  string
	Reason string
}

type Result struct {
	TaskID          string
	Outcome         Outcome
	DossierManifest autonomous.TaskDossierManifest
	Supervisor      supervisor.Result
	Route           *autonomouspolicy.Route
	Worker          WorkerEvidence
	Source          SourceEvidence
	Failure         *Failure
	SafetyPolicy    *autonomoussafety.Policy
	SafetyPreflight *autonomoussafety.PreflightResult
}
