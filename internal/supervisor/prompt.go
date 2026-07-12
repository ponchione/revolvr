package supervisor

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"revolvr/internal/autonomous"
	"revolvr/internal/prompt"
)

const SupervisorProfileName = "supervisor"

type PromptInput struct {
	TaskID  string
	Dossier autonomous.TaskDossier
	Profile prompt.RunProfile
}

func BuildPrompt(in PromptInput) ([]byte, error) {
	if err := validateTaskID(in.TaskID); err != nil {
		return nil, err
	}
	if err := ValidateDossier(in.TaskID, in.Dossier); err != nil {
		return nil, err
	}
	if in.Profile.Name != SupervisorProfileName {
		return nil, fmt.Errorf("build supervisor prompt: profile name must be %q (got %q)", SupervisorProfileName, in.Profile.Name)
	}
	if strings.TrimSpace(in.Profile.SourcePath) == "" {
		return nil, errors.New("build supervisor prompt: profile source path is required")
	}
	if strings.TrimSpace(in.Profile.Description) == "" {
		return nil, errors.New("build supervisor prompt: profile content is required")
	}

	var out bytes.Buffer
	out.WriteString("# Revolvr Fresh Supervisor Decision Pass\n\n")
	out.WriteString("This is one fresh, ephemeral, decision-only supervisor session. Use only the exact profile and validated task dossier below. Return exactly one SupervisorDecision that conforms to the harness-supplied JSON schema.\n\n")
	out.WriteString("Task identity: ")
	out.WriteString(in.TaskID)
	out.WriteString("\nDossier schema: ")
	out.WriteString(in.Dossier.Manifest.SchemaVersion)
	out.WriteString("\nDossier SHA-256: ")
	out.WriteString(in.Dossier.Manifest.DossierSHA256)
	out.WriteString("\nDossier byte size: ")
	out.WriteString(fmt.Sprintf("%d", in.Dossier.Manifest.DossierByteSize))
	out.WriteString("\n\n## Exact Supervisor Profile\n\n")
	out.WriteString(in.Profile.Description)
	out.WriteString("\n\n## Exact Validated Task Dossier\n\n")
	out.Write(in.Dossier.Markdown)
	if len(in.Dossier.Markdown) == 0 || in.Dossier.Markdown[len(in.Dossier.Markdown)-1] != '\n' {
		out.WriteByte('\n')
	}
	out.WriteString("\n## Harness Authority and Output Rules\n\n")
	out.WriteString("Return exactly one JSON object representing one SupervisorDecision. Follow the harness-supplied JSON schema directly. For a worker action, include explicit structured strategy material whose approach, techniques, and evidence targets materially describe this retry; formatting, run IDs, timestamps, or rationale-only changes are not a changed strategy. When an operator product decision is indispensable, use only the needs_input action and include the exact versioned question, mutually exclusive stable option IDs, one offered recommendation and rationale, typed evidence, deterministic content identity, and only genuinely option-independent read-only work. Never select the recommendation automatically. Do not include surrounding prose, Markdown fences, or multiple objects. Do not invoke Codex recursively or start a nested Codex session. Do not execute or route a worker. Do not edit repository source, task files, plans, findings, receipts, runtime state, or other evidence, and do not create a commit.\n\n")
	out.WriteString("Revolvr retains safety, verification, legal-transition, retry, commit, and terminal-state authority. A structurally valid complete or block recommendation is only a supervisor output; this pass does not transition or finalize the task.\n")
	return out.Bytes(), nil
}

func ValidateDossier(taskID string, dossier autonomous.TaskDossier) error {
	if err := validateTaskID(taskID); err != nil {
		return err
	}
	manifest := dossier.Manifest
	if manifest.SchemaVersion != autonomous.DossierManifestSchemaVersion && manifest.SchemaVersion != autonomous.RoleDossierManifestSchemaVersion {
		return fmt.Errorf("validate supervisor dossier: unsupported manifest schema %q", manifest.SchemaVersion)
	}
	if manifest.TaskID != taskID {
		return fmt.Errorf("validate supervisor dossier: manifest task_id %q does not match requested task_id %q", manifest.TaskID, taskID)
	}
	if manifest.SchemaVersion == autonomous.RoleDossierManifestSchemaVersion {
		if manifest.Projection == nil || manifest.Projection.Role != autonomous.DossierRoleSupervisor || manifest.TokenEstimate == nil {
			return errors.New("validate supervisor dossier: role projection must be supervisor with token estimates")
		}
	}
	if len(dossier.Markdown) == 0 {
		return errors.New("validate supervisor dossier: Markdown is empty")
	}
	if !utf8.Valid(dossier.Markdown) {
		return errors.New("validate supervisor dossier: Markdown is not valid UTF-8")
	}
	if manifest.DossierByteSize != len(dossier.Markdown) {
		return fmt.Errorf("validate supervisor dossier: Markdown byte size %d does not match manifest byte size %d", len(dossier.Markdown), manifest.DossierByteSize)
	}
	actual := sha256.Sum256(dossier.Markdown)
	actualHex := fmt.Sprintf("%x", actual)
	if manifest.DossierSHA256 != actualHex {
		return fmt.Errorf("validate supervisor dossier: Markdown SHA-256 %s does not match manifest SHA-256 %s", actualHex, manifest.DossierSHA256)
	}
	for i, source := range manifest.Sources {
		if !supportedDossierSourceKind(source.Kind) || strings.TrimSpace(source.ID) == "" {
			return fmt.Errorf("validate supervisor dossier: manifest sources[%d] requires kind and id", i)
		}
		decoded, err := hex.DecodeString(source.SHA256)
		if err != nil || len(decoded) != sha256.Size {
			return fmt.Errorf("validate supervisor dossier: manifest sources[%d] has invalid SHA-256", i)
		}
		if source.ByteSize < 0 {
			return fmt.Errorf("validate supervisor dossier: manifest sources[%d] has negative byte size", i)
		}
	}
	return nil
}

func supportedDossierSourceKind(kind autonomous.DossierSourceKind) bool {
	switch kind {
	case autonomous.DossierSourceKindTaskSpec,
		autonomous.DossierSourceKindExecutionState,
		autonomous.DossierSourceKindVerification,
		autonomous.DossierSourceKindAudit,
		autonomous.DossierSourceKindRecentRuns,
		autonomous.DossierSourceKindReceipt,
		autonomous.DossierSourceKindGitSnapshot,
		autonomous.DossierSourceKindRepositoryGuidance,
		autonomous.DossierSourceKindRepositoryMap:
		return true
	default:
		return false
	}
}

func validateTaskID(taskID string) error {
	if strings.TrimSpace(taskID) == "" {
		return errors.New("supervisor task_id is required")
	}
	if taskID != strings.TrimSpace(taskID) {
		return errors.New("supervisor task_id must be normalized without surrounding whitespace")
	}
	if strings.IndexFunc(taskID, unicode.IsControl) >= 0 {
		return errors.New("supervisor task_id must not contain control characters")
	}
	return nil
}
