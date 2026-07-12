// Package autonomousattempt owns durable admission, accounting, progress
// detection, and circuit breaking for exactly one autonomous action at a time.
package autonomousattempt

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"revolvr/internal/autonomous"
)

type Limits struct {
	TaskAttempts           autonomous.CountBudget
	ActionAttempts         []autonomous.ActionBudget
	Elapsed                autonomous.DurationBudget
	Tokens                 autonomous.CountBudget
	RepeatedSignatureLimit int64
}

type Strategy struct {
	Approach   string
	Techniques []string
	Targets    []autonomous.EvidenceReference
}

type VerificationFailureMaterial struct {
	Target         autonomous.VerificationFailureTarget
	Classification string
	TierID         string
	CommandSHA256  string
	Evidence       []autonomous.EvidenceReference
}

type FindingIdentity struct {
	ID            string
	ReportSHA256  string
	ReportBytes   int
	FindingSHA256 string
}

type OpenFindingMaterial struct {
	TaskID       string
	AuditRunID   string
	ReportSHA256 string
	ReportBytes  int
	Findings     []FindingIdentity
}

type OperationFailureMaterial struct {
	TaskID         string
	Action         autonomous.Action
	Stage          string
	Classification string
	Evidence       []autonomous.EvidenceReference
}

func (l Limits) Validate() error {
	if err := limitCount("task attempts", l.TaskAttempts); err != nil {
		return err
	}
	if err := limitDuration("elapsed", l.Elapsed); err != nil {
		return err
	}
	if err := limitCount("tokens", l.Tokens); err != nil {
		return err
	}
	if l.RepeatedSignatureLimit < 1 {
		return fmt.Errorf("repeated signature limit must be at least 1 (got %d)", l.RepeatedSignatureLimit)
	}
	seen := make(map[autonomous.Action]struct{}, len(l.ActionAttempts))
	for i, value := range l.ActionAttempts {
		if !knownAction(value.Action) {
			return fmt.Errorf("action attempts[%d] has unknown action %q", i, value.Action)
		}
		if _, ok := seen[value.Action]; ok {
			return fmt.Errorf("action attempts[%d] duplicates %q", i, value.Action)
		}
		seen[value.Action] = struct{}{}
		if err := limitCount(fmt.Sprintf("action %q", value.Action), value.Budget); err != nil {
			return err
		}
	}
	return nil
}

func (s Strategy) Signature() (string, error) {
	core := autonomous.Strategy{Approach: s.Approach, Techniques: append([]string(nil), s.Techniques...), Targets: append([]autonomous.EvidenceReference(nil), s.Targets...)}
	if err := core.Validate(); err != nil {
		return "", err
	}
	approach := normalizeText(s.Approach)
	if approach == "" {
		return "", errors.New("strategy approach is required")
	}
	techniques := make([]string, 0, len(s.Techniques))
	seen := make(map[string]struct{}, len(s.Techniques))
	for _, raw := range s.Techniques {
		value := normalizeText(raw)
		if value == "" {
			return "", errors.New("strategy techniques must not be empty")
		}
		if _, ok := seen[value]; !ok {
			seen[value] = struct{}{}
			techniques = append(techniques, value)
		}
	}
	sort.Strings(techniques)
	targets, err := canonicalEvidence(s.Targets)
	if err != nil {
		return "", fmt.Errorf("strategy targets: %w", err)
	}
	return hashCanonical(struct {
		Approach   string                         `json:"approach"`
		Techniques []string                       `json:"techniques"`
		Targets    []autonomous.EvidenceReference `json:"targets"`
	}{approach, techniques, targets})
}

func DecisionSignature(decision autonomous.SupervisorDecision) (autonomous.CanonicalSignature, error) {
	if err := decision.Validate(); err != nil {
		return autonomous.CanonicalSignature{}, err
	}
	findings := append([]string(nil), decision.FindingIDs...)
	sort.Strings(findings)
	criteria := make([]string, len(decision.SuccessCriteria))
	for i := range decision.SuccessCriteria {
		criteria[i] = normalizeText(decision.SuccessCriteria[i])
	}
	sort.Strings(criteria)
	evidence, err := canonicalEvidence(decision.Inputs)
	if err != nil {
		return autonomous.CanonicalSignature{}, err
	}
	sha, err := hashCanonical(struct {
		TaskID              string                                `json:"task_id"`
		Action              autonomous.Action                     `json:"action"`
		WorkerProfile       autonomous.WorkerProfile              `json:"worker_profile"`
		FindingIDs          []string                              `json:"finding_ids"`
		VerificationFailure *autonomous.VerificationFailureTarget `json:"verification_failure,omitempty"`
		SuccessCriteria     []string                              `json:"success_criteria"`
		Evidence            []autonomous.EvidenceReference        `json:"evidence"`
	}{decision.TaskID, decision.Action, decision.WorkerProfile, findings, decision.VerificationFailure, criteria, evidence})
	return autonomous.CanonicalSignature{Kind: autonomous.SignatureKindDecision, SHA256: sha, Evidence: evidence}, err
}

func VerificationFailureSignature(material VerificationFailureMaterial) (autonomous.CanonicalSignature, error) {
	if err := material.Target.Validate(); err != nil {
		return autonomous.CanonicalSignature{}, err
	}
	if strings.TrimSpace(material.Classification) == "" {
		return autonomous.CanonicalSignature{}, errors.New("verification failure classification is required")
	}
	if material.CommandSHA256 != "" && !validSHA(material.CommandSHA256) {
		return autonomous.CanonicalSignature{}, errors.New("verification failure command SHA-256 is invalid")
	}
	evidence, err := canonicalEvidence(append(append([]autonomous.EvidenceReference(nil), material.Target.Evidence...), material.Evidence...))
	if err != nil {
		return autonomous.CanonicalSignature{}, err
	}
	sha, err := hashCanonical(struct {
		Target         autonomous.VerificationFailureTarget `json:"target"`
		Classification string                               `json:"classification"`
		TierID         string                               `json:"tier_id"`
		CommandSHA256  string                               `json:"command_sha256"`
		Evidence       []autonomous.EvidenceReference       `json:"evidence"`
	}{material.Target, strings.TrimSpace(material.Classification), strings.TrimSpace(material.TierID), material.CommandSHA256, evidence})
	return autonomous.CanonicalSignature{Kind: autonomous.SignatureKindVerificationFailure, SHA256: sha, Evidence: evidence}, err
}

func OpenFindingSignature(material OpenFindingMaterial) (autonomous.CanonicalSignature, error) {
	if strings.TrimSpace(material.TaskID) == "" || strings.TrimSpace(material.AuditRunID) == "" || !validSHA(material.ReportSHA256) || material.ReportBytes <= 0 || len(material.Findings) == 0 {
		return autonomous.CanonicalSignature{}, errors.New("open finding signature requires task, audit occurrence, report identity, and findings")
	}
	findings := append([]FindingIdentity(nil), material.Findings...)
	sort.Slice(findings, func(i, j int) bool { return findings[i].ID < findings[j].ID })
	for i, finding := range findings {
		if strings.TrimSpace(finding.ID) == "" || !validSHA(finding.ReportSHA256) || finding.ReportBytes <= 0 || !validSHA(finding.FindingSHA256) {
			return autonomous.CanonicalSignature{}, fmt.Errorf("finding identity %d is invalid", i)
		}
		if i > 0 && findings[i-1].ID == finding.ID {
			return autonomous.CanonicalSignature{}, fmt.Errorf("duplicate finding ID %q", finding.ID)
		}
	}
	sha, err := hashCanonical(struct {
		TaskID       string            `json:"task_id"`
		AuditRunID   string            `json:"audit_run_id"`
		ReportSHA256 string            `json:"report_sha256"`
		ReportBytes  int               `json:"report_bytes"`
		Findings     []FindingIdentity `json:"findings"`
	}{material.TaskID, material.AuditRunID, material.ReportSHA256, material.ReportBytes, findings})
	return autonomous.CanonicalSignature{Kind: autonomous.SignatureKindOpenFindings, SHA256: sha}, err
}

func OperationFailureSignature(material OperationFailureMaterial) (autonomous.CanonicalSignature, error) {
	if strings.TrimSpace(material.TaskID) == "" || strings.TrimSpace(material.Stage) == "" || strings.TrimSpace(material.Classification) == "" {
		return autonomous.CanonicalSignature{}, errors.New("operation failure signature requires task, stage, and classification")
	}
	evidence, err := canonicalEvidence(material.Evidence)
	if err != nil {
		return autonomous.CanonicalSignature{}, err
	}
	sha, err := hashCanonical(struct {
		TaskID         string                         `json:"task_id"`
		Action         autonomous.Action              `json:"action"`
		Stage          string                         `json:"stage"`
		Classification string                         `json:"classification"`
		Evidence       []autonomous.EvidenceReference `json:"evidence"`
	}{material.TaskID, material.Action, strings.TrimSpace(material.Stage), strings.TrimSpace(material.Classification), evidence})
	return autonomous.CanonicalSignature{Kind: autonomous.SignatureKindOperationFailure, SHA256: sha, Evidence: evidence}, err
}

func limitCount(label string, budget autonomous.CountBudget) error {
	if err := budget.Validate(); err != nil {
		return fmt.Errorf("%s limit: %w", label, err)
	}
	if budget.Consumed != 0 {
		return fmt.Errorf("%s limit must not supply consumed accounting", label)
	}
	return nil
}

func limitDuration(label string, budget autonomous.DurationBudget) error {
	if err := budget.Validate(); err != nil {
		return fmt.Errorf("%s limit: %w", label, err)
	}
	if budget.Consumed != 0 {
		return fmt.Errorf("%s limit must not supply consumed accounting", label)
	}
	return nil
}

func canonicalEvidence(values []autonomous.EvidenceReference) ([]autonomous.EvidenceReference, error) {
	result := append([]autonomous.EvidenceReference(nil), values...)
	for i := range result {
		result[i].Detail = normalizeText(result[i].Detail)
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i], result[j]
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		if left.Reference != right.Reference {
			return left.Reference < right.Reference
		}
		return left.Detail < right.Detail
	})
	for i := range result {
		probe := autonomous.SupervisorDecision{TaskID: "probe", Action: autonomous.ActionBlock, Rationale: "probe", Inputs: []autonomous.EvidenceReference{result[i]}}
		if err := probe.Validate(); err != nil {
			return nil, fmt.Errorf("evidence[%d]: %w", i, err)
		}
		if i > 0 && result[i] == result[i-1] {
			result = append(result[:i], result[i+1:]...)
			i--
		}
	}
	return result, nil
}

func normalizeText(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func hashCanonical(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum), nil
}

func validSHA(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == sha256.Size && value == strings.ToLower(value)
}

func knownAction(action autonomous.Action) bool {
	switch action {
	case autonomous.ActionPlan, autonomous.ActionImplement, autonomous.ActionAudit, autonomous.ActionCorrect, autonomous.ActionDocument, autonomous.ActionSimplify, autonomous.ActionComplete, autonomous.ActionBlock:
		return true
	default:
		return false
	}
}
