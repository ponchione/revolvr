package receipt

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

type HarnessFields struct {
	Verdict            Verdict
	CodexExitCode      int
	VerificationStatus string
	CommitSHA          string
	ChangedFiles       []string
	Verification       []VerificationEntry
	Metrics            Metrics
}

func RewriteHarnessFields(content []byte, fields HarnessFields) ([]byte, Receipt, bool, error) {
	parsed, err := Parse(content)
	if err != nil {
		return nil, Receipt{}, false, err
	}

	updatedReceipt := parsed
	updatedReceipt.Verdict = fields.Verdict
	updatedReceipt.CodexExitCode = fields.CodexExitCode
	updatedReceipt.VerificationStatus = fields.VerificationStatus
	updatedReceipt.CommitSHA = fields.CommitSHA
	updatedReceipt.ChangedFiles = compactStrings(fields.ChangedFiles)
	updatedReceipt.Verification = compactVerification(fields.Verification)
	updatedReceipt.Metrics = fields.Metrics

	frontmatter, err := yaml.Marshal(updatedReceipt)
	if err != nil {
		return nil, Receipt{}, false, fmt.Errorf("receipt: encode yaml: %w", err)
	}

	var updated bytes.Buffer
	updated.WriteString("---\n")
	updated.Write(frontmatter)
	updated.WriteString("---\n")
	updated.WriteString(parsed.RawBody)

	reparsed, err := Parse(updated.Bytes())
	if err != nil {
		return nil, Receipt{}, false, err
	}
	return updated.Bytes(), reparsed, !bytes.Equal(content, updated.Bytes()), nil
}
