// Package autonomousverification owns CLI-independent tiered verification
// plans, deterministic selection, execution evidence, and final-gate
// projections. It does not own task state, correction, audit, or commits.
package autonomousverification

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"revolvr/internal/verification"
)

const (
	PlanSchemaVersion   = "autonomous-verification-plan-v1"
	ResultSchemaVersion = "autonomous-verification-result-v1"
	GateSchemaVersion   = "autonomous-verification-gate-v1"
)

type TierKind string

const (
	TierStructural     TierKind = "structural"
	TierFocused        TierKind = "focused"
	TierTaskAcceptance TierKind = "task_acceptance"
	TierFullSuite      TierKind = "full_suite"
	TierRace           TierKind = "race"
	TierIntegration    TierKind = "integration"
	TierSecurity       TierKind = "security"
)

type Purpose string

const (
	PurposeFast  Purpose = "fast"
	PurposeFinal Purpose = "final"
)

type RerunPolicy string

const (
	RerunNever               RerunPolicy = "never"
	RerunOnceToClassifyFlaky RerunPolicy = "once_to_classify_flaky"
)

type Tier struct {
	ID               string                 `json:"id"`
	Kind             TierKind               `json:"kind"`
	RequiredForFinal bool                   `json:"required_for_final"`
	RunForFast       bool                   `json:"run_for_fast"`
	RunForFinal      bool                   `json:"run_for_final"`
	Commands         []verification.Command `json:"commands"`
	RerunPolicy      RerunPolicy            `json:"rerun_policy"`
}

type Plan struct {
	SchemaVersion string `json:"schema_version"`
	Tiers         []Tier `json:"tiers"`
}

type PlanIdentity struct {
	SchemaVersion string `json:"schema_version"`
	SHA256        string `json:"sha256"`
	ByteSize      int    `json:"byte_size"`
}

type TierSelection struct {
	Purpose            Purpose  `json:"purpose"`
	RequiredFinalTiers []string `json:"required_final_tiers"`
	SelectedTiers      []Tier   `json:"selected_tiers"`
	SelectedTierIDs    []string `json:"selected_tier_ids"`
}

func (p Plan) Validate() error {
	if p.SchemaVersion != PlanSchemaVersion {
		return fmt.Errorf("verification plan: unsupported schema_version %q (want %q)", p.SchemaVersion, PlanSchemaVersion)
	}
	if len(p.Tiers) == 0 {
		return errors.New("verification plan: at least one tier is required")
	}
	ids := make(map[string]struct{}, len(p.Tiers))
	kinds := make(map[TierKind]struct{}, len(p.Tiers))
	previousOrder := -1
	for i, tier := range p.Tiers {
		prefix := fmt.Sprintf("verification plan: tiers[%d]", i)
		if !validStableID(tier.ID) {
			return fmt.Errorf("%s.id %q must be lower-case kebab-case", prefix, tier.ID)
		}
		if _, exists := ids[tier.ID]; exists {
			return fmt.Errorf("%s.id duplicates tier ID %q", prefix, tier.ID)
		}
		ids[tier.ID] = struct{}{}
		order, ok := tierOrder(tier.Kind)
		if !ok {
			return fmt.Errorf("%s.kind has unknown value %q", prefix, tier.Kind)
		}
		if _, exists := kinds[tier.Kind]; exists {
			return fmt.Errorf("%s.kind duplicates tier kind %q", prefix, tier.Kind)
		}
		kinds[tier.Kind] = struct{}{}
		if order <= previousOrder {
			return fmt.Errorf("%s.kind %q violates canonical tier order", prefix, tier.Kind)
		}
		previousOrder = order
		if tier.RequiredForFinal && !tier.RunForFinal {
			return fmt.Errorf("%s requires final verification but run_for_final is false", prefix)
		}
		if tier.RunForFast && order > mustTierOrder(TierTaskAcceptance) {
			return fmt.Errorf("%s kind %q cannot be selected for fast verification", prefix, tier.Kind)
		}
		if !tier.RequiredForFinal && !tier.RunForFast && !tier.RunForFinal {
			return fmt.Errorf("%s is never selected", prefix)
		}
		switch tier.RerunPolicy {
		case RerunNever, RerunOnceToClassifyFlaky:
		default:
			return fmt.Errorf("%s.rerun_policy has unknown value %q", prefix, tier.RerunPolicy)
		}
		for j, command := range tier.Commands {
			if err := validateCommand(command); err != nil {
				return fmt.Errorf("%s.commands[%d]: %w", prefix, j, err)
			}
		}
	}
	return nil
}

func DecodePlan(raw []byte) (Plan, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var plan Plan
	if err := decoder.Decode(&plan); err != nil {
		return Plan{}, fmt.Errorf("decode verification plan: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return Plan{}, errors.New("decode verification plan: multiple JSON values are not allowed")
	} else if !errors.Is(err, io.EOF) {
		return Plan{}, fmt.Errorf("decode verification plan: trailing JSON: %w", err)
	}
	if err := plan.Validate(); err != nil {
		return Plan{}, err
	}
	return ClonePlan(plan), nil
}

func MarshalPlan(plan Plan) ([]byte, error) {
	if err := plan.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(ClonePlan(plan), "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal verification plan: %w", err)
	}
	return append(raw, '\n'), nil
}

func Identity(plan Plan) (PlanIdentity, error) {
	raw, err := MarshalPlan(plan)
	if err != nil {
		return PlanIdentity{}, err
	}
	return PlanIdentity{SchemaVersion: plan.SchemaVersion, SHA256: hashBytes(raw), ByteSize: len(raw)}, nil
}

func AdaptLegacy(commands []verification.Command) Plan {
	return Plan{
		SchemaVersion: PlanSchemaVersion,
		Tiers: []Tier{{
			ID: "legacy-flat", Kind: TierFullSuite,
			RequiredForFinal: true, RunForFinal: true,
			Commands: cloneCommands(commands), RerunPolicy: RerunNever,
		}},
	}
}

func ClonePlan(plan Plan) Plan {
	cloned := plan
	cloned.Tiers = make([]Tier, len(plan.Tiers))
	for i, tier := range plan.Tiers {
		cloned.Tiers[i] = tier
		cloned.Tiers[i].Commands = cloneCommands(tier.Commands)
	}
	return cloned
}

func Select(plan Plan, purpose Purpose) (TierSelection, error) {
	if err := plan.Validate(); err != nil {
		return TierSelection{}, err
	}
	if purpose != PurposeFast && purpose != PurposeFinal {
		return TierSelection{}, fmt.Errorf("verification selection: unknown purpose %q", purpose)
	}
	selection := TierSelection{Purpose: purpose}
	for _, tier := range plan.Tiers {
		if tier.RequiredForFinal {
			selection.RequiredFinalTiers = append(selection.RequiredFinalTiers, tier.ID)
		}
		selected := purpose == PurposeFast && tier.RunForFast || purpose == PurposeFinal && tier.RunForFinal
		if selected {
			selection.SelectedTiers = append(selection.SelectedTiers, cloneTier(tier))
			selection.SelectedTierIDs = append(selection.SelectedTierIDs, tier.ID)
		}
	}
	if len(selection.SelectedTiers) == 0 {
		return TierSelection{}, fmt.Errorf("verification selection: purpose %q selects no tiers", purpose)
	}
	if purpose == PurposeFinal && len(selection.RequiredFinalTiers) == 0 {
		return TierSelection{}, errors.New("verification selection: final verification has no configured required tiers")
	}
	return selection, nil
}

func validateCommand(command verification.Command) error {
	if strings.TrimSpace(command.Name) == "" || command.Name != strings.TrimSpace(command.Name) || strings.ContainsAny(command.Name, "\r\n") {
		return errors.New("command name is empty or malformed")
	}
	if command.Timeout < 0 || command.StdoutCap < 0 || command.StderrCap < 0 {
		return errors.New("timeout and output caps cannot be negative")
	}
	for i, value := range command.Args {
		if strings.ContainsAny(value, "\x00\r\n") {
			return fmt.Errorf("args[%d] contains a NUL or newline", i)
		}
	}
	for i, value := range command.Env {
		if strings.ContainsAny(value, "\x00\r\n") || !strings.Contains(value, "=") || strings.HasPrefix(value, "=") {
			return fmt.Errorf("env[%d] must be an exact NAME=value entry without NULs or newlines", i)
		}
	}
	dir := strings.TrimSpace(command.Dir)
	if dir != command.Dir || filepath.IsAbs(dir) || dir == ".." || strings.HasPrefix(filepath.Clean(dir), ".."+string(filepath.Separator)) {
		return fmt.Errorf("unsafe command directory %q", command.Dir)
	}
	return nil
}

func tierOrder(kind TierKind) (int, bool) {
	switch kind {
	case TierStructural:
		return 0, true
	case TierFocused:
		return 1, true
	case TierTaskAcceptance:
		return 2, true
	case TierFullSuite:
		return 3, true
	case TierRace:
		return 4, true
	case TierIntegration:
		return 5, true
	case TierSecurity:
		return 6, true
	default:
		return 0, false
	}
}

func mustTierOrder(kind TierKind) int { value, _ := tierOrder(kind); return value }

func validStableID(value string) bool {
	if value == "" || value != strings.TrimSpace(value) {
		return false
	}
	for i, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case i > 0 && r >= '0' && r <= '9':
		case i > 0 && r == '-' && value[i-1] != '-' && i < len(value)-1:
		default:
			return false
		}
	}
	return true
}

func cloneTier(tier Tier) Tier { tier.Commands = cloneCommands(tier.Commands); return tier }

func cloneCommands(commands []verification.Command) []verification.Command {
	out := make([]verification.Command, len(commands))
	for i, command := range commands {
		out[i] = command
		out[i].Args = append([]string(nil), command.Args...)
		out[i].Env = append([]string(nil), command.Env...)
	}
	return out
}

func hashBytes(raw []byte) string { sum := sha256.Sum256(raw); return fmt.Sprintf("%x", sum) }

func validHash(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}

func duration(start, end time.Time) time.Duration {
	if start.IsZero() || end.IsZero() || end.Before(start) {
		return 0
	}
	return end.Sub(start)
}
