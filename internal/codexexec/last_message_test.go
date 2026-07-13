package codexexec

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"revolvr/internal/redact"
	"revolvr/internal/runner"
)

func TestLastMessagePublicationFailuresNeverExposeRawCanonical(t *testing.T) {
	points := []LastMessageFailurePoint{
		LastMessageFailureAfterChild,
		LastMessageFailureRead,
		LastMessageFailureRedact,
		LastMessageFailureTempWrite,
		LastMessageFailureFileSync,
		LastMessageFailureRename,
		LastMessageFailureDirectorySync,
	}
	for _, point := range points {
		t.Run(string(point), func(t *testing.T) {
			workDir := t.TempDir()
			canonical := filepath.Join(workDir, "run/final.txt")
			rawTemporary := lastMessageRawPath(canonical)
			redactedTemporary := lastMessageRedactedPath(canonical)
			secret := "token-super-secret"
			redactor := newLastMessageTestRedactor(t, secret)
			injected := errors.New("injected publication failure")

			result, err := Run(context.Background(), Config{
				WorkingDir: workDir,
				Prompt:     "test publication failure",
				Redactor:   redactor,
				Artifacts: ArtifactPaths{
					StdoutJSONL: "run/codex.jsonl",
					LastMessage: "run/final.txt",
				},
				CommandRunner: func(_ context.Context, command runner.Command) runner.Result {
					if got := argAfter(command.Args, "--output-last-message"); got != rawTemporary {
						t.Fatalf("last-message argument = %q, want %q", got, rawTemporary)
					}
					assertPathAbsent(t, canonical)
					assertFileMode(t, rawTemporary, 0o600)
					if err := os.WriteFile(rawTemporary, []byte("prefix "+secret+" suffix\n"), 0o644); err != nil {
						t.Fatalf("write raw last-message temporary: %v", err)
					}
					return runner.Result{ExitCode: 0}
				},
				LastMessageFailureInjector: func(got LastMessageFailurePoint) error {
					if got != point {
						return nil
					}
					if point == LastMessageFailureDirectorySync {
						assertCanonicalContainsOnlyRedactedLastMessage(t, canonical, secret)
					} else {
						assertPathAbsent(t, canonical)
					}
					return injected
				},
			})
			if err != nil {
				t.Fatalf("run Codex: %v", err)
			}
			if !errors.Is(result.ArtifactError, injected) {
				t.Fatalf("artifact error = %v, want injected failure", result.ArtifactError)
			}
			if point == LastMessageFailureDirectorySync {
				assertCanonicalContainsOnlyRedactedLastMessage(t, canonical, secret)
				assertFileMode(t, canonical, 0o644)
			} else {
				assertPathAbsent(t, canonical)
			}
			assertPathAbsent(t, rawTemporary)
			assertPathAbsent(t, redactedTemporary)
			for label, value := range map[string]string{
				"artifact error": errorString(result.ArtifactError),
				"final message":  result.FinalMessage,
				"stdout":         result.Stdout.Content,
				"stderr":         result.Stderr.Content,
			} {
				if strings.Contains(value, secret) {
					t.Fatalf("%s leaked secret: %q", label, value)
				}
			}
		})
	}
}

func TestLastMessageRestartCleansOrphansAndMissingOutput(t *testing.T) {
	workDir := t.TempDir()
	canonical := filepath.Join(workDir, "run/final.txt")
	rawTemporary := lastMessageRawPath(canonical)
	redactedTemporary := lastMessageRedactedPath(canonical)
	if err := os.MkdirAll(filepath.Dir(canonical), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(canonical, []byte("stale canonical\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rawTemporary, []byte("orphan raw secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(redactedTemporary, []byte("orphan redacted\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Run(context.Background(), Config{
		WorkingDir: workDir,
		Prompt:     "test restart cleanup",
		Artifacts: ArtifactPaths{
			StdoutJSONL: "run/codex.jsonl",
			LastMessage: "run/final.txt",
		},
		CommandRunner: func(_ context.Context, command runner.Command) runner.Result {
			assertPathAbsent(t, canonical)
			assertPathAbsent(t, redactedTemporary)
			assertFileMode(t, rawTemporary, 0o600)
			raw, err := os.ReadFile(rawTemporary)
			if err != nil {
				t.Fatal(err)
			}
			if len(raw) != 0 {
				t.Fatalf("raw temporary contains stale bytes: %q", raw)
			}
			return runner.Result{ExitCode: 0}
		},
	})
	if err != nil {
		t.Fatalf("run Codex: %v", err)
	}
	if result.ArtifactError != nil {
		t.Fatalf("artifact error = %v", result.ArtifactError)
	}
	if result.FinalMessage != "" {
		t.Fatalf("final message = %q, want empty for missing output", result.FinalMessage)
	}
	assertPathAbsent(t, canonical)
	assertPathAbsent(t, rawTemporary)
	assertPathAbsent(t, redactedTemporary)
}

func TestLastMessageWithoutRedactorPreservesBytesAndParsing(t *testing.T) {
	workDir := t.TempDir()
	canonical := filepath.Join(workDir, "run/final.txt")
	rawContent := "  final raw message  \n\n"

	result, err := Run(context.Background(), Config{
		WorkingDir: workDir,
		Prompt:     "test unredacted compatibility",
		Artifacts: ArtifactPaths{
			StdoutJSONL: "run/codex.jsonl",
			LastMessage: "run/final.txt",
		},
		CommandRunner: func(_ context.Context, command runner.Command) runner.Result {
			if err := os.WriteFile(argAfter(command.Args, "--output-last-message"), []byte(rawContent), 0o644); err != nil {
				t.Fatal(err)
			}
			return runner.Result{ExitCode: 0}
		},
	})
	if err != nil {
		t.Fatalf("run Codex: %v", err)
	}
	if result.ArtifactError != nil {
		t.Fatalf("artifact error = %v", result.ArtifactError)
	}
	if result.FinalMessage != "final raw message" {
		t.Fatalf("final message = %q, want trimmed raw message", result.FinalMessage)
	}
	assertFile(t, canonical, rawContent)
	assertFileMode(t, canonical, 0o644)
}

func TestLastMessageRejectsUnsafeRawTemporaryMode(t *testing.T) {
	workDir := t.TempDir()
	canonical := filepath.Join(workDir, "run/final.txt")
	rawTemporary := lastMessageRawPath(canonical)

	result, err := Run(context.Background(), Config{
		WorkingDir: workDir,
		Prompt:     "test unsafe raw mode",
		Artifacts: ArtifactPaths{
			StdoutJSONL: "run/codex.jsonl",
			LastMessage: "run/final.txt",
		},
		CommandRunner: func(_ context.Context, command runner.Command) runner.Result {
			if err := os.WriteFile(rawTemporary, []byte("unsafe raw content\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(rawTemporary, 0o644); err != nil {
				t.Fatal(err)
			}
			return runner.Result{ExitCode: 0}
		},
	})
	if err != nil {
		t.Fatalf("run Codex: %v", err)
	}
	if result.ArtifactError == nil || !strings.Contains(result.ArtifactError.Error(), "unsafe mode 0644") {
		t.Fatalf("artifact error = %v, want unsafe-mode failure", result.ArtifactError)
	}
	assertPathAbsent(t, canonical)
	assertPathAbsent(t, rawTemporary)
	assertPathAbsent(t, lastMessageRedactedPath(canonical))
}

func newLastMessageTestRedactor(t *testing.T, secret string) *redact.Redactor {
	t.Helper()
	redactor, _, err := redact.New(
		redact.Policy{SchemaVersion: redact.PolicySchemaVersion, EnvironmentVariables: []string{"TOKEN"}},
		func(name string) (string, bool) { return secret, name == "TOKEN" },
	)
	if err != nil {
		t.Fatalf("create redactor: %v", err)
	}
	return redactor
}

func assertCanonicalContainsOnlyRedactedLastMessage(t *testing.T, path, secret string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read canonical last-message: %v", err)
	}
	if strings.Contains(string(raw), secret) || !strings.Contains(string(raw), redact.Replacement) {
		t.Fatalf("canonical last-message is not safely redacted: %q", raw)
	}
}
