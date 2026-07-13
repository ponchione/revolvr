// Package jsonl provides bounded streaming support for JSON Lines records.
package jsonl

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
)

// MaxRecordBytes is the maximum supported record size, excluding the newline
// delimiter. The bound limits memory use while allowing machine records that
// are substantially larger than human-facing output previews.
const MaxRecordBytes = 1024 * 1024

const readBufferBytes = 32 * 1024

var (
	ErrRecordTooLarge = errors.New("jsonl record exceeds maximum size")
	ErrWriterClosed   = errors.New("jsonl record writer is closed")
)

// RecordTooLargeError identifies the record and configured hard limit without
// retaining or rendering any of the rejected record's content.
type RecordTooLargeError struct {
	Record int
	Limit  int
}

func (e *RecordTooLargeError) Error() string {
	return fmt.Sprintf("jsonl: record %d exceeds maximum size of %d bytes", e.Record, e.Limit)
}

func (e *RecordTooLargeError) Unwrap() error {
	return ErrRecordTooLarge
}

// RecordWriter reconstructs newline-delimited records from arbitrary input
// chunks. Complete records are delivered without their newline delimiter.
type RecordWriter struct {
	pending  []byte
	records  int
	onRecord func(recordNumber int, record []byte) error
	err      error
	closed   bool
}

func NewRecordWriter(onRecord func(recordNumber int, record []byte) error) *RecordWriter {
	return &RecordWriter{onRecord: onRecord}
}

// ReadRecords incrementally reads one JSONL stream with memory bounded by the
// shared record limit. The reader remains owned by the caller.
func ReadRecords(ctx context.Context, reader io.Reader, onRecord func(recordNumber int, record []byte) error) error {
	if ctx == nil {
		return errors.New("jsonl: context is required")
	}
	if reader == nil {
		return errors.New("jsonl: reader is required")
	}
	writer := NewRecordWriter(func(recordNumber int, record []byte) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if onRecord == nil {
			return nil
		}
		return onRecord(recordNumber, record)
	})
	buffer := make([]byte, readBufferBytes)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		read, readErr := reader.Read(buffer)
		if read > 0 {
			if _, err := writer.Write(buffer[:read]); err != nil {
				return err
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		switch readErr {
		case nil:
			if read == 0 {
				return io.ErrNoProgress
			}
		case io.EOF:
			return writer.Close()
		default:
			return fmt.Errorf("jsonl: read stream: %w", readErr)
		}
	}
}

func (w *RecordWriter) Write(p []byte) (int, error) {
	if w.closed {
		return 0, ErrWriterClosed
	}
	if w.err != nil {
		return 0, w.err
	}

	consumed := 0
	for len(p) > 0 {
		newline := bytes.IndexByte(p, '\n')
		segment := p
		if newline >= 0 {
			segment = p[:newline]
		}

		available := MaxRecordBytes - len(w.pending)
		if len(segment) > available {
			w.pending = append(w.pending, segment[:available]...)
			consumed += available
			w.err = &RecordTooLargeError{Record: w.records + 1, Limit: MaxRecordBytes}
			return consumed, w.err
		}
		w.pending = append(w.pending, segment...)
		consumed += len(segment)
		if newline < 0 {
			return consumed, nil
		}

		consumed++
		if err := w.emit(); err != nil {
			return consumed, err
		}
		p = p[newline+1:]
	}
	return consumed, nil
}

// Close emits a final unterminated record. A trailing newline does not create
// an additional empty record.
func (w *RecordWriter) Close() error {
	if w.closed {
		return w.err
	}
	w.closed = true
	if w.err != nil {
		return w.err
	}
	if len(w.pending) == 0 {
		return nil
	}
	return w.emit()
}

func (w *RecordWriter) emit() error {
	recordNumber := w.records + 1
	if w.onRecord != nil {
		if err := w.onRecord(recordNumber, w.pending); err != nil {
			w.err = fmt.Errorf("jsonl: process record %d: %w", recordNumber, err)
			return w.err
		}
	}
	w.records = recordNumber
	w.pending = w.pending[:0]
	return nil
}
