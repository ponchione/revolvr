package artifactretention

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"revolvr/internal/ledger"
	"revolvr/internal/taskfile"
)

const PlanSchema = "revolvr-artifact-gc-plan-v1"

type ArtifactClass string

const (
	ClassCodexJSONL  ArtifactClass = "codex_jsonl"
	ClassCodexStderr ArtifactClass = "codex_stderr"
)

type ActionKind string

const (
	ActionRetain   ActionKind = "retain"
	ActionCompress ActionKind = "compress"
	ActionPrune    ActionKind = "prune"
)

type Pin struct {
	Reason    string `json:"reason"`
	Authority string `json:"authority"`
}
type Action struct {
	Kind           ActionKind    `json:"kind"`
	Path           string        `json:"path"`
	RunID          string        `json:"run_id"`
	TaskID         string        `json:"task_id"`
	Class          ArtifactClass `json:"class"`
	Reason         string        `json:"reason"`
	Source         Identity      `json:"source"`
	ModifiedAt     time.Time     `json:"modified_at"`
	Representation string        `json:"representation"`
	Compressed     *Identity     `json:"compressed,omitempty"`
	Pins           []Pin         `json:"pins,omitempty"`
	BytesAfter     int64         `json:"bytes_after"`
}
type Totals struct {
	Candidates        int   `json:"candidates"`
	Pinned            int   `json:"pinned"`
	Retained          int   `json:"retained"`
	Compress          int   `json:"compress"`
	Prune             int   `json:"prune"`
	BytesBefore       int64 `json:"bytes_before"`
	BytesAfter        int64 `json:"bytes_after"`
	RemainingEligible int   `json:"remaining_eligible"`
}
type LedgerIdentity struct {
	Path             string `json:"path"`
	SHA256           string `json:"sha256"`
	ByteSize         int64  `json:"byte_size"`
	HighWaterEventID int64  `json:"high_water_event_id"`
}
type Plan struct {
	SchemaVersion         string         `json:"schema_version"`
	OperationID           string         `json:"operation_id"`
	PlanID                string         `json:"plan_id"`
	PlanSHA256            string         `json:"plan_sha256"`
	PlanByteSize          int64          `json:"plan_byte_size"`
	FrozenAt              time.Time      `json:"frozen_at"`
	Policy                Policy         `json:"policy"`
	PolicySHA256          string         `json:"policy_sha256"`
	EffectiveConfigSHA256 string         `json:"effective_config_sha256"`
	RepositoryRoot        string         `json:"repository_root"`
	Ledger                LedgerIdentity `json:"ledger"`
	RequiredExport        bool           `json:"required_export"`
	Actions               []Action       `json:"actions"`
	Totals                Totals         `json:"totals"`
	Warnings              []string       `json:"warnings,omitempty"`
}
type PlanInput struct {
	RepositoryRoot        string
	LedgerPath            string
	OperationID           string
	FrozenAt              time.Time
	Policy                Policy
	EffectiveConfigSHA256 string
}

type candidate struct {
	path, runID, taskID string
	class               ArtifactClass
	identity            Identity
	mtime               time.Time
	representation      string
	compressed          *Identity
	pins                []Pin
}

func PlanGC(ctx context.Context, in PlanInput) (Plan, error) {
	if err := in.Policy.Validate(); err != nil {
		return Plan{}, err
	}
	if strings.TrimSpace(in.OperationID) == "" || in.FrozenAt.IsZero() || in.FrozenAt.Location() != time.UTC {
		return Plan{}, errors.New("artifact GC plan: operation ID and frozen UTC planning time are required")
	}
	root, err := canonicalRoot(in.RepositoryRoot)
	if err != nil {
		return Plan{}, err
	}
	policyHash, _, _ := in.Policy.Fingerprint()
	ledgerPath := strings.TrimSpace(in.LedgerPath)
	if ledgerPath == "" {
		ledgerPath = filepath.Join(root, ".revolvr", "ledger.sqlite")
	}
	ledgerRaw, _, err := readRegular(ledgerPath, 1<<30)
	if err != nil {
		return Plan{}, fmt.Errorf("artifact GC plan: ledger: %w", err)
	}
	ledgerRel, _ := filepath.Rel(root, ledgerPath)
	store, err := ledger.OpenLiveReadOnly(ctx, ledgerPath)
	if err != nil {
		return Plan{}, err
	}
	snapshot, readErr := store.ReadSnapshot(ctx)
	closeErr := store.Close()
	if readErr != nil {
		return Plan{}, readErr
	}
	if closeErr != nil {
		return Plan{}, closeErr
	}
	candidates, err := inventory(ctx, root, snapshot, in.Policy)
	if err != nil {
		return Plan{}, err
	}
	afterLedger, _, err := readRegular(ledgerPath, 1<<30)
	if err != nil || !bytes.Equal(afterLedger, ledgerRaw) {
		return Plan{}, errors.Join(err, errors.New("artifact GC plan: ledger changed during inventory"))
	}

	active := map[string]bool{}
	tasks, err := taskfile.List(root)
	if err != nil {
		return Plan{}, fmt.Errorf("artifact GC plan: active tasks: %w", err)
	}
	for _, task := range tasks {
		active[task.ID] = true
	}
	recent := map[string]bool{}
	for i := len(snapshot.Runs) - 1; i >= 0 && len(recent) < in.Policy.RecentRunCount; i-- {
		recent[snapshot.Runs[i].Run.ID] = true
	}
	controlRefs, err := controlReferences(root, candidates, true)
	if err != nil {
		return Plan{}, err
	}
	for i := range candidates {
		c := &candidates[i]
		if active[c.taskID] {
			c.pins = append(c.pins, Pin{Reason: "active_task", Authority: c.taskID})
		}
		if recent[c.runID] {
			c.pins = append(c.pins, Pin{Reason: "recent_run", Authority: c.runID})
		}
		for _, h := range snapshot.Runs {
			if h.Run.ID == c.runID && h.Run.Status == ledger.StatusRunning {
				c.pins = append(c.pins, Pin{Reason: "nonterminal_run", Authority: c.runID})
				break
			}
		}
		for _, authority := range controlRefs[c.path] {
			c.pins = append(c.pins, Pin{Reason: "control_reference", Authority: authority})
		}
		sort.Slice(c.pins, func(i, j int) bool {
			if c.pins[i].Reason != c.pins[j].Reason {
				return c.pins[i].Reason < c.pins[j].Reason
			}
			return c.pins[i].Authority < c.pins[j].Authority
		})
	}

	plan := Plan{SchemaVersion: PlanSchema, OperationID: strings.TrimSpace(in.OperationID), FrozenAt: in.FrozenAt, Policy: in.Policy, PolicySHA256: policyHash, EffectiveConfigSHA256: strings.TrimSpace(in.EffectiveConfigSHA256), RepositoryRoot: root, Ledger: LedgerIdentity{Path: filepath.ToSlash(ledgerRel), SHA256: hash(ledgerRaw), ByteSize: int64(len(ledgerRaw)), HighWaterEventID: snapshot.MaxEventID}}
	eligibleFiles := 0
	var eligibleBytes int64
	for _, c := range candidates {
		a := Action{Path: c.path, RunID: c.runID, TaskID: c.taskID, Class: c.class, Source: c.identity, ModifiedAt: c.mtime, Representation: c.representation, Pins: append([]Pin(nil), c.pins...), BytesAfter: c.identity.ByteSize}
		age := in.FrozenAt.Sub(c.mtime)
		if age < 0 {
			return Plan{}, fmt.Errorf("artifact GC plan: artifact %s modification time is after frozen time", c.path)
		}
		switch {
		case len(c.pins) > 0:
			a.Kind = ActionRetain
			a.Reason = "pinned"
			plan.Totals.Pinned++
		case !in.Policy.MutationEnabled:
			a.Kind = ActionRetain
			a.Reason = "mutation_disabled"
		case in.Policy.PruneCompressedStreams && c.representation == "compressed" && in.Policy.PruneAfter > 0 && age >= in.Policy.PruneAfter:
			a.Kind = ActionPrune
			a.Reason = "prune_age_reached"
			a.BytesAfter = 0
			eligibleFiles++
			eligibleBytes += c.identity.ByteSize
		case c.representation == "original" && c.identity.ByteSize >= in.Policy.MinimumCompressBytes && age >= in.Policy.CompressAfter && classCompressible(in.Policy, c.class):
			raw, _, readErr := readRegular(filepath.Join(root, filepath.FromSlash(c.path)), c.identity.ByteSize)
			if readErr != nil {
				return Plan{}, readErr
			}
			_, compressed, compressErr := compressIdentity(ctx, raw)
			if compressErr != nil {
				return Plan{}, compressErr
			}
			a.Kind = ActionCompress
			a.Reason = "compression_age_reached"
			a.Compressed = &compressed
			a.BytesAfter = compressed.ByteSize
			eligibleFiles++
			eligibleBytes += c.identity.ByteSize
		default:
			a.Kind = ActionRetain
			if c.representation == "compressed" {
				a.Reason = "already_compressed"
			} else {
				a.Reason = "too_recent_or_below_threshold"
			}
		}
		if (a.Kind == ActionCompress || a.Kind == ActionPrune) && (eligibleFiles > in.Policy.MaxFilesPerOperation || eligibleBytes > in.Policy.MaxBytesPerOperation) {
			a.Kind = ActionRetain
			a.Reason = "operation_bound"
			a.Compressed = nil
			a.BytesAfter = a.Source.ByteSize
			plan.Totals.RemainingEligible++
		}
		plan.Actions = append(plan.Actions, a)
		plan.Totals.Candidates++
		plan.Totals.BytesBefore += a.Source.ByteSize
		plan.Totals.BytesAfter += a.BytesAfter
		switch a.Kind {
		case ActionCompress:
			plan.Totals.Compress++
		case ActionPrune:
			plan.Totals.Prune++
		default:
			plan.Totals.Retained++
		}
	}
	plan.RequiredExport = plan.Totals.Prune > 0 && in.Policy.RequireVerifiedExport
	sort.Slice(plan.Actions, func(i, j int) bool { return plan.Actions[i].Path < plan.Actions[j].Path })
	core := plan
	core.PlanID = ""
	core.PlanSHA256 = ""
	core.PlanByteSize = 0
	coreRaw, _ := json.Marshal(core)
	plan.PlanID = hash(coreRaw)
	for i := 0; i < 4; i++ {
		candidate := plan
		candidate.PlanSHA256 = ""
		candidate.PlanByteSize = 0
		raw, _ := canonicalJSON(candidate)
		plan.PlanSHA256 = hash(raw)
		plan.PlanByteSize = int64(len(raw))
	}
	return plan, nil
}

func MarshalPlan(plan Plan) ([]byte, error) {
	if err := ValidatePlan(plan); err != nil {
		return nil, err
	}
	return canonicalJSON(plan)
}
func ValidatePlan(plan Plan) error {
	if plan.SchemaVersion != PlanSchema || plan.PlanID == "" || plan.OperationID == "" || plan.FrozenAt.IsZero() {
		return errors.New("invalid artifact GC plan")
	}
	if err := plan.Policy.Validate(); err != nil {
		return err
	}
	policyHash, _, _ := plan.Policy.Fingerprint()
	if policyHash != plan.PolicySHA256 {
		return errors.New("artifact GC plan: policy identity mismatch")
	}
	var totals Totals
	previous := ""
	for _, action := range plan.Actions {
		if action.Path == "" || action.Path <= previous || !knownStreamPath(action.Path, action.Class) || action.Source.ByteSize < 0 || len(action.Source.SHA256) != 64 {
			return errors.New("artifact GC plan: invalid or unordered action")
		}
		previous = action.Path
		totals.Candidates++
		totals.BytesBefore += action.Source.ByteSize
		totals.BytesAfter += action.BytesAfter
		if len(action.Pins) > 0 {
			totals.Pinned++
		}
		switch action.Kind {
		case ActionRetain:
			totals.Retained++
		case ActionCompress:
			if action.Compressed == nil {
				return errors.New("artifact GC plan: compression identity is missing")
			}
			totals.Compress++
		case ActionPrune:
			totals.Prune++
		default:
			return errors.New("artifact GC plan: unknown action kind")
		}
		if action.Reason == "operation_bound" {
			totals.RemainingEligible++
		}
	}
	if totals != plan.Totals || plan.RequiredExport != (plan.Totals.Prune > 0 && plan.Policy.RequireVerifiedExport) {
		return errors.New("artifact GC plan: totals or export authority mismatch")
	}
	core := plan
	core.PlanID, core.PlanSHA256, core.PlanByteSize = "", "", 0
	coreRaw, _ := json.Marshal(core)
	if hash(coreRaw) != plan.PlanID {
		return errors.New("artifact GC plan: plan ID mismatch")
	}
	identityProjection := plan
	identityProjection.PlanSHA256, identityProjection.PlanByteSize = "", 0
	identityRaw, _ := canonicalJSON(identityProjection)
	if hash(identityRaw) != plan.PlanSHA256 || int64(len(identityRaw)) != plan.PlanByteSize {
		return errors.New("artifact GC plan: plan hash or size mismatch")
	}
	return nil
}

func inventory(ctx context.Context, root string, snapshot ledger.Snapshot, policy Policy) ([]candidate, error) {
	byPath := map[string]candidate{}
	for _, history := range snapshot.Runs {
		artifacts, _ := ledger.RunArtifactsFromEvents(history.Events)
		items := []struct {
			value string
			class ArtifactClass
		}{{artifacts.CodexStdoutJSONLPath, ClassCodexJSONL}, {artifacts.CodexStderrPath, ClassCodexStderr}}
		for _, item := range items {
			if strings.TrimSpace(item.value) == "" || !classCompressible(policy, item.class) {
				continue
			}
			rel, err := normalizeArtifactPath(root, item.value)
			if err != nil {
				return nil, fmt.Errorf("artifact GC inventory: run %s: %w", history.Run.ID, err)
			}
			if !knownStreamPath(rel, item.class) {
				return nil, fmt.Errorf("artifact GC inventory: unsafe or unknown stream path %s", rel)
			}
			if prior, ok := byPath[rel]; ok && (prior.runID != history.Run.ID || prior.taskID != history.Run.TaskID || prior.class != item.class) {
				return nil, fmt.Errorf("artifact GC inventory: ambiguous artifact ownership %s", rel)
			}
			abs := filepath.Join(root, filepath.FromSlash(rel))
			raw, info, readErr := readRegular(abs, 1<<30)
			representation := "original"
			var original Identity
			var mtime time.Time
			var compressed *Identity
			if readErr == nil {
				original = identity(raw)
				mtime = info.ModTime().UTC()
			} else if errors.Is(readErr, os.ErrNotExist) {
				manifestRaw, _, merr := readRegular(abs+".gz.manifest.json", 1<<20)
				if merr != nil {
					return nil, fmt.Errorf("artifact GC inventory: missing required artifact %s", rel)
				}
				var manifest CompressionManifest
				if err := strictJSON(manifestRaw, &manifest); err != nil {
					return nil, err
				}
				canonical, _ := canonicalJSON(manifest)
				if !bytes.Equal(manifestRaw, canonical) || manifest.SchemaVersion != CompressionManifestSchema || filepath.ToSlash(filepath.Clean(manifest.OriginalPath)) != rel {
					return nil, fmt.Errorf("artifact GC inventory: invalid compressed manifest %s", rel)
				}
				if _, err := Read(ctx, root, rel, manifest.Original, policy.DecompressionCapBytes); err != nil {
					return nil, err
				}
				original = manifest.Original
				mtime = manifest.OriginalMTime
				representation = "compressed"
				value := manifest.Compressed
				compressed = &value
			} else {
				return nil, readErr
			}
			byPath[rel] = candidate{path: rel, runID: history.Run.ID, taskID: history.Run.TaskID, class: item.class, identity: original, mtime: mtime, representation: representation, compressed: compressed}
		}
	}
	// Unknown stream-shaped files are never silently admitted by age.
	runsDir := filepath.Join(root, ".revolvr", "runs")
	if err := filepath.WalkDir(runsDir, func(path string, d fs.DirEntry, walkErr error) error {
		if errors.Is(walkErr, os.ErrNotExist) {
			return fs.SkipDir
		}
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return errors.New("artifact GC inventory: symlink in run artifacts")
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if strings.HasSuffix(rel, ".jsonl") || strings.HasSuffix(rel, ".stderr") {
			if _, ok := byPath[rel]; !ok {
				return fmt.Errorf("artifact GC inventory: unknown stream artifact %s", rel)
			}
		}
		return nil
	}); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	out := make([]candidate, 0, len(byPath))
	for _, c := range byPath {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out, nil
}

func controlReferences(root string, candidates []candidate, includeGC bool) (map[string][]string, error) {
	wanted := map[string]bool{}
	runPaths := map[string][]string{}
	for _, c := range candidates {
		wanted[c.path] = true
		runPaths[c.runID] = append(runPaths[c.runID], c.path)
	}
	out := map[string][]string{}
	bases := []string{filepath.Join(root, ".revolvr", "autonomous"), filepath.Join(root, ".agent", "archive")}
	if includeGC {
		bases = append(bases, filepath.Join(root, ".revolvr", "retention", "gc"))
	}
	for _, base := range bases {
		err := filepath.WalkDir(base, func(path string, d fs.DirEntry, walkErr error) error {
			if errors.Is(walkErr, os.ErrNotExist) {
				return fs.SkipDir
			}
			if walkErr != nil {
				return walkErr
			}
			if d.Type()&os.ModeSymlink != 0 {
				return errors.New("artifact GC references: symlinked control path")
			}
			if d.IsDir() && filepath.Clean(filepath.Dir(path)) == filepath.Clean(base) && strings.HasSuffix(filepath.ToSlash(base), "/.revolvr/retention/gc") {
				if cleaned, err := cleanedGCOperation(path); err != nil {
					return err
				} else if cleaned {
					return fs.SkipDir
				}
			}
			if d.IsDir() || filepath.Ext(path) != ".json" {
				return nil
			}
			raw, _, err := readRegular(path, 16<<20)
			if err != nil {
				return err
			}
			var value any
			if json.Unmarshal(raw, &value) != nil {
				return nil
			}
			authority, _ := filepath.Rel(root, path)
			walkStrings(value, func(s string) {
				if paths := runPaths[strings.TrimSpace(s)]; len(paths) > 0 {
					for _, candidatePath := range paths {
						out[candidatePath] = append(out[candidatePath], filepath.ToSlash(authority))
					}
				}
				if rel, err := normalizeArtifactPath(root, s); err == nil && wanted[rel] {
					out[rel] = append(out[rel], filepath.ToSlash(authority))
				}
			})
			return nil
		})
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	for key := range out {
		sort.Strings(out[key])
		out[key] = dedupe(out[key])
	}
	return out, nil
}

func cleanedGCOperation(dir string) (bool, error) {
	raw, _, err := readRegular(filepath.Join(dir, "journal.json"), 32<<20)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var header struct {
		Stage string `json:"stage"`
	}
	if err := json.Unmarshal(raw, &header); err != nil {
		return false, err
	}
	return header.Stage == "cleaned", nil
}
func walkStrings(value any, fn func(string)) {
	switch v := value.(type) {
	case string:
		fn(v)
	case []any:
		for _, x := range v {
			walkStrings(x, fn)
		}
	case map[string]any:
		for _, x := range v {
			walkStrings(x, fn)
		}
	}
}
func normalizeArtifactPath(root, value string) (string, error) {
	value = strings.TrimSpace(value)
	if filepath.IsAbs(value) {
		abs, err := filepath.Abs(value)
		if err != nil {
			return "", err
		}
		if !strings.HasPrefix(abs, root+string(filepath.Separator)) {
			return "", errors.New("artifact path escapes repository")
		}
		value, _ = filepath.Rel(root, abs)
	}
	clean := filepath.Clean(value)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("artifact path escapes repository")
	}
	return filepath.ToSlash(clean), nil
}
func knownStreamPath(rel string, class ArtifactClass) bool {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) < 4 || parts[0] != ".revolvr" || parts[1] != "runs" || parts[2] == "" {
		return false
	}
	switch class {
	case ClassCodexJSONL:
		return strings.HasSuffix(rel, ".jsonl")
	case ClassCodexStderr:
		return strings.HasSuffix(rel, ".stderr")
	default:
		return false
	}
}
func classCompressible(p Policy, class ArtifactClass) bool {
	return class == ClassCodexJSONL && p.CompressCodexJSONL || class == ClassCodexStderr && p.CompressCodexStderr
}
func hash(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func dedupe(values []string) []string {
	if len(values) < 2 {
		return values
	}
	out := values[:1]
	for _, v := range values[1:] {
		if v != out[len(out)-1] {
			out = append(out, v)
		}
	}
	return out
}
