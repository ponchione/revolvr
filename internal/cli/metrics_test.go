package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"revolvr/internal/ledger"
)

func TestMetricsShowHumanJSONAndHelp(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".revolvr"), 0700); err != nil {
		t.Fatal(err)
	}
	store, err := ledger.OpenWithClock(context.Background(), filepath.Join(repo, ".revolvr", "ledger.sqlite"), func() time.Time { return time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC) })
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	human, err := executeCLI(t, repo, "metrics", "show")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Metrics schema: autonomous-loop-metrics-v1", "Task success: 0/0 terminal=0", "Omissions: 0"} {
		if !strings.Contains(human, want) {
			t.Fatalf("human missing %q:\n%s", want, human)
		}
	}
	jsonOut, err := executeCLI(t, repo, "metrics", "show", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(jsonOut, "{\n  \"schema_version\": \"autonomous-loop-metrics-v1\"") || !strings.HasSuffix(jsonOut, "\n") {
		t.Fatalf("json output=%q", jsonOut)
	}
	help, err := executeCLI(t, repo, "metrics", "--help")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(help, "show") {
		t.Fatalf("help=%s", help)
	}
}
