package sandbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRootlessRuntimeSecurityProfile(t *testing.T) {
	if os.Getenv("REVOLVR_SANDBOX_INTEGRATION") != "1" {
		t.Skip("set REVOLVR_SANDBOX_INTEGRATION=1 with a rootless Docker daemon and alpine:3.22 image")
	}
	runtimeDirectory := os.Getenv("XDG_RUNTIME_DIR")
	if !filepath.IsAbs(runtimeDirectory) {
		runtimeDirectory = filepath.Join("/run/user", strconv.Itoa(os.Getuid()))
	}
	host := os.Getenv("REVOLVR_SANDBOX_DOCKER_HOST")
	if host == "" {
		host = "unix://" + filepath.Join(runtimeDirectory, "docker.sock")
	}
	runtime, err := NewDockerRuntime("docker", host, strings.Repeat("d", 64))
	if err != nil {
		t.Fatal(err)
	}
	image := integrationImage(t, runtime)

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	hostSecret := filepath.Join(home, ".revolvr-sandbox-security-test-"+strconv.Itoa(os.Getpid()))
	if err := os.WriteFile(hostSecret, []byte("host-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(hostSecret)

	policy, request := sandboxFixture(t)
	policy.ApprovedImages = []Image{image}
	policy.AllowedProfiles = []RuntimeProfile{ProfileCompatible, ProfileStrict}
	request.Image = image
	request.RuntimeProfile = ProfileCompatible
	request.Resources = Resources{CPUs: 1, MemoryBytes: 128 << 20, PIDs: 64, TimeoutSeconds: 10, TmpfsBytes: 16 << 20}
	policy.MaximumResources = request.Resources
	request.Command = []string{
		"/bin/sh", "-ceu",
		`test "$(id -u)" -ne 0; test ! -r "$1"; test ! -S /var/run/docker.sock; test ! -S /run/docker.sock; ! wget -q -T 2 -O /dev/null http://1.1.1.1; ! touch /outside; printf ok >/tmp/result`,
		"sandbox-security", hostSecret,
	}
	specification, err := Validate(request, policy)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(runtime, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := manager.Run(context.Background(), specification)
	if err != nil {
		t.Fatalf("rootless security fixture: %v; evidence=%#v", err, evidence)
	}
	if evidence.ExitCode != 0 || evidence.RuntimeProfile != ProfileCompatible || evidence.Network != NetworkNone {
		t.Fatalf("rootless security evidence = %#v", evidence)
	}
	assertNoManagedContainers(t, runtime)

	timeoutRequest := request
	timeoutRequest.SandboxID = "sandbox-timeout"
	timeoutRequest.Command = []string{"/bin/sh", "-c", "sleep 30"}
	timeoutRequest.Resources.TimeoutSeconds = 1
	timeoutSpec, err := Validate(timeoutRequest, policy)
	if err != nil {
		t.Fatal(err)
	}
	timeoutEvidence, err := manager.Run(context.Background(), timeoutSpec)
	if err == nil || !timeoutEvidence.TimedOut {
		t.Fatalf("timeout result = %#v, %v", timeoutEvidence, err)
	}
	assertNoManagedContainers(t, runtime)

	cancelRequest := request
	cancelRequest.SandboxID = "sandbox-cancel"
	cancelRequest.Command = []string{"/bin/sh", "-c", "sleep 30"}
	cancelSpec, err := Validate(cancelRequest, policy)
	if err != nil {
		t.Fatal(err)
	}
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancelled := make(chan struct {
		evidence Evidence
		err      error
	}, 1)
	go func() {
		evidence, runErr := manager.Run(cancelCtx, cancelSpec)
		cancelled <- struct {
			evidence Evidence
			err      error
		}{evidence, runErr}
	}()
	time.Sleep(time.Second)
	cancel()
	cancelResult := <-cancelled
	if cancelResult.err == nil || !cancelResult.evidence.Cancelled {
		t.Fatalf("cancellation result = %#v, %v", cancelResult.evidence, cancelResult.err)
	}
	assertNoManagedContainers(t, runtime)

	orphanRequest := request
	orphanRequest.SandboxID = "sandbox-orphan"
	orphanRequest.Command = []string{"/bin/sh", "-c", "sleep 30"}
	orphanSpec, err := Validate(orphanRequest, policy)
	if err != nil {
		t.Fatal(err)
	}
	orphan, err := runtime.Create(context.Background(), orphanSpec)
	if err != nil {
		t.Fatal(err)
	}
	start := runtime.command(context.Background(), 10*time.Second, "start", orphan.ID)
	if err := dockerCommandError("start orphan fixture", start); err != nil {
		t.Fatal(err)
	}
	removed, err := runtime.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(removed, orphan.ID) {
		t.Fatalf("reconciled containers = %v, want %s", removed, orphan.ID)
	}
	if _, err := runtime.Inspect(context.Background(), orphan); !errors.Is(err, ErrSandboxNotFound) {
		t.Fatalf("orphan inspect error = %v", err)
	}
}

func integrationImage(t *testing.T, runtime *DockerRuntime) Image {
	t.Helper()
	configured := os.Getenv("REVOLVR_SANDBOX_TEST_IMAGE")
	if configured == "" {
		configured = "alpine:3.22"
	}
	result := runtime.command(context.Background(), 10*time.Second, "image", "inspect", "--format", "{{index .RepoDigests 0}}", configured)
	if err := dockerCommandError("inspect integration image", result); err != nil {
		t.Fatalf("%v; preload a pinned alpine:3.22 image in the rootless daemon", err)
	}
	pinned := strings.TrimSpace(result.Stdout)
	reference, digest, found := strings.Cut(pinned, "@")
	if !found || !validDigest(digest) {
		t.Fatalf("integration image identity = %q", pinned)
	}
	return Image{Reference: reference, Digest: digest}
}

func assertNoManagedContainers(t *testing.T, runtime *DockerRuntime) {
	t.Helper()
	result := runtime.command(context.Background(), 10*time.Second, "ps", "--all", "--quiet",
		"--filter", "label="+dockerManagedLabel+"=true", "--filter", "label="+dockerOwnerLabel+"="+runtime.Owner)
	if err := dockerCommandError("inspect leaked containers", result); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(result.Stdout) != "" {
		t.Fatalf("managed containers leaked: %s", result.Stdout)
	}
}
