package codexexec

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"revolvr/internal/redact"
	"revolvr/internal/runtimepath"
)

const (
	lastMessageRawSuffix      = ".revolvr-raw.tmp"
	lastMessageRedactedSuffix = ".revolvr-redacted.tmp"
)

type LastMessageFailurePoint string

const (
	LastMessageFailureAfterChild    LastMessageFailurePoint = "after_child_completion"
	LastMessageFailureRead          LastMessageFailurePoint = "read_raw_temporary"
	LastMessageFailureRedact        LastMessageFailurePoint = "redact_raw_temporary"
	LastMessageFailureTempWrite     LastMessageFailurePoint = "write_redacted_temporary"
	LastMessageFailureFileSync      LastMessageFailurePoint = "sync_redacted_temporary"
	LastMessageFailureRename        LastMessageFailurePoint = "rename_canonical"
	LastMessageFailureDirectorySync LastMessageFailurePoint = "sync_parent_directory"
)

type LastMessageFailureInjector func(LastMessageFailurePoint) error

type lastMessageStage struct {
	canonical     string
	raw           string
	redacted      string
	canonicalName string
	rawName       string
	redactedName  string
	directory     *runtimepath.Directory
	rawFile       *runtimepath.File
	redactedFile  *runtimepath.File
	published     bool
	inject        LastMessageFailureInjector
}

func lastMessageRawPath(canonical string) string {
	return filepath.Join(filepath.Dir(canonical), "."+filepath.Base(canonical)+lastMessageRawSuffix)
}

func lastMessageRedactedPath(canonical string) string {
	return filepath.Join(filepath.Dir(canonical), "."+filepath.Base(canonical)+lastMessageRedactedSuffix)
}

func prepareLastMessageStage(canonical string, inject LastMessageFailureInjector) (*lastMessageStage, error) {
	if err := ensureParent(canonical); err != nil {
		return nil, err
	}
	boundary, err := runtimepath.Bind(filepath.Dir(canonical))
	if err != nil {
		return nil, fmt.Errorf("prepare last-message artifact: bind directory: %w", err)
	}
	directory, found, err := boundary.OpenDir(boundary.Root(), false)
	if err != nil || !found {
		if err == nil {
			err = os.ErrNotExist
		}
		return nil, fmt.Errorf("prepare last-message artifact: open directory: %w", err)
	}
	canonical = filepath.Join(boundary.Root(), filepath.Base(canonical))
	stage := &lastMessageStage{
		canonical:     canonical,
		raw:           lastMessageRawPath(canonical),
		redacted:      lastMessageRedactedPath(canonical),
		canonicalName: filepath.Base(canonical),
		rawName:       filepath.Base(lastMessageRawPath(canonical)),
		redactedName:  filepath.Base(lastMessageRedactedPath(canonical)),
		directory:     directory,
		inject:        inject,
	}
	removed := false
	for _, name := range []string{stage.canonicalName, stage.rawName, stage.redactedName} {
		didRemove, err := removeLastMessageFile(directory, name)
		if err != nil {
			_ = directory.Close()
			return nil, fmt.Errorf("prepare last-message artifact: remove %s: %w", filepath.Join(boundary.Root(), name), err)
		}
		removed = removed || didRemove
	}
	if removed {
		if err := directory.Sync(); err != nil {
			_ = directory.Close()
			return nil, fmt.Errorf("prepare last-message artifact: sync directory: %w", err)
		}
	}

	raw, err := directory.OpenFile(stage.rawName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		_ = directory.Close()
		return nil, fmt.Errorf("prepare last-message artifact: create raw temporary: %w", err)
	}
	if err := raw.Chmod(0o600); err != nil {
		stage.rawFile = raw
		return nil, errors.Join(
			fmt.Errorf("prepare last-message artifact: restrict raw temporary: %w", err),
			stage.cleanup(),
		)
	}
	if err := raw.Close(); err != nil {
		return nil, errors.Join(
			fmt.Errorf("prepare last-message artifact: close raw temporary: %w", err),
			stage.cleanup(),
		)
	}
	return stage, nil
}

func (s *lastMessageStage) publish(redactor *redact.Redactor) (message string, facts redact.Facts, err error) {
	defer func() {
		err = errors.Join(err, s.cleanup())
	}()
	if err := s.fail(LastMessageFailureAfterChild); err != nil {
		return "", redact.Facts{}, err
	}
	if err := s.fail(LastMessageFailureRead); err != nil {
		return "", redact.Facts{}, err
	}
	raw, exists, err := s.readRawLastMessage()
	if err != nil {
		return "", redact.Facts{}, err
	}
	if !exists || len(raw) == 0 {
		return "", redact.Facts{}, nil
	}
	if err := s.fail(LastMessageFailureRedact); err != nil {
		return "", redact.Facts{}, err
	}

	published := append([]byte(nil), raw...)
	message = strings.TrimSpace(string(raw))
	if redactor != nil {
		redactedRaw, redaction := redactor.Redact(string(raw))
		facts = redaction
		message = strings.TrimSpace(redactedRaw)
		if message == "" {
			published = []byte(redactedRaw)
		} else {
			published = []byte(message + "\n")
		}
	}

	temporary, err := s.directory.OpenFile(s.redactedName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", redact.Facts{}, fmt.Errorf("publish last-message artifact: create redacted temporary: %w", err)
	}
	s.redactedFile = temporary
	if err := s.fail(LastMessageFailureTempWrite); err != nil {
		return "", redact.Facts{}, err
	}
	if err := writeLastMessageAll(temporary, published); err != nil {
		return "", redact.Facts{}, fmt.Errorf("publish last-message artifact: write redacted temporary: %w", err)
	}
	if err := temporary.Chmod(0o644); err != nil {
		return "", redact.Facts{}, fmt.Errorf("publish last-message artifact: set published mode: %w", err)
	}
	if err := s.fail(LastMessageFailureFileSync); err != nil {
		return "", redact.Facts{}, err
	}
	if err := temporary.Sync(); err != nil {
		return "", redact.Facts{}, fmt.Errorf("publish last-message artifact: sync redacted temporary: %w", err)
	}
	if err := s.fail(LastMessageFailureRename); err != nil {
		return "", redact.Facts{}, err
	}
	replaceErr := s.directory.Replace(temporary, s.canonicalName)
	s.published = temporary.IsNamed(s.canonicalName)
	if replaceErr != nil {
		return "", redact.Facts{}, fmt.Errorf("publish last-message artifact: rename canonical: %w", replaceErr)
	}
	if s.rawFile != nil {
		if err := s.directory.Remove(s.rawFile); err != nil {
			return "", redact.Facts{}, fmt.Errorf("publish last-message artifact: remove raw temporary: %w", err)
		}
		if err := s.rawFile.Close(); err != nil {
			return "", redact.Facts{}, fmt.Errorf("publish last-message artifact: close raw temporary: %w", err)
		}
		s.rawFile = nil
	}
	if err := verifyPublishedLastMessage(temporary); err != nil {
		return "", redact.Facts{}, err
	}
	if err := s.fail(LastMessageFailureDirectorySync); err != nil {
		return "", redact.Facts{}, err
	}
	if err := s.directory.Sync(); err != nil {
		return "", redact.Facts{}, fmt.Errorf("publish last-message artifact: sync directory: %w", err)
	}
	return message, facts, nil
}

func (s *lastMessageStage) readRawLastMessage() ([]byte, bool, error) {
	file, err := s.directory.OpenFile(s.rawName, os.O_RDONLY, 0)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read last-message raw temporary: open: %w", err)
	}
	s.rawFile = file
	mode, err := file.Perm()
	if err != nil {
		return nil, false, fmt.Errorf("read last-message raw temporary: inspect: %w", err)
	}
	if mode&0o077 != 0 {
		return nil, false, fmt.Errorf("read last-message raw temporary: unsafe mode %04o", mode)
	}
	raw, err := file.ReadAll()
	if err != nil {
		return nil, false, fmt.Errorf("read last-message raw temporary: %w", err)
	}
	return raw, true, nil
}

func verifyPublishedLastMessage(file *runtimepath.File) error {
	mode, err := file.Perm()
	if err != nil {
		return fmt.Errorf("verify published last-message artifact: %w", err)
	}
	if mode != 0o644 {
		return fmt.Errorf("verify published last-message artifact: mode is %04o, want 0644", mode)
	}
	return nil
}

func removeLastMessageFile(directory *runtimepath.Directory, name string) (bool, error) {
	file, err := directory.OpenFile(name, os.O_RDONLY, 0)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer file.Close()
	if err := directory.Remove(file); err != nil {
		return false, err
	}
	return true, nil
}

func (s *lastMessageStage) fail(point LastMessageFailurePoint) error {
	if s.inject == nil {
		return nil
	}
	if err := s.inject(point); err != nil {
		return fmt.Errorf("publish last-message artifact at %s: %w", point, err)
	}
	return nil
}

func (s *lastMessageStage) cleanup() error {
	if s == nil || s.directory == nil {
		return nil
	}
	removed := false
	var result error
	if s.rawFile != nil {
		if err := s.directory.Remove(s.rawFile); err != nil {
			result = errors.Join(result, fmt.Errorf("cleanup last-message temporary %s: %w", s.raw, err))
		} else {
			removed = true
		}
		result = errors.Join(result, s.rawFile.Close())
		s.rawFile = nil
	} else {
		didRemove, err := removeLastMessageFile(s.directory, s.rawName)
		removed = removed || didRemove
		if err != nil {
			result = errors.Join(result, fmt.Errorf("cleanup last-message temporary %s: %w", s.raw, err))
		}
	}
	if s.redactedFile != nil {
		if !s.published {
			if err := s.directory.Remove(s.redactedFile); err != nil {
				result = errors.Join(result, fmt.Errorf("cleanup last-message temporary %s: %w", s.redacted, err))
			} else {
				removed = true
			}
		}
		result = errors.Join(result, s.redactedFile.Close())
		s.redactedFile = nil
	}
	if removed {
		if err := s.directory.Sync(); err != nil {
			result = errors.Join(result, fmt.Errorf("cleanup last-message temporaries: sync directory: %w", err))
		}
	}
	result = errors.Join(result, s.directory.Close())
	s.directory = nil
	return result
}

func writeLastMessageAll(writer io.Writer, content []byte) error {
	for len(content) > 0 {
		written, err := writer.Write(content)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		content = content[written:]
	}
	return nil
}
