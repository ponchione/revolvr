package index

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type AdmissionRules struct {
	Include []string
	Exclude []string
}

var defaultExtensions = map[string]struct{}{
	".go": {}, ".ts": {}, ".tsx": {}, ".js": {}, ".jsx": {}, ".mjs": {}, ".cjs": {},
	".py": {}, ".md": {}, ".markdown": {}, ".sql": {},
}

func ReadGitSnapshot(ctx context.Context, projectID, repository, revision, expectedTree string, rules AdmissionRules, limits Limits) (Snapshot, error) {
	limits, err := limits.normalized()
	if err != nil {
		return Snapshot{}, err
	}
	if strings.TrimSpace(repository) == "" || !filepath.IsAbs(repository) {
		return Snapshot{}, errors.New("code index: managed repository path must be absolute")
	}
	treeRaw, err := runGit(ctx, repository, limits.MaxFileBytes, "rev-parse", "--verify", revision+"^{tree}")
	if err != nil {
		return Snapshot{}, err
	}
	tree := strings.TrimSpace(string(treeRaw))
	if tree != expectedTree {
		return Snapshot{}, errors.New("code index: admitted source tree changed")
	}
	raw, err := runGit(ctx, repository, 64<<20, "ls-tree", "-r", "-z", "--full-tree", revision)
	if err != nil {
		return Snapshot{}, err
	}
	entries := bytes.Split(raw, []byte{0})
	files := make([]File, 0, min(len(entries), limits.MaxFiles))
	for _, record := range entries {
		if len(record) == 0 {
			continue
		}
		tab := bytes.IndexByte(record, '\t')
		if tab <= 0 {
			return Snapshot{}, errors.New("code index: malformed Git tree record")
		}
		meta := strings.Fields(string(record[:tab]))
		path := string(record[tab+1:])
		if len(meta) != 3 || meta[1] != "blob" || meta[0] == "120000" || !admittedPath(path, rules) {
			continue
		}
		if len(files) >= limits.MaxFiles {
			return Snapshot{}, fmt.Errorf("code index: admitted file count exceeds %d", limits.MaxFiles)
		}
		sizeRaw, err := runGit(ctx, repository, 128, "cat-file", "-s", meta[2])
		if err != nil {
			return Snapshot{}, err
		}
		size, err := strconv.Atoi(strings.TrimSpace(string(sizeRaw)))
		if err != nil || size < 0 || size > limits.MaxFileBytes {
			return Snapshot{}, fmt.Errorf("code index: admitted file %s exceeds bounds", path)
		}
		content, err := runGit(ctx, repository, limits.MaxFileBytes, "cat-file", "blob", meta[2])
		if err != nil {
			return Snapshot{}, err
		}
		files = append(files, File{Path: filepath.ToSlash(path), Content: content})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	snapshot := Snapshot{ProjectID: projectID, SourceRevision: revision, SourceTree: tree, Files: files}
	if err := snapshot.Validate(limits); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func admittedPath(path string, rules AdmissionRules) bool {
	path = filepath.ToSlash(path)
	if path == "" || strings.HasPrefix(path, ".git/") || strings.HasPrefix(path, ".revolvr/") || strings.Contains(path, "/vendor/") || strings.HasPrefix(path, "vendor/") || strings.Contains(path, "/node_modules/") || strings.HasPrefix(path, "node_modules/") {
		return false
	}
	included := len(rules.Include) == 0
	if included {
		_, included = defaultExtensions[strings.ToLower(filepath.Ext(path))]
	}
	for _, pattern := range rules.Include {
		if globMatch(pattern, path) {
			included = true
			break
		}
	}
	if !included {
		return false
	}
	for _, pattern := range rules.Exclude {
		if globMatch(pattern, path) {
			return false
		}
	}
	return true
}

func globMatch(pattern, path string) bool {
	pattern = filepath.ToSlash(strings.TrimSpace(pattern))
	if pattern == "" {
		return false
	}
	var expression strings.Builder
	expression.WriteByte('^')
	for i := 0; i < len(pattern); {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				if i+2 < len(pattern) && pattern[i+2] == '/' {
					expression.WriteString("(?:.*/)?")
					i += 3
				} else {
					expression.WriteString(".*")
					i += 2
				}
			} else {
				expression.WriteString("[^/]*")
				i++
			}
		case '?':
			expression.WriteString("[^/]")
			i++
		default:
			expression.WriteString(regexp.QuoteMeta(string(pattern[i])))
			i++
		}
	}
	expression.WriteByte('$')
	matched, _ := regexp.MatchString(expression.String(), path)
	return matched
}

func runGit(ctx context.Context, repository string, capBytes int, args ...string) ([]byte, error) {
	arguments := append([]string{
		"-c", "core.hooksPath=/dev/null", "-c", "credential.helper=", "-c", "protocol.file.allow=never",
		"-C", repository,
	}, args...)
	command := exec.CommandContext(ctx, "git", arguments...)
	command.Env = []string{"PATH=/usr/bin:/bin", "HOME=/nonexistent", "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0", "LC_ALL=C"}
	var stdout, stderr boundedBuffer
	stdout.limit, stderr.limit = capBytes, 8192
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("code index git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	if stdout.truncated {
		return nil, fmt.Errorf("code index git %s: output exceeds %d bytes", args[0], capBytes)
	}
	return stdout.Bytes(), nil
}

type boundedBuffer struct {
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func (b *boundedBuffer) Write(value []byte) (int, error) {
	original := len(value)
	remaining := b.limit - b.buffer.Len()
	if remaining <= 0 {
		b.truncated = b.truncated || original > 0
		return original, nil
	}
	if len(value) > remaining {
		value = value[:remaining]
		b.truncated = true
	}
	_, _ = b.buffer.Write(value)
	return original, nil
}
func (b *boundedBuffer) Bytes() []byte  { return append([]byte(nil), b.buffer.Bytes()...) }
func (b *boundedBuffer) String() string { return b.buffer.String() }
