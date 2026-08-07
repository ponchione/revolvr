package evaluation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type ScenarioExecutor interface {
	Execute(context.Context, ExecutionRequest) (Result, error)
}

// Runner owns mode admission. A reserved or unknown mode is refused before
// authority reads or the executor's source/model/sandbox/acceptance effects.
type Runner struct {
	RepositoryRoot string
	Executor       ScenarioExecutor
}

func (r Runner) Run(ctx context.Context, suite Suite, scenario Scenario, mode WorkerExecutionMode) (Result, error) {
	if err := ValidateMode(mode); err != nil {
		return Result{
			SchemaVersion: ResultSchemaVersion,
			ScenarioID:    scenario.ID,
			WorkerMode:    mode,
			Outcome:       "worker_execution_mode_not_implemented_not_admitted",
			StopReason:    "mode_refused_before_effects",
			Metrics:       Metrics{SchemaVersion: MetricsSchemaVersion, WorkerExecutionMode: mode, FinalTypedOutcome: "worker_execution_mode_not_implemented_not_admitted"},
		}, err
	}
	if r.Executor == nil {
		return Result{}, errors.New("evaluation: direct-tools executor is required")
	}
	authority, err := FreezeAuthority(r.RepositoryRoot, suite, scenario)
	if err != nil {
		return Result{}, err
	}
	return r.Executor.Execute(ctx, ExecutionRequest{Mode: mode, Scenario: scenario, Authority: authority})
}

func RunSuite(ctx context.Context, repositoryRoot string) (Baseline, error) {
	suite, suiteRaw, err := LoadSuite(repositoryRoot)
	if err != nil {
		return Baseline{}, err
	}
	workRoot, err := os.MkdirTemp("", "revolvr-evaluation-")
	if err != nil {
		return Baseline{}, err
	}
	defer os.RemoveAll(workRoot)
	executor := &DirectExecutor{RepositoryRoot: repositoryRoot, WorkRoot: workRoot}
	runner := Runner{RepositoryRoot: repositoryRoot, Executor: executor}
	baseline := Baseline{
		SchemaVersion: BaselineSchemaVersion,
		SuiteSHA256:   hashBytes(suiteRaw),
		WorkerModes:   []WorkerExecutionMode{DirectToolsV1},
		ScenarioCount: len(suite.Scenarios),
		LiveDogfood:   "omitted_not_run_use_scripts/dogfood-live.sh_with_recorded_source_model_prompt_sandbox_task_and_outcome_identities",
		Omissions: []Omission{
			{Field: "quality_threshold", Reason: "not_set_before_measured_baseline"},
			{Field: "estimated_cost", Reason: "not_reported_by_deterministic_fake_and_not_estimated"},
			{Field: "live_dogfood", Reason: "separate_opt_in_command_not_run_by_architecture_evaluation"},
		},
	}
	baseline.FixtureRepositorySHA256, err = FixtureIdentity(repositoryRoot, suite.FixtureRepository)
	if err != nil {
		return Baseline{}, err
	}
	baseline.ImplementationSHA256, err = implementationIdentity(repositoryRoot)
	if err != nil {
		return Baseline{}, err
	}
	for _, scenario := range suite.Scenarios {
		result, err := runner.Run(ctx, suite, scenario, DirectToolsV1)
		if err != nil {
			return Baseline{}, fmt.Errorf("evaluation: run %s: %w", scenario.ID, err)
		}
		if result.Outcome != scenario.ExpectedOutcome || result.StopReason != scenario.ExpectedStopReason {
			return Baseline{}, fmt.Errorf("evaluation: %s outcome %s/%s, want %s/%s", scenario.ID, result.Outcome, result.StopReason, scenario.ExpectedOutcome, scenario.ExpectedStopReason)
		}
		baseline.Results = append(baseline.Results, result)
	}
	baseline.RetrievalQuality, err = measureQuality(repositoryRoot)
	if err != nil {
		return Baseline{}, err
	}
	return baseline, nil
}

func implementationIdentity(repositoryRoot string) (string, error) {
	paths := []string{
		"internal/evaluation/types.go",
		"internal/evaluation/loader.go",
		"internal/evaluation/runner.go",
		"internal/evaluation/source.go",
		"internal/evaluation/direct.go",
		"internal/evaluation/fakes.go",
		"internal/evaluation/recovery.go",
		"internal/evaluation/retrieval.go",
	}
	type file struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
	}
	values := make([]file, 0, len(paths))
	for _, path := range paths {
		raw, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(path)))
		if err != nil {
			return "", err
		}
		values = append(values, file{Path: path, SHA256: hashBytes(raw)})
	}
	raw, err := Canonical(values)
	if err != nil {
		return "", err
	}
	return hashBytes(raw), nil
}
