package autonomouscycle

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousaudit"
	"revolvr/internal/autonomousplanning"
	"revolvr/internal/autonomouspolicy"
	"revolvr/internal/prompt"
	"revolvr/internal/taskfile"
)

type workerPromptInput struct {
	Task              taskfile.Task
	Dossier           autonomous.TaskDossier
	Decision          autonomous.SupervisorDecision
	Reference         autonomous.DecisionReference
	Route             autonomouspolicy.Route
	Profile           prompt.RunProfile
	RunID             string
	ReceiptPath       string
	OutputPath        string
	SourceRevision    string
	Verification      *autonomouspolicy.VerificationEvidence
	Audit             *autonomouspolicy.AuditEvidence
	LatestMutation    *autonomouspolicy.SourceMutation
	CorrectionFailure *autonomous.VerificationFailureTarget
}

type routePromptProjection struct {
	Kind           autonomouspolicy.RouteKind `json:"kind"`
	TaskID         string                     `json:"task_id"`
	DecisionID     string                     `json:"decision_id"`
	Action         autonomous.Action          `json:"action"`
	WorkerProfile  autonomous.WorkerProfile   `json:"worker_profile"`
	SourceRevision string                     `json:"source_revision"`
}

type correctionScope struct {
	FindingIDs          []string                              `json:"finding_ids"`
	Findings            []autonomous.AuditFinding             `json:"findings"`
	VerificationFailure *autonomous.VerificationFailureTarget `json:"verification_failure,omitempty"`
}

func buildWorkerPrompt(in workerPromptInput) ([]byte, error) {
	if in.Task.ID == "" || in.Task.ID != in.Route.TaskID || in.Decision.TaskID != in.Route.TaskID || in.Reference.TaskID != in.Route.TaskID {
		return nil, errors.New("build worker prompt: task identity mismatch")
	}
	if in.Profile.Name != string(in.Route.WorkerProfile) {
		return nil, errors.New("build worker prompt: profile does not match route")
	}
	if in.Decision.Action != in.Route.Action || in.Reference.Action != in.Route.Action || in.Reference.DecisionID != in.Route.DecisionID {
		return nil, errors.New("build worker prompt: decision/reference/route mismatch")
	}
	if in.SourceRevision != in.Route.SourceRevision {
		return nil, errors.New("build worker prompt: source revision does not match route")
	}
	decisionRaw, err := marshalPromptJSON(in.Decision)
	if err != nil {
		return nil, err
	}
	referenceRaw, err := marshalPromptJSON(in.Reference)
	if err != nil {
		return nil, err
	}
	routeRaw, err := marshalPromptJSON(routePromptProjection{
		Kind:           in.Route.Kind,
		TaskID:         in.Route.TaskID,
		DecisionID:     in.Route.DecisionID,
		Action:         in.Route.Action,
		WorkerProfile:  in.Route.WorkerProfile,
		SourceRevision: in.Route.SourceRevision,
	})
	if err != nil {
		return nil, err
	}

	var out bytes.Buffer
	out.WriteString("# Revolvr Fresh Autonomous Worker Pass\n\n")
	out.WriteString("This is exactly one fresh, ephemeral worker session. Execute only the validated route below and then stop. You must not route or start another worker, invoke Codex recursively, launch nested Codex, or resume any session. Do not create a Git commit; Revolvr owns verification and commit decisions.\n\n")
	out.WriteString("Worker run ID: ")
	out.WriteString(in.RunID)
	out.WriteString("\nTask ID: ")
	out.WriteString(in.Task.ID)
	out.WriteString("\nAction: ")
	out.WriteString(string(in.Route.Action))
	out.WriteString("\nExact profile: ")
	out.WriteString(string(in.Route.WorkerProfile))
	out.WriteString("\nExact admitted source revision: ")
	out.WriteString(in.SourceRevision)
	out.WriteString("\nReceipt path: ")
	out.WriteString(in.ReceiptPath)
	out.WriteString("\nFinal-output artifact: ")
	out.WriteString(in.OutputPath)
	out.WriteString("\n\n## Exact Repo-Authored Worker Profile\n\n")
	out.WriteString(in.Profile.Description)
	out.WriteString("\n\n## Mutation Authority\n\n")
	out.WriteString(actionAuthority(in.Route.Action))
	out.WriteString("\n\nNever edit the canonical task file, autonomous execution state, plans, acceptance records, findings, ledger, or other harness runtime evidence. Preserve unrelated and pre-existing user work. Observed Git evidence, harness verification, and the harness commit result override worker claims.\n")
	out.WriteString("\n## Exact Validated Route\n\n```json\n")
	out.Write(routeRaw)
	out.WriteString("```\n\n## Exact Validated Supervisor Decision\n\n```json\n")
	out.Write(decisionRaw)
	out.WriteString("```\n\n## Exact Supervisor Decision Reference\n\n```json\n")
	out.Write(referenceRaw)
	out.WriteString("```\n")
	if in.Route.Action == autonomous.ActionCorrect {
		scope, err := selectedCorrectionScope(in.Decision, in.Audit, in.CorrectionFailure)
		if err != nil {
			return nil, err
		}
		scopeRaw, err := marshalPromptJSON(scope)
		if err != nil {
			return nil, err
		}
		out.WriteString("\n## Exclusive Correction Authority\n\nOnly the following cited finding IDs and evidence authorize source changes. Other dossier material is context, not correction scope.\n\n```json\n")
		out.Write(scopeRaw)
		out.WriteString("```\n")
	}
	if in.Route.Action == autonomous.ActionPlan {
		profileSum := sha256.Sum256([]byte(in.Profile.Description))
		provenance := autonomousplanning.PlanningProvenance{
			Action: in.Route.Action, WorkerProfile: in.Route.WorkerProfile,
			WorkerRunID: in.RunID, Decision: in.Reference,
			Dossier: autonomousplanning.DossierIdentity{
				SchemaVersion: in.Dossier.Manifest.SchemaVersion,
				TaskID:        in.Dossier.Manifest.TaskID,
				SHA256:        in.Dossier.Manifest.DossierSHA256,
				ByteSize:      in.Dossier.Manifest.DossierByteSize,
			},
			Profile: autonomousplanning.ProfileIdentity{
				Name: in.Route.WorkerProfile, Path: filepath.ToSlash(in.Profile.SourcePath),
				SHA256: fmt.Sprintf("%x", profileSum), ByteSize: len([]byte(in.Profile.Description)),
			},
			RawOutputPath: filepath.ToSlash(in.OutputPath), SourceRevision: in.SourceRevision,
		}
		provenanceRaw, err := marshalPromptJSON(provenance)
		if err != nil {
			return nil, err
		}
		out.WriteString("\n## Exact Planning Output Provenance\n\nCopy these values exactly into `provenance` in the PlanningOutput object.\n\n```json\n")
		out.Write(provenanceRaw)
		out.WriteString("```\n")
	} else if in.Route.Action == autonomous.ActionAudit {
		if in.Verification == nil {
			return nil, errors.New("build worker prompt: audit requires exact current verification evidence")
		}
		profileSum := sha256.Sum256([]byte(in.Profile.Description))
		provenance := autonomousaudit.AuditProvenance{
			Action: in.Route.Action, WorkerProfile: in.Route.WorkerProfile,
			WorkerRunID: in.RunID, Decision: in.Reference,
			Dossier:       autonomousaudit.DossierIdentity{SchemaVersion: in.Dossier.Manifest.SchemaVersion, TaskID: in.Dossier.Manifest.TaskID, SHA256: in.Dossier.Manifest.DossierSHA256, ByteSize: in.Dossier.Manifest.DossierByteSize},
			Profile:       autonomousaudit.ProfileIdentity{Name: in.Route.WorkerProfile, Path: filepath.ToSlash(in.Profile.SourcePath), SHA256: fmt.Sprintf("%x", profileSum), ByteSize: len([]byte(in.Profile.Description))},
			RawOutputPath: filepath.ToSlash(in.OutputPath), SourceRevision: in.SourceRevision,
			Verification: *in.Verification, LatestSourceMutation: autonomousaudit.SourceMutationFromPolicy(in.LatestMutation),
		}
		provenanceRaw, err := marshalPromptJSON(provenance)
		if err != nil {
			return nil, err
		}
		out.WriteString("\n## Exact Audit Output Provenance\n\nCopy these values exactly into `provenance` in the AuditOutput object.\n\n```json\n")
		out.Write(provenanceRaw)
		out.WriteString("```\n")
	}
	out.WriteString("\n## Exact Current Task Dossier\n\n")
	out.Write(in.Dossier.Markdown)
	if len(in.Dossier.Markdown) == 0 || in.Dossier.Markdown[len(in.Dossier.Markdown)-1] != '\n' {
		out.WriteByte('\n')
	}
	out.WriteString("\n## Worker Output and Receipt\n\n")
	if in.Route.Action == autonomous.ActionPlan {
		taskOrigin := autonomousplanning.CanonicalTaskOrigin(in.Task.SourcePath, in.Task.SourceSHA256())
		originRaw, err := marshalPromptJSON(taskOrigin)
		if err != nil {
			return nil, err
		}
		out.WriteString("Return exactly one PlanningOutput JSON object conforming to the harness-supplied planner-only output schema, with no surrounding prose or Markdown. The object must contain the complete proposed current plan revision, complete current acceptance matrix, typed inputs, and exact provenance values shown above. Set provenance.raw_output_path to the Final-output artifact path, provenance.worker_run_id to the Worker run ID, and use the exact route, decision reference, dossier, profile, and admitted source identities. Task-origin acceptance criteria must cite this exact source object and repeat a requirement exactly present in the canonical task/specification:\n\n```json\n")
		out.Write(originRaw)
		out.WriteString("```\n\nA supervisor-refinement criterion must exactly match one current decision success_criteria string and cite the exact supervisor decision artifact. Preserve every existing criterion and every terminal plan step unchanged. Do not write a receipt; the structured planner output is the authoritative planner artifact and Revolvr will synthesize any advisory run receipt separately.\n")
	} else if in.Route.Action == autonomous.ActionAudit {
		out.WriteString("Return exactly one AuditOutput JSON object conforming to the harness-supplied auditor-only output schema, with no surrounding prose or Markdown. Inspect the canonical task/specification, current plan and acceptance matrix, exact source/diff evidence, verification commands/results, and concrete repository files independently. Copy the exact provenance above. The report must cite every exact current verification evidence reference in report.inputs. Use `clean` only with no findings; otherwise use `changes_required` with stable lower-case kebab-case finding IDs, explicit significance, concrete typed evidence, and required corrections. Do not use receipts or implementation self-report as substitutes for source and verification inspection. Do not write a receipt; the structured output is authoritative and Revolvr will synthesize any advisory receipt separately.\n")
	} else if in.Route.Action == autonomous.ActionCorrect {
		out.WriteString("Return exactly one CorrectionOutput JSON object conforming to the harness-supplied corrector-only schema, with no surrounding prose or Markdown. Copy task_id, worker_run_id, decision_id, and the exclusive authority exactly. Partition cited finding IDs between resolved_finding_ids and remaining_finding_ids; never name an uncited finding. For verification-failure repair, set verification_failure_addressed only when the exact failure was addressed. Cite concrete new file/test evidence and report failed or partial work honestly. Do not write a receipt; Revolvr will synthesize advisory receipt evidence separately.\n")
	} else {
		out.WriteString("Return a concise exact final response describing observed work and blockers. Also write or update the receipt path above using schema `revolvr.receipt.v1`, with run_id and pass_id equal to the worker run ID and task_id equal to the exact task ID. Include `Summary`, `Changed Files`, `Verification`, `Concerns`, and `Next Steps` sections. Worker claims are advisory: Revolvr will deterministically synthesize a fallback for a missing or malformed receipt and rewrite changed-file, verification, commit, verdict, timestamp, and metric fields from harness evidence.\n")
	}
	return out.Bytes(), nil
}

func actionAuthority(action autonomous.Action) string {
	switch action {
	case autonomous.ActionPlan:
		return "Planning is source read-only. Produce one structured planning proposal only. Do not edit source, tests, documentation, task files, or runtime state; a separate AW-11 boundary validates and may persist the proposal after this AW-10 cycle returns."
	case autonomous.ActionAudit:
		return "Auditing is source read-only. Produce one structured independent AuditOutput only. Do not edit source, tests, documentation, task files, or runtime state; a separate AW-12 boundary validates and may persist the report after this AW-10 cycle returns."
	case autonomous.ActionImplement:
		return "Implementation may edit only source and tests needed for the selected task and current plan. Do not broaden scope or alter harness-owned lifecycle evidence."
	case autonomous.ActionCorrect:
		return "Correction may edit source and tests only within the exclusive cited audit-finding or verification-failure scope below. Do not address uncited findings or unrelated work."
	case autonomous.ActionDocument:
		return "Documentation may edit only the exact documentation targets identified by structured evidence in the validated route. Do not change behavior or unrelated source. A clean no-change result is allowed and must be reported honestly."
	case autonomous.ActionSimplify:
		return "Simplification may edit only the exact complexity, duplication, or maintainability targets identified by structured evidence in the validated route. Preserve behavior and tests; do not add features or unrelated cleanup. A clean no-change result is allowed and must be reported honestly."
	default:
		return "No worker mutation authority exists for this action."
	}
}

func selectedCorrectionScope(decision autonomous.SupervisorDecision, audit *autonomouspolicy.AuditEvidence, failure *autonomous.VerificationFailureTarget) (correctionScope, error) {
	if decision.VerificationFailure != nil {
		if failure == nil {
			return correctionScope{}, errors.New("build worker prompt: correction requires exact verification-failure evidence")
		}
		if err := autonomous.ValidateVerificationCorrectionDecision(decision, *failure); err != nil {
			return correctionScope{}, err
		}
		copy := *failure
		copy.Evidence = append([]autonomous.EvidenceReference(nil), failure.Evidence...)
		return correctionScope{VerificationFailure: &copy}, nil
	}
	if audit == nil {
		return correctionScope{}, errors.New("build worker prompt: correction requires current audit evidence")
	}
	byID := make(map[string]autonomous.AuditFinding, len(audit.Report.Findings))
	for _, finding := range audit.Report.Findings {
		byID[finding.ID] = finding
	}
	scope := correctionScope{FindingIDs: append([]string(nil), decision.FindingIDs...)}
	for _, findingID := range decision.FindingIDs {
		finding, ok := byID[findingID]
		if !ok {
			return correctionScope{}, fmt.Errorf("build worker prompt: cited finding %q is absent from current audit", findingID)
		}
		scope.Findings = append(scope.Findings, finding)
	}
	return scope, nil
}

func marshalPromptJSON(value any) ([]byte, error) {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}
