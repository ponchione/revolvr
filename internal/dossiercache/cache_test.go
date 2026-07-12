package dossiercache

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
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
