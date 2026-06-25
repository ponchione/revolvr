package outputcap

import "testing"

func TestBufferCapturesWithinLimit(t *testing.T) {
	buf := NewBuffer(10)

	n, err := buf.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if n != len("hello") {
		t.Fatalf("write length = %d, want %d", n, len("hello"))
	}
	if got, want := buf.String(), "hello"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
	if got := buf.TruncatedBytes(); got != 0 {
		t.Fatalf("truncated bytes = %d, want 0", got)
	}
}

func TestBufferCapsOutputAndCountsTruncatedBytes(t *testing.T) {
	buf := NewBuffer(5)

	n, err := buf.Write([]byte("hello world"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if n != len("hello world") {
		t.Fatalf("write length = %d, want %d", n, len("hello world"))
	}
	if got, want := buf.String(), "hello"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
	if got, want := buf.TruncatedBytes(), int64(6); got != want {
		t.Fatalf("truncated bytes = %d, want %d", got, want)
	}

	n, err = buf.Write([]byte("!!!"))
	if err != nil {
		t.Fatalf("second write: %v", err)
	}
	if n != len("!!!") {
		t.Fatalf("second write length = %d, want %d", n, len("!!!"))
	}
	if got, want := buf.String(), "hello"; got != want {
		t.Fatalf("buffer after second write = %q, want %q", got, want)
	}
	if got, want := buf.TruncatedBytes(), int64(9); got != want {
		t.Fatalf("truncated bytes after second write = %d, want %d", got, want)
	}
}

func TestBufferUsesDefaultLimitForNonPositiveLimit(t *testing.T) {
	buf := NewBuffer(0)
	if got, want := buf.limit, DefaultLimit; got != want {
		t.Fatalf("limit = %d, want %d", got, want)
	}
}
