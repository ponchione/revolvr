package postgres

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"revolvr/internal/id"
)

func TestArtifactAndEventTransaction(t *testing.T) {
	databaseURL := os.Getenv("REVOLVR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("REVOLVR_TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	queries := New(pool)
	createdAt := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	artifact := InsertArtifactParams{
		ID:          testUUID(),
		Sha256:      testSHA256(),
		SizeBytes:   12,
		MediaType:   "application/json",
		LogicalKind: "test-result",
		StoragePath: "artifacts/test-result.json",
		CreatedAt:   createdAt,
	}
	event := AppendEventParams{
		ID:               testUUID(),
		EventType:        "artifact.created",
		AggregateType:    "test",
		AggregateID:      testUUID(),
		AggregateVersion: 1,
		Payload:          []byte(`{"result":"committed"}`),
		CreatedAt:        createdAt,
	}

	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		qtx := queries.WithTx(tx)
		if _, err := qtx.InsertArtifact(ctx, artifact); err != nil {
			return err
		}
		_, err := qtx.AppendEvent(ctx, event)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := queries.GetArtifactBySHA256(ctx, artifact.Sha256); err != nil || got.ID != artifact.ID {
		t.Fatalf("get committed artifact: id = %v, err = %v", got.ID, err)
	}
	if got, err := queries.GetEvent(ctx, event.ID); err != nil || got.ID != event.ID {
		t.Fatalf("get committed event: id = %v, err = %v", got.ID, err)
	}

	rolledBackArtifact := artifact
	rolledBackArtifact.ID = testUUID()
	rolledBackArtifact.Sha256 = testSHA256()
	rolledBackEvent := event
	rolledBackEvent.ID = testUUID()
	rolledBackEvent.AggregateID = testUUID()
	callbackErr := errors.New("intentional rollback")
	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		qtx := queries.WithTx(tx)
		if _, err := qtx.InsertArtifact(ctx, rolledBackArtifact); err != nil {
			return err
		}
		if _, err := qtx.AppendEvent(ctx, rolledBackEvent); err != nil {
			return err
		}
		return callbackErr
	})
	if !errors.Is(err, callbackErr) {
		t.Fatalf("BeginFunc() error = %v, want %v", err, callbackErr)
	}
	if _, err := queries.GetArtifactBySHA256(ctx, rolledBackArtifact.Sha256); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("get rolled-back artifact error = %v, want %v", err, pgx.ErrNoRows)
	}
	if _, err := queries.GetEvent(ctx, rolledBackEvent.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("get rolled-back event error = %v, want %v", err, pgx.ErrNoRows)
	}

	duplicate := event
	duplicate.ID = testUUID()
	_, err = queries.AppendEvent(ctx, duplicate)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("append duplicate aggregate version error = %v, want PostgreSQL unique violation", err)
	}
}

func testUUID() pgtype.UUID {
	return pgtype.UUID{Bytes: uuid.MustParse(id.New()), Valid: true}
}

func testSHA256() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(id.New())))
}
