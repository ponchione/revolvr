package planner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"revolvr/internal/storage/postgres"
)

func TestPostgresPlanCandidateAcceptanceConcurrencyAndRollback(t *testing.T) {
	databaseURL := os.Getenv("REVOLVR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("REVOLVR_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	t.Run("complete provenance and exact retry", func(t *testing.T) {
		candidate := databaseCandidate(t, ctx, pool)
		at := time.Now().UTC().Truncate(time.Microsecond)
		command := AcceptanceCommand{OperationID: "accept-" + uuid.NewString(), AcceptedBy: "planner-test-host", Authority: TrustedHostAcceptance, AcceptedAt: at}
		result, err := Accept(ctx, pool, candidate, command)
		if err != nil {
			t.Fatal(err)
		}
		if result.Disposition != PersistenceAccepted || result.PlanAggregateVersion != 2 || len(result.EventIDs) != 2 {
			t.Fatalf("accept result = %#v", result)
		}
		q := postgres.New(pool)
		plan, err := q.GetPlan(ctx, mustUUID(candidate.PlanID))
		if err != nil {
			t.Fatal(err)
		}
		if uuidString(plan.AcceptedVersionID) != candidate.PlanVersionID || plan.AcceptedOperationID.String != command.OperationID {
			t.Fatalf("accepted plan = %#v", plan)
		}
		version, err := q.GetPlanVersion(ctx, mustUUID(candidate.PlanVersionID))
		if err != nil {
			t.Fatal(err)
		}
		if version.CandidateSha256 != candidate.CandidateSHA256 || version.DossierSha256 != candidate.Dossier.SHA256 || version.PromptSha256 != candidate.Prompt.SHA256 || version.ResponseSchemaSha256 != candidate.ResponseSchema.SHA256 || version.ModelPolicySha256 != candidate.ModelPolicy.SHA256 || version.HostPolicySha256 != candidate.HostPolicy.SHA256 || version.SupervisorDecisionID != candidate.SupervisorDecisionID || version.SourceRevision != candidate.SourceRevision {
			t.Fatalf("persisted provenance = %#v", version)
		}
		steps, err := q.ListPlanSteps(ctx, mustUUID(candidate.PlanVersionID))
		if err != nil {
			t.Fatal(err)
		}
		if len(steps) != 2 || steps[0].Ordinal != 1 || steps[0].StepID != "step-1" || steps[1].Ordinal != 2 || steps[1].StepID != "step-2" {
			t.Fatalf("stored steps = %#v", steps)
		}
		events, err := q.ListPlanEvents(ctx, mustUUID(candidate.PlanID))
		if err != nil {
			t.Fatal(err)
		}
		if len(events) != 2 || events[0].EventType != "plan.candidate_recorded" || events[1].EventType != "plan.accepted" {
			t.Fatalf("events = %#v", events)
		}
		if _, err := pool.Exec(ctx, "UPDATE core.plan_steps SET status='completed' WHERE plan_version_id=$1 AND step_id='step-1'", candidate.PlanVersionID); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, "UPDATE core.plan_steps SET status='pending' WHERE plan_version_id=$1 AND step_id='step-1'", candidate.PlanVersionID); err == nil {
			t.Fatal("PostgreSQL allowed a completed plan step to regress")
		}
		replay, err := Accept(ctx, pool, candidate, command)
		if err != nil {
			t.Fatal(err)
		}
		if replay.Disposition != PersistenceReplayed || replay.PlanAggregateVersion != 2 {
			t.Fatalf("replay = %#v", replay)
		}
	})

	t.Run("concurrent acceptance has one winner", func(t *testing.T) {
		candidate := databaseCandidate(t, ctx, pool)
		start := make(chan struct{})
		results := make(chan error, 2)
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				_, err := Accept(ctx, pool, candidate, AcceptanceCommand{OperationID: fmt.Sprintf("race-%d-%s", i, uuid.NewString()), AcceptedBy: "race-host", Authority: TrustedHostAcceptance, AcceptedAt: time.Now().UTC()})
				results <- err
			}(i)
		}
		close(start)
		wg.Wait()
		close(results)
		success, conflict := 0, 0
		for err := range results {
			if err == nil {
				success++
			} else if errors.Is(err, ErrConflict) {
				conflict++
			} else {
				t.Fatalf("unexpected race error: %v", err)
			}
		}
		if success != 1 || conflict != 1 {
			t.Fatalf("success/conflict = %d/%d", success, conflict)
		}
	})

	t.Run("event failure rolls back all rows and retry is idempotent", func(t *testing.T) {
		candidate := databaseCandidate(t, ctx, pool)
		suffix := strings.ReplaceAll(candidate.PlanID, "-", "")
		functionName := "core.fail_plan_accept_" + suffix
		triggerName := "fail_plan_accept_" + suffix
		ddl := fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.event_type = 'plan.accepted' AND NEW.aggregate_id = '%s'::uuid THEN RAISE EXCEPTION 'forced plan event failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER %s BEFORE INSERT ON core.events FOR EACH ROW EXECUTE FUNCTION %s()`, functionName, candidate.PlanID, triggerName, functionName)
		if _, err := pool.Exec(ctx, ddl); err != nil {
			t.Fatal(err)
		}
		drop := func() {
			_, _ = pool.Exec(context.Background(), fmt.Sprintf("DROP TRIGGER IF EXISTS %s ON core.events; DROP FUNCTION IF EXISTS %s()", triggerName, functionName))
		}
		t.Cleanup(drop)
		command := AcceptanceCommand{OperationID: "rollback-" + uuid.NewString(), AcceptedBy: "rollback-host", Authority: TrustedHostAcceptance, AcceptedAt: time.Now().UTC()}
		if _, err := Accept(ctx, pool, candidate, command); err == nil {
			t.Fatal("Accept succeeded despite forced event failure")
		}
		var plans, versions, steps, events int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM core.plans WHERE id=$1", candidate.PlanID).Scan(&plans); err != nil {
			t.Fatal(err)
		}
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM core.plan_versions WHERE id=$1", candidate.PlanVersionID).Scan(&versions)
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM core.plan_steps WHERE plan_version_id=$1", candidate.PlanVersionID).Scan(&steps)
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM core.events WHERE aggregate_type='plan' AND aggregate_id=$1", candidate.PlanID).Scan(&events)
		if plans+versions+steps+events != 0 {
			t.Fatalf("rollback counts plans=%d versions=%d steps=%d events=%d", plans, versions, steps, events)
		}
		drop()
		result, err := Accept(ctx, pool, candidate, command)
		if err != nil {
			t.Fatal(err)
		}
		if result.Disposition != PersistenceAccepted {
			t.Fatalf("retry = %#v", result)
		}
		replay, err := Accept(ctx, pool, candidate, command)
		if err != nil || replay.Disposition != PersistenceReplayed {
			t.Fatalf("exact replay = %#v, %v", replay, err)
		}
	})
}

func databaseCandidate(t *testing.T, ctx context.Context, pool *pgxpool.Pool) Candidate {
	t.Helper()
	cfg, state, prepared := testPrepared(t)
	output := validOutput(prepared, state)
	cfg.Model.(*fakeModel).result = successfulResult(prepared, output, cfg.ModelPolicy.Model)
	candidate, err := Generate(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	sourceArtifact := uuid.NewString()
	now := time.Now().UTC().Truncate(time.Microsecond)
	_, err = pool.Exec(ctx, `INSERT INTO core.artifacts(id,sha256,size_bytes,media_type,logical_kind,storage_path,created_at) VALUES($1,$2,1,'text/markdown','planner-test-task',$3,$4)`, sourceArtifact, strings.Repeat("9", 64), "planner-tests/"+sourceArtifact, now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO core.projects(id,name,status,created_at,updated_at) VALUES($1,$2,'active',$3,$3)`, candidate.ProjectID, "planner-test-"+candidate.ProjectID, now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO core.project_sources(id,project_id,canonical_source_path,managed_repository_path,current_commit,current_tree,dirty_state,remotes) VALUES($1,$2,$3,$4,$5,$6,'{}','[]')`, candidate.ProjectSourceID, candidate.ProjectID, "/planner/source/"+candidate.ProjectID, "/planner/managed/"+candidate.ProjectID, candidate.SourceCommit, candidate.SourceTree)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO core.tasks(id,project_id,external_task_id,status,created_at,updated_at) VALUES($1,$2,$3,'draft',$4,$4)`, candidate.TaskID, candidate.ProjectID, "planner-"+strings.ReplaceAll(candidate.TaskID, "-", ""), now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO core.task_versions(id,task_id,version_number,source_artifact_id,title,goal,risk_class,mutation_class,network_profile,priority,read_only_investigation,scope,excluded_scope,verification_plan,budget,secret_requirements,expected_paths,operator_checkpoints,created_at) VALUES($1,$2,1,$3,'Planner test','Planner persistence','low','database_migration','none',1,false,'["planner"]','[]','[]','{}','[]','["internal/planner"]','[]',$4)`, candidate.TaskVersionID, candidate.TaskID, sourceArtifact, now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `UPDATE core.tasks SET status='admitted',accepted_version_id=$2 WHERE id=$1`, candidate.TaskID, candidate.TaskVersionID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO core.runs(id,project_id,task_id,task_version_id,project_source_id,status,admitted_task_aggregate_version,source_commit,source_tree,coordinator_identity,created_at,updated_at) VALUES($1,$2,$3,$4,$5,'active',1,$6,$7,'planner-test',$8,$8)`, candidate.RunID, candidate.ProjectID, candidate.TaskID, candidate.TaskVersionID, candidate.ProjectSourceID, candidate.SourceCommit, candidate.SourceTree, now)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM core.events WHERE project_id=$1", candidate.ProjectID)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM core.plan_steps WHERE plan_id=$1", candidate.PlanID)
		_, _ = pool.Exec(cleanupCtx, "UPDATE core.plans SET accepted_version_id=NULL,accepted_operation_id=NULL,accepted_by=NULL,accepted_at=NULL WHERE id=$1", candidate.PlanID)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM core.plan_versions WHERE plan_id=$1", candidate.PlanID)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM core.plans WHERE id=$1", candidate.PlanID)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM core.runs WHERE id=$1", candidate.RunID)
		_, _ = pool.Exec(cleanupCtx, "UPDATE core.tasks SET accepted_version_id=NULL,status='draft' WHERE id=$1", candidate.TaskID)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM core.task_versions WHERE id=$1", candidate.TaskVersionID)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM core.tasks WHERE id=$1", candidate.TaskID)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM core.project_sources WHERE id=$1", candidate.ProjectSourceID)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM core.projects WHERE id=$1", candidate.ProjectID)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM core.artifacts WHERE id=$1", sourceArtifact)
	})
	return candidate
}
