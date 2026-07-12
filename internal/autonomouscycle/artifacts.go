package autonomouscycle

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomouspolicy"
	"revolvr/internal/autonomoussafety"
	"revolvr/internal/codexexec"
	"revolvr/internal/pathguard"
)

const workerProvenanceSchemaVersion = "revolvr-autonomous-worker-provenance-v1"

type workerPaths struct {
	dossier         string
	dossierManifest string
	prompt          string
	provenance      string
	outputSchema    string
	output          string
	sourceEvidence  string
	codexStdout     string
	codexStderr     string
	receipt         string
	verification    string
}

type workerProvenance struct {
	SchemaVersion           string                            `json:"schema_version"`
	RunID                   string                            `json:"run_id"`
	TaskID                  string                            `json:"task_id"`
	Dossier                 autonomous.TaskDossierManifest    `json:"dossier_manifest"`
	Decision                autonomous.SupervisorDecision     `json:"decision"`
	Reference               autonomous.DecisionReference      `json:"decision_reference"`
	Route                   autonomouspolicy.Route            `json:"route"`
	Profile                 ProfileEvidence                   `json:"profile"`
	Invocation              codexexec.InvocationProvenance    `json:"invocation"`
	Artifacts               WorkerArtifacts                   `json:"artifacts"`
	PromptByteSize          int                               `json:"prompt_byte_size"`
	PromptTokenEstimator    string                            `json:"prompt_token_estimator"`
	PromptTokenEstimate     int                               `json:"prompt_token_estimate"`
	AdmissionSourceRevision string                            `json:"admission_source_revision"`
	SafetyPolicy            *autonomoussafety.Policy          `json:"safety_policy,omitempty"`
	SafetyPreflight         *autonomoussafety.PreflightResult `json:"safety_preflight,omitempty"`
}

type optionalFile struct {
	Exists   bool
	SHA256   string
	ByteSize int
}

func prepareWorkerPaths(root, runID string, action autonomous.Action) (workerPaths, error) {
	base := filepath.Join(".revolvr", "runs", runID)
	paths := workerPaths{
		dossier:         filepath.Join(base, "worker-dossier.md"),
		dossierManifest: filepath.Join(base, "worker-dossier-manifest.json"),
		prompt:          filepath.Join(base, "worker-prompt.md"),
		provenance:      filepath.Join(base, "worker-provenance.json"),
		output:          filepath.Join(base, "worker-output.txt"),
		sourceEvidence:  filepath.Join(base, "worker-source.json"),
		codexStdout:     filepath.Join(base, "codex.jsonl"),
		codexStderr:     filepath.Join(base, "codex.stderr"),
		receipt:         filepath.Join(".revolvr", "receipts", runID+".md"),
		verification:    filepath.Join(base, "verification.json"),
	}
	if action == autonomous.ActionPlan {
		paths.outputSchema = filepath.Join(base, "planner-output-schema.json")
		paths.output = filepath.Join(base, "planner-output.raw.json")
	} else if action == autonomous.ActionAudit {
		paths.outputSchema = filepath.Join(base, "auditor-output-schema.json")
		paths.output = filepath.Join(base, "auditor-output.raw.json")
	} else if action == autonomous.ActionCorrect {
		paths.outputSchema = filepath.Join(base, "corrector-output-schema.json")
		paths.output = filepath.Join(base, "corrector-output.raw.json")
	}
	for _, path := range []string{paths.dossier, paths.dossierManifest, paths.prompt, paths.provenance, paths.outputSchema, paths.output, paths.sourceEvidence, paths.codexStdout, paths.codexStderr, paths.receipt, paths.verification} {
		if path == "" {
			continue
		}
		if _, err := pathguard.Resolve(root, path); err != nil {
			return workerPaths{}, fmt.Errorf("prepare worker artifact path %q: %w", path, err)
		}
	}
	return paths, nil
}

func workerArtifactsWithPaths(paths workerPaths) WorkerArtifacts {
	artifacts := WorkerArtifacts{
		Dossier:         Artifact{Path: paths.dossier},
		DossierManifest: Artifact{Path: paths.dossierManifest},
		Prompt:          Artifact{Path: paths.prompt},
		Provenance:      Artifact{Path: paths.provenance},
		Output:          Artifact{Path: paths.output},
		SourceEvidence:  Artifact{Path: paths.sourceEvidence},
		CodexStdout:     Artifact{Path: paths.codexStdout},
		CodexStderr:     Artifact{Path: paths.codexStderr},
		Receipt:         Artifact{Path: paths.receipt},
	}
	if paths.outputSchema != "" {
		artifact := Artifact{Path: paths.outputSchema}
		artifacts.OutputSchema = &artifact
	}
	return artifacts
}

func writeArtifact(root, relPath string, content []byte) (Artifact, error) {
	absPath, err := pathguard.Resolve(root, relPath)
	if err != nil {
		return Artifact{Path: relPath}, err
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return Artifact{Path: relPath}, fmt.Errorf("create worker artifact directory: %w", err)
	}
	if err := os.WriteFile(absPath, content, 0o644); err != nil {
		return Artifact{Path: relPath}, fmt.Errorf("write worker artifact %s: %w", relPath, err)
	}
	return artifactForBytes(relPath, content), nil
}

func referenceArtifact(root, relPath string) (Artifact, error) {
	absPath, err := pathguard.Resolve(root, relPath)
	if err != nil {
		return Artifact{Path: relPath}, err
	}
	raw, err := os.ReadFile(absPath)
	if err != nil {
		return Artifact{Path: relPath}, err
	}
	return artifactForBytes(relPath, raw), nil
}

func artifactForBytes(path string, content []byte) Artifact {
	return Artifact{Path: path, SHA256: sha256Hex(content), ByteSize: len(content)}
}

func preserveWorkerOutput(root, relPath string, worker *WorkerEvidence) error {
	artifact, err := referenceArtifact(root, relPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) || strings.TrimSpace(worker.Codex.FinalMessage) == "" {
			return fmt.Errorf("preserve exact worker output: %w", err)
		}
		raw := []byte(worker.Codex.FinalMessage)
		artifact, err = writeArtifact(root, relPath, raw)
		if err != nil {
			return err
		}
	}
	absPath, err := pathguard.Resolve(root, relPath)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(absPath)
	if err != nil {
		return err
	}
	worker.RawOutput = append([]byte(nil), raw...)
	worker.Artifacts.Output = artifact
	return nil
}

func writeWorkerSourceEvidence(root, path string, source SourceEvidence) (Artifact, error) {
	raw, err := marshalIndented(source)
	if err != nil {
		return Artifact{Path: path}, err
	}
	return writeArtifact(root, path, raw)
}

func captureOptionalFile(root, relPath string) (optionalFile, error) {
	absPath, err := pathguard.Resolve(root, relPath)
	if err != nil {
		return optionalFile{}, err
	}
	raw, err := os.ReadFile(absPath)
	if errors.Is(err, os.ErrNotExist) {
		return optionalFile{}, nil
	}
	if err != nil {
		return optionalFile{}, fmt.Errorf("read immutable evidence path %s: %w", relPath, err)
	}
	return optionalFile{Exists: true, SHA256: sha256Hex(raw), ByteSize: len(raw)}, nil
}

func ensureOptionalFileUnchanged(root, relPath string, before optionalFile) error {
	after, err := captureOptionalFile(root, relPath)
	if err != nil {
		return err
	}
	if before != after {
		return fmt.Errorf("autonomous execution state path %q changed", relPath)
	}
	return nil
}

func marshalIndented(value any) ([]byte, error) {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func sha256Hex(content []byte) string {
	sum := sha256.Sum256(content)
	return fmt.Sprintf("%x", sum)
}

func equalBytes(left, right []byte) bool { return bytes.Equal(left, right) }

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
