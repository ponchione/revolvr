package implementer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"revolvr/internal/runtimepath"
	"revolvr/internal/tool"
)

type runStore struct {
	root runtimepath.Boundary
}

func newRunStore(root string) (*runStore, error) {
	boundary, err := runtimepath.Bind(root)
	if err != nil {
		return nil, fmt.Errorf("implementer evidence store: %w", err)
	}
	if err := boundary.CheckDir(boundary.Root(), false); err != nil {
		return nil, fmt.Errorf("implementer evidence store: %w", err)
	}
	return &runStore{root: boundary}, nil
}

func (s *runStore) begin(intent []byte) (string, *Result, error) {
	existing, found, err := s.root.ReadFileLimit(filepath.Join(s.root.Root(), "intent.json"), true, int64(len(intent)+1))
	if err != nil {
		return "", nil, err
	}
	if found && !bytes.Equal(existing, intent) {
		return "conflict", nil, nil
	}
	if !found {
		if _, err := s.write("intent.json", intent); err != nil {
			return "", nil, err
		}
		return "new", nil, nil
	}
	resultRaw, resultFound, err := s.root.ReadFileLimit(filepath.Join(s.root.Root(), "result.json"), true, 16<<20)
	if err != nil {
		return "", nil, err
	}
	if !resultFound {
		return "indeterminate", nil, nil
	}
	var result Result
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		return "", nil, err
	}
	result.Replayed = true
	return "replay", &result, nil
}

func (s *runStore) write(name string, raw []byte) (tool.Artifact, error) {
	if filepath.Base(name) != name || name == "." || name == ".." {
		return tool.Artifact{}, errors.New("implementer artifact name is unsafe")
	}
	directory, found, err := s.root.OpenDir(s.root.Root(), false)
	if err != nil || !found {
		return tool.Artifact{}, errors.Join(err, os.ErrNotExist)
	}
	defer directory.Close()
	existing, exists, err := directory.ReadFileLimit(name, true, int64(len(raw)+1))
	if err != nil {
		return tool.Artifact{}, err
	}
	if exists {
		if !bytes.Equal(existing, raw) {
			return tool.Artifact{}, fmt.Errorf("implementer artifact collision at %s", name)
		}
		return tool.Artifact{Path: filepath.Join(s.root.Root(), name), SHA256: digestBytes(raw), SizeBytes: int64(len(raw))}, nil
	}
	temporary, err := directory.CreateTemp(".implementer-", 0o600)
	if err != nil {
		return tool.Artifact{}, err
	}
	defer temporary.Close()
	if n, err := temporary.Write(raw); err != nil || n != len(raw) {
		return tool.Artifact{}, errors.Join(err, errors.New("short implementer artifact write"))
	}
	if err := temporary.Sync(); err != nil {
		return tool.Artifact{}, err
	}
	if err := directory.Link(temporary, name); err != nil {
		return tool.Artifact{}, err
	}
	if err := directory.Sync(); err != nil {
		return tool.Artifact{}, err
	}
	return tool.Artifact{Path: filepath.Join(s.root.Root(), name), SHA256: digestBytes(raw), SizeBytes: int64(len(raw))}, nil
}

func (s *runStore) complete(result Result) error {
	raw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	_, err = s.write("result.json", append(raw, '\n'))
	return err
}

func digestBytes(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
