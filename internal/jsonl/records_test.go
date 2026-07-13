package jsonl

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestReadRecordsUsesBoundedReadsAndHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reader := &boundedReader{content: []byte("{\"one\":1}\n{\"two\":2}\n")}
	var records []string
	err := ReadRecords(ctx, reader, func(number int, record []byte) error {
		records = append(records, string(record))
		if number == 1 {
			cancel()
		}
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ReadRecords() error = %v, want context cancellation", err)
	}
	if !reflect.DeepEqual(records, []string{`{"one":1}`}) {
		t.Fatalf("records = %#v", records)
	}
	if reader.maxRequest > readBufferBytes {
		t.Fatalf("maximum read request = %d, want at most %d", reader.maxRequest, readBufferBytes)
	}
}

func TestReadRecordsPropagatesReaderError(t *testing.T) {
	wantErr := errors.New("injected read failure")
	err := ReadRecords(context.Background(), errorReader{err: wantErr}, nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("ReadRecords() error = %v, want injected error", err)
	}
}

func TestRecordWriterReassemblesArbitraryChunksAndFinalRecord(t *testing.T) {
	var numbers []int
	var records []string
	writer := NewRecordWriter(func(number int, record []byte) error {
		numbers = append(numbers, number)
		records = append(records, string(record))
		return nil
	})
	stream := []byte("{\"text\":\"café\"}\n{\"ok\":true}")
	for _, chunk := range [][]byte{
		stream[:5],
		stream[5:13],
		stream[13:14],
		stream[14:18],
		stream[18:19],
		stream[19:],
	} {
		if _, err := writer.Write(chunk); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if !reflect.DeepEqual(numbers, []int{1, 2}) {
		t.Fatalf("record numbers = %#v", numbers)
	}
	if !reflect.DeepEqual(records, []string{`{"text":"café"}`, `{"ok":true}`}) {
		t.Fatalf("records = %#v", records)
	}
}

func TestRecordWriterAcceptsRecordAtHardLimit(t *testing.T) {
	called := false
	writer := NewRecordWriter(func(_ int, record []byte) error {
		called = true
		if len(record) != MaxRecordBytes {
			t.Fatalf("record size = %d, want %d", len(record), MaxRecordBytes)
		}
		return nil
	})
	if _, err := writer.Write([]byte(strings.Repeat("x", MaxRecordBytes) + "\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if !called {
		t.Fatal("record callback was not called")
	}
}

func TestRecordWriterRejectsRecordAboveHardLimit(t *testing.T) {
	called := false
	writer := NewRecordWriter(func(_ int, _ []byte) error {
		called = true
		return nil
	})
	_, err := writer.Write([]byte(strings.Repeat("x", MaxRecordBytes+1)))
	if !errors.Is(err, ErrRecordTooLarge) {
		t.Fatalf("Write() error = %v, want ErrRecordTooLarge", err)
	}
	var sizeErr *RecordTooLargeError
	if !errors.As(err, &sizeErr) || sizeErr.Record != 1 || sizeErr.Limit != MaxRecordBytes {
		t.Fatalf("Write() error = %#v, want record 1 limit %d", err, MaxRecordBytes)
	}
	if closeErr := writer.Close(); !errors.Is(closeErr, ErrRecordTooLarge) {
		t.Fatalf("Close() error = %v, want ErrRecordTooLarge", closeErr)
	}
	if called {
		t.Fatal("oversized partial record reached callback")
	}
}

type boundedReader struct {
	content    []byte
	maxRequest int
}

func (r *boundedReader) Read(p []byte) (int, error) {
	if len(p) > r.maxRequest {
		r.maxRequest = len(p)
	}
	if len(r.content) == 0 {
		return 0, io.EOF
	}
	read := copy(p, r.content)
	r.content = r.content[read:]
	return read, nil
}

type errorReader struct {
	err error
}

func (r errorReader) Read([]byte) (int, error) {
	return 0, r.err
}
