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
	"time"

	"revolvr/internal/lock"
	"revolvr/internal/runtimepath"
)

type persistencePoint string

const (
	persistenceBeforeOpen        persistencePoint = "before_open"
	persistenceAfterOpen         persistencePoint = "after_open"
	persistenceHistoryWrite      persistencePoint = "history_write"
	persistenceFileSync          persistencePoint = "file_sync"
	persistenceBeforePublication persistencePoint = "before_publication"
	persistenceJournalReplace    persistencePoint = "journal_replace"
	persistenceAfterPublication  persistencePoint = "after_publication"
	persistenceDirectorySync     persistencePoint = "directory_sync"
	persistenceCleanup           persistencePoint = "cleanup"
)

type persistenceFault func(persistencePoint, string) error

type deliveryStore struct {
	directory *runtimepath.Directory
	lease     *lock.Flock
	path      string
	fault     persistenceFault
}

func deliveryDir(root, id string) string {
	return filepath.Join(root, ".revolvr", "autonomous", "notifications", id)
}

func openDeliveryStore(boundary runtimepath.Boundary, deliveryID string, lease *lock.Flock, fault persistenceFault) (*deliveryStore, bool, error) {
	if !safeID(deliveryID) {
		return nil, false, errors.New("notification delivery: invalid delivery ID")
	}
	path := deliveryDir(boundary.Root(), deliveryID)
	directory, found, err := boundary.OpenDir(path, true)
	if err != nil || !found {
		return nil, found, err
	}
	store := &deliveryStore{directory: directory, lease: lease, path: path, fault: fault}
	if err := store.check(); err != nil {
		_ = directory.Close()
		return nil, false, err
	}
	return store, true, nil
}

func (s *deliveryStore) Close() error {
	if s == nil || s.directory == nil {
		return nil
	}
	return s.directory.Close()
}

func (s *deliveryStore) check() error {
	if s == nil || s.directory == nil {
		return errors.New("notification delivery: store is closed")
	}
	if err := s.directory.Check(); err != nil {
		return err
	}
	if s.lease != nil {
		if err := s.lease.Check(); err != nil {
			return fmt.Errorf("notification delivery: validate delivery lease: %w", err)
		}
	}
	return nil
}

func (s *deliveryStore) requireMutation() error {
	if s.lease == nil {
		return errors.New("notification delivery: mutation requires the delivery lease")
	}
	return s.check()
}

func (s *deliveryStore) admit(intent Intent, payload []byte, now time.Time) (Journal, bool, error) {
	intentRaw, _ := canonical(intent)
	if err := s.publishExact(s.directory, "intent.json", intentRaw, false); err != nil {
		return Journal{}, false, err
	}
	if err := s.publishExact(s.directory, "payload.json", payload, false); err != nil {
		return Journal{}, false, err
	}
	journal, found, err := s.inspect()
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
	journal, err = s.persistTransition(Journal{}, journal, nil)
	return journal, false, err
}

func (s *deliveryStore) transition(current Journal, stage Stage, detail string, attempt *Attempt, now time.Time) (Journal, error) {
	next := current
	next.Sequence++
	next.Stage = stage
	next.Detail = strings.TrimSpace(detail)
	next.UpdatedAt = now
	if attempt != nil {
		next.Attempts = append(append([]Attempt(nil), current.Attempts...), *attempt)
	}
	return s.persistTransition(current, next, attempt)
}

func (s *deliveryStore) persistTransition(prior, next Journal, attempt *Attempt) (Journal, error) {
	if err := validateJournal(next); err != nil {
		return prior, err
	}
	history := Transition{SchemaVersion: HistorySchemaVersion, DeliveryID: next.DeliveryID, Sequence: next.Sequence, Stage: next.Stage, Detail: next.Detail, CreatedAt: next.UpdatedAt}
	if attempt != nil {
		clone := *attempt
		history.Attempt = &clone
	}
	historyRaw, _ := canonical(history)
	historyDir, err := s.ensureHistoryDir()
	if err != nil {
		return prior, err
	}
	defer historyDir.Close()
	historyName := fmt.Sprintf("%020d-%s.json", next.Sequence, next.Stage)
	if err := s.publishExact(historyDir, historyName, historyRaw, true); err != nil {
		return s.reconcileTransitionFailure(prior, next, err)
	}
	journalRaw, _ := canonical(next)
	if err := s.replaceJournal(journalRaw); err != nil {
		return s.reconcileTransitionFailure(prior, next, err)
	}
	if err := s.check(); err != nil {
		return s.reconcileTransitionFailure(prior, next, err)
	}
	observed, found, err := s.inspect()
	if err != nil || !found || observed.Sequence != next.Sequence || observed.Stage != next.Stage {
		return s.reconcileTransitionFailure(prior, next, errors.Join(err, errors.New("notification delivery: strict journal readback failed")))
	}
	if err := s.check(); err != nil {
		return s.reconcileTransitionFailure(prior, next, err)
	}
	return observed, nil
}

func (s *deliveryStore) reconcileTransitionFailure(prior, next Journal, persistErr error) (Journal, error) {
	observed, found, inspectErr := s.inspect()
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
	boundary, err := runtimepath.Bind(repositoryRoot)
	if err != nil {
		return Intent{}, Payload{}, Journal{}, false, err
	}
	store, found, err := openDeliveryStore(boundary, deliveryID, nil, nil)
	if err != nil || !found {
		return Intent{}, Payload{}, Journal{}, found, err
	}
	defer store.Close()
	intent, payload, journal, err := store.inspectEvidence(deliveryID)
	return intent, payload, journal, err == nil, err
}

type Summary struct {
	DeliveryID string    `json:"delivery_id"`
	Event      Event     `json:"event"`
	Stage      Stage     `json:"stage"`
	Attempts   int       `json:"attempts"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func List(repositoryRoot string) ([]Summary, error) {
	boundary, err := runtimepath.Bind(repositoryRoot)
	if err != nil {
		return nil, err
	}
	basePath := filepath.Join(boundary.Root(), ".revolvr", "autonomous", "notifications")
	base, found, err := boundary.OpenDir(basePath, true)
	if err != nil || !found {
		return nil, err
	}
	defer base.Close()
	entries, err := base.ReadDir()
	if err != nil {
		return nil, err
	}
	result := make([]Summary, 0, len(entries))
	for _, entry := range entries {
		if !safeID(entry.Name()) {
			return nil, errors.New("notification delivery: foreign notification entry")
		}
		directory, childFound, err := base.OpenDir(entry.Name(), false)
		if err != nil || !childFound {
			return nil, errors.Join(err, errors.New("notification delivery: foreign notification entry"))
		}
		store := &deliveryStore{directory: directory, path: filepath.Join(basePath, entry.Name())}
		_, payload, journal, readErr := store.inspectEvidence(entry.Name())
		closeErr := store.Close()
		if readErr != nil || closeErr != nil {
			return nil, errors.Join(readErr, closeErr)
		}
		result = append(result, Summary{DeliveryID: entry.Name(), Event: payload.Event, Stage: journal.Stage, Attempts: len(journal.Attempts), UpdatedAt: journal.UpdatedAt})
	}
	if err := base.Check(); err != nil {
		return nil, err
	}
	sort.Slice(result, func(i, j int) bool { return result[i].DeliveryID < result[j].DeliveryID })
	return result, nil
}

func (s *deliveryStore) inspectEvidence(deliveryID string) (Intent, Payload, Journal, error) {
	journal, found, err := s.inspect()
	if err != nil {
		return Intent{}, Payload{}, Journal{}, err
	}
	if !found {
		return Intent{}, Payload{}, Journal{}, errors.New("notification delivery: durable journal is missing")
	}
	intentRaw, err := s.readFile(s.directory, "intent.json", 1<<20, false)
	if err != nil {
		return Intent{}, Payload{}, Journal{}, err
	}
	var intent Intent
	if err := decodeCanonical(intentRaw, &intent); err != nil || intent.SchemaVersion != IntentSchemaVersion || intent.DeliveryID != deliveryID {
		return Intent{}, Payload{}, Journal{}, errors.Join(err, errors.New("notification delivery: invalid intent"))
	}
	payloadRaw, err := s.readFile(s.directory, "payload.json", 1<<20, false)
	if err != nil {
		return Intent{}, Payload{}, Journal{}, err
	}
	payload, err := DecodePayload(payloadRaw)
	if err != nil || payload.DeliveryID != deliveryID || hash(payloadRaw) != intent.PayloadSHA256 || len(payloadRaw) != intent.PayloadSize {
		return Intent{}, Payload{}, Journal{}, errors.Join(err, errors.New("notification delivery: payload identity conflict"))
	}
	if err := s.check(); err != nil {
		return Intent{}, Payload{}, Journal{}, err
	}
	return intent, payload, journal, nil
}

func (s *deliveryStore) inspect() (Journal, bool, error) {
	if err := s.check(); err != nil {
		return Journal{}, false, err
	}
	raw, err := s.readFile(s.directory, "journal.json", 4<<20, true)
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
	history, historyFound, err := s.journalFromHistory()
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
	if err := s.check(); err != nil {
		return Journal{}, false, err
	}
	return history, true, nil
}

func (s *deliveryStore) journalFromHistory() (Journal, bool, error) {
	historyDir, found, err := s.directory.OpenDir("history", true)
	if err != nil || !found {
		return Journal{}, false, err
	}
	defer historyDir.Close()
	entries, err := historyDir.ReadDir()
	if err != nil {
		return Journal{}, false, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var result Journal
	for i, entry := range entries {
		if entry.IsDir() {
			return Journal{}, false, errors.New("notification delivery: foreign history entry")
		}
		raw, readErr := s.readFile(historyDir, entry.Name(), 1<<20, false)
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
	if err := historyDir.Check(); err != nil {
		return Journal{}, false, err
	}
	return result, len(entries) > 0, nil
}

func (s *deliveryStore) ensureHistoryDir() (*runtimepath.Directory, error) {
	if err := s.requireMutation(); err != nil {
		return nil, err
	}
	historyDir, err := s.directory.EnsureDir("history", 0o700)
	if err != nil {
		return nil, err
	}
	if err := s.requireMutation(); err != nil {
		_ = historyDir.Close()
		return nil, err
	}
	return historyDir, nil
}

func (s *deliveryStore) readFile(directory *runtimepath.Directory, name string, limit int, missingOK bool) ([]byte, error) {
	if err := s.check(); err != nil {
		return nil, err
	}
	raw, found, err := directory.ReadFile(name, missingOK)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, os.ErrNotExist
	}
	if len(raw) > limit {
		return nil, errors.New("notification delivery: evidence file exceeds size limit")
	}
	if err := s.check(); err != nil {
		return nil, err
	}
	return raw, nil
}

func (s *deliveryStore) publishExact(directory *runtimepath.Directory, name string, raw []byte, transitionHistory bool) (err error) {
	path := filepath.Join(s.path, name)
	if transitionHistory {
		path = filepath.Join(s.path, "history", name)
	}
	if prior, readErr := s.readFile(directory, name, max(len(raw), 1<<20), true); readErr == nil {
		if bytes.Equal(prior, raw) {
			return nil
		}
		return errors.New("notification delivery: immutable content conflict")
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	if err := injectPersistenceFault(s.fault, persistenceBeforeOpen, path); err != nil {
		return err
	}
	if err := s.requireMutation(); err != nil {
		return err
	}
	temp, err := directory.CreateTemp(".immutable-", 0o600)
	if err != nil {
		return err
	}
	published := false
	defer func() {
		if !published {
			err = errors.Join(err, s.cleanupTemp(directory, temp, path))
		}
		err = errors.Join(err, temp.Close())
	}()
	if err := injectPersistenceFault(s.fault, persistenceAfterOpen, path); err != nil {
		return err
	}
	if err := temp.Check(); err != nil {
		return err
	}
	if err := s.requireMutation(); err != nil {
		return err
	}
	if transitionHistory {
		if err := injectPersistenceFault(s.fault, persistenceHistoryWrite, path); err != nil {
			return err
		}
	}
	if err := temp.Check(); err != nil {
		return err
	}
	if err := s.requireMutation(); err != nil {
		return err
	}
	if _, err := temp.Write(raw); err != nil {
		return err
	}
	if err := injectPersistenceFault(s.fault, persistenceFileSync, path); err != nil {
		return err
	}
	if err := temp.Check(); err != nil {
		return err
	}
	if err := s.requireMutation(); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := injectPersistenceFault(s.fault, persistenceBeforePublication, path); err != nil {
		return err
	}
	if err := temp.Check(); err != nil {
		return err
	}
	if err := s.requireMutation(); err != nil {
		return err
	}
	linkErr := directory.Link(temp, name)
	published = temp.IsNamed(name)
	if linkErr != nil {
		return linkErr
	}
	if err := injectPersistenceFault(s.fault, persistenceAfterPublication, path); err != nil {
		return err
	}
	if err := temp.Check(); err != nil {
		return err
	}
	if err := s.requireMutation(); err != nil {
		return err
	}
	if err := injectPersistenceFault(s.fault, persistenceDirectorySync, filepath.Dir(path)); err != nil {
		return err
	}
	if err := s.requireMutation(); err != nil {
		return err
	}
	if err := directory.Sync(); err != nil {
		return err
	}
	return s.requireMutation()
}

func (s *deliveryStore) replaceJournal(raw []byte) (err error) {
	const name = "journal.json"
	path := filepath.Join(s.path, name)
	if err := injectPersistenceFault(s.fault, persistenceBeforeOpen, path); err != nil {
		return err
	}
	if err := s.requireMutation(); err != nil {
		return err
	}
	temp, err := s.directory.CreateTemp(".journal-", 0o600)
	if err != nil {
		return err
	}
	published := false
	defer func() {
		if !published {
			err = errors.Join(err, s.cleanupTemp(s.directory, temp, path))
		}
		err = errors.Join(err, temp.Close())
	}()
	if err := injectPersistenceFault(s.fault, persistenceAfterOpen, path); err != nil {
		return err
	}
	if err := temp.Check(); err != nil {
		return err
	}
	if err := s.requireMutation(); err != nil {
		return err
	}
	if _, err := temp.Write(raw); err != nil {
		return err
	}
	if err := injectPersistenceFault(s.fault, persistenceFileSync, path); err != nil {
		return err
	}
	if err := temp.Check(); err != nil {
		return err
	}
	if err := s.requireMutation(); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := injectPersistenceFault(s.fault, persistenceJournalReplace, path); err != nil {
		return err
	}
	if err := injectPersistenceFault(s.fault, persistenceBeforePublication, path); err != nil {
		return err
	}
	if err := temp.Check(); err != nil {
		return err
	}
	if err := s.requireMutation(); err != nil {
		return err
	}
	replaceErr := s.directory.Replace(temp, name)
	published = temp.IsNamed(name)
	if replaceErr != nil {
		return replaceErr
	}
	if err := injectPersistenceFault(s.fault, persistenceAfterPublication, path); err != nil {
		return err
	}
	if err := temp.Check(); err != nil {
		return err
	}
	if err := s.requireMutation(); err != nil {
		return err
	}
	if err := injectPersistenceFault(s.fault, persistenceDirectorySync, s.path); err != nil {
		return err
	}
	if err := s.requireMutation(); err != nil {
		return err
	}
	if err := s.directory.Sync(); err != nil {
		return err
	}
	return s.requireMutation()
}

func (s *deliveryStore) cleanupTemp(directory *runtimepath.Directory, temp *runtimepath.File, path string) error {
	faultErr := injectPersistenceFault(s.fault, persistenceCleanup, path)
	if err := temp.Check(); err != nil {
		return errors.Join(faultErr, err)
	}
	if err := s.requireMutation(); err != nil {
		return errors.Join(faultErr, err)
	}
	if err := directory.Remove(temp); err != nil {
		return errors.Join(faultErr, err)
	}
	if err := s.requireMutation(); err != nil {
		return errors.Join(faultErr, err)
	}
	return errors.Join(faultErr, directory.Sync())
}

func injectPersistenceFault(fault persistenceFault, point persistencePoint, path string) error {
	if fault == nil {
		return nil
	}
	return fault(point, path)
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
