package context

import (
	stdctx "context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"revolvr/internal/retrieval"
	storage "revolvr/internal/storage/postgres"
)

func TestPostgresImmutablePackageAndBoundedReadOnlyHostQuery(t *testing.T) {
	databaseURL := os.Getenv("REVOLVR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("REVOLVR_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := stdctx.WithTimeout(stdctx.Background(), time.Minute)
	defer cancel()
	pool, err := storage.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	now := time.Date(2026, 8, 7, 14, 0, 0, 0, time.UTC)
	projectID, artifactID := uuid.NewString(), uuid.NewString()
	projectUUID := pgtype.UUID{Bytes: uuid.MustParse(projectID), Valid: true}
	artifactUUID := pgtype.UUID{Bytes: uuid.MustParse(artifactID), Valid: true}
	queries := storage.New(pool)
	if _, err := queries.InsertProject(ctx, storage.InsertProjectParams{ID: projectUUID, Name: "context-" + projectID, Status: "active", CreatedAt: dbTime(now), UpdatedAt: dbTime(now)}); err != nil {
		t.Fatal(err)
	}
	artifactContent := []byte("immutable artifact range bytes " + artifactID)
	artifactSHA := hash(artifactContent)
	if _, err := queries.InsertArtifact(ctx, storage.InsertArtifactParams{ID: artifactUUID, Sha256: artifactSHA, SizeBytes: int64(len(artifactContent)), MediaType: "text/plain", LogicalKind: "context_fixture", StoragePath: "context-fixture/" + artifactID, CreatedAt: dbTime(now)}); err != nil {
		t.Fatal(err)
	}
	revision := strings.Repeat("a", 40)
	inline := inlineCandidate("inline", retrieval.AuthorityExactSource, "internal/source.go", "Source", "func Source() {}")
	artifact := Candidate{Retrieval: retrieval.Candidate{Identity: "artifact", Authority: retrieval.AuthorityCanonicalEvidence, SourceKind: "artifact", SourceIdentity: "artifact:" + artifactID, SourceSHA256: artifactSHA}, ArtifactRange: &ArtifactRange{ArtifactID: artifactID, SHA256: artifactSHA, SizeBytes: int64(len(artifactContent)), Start: 0, End: int64(len(artifactContent)), MediaType: "text/plain", Resolved: true}}
	trajectorySHA := strings.Repeat("b", 64)
	trajectory := Candidate{Retrieval: retrieval.Candidate{Identity: "trajectory", Authority: retrieval.AuthorityStructural, SourceKind: "trajectory", SourceIdentity: "trajectory:reserved", SourceSHA256: trajectorySHA}, Trajectory: &TrajectoryRange{TrajectoryID: "reserved-trajectory", SHA256: trajectorySHA, Start: 10, End: 20, MediaType: "application/x-ndjson", Resolved: true}}
	value, err := Compile(CompileRequest{
		ProjectID: projectID, Role: RoleAuditor, SourceRevision: revision,
		RetrievalConfiguration: retrieval.Report{ConfigurationVersion: retrieval.ConfigurationVersion, SourceRevision: revision},
		Candidates:             []Candidate{trajectory, artifact, inline},
	})
	if err != nil {
		t.Fatal(err)
	}
	store, _ := NewPostgresStore(pool)
	first, err := store.Persist(ctx, value, now)
	if err != nil || first.Replay {
		t.Fatalf("first persist = %#v, %v", first, err)
	}
	second, err := store.Persist(ctx, value, now.Add(time.Second))
	if err != nil || !second.Replay {
		t.Fatalf("replay persist = %#v, %v", second, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE telemetry.context_packages SET role='planner' WHERE id=$1`, value.ID); err == nil {
		t.Fatal("immutable context package accepted update")
	}

	resolver := &fixtureArtifactResolver{content: artifactContent}
	host, err := NewHostQuery(pool, resolver, nil)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := host.Manifest(ctx, value.ID)
	if err != nil || !reflect.DeepEqual(manifest, value.Manifest) {
		t.Fatalf("manifest = %#v, %v", manifest, err)
	}
	if _, err := host.AdmittedItems(ctx, value.ID, 2); !errors.Is(err, ErrQueryBoundExceeded) {
		t.Fatalf("bounded items error = %v", err)
	}
	items, err := host.AdmittedItems(ctx, value.ID, 3)
	if err != nil || len(items) != 3 || items[0].CandidateIdentity != "inline" {
		t.Fatalf("admitted items = %#v, %v", items, err)
	}
	rangeResult, err := host.ArtifactRange(ctx, value.ID, "artifact", 0, int64(len(artifactContent)))
	if err != nil || string(rangeResult.Content) != string(artifactContent) || resolver.calls != 1 {
		t.Fatalf("artifact range = %#v, calls=%d, err=%v", rangeResult, resolver.calls, err)
	}
	if _, err := host.ArtifactRange(ctx, value.ID, "artifact", 1, int64(len(artifactContent))); !errors.Is(err, ErrReferenceNotAdmitted) {
		t.Fatalf("unadmitted range error = %v", err)
	}
	if _, err := host.TrajectoryRange(ctx, value.ID, "trajectory", 10, 20); !errors.Is(err, ErrTrajectoryUnavailable) {
		t.Fatalf("reserved trajectory error = %v", err)
	}

	allowed := map[string]bool{"Manifest": true, "AdmittedItems": true, "ArtifactRange": true, "TrajectoryRange": true}
	typeOfHost := reflect.TypeOf(host)
	for index := 0; index < typeOfHost.NumMethod(); index++ {
		if !allowed[typeOfHost.Method(index).Name] {
			t.Fatalf("host query exposes non-read-only method %s", typeOfHost.Method(index).Name)
		}
	}
}

type fixtureArtifactResolver struct {
	content []byte
	calls   int
}

func (r *fixtureArtifactResolver) ResolveArtifactRange(_ stdctx.Context, identity, expectedSHA string, start, end, maximum int64) (RangeResult, error) {
	r.calls++
	if expectedSHA != hash(r.content) || start != 0 || end != int64(len(r.content)) || maximum != MaximumQueryRangeBytes {
		return RangeResult{}, ErrReferenceNotAdmitted
	}
	return RangeResult{Identity: identity, SHA256: expectedSHA, Start: start, End: end, MediaType: "text/plain", Content: append([]byte(nil), r.content...)}, nil
}

func dbTime(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}
