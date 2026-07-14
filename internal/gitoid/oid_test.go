package gitoid

import (
	"strings"
	"testing"
)

func TestValidAcceptsOnlyFullLowercaseSHA1AndSHA256ObjectIDs(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "SHA-1", value: strings.Repeat("0123456789abcdef", 2) + "01234567", want: true},
		{name: "SHA-256", value: strings.Repeat("0123456789abcdef", 4), want: true},
		{name: "empty"},
		{name: "abbreviated", value: strings.Repeat("a", SHA1Length-1)},
		{name: "between algorithms", value: strings.Repeat("a", SHA1Length+1)},
		{name: "short SHA-256", value: strings.Repeat("a", SHA256Length-1)},
		{name: "long SHA-256", value: strings.Repeat("a", SHA256Length+1)},
		{name: "uppercase SHA-1", value: strings.Repeat("A", SHA1Length)},
		{name: "uppercase SHA-256", value: strings.Repeat("A", SHA256Length)},
		{name: "non-hex SHA-1", value: strings.Repeat("g", SHA1Length)},
		{name: "non-hex SHA-256", value: strings.Repeat("z", SHA256Length)},
		{name: "leading whitespace", value: " " + strings.Repeat("a", SHA1Length-1)},
		{name: "trailing newline", value: strings.Repeat("a", SHA1Length-1) + "\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Valid(tt.value); got != tt.want {
				t.Fatalf("Valid(%q) = %t, want %t", tt.value, got, tt.want)
			}
		})
	}
}
