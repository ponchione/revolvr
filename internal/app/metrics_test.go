package app

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"revolvr/internal/ledger"
)

func TestShowMetricsLiveReadIsDeterministicAndSidecarFree(t *testing.T) {
	repo := t.TempDir()
	runtime := filepath.Join(repo, ".revolvr")
	if err := os.MkdirAll(runtime, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(runtime, "ledger.sqlite")
	store, err := ledger.OpenWithClock(context.Background(), path, func() time.Time { return time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC) })
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadDir(runtime)
	if err != nil {
		t.Fatal(err)
	}
	a, err := ShowMetrics(context.Background(), Config{WorkDir: repo}, "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := ShowMetrics(context.Background(), Config{WorkDir: repo}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatal("equal logical reads differ")
	}
	after, err := os.ReadDir(runtime)
	if err != nil {
		t.Fatal(err)
	}
	if names(before) != names(after) {
		t.Fatalf("read created runtime entries: before=%v after=%v", names(before), names(after))
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Lstat(path + suffix); !os.IsNotExist(err) {
			t.Fatalf("read created %s", suffix)
		}
	}
}
func names(entries []os.DirEntry) string {
	value := ""
	for _, entry := range entries {
		value += entry.Name() + "\n"
	}
	return value
}
