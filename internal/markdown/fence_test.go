package markdown

import "testing"

func TestFenceScanTracksMarkersLengthsIndentationAndClosure(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  []FenceLine
	}{
		{
			name:  "indented backticks and longer close",
			lines: []string{"   ````markdown", "## Task: inert", "```", "   `````", "## Task: structural"},
			want:  []FenceLine{LineFenceBoundary, LineInsideFence, LineInsideFence, LineFenceBoundary, LineOutsideFence},
		},
		{
			name:  "tilde fence ignores other marker",
			lines: []string{"~~~ text", "```", "## Verification", "~~~~", "## Verification"},
			want:  []FenceLine{LineFenceBoundary, LineInsideFence, LineInsideFence, LineFenceBoundary, LineOutsideFence},
		},
		{
			name:  "unclosed fence remains open",
			lines: []string{"```", "## Changed Files", "still fenced"},
			want:  []FenceLine{LineFenceBoundary, LineInsideFence, LineInsideFence},
		},
		{
			name:  "four spaces is not a fence",
			lines: []string{"    ```", "## Task: structural"},
			want:  []FenceLine{LineOutsideFence, LineOutsideFence},
		},
		{
			name:  "backtick in info string is not a fence",
			lines: []string{"```markdown`example", "## Task: structural"},
			want:  []FenceLine{LineOutsideFence, LineOutsideFence},
		},
		{
			name:  "CRLF closing line",
			lines: []string{"~~~", "content", "~~~\r", "outside"},
			want:  []FenceLine{LineFenceBoundary, LineInsideFence, LineFenceBoundary, LineOutsideFence},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var fence Fence
			for i, line := range tt.lines {
				if got := fence.Scan(line); got != tt.want[i] {
					t.Fatalf("line %d (%q) = %v, want %v", i+1, line, got, tt.want[i])
				}
			}
		})
	}
}
