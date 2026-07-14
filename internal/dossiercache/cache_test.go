package dossiercache

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCacheMissPublishHitAndInvalidation(t *testing.T) {
	root := t.TempDir()
	store := Store{RepositoryRoot: root}
	source := testSource()
	miss, err := store.Lookup(context.Background(), source)
	if err != nil || miss.Class != ResultMiss {
		t.Fatalf("miss=%+v err=%v", miss, err)
	}
	mapped, err := BuildRepositoryMap(source, []string{"z.md", "internal/b/b.go", "internal/a/a_test.go", ".revolvr/local"})
	if err != nil {
		t.Fatal(err)
	}
	entry, err := NewEntry(source, mapped.Content, mapped.Total, mapped.Included)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Publish(context.Background(), entry); err != nil {
		t.Fatal(err)
	}
	if err := store.Publish(context.Background(), entry); err != nil {
		t.Fatalf("identical replay: %v", err)
	}
	hit, err := store.Lookup(context.Background(), source)
	if err != nil || hit.Class != ResultHit || !bytes.Equal(hit.Entry.Content, entry.Content) {
		t.Fatalf("hit=%+v err=%v", hit, err)
	}
	changed := source
	changed.CommitSHA = strings.Repeat("d", 40)
	changed.TreeSHA = strings.Repeat("e", 40)
	invalidated, err := store.Lookup(context.Background(), changed)
	if err != nil || invalidated.Class != ResultMiss || invalidated.Key == hit.Key {
		t.Fatalf("invalidation=%+v err=%v", invalidated, err)
	}
	guidance := source
	guidance.Guidance = []GuidanceIdentity{{Path: "AGENTS.md", SHA256: strings.Repeat("f", 64), ByteSize: 9}}
	guidanceMiss, err := store.Lookup(context.Background(), guidance)
	if err != nil || guidanceMiss.Class != ResultMiss || guidanceMiss.Key == hit.Key {
		t.Fatalf("guidance invalidation=%+v err=%v", guidanceMiss, err)
	}
}

func TestSourceAcceptsSupportedGitObjectIDs(t *testing.T) {
	for _, length := range []int{40, 64} {
		t.Run(fmt.Sprintf("length_%d", length), func(t *testing.T) {
			source := testSource()
			source.CommitSHA = strings.Repeat("a", length)
			source.TreeSHA = strings.Repeat("b", length)
			if err := source.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestSourceValidationReportsFirstInvalidRootIDDeterministically(t *testing.T) {
	source := testSource()
	source.ControlRootID = "invalid-control"
	source.ExecutionRootID = "invalid-execution"
	const want = "dossier cache: control_root_id must be SHA-256"
	for i := 0; i < 100; i++ {
		if err := source.Validate(); err == nil || err.Error() != want {
			t.Fatalf("Validate() error = %v, want %q (run %d)", err, want, i)
		}
	}
}

func TestSourceGuidanceAcceptsSafeDotDotPrefixes(t *testing.T) {
	source := testSource()
	source.Guidance = []GuidanceIdentity{
		{Path: "..foo", SHA256: strings.Repeat("a", 64), ByteSize: 1},
		{Path: "..well-known/file", SHA256: strings.Repeat("b", 64), ByteSize: 2},
	}
	if err := source.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestSourceGuidanceRejectsTraversalAbsoluteAndNoncanonicalPaths(t *testing.T) {
	for _, path := range invalidRepositoryPaths(t) {
		t.Run(strings.ReplaceAll(path, "/", "_"), func(t *testing.T) {
			source := testSource()
			source.Guidance = []GuidanceIdentity{{Path: path, SHA256: strings.Repeat("a", 64), ByteSize: 1}}
			if err := source.Validate(); err == nil {
				t.Fatalf("Validate() accepted invalid guidance path %q", path)
			}
		})
	}
}

func TestCacheCorruptionNeverReturnsBytes(t *testing.T) {
	root := t.TempDir()
	store := Store{RepositoryRoot: root}
	source := testSource()
	mapped, _ := BuildRepositoryMap(source, []string{"go.mod", "main.go"})
	entry, _ := NewEntry(source, mapped.Content, mapped.Total, mapped.Included)
	if err := store.Publish(context.Background(), entry); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".revolvr", "cache", "dossier", "v1", entry.Manifest.Key, "repository-map.md")
	if err := os.WriteFile(path, []byte("corrupt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := store.Lookup(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if result.Class != ResultCorrupt || result.Diagnostic != "output_identity_mismatch" || len(result.Entry.Content) != 0 {
		t.Fatalf("result=%+v", result)
	}
}

func TestRepositoryMapDeterministicWholePathBounds(t *testing.T) {
	source := testSource()
	source.MaxPaths = 2
	first, err := BuildRepositoryMap(source, []string{"z.md", "cmd/x/main.go", "go.mod", ".git/config"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildRepositoryMap(source, []string{"go.mod", ".git/config", "z.md", "cmd/x/main.go"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Content, second.Content) || first.Total != 3 || first.Included != 2 {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
	text := string(first.Content)
	for _, want := range []string{"cmd/x/main.go [go-source]", "go.mod [go-module]", "omitted=1", "truncated=true"} {
		if !strings.Contains(text, want) {
			t.Fatalf("map missing %q:\n%s", want, text)
		}
	}
}

func TestRepositoryMapAcceptsSafeDotDotPrefixes(t *testing.T) {
	mapped, err := BuildRepositoryMap(testSource(), []string{"..foo", "..well-known/file"})
	if err != nil {
		t.Fatalf("BuildRepositoryMap() error = %v", err)
	}
	for _, want := range []string{"..foo [file]", "..well-known/file [file]"} {
		if !strings.Contains(string(mapped.Content), want) {
			t.Fatalf("repository map missing %q:\n%s", want, mapped.Content)
		}
	}
}

func TestRepositoryMapRejectsTraversalAbsoluteAndNoncanonicalPaths(t *testing.T) {
	for _, path := range invalidRepositoryPaths(t) {
		t.Run(strings.ReplaceAll(path, "/", "_"), func(t *testing.T) {
			if _, err := BuildRepositoryMap(testSource(), []string{path}); err == nil {
				t.Fatalf("BuildRepositoryMap() accepted invalid path %q", path)
			}
		})
	}
}

func TestRepositoryMapMarksSymlinkAndSubmoduleWithoutReadingThem(t *testing.T) {
	raw := "120000 blob " + strings.Repeat("a", 40) + "\tlink\x00" + "160000 commit " + strings.Repeat("b", 40) + "\tthird_party/module\x00"
	items, err := ParseTreeItems(raw)
	if err != nil {
		t.Fatal(err)
	}
	mapped, err := BuildRepositoryMapItems(testSource(), items)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"link [symlink-metadata-only]", "third_party/module [submodule-metadata-only]"} {
		if !strings.Contains(string(mapped.Content), want) {
			t.Fatalf("map missing %q:\n%s", want, mapped.Content)
		}
	}
}

func TestParseTreeItemsAcceptsSHA1AndSHA256ObjectIDs(t *testing.T) {
	for _, length := range []int{40, 64} {
		t.Run(fmt.Sprintf("length_%d", length), func(t *testing.T) {
			oid := strings.Repeat("a", length)
			items, err := ParseTreeItems("100644 blob " + oid + "\ttracked.txt\x00")
			if err != nil {
				t.Fatalf("ParseTreeItems() error = %v", err)
			}
			if got, want := items, []TreeItem{{Path: "tracked.txt", Mode: "100644", Type: "blob"}}; !reflect.DeepEqual(got, want) {
				t.Fatalf("items = %#v, want %#v", got, want)
			}
		})
	}
}

func TestParseTreeItemsRejectsMalformedObjectIDs(t *testing.T) {
	for _, oid := range []string{
		strings.Repeat("a", 39),
		strings.Repeat("a", 41),
		strings.Repeat("a", 63),
		strings.Repeat("a", 65),
		strings.Repeat("A", 40),
		strings.Repeat("g", 64),
	} {
		if _, err := ParseTreeItems("100644 blob " + oid + "\ttracked.txt\x00"); err == nil {
			t.Fatalf("ParseTreeItems() accepted malformed object ID %q", oid)
		}
	}
}

func TestCacheRootAndCancellationIsolation(t *testing.T) {
	a := testSource()
	b := a
	b.ControlRootID = strings.Repeat("9", 64)
	ka, _ := DeriveKey(a)
	kb, _ := DeriveKey(b)
	if ka == kb {
		t.Fatal("different control roots produced the same key")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Store{RepositoryRoot: t.TempDir()}).Lookup(ctx, a); err == nil {
		t.Fatal("cancelled lookup succeeded")
	}
}

func testSource() Source {
	return Source{
		SchemaVersion: SchemaVersion, Algorithm: ProducerAlgorithm,
		ControlRootID: strings.Repeat("a", 64), ExecutionRootID: strings.Repeat("b", 64),
		CommitSHA: strings.Repeat("c", 40), TreeSHA: strings.Repeat("d", 40),
		MaxPaths: 100, MaxBytes: 64 * 1024,
		Guidance: []GuidanceIdentity{{Path: "AGENTS.md", SHA256: strings.Repeat("e", 64), ByteSize: 10}},
	}
}

func invalidRepositoryPaths(t *testing.T) []string {
	t.Helper()
	return []string{
		"..",
		"../foo",
		"a/../../b",
		filepath.Join(string(filepath.Separator), "tmp", "absolute"),
		"./foo",
		"a/../b",
		"a//b",
	}
}
