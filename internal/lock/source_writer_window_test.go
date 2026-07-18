package lock

import (
	"strings"
	"testing"
	"time"
)

func TestRequiredSourceWriterTimeout(t *testing.T) {
	for _, test := range []struct {
		name  string
		codex time.Duration
		git   time.Duration
		want  time.Duration
	}{
		{name: "level one external", codex: 30 * time.Minute, git: 30 * time.Second, want: 32 * time.Minute},
		{name: "custom", codex: 45 * time.Second, git: 12 * time.Second, want: 2*time.Minute + 9*time.Second},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := RequiredSourceWriterTimeout(test.codex, test.git)
			if err != nil || got != test.want {
				t.Fatalf("RequiredSourceWriterTimeout(%s, %s) = %s, %v; want %s", test.codex, test.git, got, err, test.want)
			}
		})
	}
}

func TestRequiredSourceWriterTimeoutRejectsInvalidAndOverflowingAuthority(t *testing.T) {
	const maxDuration = time.Duration(1<<63 - 1)
	for _, test := range []struct {
		name  string
		codex time.Duration
		git   time.Duration
		want  string
	}{
		{name: "missing Codex", git: time.Second, want: "positive"},
		{name: "negative Git", codex: time.Second, git: -1, want: "positive"},
		{name: "Codex overflow", codex: maxDuration, git: time.Second, want: "overflows"},
		{name: "Git multiplication overflow", codex: time.Second, git: maxDuration / 2, want: "overflows"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := RequiredSourceWriterTimeout(test.codex, test.git); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("RequiredSourceWriterTimeout(%s, %s) error = %v, want %q", test.codex, test.git, err, test.want)
			}
		})
	}
}
