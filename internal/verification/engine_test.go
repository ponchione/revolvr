package verification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"revolvr/internal/sandbox"
)

type memoryVerificationStore struct {
	reusable   map[string]ReusableCheck
	persisted  []PersistedRun
	persistErr error
	lookups    []string
}

func (s *memoryVerificationStore) FindReusable(_ context.Context, fingerprint string) (ReusableCheck, bool, error) {
	s.lookups = append(s.lookups, fingerprint)
	check, found := s.reusable[fingerprint]
	return check, found, nil
}

func (s *memoryVerificationStore) Persist(_ context.Context, run PersistedRun) error {
	if s.persistErr != nil {
		return s.persistErr
	}
	s.persisted = append(s.persisted, run)
	return nil
}

type fixtureExecutor struct {
	results    []ExecutionResult
	errors     []error
	sandboxIDs []string
}

func (e *fixtureExecutor) Execute(_ context.Context, execution GateExecution) (ExecutionResult, error) {
	e.sandboxIDs = append(e.sandboxIDs, execution.SandboxID)
	index := len(e.sandboxIDs) - 1
	result := ExecutionResult{
		SandboxSpecificationSHA256: strings.Repeat("1", 64), ExitCode: 0,
		Stdout: []byte("ok\n"), Evidence: json.RawMessage(`{"runtime":"fixture"}`),
		StartedAt:   time.Date(2026, 8, 6, 12, 0, index, 0, time.UTC),
		CompletedAt: time.Date(2026, 8, 6, 12, 0, index+1, 0, time.UTC),
	}
	if index < len(e.results) {
		result = e.results[index]
	}
	var err error
	if index < len(e.errors) {
		err = e.errors[index]
	}
	return result, err
}

type fixtureObserver struct {
	pinned   PinnedPlan
	snapshot func(Gate) AuthoritySnapshot
	err      error
}

func (o fixtureObserver) Observe(_ context.Context, gate Gate) (AuthoritySnapshot, error) {
	if o.err != nil {
		return AuthoritySnapshot{}, o.err
	}
	if o.snapshot != nil {
		return o.snapshot(gate), nil
	}
	return AuthoritySnapshot{Source: gate.Source, ProjectEnvironmentSHA256: o.pinned.ProjectEnvironment.SHA256, AuthorityInputs: append([]MaterialInput(nil), gate.AuthorityInputs...)}, nil
}

type fixtureArtifacts struct {
	fail  bool
	count int
}

func (a *fixtureArtifacts) Materialize(_ context.Context, kind, media string, raw []byte) (Artifact, error) {
	if a.fail {
		return Artifact{}, errors.New("injected artifact failure")
	}
	a.count++
	return Artifact{ID: uuid.NewString(), SHA256: hashBytes(raw), SizeBytes: int64(len(raw)), MediaType: media, LogicalKind: kind, StoragePath: fmt.Sprintf("fixture/%d", a.count), Content: append([]byte(nil), raw...)}, nil
}

func TestExecutionFingerprintCanonicalizationAndMaterialInvalidation(t *testing.T) {
	base := fixturePinned(t, TierFocused)
	baseFingerprint, err := ExecutionFingerprint(base, base.Plan.Gates[0])
	if err != nil {
		t.Fatal(err)
	}
	reordered := base
	reordered.PlanSHA256 = ""
	reordered.Plan.Gates[0].Environment = []EnvironmentVariable{{Name: "ZED", Value: "z"}, {Name: "ALPHA", Value: "a"}}
	baseWithEnvironment := base
	baseWithEnvironment.PlanSHA256 = ""
	baseWithEnvironment.Plan.Gates[0].Environment = []EnvironmentVariable{{Name: "ALPHA", Value: "a"}, {Name: "ZED", Value: "z"}}
	left, err := ExecutionFingerprint(reordered, reordered.Plan.Gates[0])
	if err != nil {
		t.Fatal(err)
	}
	right, err := ExecutionFingerprint(baseWithEnvironment, baseWithEnvironment.Plan.Gates[0])
	if err != nil || left != right {
		t.Fatalf("canonical environment fingerprints = %q/%q, err %v", left, right, err)
	}
	mutations := map[string]func(*PinnedPlan){
		"project":           func(p *PinnedPlan) { p.ProjectID = uuid.NewString() },
		"task version":      func(p *PinnedPlan) { p.TaskVersionID = uuid.NewString() },
		"verification plan": func(p *PinnedPlan) { p.Plan.VerificationPlanSHA256 = strings.Repeat("2", 64) },
		"plan version":      func(p *PinnedPlan) { p.Plan.Version = "plan-v2" },
		"source": func(p *PinnedPlan) {
			p.Plan.Gates[0].Source.Tree = strings.Repeat("2", 40)
			p.Candidate.Tree = strings.Repeat("2", 40)
		},
		"argv":              func(p *PinnedPlan) { p.Plan.Gates[0].Argv = append(p.Plan.Gates[0].Argv, "./...") },
		"working directory": func(p *PinnedPlan) { p.Plan.Gates[0].WorkingDirectory = "/workspace/internal" },
		"environment": func(p *PinnedPlan) {
			p.Plan.Gates[0].Environment = []EnvironmentVariable{{Name: "GOFLAGS", Value: "-count=1"}}
		},
		"image":             func(p *PinnedPlan) { p.Plan.Gates[0].Image.Digest = "sha256:" + strings.Repeat("2", 64) },
		"sandbox profile":   func(p *PinnedPlan) { p.Plan.Gates[0].SandboxProfileSHA256 = strings.Repeat("2", 64) },
		"sandbox resources": func(p *PinnedPlan) { p.Plan.Gates[0].Resources.PIDs++ },
		"parser":            func(p *PinnedPlan) { p.Plan.Gates[0].Parser.Version = "parser-v2" },
		"project contract": func(p *PinnedPlan) {
			p.ProjectEnvironment.Contract = json.RawMessage(`{"go":"1.26.6"}`)
			p.ProjectEnvironment.SHA256 = ""
		},
		"authority file": func(p *PinnedPlan) { p.Plan.Gates[0].AuthorityInputs[0].SHA256 = strings.Repeat("2", 64) },
		"output policy":  func(p *PinnedPlan) { p.Plan.Gates[0].OutputPolicy.StdoutMaxBytes-- },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			changed := base
			changed.Plan.Gates = append([]Gate(nil), base.Plan.Gates...)
			changed.Plan.Gates[0].Argv = append([]string(nil), base.Plan.Gates[0].Argv...)
			changed.Plan.Gates[0].AuthorityInputs = append([]MaterialInput(nil), base.Plan.Gates[0].AuthorityInputs...)
			changed.PlanSHA256 = ""
			mutate(&changed)
			fingerprint, err := ExecutionFingerprint(changed, changed.Plan.Gates[0])
			if err != nil {
				t.Fatal(err)
			}
			if fingerprint == baseFingerprint {
				t.Fatalf("material %s change did not invalidate fingerprint", name)
			}
		})
	}
}

func TestEngineExactReusePreservesOriginalAndFailure(t *testing.T) {
	for _, test := range []struct {
		name     string
		original Outcome
		want     Outcome
		status   RunStatus
	}{
		{"pass", OutcomePassed, OutcomePassedReused, RunPassed},
		{"failure", OutcomeFailed, OutcomeUnchangedFailureReused, RunFailed},
	} {
		t.Run(test.name, func(t *testing.T) {
			pinned := fixturePinned(t, TierFocused)
			fingerprint, _ := ExecutionFingerprint(pinned, pinned.Plan.Gates[0])
			originalAt := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
			store := &memoryVerificationStore{reusable: map[string]ReusableCheck{fingerprint: {
				ID: uuid.NewString(), Outcome: test.original, ExitCode: pointer(1),
				Stdout: fixtureArtifact(nil), Stderr: fixtureArtifact([]byte("failure")),
				ParsedResult: parsedFixture("case:a", "failed"), SandboxEvidence: json.RawMessage(`{"runtime":"original"}`),
				FailureSignatures: []string{"case:a"}, SandboxSpecificationSHA256: strings.Repeat("3", 64),
				OriginalExecutedAt: originalAt,
			}}}
			executor := &fixtureExecutor{}
			engine := fixtureEngine(t, pinned, store, executor, &fixtureArtifacts{})
			result, err := engine.Run(context.Background(), Request{Pinned: pinned, Purpose: PurposeCandidate})
			if err != nil {
				t.Fatal(err)
			}
			if len(executor.sandboxIDs) != 0 || len(store.persisted) != 1 || result.Status != test.status || result.Checks[0].Outcome != test.want {
				t.Fatalf("reuse result = %#v, executions %v, persisted %d", result, executor.sandboxIDs, len(store.persisted))
			}
			check := result.Checks[0]
			if check.ReusedFromCheckID == "" || !check.OriginalExecutedAt.Equal(originalAt) || !check.OccurredAt.After(originalAt) {
				t.Fatalf("reuse linkage/times = %#v", check)
			}
		})
	}
}

func TestEngineInvalidationAndFinalFreshnessForceExecution(t *testing.T) {
	for _, test := range []struct {
		name    string
		tier    Tier
		purpose Purpose
		mutate  bool
	}{
		{"material invalidation", TierFocused, PurposeCandidate, true},
		{"completion Tier 4", TierFinal, PurposeFinal, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			pinned := fixturePinned(t, test.tier)
			oldFingerprint, _ := ExecutionFingerprint(pinned, pinned.Plan.Gates[0])
			store := &memoryVerificationStore{reusable: map[string]ReusableCheck{oldFingerprint: reusablePassed()}}
			if test.mutate {
				pinned.PlanSHA256 = ""
				pinned.Plan.Gates[0].OutputPolicy.StdoutMaxBytes--
				pinned, _ = Pin(pinned)
			}
			executor := &fixtureExecutor{}
			engine := fixtureEngine(t, pinned, store, executor, &fixtureArtifacts{})
			result, err := engine.Run(context.Background(), Request{Pinned: pinned, Purpose: test.purpose})
			if err != nil || len(executor.sandboxIDs) != 1 || result.Checks[0].Outcome != OutcomePassed {
				t.Fatalf("fresh result = %#v, executions %v, err %v", result, executor.sandboxIDs, err)
			}
		})
	}
}

func TestEngineUsesFreshSandboxForEveryExecutedGate(t *testing.T) {
	pinned := fixturePinned(t, TierFocused)
	second := pinned.Plan.Gates[0]
	second.ID = "project"
	second.Tier = TierProject
	pinned.Plan.Gates = append(pinned.Plan.Gates, second)
	pinned.PlanSHA256 = ""
	pinned, _ = Pin(pinned)
	store := &memoryVerificationStore{reusable: map[string]ReusableCheck{}}
	executor := &fixtureExecutor{}
	engine := fixtureEngine(t, pinned, store, executor, &fixtureArtifacts{})
	result, err := engine.Run(context.Background(), Request{Pinned: pinned, Purpose: PurposeCandidate})
	if err != nil || result.Status != RunPassed || len(executor.sandboxIDs) != 2 || executor.sandboxIDs[0] == executor.sandboxIDs[1] {
		t.Fatalf("fresh sandbox result = %#v, IDs %v, err %v", result, executor.sandboxIDs, err)
	}
}

func TestEngineRejectsAuthorityChangedDuringExecution(t *testing.T) {
	pinned := fixturePinned(t, TierFocused)
	calls := 0
	observer := fixtureObserver{pinned: pinned, snapshot: func(gate Gate) AuthoritySnapshot {
		calls++
		source := gate.Source
		if calls > 1 {
			source.Tree = strings.Repeat("2", 40)
		}
		return AuthoritySnapshot{Source: source, ProjectEnvironmentSHA256: pinned.ProjectEnvironment.SHA256, AuthorityInputs: gate.AuthorityInputs}
	}}
	store := &memoryVerificationStore{reusable: map[string]ReusableCheck{}}
	executor := &fixtureExecutor{}
	engine := fixtureEngineWithObserver(t, store, executor, &fixtureArtifacts{}, observer)
	result, err := engine.Run(context.Background(), Request{Pinned: pinned, Purpose: PurposeCandidate})
	if err != nil || calls != 2 || len(executor.sandboxIDs) != 1 || len(store.persisted) != 1 || result.Checks[0].Outcome != OutcomeStaleSource {
		t.Fatalf("post-execution authority result = %#v, observations %d, executions %v, persisted %d, err %v", result, calls, executor.sandboxIDs, len(store.persisted), err)
	}
}

func TestEngineFailClosedOutcomes(t *testing.T) {
	malformedPlan := fixturePinned(t, TierFocused)
	malformedPlan.Plan.Gates[0].Parser.Kind = ParserJSON
	malformedPlan.PlanSHA256 = ""
	malformedPlan, _ = Pin(malformedPlan)
	tests := []struct {
		name     string
		pinned   PinnedPlan
		observer func(PinnedPlan) fixtureObserver
		result   ExecutionResult
		execErr  error
		want     Outcome
	}{
		{"stale source", fixturePinned(t, TierFocused), func(p PinnedPlan) fixtureObserver {
			return fixtureObserver{pinned: p, snapshot: func(g Gate) AuthoritySnapshot {
				return AuthoritySnapshot{Source: SourceIdentity{Commit: strings.Repeat("2", 40), Tree: g.Source.Tree}, ProjectEnvironmentSHA256: p.ProjectEnvironment.SHA256, AuthorityInputs: g.AuthorityInputs}
			}}
		}, ExecutionResult{}, nil, OutcomeStaleSource},
		{"stale environment", fixturePinned(t, TierFocused), func(p PinnedPlan) fixtureObserver {
			return fixtureObserver{pinned: p, snapshot: func(g Gate) AuthoritySnapshot {
				return AuthoritySnapshot{Source: g.Source, ProjectEnvironmentSHA256: strings.Repeat("2", 64), AuthorityInputs: g.AuthorityInputs}
			}}
		}, ExecutionResult{}, nil, OutcomeStaleEnvironment},
		{"authority tampered", fixturePinned(t, TierFocused), func(p PinnedPlan) fixtureObserver {
			return fixtureObserver{pinned: p, snapshot: func(g Gate) AuthoritySnapshot {
				return AuthoritySnapshot{Source: g.Source, ProjectEnvironmentSHA256: p.ProjectEnvironment.SHA256, AuthorityInputs: []MaterialInput{}}
			}}
		}, ExecutionResult{}, nil, OutcomeAuthorityTampered},
		{"missing command", fixturePinned(t, TierFocused), nil, executedResult(127, ""), nil, OutcomeMissingCommand},
		{"timeout", fixturePinned(t, TierFocused), nil, withTimeout(executedResult(-1, "")), context.DeadlineExceeded, OutcomeTimedOut},
		{"cancellation", fixturePinned(t, TierFocused), nil, withCancellation(executedResult(-1, "")), context.Canceled, OutcomeCancelled},
		{"malformed", malformedPlan, nil, executedResult(0, "{"), nil, OutcomeMalformedOutput},
		{"truncated", fixturePinned(t, TierFocused), nil, withTruncation(executedResult(0, "ok")), nil, OutcomeAmbiguous},
		{"infrastructure", fixturePinned(t, TierFocused), nil, executedResult(-1, ""), errors.New("runtime unavailable"), OutcomeInfrastructureFailed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			observer := fixtureObserver{pinned: test.pinned}
			if test.observer != nil {
				observer = test.observer(test.pinned)
			}
			store := &memoryVerificationStore{reusable: map[string]ReusableCheck{}}
			executor := &fixtureExecutor{results: []ExecutionResult{test.result}, errors: []error{test.execErr}}
			engine := fixtureEngineWithObserver(t, store, executor, &fixtureArtifacts{}, observer)
			result, err := engine.Run(context.Background(), Request{Pinned: test.pinned, Purpose: PurposeCandidate})
			if err != nil || len(store.persisted) != 1 || result.Checks[0].Outcome != test.want || !result.Checks[0].Outcome.Failed() {
				t.Fatalf("result = %#v, persisted %d, err %v", result, len(store.persisted), err)
			}
			if test.want == OutcomeAuthorityTampered && result.AuthorityAction != AuthorityReject {
				t.Fatalf("authority action = %q", result.AuthorityAction)
			}
		})
	}
}

func TestEngineAuthorityPolicyDirectives(t *testing.T) {
	for _, policy := range []AuthorityChangePolicy{AuthorityReject, AuthorityDualRun, AuthorityEscalate} {
		t.Run(string(policy), func(t *testing.T) {
			pinned := fixturePinned(t, TierFocused)
			pinned.Plan.AuthorityChangePolicy = policy
			pinned.PlanSHA256 = ""
			pinned, _ = Pin(pinned)
			observer := fixtureObserver{pinned: pinned, snapshot: func(g Gate) AuthoritySnapshot {
				return AuthoritySnapshot{Source: g.Source, ProjectEnvironmentSHA256: pinned.ProjectEnvironment.SHA256}
			}}
			store := &memoryVerificationStore{reusable: map[string]ReusableCheck{}}
			engine := fixtureEngineWithObserver(t, store, &fixtureExecutor{}, &fixtureArtifacts{}, observer)
			result, err := engine.Run(context.Background(), Request{Pinned: pinned, Purpose: PurposeCandidate})
			if err != nil || result.Checks[0].Outcome != OutcomeAuthorityTampered || result.DualRunRequired != (policy == AuthorityDualRun) || result.EscalationRequired != (policy == AuthorityEscalate) {
				t.Fatalf("policy result = %#v, err %v", result, err)
			}
		})
	}
}

func TestEngineArtifactAndPersistenceFailuresAcceptNothing(t *testing.T) {
	pinned := fixturePinned(t, TierFocused)
	for _, test := range []struct {
		name       string
		artifacts  *fixtureArtifacts
		persistErr error
		want       error
	}{
		{"artifact", &fixtureArtifacts{fail: true}, nil, ErrArtifact},
		{"transaction", &fixtureArtifacts{}, errors.New("injected rollback"), ErrPersistence},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &memoryVerificationStore{reusable: map[string]ReusableCheck{}, persistErr: test.persistErr}
			engine := fixtureEngine(t, pinned, store, &fixtureExecutor{}, test.artifacts)
			_, err := engine.Run(context.Background(), Request{Pinned: pinned, Purpose: PurposeCandidate})
			if !errors.Is(err, test.want) || len(store.persisted) != 0 {
				t.Fatalf("error = %v, accepted runs = %d", err, len(store.persisted))
			}
		})
	}
}

func TestDifferentialClassifiesNewResolvedUnchangedAndFlaky(t *testing.T) {
	baseline := []PersistedCheck{
		{ParsedResult: parsedFixture("resolved", "failed")},
		{ParsedResult: parsedFixture("unchanged", "failed")},
	}
	candidate := []PersistedCheck{
		{ParsedResult: parsedFixture("new", "failed")},
		{ParsedResult: parsedFixture("unchanged", "failed")},
		{ParsedResult: parsedFixture("flaky", "failed")},
		{ParsedResult: parsedFixture("flaky", "passed")},
	}
	got := ClassifyDifferential(baseline, candidate)
	want := Differential{New: []string{"new"}, Resolved: []string{"resolved"}, Unchanged: []string{"unchanged"}, Flaky: []string{"flaky"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("differential = %#v, want %#v", got, want)
	}
}

func TestConfiguredStructuredParsers(t *testing.T) {
	tests := []struct {
		kind ParserKind
		raw  string
		want string
	}{
		{ParserGoTestJSON, `{"Action":"fail","Package":"revolvr/p","Test":"TestA"}` + "\n", "go-test:revolvr/p/TestA"},
		{ParserJSON, `{"cases":[{"name":"A","status":"failed"}]}`, "json:A"},
		{ParserJUnitXML, `<testsuite><testcase classname="pkg" name="A"><failure/></testcase></testsuite>`, "junit:pkg/A"},
	}
	for _, test := range tests {
		t.Run(string(test.kind), func(t *testing.T) {
			gate := fixturePinned(t, TierFocused).Plan.Gates[0]
			gate.Parser.Kind = test.kind
			_, failures, err := parseOutput(gate, []byte(test.raw), 1)
			if err != nil || !reflect.DeepEqual(failures, []string{test.want}) {
				t.Fatalf("failures = %v, err %v", failures, err)
			}
		})
	}
}

func fixturePinned(t *testing.T, tier Tier) PinnedPlan {
	t.Helper()
	source := SourceIdentity{Commit: strings.Repeat("a", 40), Tree: strings.Repeat("b", 40)}
	pinned, err := Pin(PinnedPlan{
		Plan: Plan{
			SchemaVersion: PlanSchemaVersion, Version: "plan-v1",
			VerificationPlanVersion: "task-verification-v1", VerificationPlanSHA256: strings.Repeat("c", 64),
			AuthorityChangePolicy: AuthorityReject, AllowReuse: true, AllowMissingBaseline: tier != TierAdmissionBaseline,
			RequireFreshFinal: tier == TierFinal,
			Gates: []Gate{{
				ID: "focused", Tier: tier, Source: source, Argv: []string{"go", "test", "./internal/verification"},
				WorkingDirectory: "/workspace", Image: sandbox.Image{Reference: "example.invalid/revolvr/verifier", Digest: "sha256:" + strings.Repeat("d", 64)},
				SandboxProfile: sandbox.ProfileStrict, SandboxProfileSHA256: strings.Repeat("e", 64),
				Resources:       sandbox.Resources{CPUs: 2, MemoryBytes: 1 << 30, PIDs: 256, TimeoutSeconds: 60, TmpfsBytes: 64 << 20},
				Parser:          Parser{Kind: ParserNone, Version: DefaultStructuredParserVersion},
				AuthorityInputs: []MaterialInput{{Kind: "project-contract", Path: "go.mod", SHA256: strings.Repeat("f", 64), SizeBytes: 100}},
				OutputPolicy:    OutputPolicy{Version: DefaultOutputPolicyVersion, StdoutMaxBytes: MaximumCapturedStreamBytes, StderrMaxBytes: MaximumCapturedStreamBytes},
			}},
		},
		ProjectID: uuid.NewString(), TaskID: uuid.NewString(), TaskVersionID: uuid.NewString(),
		RunID: uuid.NewString(), WorkspaceID: uuid.NewString(), Candidate: source,
		ProjectEnvironment: ProjectEnvironment{SchemaVersion: DefaultProjectEnvironmentVersion, Contract: json.RawMessage(`{"go":"1.26.5","os":"linux"}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return pinned
}

func fixtureEngine(t *testing.T, pinned PinnedPlan, store Store, executor GateExecutor, artifacts ArtifactWriter) *Engine {
	t.Helper()
	return fixtureEngineWithObserver(t, store, executor, artifacts, fixtureObserver{pinned: pinned})
}

func fixtureEngineWithObserver(t *testing.T, store Store, executor GateExecutor, artifacts ArtifactWriter, observer AuthorityObserver) *Engine {
	t.Helper()
	now := time.Date(2026, 8, 6, 13, 0, 0, 0, time.UTC)
	engine, err := NewEngine(EngineConfig{
		Store: store, Executor: executor, Artifacts: artifacts, Observer: observer,
		Clock: func() time.Time { now = now.Add(time.Second); return now }, NewID: uuid.NewString,
	})
	if err != nil {
		t.Fatal(err)
	}
	return engine
}

func fixtureArtifact(raw []byte) Artifact {
	return Artifact{ID: uuid.NewString(), SHA256: hashBytes(raw), SizeBytes: int64(len(raw)), MediaType: "application/octet-stream", LogicalKind: "fixture", StoragePath: uuid.NewString(), Content: raw}
}

func reusablePassed() ReusableCheck {
	exit := 0
	return ReusableCheck{ID: uuid.NewString(), Outcome: OutcomePassed, ExitCode: &exit, Stdout: fixtureArtifact(nil), Stderr: fixtureArtifact(nil), ParsedResult: parsedFixture("gate:focused", "passed"), SandboxEvidence: json.RawMessage(`{"runtime":"fixture"}`), SandboxSpecificationSHA256: strings.Repeat("4", 64), OriginalExecutedAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)}
}

func parsedFixture(identity, status string) json.RawMessage {
	raw, _ := json.Marshal(ParsedOutput{SchemaVersion: "revolvr-parsed-verification-output-v1", Kind: ParserNone, Cases: []CaseResult{{Identity: identity, Status: status}}, Structured: json.RawMessage(`{}`)})
	return raw
}

func pointer(value int) *int { return &value }

func executedResult(exit int, stdout string) ExecutionResult {
	return ExecutionResult{SandboxSpecificationSHA256: strings.Repeat("5", 64), ExitCode: exit, Stdout: []byte(stdout), Evidence: json.RawMessage(`{"runtime":"fixture"}`), StartedAt: time.Date(2026, 8, 6, 14, 0, 0, 0, time.UTC), CompletedAt: time.Date(2026, 8, 6, 14, 0, 1, 0, time.UTC)}
}

func withTimeout(result ExecutionResult) ExecutionResult      { result.TimedOut = true; return result }
func withCancellation(result ExecutionResult) ExecutionResult { result.Cancelled = true; return result }
func withTruncation(result ExecutionResult) ExecutionResult {
	result.StdoutTruncatedBytes = 1
	return result
}
