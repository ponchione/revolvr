package autonomousaudit

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"strings"
)

func validateIdentity(label, value string) error {
	if value == "" || value != strings.TrimSpace(value) || strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("%s %q is empty or malformed", label, value)
	}
	return nil
}
func validatePath(label, value string) error {
	if value == "" || value != strings.TrimSpace(value) || strings.HasPrefix(value, "/") {
		return fmt.Errorf("%s %q is empty, absolute, or malformed", label, value)
	}
	clean := path.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || clean != value {
		return fmt.Errorf("%s %q is not a normalized repository-relative path", label, value)
	}
	return nil
}
func validSHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}
