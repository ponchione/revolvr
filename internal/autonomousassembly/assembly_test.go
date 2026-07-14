package autonomousassembly

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/ledger"
	"revolvr/internal/runner"
)

func TestAssembleCompleteRepositoryEvidence(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t, "task-1", true)
	head := gitOutput(t, repo, "rev-parse", "HEAD")
	writeTestFile(t, filepath.Join(repo, "tracked.txt"), []byte("dirty worktree\n"))

	receiptPath := filepath.Join(".revolvr", "receipts", "run-newest.md")
	receiptBytes := validReceiptBytes("run-newest", "task-1", "passed", head, "go test ./...")
	writeTestFile(t, filepath.Join(repo, receiptPath), receiptBytes)
	ledgerPath := filepath.Join(repo, defaultLedgerPath)
	store, err := ledger.OpenWithClock(ctx, ledgerPath, func() time.Time {
		return time.Date(2026, 7, 10, 16, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatalf("open ledger fixture: %v", err)
	}
	base := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	createHistoryRun(t, store, historyRunSpec{id: "run-old", taskID: "task-1", startedAt: base, status: ledger.StatusFailed, summary: "old failure", verification: "failed"})
	createHistoryRun(t, store, historyRunSpec{id: "run-middle", taskID: "task-1", startedAt: base.Add(time.Hour), status: ledger.StatusCompleted, summary: "middle success", verification: "passed"})
	createHistoryRun(t, store, historyRunSpec{id: "run-other", taskID: "task-other", startedAt: base.Add(3 * time.Hour), status: ledger.StatusCompleted, summary: "other task", verification: "passed"})
	createHistoryRun(t, store, historyRunSpec{
		id:           "run-newest",
		taskID:       "task-1",
		startedAt:    base.Add(2 * time.Hour),
		status:       ledger.StatusCompleted,
		summary:      "newest success",
		verification: "passed",
		commitSHA:    head,
		receiptPath:  receiptPath,
		command:      "go test ./...",
	})
	if err := store.Close(); err != nil {
		t.Fatalf("close ledger fixture: %v", err)
	}

	state := validState("task-1")
	audit := validAudit("task-1")
	stateBefore := mustJSON(t, state)
	auditBefore := mustJSON(t, audit)
	in := Input{
		RepositoryRoot: repo,
		TaskID:         "task-1",
		State:          state,
		Audit:          &audit,
		HistoryPolicy:  HistoryPolicy{CollectionLimit: 2, RenderLimit: 1},
	}

	first, err := Assemble(ctx, in)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}
	second, err := Assemble(ctx, in)
	if err != nil {
		t.Fatalf("second Assemble() error = %v", err)
	}
	if !bytes.Equal(first.Markdown, second.Markdown) {
		t.Fatal("repeated dossier Markdown differs")
	}
	firstManifest, err := autonomous.MarshalTaskDossierManifest(first.Manifest)
	if err != nil {
		t.Fatalf("marshal first manifest: %v", err)
	}
	secondManifest, err := autonomous.MarshalTaskDossierManifest(second.Manifest)
	if err != nil {
		t.Fatalf("marshal second manifest: %v", err)
	}
	if !bytes.Equal(firstManifest, secondManifest) {
		t.Fatal("repeated manifest JSON differs")
	}

	taskRaw := readTestFile(t, filepath.Join(repo, ".agent", "tasks", "task-1.md"))
	taskRecord := manifestSource(t, first.Manifest, autonomous.DossierSourceKindTaskSpec, "task-spec:task-1")
	assertSourceBytes(t, taskRecord, taskRaw)
	if got, want := taskRecord.Path, ".agent/tasks/task-1.md"; got != want {
		t.Fatalf("task source path = %q, want %q", got, want)
	}
	receiptRecord := manifestSource(t, first.Manifest, autonomous.DossierSourceKindReceipt, "receipt:run-newest")
	assertSourceBytes(t, receiptRecord, receiptBytes)
	if got, want := receiptRecord.Path, filepath.ToSlash(receiptPath); got != want {
		t.Fatalf("receipt path = %q, want %q", got, want)
	}
	for _, path := range []string{".agent/AGENTS.md", ".agent/tasks/AGENTS.md", "AGENTS.md"} {
		raw := readTestFile(t, filepath.Join(repo, filepath.FromSlash(path)))
		record := manifestSource(t, first.Manifest, autonomous.DossierSourceKindRepositoryGuidance, "guidance:"+path)
		assertSourceBytes(t, record, raw)
		if got := record.Path; got != path {
			t.Fatalf("guidance source path = %q, want %q", got, path)
		}
	}
	runsRecord := manifestSource(t, first.Manifest, autonomous.DossierSourceKindRecentRuns, "recent-runs")
	if runsRecord.Items == nil || *runsRecord.Items != (autonomous.DossierItemCounts{Total: 2, Included: 1, Omitted: 1}) {
		t.Fatalf("recent run counts = %#v, want 2/1/1", runsRecord.Items)
	}
	if runsRecord.SourceWindow == nil || *runsRecord.SourceWindow != (autonomous.DossierSourceWindow{Limit: 2, HasOlderItems: true}) {
		t.Fatalf("recent source window = %#v", runsRecord.SourceWindow)
	}
	gitEvidence := autonomous.EvidenceReference{
		Kind:      autonomous.EvidenceKindGit,
		Reference: "git:head:" + head,
		Detail:    "Read-only HEAD, worktree status, and diff summary captured from one stable Git snapshot.",
	}
	expectedGit := autonomous.GitSnapshot{
		Head:           head,
		WorktreeStatus: gitExactOutput(t, repo, "-c", "color.ui=false", "-c", "core.quotePath=true", "status", "--short", "--untracked-files=all"),
		DiffSummary:    gitExactOutput(t, repo, "-c", "color.ui=false", "-c", "core.quotePath=true", "diff", "--stat", "--no-ext-diff", "--no-renames", "HEAD", "--"),
		Evidence:       &gitEvidence,
	}
	assertSourceBytes(t, manifestSource(t, first.Manifest, autonomous.DossierSourceKindGitSnapshot, "git-snapshot"), mustJSON(t, expectedGit))

	markdown := string(first.Markdown)
	for _, want := range []string{
		"### Run 1: run-newest",
		"- Status: passed",
		"- Command/tier: go test ./...",
		"- Run ID: run-newest",
		"- HEAD/baseline: " + head,
		" M tracked.txt",
		"tracked.txt |",
		"older selected-task items exist beyond the bounded source window",
		"### Guidance 1: .agent/AGENTS.md",
		"### Guidance 2: .agent/tasks/AGENTS.md",
		"### Guidance 3: AGENTS.md",
	} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("dossier missing %q:\n%s", want, markdown)
		}
	}
	for _, notWant := range []string{"run-other", "### Run 2: run-middle", "docs/AGENTS.md", "unrelated nested guidance"} {
		if strings.Contains(markdown, notWant) {
			t.Fatalf("dossier unexpectedly contains %q:\n%s", notWant, markdown)
		}
	}
	if got := mustJSON(t, in.State); !bytes.Equal(got, stateBefore) {
		t.Fatalf("state mutated:\ngot  %s\nwant %s", got, stateBefore)
	}
	if got := mustJSON(t, *in.Audit); !bytes.Equal(got, auditBefore) {
		t.Fatalf("audit mutated:\ngot  %s\nwant %s", got, auditBefore)
	}
}

func TestAssembleSparseDoesNotCreateRuntimeStateOrMutateRepository(t *testing.T) {
	repo := newTestRepository(t, "task-sparse", false)
	before := gitOutput(t, repo, "status", "--short", "--untracked-files=all")
	if _, err := os.Stat(filepath.Join(repo, ".revolvr")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("precondition .revolvr stat = %v, want not exist", err)
	}

	result, err := Assemble(context.Background(), Input{
		RepositoryRoot: repo,
		TaskID:         "task-sparse",
		State:          validState("task-sparse"),
		HistoryPolicy:  HistoryPolicy{},
	})
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}
	for _, want := range []string{
		"No verification evidence supplied.",
		"No audit report supplied.",
		"No recent run summaries supplied.",
		"No repository guidance sources supplied.",
		"- Worktree status: clean",
		"- Diff summary: none",
	} {
		if !strings.Contains(string(result.Markdown), want) {
			t.Fatalf("sparse dossier missing %q:\n%s", want, result.Markdown)
		}
	}
	if _, err := os.Stat(filepath.Join(repo, ".revolvr")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("assembly created .revolvr: stat error = %v", err)
	}
	after := gitOutput(t, repo, "status", "--short", "--untracked-files=all")
	if before != after || after != "" {
		t.Fatalf("git status changed from %q to %q", before, after)
	}
}

func TestAssembleRepositoryMapCacheHitGuidanceInvalidationAndCorruptionFallback(t *testing.T) {
	repo := newTestRepository(t, "task-cache", false)
	in := Input{
		RepositoryRoot: repo, TaskID: "task-cache", State: validState("task-cache"),
		RepositoryMapPolicy: RepositoryMapPolicy{Enabled: true, MaxPaths: 100, MaxBytes: 64 * 1024},
		Role:                autonomous.DossierRoleSupervisor,
	}
	first, err := Assemble(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if first.Manifest.Cache == nil || first.Manifest.Cache.Result != "recomputed" || first.Manifest.Cache.Diagnostic != "cache_miss" {
		t.Fatalf("first cache=%+v", first.Manifest.Cache)
	}
	second, err := Assemble(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if second.Manifest.Cache == nil || second.Manifest.Cache.Result != "hit" || second.Manifest.Cache.Key != first.Manifest.Cache.Key {
		t.Fatalf("second cache=%+v", second.Manifest.Cache)
	}
	if !bytes.Equal(first.Markdown, second.Markdown) {
		t.Fatal("cache hit projection differs from recomputation")
	}

	contentPath := filepath.Join(repo, ".revolvr", "cache", "dossier", "v1", first.Manifest.Cache.Key, "repository-map.md")
	writeTestFile(t, contentPath, []byte("corrupt\n"))
	third, err := Assemble(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if third.Manifest.Cache == nil || third.Manifest.Cache.Result != "recomputed" || !strings.Contains(third.Manifest.Cache.Diagnostic, "output_identity_mismatch") || !bytes.Equal(third.Markdown, first.Markdown) {
		t.Fatalf("corrupt fallback cache=%+v", third.Manifest.Cache)
	}

	writeTestFile(t, filepath.Join(repo, "AGENTS.md"), []byte("# Changed guidance at the same HEAD\n"))
	changed, err := Assemble(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Manifest.Cache == nil || changed.Manifest.Cache.Key == first.Manifest.Cache.Key || changed.Manifest.Cache.Result != "recomputed" {
		t.Fatalf("changed guidance cache=%+v", changed.Manifest.Cache)
	}
}

func TestAssembleRepositoryMapIncludesSafeDotDotPrefixedNames(t *testing.T) {
	repo := newTestRepository(t, "task-dotdot", false)
	writeTestFile(t, filepath.Join(repo, "..foo"), []byte("safe prefix\n"))
	writeTestFile(t, filepath.Join(repo, "..well-known", "file"), []byte("safe nested prefix\n"))
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-q", "-m", "Add safe dot-dot-prefixed paths")

	result, err := Assemble(context.Background(), Input{
		RepositoryRoot: repo,
		TaskID:         "task-dotdot",
		State:          validState("task-dotdot"),
		RepositoryMapPolicy: RepositoryMapPolicy{
			Enabled:  true,
			MaxPaths: 100,
			MaxBytes: 64 * 1024,
		},
		Role: autonomous.DossierRoleSupervisor,
	})
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}
	for _, want := range []string{"..foo [file]", "..well-known/file [file]"} {
		if !strings.Contains(string(result.Markdown), want) {
			t.Fatalf("repository-map dossier missing %q:\n%s", want, result.Markdown)
		}
	}
}

func TestAssembleRepositoryMapFromSHA256Repository(t *testing.T) {
	repo := newSHA256TestRepository(t, "task-sha256")
	head := gitOutput(t, repo, "rev-parse", "HEAD")
	tree := gitOutput(t, repo, "rev-parse", "HEAD^{tree}")
	if len(head) != 64 || len(tree) != 64 {
		t.Fatalf("SHA-256 repository identities have lengths %d/%d, want 64/64", len(head), len(tree))
	}

	in := Input{
		RepositoryRoot: repo,
		TaskID:         "task-sha256",
		State:          validState("task-sha256"),
		RepositoryMapPolicy: RepositoryMapPolicy{
			Enabled:  true,
			MaxPaths: 100,
			MaxBytes: 64 * 1024,
		},
		Role: autonomous.DossierRoleSupervisor,
	}
	first, err := Assemble(context.Background(), in)
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}
	if first.Manifest.Cache == nil || first.Manifest.Cache.Result != "recomputed" {
		t.Fatalf("first repository-map cache = %+v, want recomputed", first.Manifest.Cache)
	}
	for _, want := range []string{"Commit: " + head, "Tree: " + tree, "tracked.txt [file]"} {
		if !strings.Contains(string(first.Markdown), want) {
			t.Fatalf("SHA-256 dossier missing %q:\n%s", want, first.Markdown)
		}
	}
	second, err := Assemble(context.Background(), in)
	if err != nil {
		t.Fatalf("second Assemble() error = %v", err)
	}
	if second.Manifest.Cache == nil || second.Manifest.Cache.Result != "hit" || second.Manifest.Cache.Key != first.Manifest.Cache.Key {
		t.Fatalf("second repository-map cache = %+v, want matching hit", second.Manifest.Cache)
	}
}

func TestAssembleTaskStateAndAuditIdentityFailures(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*testing.T, string, *Input)
		want    string
	}{
		{name: "missing task", prepare: func(_ *testing.T, _ string, in *Input) { in.TaskID = "missing"; in.State = validState("missing") }, want: `task: canonical task_id "missing" was not found`},
		{name: "state mismatch", prepare: func(_ *testing.T, _ string, in *Input) { in.State = validState("other") }, want: `requested task_id "task-1" does not match execution_state task_id "other"`},
		{name: "audit mismatch", prepare: func(_ *testing.T, _ string, in *Input) { audit := validAudit("other"); in.Audit = &audit }, want: `requested task_id "task-1" does not match audit task_id "other"`},
		{name: "duplicate task", prepare: func(t *testing.T, repo string, _ *Input) {
			writeTestFile(t, filepath.Join(repo, ".agent", "tasks", "duplicate.md"), []byte("---\nid: task-1\n---\n# Duplicate\n"))
		}, want: `task id "task-1" is duplicated`},
		{name: "malformed task", prepare: func(t *testing.T, repo string, _ *Input) {
			writeTestFile(t, filepath.Join(repo, ".agent", "tasks", "bad.md"), []byte("## Missing H1\n"))
		}, want: "no H1 title"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newTestRepository(t, "task-1", false)
			in := Input{RepositoryRoot: repo, TaskID: "task-1", State: validState("task-1")}
			tt.prepare(t, repo, &in)
			_, err := Assemble(context.Background(), in)
			assertAssemblyError(t, err, tt.want)
		})
	}
}

func TestAssembleHistoryIdentityAndLimitFailures(t *testing.T) {
	repo := newTestRepository(t, "task-1", false)
	started := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	valid := ledger.RunWithEvents{Run: ledger.Run{ID: "run-1", TaskID: "task-1", Task: "task", Status: ledger.StatusRunning, StartedAt: started}}
	tests := []struct {
		name    string
		policy  HistoryPolicy
		history []ledger.RunWithEvents
		want    string
	}{
		{name: "negative collection", policy: HistoryPolicy{CollectionLimit: -1}, want: "collection_limit cannot be negative"},
		{name: "negative render", policy: HistoryPolicy{CollectionLimit: 1, RenderLimit: -1}, want: "render_limit cannot be negative"},
		{name: "contradictory", policy: HistoryPolicy{CollectionLimit: 1, RenderLimit: 2}, want: "render_limit 2 exceeds collection_limit 1"},
		{name: "run task mismatch", policy: HistoryPolicy{CollectionLimit: 1, RenderLimit: 1}, history: []ledger.RunWithEvents{{Run: ledger.Run{ID: "run-wrong", TaskID: "other", Status: ledger.StatusRunning, StartedAt: started}}}, want: `run_id "run-wrong" task_id "other"`},
		{name: "duplicate run ids", policy: HistoryPolicy{CollectionLimit: 2, RenderLimit: 2}, history: []ledger.RunWithEvents{valid, valid}, want: `duplicates run_id "run-1"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &staticHistoryReader{history: tt.history}
			_, err := Assemble(context.Background(), Input{
				RepositoryRoot: repo,
				TaskID:         "task-1",
				State:          validState("task-1"),
				HistoryPolicy:  tt.policy,
				HistoryReader:  reader,
			})
			assertAssemblyError(t, err, tt.want)
		})
	}
}

func TestCollectHistoryZeroBoundsAndStableOrdering(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	all := []ledger.RunWithEvents{
		{Run: ledger.Run{ID: "run-z", TaskID: "task-1", Status: ledger.StatusRunning, StartedAt: base}},
		{Run: ledger.Run{ID: "run-a", TaskID: "task-1", Status: ledger.StatusRunning, StartedAt: base}},
		{Run: ledger.Run{ID: "run-old", TaskID: "task-1", Status: ledger.StatusRunning, StartedAt: base.Add(-time.Hour)}},
	}
	tests := []struct {
		name       string
		collection int
		wantIDs    []string
		wantOlder  bool
		wantCalls  int
	}{
		{name: "zero", collection: 0},
		{name: "below", collection: 4, wantIDs: []string{"run-a", "run-z", "run-old"}, wantCalls: 1},
		{name: "exact", collection: 3, wantIDs: []string{"run-a", "run-z", "run-old"}, wantCalls: 1},
		{name: "above", collection: 2, wantIDs: []string{"run-a", "run-z"}, wantOlder: true, wantCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &staticHistoryReader{history: all, honorLimit: true}
			history, older, err := collectHistory(context.Background(), root, Input{
				TaskID:        "task-1",
				HistoryPolicy: HistoryPolicy{CollectionLimit: tt.collection},
				HistoryReader: reader,
			})
			if err != nil {
				t.Fatalf("collectHistory() error = %v", err)
			}
			ids := make([]string, len(history))
			for i := range history {
				ids[i] = history[i].Run.ID
			}
			if !slices.Equal(ids, tt.wantIDs) || older != tt.wantOlder || reader.calls != tt.wantCalls {
				t.Fatalf("history ids/older/calls = %#v/%t/%d, want %#v/%t/%d", ids, older, reader.calls, tt.wantIDs, tt.wantOlder, tt.wantCalls)
			}
		})
	}
}

func TestAssembleDoesNotMutateInjectedHistoryAndIgnoresMalformedIrrelevantPayload(t *testing.T) {
	repo := newTestRepository(t, "task-1", false)
	startedAt := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	events := []ledger.Event{
		{ID: 2, RunID: "run-1", Type: ledger.EventCodexJSONEvent, Payload: json.RawMessage(`{malformed`), CreatedAt: startedAt.Add(time.Minute)},
		testEvent(t, 1, "run-1", ledger.EventTaskSelected, map[string]any{"task_id": "task-1", "phase": "audit", "profile_name": "auditor"}),
	}
	reader := &staticHistoryReader{history: []ledger.RunWithEvents{{
		Run:    ledger.Run{ID: "run-1", TaskID: "task-1", Task: "task", Status: ledger.StatusRunning, StartedAt: startedAt},
		Events: events,
	}}}
	before := cloneHistory(reader.history)
	payloadBefore := append([]byte(nil), reader.history[0].Events[0].Payload...)

	result, err := Assemble(context.Background(), Input{
		RepositoryRoot: repo,
		TaskID:         "task-1",
		State:          validState("task-1"),
		HistoryPolicy:  HistoryPolicy{CollectionLimit: 1, RenderLimit: 1},
		HistoryReader:  reader,
	})
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}
	if !strings.Contains(string(result.Markdown), "### Run 1: run-1") {
		t.Fatalf("assembled history missing run-1:\n%s", result.Markdown)
	}
	if got := reader.history; !reflect.DeepEqual(got, before) {
		t.Fatalf("caller history mutated:\ngot  %#v\nwant %#v", got, before)
	}
	if got := reader.history[0].Events[0].Payload; !bytes.Equal(got, payloadBefore) {
		t.Fatalf("caller payload bytes mutated: got %q, want %q", got, payloadBefore)
	}
}

func TestAssembleRejectsMalformedIdentityBearingEventPayload(t *testing.T) {
	repo := newTestRepository(t, "task-1", false)
	startedAt := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	run := ledger.Run{ID: "run-1", TaskID: "task-1", Task: "task", Status: ledger.StatusRunning, StartedAt: startedAt}
	event := ledger.Event{ID: 1, RunID: run.ID, Type: ledger.EventTaskSelected, Payload: json.RawMessage(`{malformed`), CreatedAt: startedAt}

	_, err := Assemble(context.Background(), Input{
		RepositoryRoot: repo,
		TaskID:         "task-1",
		State:          validState("task-1"),
		HistoryPolicy:  HistoryPolicy{CollectionLimit: 1, RenderLimit: 1},
		HistoryReader:  &staticHistoryReader{history: []ledger.RunWithEvents{{Run: run, Events: []ledger.Event{event}}}},
	})
	assertAssemblyError(t, err, "task_selected: payload is malformed JSON")
}

func TestAssembleReceiptAndProvenanceFailures(t *testing.T) {
	tests := []struct {
		name         string
		recordedPath string
		write        func(*testing.T, string, string)
		mutateRun    func(*ledger.Run)
		want         string
	}{
		{name: "unsafe absolute", recordedPath: "/tmp/receipt.md", want: "unsafe recorded path"},
		{name: "unsafe traversal", recordedPath: "../receipt.md", want: "unsafe recorded path"},
		{name: "missing recorded receipt", recordedPath: ".revolvr/receipts/missing.md", want: "read .revolvr/receipts/missing.md"},
		{name: "malformed receipt", recordedPath: ".revolvr/receipts/bad.md", write: func(t *testing.T, repo, path string) {
			writeTestFile(t, filepath.Join(repo, path), []byte("not a receipt"))
		}, want: "parse .revolvr/receipts/bad.md"},
		{name: "receipt task mismatch", recordedPath: ".revolvr/receipts/wrong-task.md", write: func(t *testing.T, repo, path string) {
			writeTestFile(t, filepath.Join(repo, path), validReceiptBytes("run-1", "other", "not_run", "", ""))
		}, want: `task_id "other" does not match dossier task_id "task-1"`},
		{name: "receipt run mismatch", recordedPath: ".revolvr/receipts/wrong-run.md", write: func(t *testing.T, repo, path string) {
			writeTestFile(t, filepath.Join(repo, path), validReceiptBytes("other-run", "task-1", "not_run", "", ""))
		}, want: `run_id "other-run" does not match ledger run_id "run-1"`},
		{name: "commit conflict", recordedPath: ".revolvr/receipts/commit.md", write: func(t *testing.T, repo, path string) {
			writeTestFile(t, filepath.Join(repo, path), validReceiptBytes("run-1", "task-1", "not_run", "receipt-sha", ""))
		}, mutateRun: func(run *ledger.Run) { run.CommitSHA = "ledger-sha" }, want: "receipt commit_sha \"receipt-sha\" conflicts with ledger commit_sha \"ledger-sha\""},
		{name: "verification conflict", recordedPath: ".revolvr/receipts/verification.md", write: func(t *testing.T, repo, path string) {
			writeTestFile(t, filepath.Join(repo, path), validReceiptBytes("run-1", "task-1", "failed", "", "go test ./..."))
		}, mutateRun: func(run *ledger.Run) { run.VerificationStatus = "passed" }, want: `receipt status "failed" conflicts with ledger status "passed"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newTestRepository(t, "task-1", false)
			if tt.write != nil {
				tt.write(t, repo, tt.recordedPath)
			}
			run := ledger.Run{ID: "run-1", TaskID: "task-1", Task: "task", Status: ledger.StatusRunning, StartedAt: time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)}
			if tt.mutateRun != nil {
				tt.mutateRun(&run)
			}
			event := testEvent(t, 1, run.ID, ledger.EventRunArtifacts, map[string]any{"receipt_path": tt.recordedPath})
			reader := &staticHistoryReader{history: []ledger.RunWithEvents{{Run: run, Events: []ledger.Event{event}}}}
			_, err := Assemble(context.Background(), Input{
				RepositoryRoot: repo,
				TaskID:         "task-1",
				State:          validState("task-1"),
				HistoryPolicy:  HistoryPolicy{CollectionLimit: 1, RenderLimit: 1},
				HistoryReader:  reader,
			})
			assertAssemblyError(t, err, tt.want)
		})
	}
}

func TestAssembleRejectsSymlinkEscapedReceipt(t *testing.T) {
	repo := newTestRepository(t, "task-1", false)
	outside := t.TempDir()
	writeTestFile(t, filepath.Join(outside, "receipt.md"), validReceiptBytes("run-1", "task-1", "not_run", "", ""))
	if err := os.MkdirAll(filepath.Join(repo, ".revolvr"), 0o755); err != nil {
		t.Fatalf("mkdir runtime: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(repo, ".revolvr", "escape")); err != nil {
		t.Fatalf("symlink escape: %v", err)
	}
	run := ledger.Run{ID: "run-1", TaskID: "task-1", Status: ledger.StatusRunning, StartedAt: time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)}
	event := testEvent(t, 1, run.ID, ledger.EventRunArtifacts, map[string]any{"receipt_path": ".revolvr/escape/receipt.md"})
	_, err := Assemble(context.Background(), Input{
		RepositoryRoot: repo, TaskID: "task-1", State: validState("task-1"),
		HistoryPolicy: HistoryPolicy{CollectionLimit: 1, RenderLimit: 1},
		HistoryReader: &staticHistoryReader{history: []ledger.RunWithEvents{{Run: run, Events: []ledger.Event{event}}}},
	})
	assertAssemblyError(t, err, "unsafe recorded path")
}

func TestAssembleGuidanceFailures(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		required bool
		write    []byte
		want     string
	}{
		{name: "unsafe", path: "../AGENTS.md", required: true, want: "unsafe path"},
		{name: "missing required", path: "docs/required.md", required: true, want: "read required path docs/required.md"},
		{name: "invalid utf8", path: "docs/rules.md", required: true, write: []byte{0xff}, want: "is not valid UTF-8"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newTestRepository(t, "task-1", false)
			if tt.write != nil {
				writeTestFile(t, filepath.Join(repo, tt.path), tt.write)
			}
			_, err := Assemble(context.Background(), Input{
				RepositoryRoot: repo,
				TaskID:         "task-1",
				State:          validState("task-1"),
				GuidancePolicy: GuidancePolicy{Additional: []GuidancePath{{Path: tt.path, Required: tt.required}}},
			})
			assertAssemblyError(t, err, tt.want)
		})
	}
}

func TestAssembleMalformedPresentLedgerFailsWithoutInitializingIt(t *testing.T) {
	repo := newTestRepository(t, "task-1", false)
	path := filepath.Join(repo, defaultLedgerPath)
	writeTestFile(t, path, []byte("not sqlite"))
	before := readTestFile(t, path)
	_, err := Assemble(context.Background(), Input{
		RepositoryRoot: repo,
		TaskID:         "task-1",
		State:          validState("task-1"),
		HistoryPolicy:  HistoryPolicy{CollectionLimit: 1, RenderLimit: 1},
	})
	assertAssemblyError(t, err, "ledger: query recent runs")
	if after := readTestFile(t, path); !bytes.Equal(after, before) {
		t.Fatalf("malformed ledger mutated: got %q, want %q", after, before)
	}
}

func TestAssembleGitCommandFailuresAndReadOnlyArguments(t *testing.T) {
	repo := repositoryWithTaskOnly(t, "task-1")
	baseInput := Input{RepositoryRoot: repo, TaskID: "task-1", State: validState("task-1")}

	t.Run("stable and read only", func(t *testing.T) {
		fake := &fakeGitRunner{heads: []string{"abc123\n", "abc123\n"}, status: " M tracked.go\n", diff: " tracked.go | 1 +\n"}
		in := baseInput
		in.Git = GitOptions{Executable: "git-test", Timeout: 2 * time.Second, StdoutLimit: 1234, StderrLimit: 4321, CommandRunner: fake.run}
		result, err := Assemble(context.Background(), in)
		if err != nil {
			t.Fatalf("Assemble() error = %v", err)
		}
		if !strings.Contains(string(result.Markdown), "- HEAD/baseline: abc123") {
			t.Fatalf("git snapshot missing:\n%s", result.Markdown)
		}
		if len(fake.commands) != 4 {
			t.Fatalf("git command count = %d, want 4", len(fake.commands))
		}
		for i, command := range fake.commands {
			if command.Name != "git-test" || command.Dir != repo || command.Timeout != 2*time.Second || command.StdoutLimit != 1234 || command.StderrLimit != 4321 {
				t.Fatalf("command[%d] framing = %#v", i, command)
			}
			joined := strings.Join(command.Args, " ")
			for _, forbidden := range []string{" add ", " commit ", " reset ", " restore ", " checkout ", " clean "} {
				if strings.Contains(" "+joined+" ", forbidden) {
					t.Fatalf("mutating git command issued: %v", command.Args)
				}
			}
		}
	})

	tests := []struct {
		name   string
		result runner.Result
		want   string
	}{
		{name: "failure", result: runner.Result{ExitCode: 128, Stderr: "fatal"}, want: "exited with code 128"},
		{name: "runner error", result: runner.Result{ExitCode: -1, Err: errors.New("git missing")}, want: "command failed: git missing"},
		{name: "timeout", result: runner.Result{ExitCode: -1, Err: context.DeadlineExceeded, TimedOut: true}, want: "timed out after"},
		{name: "stdout truncation", result: runner.Result{ExitCode: 0, Stdout: "abc", StdoutTruncatedBytes: 2}, want: "stdout was truncated by 2"},
		{name: "stderr truncation", result: runner.Result{ExitCode: 0, Stdout: "abc\n", StderrTruncatedBytes: 3}, want: "stderr was truncated by 3"},
		{name: "empty head", result: runner.Result{ExitCode: 0}, want: "returned empty required output"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := baseInput
			in.Git.CommandRunner = func(context.Context, runner.Command) runner.Result { return tt.result }
			_, err := Assemble(context.Background(), in)
			assertAssemblyError(t, err, tt.want)
		})
	}
}

func TestAssembleRejectsChangingHEADWithBothIdentities(t *testing.T) {
	repo := repositoryWithTaskOnly(t, "task-1")
	fake := &fakeGitRunner{heads: []string{"before\n", "after\n"}}
	_, err := Assemble(context.Background(), Input{
		RepositoryRoot: repo,
		TaskID:         "task-1",
		State:          validState("task-1"),
		Git:            GitOptions{CommandRunner: fake.run},
	})
	assertAssemblyError(t, err, `HEAD before "before", HEAD after "after"`)
}

func TestAssembleRejectsRealHEADChangeDuringCollection(t *testing.T) {
	repo := newTestRepository(t, "task-1", false)
	writeTestFile(t, filepath.Join(repo, "new.txt"), []byte("new commit\n"))
	committed := false
	commandRunner := func(ctx context.Context, command runner.Command) runner.Result {
		result := runner.Run(ctx, command)
		if !committed && containsArg(command.Args, "status") {
			committed = true
			gitRun(t, repo, "add", "new.txt")
			gitRun(t, repo, "commit", "-q", "-m", "Change HEAD during assembly")
		}
		return result
	}
	_, err := Assemble(context.Background(), Input{
		RepositoryRoot: repo,
		TaskID:         "task-1",
		State:          validState("task-1"),
		Git:            GitOptions{CommandRunner: commandRunner},
	})
	assertAssemblyError(t, err, "snapshot changed during assembly")
}

type staticHistoryReader struct {
	history    []ledger.RunWithEvents
	err        error
	calls      int
	honorLimit bool
}

func (r *staticHistoryReader) ListRecentRunsForTaskWithEvents(_ context.Context, _ string, limit int) ([]ledger.RunWithEvents, error) {
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	history := append([]ledger.RunWithEvents(nil), r.history...)
	if r.honorLimit && len(history) > limit {
		history = history[:limit]
	}
	return history, nil
}

type fakeGitRunner struct {
	heads    []string
	headCall int
	status   string
	diff     string
	commands []runner.Command
}

func (f *fakeGitRunner) run(_ context.Context, command runner.Command) runner.Result {
	copyCommand := command
	copyCommand.Args = append([]string(nil), command.Args...)
	f.commands = append(f.commands, copyCommand)
	switch {
	case containsArg(command.Args, "rev-parse"):
		index := f.headCall
		f.headCall++
		if index >= len(f.heads) {
			index = len(f.heads) - 1
		}
		if index < 0 {
			return runner.Result{ExitCode: 0}
		}
		return runner.Result{ExitCode: 0, Stdout: f.heads[index]}
	case containsArg(command.Args, "status"):
		return runner.Result{ExitCode: 0, Stdout: f.status}
	case containsArg(command.Args, "diff"):
		return runner.Result{ExitCode: 0, Stdout: f.diff}
	default:
		return runner.Result{ExitCode: 2, Stderr: "unexpected git command"}
	}
}

type historyRunSpec struct {
	id           string
	taskID       string
	startedAt    time.Time
	status       string
	summary      string
	verification string
	commitSHA    string
	receiptPath  string
	command      string
}

func createHistoryRun(t *testing.T, store *ledger.Store, spec historyRunSpec) {
	t.Helper()
	completed := spec.startedAt.Add(5 * time.Minute)
	run, err := store.CreateRun(context.Background(), ledger.RunSpec{
		ID:                 spec.id,
		TaskID:             spec.taskID,
		Task:               "task " + spec.taskID,
		Status:             spec.status,
		Summary:            spec.summary,
		StartedAt:          spec.startedAt,
		CompletedAt:        &completed,
		DurationSeconds:    300,
		VerificationStatus: spec.verification,
		CommitSHA:          spec.commitSHA,
	})
	if err != nil {
		t.Fatalf("create run %s: %v", spec.id, err)
	}
	if _, err := store.AppendEvent(context.Background(), run.ID, ledger.EventTaskSelected, map[string]any{
		"task_id": spec.taskID, "phase": "implement", "profile_name": "implementer",
	}); err != nil {
		t.Fatalf("append task_selected %s: %v", run.ID, err)
	}
	if spec.receiptPath != "" {
		if _, err := store.AppendEvent(context.Background(), run.ID, ledger.EventRunArtifacts, map[string]any{"receipt_path": spec.receiptPath}); err != nil {
			t.Fatalf("append artifacts %s: %v", run.ID, err)
		}
	}
	if spec.verification == "passed" || spec.verification == "failed" {
		commands := []map[string]any{}
		if spec.command != "" {
			exitCode := 0
			if spec.verification == "failed" {
				exitCode = 1
			}
			commands = append(commands, map[string]any{"command": spec.command, "status": spec.verification, "exit_code": exitCode})
		}
		if _, err := store.AppendEvent(context.Background(), run.ID, ledger.EventVerificationCompleted, map[string]any{
			"status": spec.verification, "message": "verification " + spec.verification, "commands": commands,
		}); err != nil {
			t.Fatalf("append verification %s: %v", run.ID, err)
		}
	}
	if spec.commitSHA != "" {
		if _, err := store.AppendEvent(context.Background(), run.ID, ledger.EventCommitCreated, map[string]any{"commit_sha": spec.commitSHA}); err != nil {
			t.Fatalf("append commit %s: %v", run.ID, err)
		}
	}
}

func validState(taskID string) autonomous.ExecutionState {
	return autonomous.ExecutionState{
		SchemaVersion: autonomous.ExecutionStateSchemaVersion,
		TaskID:        taskID,
		Lifecycle:     autonomous.LifecycleStateWorking,
		Attempts: autonomous.AttemptState{
			RetryBudget:       autonomous.CountBudget{Mode: autonomous.BudgetModeUnset},
			ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnset},
			TokenBudget:       autonomous.CountBudget{Mode: autonomous.BudgetModeUnset},
		},
	}
}

func validAudit(taskID string) autonomous.AuditReport {
	return autonomous.AuditReport{
		TaskID:      taskID,
		Disposition: autonomous.AuditDispositionClean,
		Rationale:   "The supplied audit is clean.",
		Inputs: []autonomous.EvidenceReference{{
			Kind: autonomous.EvidenceKindAudit, Reference: "audit:latest", Detail: "Caller-supplied validated audit evidence.",
		}},
	}
}

func validReceiptBytes(runID, taskID, verificationStatus, commitSHA, command string) []byte {
	verification := "verification: []\n"
	verificationBody := "- Not run."
	if command != "" {
		exitCode := 0
		if verificationStatus == "failed" {
			exitCode = 1
		}
		verification = fmt.Sprintf("verification:\n  - command: %s\n    exit_code: %d\n    status: %s\n", command, exitCode, verificationStatus)
		verificationBody = fmt.Sprintf("- `%s` — %s (exit %d)", command, verificationStatus, exitCode)
	}
	return []byte(fmt.Sprintf(`---
schema_version: revolvr.receipt.v1
run_id: %s
pass_id: %s
task_id: %s
task: assembled task
verdict: completed
timestamp: 2026-07-10T14:05:00Z
codex_exit_code: 0
verification_status: %s
commit_sha: %s
changed_files: []
%smetrics:
  input_tokens: 1
  output_tokens: 1
  duration_seconds: 300
---
## Summary
Complete.

## Changed Files
- None.

## Verification
%s

## Concerns
- None.

## Next Steps
- None.
`, runID, runID, taskID, verificationStatus, commitSHA, verification, verificationBody))
}

func newTestRepository(t *testing.T, taskID string, guidance bool) string {
	t.Helper()
	repo := repositoryWithTaskOnly(t, taskID)
	writeTestFile(t, filepath.Join(repo, ".gitignore"), []byte("/.revolvr/\n"))
	writeTestFile(t, filepath.Join(repo, "tracked.txt"), []byte("committed\n"))
	if guidance {
		writeTestFile(t, filepath.Join(repo, "AGENTS.md"), []byte("root guidance\r\n"))
		writeTestFile(t, filepath.Join(repo, ".agent", "AGENTS.md"), []byte("agent guidance\n"))
		writeTestFile(t, filepath.Join(repo, ".agent", "tasks", "AGENTS.md"), []byte("task guidance\n"))
		writeTestFile(t, filepath.Join(repo, "docs", "AGENTS.md"), []byte("unrelated nested guidance\n"))
	}
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-q", "-m", "Initial fixture")
	return repo
}

func newSHA256TestRepository(t *testing.T, taskID string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not available: %v", err)
	}
	repo := t.TempDir()
	cmd := exec.Command("git", "init", "-q", "--object-format=sha256")
	cmd.Dir = repo
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("installed Git does not support SHA-256 repositories: %v: %s", err, strings.TrimSpace(string(output)))
	}
	gitRun(t, repo, "config", "user.name", "Revolvr Assembly")
	gitRun(t, repo, "config", "user.email", "assembly@example.invalid")
	task := []byte(fmt.Sprintf("---\nid: %s\nstatus: pending\n---\n# Assemble %s\n\nExact task bytes.\n", taskID, taskID))
	writeTestFile(t, filepath.Join(repo, ".agent", "tasks", taskID+".md"), task)
	writeTestFile(t, filepath.Join(repo, ".gitignore"), []byte("/.revolvr/\n"))
	writeTestFile(t, filepath.Join(repo, "tracked.txt"), []byte("committed\n"))
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-q", "-m", "Initial SHA-256 fixture")
	return repo
}

func repositoryWithTaskOnly(t *testing.T, taskID string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not available: %v", err)
	}
	repo := t.TempDir()
	gitRun(t, repo, "init", "-q")
	gitRun(t, repo, "config", "user.name", "Revolvr Assembly")
	gitRun(t, repo, "config", "user.email", "assembly@example.invalid")
	task := []byte(fmt.Sprintf("---\nid: %s\nstatus: pending\n---\n# Assemble %s\n\nExact task bytes.\r\n", taskID, taskID))
	writeTestFile(t, filepath.Join(repo, ".agent", "tasks", taskID+".md"), task)
	return repo
}

func gitRun(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func gitOutput(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

func gitExactOutput(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return trimFinalCommandNewline(string(output))
}

func writeTestFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readTestFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return raw
}

func testEvent(t *testing.T, id int64, runID string, eventType ledger.EventType, payload any) ledger.Event {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal event payload: %v", err)
	}
	return ledger.Event{ID: id, RunID: runID, Type: eventType, Payload: raw, CreatedAt: time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)}
}

func manifestSource(t *testing.T, manifest autonomous.TaskDossierManifest, kind autonomous.DossierSourceKind, id string) autonomous.DossierSourceRecord {
	t.Helper()
	for _, source := range manifest.Sources {
		if source.Kind == kind && source.ID == id {
			return source
		}
	}
	t.Fatalf("manifest source kind=%q id=%q not found: %#v", kind, id, manifest.Sources)
	return autonomous.DossierSourceRecord{}
}

func assertSourceBytes(t *testing.T, record autonomous.DossierSourceRecord, content []byte) {
	t.Helper()
	sum := sha256.Sum256(content)
	if got, want := record.SHA256, fmt.Sprintf("%x", sum); got != want {
		t.Fatalf("source hash = %q, want %q", got, want)
	}
	if got, want := record.ByteSize, len(content); got != want {
		t.Fatalf("source byte size = %d, want %d", got, want)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return raw
}

func assertAssemblyError(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want substring %q", want)
	}
	if !strings.HasPrefix(err.Error(), "assemble task dossier: ") || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want stable prefix and substring %q", err, want)
	}
}

func containsArg(args []string, target string) bool {
	for _, arg := range args {
		if arg == target {
			return true
		}
	}
	return false
}

func cloneHistory(input []ledger.RunWithEvents) []ledger.RunWithEvents {
	cloned := append([]ledger.RunWithEvents(nil), input...)
	for i := range cloned {
		cloned[i].Events = append([]ledger.Event(nil), input[i].Events...)
		for j := range cloned[i].Events {
			cloned[i].Events[j].Payload = append(json.RawMessage(nil), input[i].Events[j].Payload...)
		}
	}
	return cloned
}
