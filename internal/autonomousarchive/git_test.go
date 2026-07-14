package autonomousarchive

import (
	"context"
	"strings"
	"testing"

	"revolvr/internal/runner"
)

func TestVerifyCommitReportsFirstInvalidFileDeterministically(t *testing.T) {
	const firstPath = "archive/a-file"
	const secondPath = "archive/z-file"
	sha := strings.Repeat("a", 40)
	tests := []struct {
		name       string
		fileResult runner.Result
		want       string
	}{
		{
			name:       "missing",
			fileResult: runner.Result{ExitCode: 1},
			want:       `archive git: expected committed file "archive/a-file" is missing`,
		},
		{
			name:       "different bytes",
			fileResult: runner.Result{ExitCode: 0, Stdout: "different"},
			want:       `archive git: committed file "archive/a-file" has different bytes`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i := 0; i < 1_000; i++ {
				var fileReads []string
				g := gitConfig{runner: func(_ context.Context, command runner.Command) runner.Result {
					switch {
					case len(command.Args) == 4 && command.Args[0] == "show" && command.Args[1] == "-s":
						return runner.Result{ExitCode: 0}
					case len(command.Args) == 7 && command.Args[0] == "diff-tree":
						return runner.Result{ExitCode: 0, Stdout: secondPath + "\x00" + firstPath + "\x00"}
					case len(command.Args) == 2 && command.Args[0] == "show":
						path := strings.TrimPrefix(command.Args[1], sha+":")
						fileReads = append(fileReads, path)
						return tt.fileResult
					default:
						t.Fatalf("iteration %d: unexpected git arguments: %q", i, command.Args)
						return runner.Result{ExitCode: 1}
					}
				}}

				err := verifyCommit(
					context.Background(),
					g,
					sha,
					[]string{secondPath, firstPath},
					map[string][]byte{secondPath: []byte("want-z"), firstPath: []byte("want-a")},
					nil,
				)
				if err == nil || err.Error() != tt.want {
					t.Fatalf("iteration %d: error = %v, want %q", i, err, tt.want)
				}
				if len(fileReads) != 1 || fileReads[0] != firstPath {
					t.Fatalf("iteration %d: file reads = %q, want [%q]", i, fileReads, firstPath)
				}
			}
		})
	}
}
