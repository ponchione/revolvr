package lock

import (
	"errors"
	"time"
)

const sourceWriterWindowGrace = time.Minute

// RequiredSourceWriterTimeout returns the complete supervisor source lease
// window for one Codex process and the Git snapshots on either side of it.
func RequiredSourceWriterTimeout(codexTimeout, gitTimeout time.Duration) (time.Duration, error) {
	if codexTimeout <= 0 || gitTimeout <= 0 {
		return 0, errors.New("source-writer lock window requires positive Codex and Git timeouts")
	}
	const maxDuration = time.Duration(1<<63 - 1)
	if codexTimeout > maxDuration-sourceWriterWindowGrace {
		return 0, errors.New("source-writer lock window overflows time.Duration")
	}
	remaining := maxDuration - codexTimeout - sourceWriterWindowGrace
	if gitTimeout > remaining/2 {
		return 0, errors.New("source-writer lock window overflows time.Duration")
	}
	return codexTimeout + 2*gitTimeout + sourceWriterWindowGrace, nil
}
