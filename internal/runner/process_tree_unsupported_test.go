//go:build !unix

package runner

import (
	"context"
	"errors"
	"testing"
)

func TestRunFailsClosedWhenProcessTreeBoundaryIsUnsupported(t *testing.T) {
	result := Run(context.Background(), Command{Name: "unsupported"})
	if !errors.Is(result.Err, ErrProcessTreeUnsupported) {
		t.Fatalf("error = %v, want ErrProcessTreeUnsupported", result.Err)
	}
	if result.ExitCode != -1 {
		t.Fatalf("exit code = %d, want -1", result.ExitCode)
	}
}
