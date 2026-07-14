package dossiercache

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"revolvr/internal/gitoid"
)

type MapResult struct {
	Content  []byte
	Total    int
	Included int
}

type TreeItem struct {
	Path string
	Mode string
	Type string
}

// BuildRepositoryMap derives a bounded deterministic path map from the exact
// path list returned by `git ls-tree` for Source.CommitSHA. It never reads the
// ambient worktree.
func BuildRepositoryMap(source Source, paths []string) (MapResult, error) {
	items := make([]TreeItem, len(paths))
	for i, path := range paths {
		items[i] = TreeItem{Path: path, Mode: "100644", Type: "blob"}
	}
	return BuildRepositoryMapItems(source, items)
}

// ParseTreeItems parses the NUL-delimited output of `git ls-tree -r -z`.
func ParseTreeItems(raw string) ([]TreeItem, error) {
	if raw == "" {
		return nil, nil
	}
	records := strings.Split(raw, "\x00")
	if records[len(records)-1] == "" {
		records = records[:len(records)-1]
	}
	items := make([]TreeItem, 0, len(records))
	for _, record := range records {
		tab := strings.IndexByte(record, '\t')
		fields := strings.Fields(record[:maxInt(tab, 0)])
		if tab <= 0 || len(fields) != 3 || len(fields[0]) != 6 || (fields[1] != "blob" && fields[1] != "tree" && fields[1] != "commit") || !gitoid.Valid(fields[2]) {
			return nil, errors.New("dossier cache: malformed git ls-tree record")
		}
		items = append(items, TreeItem{Path: record[tab+1:], Mode: fields[0], Type: fields[1]})
	}
	return items, nil
}

func BuildRepositoryMapItems(source Source, items []TreeItem) (MapResult, error) {
	if err := source.Validate(); err != nil {
		return MapResult{}, err
	}
	copyItems := append([]TreeItem(nil), items...)
	sort.Slice(copyItems, func(i, j int) bool { return copyItems[i].Path < copyItems[j].Path })
	filtered := make([]TreeItem, 0, len(copyItems))
	previous := ""
	for _, item := range copyItems {
		path := item.Path
		if path == "" || !utf8.ValidString(path) || filepath.IsAbs(path) || filepath.Clean(path) != path || strings.HasPrefix(path, "..") || strings.ContainsRune(path, '\x00') {
			return MapResult{}, fmt.Errorf("dossier cache: invalid Git tree path %q", path)
		}
		if len(item.Mode) != 6 || (item.Type != "blob" && item.Type != "tree" && item.Type != "commit") {
			return MapResult{}, fmt.Errorf("dossier cache: invalid Git tree type/mode for %q", path)
		}
		path = filepath.ToSlash(path)
		if path == previous {
			return MapResult{}, errors.New("dossier cache: duplicate Git tree path")
		}
		previous = path
		if path == ".git" || strings.HasPrefix(path, ".git/") || path == ".revolvr" || strings.HasPrefix(path, ".revolvr/") {
			continue
		}
		item.Path = path
		filtered = append(filtered, item)
	}

	var out bytes.Buffer
	fmt.Fprintf(&out, "Repository map algorithm: %s\nCommit: %s\nTree: %s\n\n", source.Algorithm, source.CommitSHA, source.TreeSHA)
	included := 0
	for _, item := range filtered {
		kind := classifyPath(item.Path)
		if item.Mode == "120000" {
			kind = "symlink-metadata-only"
		} else if item.Mode == "160000" || item.Type == "commit" {
			kind = "submodule-metadata-only"
		}
		line := fmt.Sprintf("- %s [%s] mode=%s type=%s\n", item.Path, kind, item.Mode, item.Type)
		if included >= source.MaxPaths || out.Len()+len(line)+128 > source.MaxBytes {
			break
		}
		out.WriteString(line)
		included++
	}
	omitted := len(filtered) - included
	fmt.Fprintf(&out, "\nItems: total=%d included=%d omitted=%d truncated=%t\n", len(filtered), included, omitted, omitted > 0)
	if out.Len() > source.MaxBytes {
		return MapResult{}, errors.New("dossier cache: repository map bounds too small for required identity and omission facts")
	}
	return MapResult{Content: out.Bytes(), Total: len(filtered), Included: included}, nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func classifyPath(path string) string {
	base := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(base))
	switch {
	case base == "go.mod":
		return "go-module"
	case ext == ".go":
		if strings.HasSuffix(base, "_test.go") {
			return "go-test"
		}
		return "go-source"
	case ext == ".md":
		return "documentation"
	case ext == ".json" || ext == ".yaml" || ext == ".yml" || ext == ".toml":
		return "configuration"
	case strings.HasPrefix(path, "cmd/"):
		return "command"
	case strings.HasPrefix(path, "internal/"):
		return "internal"
	default:
		return "file"
	}
}
