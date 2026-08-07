package evaluation

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	codeindex "revolvr/internal/index"
)

func TestDeterministicArchitectureScenarios(t *testing.T) {
	root := repositoryRoot(t)
	baseline, err := RunSuite(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if baseline.ScenarioCount != 20 || len(baseline.Results) != 20 || !reflect.DeepEqual(baseline.WorkerModes, []WorkerExecutionMode{DirectToolsV1}) {
		t.Fatalf("baseline identity = %+v", baseline)
	}
	wantIDs := []string{
		"straight-success", "compile-failure-correction", "test-failure-correction", "audit-finding-correction",
		"ambiguous-requirement", "missing-dependency", "cyclic-dependency", "scope-violation",
		"protected-path-violation", "repeated-failed-strategy", "no-source-changes", "test-tampering",
		"mid-run-source-change", "cancellation", "crash-during-state-effects", "crash-during-external-effects",
		"stale-retrieval-index", "missing-embedding-service", "sandbox-timeout", "network-denied-dependency-install",
	}
	for index, result := range baseline.Results {
		if result.ScenarioID != wantIDs[index] {
			t.Fatalf("scenario %d = %q, want %q", index, result.ScenarioID, wantIDs[index])
		}
		assertCompleteResultContract(t, result)
	}
	if baseline.RetrievalQuality.FixtureCount != 5 || baseline.RetrievalQuality.RecallAt5 != 1 || baseline.RetrievalQuality.RecallAt10 != 1 || baseline.RetrievalQuality.MRR != 1 || baseline.RetrievalQuality.ExactSymbolPreservation != 1 || baseline.RetrievalQuality.Threshold != nil {
		t.Fatalf("retrieval quality baseline = %+v", baseline.RetrievalQuality)
	}
}

func TestGoldenBaseline(t *testing.T) {
	root := repositoryRoot(t)
	baseline, err := RunSuite(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := Canonical(baseline)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "evals", "golden", "baseline.json")
	if os.Getenv("REVOLVR_UPDATE_EVALUATION_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, want) {
		t.Fatalf("evaluation baseline changed; review with REVOLVR_UPDATE_EVALUATION_GOLDEN=1 and inspect evals/golden/baseline.json")
	}
}

func TestRepeatedRunsAreByteStable(t *testing.T) {
	root := repositoryRoot(t)
	first, err := RunSuite(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RunSuite(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	firstRaw, _ := Canonical(first)
	secondRaw, _ := Canonical(second)
	if !bytes.Equal(firstRaw, secondRaw) {
		t.Fatal("identical deterministic suite runs produced different bytes")
	}
}

func TestReservedModeRefusesBeforeEveryEffect(t *testing.T) {
	executor := &effectCountingExecutor{}
	suite := Suite{FixtureRepository: "must-not-be-read"}
	scenario := Scenario{ID: "reserved-mode"}
	result, err := (Runner{RepositoryRoot: t.TempDir(), Executor: executor}).Run(context.Background(), suite, scenario, ProgrammaticWorkspaceV1)
	var refusal *ModeRefusalError
	if !errors.As(err, &refusal) || refusal.Code != "not_implemented_not_admitted" {
		t.Fatalf("reserved mode error = %v", err)
	}
	if result.Outcome != "worker_execution_mode_not_implemented_not_admitted" || result.StopReason != "mode_refused_before_effects" {
		t.Fatalf("reserved mode result = %+v", result)
	}
	if executor.source != 0 || executor.model != 0 || executor.sandbox != 0 || executor.acceptance != 0 {
		t.Fatalf("reserved mode effects = %+v", executor)
	}
}

func TestModeNeutralAuthorityIsIdenticalForFutureMode(t *testing.T) {
	root := repositoryRoot(t)
	suite, _, err := LoadSuite(root)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := FreezeAuthority(root, suite, suite.Scenarios[0])
	if err != nil {
		t.Fatal(err)
	}
	direct := ExecutionRequest{Mode: DirectToolsV1, Scenario: suite.Scenarios[0], Authority: authority}
	future := ExecutionRequest{Mode: ProgrammaticWorkspaceV1, Scenario: suite.Scenarios[0], Authority: authority}
	directRaw, _ := Canonical(direct.Authority)
	futureRaw, _ := Canonical(future.Authority)
	if !bytes.Equal(directRaw, futureRaw) || direct.Authority.SHA256 != future.Authority.SHA256 {
		t.Fatal("worker mode changed task, acceptance, policy, source, or expected-outcome authority")
	}
	if err := ValidateMode(future.Mode); err == nil {
		t.Fatal("future mode was unexpectedly admitted")
	}
}

func TestExpectedOutcomeAuthorityCannotRewriteActualBehavior(t *testing.T) {
	root := repositoryRoot(t)
	suite, _, err := LoadSuite(root)
	if err != nil {
		t.Fatal(err)
	}
	scenario := suite.Scenarios[0]
	scenario.ExpectedOutcome = "unsafe_fabricated_success"
	workRoot := t.TempDir()
	_, err = (Runner{RepositoryRoot: root, Executor: &DirectExecutor{RepositoryRoot: root, WorkRoot: workRoot}}).Run(context.Background(), suite, scenario, DirectToolsV1)
	if err == nil || !strings.Contains(err.Error(), "immutable acceptance authority changed") {
		t.Fatalf("changed expected outcome error = %v", err)
	}
}

func TestEveryRecoveryBoundaryIsIdempotentAndDivergenceFailsClosed(t *testing.T) {
	seen := map[CrashBoundary]bool{}
	for _, boundary := range AllCrashBoundaries() {
		fact, err := exerciseCrashBoundary(boundary, "recovery-test")
		if err != nil {
			t.Fatal(err)
		}
		if fact.ReplayCount != 1 || !fact.ExactReplayIdempotent || fact.DivergentReplayOutcome != "unsafe_or_ambiguous" {
			t.Fatalf("boundary %s fact = %+v", boundary, fact)
		}
		seen[boundary] = true
	}
	if len(seen) != 6 {
		t.Fatalf("covered crash boundaries = %v", seen)
	}
}

func TestDegradedRetrievalPreservesExactQwenAndContextAuthority(t *testing.T) {
	baseline, err := RunSuite(context.Background(), repositoryRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	evidence := codeindex.SelectedEmbeddingEvidence()
	for _, id := range []string{"stale-retrieval-index", "missing-embedding-service"} {
		result := resultByID(t, baseline, id)
		if !result.Retrieval.ExactSourceFirst || !result.Retrieval.DegradedWithoutFallback || result.Retrieval.EmbeddingModelName != evidence.Model.ModelName || result.Retrieval.EmbeddingRevision != evidence.Model.Revision || result.Retrieval.EmbeddingSpaceSHA256 != evidence.SpaceSHA256 || result.Retrieval.QueryInstructionSHA256 != codeindex.SelectedQueryInstructionSHA256 {
			t.Fatalf("%s retrieval = %+v", id, result.Retrieval)
		}
		if result.Retrieval.ContextManifestSHA256 == "" || result.Retrieval.DossierSHA256 == "" {
			t.Fatalf("%s context identities are missing", id)
		}
	}
}

func TestEventOrderingLeaseCleanupAndOriginalCheckoutIdentity(t *testing.T) {
	baseline, err := RunSuite(context.Background(), repositoryRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range baseline.Results {
		for index, event := range result.Events {
			if event.Sequence != index+1 {
				t.Fatalf("%s event sequence = %+v", result.ScenarioID, result.Events)
			}
		}
		if !result.LeaseReleased || !result.Workspace.OriginalCheckoutUnchanged || result.Workspace.OriginalCheckoutBefore != result.Workspace.OriginalCheckoutAfter || !result.Workspace.Cleaned && result.Workspace.State.Applicability == "applicable" {
			t.Fatalf("%s cleanup/checkout = lease:%t workspace:%+v", result.ScenarioID, result.LeaseReleased, result.Workspace)
		}
		wantLease := result.ScenarioID != "missing-dependency" && result.ScenarioID != "cyclic-dependency"
		if result.LeaseAcquired != wantLease {
			t.Fatalf("%s lease acquired = %t, want %t", result.ScenarioID, result.LeaseAcquired, wantLease)
		}
	}
}

func TestFalseCompletionAndHostSafetyMatrix(t *testing.T) {
	baseline, err := RunSuite(context.Background(), repositoryRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		id                   string
		task                 string
		verification         string
		candidateCommit      bool
		completionAuthorized bool
	}{
		{"scope-violation", "unsafe", "not_run", false, false},
		{"protected-path-violation", "unsafe", "not_run", false, false},
		{"repeated-failed-strategy", "blocked", "failed", false, false},
		{"no-source-changes", "blocked", "not_run", false, false},
		{"test-tampering", "unsafe", "authority_tampered", false, false},
		{"mid-run-source-change", "unsafe", "not_run", true, false},
		{"cancellation", "cancelled", "cancelled", false, false},
		{"sandbox-timeout", "blocked", "timed_out", false, false},
		{"network-denied-dependency-install", "blocked", "not_run", false, false},
	}
	for _, test := range tests {
		result := resultByID(t, baseline, test.id)
		if result.Task.Status != test.task || result.Verification.State.Status != test.verification || (result.Workspace.CandidateCommit != "") != test.candidateCommit || result.Completion.Authorized != test.completionAuthorized || result.Sandbox.State.Status != "removed" || result.Sandbox.Network != "none" || result.Sandbox.AmbientEnv || result.Sandbox.RuntimeSocket || result.Sandbox.OriginalSource {
			t.Fatalf("%s false-completion/safety result = %+v", test.id, result)
		}
	}
}

func TestPostgreSQLRollbackFixture(t *testing.T) {
	databaseURL := os.Getenv("REVOLVR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("REVOLVR_TEST_DATABASE_URL is not set")
	}
	connection, err := pgx.Connect(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(context.Background())
	transaction, err := connection.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transaction.Exec(context.Background(), "create temp table revolvr_evaluation_rollback (scenario_id text primary key, outcome text not null) on commit drop"); err != nil {
		t.Fatal(err)
	}
	if _, err := transaction.Exec(context.Background(), "insert into revolvr_evaluation_rollback values ($1, $2)", "straight-success", "completed"); err != nil {
		t.Fatal(err)
	}
	if err := transaction.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	var exists bool
	if err := connection.QueryRow(context.Background(), "select to_regclass('pg_temp.revolvr_evaluation_rollback') is not null").Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("rolled-back deterministic evaluation state remained visible")
	}
}

type effectCountingExecutor struct {
	source     int
	model      int
	sandbox    int
	acceptance int
}

func (e *effectCountingExecutor) Execute(context.Context, ExecutionRequest) (Result, error) {
	e.source++
	e.model++
	e.sandbox++
	e.acceptance++
	return Result{}, errors.New("must not execute")
}

func assertCompleteResultContract(t *testing.T, result Result) {
	t.Helper()
	states := []StateFact{result.Task, result.Run, result.Plan, result.Workspace.State, result.Sandbox.State, result.Verification.State, result.Audit.State, result.Completion.State}
	for _, state := range states {
		if state.Status == "" || state.Applicability == "" || len(state.SHA256) != 64 {
			t.Fatalf("%s state = %+v", result.ScenarioID, state)
		}
	}
	if len(result.Criteria) != 1 || len(result.Events) == 0 || len(result.Artifacts) < 3 || result.Outcome == "" || result.StopReason == "" || len(result.AuthoritySHA256) != 64 {
		t.Fatalf("%s incomplete result = %+v", result.ScenarioID, result)
	}
	if result.Metrics.WorkerExecutionMode != DirectToolsV1 || result.Metrics.ContextBytes <= 0 || result.Metrics.WallTimeNanoseconds < 0 || len(result.Metrics.Omissions) != 4 {
		t.Fatalf("%s metrics = %+v", result.ScenarioID, result.Metrics)
	}
	fields := make([]string, 0, len(result.Metrics.Omissions))
	for _, omission := range result.Metrics.Omissions {
		fields = append(fields, omission.Field)
	}
	sort.Strings(fields)
	if strings.Join(fields, ",") != "tokens.cached,tokens.input,tokens.output,tokens.reasoning" {
		t.Fatalf("%s token omissions = %v", result.ScenarioID, fields)
	}
	if !result.Safety.NoLiveModel || !result.Safety.NoPublicNetwork || !result.Safety.NoAmbientCredentials || !result.Safety.NoOperatorHomeData || !result.Safety.NoRuntimeSocket || !result.Safety.OriginalCheckoutIntact {
		t.Fatalf("%s safety = %+v", result.ScenarioID, result.Safety)
	}
}

func resultByID(t *testing.T, baseline Baseline, id string) Result {
	t.Helper()
	for _, result := range baseline.Results {
		if result.ScenarioID == id {
			return result
		}
	}
	t.Fatalf("missing result %s", id)
	return Result{}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return root
}
