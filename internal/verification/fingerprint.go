package verification

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"revolvr/internal/sandbox"
)

var (
	hexRevision = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)
	hexSHA256   = regexp.MustCompile(`^[0-9a-f]{64}$`)
	identity    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	envName     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

func Pin(plan PinnedPlan) (PinnedPlan, error) {
	if plan.Plan.SchemaVersion == "" {
		plan.Plan.SchemaVersion = PlanSchemaVersion
	}
	if plan.VerifierProtocol == "" {
		plan.VerifierProtocol = VerifierProtocolVersion
	}
	if plan.VerifierImplementation == "" {
		plan.VerifierImplementation = VerifierImplementationVersion
	}
	if plan.ProjectEnvironment.SchemaVersion == "" {
		plan.ProjectEnvironment.SchemaVersion = DefaultProjectEnvironmentVersion
	}
	if plan.Plan.SchemaVersion != PlanSchemaVersion || plan.VerifierProtocol != VerifierProtocolVersion || plan.VerifierImplementation != VerifierImplementationVersion {
		return PinnedPlan{}, invalidPlan("verifier schema, protocol, or implementation version is not supported")
	}
	for name, value := range map[string]string{
		"project_id": plan.ProjectID, "task_id": plan.TaskID, "task_version_id": plan.TaskVersionID,
		"run_id": plan.RunID, "workspace_id": plan.WorkspaceID,
	} {
		if _, err := uuid.Parse(value); err != nil {
			return PinnedPlan{}, invalidPlan("%s is not a UUID", name)
		}
	}
	if err := validateSource(plan.Candidate); err != nil {
		return PinnedPlan{}, err
	}
	contract, err := canonicalJSON(plan.ProjectEnvironment.Contract)
	if err != nil || len(contract) == 0 || len(contract) > 1<<20 {
		return PinnedPlan{}, invalidPlan("project environment contract is invalid")
	}
	plan.ProjectEnvironment.Contract = contract
	contractSHA := hashBytes(contract)
	if plan.ProjectEnvironment.SHA256 != "" && plan.ProjectEnvironment.SHA256 != contractSHA {
		return PinnedPlan{}, invalidPlan("project environment hash does not match contract")
	}
	plan.ProjectEnvironment.SHA256 = contractSHA
	if strings.TrimSpace(plan.Plan.Version) == "" || len(plan.Plan.Version) > 256 || strings.TrimSpace(plan.Plan.VerificationPlanVersion) == "" || len(plan.Plan.VerificationPlanVersion) > 256 || !hexSHA256.MatchString(plan.Plan.VerificationPlanSHA256) {
		return PinnedPlan{}, invalidPlan("verification plan version or authority hash is invalid")
	}
	switch plan.Plan.AuthorityChangePolicy {
	case AuthorityReject, AuthorityDualRun, AuthorityEscalate:
	default:
		return PinnedPlan{}, invalidPlan("authority change policy %q is invalid", plan.Plan.AuthorityChangePolicy)
	}
	if len(plan.Plan.Gates) == 0 || len(plan.Plan.Gates) > 256 {
		return PinnedPlan{}, invalidPlan("plan requires between 1 and 256 gates")
	}
	seenGate := map[string]bool{}
	baseline := false
	lastTier := TierAdmissionBaseline
	for index := range plan.Plan.Gates {
		gate, err := normalizeGate(plan.Plan.Gates[index])
		if err != nil {
			return PinnedPlan{}, invalidPlan("gate %d: %v", index+1, err)
		}
		if seenGate[gate.ID] {
			return PinnedPlan{}, invalidPlan("gate %q is duplicated", gate.ID)
		}
		if index > 0 && gate.Tier < lastTier {
			return PinnedPlan{}, invalidPlan("gates are not ordered by tier")
		}
		if gate.Tier == TierAdmissionBaseline {
			baseline = true
		} else if gate.Source != plan.Candidate {
			return PinnedPlan{}, invalidPlan("non-baseline gate %q is not bound to the candidate source", gate.ID)
		}
		seenGate[gate.ID] = true
		lastTier = gate.Tier
		plan.Plan.Gates[index] = gate
	}
	if !baseline && !plan.Plan.AllowMissingBaseline {
		return PinnedPlan{}, invalidPlan("Tier 0 baseline is absent without explicit policy")
	}
	planRaw, err := json.Marshal(plan.Plan)
	if err != nil {
		return PinnedPlan{}, invalidPlan("encode plan: %v", err)
	}
	planSHA := hashBytes(planRaw)
	if plan.PlanSHA256 != "" && plan.PlanSHA256 != planSHA {
		return PinnedPlan{}, invalidPlan("plan hash does not match canonical plan")
	}
	plan.PlanSHA256 = planSHA
	return plan, nil
}

func normalizeGate(gate Gate) (Gate, error) {
	if !identity.MatchString(gate.ID) || gate.Tier < TierAdmissionBaseline || gate.Tier > TierFinal {
		return Gate{}, invalidPlan("identity or tier is invalid")
	}
	if err := validateSource(gate.Source); err != nil {
		return Gate{}, err
	}
	if len(gate.Argv) == 0 || len(gate.Argv) > 256 {
		return Gate{}, invalidPlan("command argv is empty or oversized")
	}
	total := 0
	for _, argument := range gate.Argv {
		if argument == "" || !utf8.ValidString(argument) || strings.ContainsRune(argument, 0) || len(argument) > 4096 {
			return Gate{}, invalidPlan("command argv contains an invalid argument")
		}
		total += len(argument)
	}
	if total > 64<<10 {
		return Gate{}, invalidPlan("command argv exceeds 65536 bytes")
	}
	if gate.WorkingDirectory == "" {
		gate.WorkingDirectory = "/workspace"
	}
	cleaned := path.Clean(gate.WorkingDirectory)
	if cleaned != gate.WorkingDirectory || (cleaned != "/workspace" && !strings.HasPrefix(cleaned, "/workspace/")) || len(cleaned) > 4096 {
		return Gate{}, invalidPlan("working directory is not a canonical workspace path")
	}
	sort.Slice(gate.Environment, func(i, j int) bool { return gate.Environment[i].Name < gate.Environment[j].Name })
	for index, variable := range gate.Environment {
		upper := strings.ToUpper(variable.Name)
		if !envName.MatchString(variable.Name) || variable.Value == "" || !utf8.ValidString(variable.Value) || strings.ContainsRune(variable.Value, 0) || len(variable.Value) > 16<<10 || (index > 0 && gate.Environment[index-1].Name == variable.Name) {
			return Gate{}, invalidPlan("environment authority is invalid or duplicated")
		}
		for _, secret := range []string{"SECRET", "TOKEN", "PASSWORD", "CREDENTIAL", "PRIVATE_KEY"} {
			if strings.Contains(upper, secret) {
				return Gate{}, invalidPlan("environment %q cannot persist a secret value", variable.Name)
			}
		}
	}
	if gate.Image.Reference == "" || len(gate.Image.Reference) > 512 || !regexp.MustCompile(`^sha256:[0-9a-f]{64}$`).MatchString(gate.Image.Digest) {
		return Gate{}, invalidPlan("image reference or digest is invalid")
	}
	if gate.SandboxProfile != sandbox.ProfileStrict && gate.SandboxProfile != sandbox.ProfileCompatible {
		return Gate{}, invalidPlan("sandbox profile is invalid")
	}
	if !hexSHA256.MatchString(gate.SandboxProfileSHA256) {
		return Gate{}, invalidPlan("sandbox profile hash is invalid")
	}
	if gate.Resources.CPUs <= 0 || gate.Resources.MemoryBytes <= 0 || gate.Resources.PIDs <= 0 || gate.Resources.TimeoutSeconds <= 0 || gate.Resources.TmpfsBytes <= 0 {
		return Gate{}, invalidPlan("sandbox resource authority is incomplete")
	}
	if gate.Parser.Version == "" {
		gate.Parser.Version = DefaultStructuredParserVersion
	}
	switch gate.Parser.Kind {
	case ParserNone, ParserGoTestJSON, ParserJSON, ParserJUnitXML:
	default:
		return Gate{}, invalidPlan("parser kind %q is invalid", gate.Parser.Kind)
	}
	if len(gate.Parser.Version) > 256 {
		return Gate{}, invalidPlan("parser version is oversized")
	}
	if gate.OutputPolicy.Version == "" {
		gate.OutputPolicy.Version = DefaultOutputPolicyVersion
	}
	if gate.OutputPolicy.StdoutMaxBytes <= 0 || gate.OutputPolicy.StdoutMaxBytes > MaximumCapturedStreamBytes || gate.OutputPolicy.StderrMaxBytes <= 0 || gate.OutputPolicy.StderrMaxBytes > MaximumCapturedStreamBytes {
		return Gate{}, invalidPlan("output policy limits must be within the authoritative capture bound")
	}
	sort.Slice(gate.AuthorityInputs, func(i, j int) bool {
		if gate.AuthorityInputs[i].Kind == gate.AuthorityInputs[j].Kind {
			return gate.AuthorityInputs[i].Path < gate.AuthorityInputs[j].Path
		}
		return gate.AuthorityInputs[i].Kind < gate.AuthorityInputs[j].Kind
	})
	for index, input := range gate.AuthorityInputs {
		cleanedPath := path.Clean(input.Path)
		if !identity.MatchString(input.Kind) || input.Path == "" || cleanedPath != input.Path || path.IsAbs(input.Path) || strings.HasPrefix(cleanedPath, "../") || !hexSHA256.MatchString(input.SHA256) || input.SizeBytes < 0 {
			return Gate{}, invalidPlan("material authority input is invalid")
		}
		if index > 0 && input.Kind == gate.AuthorityInputs[index-1].Kind && input.Path == gate.AuthorityInputs[index-1].Path {
			return Gate{}, invalidPlan("material authority input is duplicated")
		}
	}
	return gate, nil
}

func validateSource(source SourceIdentity) error {
	if !hexRevision.MatchString(source.Commit) || !hexRevision.MatchString(source.Tree) {
		return invalidPlan("source commit or tree is invalid")
	}
	return nil
}

func ExecutionFingerprint(plan PinnedPlan, gate Gate) (string, error) {
	pinned, err := Pin(plan)
	if err != nil {
		return "", err
	}
	var selected *Gate
	for index := range pinned.Plan.Gates {
		if pinned.Plan.Gates[index].ID == gate.ID {
			selected = &pinned.Plan.Gates[index]
			break
		}
	}
	if selected == nil {
		return "", invalidPlan("gate %q is not in the pinned plan", gate.ID)
	}
	input := struct {
		SchemaVersion           string                 `json:"schema_version"`
		VerifierProtocol        string                 `json:"verifier_protocol_version"`
		VerifierImplementation  string                 `json:"verifier_implementation_version"`
		ProjectID               string                 `json:"project_id"`
		TaskID                  string                 `json:"task_id"`
		TaskVersionID           string                 `json:"task_version_id"`
		VerificationPlanVersion string                 `json:"verification_plan_version"`
		VerificationPlanSHA256  string                 `json:"verification_plan_sha256"`
		PlanVersion             string                 `json:"plan_version"`
		PlanSHA256              string                 `json:"plan_sha256"`
		Source                  SourceIdentity         `json:"source"`
		Argv                    []string               `json:"argv"`
		WorkingDirectory        string                 `json:"working_directory"`
		Environment             []EnvironmentVariable  `json:"environment"`
		Image                   sandbox.Image          `json:"image"`
		SandboxRole             sandbox.Role           `json:"sandbox_role"`
		SandboxNetwork          sandbox.NetworkProfile `json:"sandbox_network"`
		SandboxProfile          sandbox.RuntimeProfile `json:"sandbox_profile"`
		SandboxProfileSHA256    string                 `json:"sandbox_profile_sha256"`
		Resources               sandbox.Resources      `json:"resources"`
		Parser                  Parser                 `json:"parser"`
		ProjectEnvironment      ProjectEnvironment     `json:"project_environment"`
		AuthorityInputs         []MaterialInput        `json:"authority_inputs"`
		OutputPolicy            OutputPolicy           `json:"output_policy"`
	}{
		SchemaVersion:    "revolvr-verification-execution-fingerprint-v1",
		VerifierProtocol: pinned.VerifierProtocol, VerifierImplementation: pinned.VerifierImplementation,
		ProjectID: pinned.ProjectID, TaskID: pinned.TaskID, TaskVersionID: pinned.TaskVersionID,
		VerificationPlanVersion: pinned.Plan.VerificationPlanVersion,
		VerificationPlanSHA256:  pinned.Plan.VerificationPlanSHA256,
		PlanVersion:             pinned.Plan.Version, PlanSHA256: pinned.PlanSHA256,
		Source: selected.Source, Argv: selected.Argv, WorkingDirectory: selected.WorkingDirectory,
		Environment: selected.Environment, Image: selected.Image, SandboxRole: sandbox.RoleVerifier,
		SandboxNetwork: sandbox.NetworkNone, SandboxProfile: selected.SandboxProfile,
		SandboxProfileSHA256: selected.SandboxProfileSHA256, Resources: selected.Resources,
		Parser: selected.Parser, ProjectEnvironment: pinned.ProjectEnvironment,
		AuthorityInputs: selected.AuthorityInputs, OutputPolicy: selected.OutputPolicy,
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("verification fingerprint: %w", err)
	}
	return hashBytes(raw), nil
}

func canonicalJSON(raw json.RawMessage) ([]byte, error) {
	var value map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil || value == nil {
		return nil, fmt.Errorf("empty, malformed, or non-object JSON")
	}
	return json.Marshal(value)
}

func canonicalAnyJSON(raw json.RawMessage) ([]byte, error) {
	var value any
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil || value == nil {
		return nil, fmt.Errorf("empty or malformed JSON")
	}
	return json.Marshal(value)
}

func hashBytes(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func invalidPlan(format string, values ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidPlan, fmt.Sprintf(format, values...))
}
