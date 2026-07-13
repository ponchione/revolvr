package autonomousmetrics

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousarchive"
	"revolvr/internal/autonomousfinalization"
	"revolvr/internal/autonomouspolicy"
	"revolvr/internal/autonomousqueue"
	"revolvr/internal/autonomoustaskrun"
	"revolvr/internal/autonomousverification"
	"revolvr/internal/ledger"
)

func LogicalSource(snapshot ledger.Snapshot) Source {
	raw, _ := json.Marshal(snapshot)
	sum := sha256.Sum256(raw)
	events := 0
	for _, run := range snapshot.Runs {
		events += len(run.Events)
	}
	return Source{Kind: "logical_ledger", Reference: "sha256:" + hex.EncodeToString(sum[:]), RunCount: len(snapshot.Runs), EventCount: events, MaxEventID: snapshot.MaxEventID}
}

func Project(snapshot ledger.Snapshot, source Source) (Projection, error) {
	result := Projection{SchemaVersion: SchemaVersion, Source: source}
	wantSource := LogicalSource(snapshot)
	if result.Source.Kind == "" {
		result.Source = wantSource
	} else if result.Source.Kind == "logical_ledger" && result.Source.Reference != wantSource.Reference {
		return Projection{}, errors.New("metrics: logical source identity mismatch")
	}
	result.Source.RunCount = len(snapshot.Runs)
	result.Source.MaxEventID = snapshot.MaxEventID
	result.Source.EventCount = 0
	counts := map[string]int64{}
	correctionCounts := map[string]int64{}
	resolutionCounts := map[string]int64{}
	queueStops := map[string]int64{}
	seenTask := map[string][]byte{}
	seenAttempt := map[string][]byte{}
	seenAudit := map[string][]byte{}
	seenFinding := map[string]bool{}
	seenVerification := map[string][]byte{}
	seenArchive := map[string][]byte{}
	seenFinalization := map[string][]byte{}
	seenQueue := map[string][]byte{}
	latestResolution := map[string]autonomous.FindingResolution{}
	lastStats := map[string]autonomoustaskrun.Statistics{}
	for _, history := range snapshot.Runs {
		if history.Run.CompletedAt != nil {
			result.Usage.RecordedRunDurationNanoseconds += int64(history.Run.DurationSeconds) * int64(time.Second)
		}
		for _, event := range history.Events {
			result.Source.EventCount++
			ref := Reference{RunID: history.Run.ID, EventID: event.ID, TaskID: history.Run.TaskID}
			schema := ledger.EventPayloadSchema(event.Payload)
			switch event.Type {
			case ledger.EventTaskRunCycleCompleted, ledger.EventTaskRunStopped:
				if schema == "autonomous-task-run-event-v1" || schema == ledger.LegacyEventPayloadSchema {
					result.Omissions = append(result.Omissions, Omission{Code: "legacy_task_run_event", Detail: "typed task metrics unavailable", Reference: &ref})
					continue
				}
				item, err := autonomoustaskrun.DecodeLedgerEvent(event.Payload)
				if err != nil {
					return Projection{}, fmt.Errorf("metrics task event %d: %w", event.ID, err)
				}
				ref.TaskID, ref.Operation = item.TaskID, item.OperationID
				if item.Metrics != nil {
					for _, r := range item.Metrics.FindingResolutions {
						latestResolution[item.TaskID+"\x00"+r.FindingID] = r
					}
					if err := addAttempts(&result, seenAttempt, item.TaskID, item.Metrics.Attempts.Events, ref); err != nil {
						return Projection{}, err
					}
					if item.Metrics.CircuitBreaker != nil {
						result.Stops = appendStop(result.Stops, item.TaskID, item.OperationID, "circuit_breaker", string(item.Metrics.CircuitBreaker.Reason), ref)
					}
				}
				if event.Type == ledger.EventTaskRunCycleCompleted {
					prior := lastStats[item.OperationID]
					if item.Statistics.Corrections > prior.Corrections {
						kind := correctionKind(item)
						delta := item.Statistics.Corrections - prior.Corrections
						correctionCounts[kind] += delta
						result.Attempts.CorrectionCycles += delta
						for i := int64(0); i < delta; i++ {
							result.Attempts.CorrectionFacts = append(result.Attempts.CorrectionFacts, CorrectionFact{TaskID: item.TaskID, OperationID: item.OperationID, Cycle: item.Cycle, Kind: kind, RunID: item.RunID, Reference: ref})
						}
					}
					lastStats[item.OperationID] = item.Statistics
					if len(item.Audit) > 0 {
						if err := addAudit(&result, seenAudit, seenFinding, item, ref); err != nil {
							return Projection{}, err
						}
					}
					continue
				}
				if item.StopReason == autonomoustaskrun.StopNoTask {
					result.Omissions = append(result.Omissions, Omission{Code: "no_task_not_terminal_operation", Detail: item.OperationID, Reference: &ref})
					continue
				}
				canonical, _ := json.Marshal(item)
				if prior, ok := seenTask[item.OperationID]; ok {
					if !bytes.Equal(prior, canonical) {
						return Projection{}, errors.New("metrics: conflicting terminal task operation " + item.OperationID)
					}
					continue
				}
				seenTask[item.OperationID] = canonical
				counts[outcomeClass(item.StopReason)]++
				result.TaskOutcomes.Total++
				if item.StopReason == autonomoustaskrun.StopCompleted {
					result.TaskOutcomes.SuccessNumerator++
				}
				result.TaskOutcomes.Facts = append(result.TaskOutcomes.Facts, OutcomeFact{item.TaskID, item.OperationID, string(item.StopReason), *item.CompletedAt, ref})
				result.Usage.TaskDurationNanoseconds += item.CompletedAt.Sub(item.StartedAt).Nanoseconds()
				if item.StopReason == autonomoustaskrun.StopBudgetExhausted || item.StopReason == autonomoustaskrun.StopNoProgress || item.StopReason == autonomoustaskrun.StopSafety || item.StopReason == autonomoustaskrun.StopUnsafeAmbiguous {
					result.Stops = appendStop(result.Stops, item.TaskID, item.OperationID, "task_stop", item.StopDetail, ref)
				}
			case ledger.EventVerificationCompleted:
				if schema == ledger.LegacyEventPayloadSchema {
					result.Omissions = append(result.Omissions, Omission{Code: "legacy_verification_event", Detail: "tier and flaky evidence unavailable", Reference: &ref})
					continue
				}
				item, err := autonomousverification.DecodeCompletedLedgerEvent(event.Payload)
				if err != nil {
					return Projection{}, fmt.Errorf("metrics verification event %d: %w", event.ID, err)
				}
				key := history.Run.ID + "\x00" + item.OccurrenceID
				raw, _ := json.Marshal(item)
				if prior, ok := seenVerification[key]; ok {
					if !bytes.Equal(prior, raw) {
						return Projection{}, errors.New("metrics: conflicting verification occurrence " + key)
					}
					continue
				}
				seenVerification[key] = raw
				addVerification(&result, item, Reference{RunID: history.Run.ID, EventID: event.ID, TaskID: item.TaskID, Occurrence: item.OccurrenceID})
			case ledger.EventArchiveCompleted:
				if schema == autonomousarchive.LegacyLedgerEventSchemaVersion || schema == ledger.LegacyEventPayloadSchema {
					result.Omissions = append(result.Omissions, Omission{Code: "legacy_archive_event", Detail: "exact archive latency unavailable", Reference: &ref})
					continue
				}
				item, err := autonomousarchive.DecodeLedgerEvent(event.Payload)
				if err != nil {
					return Projection{}, fmt.Errorf("metrics archive event %d: %w", event.ID, err)
				}
				raw, _ := json.Marshal(item)
				if prior, ok := seenArchive[item.ArchiveID]; ok {
					if !bytes.Equal(prior, raw) {
						return Projection{}, errors.New("metrics: conflicting archive " + item.ArchiveID)
					}
					continue
				}
				seenArchive[item.ArchiveID] = raw
				addArchive(&result, item, ref)
			case ledger.EventFinalizationCompleted:
				if schema == autonomousfinalization.LegacyLedgerEventSchemaVersion || schema == ledger.LegacyEventPayloadSchema {
					result.Omissions = append(result.Omissions, Omission{Code: "legacy_finalization_event", Detail: "exact completed-task terminal time unavailable", Reference: &ref})
					continue
				}
				item, err := autonomousfinalization.DecodeLedgerEvent(event.Payload)
				if err != nil {
					return Projection{}, fmt.Errorf("metrics finalization event %d: %w", event.ID, err)
				}
				raw, _ := json.Marshal(item)
				if prior, ok := seenFinalization[item.OperationID]; ok {
					if !bytes.Equal(prior, raw) {
						return Projection{}, errors.New("metrics: conflicting finalization " + item.OperationID)
					}
					continue
				}
				seenFinalization[item.OperationID] = raw
				result.Archives.TerminalCompletions = append(result.Archives.TerminalCompletions, CompletionFact{TaskID: item.TaskID, OperationID: item.OperationID, TerminalAt: item.TerminalAt, Reference: ref})
			case ledger.EventQueueStopped:
				if schema == "autonomous-queue-event-v1" || schema == ledger.LegacyEventPayloadSchema {
					result.Omissions = append(result.Omissions, Omission{Code: "legacy_queue_event", Detail: "exact queue throughput unavailable", Reference: &ref})
					continue
				}
				item, err := autonomousqueue.DecodeLedgerEvent(event.Payload)
				if err != nil {
					return Projection{}, fmt.Errorf("metrics queue event %d: %w", event.ID, err)
				}
				if item.SchemaVersion == autonomousqueue.LegacyLedgerEventSchemaVersion {
					result.Omissions = append(result.Omissions, Omission{Code: "legacy_queue_concurrency", Detail: "configured and peak queue workers unavailable", Reference: &ref})
				}
				raw, _ := json.Marshal(item)
				if prior, ok := seenQueue[item.OperationID]; ok {
					if !bytes.Equal(prior, raw) {
						return Projection{}, errors.New("metrics: conflicting queue " + item.OperationID)
					}
					continue
				}
				seenQueue[item.OperationID] = raw
				addQueue(&result, item, ref)
				queueStops[string(item.StopReason)]++
			}
		}
	}
	for key, resolution := range latestResolution {
		_ = key
		if resolution.Status != autonomous.FindingResolutionStatusOpen {
			resolutionCounts[string(resolution.Status)]++
		}
	}
	result.TaskOutcomes.SuccessDenominator = result.TaskOutcomes.Total
	result.TaskOutcomes.Counts = countsSlice(counts)
	result.Attempts.Corrections = countsSlice(correctionCounts)
	result.Audits.Resolutions = countsSlice(resolutionCounts)
	result.Queues.StopReasons = countsSlice(queueStops)
	applyFindingResolutions(&result, latestResolution)
	sortProjection(&result)
	if err := result.Validate(); err != nil {
		return Projection{}, err
	}
	return result, nil
}

func addAttempts(result *Projection, seen map[string][]byte, task string, events []autonomous.AttemptEvent, ref Reference) error {
	for _, event := range events {
		if !validAttemptEvent(event) {
			return fmt.Errorf("metrics: malformed attempt event %q", event.AttemptID)
		}
		key := task + "\x00" + event.AttemptID + "\x00" + string(event.Kind)
		raw, _ := json.Marshal(event)
		if prior, ok := seen[key]; ok {
			if !bytes.Equal(prior, raw) {
				return errors.New("metrics: conflicting attempt " + event.AttemptID)
			}
			continue
		}
		seen[key] = raw
		fact := AttemptFact{TaskID: task, AttemptID: event.AttemptID, Sequence: event.Sequence, Kind: string(event.Kind), Action: string(event.Action), RunID: event.RunID, OccurrenceID: event.OccurrenceID, Outcome: string(event.Outcome), DurationNanoseconds: event.Duration.Nanoseconds(), Tokens: event.Tokens, CreatedAt: event.CreatedAt, Reference: ref}
		result.Attempts.Facts = append(result.Attempts.Facts, fact)
		if event.Kind == autonomous.AttemptEventAdmitted {
			result.Attempts.Admitted++
		} else {
			result.Attempts.Completed++
			result.Usage.AttemptDurationNanoseconds += event.Duration.Nanoseconds()
			if event.Tokens == nil {
				result.Usage.AttemptsMissingTokens++
			} else {
				result.Usage.AttemptsWithTokens++
				result.Usage.RecordedTokens += *event.Tokens
			}
		}
	}
	return nil
}

func validAttemptEvent(event autonomous.AttemptEvent) bool {
	if event.Sequence <= 0 || event.AttemptID == "" || event.CreatedAt.IsZero() {
		return false
	}
	if event.Kind == autonomous.AttemptEventAdmitted {
		return event.Outcome == ""
	}
	if event.Kind != autonomous.AttemptEventCompleted || event.Duration < 0 {
		return false
	}
	switch event.Outcome {
	case autonomous.AttemptOutcomeSucceeded, autonomous.AttemptOutcomeFailed, autonomous.AttemptOutcomeNoProgress, autonomous.AttemptOutcomeCancelled, autonomous.AttemptOutcomeSafetyStopped:
		return true
	default:
		return false
	}
}

func correctionKind(item autonomoustaskrun.LedgerEvent) string {
	var audit autonomouspolicy.AuditEvidence
	if len(item.Audit) > 0 && json.Unmarshal(item.Audit, &audit) == nil && audit.Report.Disposition == autonomous.AuditDispositionChangesRequired {
		return "audit_finding"
	}
	var verification autonomouspolicy.VerificationEvidence
	if len(item.Verification) > 0 && json.Unmarshal(item.Verification, &verification) == nil && verification.Summary.Status == autonomous.VerificationStatusFailed {
		return "verification_repair"
	}
	return "typed_unspecified"
}

func addAudit(result *Projection, seen map[string][]byte, seenFinding map[string]bool, item autonomoustaskrun.LedgerEvent, ref Reference) error {
	var audit autonomouspolicy.AuditEvidence
	decoder := json.NewDecoder(bytes.NewReader(item.Audit))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&audit); err != nil {
		return err
	}
	if err := audit.Report.Validate(); err != nil {
		return err
	}
	key := item.TaskID + "\x00" + audit.RunID
	raw, _ := json.Marshal(audit)
	if prior, ok := seen[key]; ok {
		if !bytes.Equal(prior, raw) {
			return errors.New("metrics: conflicting audit " + audit.RunID)
		}
		return nil
	}
	seen[key] = raw
	fact := AuditFact{TaskID: item.TaskID, AuditRunID: audit.RunID, Disposition: string(audit.Report.Disposition), Reference: ref}
	for _, finding := range audit.Report.Findings {
		if finding.Significance == autonomous.FindingSignificanceBlocking {
			fact.BlockingFindings++
			result.Audits.BlockingFindings++
		} else {
			fact.NonblockingFindings++
			result.Audits.NonblockingFindings++
		}
		fkey := item.TaskID + "\x00" + finding.ID
		if !seenFinding[fkey] {
			seenFinding[fkey] = true
			result.Audits.Findings = append(result.Audits.Findings, FindingFact{TaskID: item.TaskID, FindingID: finding.ID, Significance: string(finding.Significance), IntroducedByAuditRunID: audit.RunID, Disposition: "open", Reference: ref})
		}
	}
	result.Audits.Performed++
	if audit.Report.Disposition == autonomous.AuditDispositionClean {
		result.Audits.Clean++
	} else {
		result.Audits.ChangesRequired++
	}
	result.Audits.Facts = append(result.Audits.Facts, fact)
	return nil
}

func addVerification(result *Projection, item autonomousverification.CompletedLedgerEvent, ref Reference) {
	fact := VerificationFact{TaskID: item.TaskID, RunID: ref.RunID, OccurrenceID: item.OccurrenceID, Purpose: string(item.Purpose), Outcome: string(item.Outcome), Reference: ref}
	result.Verification.Occurrences++
	for _, tier := range item.Tiers {
		fact.TierAttempts++
		result.Verification.TierAttempts++
		for _, command := range tier.Commands {
			fact.CommandAttempts += int64(len(command.Attempts))
			result.Verification.CommandAttempts += int64(len(command.Attempts))
			if len(command.Attempts) > 1 {
				fact.Reruns++
				result.Verification.Reruns++
			}
			if command.Outcome == autonomousverification.OutcomeFlaky {
				fact.Flaky++
				result.Verification.FlakyClassifications++
			}
			for _, attempt := range command.Attempts {
				switch attempt.Outcome {
				case autonomousverification.OutcomePassed:
					result.Verification.OrdinaryPasses++
				case autonomousverification.OutcomeFailed:
					result.Verification.OrdinaryFailures++
				case autonomousverification.OutcomeTimedOut:
					result.Verification.Timeouts++
				case autonomousverification.OutcomeCancelled:
					result.Verification.Cancellations++
				case autonomousverification.OutcomeMissing:
					result.Verification.MissingCommands++
				case autonomousverification.OutcomeRunnerError:
					result.Verification.RunnerErrors++
				}
				fact.DurationNanoseconds += attempt.Duration.Nanoseconds()
			}
		}
	}
	result.Usage.VerificationDurationNanoseconds += fact.DurationNanoseconds
	result.Verification.Facts = append(result.Verification.Facts, fact)
}

func addArchive(result *Projection, item autonomousarchive.LedgerEvent, ref Reference) {
	latency := item.ArchivedAt.Sub(item.TerminalAt).Nanoseconds()
	fact := ArchiveFact{item.TaskID, item.ArchiveID, string(item.Disposition), item.TerminalAt, item.ArchivedAt, latency, ref}
	result.Archives.Facts = append(result.Archives.Facts, fact)
	switch item.Disposition {
	case autonomousarchive.DispositionCompleted:
		result.Archives.Completed++
	case autonomousarchive.DispositionCancelled:
		result.Archives.Cancelled++
	case autonomousarchive.DispositionSuperseded:
		result.Archives.Superseded++
	case autonomousarchive.DispositionAbandoned:
		result.Archives.Abandoned++
	}
	result.Archives.LatencyCount++
	result.Archives.LatencyNanoseconds += latency
}

func addQueue(result *Projection, item autonomousqueue.LedgerEvent, ref Reference) {
	duration := item.CompletedAt.Sub(item.StartedAt).Nanoseconds()
	drained := int64(0)
	if item.StopReason == autonomousqueue.StopDrained {
		drained = 1
	}
	fact := QueueFact{OperationID: item.OperationID, Mode: string(item.Mode), Sweep: item.Sweep, Selections: item.Statistics.Selections, TasksRun: item.Statistics.TasksRun, MaximumWorkers: item.MaximumWorkers, PeakActiveWorkers: item.Statistics.PeakActiveWorkers, Batches: item.Statistics.Batches, SequentialFallbacks: item.Statistics.SequentialFallbacks, Drained: drained, StopReason: string(item.StopReason), DurationNanoseconds: duration, ThroughputNumerator: item.Statistics.TasksRun, ThroughputDenominatorNanoseconds: duration, Reference: ref}
	result.Queues.Sweeps++
	result.Queues.Selections += item.Statistics.Selections
	result.Queues.TasksRun += item.Statistics.TasksRun
	result.Queues.Drained += drained
	if item.MaximumWorkers > result.Queues.MaximumConfiguredWorkers {
		result.Queues.MaximumConfiguredWorkers = item.MaximumWorkers
	}
	if item.Statistics.PeakActiveWorkers > result.Queues.PeakActiveWorkers {
		result.Queues.PeakActiveWorkers = item.Statistics.PeakActiveWorkers
	}
	if item.Statistics.PeakActiveWorkers > 1 {
		result.Queues.ParallelSweeps++
	}
	result.Queues.SequentialFallbacks += item.Statistics.SequentialFallbacks
	result.Usage.QueueDurationNanoseconds += duration
	result.Queues.Facts = append(result.Queues.Facts, fact)
}

func appendStop(values []StopFact, task, operation, class, reason string, ref Reference) []StopFact {
	return append(values, StopFact{task, operation, class, strings.TrimSpace(reason), ref})
}

func outcomeClass(reason autonomoustaskrun.StopReason) string {
	switch reason {
	case autonomoustaskrun.StopCompleted:
		return "completed"
	case autonomoustaskrun.StopBlocked:
		return "blocked"
	case autonomoustaskrun.StopNeedsInput:
		return "needs_input"
	case autonomoustaskrun.StopSafety:
		return "safety"
	case autonomoustaskrun.StopBudgetExhausted:
		return "budget"
	case autonomoustaskrun.StopNoProgress:
		return "no_progress"
	case autonomoustaskrun.StopTaskCancelled, autonomoustaskrun.StopOperationCancelled:
		return "cancelled"
	case autonomoustaskrun.StopMaxCycles:
		return "max_cycle"
	case autonomoustaskrun.StopUnsafeAmbiguous:
		return "unsafe"
	default:
		return string(reason)
	}
}
func countsSlice(values map[string]int64) []Count {
	out := make([]Count, 0, len(values))
	for key, value := range values {
		out = append(out, Count{key, value})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
func applyFindingResolutions(result *Projection, resolutions map[string]autonomous.FindingResolution) {
	for i := range result.Audits.Findings {
		key := result.Audits.Findings[i].TaskID + "\x00" + result.Audits.Findings[i].FindingID
		if value, ok := resolutions[key]; ok {
			result.Audits.Findings[i].Disposition = string(value.Status)
		}
	}
}
func sortProjection(p *Projection) {
	sort.Slice(p.TaskOutcomes.Facts, func(i, j int) bool { return p.TaskOutcomes.Facts[i].OperationID < p.TaskOutcomes.Facts[j].OperationID })
	sort.Slice(p.Attempts.Facts, func(i, j int) bool {
		a, b := p.Attempts.Facts[i], p.Attempts.Facts[j]
		if a.TaskID != b.TaskID {
			return a.TaskID < b.TaskID
		}
		if a.Sequence != b.Sequence {
			return a.Sequence < b.Sequence
		}
		return a.AttemptID < b.AttemptID
	})
	sort.Slice(p.Attempts.CorrectionFacts, func(i, j int) bool {
		a, b := p.Attempts.CorrectionFacts[i], p.Attempts.CorrectionFacts[j]
		if a.OperationID != b.OperationID {
			return a.OperationID < b.OperationID
		}
		return a.Cycle < b.Cycle
	})
	sort.Slice(p.Audits.Facts, func(i, j int) bool { return p.Audits.Facts[i].AuditRunID < p.Audits.Facts[j].AuditRunID })
	sort.Slice(p.Audits.Findings, func(i, j int) bool {
		a, b := p.Audits.Findings[i], p.Audits.Findings[j]
		if a.TaskID != b.TaskID {
			return a.TaskID < b.TaskID
		}
		return a.FindingID < b.FindingID
	})
	sort.Slice(p.Verification.Facts, func(i, j int) bool {
		a, b := p.Verification.Facts[i], p.Verification.Facts[j]
		if a.RunID != b.RunID {
			return a.RunID < b.RunID
		}
		return a.OccurrenceID < b.OccurrenceID
	})
	sort.Slice(p.Archives.Facts, func(i, j int) bool { return p.Archives.Facts[i].ArchiveID < p.Archives.Facts[j].ArchiveID })
	sort.Slice(p.Archives.TerminalCompletions, func(i, j int) bool {
		return p.Archives.TerminalCompletions[i].OperationID < p.Archives.TerminalCompletions[j].OperationID
	})
	sort.Slice(p.Queues.Facts, func(i, j int) bool { return p.Queues.Facts[i].OperationID < p.Queues.Facts[j].OperationID })
	sort.Slice(p.Stops, func(i, j int) bool {
		a, b := p.Stops[i], p.Stops[j]
		if a.OperationID != b.OperationID {
			return a.OperationID < b.OperationID
		}
		return a.Reason < b.Reason
	})
	sort.Slice(p.Omissions, func(i, j int) bool {
		if p.Omissions[i].Code != p.Omissions[j].Code {
			return p.Omissions[i].Code < p.Omissions[j].Code
		}
		return p.Omissions[i].Detail < p.Omissions[j].Detail
	})
}
