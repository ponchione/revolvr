package autonomousassembly

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousverification"
	"revolvr/internal/dossiercache"
	"revolvr/internal/ledger"
	"revolvr/internal/pathguard"
	"revolvr/internal/receipt"
	"revolvr/internal/runner"
	"revolvr/internal/taskfile"
)

const (
	defaultLedgerPath    = ".revolvr/ledger.sqlite"
	defaultGitExecutable = "git"
	defaultGitTimeout    = 30 * time.Second
	defaultGitStdoutCap  = 1024 * 1024
	defaultGitStderrCap  = 64 * 1024
)

// HistoryReader supplies selected-task history in a bounded read-only query.
// Implementations must filter by task ID before applying limit. Store satisfies
// this interface through ListRecentRunsForTaskWithEvents.
type HistoryReader interface {
	ListRecentRunsForTaskWithEvents(context.Context, string, int) ([]ledger.RunWithEvents, error)
}

type HistoryPolicy struct {
	// CollectionLimit bounds the complete selected-task source window. Zero
	// collects no history and does not query the ledger.
	CollectionLimit int
	// RenderLimit bounds the newest prefix rendered by AW-03 and must not
	// exceed CollectionLimit. The source hash still covers the full collected
	// window.
	RenderLimit int
}

type GuidancePath struct {
	Path     string
	Required bool
}

type GuidancePolicy struct {
	Additional []GuidancePath
}

type RepositoryMapPolicy struct {
	Enabled  bool
	MaxPaths int
	MaxBytes int
}

type GitOptions struct {
	Executable    string
	Timeout       time.Duration
	StdoutLimit   int
	StderrLimit   int
	CommandRunner func(context.Context, runner.Command) runner.Result
}

type Input struct {
	RepositoryRoot string
	// ExecutionRoot is the admitted task worktree used only for source/Git
	// evidence. Canonical task, guidance, ledger, receipts, and artifacts remain
	// rooted at RepositoryRoot. Empty retains the pre-AW-18 read-only behavior.
	ExecutionRoot string
	TaskID        string
	State         autonomous.ExecutionState
	Audit         *autonomous.AuditReport
	Verification  *autonomous.VerificationSummary

	HistoryPolicy HistoryPolicy
	// LedgerPath is repository-relative and defaults to .revolvr/ledger.sqlite.
	// A missing default or configured ledger is optional. HistoryReader may be
	// injected instead; setting both is contradictory.
	LedgerPath    string
	HistoryReader HistoryReader

	GuidancePolicy      GuidancePolicy
	RepositoryMapPolicy RepositoryMapPolicy
	Role                autonomous.DossierRole
	Git                 GitOptions
}

type assembledRun struct {
	summary      autonomous.RecentRunSummary
	receipt      *autonomous.ReceiptSource
	verification *autonomous.VerificationSummary
}

type verificationEvidence struct {
	status   string
	summary  string
	command  string
	event    *ledger.Event
	entries  []receipt.VerificationEntry
	evidence []autonomous.EvidenceReference
	tiered   *autonomousverification.Result
}

// Assemble reads a single repository evidence snapshot and feeds the pure
// AW-03 projection. It does not create runtime state or write any source.
func Assemble(ctx context.Context, in Input) (autonomous.TaskDossier, error) {
	dossier, err := assemble(ctx, in)
	if err != nil {
		return autonomous.TaskDossier{}, fmt.Errorf("assemble task dossier: %w", err)
	}
	return dossier, nil
}

func assemble(ctx context.Context, in Input) (autonomous.TaskDossier, error) {
	root, err := repositoryRoot(in.RepositoryRoot)
	if err != nil {
		return autonomous.TaskDossier{}, err
	}
	gitRoot := root
	if strings.TrimSpace(in.ExecutionRoot) != "" {
		gitRoot, err = repositoryRoot(in.ExecutionRoot)
		if err != nil {
			return autonomous.TaskDossier{}, fmt.Errorf("execution root: %w", err)
		}
	}
	if strings.TrimSpace(in.TaskID) == "" {
		return autonomous.TaskDossier{}, errors.New("task: requested task_id is required")
	}
	if in.TaskID != strings.TrimSpace(in.TaskID) {
		return autonomous.TaskDossier{}, fmt.Errorf("task: requested task_id %q must not contain leading or trailing whitespace", in.TaskID)
	}
	if err := in.State.Validate(); err != nil {
		return autonomous.TaskDossier{}, fmt.Errorf("state: %w", err)
	}
	if in.State.TaskID != in.TaskID {
		return autonomous.TaskDossier{}, fmt.Errorf("state: requested task_id %q does not match execution_state task_id %q", in.TaskID, in.State.TaskID)
	}
	if in.Audit != nil {
		if err := in.Audit.Validate(); err != nil {
			return autonomous.TaskDossier{}, fmt.Errorf("audit: %w", err)
		}
		if in.Audit.TaskID != in.TaskID {
			return autonomous.TaskDossier{}, fmt.Errorf("audit: requested task_id %q does not match audit task_id %q", in.TaskID, in.Audit.TaskID)
		}
	}
	if in.Verification != nil && in.Verification.TaskID != in.TaskID {
		return autonomous.TaskDossier{}, fmt.Errorf("verification: requested task_id %q does not match verification task_id %q", in.TaskID, in.Verification.TaskID)
	}
	if err := validateHistoryPolicy(in.HistoryPolicy); err != nil {
		return autonomous.TaskDossier{}, err
	}
	if in.HistoryReader != nil && strings.TrimSpace(in.LedgerPath) != "" {
		return autonomous.TaskDossier{}, errors.New("ledger: ledger_path and injected history_reader cannot both be set")
	}
	gitOptions, err := normalizeGitOptions(in.Git)
	if err != nil {
		return autonomous.TaskDossier{}, err
	}

	headBefore, err := captureHEAD(ctx, gitRoot, gitOptions, "before evidence collection")
	if err != nil {
		return autonomous.TaskDossier{}, err
	}

	task, found, err := taskfile.FindByID(root, in.TaskID)
	if err != nil {
		return autonomous.TaskDossier{}, fmt.Errorf("task: load canonical task_id %q: %w", in.TaskID, err)
	}
	if !found {
		return autonomous.TaskDossier{}, fmt.Errorf("task: canonical task_id %q was not found under %s", in.TaskID, taskfile.TasksDir)
	}
	if task.ID != in.TaskID {
		return autonomous.TaskDossier{}, fmt.Errorf("task: loaded task_id %q does not match requested task_id %q", task.ID, in.TaskID)
	}

	history, hasOlder, err := collectHistory(ctx, root, in)
	if err != nil {
		return autonomous.TaskDossier{}, err
	}
	runs := make([]autonomous.RecentRunSummary, 0, len(history))
	receipts := make([]autonomous.ReceiptSource, 0, len(history))
	var currentVerification *autonomous.VerificationSummary
	if in.Verification != nil {
		verification := *in.Verification
		currentVerification = &verification
	}
	for i, item := range history {
		assembled, err := assembleRun(root, in.TaskID, item)
		if err != nil {
			return autonomous.TaskDossier{}, fmt.Errorf("run[%d] %q: %w", i, item.Run.ID, err)
		}
		runs = append(runs, assembled.summary)
		if assembled.receipt != nil {
			receipts = append(receipts, *assembled.receipt)
		}
		if currentVerification == nil && assembled.verification != nil {
			verification := *assembled.verification
			currentVerification = &verification
		}
	}

	guidance, err := collectGuidance(root, task.SourcePath, in.GuidancePolicy)
	if err != nil {
		return autonomous.TaskDossier{}, err
	}
	repositoryMap, err := collectRepositoryMap(ctx, root, gitRoot, headBefore, guidance, in.RepositoryMapPolicy, gitOptions)
	if err != nil {
		return autonomous.TaskDossier{}, err
	}
	status, err := captureGitText(ctx, gitRoot, gitOptions, "worktree status", []string{
		"-c", "color.ui=false", "-c", "core.quotePath=true",
		"status", "--short", "--untracked-files=all",
	})
	if err != nil {
		return autonomous.TaskDossier{}, err
	}
	if status == "" {
		status = "clean"
	}
	diffSummary, err := captureGitText(ctx, gitRoot, gitOptions, "diff summary", []string{
		"-c", "color.ui=false", "-c", "core.quotePath=true",
		"diff", "--stat", "--no-ext-diff", "--no-renames", "HEAD", "--",
	})
	if err != nil {
		return autonomous.TaskDossier{}, err
	}
	if diffSummary == "" {
		diffSummary = "none"
	}
	headAfter, err := captureHEAD(ctx, gitRoot, gitOptions, "after evidence collection")
	if err != nil {
		return autonomous.TaskDossier{}, err
	}
	if headBefore != headAfter {
		return autonomous.TaskDossier{}, fmt.Errorf("git: snapshot changed during assembly: HEAD before %q, HEAD after %q", headBefore, headAfter)
	}
	taskAfter, found, err := taskfile.FindByID(root, in.TaskID)
	if err != nil || !found {
		return autonomous.TaskDossier{}, errors.Join(err, errors.New("task: canonical task changed or disappeared during assembly"))
	}
	if taskAfter.SourcePath != task.SourcePath || !reflect.DeepEqual(taskAfter.SourceBytes, task.SourceBytes) {
		return autonomous.TaskDossier{}, errors.New("task: canonical task bytes changed during assembly")
	}
	guidanceAfter, err := collectGuidance(root, task.SourcePath, in.GuidancePolicy)
	if err != nil {
		return autonomous.TaskDossier{}, err
	}
	if !reflect.DeepEqual(guidanceAfter, guidance) {
		return autonomous.TaskDossier{}, errors.New("guidance: applicable source bytes changed during assembly")
	}

	gitEvidence := autonomous.EvidenceReference{
		Kind:      autonomous.EvidenceKindGit,
		Reference: "git:head:" + headBefore,
		Detail:    "Read-only HEAD, worktree status, and diff summary captured from one stable Git snapshot.",
	}
	window := &autonomous.DossierSourceWindow{
		Limit:         in.HistoryPolicy.CollectionLimit,
		HasOlderItems: hasOlder,
	}
	dossierInput := autonomous.TaskDossierInput{
		TaskID: in.TaskID,
		TaskSpec: autonomous.TaskSpecSource{
			ID:      "task-spec:" + task.ID,
			Path:    task.SourcePath,
			Label:   task.Title,
			Content: task.SourceBytes,
		},
		State:           in.State,
		Verification:    currentVerification,
		Audit:           in.Audit,
		RecentRuns:      runs,
		RecentRunLimit:  in.HistoryPolicy.RenderLimit,
		RecentRunWindow: window,
		Receipts:        receipts,
		Git: &autonomous.GitSnapshot{
			Head:           headBefore,
			WorktreeStatus: status,
			DiffSummary:    diffSummary,
			Evidence:       &gitEvidence,
		},
		Guidance:      guidance,
		RepositoryMap: repositoryMap,
	}
	var dossier autonomous.TaskDossier
	if in.Role == "" {
		dossier, err = autonomous.BuildTaskDossier(dossierInput)
	} else {
		dossier, err = autonomous.ProjectTaskDossier(dossierInput, in.Role)
	}
	if err != nil {
		return autonomous.TaskDossier{}, fmt.Errorf("dossier projection: %w", err)
	}
	return dossier, nil
}

func collectRepositoryMap(ctx context.Context, controlRoot, executionRoot, head string, guidance []autonomous.GuidanceSource, policy RepositoryMapPolicy, gitOptions GitOptions) (*autonomous.RepositoryMapSource, error) {
	if !policy.Enabled {
		return nil, nil
	}
	if policy.MaxPaths <= 0 {
		policy.MaxPaths = 4000
	}
	if policy.MaxBytes <= 0 {
		policy.MaxBytes = 512 * 1024
	}
	controlID, err := dossiercache.RootIdentity(controlRoot)
	if err != nil {
		return nil, fmt.Errorf("repository map: control root identity: %w", err)
	}
	executionID, err := dossiercache.RootIdentity(executionRoot)
	if err != nil {
		return nil, fmt.Errorf("repository map: execution root identity: %w", err)
	}
	tree, err := captureGitText(ctx, executionRoot, gitOptions, "HEAD tree identity", []string{"rev-parse", "--verify", head + "^{tree}"})
	if err != nil {
		return nil, err
	}
	identities := make([]dossiercache.GuidanceIdentity, len(guidance))
	for i, item := range guidance {
		sum := sha256.Sum256(item.Content)
		identities[i] = dossiercache.GuidanceIdentity{Path: item.Path, SHA256: fmt.Sprintf("%x", sum), ByteSize: len(item.Content)}
	}
	source := dossiercache.Source{
		SchemaVersion: dossiercache.SchemaVersion, Algorithm: dossiercache.ProducerAlgorithm,
		ControlRootID: controlID, ExecutionRootID: executionID, CommitSHA: head, TreeSHA: tree,
		MaxPaths: policy.MaxPaths, MaxBytes: policy.MaxBytes, Guidance: identities,
	}
	store := dossiercache.Store{RepositoryRoot: controlRoot}
	lookup, err := store.Lookup(ctx, source)
	if err != nil {
		return nil, fmt.Errorf("repository map cache lookup: %w", err)
	}
	if lookup.Class == dossiercache.ResultHit {
		manifestSHA, err := dossierCacheManifestSHA(lookup.Entry.Manifest)
		if err != nil {
			return nil, err
		}
		return &autonomous.RepositoryMapSource{ID: "repository-map:" + tree, CommitSHA: head, TreeSHA: tree, Content: lookup.Entry.Content, CacheKey: lookup.Key, CacheResult: string(lookup.Class), CacheManifestSHA256: manifestSHA}, nil
	}
	treeRaw, err := captureGitText(ctx, executionRoot, gitOptions, "committed tree path map", []string{"ls-tree", "-r", "-z", head})
	if err != nil {
		return nil, err
	}
	items, err := dossiercache.ParseTreeItems(treeRaw)
	if err != nil {
		return nil, fmt.Errorf("repository map tree parse: %w", err)
	}
	mapped, err := dossiercache.BuildRepositoryMapItems(source, items)
	if err != nil {
		return nil, fmt.Errorf("repository map collection: %w", err)
	}
	entry, err := dossiercache.NewEntry(source, mapped.Content, mapped.Total, mapped.Included)
	if err != nil {
		return nil, fmt.Errorf("repository map cache entry: %w", err)
	}
	diagnostic := "cache_" + string(lookup.Class)
	if lookup.Diagnostic != "" {
		diagnostic += ":" + lookup.Diagnostic
	}
	if lookup.Class == dossiercache.ResultMiss {
		if err := store.Publish(ctx, entry); err != nil {
			return nil, fmt.Errorf("repository map cache publication: %w", err)
		}
	}
	manifestSHA, err := dossierCacheManifestSHA(entry.Manifest)
	if err != nil {
		return nil, err
	}
	return &autonomous.RepositoryMapSource{ID: "repository-map:" + tree, CommitSHA: head, TreeSHA: tree, Content: mapped.Content, CacheKey: entry.Manifest.Key, CacheResult: string(dossiercache.ResultRecomputed), CacheDiagnostic: diagnostic, CacheManifestSHA256: manifestSHA}, nil
}

func dossierCacheManifestSHA(manifest dossiercache.Manifest) (string, error) {
	raw, err := dossiercache.MarshalManifest(manifest)
	if err != nil {
		return "", fmt.Errorf("repository map cache manifest: %w", err)
	}
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum), nil
}

func repositoryRoot(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("repository: root is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("repository: resolve root %q: %w", path, err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("repository: resolve root %q: %w", path, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("repository: inspect root %q: %w", path, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("repository: root %q is not a directory", path)
	}
	return resolved, nil
}

func validateHistoryPolicy(policy HistoryPolicy) error {
	if policy.CollectionLimit < 0 {
		return fmt.Errorf("history policy: collection_limit cannot be negative (got %d; zero collects no runs)", policy.CollectionLimit)
	}
	if policy.RenderLimit < 0 {
		return fmt.Errorf("history policy: render_limit cannot be negative (got %d; zero renders no runs)", policy.RenderLimit)
	}
	if policy.RenderLimit > policy.CollectionLimit {
		return fmt.Errorf("history policy: render_limit %d exceeds collection_limit %d", policy.RenderLimit, policy.CollectionLimit)
	}
	if policy.CollectionLimit == math.MaxInt {
		return errors.New("history policy: collection_limit is too large to probe for an older bounded item")
	}
	return nil
}

func collectHistory(ctx context.Context, root string, in Input) ([]ledger.RunWithEvents, bool, error) {
	limit := in.HistoryPolicy.CollectionLimit
	if limit == 0 {
		return nil, false, nil
	}

	reader := in.HistoryReader
	var closeReader func() error
	if reader == nil {
		ledgerPath := strings.TrimSpace(in.LedgerPath)
		if ledgerPath == "" {
			ledgerPath = defaultLedgerPath
		}
		cleanPath, absPath, err := resolveRepositoryPath(root, ledgerPath)
		if err != nil {
			return nil, false, fmt.Errorf("ledger: unsafe path %q: %w", ledgerPath, err)
		}
		if _, err := os.Stat(absPath); errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		} else if err != nil {
			return nil, false, fmt.Errorf("ledger: inspect %s: %w", cleanPath, err)
		}
		store, err := ledger.OpenLiveReadOnly(ctx, absPath)
		if err != nil {
			return nil, false, fmt.Errorf("ledger: open %s: %w", cleanPath, err)
		}
		reader = store
		closeReader = store.Close
	}
	if closeReader != nil {
		defer closeReader()
	}

	requested := limit + 1
	history, err := reader.ListRecentRunsForTaskWithEvents(ctx, in.TaskID, requested)
	if err != nil {
		return nil, false, fmt.Errorf("ledger: query recent runs for task_id %q with limit %d: %w", in.TaskID, requested, err)
	}
	if len(history) > requested {
		return nil, false, fmt.Errorf("ledger: query returned %d runs for requested limit %d", len(history), requested)
	}

	copyHistory := make([]ledger.RunWithEvents, len(history))
	seen := make(map[string]struct{}, len(history))
	for i, item := range history {
		copyHistory[i] = item
		copyHistory[i].Events = append([]ledger.Event(nil), item.Events...)
		if strings.TrimSpace(item.Run.ID) == "" {
			return nil, false, fmt.Errorf("ledger: runs[%d].id is required", i)
		}
		if _, exists := seen[item.Run.ID]; exists {
			return nil, false, fmt.Errorf("ledger: runs[%d].id duplicates run_id %q", i, item.Run.ID)
		}
		seen[item.Run.ID] = struct{}{}
		if item.Run.TaskID != in.TaskID {
			return nil, false, fmt.Errorf("ledger: runs[%d] run_id %q task_id %q does not match requested task_id %q", i, item.Run.ID, item.Run.TaskID, in.TaskID)
		}
	}
	sort.Slice(copyHistory, func(i, j int) bool {
		left, right := copyHistory[i].Run, copyHistory[j].Run
		if left.StartedAt.Equal(right.StartedAt) {
			return left.ID < right.ID
		}
		return left.StartedAt.After(right.StartedAt)
	})
	hasOlder := len(copyHistory) > limit
	if hasOlder {
		copyHistory = copyHistory[:limit]
	}
	return copyHistory, hasOlder, nil
}

func assembleRun(root, taskID string, item ledger.RunWithEvents) (assembledRun, error) {
	run := item.Run
	if run.TaskID != taskID {
		return assembledRun{}, fmt.Errorf("ledger identity: run task_id %q does not match dossier task_id %q", run.TaskID, taskID)
	}
	if run.Status != ledger.StatusRunning && run.Status != ledger.StatusCompleted && run.Status != ledger.StatusFailed {
		return assembledRun{}, fmt.Errorf("ledger status: unknown status %q", run.Status)
	}
	if run.StartedAt.IsZero() {
		return assembledRun{}, errors.New("ledger started_at is required")
	}
	if run.CompletedAt != nil && run.CompletedAt.Before(run.StartedAt) {
		return assembledRun{}, fmt.Errorf("ledger completed_at %s precedes started_at %s", run.CompletedAt.UTC().Format(time.RFC3339Nano), run.StartedAt.UTC().Format(time.RFC3339Nano))
	}

	events, err := orderedEvents(run.ID, item.Events)
	if err != nil {
		return assembledRun{}, err
	}
	var action autonomous.Action
	var profile autonomous.WorkerProfile
	var receiptPath string
	var commitEventSHA string
	commitEventFound := false
	var verification verificationEvidence
	verificationFound := false

	for i := range events {
		event := events[i]
		switch event.Type {
		case ledger.EventTaskSelected:
			eventAction, eventProfile, err := taskSelectionEvidence(event, taskID)
			if err != nil {
				return assembledRun{}, fmt.Errorf("event %d task_selected: %w", event.ID, err)
			}
			if eventAction != "" {
				if action != "" && action != eventAction {
					return assembledRun{}, fmt.Errorf("event %d task_selected action %q conflicts with earlier action %q", event.ID, eventAction, action)
				}
				action = eventAction
			}
			if eventProfile != "" {
				if profile != "" && profile != eventProfile {
					return assembledRun{}, fmt.Errorf("event %d task_selected profile %q conflicts with earlier profile %q", event.ID, eventProfile, profile)
				}
				profile = eventProfile
			}
		case ledger.EventRunArtifacts, ledger.EventContextBuilt, ledger.EventReceiptParsed, ledger.EventReceiptSynthesized:
			path, present, err := receiptPathEvidence(event)
			if err != nil {
				return assembledRun{}, fmt.Errorf("event %d %s receipt provenance: %w", event.ID, event.Type, err)
			}
			if present {
				if receiptPath != "" && receiptPath != path {
					return assembledRun{}, fmt.Errorf("event %d %s receipt path %q conflicts with earlier path %q", event.ID, event.Type, path, receiptPath)
				}
				receiptPath = path
			}
		case ledger.EventCommitCreated:
			sha, err := requiredEventString(event, "commit_sha")
			if err != nil {
				return assembledRun{}, fmt.Errorf("event %d commit_created: %w", event.ID, err)
			}
			if commitEventFound && commitEventSHA != sha {
				return assembledRun{}, fmt.Errorf("event %d commit_created commit_sha %q conflicts with earlier commit_sha %q", event.ID, sha, commitEventSHA)
			}
			commitEventFound = true
			commitEventSHA = sha
		case ledger.EventVerificationCompleted:
			candidate, err := verificationFromEvent(event, run.ID)
			if err != nil {
				return assembledRun{}, fmt.Errorf("event %d verification_completed: %w", event.ID, err)
			}
			if verificationFound && verification.status != candidate.status {
				return assembledRun{}, fmt.Errorf("event %d verification_completed status %q conflicts with earlier status %q", event.ID, candidate.status, verification.status)
			}
			verificationFound = true
			verification = candidate
		}
	}
	if action != "" && profile != "" {
		if expectedProfile(action) != profile {
			return assembledRun{}, fmt.Errorf("ledger action %q is incompatible with profile %q", action, profile)
		}
	}

	evidence := []autonomous.EvidenceReference{{
		Kind:      autonomous.EvidenceKindLedger,
		Reference: "ledger:run:" + run.ID,
		Detail:    fmt.Sprintf("Run status %s with %d ordered ledger event(s).", run.Status, len(events)),
	}}
	var receiptSource *autonomous.ReceiptSource
	var parsedReceipt *receipt.Receipt
	if receiptPath != "" {
		parsed, source, err := loadReceipt(root, taskID, run.ID, receiptPath)
		if err != nil {
			return assembledRun{}, err
		}
		parsedReceipt = &parsed
		receiptSource = &source
		evidence = append(evidence, autonomous.EvidenceReference{
			Kind:      autonomous.EvidenceKindReceipt,
			Reference: source.Path,
			Detail:    fmt.Sprintf("Exact receipt bytes for run %s.", run.ID),
		})
	}
	if err := validateCommitConsistency(run, commitEventFound, commitEventSHA, parsedReceipt); err != nil {
		return assembledRun{}, err
	}
	if run.CommitSHA != "" {
		evidence = append(evidence, autonomous.EvidenceReference{
			Kind:      autonomous.EvidenceKindGit,
			Reference: run.CommitSHA,
			Detail:    "Commit identity recorded by the ledger run.",
		})
	}

	verificationSummary, err := reconcileVerification(run, verificationFound, verification, parsedReceipt, receiptSource)
	if err != nil {
		return assembledRun{}, err
	}
	if verificationSummary != nil {
		evidence = append(evidence, verificationSummary.Evidence...)
		evidence = uniqueEvidence(evidence)
	}

	completedAt := cloneTime(run.CompletedAt)
	return assembledRun{
		summary: autonomous.RecentRunSummary{
			RunID:       run.ID,
			TaskID:      taskID,
			Action:      action,
			Profile:     profile,
			Outcome:     runOutcome(run),
			StartedAt:   run.StartedAt,
			CompletedAt: completedAt,
			Evidence:    uniqueEvidence(evidence),
		},
		receipt:      receiptSource,
		verification: verificationSummary,
	}, nil
}

func orderedEvents(runID string, input []ledger.Event) ([]ledger.Event, error) {
	events := append([]ledger.Event(nil), input...)
	seen := make(map[int64]struct{}, len(events))
	for i, event := range events {
		if event.RunID != runID {
			return nil, fmt.Errorf("event[%d] id %d run_id %q does not match ledger run_id %q", i, event.ID, event.RunID, runID)
		}
		if _, exists := seen[event.ID]; exists {
			return nil, fmt.Errorf("event[%d] duplicates event id %d", i, event.ID)
		}
		seen[event.ID] = struct{}{}
	}
	sort.Slice(events, func(i, j int) bool { return events[i].ID < events[j].ID })
	return events, nil
}

func taskSelectionEvidence(event ledger.Event, taskID string) (autonomous.Action, autonomous.WorkerProfile, error) {
	fields, err := eventObject(event)
	if err != nil {
		return "", "", err
	}
	recordedTaskID, present, err := eventString(fields, "task_id")
	if err != nil {
		return "", "", err
	}
	if !present || recordedTaskID == "" {
		return "", "", errors.New("task_id is required")
	}
	if recordedTaskID != taskID {
		return "", "", fmt.Errorf("task_id %q does not match dossier task_id %q", recordedTaskID, taskID)
	}

	var action autonomous.Action
	if value, present, err := eventString(fields, "action"); err != nil {
		return "", "", err
	} else if present && value != "" {
		action = autonomous.Action(value)
		if !knownAction(action) {
			return "", "", fmt.Errorf("unknown action %q", value)
		}
	}
	if phase, present, err := eventString(fields, "phase"); err != nil {
		return "", "", err
	} else if present && phase != "" {
		phaseAction, ok := actionForPhase(phase)
		if !ok {
			return "", "", fmt.Errorf("unknown phase %q", phase)
		}
		if action != "" && action != phaseAction {
			return "", "", fmt.Errorf("action %q conflicts with phase %q", action, phase)
		}
		action = phaseAction
	}

	profileName, profilePresent, err := eventString(fields, "profile_name")
	if err != nil {
		return "", "", err
	}
	legacyProfile, legacyPresent, err := eventString(fields, "profile")
	if err != nil {
		return "", "", err
	}
	if profilePresent && legacyPresent && profileName != legacyProfile {
		return "", "", fmt.Errorf("profile_name %q conflicts with profile %q", profileName, legacyProfile)
	}
	if !profilePresent {
		profileName = legacyProfile
		profilePresent = legacyPresent
	}
	var profile autonomous.WorkerProfile
	if profilePresent && profileName != "" {
		profile = autonomous.WorkerProfile(profileName)
		if !knownProfile(profile) {
			return "", "", fmt.Errorf("unknown profile %q", profileName)
		}
	}
	if action != "" && profile != "" && expectedProfile(action) != profile {
		return "", "", fmt.Errorf("action %q is incompatible with profile %q", action, profile)
	}
	return action, profile, nil
}

func receiptPathEvidence(event ledger.Event) (string, bool, error) {
	fields, err := eventObject(event)
	if err != nil {
		return "", false, err
	}
	path, present, err := eventString(fields, "receipt_path")
	if err != nil {
		return "", false, err
	}
	if (event.Type == ledger.EventReceiptParsed || event.Type == ledger.EventReceiptSynthesized) && (!present || path == "") {
		return "", false, errors.New("receipt_path is required")
	}
	return path, present && path != "", nil
}

func requiredEventString(event ledger.Event, key string) (string, error) {
	fields, err := eventObject(event)
	if err != nil {
		return "", err
	}
	value, present, err := eventString(fields, key)
	if err != nil {
		return "", err
	}
	if !present || value == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return value, nil
}

func verificationFromEvent(event ledger.Event, runID string) (verificationEvidence, error) {
	fields, err := eventObject(event)
	if err != nil {
		return verificationEvidence{}, err
	}
	status, present, err := eventString(fields, "status")
	if err != nil {
		return verificationEvidence{}, err
	}
	if !present || status == "" {
		return verificationEvidence{}, errors.New("status is required")
	}
	if err := validateVerificationStatus(status); err != nil {
		return verificationEvidence{}, err
	}
	message, _, err := eventString(fields, "message")
	if err != nil {
		return verificationEvidence{}, err
	}
	if message == "" {
		message = "Verification " + status + "."
	}

	var entries []receipt.VerificationEntry
	var commands []struct {
		Command  string `json:"command"`
		Status   string `json:"status"`
		ExitCode int    `json:"exit_code"`
	}
	if raw, ok := fields["commands"]; ok {
		if err := json.Unmarshal(raw, &commands); err != nil {
			return verificationEvidence{}, fmt.Errorf("commands: %w", err)
		}
		entries = make([]receipt.VerificationEntry, 0, len(commands))
		for i, command := range commands {
			command.Command = strings.TrimSpace(command.Command)
			command.Status = strings.TrimSpace(command.Status)
			if command.Command == "" {
				return verificationEvidence{}, fmt.Errorf("commands[%d].command is required", i)
			}
			if command.Status != "" {
				if err := validateVerificationStatus(command.Status); err != nil {
					return verificationEvidence{}, fmt.Errorf("commands[%d]: %w", i, err)
				}
			}
			entries = append(entries, receipt.VerificationEntry{Command: command.Command, Status: command.Status, ExitCode: command.ExitCode})
		}
	}
	commandNames := make([]string, 0, len(entries))
	for _, entry := range entries {
		commandNames = append(commandNames, entry.Command)
	}
	evidence := autonomous.EvidenceReference{
		Kind:      autonomous.EvidenceKindVerification,
		Reference: eventReference(runID, event),
		Detail:    fmt.Sprintf("Ledger verification_completed event recorded status %s.", status),
	}
	var tiered *autonomousverification.Result
	if _, ok := fields["gate"]; ok {
		var payload struct {
			TaskID         string                              `json:"task_id"`
			OccurrenceID   string                              `json:"occurrence_id"`
			SourceRevision string                              `json:"source_revision"`
			Plan           autonomousverification.PlanIdentity `json:"plan"`
			Purpose        autonomousverification.Purpose      `json:"purpose"`
			Outcome        autonomousverification.Outcome      `json:"outcome"`
			Gate           autonomousverification.GateEvidence `json:"gate"`
			Tiers          []autonomousverification.TierResult `json:"tiers"`
			Artifact       *autonomousverification.Artifact    `json:"artifact"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return verificationEvidence{}, err
		}
		candidate := autonomousverification.Result{SchemaVersion: autonomousverification.ResultSchemaVersion, TaskID: payload.TaskID, RunID: runID, OccurrenceID: payload.OccurrenceID, SourceRevision: payload.SourceRevision, Plan: payload.Plan, Purpose: payload.Purpose, Outcome: payload.Outcome, Gate: payload.Gate, Tiers: payload.Tiers, Artifact: payload.Artifact}
		if err := candidate.Validate(); err != nil {
			return verificationEvidence{}, fmt.Errorf("tiered result: %w", err)
		}
		tiered = &candidate
	}
	return verificationEvidence{
		status:   status,
		summary:  strings.TrimSpace(message),
		command:  strings.Join(commandNames, " ; "),
		event:    &event,
		entries:  entries,
		evidence: []autonomous.EvidenceReference{evidence},
		tiered:   tiered,
	}, nil
}

func loadReceipt(root, taskID, runID, recordedPath string) (receipt.Receipt, autonomous.ReceiptSource, error) {
	cleanPath, absPath, err := resolveRepositoryPath(root, recordedPath)
	if err != nil {
		return receipt.Receipt{}, autonomous.ReceiptSource{}, fmt.Errorf("receipt: run_id %q unsafe recorded path %q: %w", runID, recordedPath, err)
	}
	raw, err := os.ReadFile(absPath)
	if err != nil {
		return receipt.Receipt{}, autonomous.ReceiptSource{}, fmt.Errorf("receipt: run_id %q read %s: %w", runID, cleanPath, err)
	}
	parsed, err := receipt.Parse(raw)
	if err != nil {
		return receipt.Receipt{}, autonomous.ReceiptSource{}, fmt.Errorf("receipt: run_id %q parse %s: %w", runID, cleanPath, err)
	}
	if parsed.RunID != runID {
		return receipt.Receipt{}, autonomous.ReceiptSource{}, fmt.Errorf("receipt: path %s run_id %q does not match ledger run_id %q", cleanPath, parsed.RunID, runID)
	}
	if parsed.PassID != runID {
		return receipt.Receipt{}, autonomous.ReceiptSource{}, fmt.Errorf("receipt: path %s pass_id %q does not match ledger run_id %q", cleanPath, parsed.PassID, runID)
	}
	if parsed.TaskID != taskID {
		return receipt.Receipt{}, autonomous.ReceiptSource{}, fmt.Errorf("receipt: path %s task_id %q does not match dossier task_id %q", cleanPath, parsed.TaskID, taskID)
	}
	return parsed, autonomous.ReceiptSource{
		ID:      "receipt:" + runID,
		Path:    cleanPath,
		Content: raw,
	}, nil
}

func validateCommitConsistency(run ledger.Run, eventFound bool, eventSHA string, parsed *receipt.Receipt) error {
	ledgerSHA := strings.TrimSpace(run.CommitSHA)
	if run.CommitSHA != ledgerSHA {
		return fmt.Errorf("commit: ledger run_id %q commit_sha contains leading or trailing whitespace", run.ID)
	}
	if eventFound && eventSHA != ledgerSHA {
		return fmt.Errorf("commit: run_id %q commit_created commit_sha %q conflicts with ledger commit_sha %q", run.ID, eventSHA, ledgerSHA)
	}
	if parsed != nil && parsed.CommitSHA != ledgerSHA {
		return fmt.Errorf("commit: run_id %q receipt commit_sha %q conflicts with ledger commit_sha %q", run.ID, parsed.CommitSHA, ledgerSHA)
	}
	return nil
}

func reconcileVerification(run ledger.Run, eventFound bool, event verificationEvidence, parsed *receipt.Receipt, source *autonomous.ReceiptSource) (*autonomous.VerificationSummary, error) {
	type statusSource struct {
		name   string
		status string
	}
	sources := make([]statusSource, 0, 3)
	ledgerStatus := strings.TrimSpace(run.VerificationStatus)
	if run.VerificationStatus != ledgerStatus {
		return nil, fmt.Errorf("verification: run_id %q ledger status contains leading or trailing whitespace", run.ID)
	}
	if ledgerStatus != "" {
		if err := validateVerificationStatus(ledgerStatus); err != nil {
			return nil, fmt.Errorf("verification: run_id %q ledger: %w", run.ID, err)
		}
		sources = append(sources, statusSource{name: "ledger", status: ledgerStatus})
	}
	if eventFound {
		sources = append(sources, statusSource{name: "verification_completed event", status: event.status})
	}
	if parsed != nil {
		if err := validateVerificationStatus(parsed.VerificationStatus); err != nil {
			return nil, fmt.Errorf("verification: run_id %q receipt: %w", run.ID, err)
		}
		sources = append(sources, statusSource{name: "receipt", status: parsed.VerificationStatus})
	}
	for i := 1; i < len(sources); i++ {
		if sources[i].status != sources[0].status {
			return nil, fmt.Errorf("verification: run_id %q %s status %q conflicts with %s status %q", run.ID, sources[i].name, sources[i].status, sources[0].name, sources[0].status)
		}
	}
	if eventFound && parsed != nil {
		got := normalizeVerificationEntries(parsed.Verification)
		want := normalizeVerificationEntries(event.entries)
		if !reflect.DeepEqual(got, want) {
			return nil, fmt.Errorf("verification: run_id %q receipt command results %#v conflict with verification_completed results %#v", run.ID, got, want)
		}
	}
	if len(sources) == 0 || sources[0].status == "not_run" {
		return nil, nil
	}

	status := autonomous.VerificationStatus(sources[0].status)
	summaryText := "Ledger run records verification " + sources[0].status + "."
	command := ""
	occurrenceID := ""
	evidence := []autonomous.EvidenceReference{{
		Kind:      autonomous.EvidenceKindLedger,
		Reference: "ledger:run:" + run.ID,
		Detail:    fmt.Sprintf("Ledger run records verification status %s.", sources[0].status),
	}}
	if eventFound {
		summaryText = event.summary
		command = event.command
		evidence = append(evidence, event.evidence...)
		if event.event != nil {
			occurrenceID = fmt.Sprintf("ledger-event-%d", event.event.ID)
		}
	}
	if parsed != nil && source != nil {
		evidence = append(evidence, autonomous.EvidenceReference{
			Kind:      autonomous.EvidenceKindReceipt,
			Reference: source.Path,
			Detail:    fmt.Sprintf("Receipt records verification status %s for run %s.", parsed.VerificationStatus, run.ID),
		})
	}
	return &autonomous.VerificationSummary{
		TaskID:       run.TaskID,
		Status:       status,
		Command:      command,
		Summary:      summaryText,
		RunID:        run.ID,
		OccurrenceID: occurrenceID,
		Evidence:     uniqueEvidence(evidence),
		Tiered:       event.tiered,
	}, nil
}

func normalizeVerificationEntries(entries []receipt.VerificationEntry) []receipt.VerificationEntry {
	out := make([]receipt.VerificationEntry, len(entries))
	for i, entry := range entries {
		entry.Command = strings.TrimSpace(entry.Command)
		entry.Status = strings.TrimSpace(entry.Status)
		out[i] = entry
	}
	return out
}

func validateVerificationStatus(status string) error {
	switch status {
	case "passed", "failed", "not_run":
		return nil
	default:
		return fmt.Errorf("unknown status %q", status)
	}
}

func collectGuidance(root, taskPath string, policy GuidancePolicy) ([]autonomous.GuidanceSource, error) {
	type candidate struct {
		path     string
		required bool
		explicit bool
	}
	automatic := guidanceChain(taskPath)
	candidates := make([]candidate, 0, len(automatic)+len(policy.Additional))
	seen := make(map[string]string, len(automatic)+len(policy.Additional))
	for _, path := range automatic {
		seen[path] = "automatic"
		candidates = append(candidates, candidate{path: path})
	}
	for i, configured := range policy.Additional {
		path := strings.TrimSpace(configured.Path)
		if path == "" {
			return nil, fmt.Errorf("guidance: additional[%d].path is required", i)
		}
		clean, _, err := resolveRepositoryPath(root, path)
		if err != nil {
			return nil, fmt.Errorf("guidance: additional[%d] unsafe path %q: %w", i, configured.Path, err)
		}
		if prior, exists := seen[clean]; exists {
			return nil, fmt.Errorf("guidance: additional[%d] path %q duplicates %s guidance path", i, clean, prior)
		}
		seen[clean] = fmt.Sprintf("additional[%d]", i)
		candidates = append(candidates, candidate{path: clean, required: configured.Required, explicit: true})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].path < candidates[j].path })

	guidance := make([]autonomous.GuidanceSource, 0, len(candidates))
	for _, item := range candidates {
		clean, abs, err := resolveRepositoryPath(root, item.path)
		if err != nil {
			return nil, fmt.Errorf("guidance: unsafe path %q: %w", item.path, err)
		}
		raw, err := os.ReadFile(abs)
		if errors.Is(err, os.ErrNotExist) && !item.required {
			continue
		}
		if err != nil {
			kind := "applicable"
			if item.explicit && item.required {
				kind = "required"
			}
			return nil, fmt.Errorf("guidance: read %s path %s: %w", kind, clean, err)
		}
		if !utf8.Valid(raw) {
			return nil, fmt.Errorf("guidance: path %s is not valid UTF-8", clean)
		}
		guidance = append(guidance, autonomous.GuidanceSource{
			ID:      "guidance:" + filepath.ToSlash(clean),
			Path:    clean,
			Content: raw,
		})
	}
	return guidance, nil
}

func guidanceChain(taskPath string) []string {
	dir := filepath.Clean(filepath.Dir(taskPath))
	paths := []string{"AGENTS.md"}
	if dir == "." {
		return paths
	}
	parts := strings.Split(filepath.ToSlash(dir), "/")
	current := ""
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		paths = append(paths, filepath.Join(current, "AGENTS.md"))
	}
	return paths
}

func resolveRepositoryPath(root, path string) (string, string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", "", pathguard.ErrEmptyPath
	}
	if filepath.IsAbs(path) {
		return "", "", pathguard.ErrAbsolutePath
	}
	clean := filepath.Clean(path)
	if clean == "." || clean != path {
		return "", "", fmt.Errorf("path %q must be a clean repository-relative path", path)
	}
	abs, err := pathguard.Resolve(root, clean)
	if err != nil {
		return "", "", err
	}
	return filepath.ToSlash(clean), abs, nil
}

func normalizeGitOptions(options GitOptions) (GitOptions, error) {
	options.Executable = strings.TrimSpace(options.Executable)
	if options.Executable == "" {
		options.Executable = defaultGitExecutable
	}
	if options.Timeout < 0 {
		return GitOptions{}, fmt.Errorf("git: timeout cannot be negative (got %s)", options.Timeout)
	}
	if options.Timeout == 0 {
		options.Timeout = defaultGitTimeout
	}
	if options.StdoutLimit < 0 {
		return GitOptions{}, fmt.Errorf("git: stdout_limit cannot be negative (got %d)", options.StdoutLimit)
	}
	if options.StdoutLimit == 0 {
		options.StdoutLimit = defaultGitStdoutCap
	}
	if options.StderrLimit < 0 {
		return GitOptions{}, fmt.Errorf("git: stderr_limit cannot be negative (got %d)", options.StderrLimit)
	}
	if options.StderrLimit == 0 {
		options.StderrLimit = defaultGitStderrCap
	}
	if options.CommandRunner == nil {
		options.CommandRunner = runner.Run
	}
	return options, nil
}

func captureHEAD(ctx context.Context, root string, options GitOptions, stage string) (string, error) {
	head, err := captureGitText(ctx, root, options, "HEAD "+stage, []string{"rev-parse", "--verify", "HEAD"})
	if err != nil {
		return "", err
	}
	if head == "" {
		return "", fmt.Errorf("git: HEAD %s returned empty required output", stage)
	}
	if strings.ContainsAny(head, "\r\n") || strings.TrimSpace(head) != head {
		return "", fmt.Errorf("git: HEAD %s returned malformed output %q", stage, head)
	}
	return head, nil
}

func captureGitText(ctx context.Context, root string, options GitOptions, operation string, args []string) (string, error) {
	command := runner.Command{
		Name:        options.Executable,
		Args:        append([]string(nil), args...),
		Dir:         root,
		Timeout:     options.Timeout,
		StdoutLimit: options.StdoutLimit,
		StderrLimit: options.StderrLimit,
	}
	result := options.CommandRunner(ctx, command)
	if result.TimedOut {
		return "", fmt.Errorf("git: %s timed out after %s", operation, options.Timeout)
	}
	if result.Err != nil {
		return "", fmt.Errorf("git: %s command failed: %w", operation, result.Err)
	}
	if result.ExitCode != 0 {
		stderr := strings.TrimSpace(result.Stderr)
		if stderr == "" {
			return "", fmt.Errorf("git: %s exited with code %d", operation, result.ExitCode)
		}
		return "", fmt.Errorf("git: %s exited with code %d: %s", operation, result.ExitCode, stderr)
	}
	if result.StdoutTruncatedBytes != 0 {
		return "", fmt.Errorf("git: %s stdout was truncated by %d byte(s)", operation, result.StdoutTruncatedBytes)
	}
	if result.StderrTruncatedBytes != 0 {
		return "", fmt.Errorf("git: %s stderr was truncated by %d byte(s)", operation, result.StderrTruncatedBytes)
	}
	return trimFinalCommandNewline(result.Stdout), nil
}

func trimFinalCommandNewline(value string) string {
	if strings.HasSuffix(value, "\r\n") {
		return value[:len(value)-2]
	}
	return strings.TrimSuffix(value, "\n")
}

func eventObject(event ledger.Event) (map[string]json.RawMessage, error) {
	if len(event.Payload) == 0 {
		return nil, errors.New("payload is required")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(event.Payload, &fields); err != nil {
		return nil, fmt.Errorf("payload is malformed JSON: %w", err)
	}
	if fields == nil {
		return nil, errors.New("payload must be a JSON object")
	}
	return fields, nil
}

func eventString(fields map[string]json.RawMessage, key string) (string, bool, error) {
	raw, present := fields[key]
	if !present {
		return "", false, nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", true, fmt.Errorf("%s must be a string", key)
	}
	return strings.TrimSpace(value), true, nil
}

func actionForPhase(phase string) (autonomous.Action, bool) {
	switch phase {
	case taskfile.PhaseImplement:
		return autonomous.ActionImplement, true
	case taskfile.PhaseAudit:
		return autonomous.ActionAudit, true
	case taskfile.PhaseDocument:
		return autonomous.ActionDocument, true
	case taskfile.PhaseSimplify:
		return autonomous.ActionSimplify, true
	default:
		return "", false
	}
}

func knownAction(action autonomous.Action) bool {
	switch action {
	case autonomous.ActionPlan, autonomous.ActionImplement, autonomous.ActionAudit, autonomous.ActionCorrect, autonomous.ActionDocument, autonomous.ActionSimplify, autonomous.ActionComplete, autonomous.ActionBlock:
		return true
	default:
		return false
	}
}

func knownProfile(profile autonomous.WorkerProfile) bool {
	switch profile {
	case autonomous.WorkerProfilePlanner, autonomous.WorkerProfileImplementer, autonomous.WorkerProfileAuditor, autonomous.WorkerProfileCorrector, autonomous.WorkerProfileDocumentor, autonomous.WorkerProfileSimplifier:
		return true
	default:
		return false
	}
}

func expectedProfile(action autonomous.Action) autonomous.WorkerProfile {
	switch action {
	case autonomous.ActionPlan:
		return autonomous.WorkerProfilePlanner
	case autonomous.ActionImplement:
		return autonomous.WorkerProfileImplementer
	case autonomous.ActionAudit:
		return autonomous.WorkerProfileAuditor
	case autonomous.ActionCorrect:
		return autonomous.WorkerProfileCorrector
	case autonomous.ActionDocument:
		return autonomous.WorkerProfileDocumentor
	case autonomous.ActionSimplify:
		return autonomous.WorkerProfileSimplifier
	default:
		return ""
	}
}

func runOutcome(run ledger.Run) string {
	summary := strings.Join(strings.Fields(strings.TrimSpace(run.Summary)), " ")
	if summary == "" {
		return run.Status
	}
	return run.Status + ": " + summary
}

func eventReference(runID string, event ledger.Event) string {
	return fmt.Sprintf("ledger:run:%s:event:%d", runID, event.ID)
}

func uniqueEvidence(input []autonomous.EvidenceReference) []autonomous.EvidenceReference {
	out := make([]autonomous.EvidenceReference, 0, len(input))
	seen := make(map[autonomous.EvidenceReference]struct{}, len(input))
	for _, evidence := range input {
		if _, exists := seen[evidence]; exists {
			continue
		}
		seen[evidence] = struct{}{}
		out = append(out, evidence)
	}
	return out
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
