package prompt

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type ContextManifestInput struct {
	Input              Input
	ContextPayload     []byte
	ContextPayloadPath string
	GeneratedAt        time.Time
}

type ContextManifest struct {
	RunID                  string          `json:"run_id"`
	TaskID                 string          `json:"task_id"`
	ProfileName            string          `json:"profile_name"`
	ContextPayloadPath     string          `json:"context_payload_path"`
	ContextPayloadSHA256   string          `json:"context_payload_sha256"`
	ContextPayloadByteSize int             `json:"context_payload_byte_size"`
	GeneratedAt            time.Time       `json:"generated_at"`
	Sources                []ContextSource `json:"sources"`
}

type ContextSource struct {
	Label    string `json:"label"`
	Path     string `json:"path,omitempty"`
	SHA256   string `json:"sha256"`
	ByteSize int    `json:"byte_size"`
}

func BuildContextManifest(in ContextManifestInput) (ContextManifest, error) {
	normalized, err := normalize(in.Input)
	if err != nil {
		return ContextManifest{}, err
	}

	payloadPath := strings.TrimSpace(in.ContextPayloadPath)
	if payloadPath == "" {
		return ContextManifest{}, errors.New("build context manifest: context payload path is required")
	}
	runProfileSourcePath := strings.TrimSpace(normalized.RunProfile.SourcePath)
	if runProfileSourcePath == "" {
		return ContextManifest{}, errors.New("build context manifest: run profile source path is required")
	}

	generatedAt := in.GeneratedAt
	if generatedAt.IsZero() {
		generatedAt = time.Now()
	}
	generatedAt = generatedAt.UTC()

	selectedTaskSource, err := selectedTaskContextSource(normalized)
	if err != nil {
		return ContextManifest{}, err
	}

	return ContextManifest{
		RunID:                  normalized.RunID,
		TaskID:                 normalized.TaskID,
		ProfileName:            normalized.RunProfile.Name,
		ContextPayloadPath:     payloadPath,
		ContextPayloadSHA256:   sha256Hex(in.ContextPayload),
		ContextPayloadByteSize: len(in.ContextPayload),
		GeneratedAt:            generatedAt,
		Sources: []ContextSource{
			selectedTaskSource,
			contextSource("run_profile", runProfileSourcePath, []byte(normalized.RunProfile.Description)),
		},
	}, nil
}

func MarshalContextManifest(manifest ContextManifest) ([]byte, error) {
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal context manifest: %w", err)
	}
	return append(raw, '\n'), nil
}

func contextSource(label string, path string, content []byte) ContextSource {
	return ContextSource{
		Label:    label,
		Path:     strings.TrimSpace(path),
		SHA256:   sha256Hex(content),
		ByteSize: len(content),
	}
}

func selectedTaskContextSource(in Input) (ContextSource, error) {
	if in.TaskSource.Path == "" {
		return contextSource("selected_task", "", []byte(in.Task)), nil
	}
	if in.TaskSource.Content == nil {
		return ContextSource{}, errors.New("build context manifest: selected task source content is required when source path is provided")
	}
	return contextSource("selected_task", in.TaskSource.Path, in.TaskSource.Content), nil
}

func sha256Hex(content []byte) string {
	sum := sha256.Sum256(content)
	return fmt.Sprintf("%x", sum)
}
