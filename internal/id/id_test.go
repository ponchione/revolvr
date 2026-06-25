package id

import (
	"sort"
	"testing"
)

func TestNewReturnsNonEmptyUniqueIDs(t *testing.T) {
	const count = 1000
	seen := make(map[string]bool, count)

	for i := 0; i < count; i++ {
		got := New()
		if got == "" {
			t.Fatal("id is empty")
		}
		if seen[got] {
			t.Fatalf("duplicate id %q", got)
		}
		seen[got] = true
	}
}

func TestNewIDsSortByGenerationOrder(t *testing.T) {
	const count = 1000
	ids := make([]string, 0, count)
	for i := 0; i < count; i++ {
		ids = append(ids, New())
	}

	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)

	for i := range ids {
		if ids[i] != sorted[i] {
			t.Fatalf("ids are not sorted by generation order at index %d: got %q, want %q", i, ids[i], sorted[i])
		}
	}
}
