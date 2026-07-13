package codexexec

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"revolvr/internal/redact"
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
	canonical string
	raw       string
	redacted  string
	directory string
	inject    LastMessageFailureInjector
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
	stage := &lastMessageStage{
		canonical: canonical,
		raw:       lastMessageRawPath(canonical),
		redacted:  lastMessageRedactedPath(canonical),
		directory: filepath.Dir(canonical),
		inject:    inject,
	}
	removed := false
	for _, path := range []string{stage.canonical, stage.raw, stage.redacted} {
		didRemove, err := removeLastMessagePath(path)
		if err != nil {
			return nil, fmt.Errorf("prepare last-message artifact: remove %s: %w", path, err)
		}
		removed = removed || didRemove
	}
	if removed {
		if err := syncLastMessageDirectory(stage.directory); err != nil {
			return nil, fmt.Errorf("prepare last-message artifact: sync directory: %w", err)
		}
	}

	raw, err := os.OpenFile(stage.raw, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("prepare last-message artifact: create raw temporary: %w", err)
	}
	if err := raw.Chmod(0o600); err != nil {
		_ = raw.Close()
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
	raw, exists, err := readRawLastMessage(s.raw)
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

	temporary, err := os.OpenFile(s.redacted, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", redact.Facts{}, fmt.Errorf("publish last-message artifact: create redacted temporary: %w", err)
	}
	closed := false
	defer func() {
		if !closed {
			err = errors.Join(err, temporary.Close())
		}
	}()
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
	if err := temporary.Close(); err != nil {
		closed = true
		return "", redact.Facts{}, fmt.Errorf("publish last-message artifact: close redacted temporary: %w", err)
	}
	closed = true
	if err := s.fail(LastMessageFailureRename); err != nil {
		return "", redact.Facts{}, err
	}
	if err := os.Rename(s.redacted, s.canonical); err != nil {
		return "", redact.Facts{}, fmt.Errorf("publish last-message artifact: rename canonical: %w", err)
	}
	if _, err := removeLastMessagePath(s.raw); err != nil {
		return "", redact.Facts{}, fmt.Errorf("publish last-message artifact: remove raw temporary: %w", err)
	}
	if err := verifyPublishedLastMessage(s.canonical); err != nil {
		return "", redact.Facts{}, err
	}
	if err := s.fail(LastMessageFailureDirectorySync); err != nil {
		return "", redact.Facts{}, err
	}
	if err := syncLastMessageDirectory(s.directory); err != nil {
		return "", redact.Facts{}, fmt.Errorf("publish last-message artifact: sync directory: %w", err)
	}
	return message, facts, nil
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
	removed := false
	var result error
	for _, path := range []string{s.raw, s.redacted} {
		didRemove, err := removeLastMessagePath(path)
		removed = removed || didRemove
		if err != nil {
			result = errors.Join(result, fmt.Errorf("cleanup last-message temporary %s: %w", path, err))
		}
	}
	if removed {
		if err := syncLastMessageDirectory(s.directory); err != nil {
			result = errors.Join(result, fmt.Errorf("cleanup last-message temporaries: sync directory: %w", err))
		}
	}
	return result
}

func readRawLastMessage(path string) ([]byte, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read last-message raw temporary: inspect: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, false, errors.New("read last-message raw temporary: expected regular non-symlink file")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, false, fmt.Errorf("read last-message raw temporary: unsafe mode %04o", info.Mode().Perm())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, false, fmt.Errorf("read last-message raw temporary: %w", err)
	}
	return raw, true, nil
}

func verifyPublishedLastMessage(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("verify published last-message artifact: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("verify published last-message artifact: expected regular non-symlink file")
	}
	if info.Mode().Perm() != 0o644 {
		return fmt.Errorf("verify published last-message artifact: mode is %04o, want 0644", info.Mode().Perm())
	}
	return nil
}

func removeLastMessagePath(path string) (bool, error) {
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if err := os.Remove(path); err != nil {
		return false, err
	}
	return true, nil
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

func syncLastMessageDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
