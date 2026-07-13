package autonomousnotification

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"syscall"
	"time"
)

type persistencePoint string

const (
	persistenceHistoryWrite   persistencePoint = "history_write"
	persistenceJournalReplace persistencePoint = "journal_replace"
	persistenceFileSync       persistencePoint = "file_sync"
	persistenceDirectorySync  persistencePoint = "directory_sync"
)

type persistenceFault func(persistencePoint, string) error

func deliveryDir(root, id string) string {
	return filepath.Join(root, ".revolvr", "autonomous", "notifications", id)
}

func admit(dir string, intent Intent, payload []byte, now time.Time) (Journal, bool, error) {
	return admitWithFault(dir, intent, payload, now, nil)
}

func admitWithFault(dir string, intent Intent, payload []byte, now time.Time, fault persistenceFault) (Journal, bool, error) {
	intentRaw, _ := canonical(intent)
	if err := publishExact(filepath.Join(dir, "intent.json"), intentRaw); err != nil {
		return Journal{}, false, err
	}
	if err := publishExact(filepath.Join(dir, "payload.json"), payload); err != nil {
		return Journal{}, false, err
	}
	journal, found, err := inspectDir(dir)
	if err != nil {
		return Journal{}, false, err
	}
	if found {
		if journal.DeliveryID != intent.DeliveryID {
			return Journal{}, false, errors.New("notification delivery: journal identity conflict")
		}
		return journal, true, nil
	}
	journal = Journal{SchemaVersion: JournalSchemaVersion, DeliveryID: intent.DeliveryID, Sequence: 1, Stage: StageAdmitted, Detail: "intent admitted", UpdatedAt: now}
	journal, err = persistTransitionWithFault(dir, Journal{}, journal, nil, fault)
	return journal, false, err
}

func transition(dir string, current Journal, stage Stage, detail string, attempt *Attempt, now time.Time) (Journal, error) {
	return transitionWithFault(dir, current, stage, detail, attempt, now, nil)
}

func transitionWithFault(dir string, current Journal, stage Stage, detail string, attempt *Attempt, now time.Time, fault persistenceFault) (Journal, error) {
	next := current
	next.Sequence++
	next.Stage = stage
	next.Detail = strings.TrimSpace(detail)
	next.UpdatedAt = now
	if attempt != nil {
		next.Attempts = append(append([]Attempt(nil), current.Attempts...), *attempt)
	}
	return persistTransitionWithFault(dir, current, next, attempt, fault)
}

func persistTransition(dir string, prior, next Journal, attempt *Attempt) (Journal, error) {
	return persistTransitionWithFault(dir, prior, next, attempt, nil)
}

func persistTransitionWithFault(dir string, prior, next Journal, attempt *Attempt, fault persistenceFault) (Journal, error) {
	if err := validateJournal(next); err != nil {
		return prior, err
	}
	history := Transition{SchemaVersion: HistorySchemaVersion, DeliveryID: next.DeliveryID, Sequence: next.Sequence, Stage: next.Stage, Detail: next.Detail, CreatedAt: next.UpdatedAt}
	if attempt != nil {
		clone := *attempt
		history.Attempt = &clone
	}
	historyRaw, _ := canonical(history)
	historyDir := filepath.Join(dir, "history")
	if err := os.MkdirAll(historyDir, 0o700); err != nil {
		return prior, err
	}
	historyPath := filepath.Join(historyDir, fmt.Sprintf("%020d-%s.json", next.Sequence, next.Stage))
	if err := publishExactWithFault(historyPath, historyRaw, fault); err != nil {
		return reconcileTransitionFailure(dir, prior, next, err)
	}
	journalRaw, _ := canonical(next)
	if err := replaceFileWithFault(filepath.Join(dir, "journal.json"), journalRaw, fault); err != nil {
		return reconcileTransitionFailure(dir, prior, next, err)
	}
	observed, found, err := inspectDir(dir)
	if err != nil || !found || observed.Sequence != next.Sequence || observed.Stage != next.Stage {
		return reconcileTransitionFailure(dir, prior, next, errors.Join(err, errors.New("notification delivery: strict journal readback failed")))
	}
	return observed, nil
}

func reconcileTransitionFailure(dir string, prior, next Journal, persistErr error) (Journal, error) {
	observed, found, inspectErr := inspectDir(dir)
	if inspectErr == nil && found {
		switch {
		case prior.Sequence > 0 && reflect.DeepEqual(observed, prior):
			return observed, persistErr
		case observed.DeliveryID == next.DeliveryID && observed.Sequence == next.Sequence && observed.Stage == next.Stage:
			return observed, persistErr
		default:
			inspectErr = errors.New("notification delivery: transition failure reconciliation found unexpected authority")
		}
	}
	return prior, errors.Join(persistErr, inspectErr)
}

func Inspect(repositoryRoot, deliveryID string) (Intent, Payload, Journal, bool, error) {
	if !safeID(deliveryID) {
		return Intent{}, Payload{}, Journal{}, false, errors.New("notification delivery: invalid delivery ID")
	}
	root, err := filepath.Abs(repositoryRoot)
	if err != nil {
		return Intent{}, Payload{}, Journal{}, false, err
	}
	dir := deliveryDir(root, deliveryID)
	if err := ensureSafeExistingParents(root, dir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Intent{}, Payload{}, Journal{}, false, nil
		}
		return Intent{}, Payload{}, Journal{}, false, err
	}
	journal, found, err := inspectDir(dir)
	if err != nil || !found {
		return Intent{}, Payload{}, Journal{}, found, err
	}
	intentRaw, err := readRegular(filepath.Join(dir, "intent.json"), 1<<20)
	if err != nil {
		return Intent{}, Payload{}, Journal{}, false, err
	}
	var intent Intent
	if err := decodeCanonical(intentRaw, &intent); err != nil || intent.SchemaVersion != IntentSchemaVersion || intent.DeliveryID != deliveryID {
		return Intent{}, Payload{}, Journal{}, false, errors.Join(err, errors.New("notification delivery: invalid intent"))
	}
	payloadRaw, err := readRegular(filepath.Join(dir, "payload.json"), 1<<20)
	if err != nil {
		return Intent{}, Payload{}, Journal{}, false, err
	}
	payload, err := DecodePayload(payloadRaw)
	if err != nil || payload.DeliveryID != deliveryID || hash(payloadRaw) != intent.PayloadSHA256 || len(payloadRaw) != intent.PayloadSize {
		return Intent{}, Payload{}, Journal{}, false, errors.Join(err, errors.New("notification delivery: payload identity conflict"))
	}
	return intent, payload, journal, true, nil
}

type Summary struct {
	DeliveryID string    `json:"delivery_id"`
	Event      Event     `json:"event"`
	Stage      Stage     `json:"stage"`
	Attempts   int       `json:"attempts"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func List(repositoryRoot string) ([]Summary, error) {
	root, err := filepath.Abs(repositoryRoot)
	if err != nil {
		return nil, err
	}
	base := filepath.Join(root, ".revolvr", "autonomous", "notifications")
	if err := ensureSafeExistingParents(root, base); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	entries, err := os.ReadDir(base)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	result := make([]Summary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !safeID(entry.Name()) {
			return nil, errors.New("notification delivery: foreign notification entry")
		}
		_, payload, journal, found, readErr := Inspect(root, entry.Name())
		if readErr != nil || !found {
			return nil, readErr
		}
		result = append(result, Summary{DeliveryID: entry.Name(), Event: payload.Event, Stage: journal.Stage, Attempts: len(journal.Attempts), UpdatedAt: journal.UpdatedAt})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].DeliveryID < result[j].DeliveryID })
	return result, nil
}

func inspectDir(dir string) (Journal, bool, error) {
	raw, err := readRegular(filepath.Join(dir, "journal.json"), 4<<20)
	journalFound := !errors.Is(err, os.ErrNotExist)
	if err != nil && journalFound {
		return Journal{}, false, err
	}
	var journal Journal
	if journalFound {
		if err := decodeCanonical(raw, &journal); err != nil {
			return Journal{}, false, err
		}
		if err := validateJournal(journal); err != nil {
			return Journal{}, false, err
		}
	}
	history, historyFound, err := journalFromHistory(filepath.Join(dir, "history"))
	if err != nil {
		return Journal{}, false, err
	}
	if !journalFound && !historyFound {
		return Journal{}, false, nil
	}
	if journalFound && !historyFound {
		return Journal{}, false, errors.New("notification delivery: journal exists without immutable history")
	}
	if journalFound {
		if journal.Sequence > history.Sequence {
			return Journal{}, false, errors.New("notification delivery: journal is ahead of immutable history")
		}
		if journal.Sequence == history.Sequence && !reflect.DeepEqual(journal, history) {
			return Journal{}, false, errors.New("notification delivery: journal/history conflict")
		}
	}
	return history, true, nil
}

func journalFromHistory(historyDir string) (Journal, bool, error) {
	entries, err := os.ReadDir(historyDir)
	if errors.Is(err, os.ErrNotExist) {
		return Journal{}, false, nil
	}
	if err != nil {
		return Journal{}, false, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var result Journal
	for i, entry := range entries {
		if entry.IsDir() {
			return Journal{}, false, errors.New("notification delivery: foreign history entry")
		}
		raw, readErr := readRegular(filepath.Join(historyDir, entry.Name()), 1<<20)
		if readErr != nil {
			return Journal{}, false, readErr
		}
		var transition Transition
		if err := decodeCanonical(raw, &transition); err != nil {
			return Journal{}, false, err
		}
		if transition.SchemaVersion != HistorySchemaVersion || !safeID(transition.DeliveryID) || transition.Sequence != int64(i+1) || transition.CreatedAt.IsZero() || entry.Name() != fmt.Sprintf("%020d-%s.json", transition.Sequence, transition.Stage) {
			return Journal{}, false, errors.New("notification delivery: invalid or noncanonical history")
		}
		if i > 0 && transition.DeliveryID != result.DeliveryID {
			return Journal{}, false, errors.New("notification delivery: divergent history identity")
		}
		result.SchemaVersion, result.DeliveryID, result.Sequence, result.Stage, result.Detail, result.UpdatedAt = JournalSchemaVersion, transition.DeliveryID, transition.Sequence, transition.Stage, transition.Detail, transition.CreatedAt
		if transition.Attempt != nil {
			result.Attempts = append(result.Attempts, *transition.Attempt)
		}
		if err := validateJournal(result); err != nil {
			return Journal{}, false, err
		}
	}
	return result, len(entries) > 0, nil
}

func validateJournal(j Journal) error {
	if j.SchemaVersion != JournalSchemaVersion || !safeID(j.DeliveryID) || j.Sequence <= 0 || j.UpdatedAt.IsZero() {
		return errors.New("notification delivery: invalid journal identity")
	}
	switch j.Stage {
	case StageAdmitted, StageRunning, StageRetryable, StageSucceeded, StageFailed, StageResumable:
	default:
		return errors.New("notification delivery: invalid journal stage")
	}
	for i, attempt := range j.Attempts {
		if attempt.Number != i+1 || attempt.StartedAt.IsZero() || attempt.CompletedAt.Before(attempt.StartedAt) {
			return errors.New("notification delivery: invalid attempt history")
		}
	}
	return nil
}

func decodeCanonical(raw []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return errors.New("multiple JSON values")
	} else if !errors.Is(err, io.EOF) {
		return err
	}
	canonicalRaw, err := canonical(value)
	if err != nil {
		return err
	}
	if !bytes.Equal(raw, canonicalRaw) {
		return errors.New("non-canonical JSON")
	}
	return nil
}

func readRegular(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 || info.Sys().(*syscall.Stat_t).Nlink != 1 || info.Size() > limit {
		return nil, errors.New("notification delivery: unsafe evidence file")
	}
	return os.ReadFile(path)
}

func publishExact(path string, raw []byte) error {
	return publishExactWithFault(path, raw, nil)
}

func publishExactWithFault(path string, raw []byte, fault persistenceFault) error {
	if prior, err := os.ReadFile(path); err == nil {
		if bytes.Equal(prior, raw) {
			return nil
		}
		return errors.New("notification delivery: immutable content conflict")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if err = injectPersistenceFault(fault, persistenceHistoryWrite, path); err == nil {
		_, err = file.Write(raw)
	}
	if err == nil {
		err = injectPersistenceFault(fault, persistenceFileSync, path)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	err = errors.Join(err, closeErr)
	if err != nil {
		removeErr := os.Remove(path)
		syncErr := syncDir(filepath.Dir(path))
		return errors.Join(err, removeErr, syncErr)
	}
	if err = injectPersistenceFault(fault, persistenceDirectorySync, filepath.Dir(path)); err == nil {
		err = syncDir(filepath.Dir(path))
	}
	return err
}

func replaceFile(path string, raw []byte) error {
	return replaceFileWithFault(path, raw, nil)
}

func replaceFileWithFault(path string, raw []byte, fault persistenceFault) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".journal-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(raw)
	}
	if err == nil {
		err = injectPersistenceFault(fault, persistenceFileSync, path)
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	err = errors.Join(err, closeErr)
	if err == nil {
		err = injectPersistenceFault(fault, persistenceJournalReplace, path)
	}
	if err == nil {
		err = os.Rename(name, path)
	}
	if err == nil {
		err = injectPersistenceFault(fault, persistenceDirectorySync, filepath.Dir(path))
	}
	if err == nil {
		err = syncDir(filepath.Dir(path))
	}
	return err
}

func injectPersistenceFault(fault persistenceFault, point persistencePoint, path string) error {
	if fault == nil {
		return nil
	}
	return fault(point, path)
}
func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func ensureSafeDirectory(root, target string) error {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("notification delivery: path escapes repository")
	}
	current := root
	if info, statErr := os.Lstat(current); statErr != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.Join(statErr, errors.New("notification delivery: repository root is unsafe"))
	}
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, os.ErrNotExist) {
			if err := os.Mkdir(current, 0o700); err != nil {
				return err
			}
			continue
		}
		if statErr != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.Join(statErr, errors.New("notification delivery: unsafe directory or symlink"))
		}
	}
	return nil
}

func ensureSafeExistingParents(root, target string) error {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("notification delivery: path escapes repository")
	}
	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if statErr != nil {
			return statErr
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("notification delivery: unsafe directory or symlink")
		}
	}
	return nil
}
