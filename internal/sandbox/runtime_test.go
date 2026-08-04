package sandbox

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeSandboxRuntime struct {
	mu         sync.Mutex
	calls      []string
	exec       func(context.Context) CommandResult
	removed    chan struct{}
	removeOnce sync.Once
}

func (f *fakeSandboxRuntime) Create(_ context.Context, specification Specification) (SandboxHandle, error) {
	f.record("create")
	return SandboxHandle{ID: strings.Repeat("a", 64), Name: "fixture", Command: append([]string(nil), specification.Command...)}, nil
}

func (f *fakeSandboxRuntime) Exec(ctx context.Context, _ SandboxHandle, _ CommandSpec) (CommandResult, error) {
	f.record("exec")
	if f.exec != nil {
		return f.exec(ctx), nil
	}
	return CommandResult{ExitCode: 0, Stdout: "stdout\n", Stderr: "stderr\n", StdoutTruncatedBytes: 3, StderrTruncatedBytes: 4}, nil
}

func (f *fakeSandboxRuntime) Stop(context.Context, SandboxHandle) error {
	f.record("stop")
	return nil
}

func (f *fakeSandboxRuntime) Inspect(context.Context, SandboxHandle) (SandboxStatus, error) {
	f.record("inspect")
	return SandboxStatus{State: "exited"}, nil
}

func (f *fakeSandboxRuntime) Remove(context.Context, SandboxHandle) error {
	f.record("remove")
	if f.removed != nil {
		f.removeOnce.Do(func() { close(f.removed) })
	}
	return nil
}

func (f *fakeSandboxRuntime) record(call string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
}

func (f *fakeSandboxRuntime) recorded() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func TestManagerRecordsLifecycleEvidenceAndArtifacts(t *testing.T) {
	specification := validatedSandboxSpecification(t)
	state := t.TempDir()
	runtime := &fakeSandboxRuntime{}
	manager, err := NewManager(runtime, state)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	manager.now = func() time.Time {
		now = now.Add(time.Second)
		return now
	}
	evidence, err := manager.Run(context.Background(), specification)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, want := runtime.recorded(), []string{"create", "exec", "inspect", "remove"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("runtime calls = %v, want %v", got, want)
	}
	if evidence.SchemaVersion != EvidenceSchemaVersion || evidence.Runtime != "docker-rootless" || evidence.ExitCode != 0 || evidence.TimedOut || evidence.Cancelled {
		t.Fatalf("evidence = %#v", evidence)
	}
	states := make([]string, 0, len(evidence.Transitions))
	for _, transition := range evidence.Transitions {
		states = append(states, transition.State)
	}
	if want := []string{"requested", "validated", "creating", "running", "exited", "removed"}; !reflect.DeepEqual(states, want) {
		t.Fatalf("transitions = %v, want %v", states, want)
	}
	if evidence.Stdout.TruncatedBytes != 3 || evidence.Stderr.TruncatedBytes != 4 {
		t.Fatalf("truncation evidence = %#v %#v", evidence.Stdout, evidence.Stderr)
	}
	for path, want := range map[string]string{evidence.Stdout.Path: "stdout\n", evidence.Stderr.Path: "stderr\n"} {
		raw, readErr := os.ReadFile(path)
		if readErr != nil || string(raw) != want {
			t.Fatalf("artifact %s = %q, %v; want %q", path, raw, readErr, want)
		}
	}
	raw, err := os.ReadFile(evidence.EvidencePath)
	if err != nil {
		t.Fatal(err)
	}
	var recorded Evidence
	if err := json.Unmarshal(raw, &recorded); err != nil || !reflect.DeepEqual(recorded, evidence) {
		t.Fatalf("recorded evidence = %#v, %v; want %#v", recorded, err, evidence)
	}
}

func TestManagerTimeoutAndCancellationForceExactCleanup(t *testing.T) {
	tests := []struct {
		name string
		run  func(context.Context, context.CancelFunc, *Manager) (Evidence, error)
	}{
		{
			name: "timeout",
			run: func(ctx context.Context, _ context.CancelFunc, manager *Manager) (Evidence, error) {
				return manager.Run(ctx, validatedSandboxSpecification(t))
			},
		},
		{
			name: "cancellation",
			run: func(ctx context.Context, cancel context.CancelFunc, manager *Manager) (Evidence, error) {
				cancel()
				return manager.Run(ctx, validatedSandboxSpecification(t))
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := &fakeSandboxRuntime{}
			runtime.exec = func(ctx context.Context) CommandResult {
				if test.name == "timeout" {
					return CommandResult{ExitCode: -1, Error: context.DeadlineExceeded, TimedOut: true}
				}
				<-ctx.Done()
				return CommandResult{ExitCode: -1, Error: ctx.Err(), Cancelled: true}
			}
			manager, err := NewManager(runtime, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			evidence, err := test.run(ctx, cancel, manager)
			if err == nil {
				t.Fatal("Run error = nil")
			}
			if (test.name == "timeout" && !evidence.TimedOut) || (test.name == "cancellation" && !evidence.Cancelled) {
				t.Fatalf("evidence = %#v", evidence)
			}
			calls := runtime.recorded()
			if !reflect.DeepEqual(calls[len(calls)-3:], []string{"stop", "inspect", "remove"}) {
				t.Fatalf("cleanup calls = %v", calls)
			}
		})
	}
}

func TestCheckSpecificationRejectsFilesystemIdentitySubstitution(t *testing.T) {
	specification := validatedSandboxSpecification(t)
	workspace := specification.Mounts[len(specification.Mounts)-1].SourcePath
	if err := os.Rename(workspace, workspace+"-old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(workspace, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := CheckSpecification(specification); err == nil || !strings.Contains(err.Error(), "identity changed") {
		t.Fatalf("CheckSpecification error = %v", err)
	}
}

func TestServerSocketProtocolAndDisconnectCleanup(t *testing.T) {
	root := t.TempDir()
	state := filepath.Join(root, "state")
	if err := os.Mkdir(state, 0o700); err != nil {
		t.Fatal(err)
	}
	runtime := &fakeSandboxRuntime{removed: make(chan struct{})}
	started := make(chan struct{})
	runtime.exec = func(ctx context.Context) CommandResult {
		close(started)
		<-ctx.Done()
		return CommandResult{ExitCode: -1, Error: ctx.Err(), Cancelled: true}
	}
	manager, err := NewManager(runtime, state)
	if err != nil {
		t.Fatal(err)
	}
	socketPath := filepath.Join(root, "socket", "sandboxd.sock")
	listener, err := ListenUnix(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	info, err := os.Lstat(socketPath)
	if err != nil || info.Mode().Perm() != 0o600 || info.Mode()&os.ModeSocket == 0 {
		t.Fatalf("socket mode = %v, %v", info, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- (&Server{Manager: manager}).Serve(ctx, listener) }()

	malformed := dialUnix(t, socketPath)
	if err := writeFrame(malformed, map[string]any{"schema_version": ProtocolSchemaVersion, "unknown": true}); err != nil {
		t.Fatal(err)
	}
	raw, err := readFrame(malformed)
	if err != nil {
		t.Fatal(err)
	}
	_ = malformed.Close()
	var response RunResponse
	if err := json.Unmarshal(raw, &response); err != nil || response.Error == "" {
		t.Fatalf("malformed response = %#v, %v", response, err)
	}
	if calls := runtime.recorded(); len(calls) != 0 {
		t.Fatalf("malformed request reached runtime: %v", calls)
	}

	client := dialUnix(t, socketPath)
	if err := writeFrame(client, RunRequest{SchemaVersion: ProtocolSchemaVersion, Specification: validatedSandboxSpecification(t)}); err != nil {
		t.Fatal(err)
	}
	<-started
	_ = client.Close()
	select {
	case <-runtime.removed:
	case <-time.After(5 * time.Second):
		t.Fatal("client disconnect did not remove sandbox")
	}
	if calls := runtime.recorded(); !reflect.DeepEqual(calls[len(calls)-3:], []string{"stop", "inspect", "remove"}) {
		t.Fatalf("disconnect cleanup calls = %v", calls)
	}
	cancel()
	select {
	case err := <-served:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not stop")
	}
}

func validatedSandboxSpecification(t *testing.T) Specification {
	t.Helper()
	policy, request := sandboxFixture(t)
	request.RuntimeProfile = ProfileCompatible
	specification, err := Validate(request, policy)
	if err != nil {
		t.Fatal(err)
	}
	return specification
}

func dialUnix(t *testing.T, path string) *net.UnixConn {
	t.Helper()
	connection, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	if err := connection.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	return connection
}

var _ SandboxRuntime = (*fakeSandboxRuntime)(nil)
