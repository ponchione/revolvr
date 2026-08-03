package postgres

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestOpen(t *testing.T) {
	databaseURL := os.Getenv("REVOLVR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("REVOLVR_TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	got, err := New(pool).Ping(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got != 1 {
		t.Fatalf("Ping() = %d, want 1", got)
	}
}
