package sandbox

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestValidRequestNormalizesDeterministically(t *testing.T) {
	policy, request := sandboxFixture(t)

	first, err := Validate(request, policy)
	if err != nil {
		t.Fatalf("Validate first: %v", err)
	}
	firstHash, err := first.SHA256()
	if err != nil {
		t.Fatalf("SHA256 first: %v", err)
	}

	secondRequest := cloneRequest(request)
	secondRequest.Network = ""
	secondRequest.Mounts[0], secondRequest.Mounts[2] = secondRequest.Mounts[2], secondRequest.Mounts[0]
	second, err := Validate(secondRequest, policy)
	if err != nil {
		t.Fatalf("Validate second: %v", err)
	}
	secondHash, err := second.SHA256()
	if err != nil {
		t.Fatalf("SHA256 second: %v", err)
	}
	if firstHash != secondHash || !reflect.DeepEqual(first, second) {
		t.Fatalf("equivalent requests differ:\nfirst  = %#v (%s)\nsecond = %#v (%s)", first, firstHash, second, secondHash)
	}
	if first.Network != NetworkNone || first.Mounts[0].Target != "/cache/go" || first.Mounts[1].Target != "/context" || first.Mounts[2].Target != "/workspace" {
		t.Fatalf("normalization = %#v", first)
	}
	if first.Environment[0].Name != "ROLE" || first.Environment[1].Name != "RUN_ID" || first.Environment[2].Name != "TASK_ID" {
		t.Fatalf("environment order = %#v", first.Environment)
	}
	for _, mount := range first.Mounts {
		if mount.SourceDevice == 0 || mount.SourceInode == 0 || !filepath.IsAbs(mount.SourcePath) {
			t.Fatalf("mount lacks resolved identity: %#v", mount)
		}
	}

	request.Command[0] = "changed"
	request.Environment["ROLE"] = "changed"
	request.Mounts[0].Target = "/changed"
	if first.Command[0] != "/usr/local/bin/revolvr-worker" || first.Environment[0].Value != "implementer" || first.Mounts[1].Target != "/context" {
		t.Fatalf("specification retained mutable request storage: %#v", first)
	}
}

func TestValidAttendedDiagnosticAndDependencyProfiles(t *testing.T) {
	policy, request := sandboxFixture(t)
	policy.Attended = true
	policy.AllowedProfiles = []RuntimeProfile{ProfileStrict, ProfileCompatible, ProfileDiagnostic}
	policy.AllowedNetworks = []NetworkProfile{NetworkDependencies, NetworkOpen}
	request.RuntimeProfile = ProfileDiagnostic
	request.Network = NetworkOpen
	if _, err := Validate(request, policy); err != nil {
		t.Fatalf("attended diagnostic request: %v", err)
	}

	request.RuntimeProfile = ProfileCompatible
	request.Network = NetworkDependencies
	if _, err := Validate(request, policy); err != nil {
		t.Fatalf("compatible dependency request: %v", err)
	}
}

func TestValidJSONMatchesTypedValidation(t *testing.T) {
	policy, request := sandboxFixture(t)
	want, err := Validate(request, policy)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(raw, policy)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded specification = %#v, want %#v", got, want)
	}
}

func TestRejectMalformedDuplicateAndUnsafeRuntimeFields(t *testing.T) {
	policy, request := sandboxFixture(t)
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	unknownFields := []string{"privileged", "host_pid", "host_ipc", "host_network", "cap_add", "devices", "runtime_socket", "host_home", "raw_oci_flags"}
	for _, field := range unknownFields {
		t.Run(field, func(t *testing.T) {
			candidate := append(append([]byte(nil), raw[:len(raw)-1]...), []byte(`,"`+field+`":true}`)...)
			_, err := Decode(candidate, policy)
			assertRejected(t, err, "unknown field")
		})
	}

	duplicate := strings.Replace(string(raw), `"schema_version":"`+RequestSchemaVersion+`"`, `"schema_version":"`+RequestSchemaVersion+`","schema_version":"`+RequestSchemaVersion+`"`, 1)
	_, err = Decode([]byte(duplicate), policy)
	assertRejected(t, err, "duplicate field")
	nestedDuplicate := strings.Replace(string(raw), `"reference":"revolvr/go-worker"`, `"reference":"revolvr/go-worker","reference":"attacker/image"`, 1)
	_, err = Decode([]byte(nestedDuplicate), policy)
	assertRejected(t, err, "duplicate field")
	_, err = Decode([]byte(`{"schema_version":`), policy)
	assertRejected(t, err, "decode JSON")
	_, err = Decode(append(raw, []byte(` {}`)...), policy)
	assertRejected(t, err, "multiple JSON values")
	_, err = Decode(make([]byte, maxRequestBytes+1), policy)
	assertRejected(t, err, "exceeds")
}

func TestRejectInvalidRequestAuthorityAndBounds(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Policy, *Request)
		want   string
	}{
		{"version", func(_ *Policy, r *Request) { r.SchemaVersion = "v2" }, "schema_version"},
		{"empty sandbox", func(_ *Policy, r *Request) { r.SandboxID = "" }, "sandbox_id"},
		{"project drift", func(_ *Policy, r *Request) { r.ProjectID = "other" }, "project_id"},
		{"task drift", func(_ *Policy, r *Request) { r.TaskID = "other" }, "task_id"},
		{"run drift", func(_ *Policy, r *Request) { r.RunID = "other" }, "run_id"},
		{"unsafe role", func(_ *Policy, r *Request) { r.Role = "auditor" }, "role"},
		{"mutable image only", func(_ *Policy, r *Request) { r.Image.Digest = "" }, "not approved"},
		{"unknown image", func(_ *Policy, r *Request) { r.Image.Reference = "attacker/image:latest" }, "not approved"},
		{"unknown profile", func(_ *Policy, r *Request) { r.RuntimeProfile = "privileged" }, "runtime_profile"},
		{"diagnostic unattended", func(p *Policy, r *Request) {
			p.AllowedProfiles = append(p.AllowedProfiles, ProfileDiagnostic)
			r.RuntimeProfile = ProfileDiagnostic
		}, "requires attended"},
		{"network escalation", func(p *Policy, r *Request) {
			p.AllowedNetworks = []NetworkProfile{NetworkOpen}
			r.Network = NetworkOpen
		}, "open network"},
		{"empty command", func(_ *Policy, r *Request) { r.Command = nil }, "command"},
		{"empty argument", func(_ *Policy, r *Request) { r.Command[0] = "" }, "command[0]"},
		{"oversized argument", func(_ *Policy, r *Request) { r.Command[0] = strings.Repeat("x", maxArgumentBytes+1) }, "oversized"},
		{"zero resource", func(_ *Policy, r *Request) { r.Resources.PIDs = 0 }, "must be positive"},
		{"resource over policy", func(p *Policy, r *Request) { r.Resources.MemoryBytes = p.MaximumResources.MemoryBytes + 1 }, "exceeds policy"},
		{"unknown environment", func(_ *Policy, r *Request) { r.Environment["UNLISTED"] = "value" }, "not allowed"},
		{"secret environment", func(p *Policy, r *Request) {
			p.AllowedEnvironmentNames = append(p.AllowedEnvironmentNames, "OPENAI_API_KEY")
			r.Environment["OPENAI_API_KEY"] = "secret"
		}, "unsafe"},
		{"ssh agent", func(p *Policy, r *Request) {
			p.AllowedEnvironmentNames = append(p.AllowedEnvironmentNames, "SSH_AUTH_SOCK")
			r.Environment["SSH_AUTH_SOCK"] = "/agent.sock"
		}, "unsafe"},
		{"database credential", func(p *Policy, r *Request) {
			p.AllowedEnvironmentNames = append(p.AllowedEnvironmentNames, "PGPASSWORD")
			r.Environment["PGPASSWORD"] = "secret"
		}, "unsafe"},
		{"empty environment value", func(_ *Policy, r *Request) { r.Environment["ROLE"] = "" }, "empty"},
		{"identity environment drift", func(_ *Policy, r *Request) { r.Environment["RUN_ID"] = "other" }, "does not match"},
		{"unknown mount", func(_ *Policy, r *Request) { r.Mounts[0].SourceID = "context:unknown" }, "not managed"},
		{"mount target", func(_ *Policy, r *Request) { r.Mounts[0].Target = "/etc" }, "target"},
		{"workspace read only", func(_ *Policy, r *Request) { r.Mounts[1].Mode = MountReadOnly }, "workspace"},
		{"context writable", func(_ *Policy, r *Request) { r.Mounts[0].Mode = MountReadWrite }, "read-only"},
		{"missing workspace", func(_ *Policy, r *Request) { r.Mounts = r.Mounts[:1] }, "one writable workspace"},
		{"duplicate source", func(_ *Policy, r *Request) { r.Mounts = append(r.Mounts, r.Mounts[0]) }, "duplicates source_id"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy, request := sandboxFixture(t)
			test.change(&policy, &request)
			_, err := Validate(request, policy)
			assertRejected(t, err, test.want)
		})
	}
}

func TestRejectVerifierNetwork(t *testing.T) {
	policy, request := sandboxFixture(t)
	policy.Role = RoleVerifier
	request.Role = RoleVerifier
	request.Environment["ROLE"] = "verifier"
	policy.AllowedNetworks = []NetworkProfile{NetworkDependencies}
	request.Network = NetworkDependencies
	_, err := Validate(request, policy)
	assertRejected(t, err, "verifier network")
}

func TestRejectEveryInvalidResourceBound(t *testing.T) {
	tests := []struct {
		name string
		zero func(*Resources)
		over func(*Resources, Resources)
	}{
		{"cpus", func(r *Resources) { r.CPUs = 0 }, func(r *Resources, m Resources) { r.CPUs = m.CPUs + 1 }},
		{"memory", func(r *Resources) { r.MemoryBytes = 0 }, func(r *Resources, m Resources) { r.MemoryBytes = m.MemoryBytes + 1 }},
		{"pids", func(r *Resources) { r.PIDs = 0 }, func(r *Resources, m Resources) { r.PIDs = m.PIDs + 1 }},
		{"timeout", func(r *Resources) { r.TimeoutSeconds = 0 }, func(r *Resources, m Resources) { r.TimeoutSeconds = m.TimeoutSeconds + 1 }},
		{"tmpfs", func(r *Resources) { r.TmpfsBytes = 0 }, func(r *Resources, m Resources) { r.TmpfsBytes = m.TmpfsBytes + 1 }},
	}
	for _, test := range tests {
		t.Run(test.name+" nonpositive", func(t *testing.T) {
			policy, request := sandboxFixture(t)
			test.zero(&request.Resources)
			_, err := Validate(request, policy)
			assertRejected(t, err, "must be positive")
		})
		t.Run(test.name+" over policy", func(t *testing.T) {
			policy, request := sandboxFixture(t)
			test.over(&request.Resources, policy.MaximumResources)
			_, err := Validate(request, policy)
			assertRejected(t, err, "exceeds policy")
		})
	}
}

func TestPathValidationRejectsTraversalSymlinkHardLinkWrongTypeAndMode(t *testing.T) {
	tests := []struct {
		name   string
		change func(*testing.T, *Policy, *Request)
		want   string
	}{
		{"traversal", func(_ *testing.T, p *Policy, _ *Request) { p.ManagedSources[1].RelativePath = "../outside" }, "traversal"},
		{"symlink", func(t *testing.T, p *Policy, _ *Request) {
			workspace := filepath.Join(p.ManagedSources[1].Root, "workspace")
			if err := os.Remove(workspace); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(t.TempDir(), workspace); err != nil {
				t.Fatal(err)
			}
		}, "escapes root"},
		{"symlink ancestor", func(t *testing.T, p *Policy, _ *Request) {
			root := p.ManagedSources[1].Root
			if err := os.Mkdir(filepath.Join(root, "outside"), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(root, "outside"), filepath.Join(root, "redirect")); err != nil {
				t.Fatal(err)
			}
			p.ManagedSources[1].RelativePath = "redirect/workspace"
		}, "symlink"},
		{"hard link", func(t *testing.T, p *Policy, _ *Request) {
			contextPath := filepath.Join(p.ManagedSources[0].Root, "context.json")
			if err := os.Link(contextPath, filepath.Join(p.ManagedSources[0].Root, "context-link.json")); err != nil {
				t.Fatal(err)
			}
		}, "hard-link"},
		{"wrong directory type", func(t *testing.T, p *Policy, _ *Request) {
			workspace := filepath.Join(p.ManagedSources[1].Root, "workspace")
			if err := os.Remove(workspace); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(workspace, []byte("not a directory"), 0o640); err != nil {
				t.Fatal(err)
			}
		}, "not a directory"},
		{"wrong file type", func(_ *testing.T, p *Policy, _ *Request) {
			p.ManagedSources[0].Type = SourceDirectory
		}, "not a directory"},
		{"unsafe mode", func(t *testing.T, p *Policy, _ *Request) {
			if err := os.Chmod(filepath.Join(p.ManagedSources[1].Root, "workspace"), 0o770); err != nil {
				t.Fatal(err)
			}
		}, "unsafe directory mode"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy, request := sandboxFixture(t)
			test.change(t, &policy, &request)
			_, err := Validate(request, policy)
			assertRejected(t, err, test.want)
		})
	}
}

func TestSymlinkManagedRootIdentityIsCanonicalized(t *testing.T) {
	policy, request := sandboxFixture(t)
	realRoot := policy.ManagedSources[0].Root
	linkParent := t.TempDir()
	linkedRoot := filepath.Join(linkParent, "managed")
	if err := os.Symlink(realRoot, linkedRoot); err != nil {
		t.Fatal(err)
	}
	for i := range policy.ManagedSources {
		policy.ManagedSources[i].Root = linkedRoot
	}
	specification, err := Validate(request, policy)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	for _, mount := range specification.Mounts {
		if mount.ManagedRoot != realRoot {
			t.Fatalf("managed root = %q, want canonical %q", mount.ManagedRoot, realRoot)
		}
	}
}

func TestPathValidationRejectsMountOverlapAndForbiddenHostAccess(t *testing.T) {
	t.Run("source overlap", func(t *testing.T) {
		policy, request := sandboxFixture(t)
		root := policy.ManagedSources[0].Root
		policy.ManagedSources[0].Target = "/context/base.json"
		request.Mounts[0].Target = "/context/base.json"
		if err := os.Mkdir(filepath.Join(root, "workspace", "nested"), 0o750); err != nil {
			t.Fatal(err)
		}
		policy.ManagedSources = append(policy.ManagedSources, ManagedSource{
			ID: "context:nested", Root: root, RelativePath: "workspace/nested",
			Kind: SourceContext, Type: SourceDirectory, Target: "/context/nested",
		})
		request.Mounts = append(request.Mounts, Mount{SourceID: "context:nested", Target: "/context/nested", Mode: MountReadOnly})
		_, err := Validate(request, policy)
		assertRejected(t, err, "mount sources")
	})

	t.Run("target overlap", func(t *testing.T) {
		policy, request := sandboxFixture(t)
		root := policy.ManagedSources[0].Root
		if err := os.WriteFile(filepath.Join(root, "nested.json"), []byte("{}"), 0o640); err != nil {
			t.Fatal(err)
		}
		policy.ManagedSources = append(policy.ManagedSources, ManagedSource{
			ID: "context:nested", Root: root, RelativePath: "nested.json",
			Kind: SourceContext, Type: SourceFile, Target: "/context/nested.json",
		})
		request.Mounts = append(request.Mounts, Mount{SourceID: "context:nested", Target: "/context/nested.json", Mode: MountReadOnly})
		_, err := Validate(request, policy)
		assertRejected(t, err, "mount targets")
	})

	t.Run("host home", func(t *testing.T) {
		policy, request := sandboxFixture(t)
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatal(err)
		}
		policy.ManagedSources[0] = ManagedSource{
			ID: "context:run", Root: filepath.Dir(home), RelativePath: filepath.Base(home),
			Kind: SourceContext, Type: SourceDirectory, Target: "/context",
		}
		_, err = Validate(request, policy)
		assertRejected(t, err, "forbidden host path")
	})

	t.Run("runtime socket parent", func(t *testing.T) {
		policy, request := sandboxFixture(t)
		root := policy.ManagedSources[0].Root
		policy.ForbiddenHostPaths = []string{filepath.Join(root, "workspace", "engine.sock")}
		_, err := Validate(request, policy)
		assertRejected(t, err, "forbidden host path")
	})
}

func sandboxFixture(t *testing.T) (Policy, Request) {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, directory := range []string{"workspace", "cache"} {
		if err := os.Mkdir(filepath.Join(root, directory), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "context.json"), []byte("{}\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	image := Image{Reference: "revolvr/go-worker", Digest: "sha256:" + strings.Repeat("a", 64)}
	resources := Resources{CPUs: 8, MemoryBytes: 24 << 30, PIDs: 1024, TimeoutSeconds: 2700, TmpfsBytes: 4 << 30}
	policy := Policy{
		ProjectID: "project-1", TaskID: "task-1", RunID: "run-1", Role: RoleImplementer,
		ApprovedImages: []Image{image}, AllowedProfiles: []RuntimeProfile{ProfileStrict, ProfileCompatible},
		AllowedEnvironmentNames: []string{"TASK_ID", "RUN_ID", "ROLE"},
		ManagedSources: []ManagedSource{
			{ID: "context:run", Root: root, RelativePath: "context.json", Kind: SourceContext, Type: SourceFile, Target: "/context"},
			{ID: "workspace:run", Root: root, RelativePath: "workspace", Kind: SourceWorkspace, Type: SourceDirectory, Target: "/workspace"},
			{ID: "cache:go", Root: root, RelativePath: "cache", Kind: SourceCache, Type: SourceDirectory, Target: "/cache/go"},
		},
		MaximumResources: resources,
	}
	request := Request{
		SchemaVersion: RequestSchemaVersion, SandboxID: "sandbox-1",
		ProjectID: policy.ProjectID, TaskID: policy.TaskID, RunID: policy.RunID, Role: policy.Role,
		Image: image, RuntimeProfile: ProfileStrict,
		Command: []string{"/usr/local/bin/revolvr-worker", "--once"},
		Mounts: []Mount{
			{SourceID: "context:run", Target: "/context", Mode: MountReadOnly},
			{SourceID: "workspace:run", Target: "/workspace", Mode: MountReadWrite},
			{SourceID: "cache:go", Target: "/cache/go", Mode: MountReadOnly},
		},
		Network: NetworkNone, Resources: resources,
		Environment: map[string]string{"TASK_ID": policy.TaskID, "RUN_ID": policy.RunID, "ROLE": string(policy.Role)},
	}
	return policy, request
}

func cloneRequest(request Request) Request {
	request.Command = append([]string(nil), request.Command...)
	request.Mounts = append([]Mount(nil), request.Mounts...)
	request.Environment = make(map[string]string, len(request.Environment))
	for name, value := range map[string]string{"TASK_ID": request.TaskID, "RUN_ID": request.RunID, "ROLE": string(request.Role)} {
		request.Environment[name] = value
	}
	return request
}

func assertRejected(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !errors.Is(err, ErrInvalidRequest) || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want ErrInvalidRequest containing %q", err, want)
	}
}
