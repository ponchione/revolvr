package embedding

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestComposeEmbeddingServiceIsolation(t *testing.T) {
	base := readCompose(t, "compose.yaml")
	service := composeService(t, base, "embedding-service")

	if _, ok := service["ports"]; ok {
		t.Fatal("base embedding service publishes a host port")
	}
	if _, ok := service["secrets"]; ok {
		t.Fatal("embedding service receives Compose secrets")
	}
	if service["read_only"] != true || service["user"] != "65532:65532" || service["gpus"] != "all" {
		t.Fatalf("embedding service hardening/GPU = read_only=%v user=%v gpus=%v", service["read_only"], service["user"], service["gpus"])
	}
	networks := stringSlice(t, service["networks"])
	if len(networks) != 1 || networks[0] != "control" {
		t.Fatalf("embedding service networks = %v, want internal control only", networks)
	}
	declaredNetworks, ok := base["networks"].(map[string]any)
	if !ok {
		t.Fatalf("compose networks = %#v", base["networks"])
	}
	control, ok := declaredNetworks["control"].(map[string]any)
	if !ok || control["internal"] != true {
		t.Fatalf("control network = %#v, want internal", declaredNetworks["control"])
	}
	volumes, ok := service["volumes"].([]any)
	if !ok || len(volumes) != 1 {
		t.Fatalf("embedding service volumes = %#v, want one model mount", service["volumes"])
	}
	volume, ok := volumes[0].(map[string]any)
	if !ok || volume["type"] != "bind" || volume["target"] != "/models/embedding" || volume["read_only"] != true {
		t.Fatalf("embedding model volume = %#v", volumes[0])
	}
	if source, ok := volume["source"].(string); !ok || !strings.HasPrefix(source, "${REVOLVR_EMBEDDING_MODEL_PATH:-") {
		t.Fatalf("embedding model source = %#v, want dedicated configured model path", volume["source"])
	}
	for _, forbidden := range []string{"workspace", "project", "docker.sock", "podman.sock", "postgres", "openai"} {
		if strings.Contains(strings.ToLower(volume["target"].(string)), forbidden) {
			t.Fatalf("embedding volume target contains forbidden authority %q", forbidden)
		}
	}
	environment, ok := service["environment"].(map[string]any)
	if !ok {
		t.Fatalf("embedding environment = %#v", service["environment"])
	}
	for _, required := range []string{
		"REVOLVR_EMBEDDING_MODEL_NAME", "REVOLVR_EMBEDDING_MODEL_REVISION",
		"REVOLVR_EMBEDDING_DIMENSIONS", "REVOLVR_EMBEDDING_POOLING",
		"REVOLVR_EMBEDDING_NORMALIZATION", "REVOLVR_EMBEDDING_QUANTIZATION",
		"REVOLVR_EMBEDDING_ARTIFACT_SHA256",
	} {
		if _, ok := environment[required]; !ok {
			t.Fatalf("embedding environment missing %s", required)
		}
	}
	for name := range environment {
		lower := strings.ToLower(name)
		if strings.Contains(lower, "database") || strings.Contains(lower, "postgres") || strings.Contains(lower, "openai") || strings.Contains(lower, "secret") {
			t.Fatalf("embedding environment contains forbidden authority %q", name)
		}
	}

	development := readCompose(t, "compose.dev.yaml")
	devService := composeService(t, development, "embedding-service")
	ports := stringSlice(t, devService["ports"])
	if len(ports) != 1 || !strings.HasPrefix(ports[0], "127.0.0.1:") {
		t.Fatalf("development embedding ports = %v, want one loopback-only port", ports)
	}
	if _, ok := devService["networks"]; ok {
		t.Fatal("development override broadens embedding service networks")
	}
}

func readCompose(t *testing.T, name string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "compose", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(raw, &document); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return document
}

func composeService(t *testing.T, document map[string]any, name string) map[string]any {
	t.Helper()
	services, ok := document["services"].(map[string]any)
	if !ok {
		t.Fatalf("compose services = %#v", document["services"])
	}
	service, ok := services[name].(map[string]any)
	if !ok {
		t.Fatalf("compose service %s = %#v", name, services[name])
	}
	return service
}

func stringSlice(t *testing.T, value any) []string {
	t.Helper()
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("compose list = %#v", value)
	}
	result := make([]string, len(items))
	for i, item := range items {
		text, ok := item.(string)
		if !ok {
			t.Fatalf("compose list item = %#v", item)
		}
		result[i] = text
	}
	return result
}
