// Package autonomousnotification owns bounded external notification delivery.
// It consumes durable source outcomes but never mutates their authority.
package autonomousnotification

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	PolicySchemaVersion  = "revolvr-notification-policy-v1"
	PayloadSchemaVersion = "revolvr-notification-payload-v1"
	IntentSchemaVersion  = "revolvr-notification-intent-v1"
	JournalSchemaVersion = "revolvr-notification-journal-v1"
	HistorySchemaVersion = "revolvr-notification-transition-v1"
	ResultSchemaVersion  = "revolvr-notification-result-v1"
	DirectoryRepository  = "repository_root"
	MaxDetailBytes       = 2048
	MaxAttempts          = 5
	MaxTimeout           = 5 * time.Minute
	MaxRetryDelay        = time.Minute
	MaxOutputCap         = 1 << 20
)

type Event string

const (
	EventTaskCompleted  Event = "task_completed"
	EventTaskBlocked    Event = "task_blocked"
	EventTaskNeedsInput Event = "task_needs_input"
	EventSafetyStop     Event = "safety_stop"
	EventQueueDrained   Event = "queue_drained"
	EventDaemonFailed   Event = "daemon_failed"
)

var eventOrder = []Event{EventTaskCompleted, EventTaskBlocked, EventTaskNeedsInput, EventSafetyStop, EventQueueDrained, EventDaemonFailed}
var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var safeIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func (e Event) Valid() bool {
	for _, candidate := range eventOrder {
		if e == candidate {
			return true
		}
	}
	return false
}

type Policy struct {
	SchemaVersion    string        `json:"schema_version"`
	Enabled          bool          `json:"enabled"`
	Events           []Event       `json:"events,omitempty"`
	Executable       string        `json:"executable,omitempty"`
	Args             []string      `json:"args,omitempty"`
	Directory        string        `json:"directory,omitempty"`
	EnvironmentNames []string      `json:"environment_names,omitempty"`
	Timeout          time.Duration `json:"timeout,omitempty"`
	StdoutCap        int           `json:"stdout_cap,omitempty"`
	StderrCap        int           `json:"stderr_cap,omitempty"`
	MaximumAttempts  int           `json:"maximum_attempts,omitempty"`
	RetryDelay       time.Duration `json:"retry_delay,omitempty"`
}

func DefaultPolicy() Policy { return Policy{SchemaVersion: PolicySchemaVersion} }

func (p Policy) Normalize(redactionNames []string) (Policy, error) {
	if p.SchemaVersion == "" {
		p.SchemaVersion = PolicySchemaVersion
	}
	if p.SchemaVersion != PolicySchemaVersion {
		return Policy{}, fmt.Errorf("notification policy: unsupported schema_version %q", p.SchemaVersion)
	}
	p.Events = append([]Event(nil), p.Events...)
	p.Args = append([]string(nil), p.Args...)
	p.EnvironmentNames = append([]string(nil), p.EnvironmentNames...)
	if !p.Enabled {
		if len(p.Events) != 0 || strings.TrimSpace(p.Executable) != "" || len(p.Args) != 0 || strings.TrimSpace(p.Directory) != "" || len(p.EnvironmentNames) != 0 || p.Timeout != 0 || p.StdoutCap != 0 || p.StderrCap != 0 || p.MaximumAttempts != 0 || p.RetryDelay != 0 {
			return Policy{}, errors.New("notification policy: disabled policy cannot configure hook authority")
		}
		return DefaultPolicy(), nil
	}
	if strings.TrimSpace(p.Executable) == "" || p.Executable != strings.TrimSpace(p.Executable) || strings.ContainsRune(p.Executable, 0) {
		return Policy{}, errors.New("notification policy: enabled executable is required and must be exact")
	}
	if len(p.Events) == 0 {
		return Policy{}, errors.New("notification policy: enabled events allowlist is required")
	}
	seenEvents := map[Event]bool{}
	for i, event := range p.Events {
		if !event.Valid() {
			return Policy{}, fmt.Errorf("notification policy: events[%d] is unknown: %q", i, event)
		}
		if seenEvents[event] {
			return Policy{}, fmt.Errorf("notification policy: duplicate event %q", event)
		}
		seenEvents[event] = true
	}
	sort.Slice(p.Events, func(i, j int) bool { return eventIndex(p.Events[i]) < eventIndex(p.Events[j]) })
	for i, arg := range p.Args {
		if strings.ContainsRune(arg, 0) {
			return Policy{}, fmt.Errorf("notification policy: args[%d] contains NUL", i)
		}
	}
	if p.Directory == "" {
		p.Directory = DirectoryRepository
	}
	if p.Directory != DirectoryRepository {
		return Policy{}, fmt.Errorf("notification policy: unsupported directory %q", p.Directory)
	}
	redacted := map[string]bool{}
	for _, name := range redactionNames {
		redacted[name] = true
	}
	seenNames := map[string]bool{}
	for i, name := range p.EnvironmentNames {
		if name != strings.TrimSpace(name) || !envName.MatchString(name) {
			return Policy{}, fmt.Errorf("notification policy: environment_names[%d] is malformed", i)
		}
		if seenNames[name] {
			return Policy{}, fmt.Errorf("notification policy: duplicate environment name %q", name)
		}
		if !redacted[name] {
			return Policy{}, fmt.Errorf("notification policy: environment name %q is not covered by configured secret redaction", name)
		}
		seenNames[name] = true
	}
	if p.Timeout <= 0 || p.Timeout > MaxTimeout {
		return Policy{}, fmt.Errorf("notification policy: timeout must be positive and at most %s", MaxTimeout)
	}
	if p.StdoutCap <= 0 || p.StdoutCap > MaxOutputCap || p.StderrCap <= 0 || p.StderrCap > MaxOutputCap {
		return Policy{}, fmt.Errorf("notification policy: output caps must be positive and at most %d", MaxOutputCap)
	}
	if p.MaximumAttempts <= 0 || p.MaximumAttempts > MaxAttempts {
		return Policy{}, fmt.Errorf("notification policy: maximum_attempts must be between 1 and %d", MaxAttempts)
	}
	if p.RetryDelay < 0 || p.RetryDelay > MaxRetryDelay {
		return Policy{}, fmt.Errorf("notification policy: retry_delay must be between zero and %s", MaxRetryDelay)
	}
	return p, nil
}

func (p Policy) Allows(event Event) bool {
	if !p.Enabled {
		return false
	}
	for _, candidate := range p.Events {
		if event == candidate {
			return true
		}
	}
	return false
}

func (p Policy) Identity(redactionNames []string) (string, error) {
	normalized, err := p.Normalize(redactionNames)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return hash(raw), nil
}

type Reference struct {
	Applicable bool   `json:"applicable"`
	ID         string `json:"id,omitempty"`
	Path       string `json:"path,omitempty"`
	SHA256     string `json:"sha256,omitempty"`
	ByteSize   int    `json:"byte_size,omitempty"`
}

func (r Reference) validate(label string) error {
	if !r.Applicable {
		if r.ID != "" || r.Path != "" || r.SHA256 != "" || r.ByteSize != 0 {
			return fmt.Errorf("notification payload: inapplicable %s reference has material", label)
		}
		return nil
	}
	if strings.TrimSpace(r.ID) == "" && strings.TrimSpace(r.Path) == "" {
		return fmt.Errorf("notification payload: applicable %s reference lacks identity", label)
	}
	if r.SHA256 != "" && !validHash(r.SHA256) || r.ByteSize < 0 {
		return fmt.Errorf("notification payload: invalid %s reference", label)
	}
	return nil
}

type References struct {
	Task      Reference `json:"task"`
	TaskRun   Reference `json:"task_run"`
	WorkerRun Reference `json:"worker_run"`
	Question  Reference `json:"question"`
	Queue     Reference `json:"queue"`
	Daemon    Reference `json:"daemon"`
	Archive   Reference `json:"archive"`
	Safety    Reference `json:"safety"`
	Terminal  Reference `json:"terminal"`
	Evidence  Reference `json:"evidence"`
}

type Payload struct {
	SchemaVersion         string     `json:"schema_version"`
	DeliveryID            string     `json:"delivery_id"`
	EventID               string     `json:"event_id"`
	Event                 Event      `json:"event"`
	OccurredAt            time.Time  `json:"occurred_at"`
	RepositorySHA256      string     `json:"repository_sha256"`
	EffectiveConfigSchema string     `json:"effective_config_schema"`
	EffectiveConfigSHA256 string     `json:"effective_config_sha256"`
	HookPolicySchema      string     `json:"hook_policy_schema"`
	HookPolicySHA256      string     `json:"hook_policy_sha256"`
	SubjectKind           string     `json:"subject_kind"`
	Outcome               string     `json:"outcome"`
	StopReason            string     `json:"stop_reason,omitempty"`
	Detail                string     `json:"detail,omitempty"`
	References            References `json:"references"`
	Omissions             []string   `json:"omissions"`
}

func (p Payload) Validate() error {
	if p.SchemaVersion != PayloadSchemaVersion || !safeID(p.DeliveryID) || !safeID(p.EventID) || !p.Event.Valid() || p.OccurredAt.IsZero() || p.OccurredAt.Location() != time.UTC {
		return errors.New("notification payload: invalid schema, identity, event, or occurrence time")
	}
	if !validHash(p.RepositorySHA256) || strings.TrimSpace(p.EffectiveConfigSchema) == "" || !validHash(p.EffectiveConfigSHA256) || p.HookPolicySchema != PolicySchemaVersion || !validHash(p.HookPolicySHA256) {
		return errors.New("notification payload: invalid repository, config, or hook policy identity")
	}
	if strings.TrimSpace(p.SubjectKind) == "" || strings.TrimSpace(p.Outcome) == "" || len(p.Detail) > MaxDetailBytes {
		return errors.New("notification payload: invalid subject, outcome, or detail")
	}
	refs := []struct {
		name  string
		value Reference
	}{
		{"task", p.References.Task}, {"task_run", p.References.TaskRun}, {"worker_run", p.References.WorkerRun}, {"question", p.References.Question}, {"queue", p.References.Queue}, {"daemon", p.References.Daemon}, {"archive", p.References.Archive}, {"safety", p.References.Safety}, {"terminal", p.References.Terminal}, {"evidence", p.References.Evidence},
	}
	for _, ref := range refs {
		if err := ref.value.validate(ref.name); err != nil {
			return err
		}
	}
	if len(p.Omissions) == 0 {
		return errors.New("notification payload: explicit omissions are required")
	}
	for i, omission := range p.Omissions {
		if strings.TrimSpace(omission) == "" || i > 0 && p.Omissions[i-1] >= omission {
			return errors.New("notification payload: omissions must be nonempty and canonically ordered")
		}
	}
	return nil
}

type EventInput struct {
	Event                 Event
	SourceIdentity        string
	OccurredAt            time.Time
	RepositoryRoot        string
	EffectiveConfigSchema string
	EffectiveConfigSHA256 string
	HookPolicy            Policy
	RedactionNames        []string
	SubjectKind           string
	Outcome               string
	StopReason            string
	Detail                string
	References            References
	Omissions             []string
}

func BuildPayload(in EventInput, redact func(string) string) (Payload, []byte, error) {
	policy, err := in.HookPolicy.Normalize(in.RedactionNames)
	if err != nil {
		return Payload{}, nil, err
	}
	if !policy.Allows(in.Event) {
		return Payload{}, nil, fmt.Errorf("notification payload: event %q is not enabled", in.Event)
	}
	policyID, _ := policy.Identity(in.RedactionNames)
	repo := sha256.Sum256([]byte(strings.TrimSpace(in.RepositoryRoot)))
	omissions := append([]string(nil), in.Omissions...)
	sort.Strings(omissions)
	detail := strings.TrimSpace(in.Detail)
	refs := in.References
	if redact != nil {
		detail = redact(detail)
		for i := range omissions {
			omissions[i] = redact(omissions[i])
		}
		refs = redactReferences(refs, redact)
	}
	sort.Strings(omissions)
	if len(detail) > MaxDetailBytes {
		detail = detail[:MaxDetailBytes]
	}
	eventID := "evt-" + hash([]byte(strings.Join([]string{string(in.Event), in.SourceIdentity, in.OccurredAt.UTC().Format(time.RFC3339Nano)}, "\x00")))[:32]
	subject, outcome, stop := strings.TrimSpace(in.SubjectKind), strings.TrimSpace(in.Outcome), strings.TrimSpace(in.StopReason)
	if redact != nil {
		subject, outcome, stop = redact(subject), redact(outcome), redact(stop)
	}
	payload := Payload{SchemaVersion: PayloadSchemaVersion, EventID: eventID, Event: in.Event, OccurredAt: in.OccurredAt.UTC(), RepositorySHA256: hex.EncodeToString(repo[:]), EffectiveConfigSchema: in.EffectiveConfigSchema, EffectiveConfigSHA256: in.EffectiveConfigSHA256, HookPolicySchema: PolicySchemaVersion, HookPolicySHA256: policyID, SubjectKind: subject, Outcome: outcome, StopReason: stop, Detail: detail, References: refs, Omissions: omissions}
	identityRaw, _ := json.Marshal(payload)
	payload.DeliveryID = "delivery-" + hash(identityRaw)[:32]
	if err := payload.Validate(); err != nil {
		return Payload{}, nil, err
	}
	raw, err := canonical(payload)
	return payload, raw, err
}

func redactReferences(refs References, redact func(string) string) References {
	values := []*Reference{&refs.Task, &refs.TaskRun, &refs.WorkerRun, &refs.Question, &refs.Queue, &refs.Daemon, &refs.Archive, &refs.Safety, &refs.Terminal, &refs.Evidence}
	for _, ref := range values {
		ref.ID, ref.Path = redact(ref.ID), redact(ref.Path)
	}
	return refs
}

func DecodePayload(raw []byte) (Payload, error) {
	var payload Payload
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return Payload{}, fmt.Errorf("notification payload: decode: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return Payload{}, err
	}
	if err := payload.Validate(); err != nil {
		return Payload{}, err
	}
	canonicalRaw, _ := canonical(payload)
	if !bytes.Equal(raw, canonicalRaw) {
		return Payload{}, errors.New("notification payload: non-canonical JSON")
	}
	return payload, nil
}

func canonical(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return errors.New("notification payload: multiple JSON values")
	} else if !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func eventIndex(event Event) int {
	for i, candidate := range eventOrder {
		if candidate == event {
			return i
		}
	}
	return len(eventOrder)
}
func hash(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func validHash(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
func safeID(value string) bool {
	return value == strings.TrimSpace(value) && safeIDPattern.MatchString(value)
}
