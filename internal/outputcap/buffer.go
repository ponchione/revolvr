package outputcap

import "bytes"

const DefaultLimit = 1 << 20

// Buffer is an io.Writer that keeps only the first limit bytes while reporting
// successful writes, so subprocess pipes are still drained after the cap is hit.
type Buffer struct {
	buf       bytes.Buffer
	limit     int
	truncated int64
}

func NewBuffer(limit int) *Buffer {
	if limit <= 0 {
		limit = DefaultLimit
	}
	return &Buffer{limit: limit}
}

func (b *Buffer) Write(p []byte) (int, error) {
	if b == nil {
		return len(p), nil
	}

	remaining := b.limit - b.buf.Len()
	if remaining > 0 {
		if len(p) <= remaining {
			_, _ = b.buf.Write(p)
		} else {
			_, _ = b.buf.Write(p[:remaining])
			b.truncated += int64(len(p) - remaining)
		}
	} else {
		b.truncated += int64(len(p))
	}

	return len(p), nil
}

func (b *Buffer) String() string {
	if b == nil {
		return ""
	}
	return b.buf.String()
}

func (b *Buffer) Len() int {
	if b == nil {
		return 0
	}
	return b.buf.Len()
}

func (b *Buffer) TruncatedBytes() int64 {
	if b == nil {
		return 0
	}
	return b.truncated
}
