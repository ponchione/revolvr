package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousdaemon"
	"revolvr/internal/autonomousnotification"
	"revolvr/internal/autonomousqueue"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/autonomoustaskrun"
	"revolvr/internal/redact"
	"revolvr/internal/runonce"
)

type NotificationObserver func(autonomousnotification.Result, error)

type NotificationRuntime struct {
	Clock           func() time.Time
	Runner          autonomousnotification.CommandRunner
	LookPath        autonomousnotification.LookPath
	LookupEnv       autonomousnotification.LookupEnv
	Wait            autonomousnotification.Wait
	failureInjector notificationInterruptionInjector
}

type notificationInterruptionPoint string

const (
	notificationFailureBeforeDelivery notificationInterruptionPoint = "before_delivery"
	notificationFailureAfterDelivery  notificationInterruptionPoint = "after_delivery"
)

type notificationInterruptionInjector func(notificationInterruptionPoint) error

func dispatchTaskOutcome(ctx context.Context, root string, result autonomoustaskrun.Result, runtime NotificationRuntime, observer NotificationObserver) {
	event := autonomousnotification.Event("")
	switch result.StopReason {
	case autonomoustaskrun.StopCompleted:
		event = autonomousnotification.EventTaskCompleted
	case autonomoustaskrun.StopBlocked:
		event = autonomousnotification.EventTaskBlocked
	case autonomoustaskrun.StopNeedsInput:
		event = autonomousnotification.EventTaskNeedsInput
	case autonomoustaskrun.StopSafety:
		event = autonomousnotification.EventSafetyStop
	default:
		return
	}
	policy, effective, fingerprint, redactor, ok, err := notificationAuthority(root, runtime, event)
	if err != nil {
		observeNotification(observer, autonomousnotification.Result{}, err)
		return
	}
	if !ok {
		return
	}
	op, found, err := autonomoustaskrun.Inspect(root, result.OperationID)
	if err != nil || !found || op.CompletedAt == nil || op.StopReason != result.StopReason || op.TaskID != result.TaskID {
		observeNotification(observer, autonomousnotification.Result{}, errors.Join(err, errors.New("notification adapter: durable task-run authority is missing or mismatched")))
		return
	}
	refs := emptyReferences()
	refs.Task = autonomousnotification.Reference{Applicable: true, ID: op.TaskID, Path: op.Task.Path, SHA256: op.Task.SHA256, ByteSize: op.Task.ByteSize}
	refs.TaskRun = autonomousnotification.Reference{Applicable: true, ID: op.OperationID, Path: fmt.Sprintf(".revolvr/autonomous/task-runs/%s/operation.json", op.OperationID)}
	if op.LastRunID != "" {
		refs.WorkerRun = autonomousnotification.Reference{Applicable: true, ID: op.LastRunID}
	}
	refs.Terminal = autonomousnotification.Reference{Applicable: true, ID: "terminal-" + hashText(op.OperationID, string(op.StopReason))[:24]}
	refs.Evidence = autonomousnotification.Reference{Applicable: true, Path: op.State.Path, SHA256: op.State.SHA256, ByteSize: op.State.ByteSize}
	omissions := []string{"daemon reference not applicable", "queue reference not applicable"}
	omissions = append(omissions, "archive reference not applicable because the task outcome does not create or verify an archive")
	if event == autonomousnotification.EventTaskNeedsInput {
		store, storeErr := autonomousstate.New(autonomousstate.Config{RepositoryRoot: root})
		snapshot, stateFound, loadErr := store.Load(ctx, op.TaskID)
		if storeErr != nil || loadErr != nil || !stateFound {
			observeNotification(observer, autonomousnotification.Result{}, errors.Join(storeErr, loadErr, errors.New("notification adapter: canonical task state authority is missing")))
			return
		}
		var question *autonomous.QuestionIdentity
		for i := len(snapshot.State.Input.Questions) - 1; i >= 0; i-- {
			record := snapshot.State.Input.Questions[i]
			if op.LastDecisionID != "" && record.Decision.DecisionID == op.LastDecisionID {
				identity := record.Question.Identity()
				question = &identity
				break
			}
		}
		if question == nil && op.LastDecisionID == "" && snapshot.State.NeedsInput != nil && snapshot.State.NeedsInput.CurrentQuestion != nil {
			current := *snapshot.State.NeedsInput.CurrentQuestion
			question = &current
		}
		if question == nil {
			observeNotification(observer, autonomousnotification.Result{}, errors.New("notification adapter: legacy needs-input outcome has no typed question authority"))
			return
		}
		refs.Question = autonomousnotification.Reference{Applicable: true, ID: fmt.Sprintf("%s-r%d", question.QuestionID, question.Revision), SHA256: question.ContentSHA256}
	} else {
		omissions = append(omissions, "question reference not applicable")
	}
	if event == autonomousnotification.EventSafetyStop {
		refs.Safety = autonomousnotification.Reference{Applicable: true, ID: "task-" + op.OperationID}
	} else {
		omissions = append(omissions, "safety reference not applicable")
	}
	deliverNotification(ctx, root, effective, fingerprint, redactor, policy, autonomousnotification.EventInput{Event: event, SourceIdentity: hashText("task-run", op.OperationID, fmt.Sprint(op.Sequence), string(op.StopReason)), OccurredAt: *op.CompletedAt, SubjectKind: "task", Outcome: string(op.StopReason), StopReason: string(op.StopReason), Detail: op.StopDetail, References: refs, Omissions: omissions}, runtime, observer)
}

func dispatchQueueOutcome(ctx context.Context, root string, result autonomousqueue.Result, runtime NotificationRuntime, observer NotificationObserver) {
	for _, outcome := range result.Outcomes {
		dispatchTaskOutcome(ctx, root, autonomoustaskrun.Result{SchemaVersion: autonomoustaskrun.ResultSchemaVersion, OperationID: outcome.TaskOperationID, TaskID: outcome.TaskID, StopReason: outcome.StopReason}, runtime, observer)
	}
	event := autonomousnotification.Event("")
	switch result.StopReason {
	case autonomousqueue.StopDrained:
		event = autonomousnotification.EventQueueDrained
	case autonomousqueue.StopSafety:
		event = autonomousnotification.EventSafetyStop
	default:
		return
	}
	policy, effective, fingerprint, redactor, ok, err := notificationAuthority(root, runtime, event)
	if err != nil {
		observeNotification(observer, autonomousnotification.Result{}, err)
		return
	}
	if !ok {
		return
	}
	op, found, err := autonomousqueue.Inspect(root, result.OperationID)
	if err != nil || !found || op.CompletedAt == nil || op.StopReason != result.StopReason {
		observeNotification(observer, autonomousnotification.Result{}, errors.Join(err, errors.New("notification adapter: durable queue authority is missing or mismatched")))
		return
	}
	refs := emptyReferences()
	refs.Queue = autonomousnotification.Reference{Applicable: true, ID: op.OperationID, Path: fmt.Sprintf(".revolvr/autonomous/queues/%s/queue.json", op.OperationID)}
	refs.Terminal = autonomousnotification.Reference{Applicable: true, ID: "terminal-" + hashText(op.OperationID, string(op.StopReason))[:24]}
	omissions := []string{"archive reference not applicable", "daemon reference not applicable", "question reference not applicable", "task reference not applicable", "task-run reference not applicable", "worker-run reference not applicable"}
	if event == autonomousnotification.EventSafetyStop {
		refs.Safety = autonomousnotification.Reference{Applicable: true, ID: "queue-" + op.OperationID}
	} else {
		omissions = append(omissions, "safety reference not applicable")
	}
	deliverNotification(ctx, root, effective, fingerprint, redactor, policy, autonomousnotification.EventInput{Event: event, SourceIdentity: hashText("queue", op.OperationID, fmt.Sprint(op.Sequence), string(op.StopReason)), OccurredAt: *op.CompletedAt, SubjectKind: "queue", Outcome: string(op.StopReason), StopReason: string(op.StopReason), Detail: op.StopDetail, References: refs, Omissions: omissions}, runtime, observer)
}

func dispatchDaemonFailure(ctx context.Context, root, operationID string, result autonomousdaemon.Result, sourceErr error, runtime NotificationRuntime, observer NotificationObserver) {
	if errors.Is(sourceErr, context.Canceled) || result.StopReason == autonomousdaemon.StopCancelled || result.StopReason == autonomousdaemon.StopSafety {
		return
	}
	if sourceErr == nil && result.StopReason != autonomousdaemon.StopUnsafe {
		return
	}
	policy, effective, fingerprint, redactor, ok, err := notificationAuthority(root, runtime, autonomousnotification.EventDaemonFailed)
	if err != nil {
		observeNotification(observer, autonomousnotification.Result{}, err)
		return
	}
	if !ok {
		return
	}
	refs := emptyReferences()
	refs.Daemon = autonomousnotification.Reference{Applicable: true, ID: operationID}
	omissions := []string{"archive reference not applicable", "question reference not applicable", "safety reference not applicable", "task reference not applicable", "task-run reference not applicable", "worker-run reference not applicable"}
	var occurredAt time.Time
	detail := result.StopDetail
	if sourceErr != nil {
		detail = sourceErr.Error()
	}
	sourceIdentity := hashText("daemon", operationID, string(result.StopReason), redactor.String(detail))
	if result.LastQueue.OperationID != "" {
		if op, found, inspectErr := autonomousqueue.Inspect(root, result.LastQueue.OperationID); inspectErr == nil && found && op.CompletedAt != nil {
			occurredAt = *op.CompletedAt
			refs.Queue = autonomousnotification.Reference{Applicable: true, ID: op.OperationID}
			sourceIdentity = hashText("daemon", operationID, op.OperationID, fmt.Sprint(op.Sequence), string(result.StopReason))
		}
	}
	if occurredAt.IsZero() {
		observeNotification(observer, autonomousnotification.Result{}, errors.New("notification adapter: daemon failure lacks durable queue occurrence authority"))
		return
	}
	deliverNotification(ctx, root, effective, fingerprint, redactor, policy, autonomousnotification.EventInput{Event: autonomousnotification.EventDaemonFailed, SourceIdentity: sourceIdentity, OccurredAt: occurredAt, SubjectKind: "daemon", Outcome: "failed", StopReason: string(result.StopReason), Detail: detail, References: refs, Omissions: omissions}, runtime, observer)
}

func notificationAuthority(root string, runtime NotificationRuntime, event autonomousnotification.Event) (autonomousnotification.Policy, runonce.Config, runonce.EffectiveConfigFingerprint, *redact.Redactor, bool, error) {
	cfg, err := LoadRunOnceConfig(root, DefaultRunOnceConfig(root))
	if err != nil {
		return autonomousnotification.Policy{}, runonce.Config{}, runonce.EffectiveConfigFingerprint{}, nil, false, err
	}
	effective, err := runonce.EffectiveConfig(cfg)
	if err != nil {
		return autonomousnotification.Policy{}, runonce.Config{}, runonce.EffectiveConfigFingerprint{}, nil, false, err
	}
	if !effective.NotificationPolicy.Enabled || !effective.NotificationPolicy.Allows(event) {
		return effective.NotificationPolicy, effective, runonce.EffectiveConfigFingerprint{}, nil, false, nil
	}
	fingerprint, err := runonce.FingerprintEffectiveConfig(effective)
	if err != nil {
		return autonomousnotification.Policy{}, runonce.Config{}, runonce.EffectiveConfigFingerprint{}, nil, false, err
	}
	lookup := redact.LookupEnv(os.LookupEnv)
	if runtime.LookupEnv != nil {
		lookup = redact.LookupEnv(runtime.LookupEnv)
	}
	redactor, _, err := redact.New(effective.SafetyDeclaration.Redaction, lookup)
	if err != nil {
		return autonomousnotification.Policy{}, runonce.Config{}, runonce.EffectiveConfigFingerprint{}, nil, false, err
	}
	return effective.NotificationPolicy, effective, fingerprint, redactor, true, nil
}

func deliverNotification(ctx context.Context, root string, effective runonce.Config, fingerprint runonce.EffectiveConfigFingerprint, redactor *redact.Redactor, policy autonomousnotification.Policy, input autonomousnotification.EventInput, runtime NotificationRuntime, observer NotificationObserver) {
	input.RepositoryRoot, input.EffectiveConfigSchema, input.EffectiveConfigSHA256, input.HookPolicy, input.RedactionNames = root, fingerprint.Schema, fingerprint.SHA256, policy, append([]string(nil), effective.SafetyDeclaration.Redaction.EnvironmentVariables...)
	payload, raw, err := autonomousnotification.BuildPayload(input, redactor.String)
	if err != nil {
		observeNotification(observer, autonomousnotification.Result{}, redactor.Error(err))
		return
	}
	if runtime.failureInjector != nil {
		if err := runtime.failureInjector(notificationFailureBeforeDelivery); err != nil {
			observeNotification(observer, autonomousnotification.Result{SchemaVersion: autonomousnotification.ResultSchemaVersion, DeliveryID: payload.DeliveryID, Event: payload.Event}, redactor.Error(err))
			return
		}
	}
	result, deliveryErr := autonomousnotification.Deliver(ctx, autonomousnotification.DeliveryConfig{RepositoryRoot: root, Payload: payload, PayloadBytes: raw, Policy: policy, RedactionNames: input.RedactionNames, Clock: runtime.Clock, Runner: runtime.Runner, LookPath: runtime.LookPath, LookupEnv: runtime.LookupEnv, Wait: runtime.Wait})
	if runtime.failureInjector != nil {
		if err := runtime.failureInjector(notificationFailureAfterDelivery); err != nil {
			deliveryErr = errors.Join(deliveryErr, err)
		}
	}
	observeNotification(observer, result, redactor.Error(deliveryErr))
}

func emptyReferences() autonomousnotification.References { return autonomousnotification.References{} }
func observeNotification(observer NotificationObserver, result autonomousnotification.Result, err error) {
	if observer != nil {
		defer func() { _ = recover() }()
		observer(result, err)
	}
}
