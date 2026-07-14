package operatorcheckpoint

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReceiptStrictRoundTrip(t *testing.T) {
	receipt := validReceipt("license-acceptance")
	receipt.BuildSHA256 = testHash("build")
	receipt.SourceSHA256 = testHash("source")
	raw, err := Marshal(receipt)
	if err != nil {
		t.Fatalf("marshal receipt: %v", err)
	}
	decoded, err := Decode(raw, receipt.TaskID)
	if err != nil {
		t.Fatalf("decode receipt: %v", err)
	}
	repeated, err := Marshal(decoded)
	if err != nil {
		t.Fatalf("marshal decoded receipt: %v", err)
	}
	if !bytes.Equal(raw, repeated) {
		t.Fatalf("receipt round trip changed bytes:\nfirst  %s\nsecond %s", raw, repeated)
	}
}

func TestReceiptRejectsMalformedOrUnsafeContent(t *testing.T) {
	validRaw, err := Marshal(validReceipt("license-acceptance"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		raw  []byte
		want string
	}{
		{name: "empty", raw: nil, want: "input is empty"},
		{name: "malformed", raw: []byte(`{"schema_version":`), want: "decode operator checkpoint receipt"},
		{name: "multiple values", raw: append(append([]byte(nil), validRaw...), []byte(` {}`)...), want: "multiple JSON values"},
		{name: "unknown top-level field", raw: replaceJSON(validRaw, `"decision": "Approved."`, `"decision": "Approved.", "private": "inline secret"`), want: "unknown field"},
		{name: "unknown evidence field", raw: replaceJSON(validRaw, `"sha256": "`+testHash("evidence")+`"`, `"sha256": "`+testHash("evidence")+`", "contents": "private bytes"`), want: "unknown field"},
		{name: "duplicate top-level field", raw: replaceJSON(validRaw, `"decision": "Approved."`, `"decision": "Approved.", "decision": "Accepted twice."`), want: `duplicate field "decision"`},
		{name: "duplicate evidence field", raw: replaceJSON(validRaw, `"kind": "file"`, `"kind": "file", "kind": "source"`), want: `duplicate field "kind"`},
		{name: "missing evidence", raw: replaceJSON(validRaw, evidenceJSON(), `"evidence": []`), want: "at least one evidence reference"},
		{name: "invalid evidence hash", raw: replaceJSON(validRaw, testHash("evidence"), "ABC"), want: "sha256 is malformed"},
		{name: "unsafe absolute path", raw: replaceJSON(validRaw, `docs/license.txt`, `/tmp/license.txt`), want: "repository-relative slash path"},
		{name: "unsafe traversal path", raw: replaceJSON(validRaw, `docs/license.txt`, `../license.txt`), want: "repository-relative slash path"},
		{name: "noncanonical path", raw: replaceJSON(validRaw, `docs/license.txt`, `docs/../license.txt`), want: "repository-relative slash path"},
		{name: "invalid accepted time offset", raw: replaceJSON(validRaw, `2026-07-14T15:04:05Z`, `2026-07-14T11:04:05-04:00`), want: "explicit UTC"},
		{name: "unsupported outcome", raw: replaceJSON(validRaw, `"outcome": "accepted"`, `"outcome": "rejected"`), want: "accepted outcome"},
		{name: "newline in decision", raw: replaceJSON(validRaw, `Approved.`, `Approved.\nsecret`), want: "single-line"},
		{name: "invalid optional identity", raw: replaceJSON(validRaw, `"subject":`, `"build_sha256": "bad", "subject":`), want: "build_sha256 is malformed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Decode(test.raw, "license-acceptance")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("decode error = %v, want substring %q", err, test.want)
			}
		})
	}

	otherRaw, err := Marshal(validReceipt("other-checkpoint"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(otherRaw, "license-acceptance"); err == nil || !strings.Contains(err.Error(), "does not match checkpoint") {
		t.Fatalf("task mismatch error = %v", err)
	}
}

func TestLoadRequiresCanonicalNonSymlinkReceipt(t *testing.T) {
	root := t.TempDir()
	taskID := "manual-acceptance"
	receiptPath := ExpectedReceiptPath(taskID)
	raw, err := Marshal(validReceipt(taskID))
	if err != nil {
		t.Fatal(err)
	}
	absPath := filepath.Join(root, filepath.FromSlash(receiptPath))
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	snapshot, err := Load(root, receiptPath, taskID)
	if err != nil {
		t.Fatalf("load receipt: %v", err)
	}
	if snapshot.Path != receiptPath || snapshot.SHA256 != testHashBytes(raw) || snapshot.ByteSize != len(raw) || !bytes.Equal(snapshot.Raw, raw) {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if _, err := Load(root, ".agent/checkpoints/../outside.json", taskID); err == nil || !strings.Contains(err.Error(), "must be") {
		t.Fatalf("alternate path error = %v", err)
	}

	if err := os.Remove(absPath); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "outside.json")
	if err := os.WriteFile(target, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, absPath); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root, receiptPath, taskID); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("symlink error = %v", err)
	}
}

func validReceipt(taskID string) Receipt {
	return Receipt{
		SchemaVersion: ReceiptSchemaVersion,
		TaskID:        taskID,
		Outcome:       OutcomeAccepted,
		Operator:      "operator@example.test",
		Provenance:    "manual review under repository acceptance policy",
		AcceptedAt:    time.Date(2026, 7, 14, 15, 4, 5, 0, time.UTC),
		Subject:       "License terms for bundled asset",
		Decision:      "Approved.",
		Evidence: []EvidenceReference{{
			Kind: EvidenceFile, Path: "docs/license.txt", SHA256: testHash("evidence"),
		}},
	}
}

func replaceJSON(raw []byte, old, replacement string) []byte {
	return bytes.Replace(raw, []byte(old), []byte(replacement), 1)
}

func evidenceJSON() string {
	return fmt.Sprintf(`"evidence": [
    {
      "kind": "file",
      "path": "docs/license.txt",
      "sha256": "%s"
    }
  ]`, testHash("evidence"))
}

func testHash(value string) string {
	return testHashBytes([]byte(value))
}

func testHashBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return fmt.Sprintf("%x", sum)
}
