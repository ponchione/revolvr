package sandbox

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"revolvr/internal/runner"
)

func TestDockerRuntimeTranslatesOnlyHardenedArguments(t *testing.T) {
	specification := validatedSandboxSpecification(t)
	specification.RuntimeProfile = ProfileStrict
	var commands [][]string
	runtime := fakeDockerRuntime(func(command runner.Command) runner.Result {
		commands = append(commands, append([]string(nil), command.Args...))
		switch command.Args[0] {
		case "info":
			return runner.Result{ExitCode: 0, Stdout: "[\"name=rootless\"]\n{\"runc\":{},\"runsc\":{}}\n"}
		case "inspect":
			return runner.Result{ExitCode: 1, Stderr: "error: no such object"}
		case "create":
			return runner.Result{ExitCode: 0, Stdout: strings.Repeat("a", 64) + "\n"}
		default:
			t.Fatalf("unexpected Docker command %v", command.Args)
			return runner.Result{}
		}
	})
	handle, err := runtime.Create(context.Background(), specification)
	if err != nil {
		t.Fatal(err)
	}
	if handle.ID != strings.Repeat("a", 64) {
		t.Fatalf("handle = %#v", handle)
	}
	create := commands[len(commands)-1]
	for _, required := range []string{
		"--pull=never", "--user", "65532:65532", "--cap-drop=ALL",
		"--security-opt=no-new-privileges=true", "--read-only", "--network=none",
		"--runtime=runsc", "--pids-limit=1024", "--cpus=8", "--memory=25769803776",
	} {
		if !containsArgument(create, required) {
			t.Errorf("create arguments lack %q: %v", required, create)
		}
	}
	for _, prohibited := range []string{"--privileged", "--network=host", "--pid=host", "--ipc=host", "--device", "/var/run/docker.sock"} {
		if containsArgument(create, prohibited) {
			t.Errorf("create arguments contain %q: %v", prohibited, create)
		}
	}
	for _, mount := range specification.Mounts {
		if !containsSubstring(create, "src="+mount.SourcePath) || !containsSubstring(create, "dst="+mount.Target) {
			t.Errorf("create arguments lack mount %#v: %v", mount, create)
		}
	}
	if got := create[len(create)-len(specification.Command)-1:]; !reflect.DeepEqual(got, append([]string{specification.Image.Reference + "@" + specification.Image.Digest}, specification.Command...)) {
		t.Fatalf("image and command = %v", got)
	}
}

func TestDockerRuntimeRejectsRootfulAndUnavailableStrictProfiles(t *testing.T) {
	tests := []struct {
		name string
		info string
		want error
	}{
		{"rootful", "[\"name=seccomp\"]\n{\"runc\":{}}\n", ErrRuntimeUnavailable},
		{"strict without runsc", "[\"name=rootless\"]\n{\"runc\":{}}\n", ErrProfileUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := fakeDockerRuntime(func(command runner.Command) runner.Result {
				if command.Args[0] != "info" {
					t.Fatalf("unexpected Docker command %v", command.Args)
				}
				return runner.Result{ExitCode: 0, Stdout: test.info}
			})
			specification := validatedSandboxSpecification(t)
			specification.RuntimeProfile = ProfileStrict
			_, err := runtime.Create(context.Background(), specification)
			if !errors.Is(err, test.want) {
				t.Fatalf("Create error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestDockerReconcileFiltersByExactOwnerAndRemovesExactContainers(t *testing.T) {
	first := strings.Repeat("a", 64)
	second := strings.Repeat("b", 64)
	var commands [][]string
	runtime := fakeDockerRuntime(func(command runner.Command) runner.Result {
		commands = append(commands, append([]string(nil), command.Args...))
		switch command.Args[0] {
		case "info":
			return runner.Result{ExitCode: 0, Stdout: "[\"name=rootless\"]\n{\"runc\":{}}\n"}
		case "ps":
			return runner.Result{ExitCode: 0, Stdout: first + "\n" + second + "\n"}
		case "stop", "rm":
			return runner.Result{ExitCode: 0}
		default:
			t.Fatalf("unexpected Docker command %v", command.Args)
			return runner.Result{}
		}
	})
	removed, err := runtime.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(removed, []string{first, second}) {
		t.Fatalf("removed = %v", removed)
	}
	list := commands[1]
	if !containsArgument(list, "--no-trunc") || !containsArgument(list, "label="+dockerManagedLabel+"=true") || !containsArgument(list, "label="+dockerOwnerLabel+"="+runtime.Owner) || containsSubstring(list, "name=revolvr") {
		t.Fatalf("reconcile filters = %v", list)
	}
	for _, id := range removed {
		if !commandTargets(commands, "stop", id) || !commandTargets(commands, "rm", id) {
			t.Fatalf("container %s was not stopped and removed exactly: %v", id, commands)
		}
	}
}

func fakeDockerRuntime(run func(runner.Command) runner.Result) *DockerRuntime {
	return &DockerRuntime{
		Executable: "/bin/docker", Host: "unix:///run/user/1000/docker.sock", Owner: strings.Repeat("c", 64),
		run: func(_ context.Context, command runner.Command) runner.Result { return run(command) },
	}
}

func containsArgument(arguments []string, want string) bool {
	return slices.Contains(arguments, want)
}

func containsSubstring(arguments []string, want string) bool {
	for _, argument := range arguments {
		if strings.Contains(argument, want) {
			return true
		}
	}
	return false
}

func commandTargets(commands [][]string, operation, id string) bool {
	for _, command := range commands {
		if len(command) > 1 && command[0] == operation && command[len(command)-1] == id {
			return true
		}
	}
	return false
}
