// Package gitoid validates full Git object identities supported by Revolvr.
package gitoid

const (
	SHA1Length   = 40
	SHA256Length = 64
)

// Valid reports whether value is a full lowercase SHA-1 or SHA-256 Git object
// identity. Abbreviated, uppercase, non-hex, and whitespace-padded forms are
// rejected.
func Valid(value string) bool {
	if len(value) != SHA1Length && len(value) != SHA256Length {
		return false
	}
	for i := range value {
		if (value[i] < '0' || value[i] > '9') && (value[i] < 'a' || value[i] > 'f') {
			return false
		}
	}
	return true
}
