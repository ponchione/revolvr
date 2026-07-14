package autonomousfinalization

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"revolvr/internal/autonomous"
	"revolvr/internal/runtimepath"
)

type finalizationStorage struct {
	boundary runtimepath.Boundary
	inject   func(FailurePoint) error
}

func bindFinalizationStorage(root string, inject func(FailurePoint) error) (*finalizationStorage, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	boundary, err := runtimepath.Bind(root)
	if err != nil {
		return nil, fmt.Errorf("completion artifact: bind repository root: %w", err)
	}
	return &finalizationStorage{boundary: boundary, inject: inject}, nil
}

func (s *finalizationStorage) root() string { return s.boundary.Root() }

func (s *finalizationStorage) fail(point FailurePoint) error {
	if s.inject == nil {
		return nil
	}
	return s.inject(point)
}

func (s *finalizationStorage) resolve(rel string) (string, error) {
	rel = filepath.Clean(filepath.FromSlash(strings.TrimSpace(rel)))
	if rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("completion artifact: unsafe path %q", filepath.ToSlash(rel))
	}
	return filepath.Join(s.root(), rel), nil
}

func (s *finalizationStorage) openParent(rel string, create bool) (*runtimepath.Directory, string, error) {
	abs, err := s.resolve(rel)
	if err != nil {
		return nil, "", err
	}
	if err := s.fail(FailureBeforeArtifactDirectoryOpen); err != nil {
		return nil, "", err
	}
	parent := filepath.Dir(abs)
	if create {
		if err := s.boundary.EnsureDir(parent, 0o755); err != nil {
			return nil, "", err
		}
	}
	directory, found, err := s.boundary.OpenDir(parent, false)
	if err != nil {
		return nil, "", err
	}
	if !found {
		return nil, "", os.ErrNotExist
	}
	if err := s.fail(FailureAfterArtifactDirectoryOpen); err != nil {
		_ = directory.Close()
		return nil, "", err
	}
	if err := directory.Check(); err != nil {
		_ = directory.Close()
		return nil, "", err
	}
	return directory, filepath.Base(abs), nil
}

func (s *finalizationStorage) readRegular(rel string, missingOK bool) ([]byte, bool, error) {
	directory, name, err := s.openParent(rel, false)
	if errors.Is(err, os.ErrNotExist) && missingOK {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer directory.Close()
	return s.readDirectoryFile(directory, name, missingOK)
}

func (s *finalizationStorage) readDirectoryFile(directory *runtimepath.Directory, name string, missingOK bool) ([]byte, bool, error) {
	file, err := directory.OpenFile(name, os.O_RDONLY, 0)
	if errors.Is(err, os.ErrNotExist) && missingOK {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer file.Close()
	if err := s.fail(FailureAfterArtifactReadOpen); err != nil {
		return nil, false, err
	}
	if err := file.Check(); err != nil {
		return nil, false, err
	}
	if err := directory.Check(); err != nil {
		return nil, false, err
	}
	raw, err := file.ReadAll()
	if err != nil {
		return nil, false, err
	}
	if err := directory.Check(); err != nil {
		return nil, false, err
	}
	return raw, true, nil
}

func (s *finalizationStorage) verifyArtifact(id autonomous.FinalizationArtifact, want []byte) error {
	raw, found, err := s.readRegular(id.Path, false)
	if err != nil {
		return err
	}
	if !found {
		return os.ErrNotExist
	}
	if got := artifact(id.Path, raw); got != id || !bytes.Equal(raw, want) {
		return errors.New("completion artifact readback identity mismatch")
	}
	return nil
}

func (s *finalizationStorage) writeImmutable(id autonomous.FinalizationArtifact, raw []byte) (err error) {
	if got := artifact(id.Path, raw); got != id {
		return errors.New("completion artifact identity does not match supplied bytes")
	}
	if existing, found, readErr := s.readRegular(id.Path, true); readErr != nil {
		return readErr
	} else if found {
		if bytes.Equal(existing, raw) {
			return nil
		}
		return fmt.Errorf("completion artifact %q already exists with different bytes", id.Path)
	}
	directory, name, err := s.openParent(id.Path, true)
	if err != nil {
		return err
	}
	defer directory.Close()
	if existing, found, readErr := s.readDirectoryFile(directory, name, true); readErr != nil {
		return readErr
	} else if found {
		if bytes.Equal(existing, raw) {
			return nil
		}
		return fmt.Errorf("completion artifact %q already exists with different bytes", id.Path)
	}
	temp, err := directory.CreateTemp(".completion.tmp-", 0o644)
	if err != nil {
		return err
	}
	published := false
	defer func() {
		if !published {
			err = errors.Join(err, s.cleanupTemp(directory, temp))
		}
		err = errors.Join(err, temp.Close())
	}()
	if err := s.fail(FailureAfterArtifactTemporaryOpen); err != nil {
		return err
	}
	if err := temp.Check(); err != nil {
		return err
	}
	if err := directory.Check(); err != nil {
		return err
	}
	if _, err := temp.Write(raw); err != nil {
		return err
	}
	if err := s.fail(FailureBeforeArtifactFileSync); err != nil {
		return err
	}
	if err := temp.Check(); err != nil {
		return err
	}
	if err := directory.Check(); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := s.fail(FailureBeforeArtifactPublish); err != nil {
		return err
	}
	if err := temp.Check(); err != nil {
		return err
	}
	if err := directory.Check(); err != nil {
		return err
	}
	linkErr := directory.Link(temp, name)
	published = temp.IsNamed(name)
	if linkErr != nil {
		if errors.Is(linkErr, os.ErrExist) {
			existing, found, readErr := s.readDirectoryFile(directory, name, false)
			if readErr == nil && found && bytes.Equal(existing, raw) {
				return nil
			}
			if readErr != nil {
				return readErr
			}
		}
		return linkErr
	}
	if err := s.fail(FailureAfterArtifactPublish); err != nil {
		return err
	}
	if err := temp.Check(); err != nil {
		return err
	}
	if err := directory.Check(); err != nil {
		return err
	}
	if err := s.fail(FailureBeforeArtifactDirectorySync); err != nil {
		return err
	}
	if err := directory.Check(); err != nil {
		return err
	}
	if err := directory.Sync(); err != nil {
		return err
	}
	if err := s.fail(FailureBeforeArtifactReadback); err != nil {
		return err
	}
	if err := directory.Check(); err != nil {
		return err
	}
	observed, found, err := s.readDirectoryFile(directory, name, false)
	if err != nil {
		return err
	}
	if !found || artifact(id.Path, observed) != id || !bytes.Equal(observed, raw) {
		return errors.New("completion artifact readback identity mismatch")
	}
	return directory.Check()
}

func (s *finalizationStorage) cleanupTemp(directory *runtimepath.Directory, temp *runtimepath.File) error {
	faultErr := s.fail(FailureBeforeArtifactCleanup)
	if err := temp.Check(); err != nil {
		return errors.Join(faultErr, err)
	}
	if err := directory.Check(); err != nil {
		return errors.Join(faultErr, err)
	}
	if err := directory.Remove(temp); err != nil {
		return errors.Join(faultErr, err)
	}
	if err := directory.Check(); err != nil {
		return errors.Join(faultErr, err)
	}
	return errors.Join(faultErr, directory.Sync())
}
