package receipt

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"revolvr/internal/jsonl"
)

func TestParseCodexUsageMetricsReaderSupportsExplicitSchemas(t *testing.T) {
	type testCase struct {
		name   string
		record string
		want   Metrics
	}
	var tests []testCase
	nextMetrics := func() Metrics {
		base := len(tests)*3 + 1
		return Metrics{InputTokens: base, OutputTokens: base + 1, DurationSeconds: base + 2}
	}
	usageJSON := func(metrics Metrics) string {
		return fmt.Sprintf(`{"input_tokens":%d,"output_tokens":%d,"duration_seconds":%d}`, metrics.InputTokens, metrics.OutputTokens, metrics.DurationSeconds)
	}
	supportedUsageKeys := []string{"usage", "total_usage", "token_usage", "total_token_usage"}
	for _, usageKey := range supportedUsageKeys {
		metrics := nextMetrics()
		tests = append(tests, testCase{
			name:   "top-level " + usageKey,
			record: fmt.Sprintf(`{"%s":%s}`, usageKey, usageJSON(metrics)),
			want:   metrics,
		})
	}
	for _, parentKey := range []string{"response", "result", "message", "event"} {
		for _, usageKey := range supportedUsageKeys {
			metrics := nextMetrics()
			tests = append(tests, testCase{
				name:   parentKey + " " + usageKey,
				record: fmt.Sprintf(`{"%s":{"%s":%s}}`, parentKey, usageKey, usageJSON(metrics)),
				want:   metrics,
			})
		}
	}
	bareMetrics := nextMetrics()
	tests = append(tests, testCase{name: "bare metrics", record: usageJSON(bareMetrics), want: bareMetrics})
	nestedMetrics := nextMetrics()
	tests = append(tests, testCase{
		name:   "unique nested legacy usage",
		record: fmt.Sprintf(`{"payload":{"details":{"usage":%s}}}`, usageJSON(nestedMetrics)),
		want:   nestedMetrics,
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics, found, err := ParseCodexUsageMetricsReader(context.Background(), strings.NewReader(tt.record))
			if err != nil {
				t.Fatalf("ParseCodexUsageMetricsReader() error = %v", err)
			}
			if !found || metrics != tt.want {
				t.Fatalf("metrics/found = %+v/%t, want %+v/true", metrics, found, tt.want)
			}
		})
	}
}

func TestParseCodexUsageMetricsReaderUsesSchemaPrecedence(t *testing.T) {
	tests := []struct {
		name   string
		record string
		want   int
	}{
		{
			name:   "top-level usage key order",
			record: `{"total_token_usage":{"input_tokens":4},"token_usage":{"input_tokens":3},"total_usage":{"input_tokens":2},"usage":{"input_tokens":1}}`,
			want:   1,
		},
		{
			name:   "top-level usage before envelopes and bare metrics",
			record: `{"input_tokens":3,"response":{"usage":{"input_tokens":2}},"usage":{"input_tokens":1}}`,
			want:   1,
		},
		{
			name:   "envelope order",
			record: `{"event":{"usage":{"input_tokens":4}},"message":{"usage":{"input_tokens":3}},"result":{"usage":{"input_tokens":2}},"response":{"usage":{"input_tokens":1}}}`,
			want:   1,
		},
		{
			name:   "usage key order inside envelope",
			record: `{"response":{"total_token_usage":{"input_tokens":4},"token_usage":{"input_tokens":3},"total_usage":{"input_tokens":2},"usage":{"input_tokens":1}}}`,
			want:   1,
		},
		{
			name:   "envelope before bare metrics",
			record: `{"input_tokens":2,"response":{"usage":{"input_tokens":1}}}`,
			want:   1,
		},
		{
			name:   "bare metrics before legacy nested usage",
			record: `{"input_tokens":1,"payload":{"usage":{"input_tokens":2}}}`,
			want:   1,
		},
		{
			name:   "explicit schema before ambiguous legacy candidates",
			record: `{"usage":{"input_tokens":1},"alpha":{"usage":{"input_tokens":2}},"omega":{"usage":{"input_tokens":3}}}`,
			want:   1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics, found, err := ParseCodexUsageMetricsReader(context.Background(), strings.NewReader(tt.record))
			if err != nil {
				t.Fatalf("ParseCodexUsageMetricsReader() error = %v", err)
			}
			if !found || metrics != (Metrics{InputTokens: tt.want}) {
				t.Fatalf("metrics/found = %+v/%t, want input_tokens=%d/true", metrics, found, tt.want)
			}
		})
	}
}

func TestParseCodexUsageMetricsReaderRejectsAmbiguousNestedUsageDeterministically(t *testing.T) {
	const record = `{"omega":[{"total_usage":{"input_tokens":101}}],"alpha":{"usage":{"input_tokens":1}}}`
	const wantCandidates = "/alpha/usage,/omega/0/total_usage"
	for i := 0; i < 1000; i++ {
		metrics, found, err := ParseCodexUsageMetricsReader(context.Background(), strings.NewReader(record))
		if found || metrics != (Metrics{}) || !errors.Is(err, ErrCodexUsageAmbiguity) {
			t.Fatalf("iteration %d metrics/found/error = %+v/%t/%v, want empty/false/ambiguity", i, metrics, found, err)
		}
		var ambiguity *CodexUsageAmbiguityError
		if !errors.As(err, &ambiguity) || ambiguity.Record != 1 || strings.Join(ambiguity.CandidatePaths, ",") != wantCandidates {
			t.Fatalf("iteration %d ambiguity diagnostic = %#v", i, err)
		}
	}
}

func TestRewriteMetricsFromCodexJSONLReaderPreservesReceiptOnAmbiguousUsage(t *testing.T) {
	original := []byte(validReceiptContent())
	record := `{"alpha":{"usage":{"input_tokens":1}},"omega":{"usage":{"input_tokens":101}}}`
	updated, parsed, changed, err := RewriteMetricsFromCodexJSONLReader(context.Background(), original, strings.NewReader(record))
	if changed || !errors.Is(err, ErrCodexUsageAmbiguity) {
		t.Fatalf("changed/error = %t/%v, want false/ambiguity", changed, err)
	}
	if string(updated) != string(original) || parsed.Metrics != (Metrics{InputTokens: 11, OutputTokens: 7, DurationSeconds: 3}) {
		t.Fatalf("receipt changed during ambiguous metrics extraction")
	}
}

func TestParseCodexUsageMetricsFileClassifiesAmbiguityAsParseDiagnostic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "codex.jsonl")
	if err := os.WriteFile(path, []byte(`{"alpha":{"usage":{"input_tokens":1}},"omega":{"usage":{"input_tokens":101}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, found, err := ParseCodexUsageMetricsFile(context.Background(), path)
	if found || !errors.Is(err, ErrCodexUsageAmbiguity) || errors.Is(err, ErrCodexJSONLSource) {
		t.Fatalf("found/error = %t/%v, want ambiguity parse diagnostic", found, err)
	}
}

func TestParseCodexUsageMetricsReaderStreamsLargeGeneratedArtifactWithBoundedReads(t *testing.T) {
	const records = 64
	event := `{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":2,"duration_seconds":3}}`
	record := []byte(event + strings.Repeat(" ", jsonl.MaxRecordBytes-len(event)-1) + "\n")
	reader := &repeatingRecordReader{record: record, remaining: records}

	metrics, found, err := ParseCodexUsageMetricsReader(context.Background(), reader)
	if err != nil {
		t.Fatalf("ParseCodexUsageMetricsReader() error = %v", err)
	}
	if !found || metrics != (Metrics{InputTokens: records, OutputTokens: records * 2, DurationSeconds: records * 3}) {
		t.Fatalf("metrics = %+v, found = %t", metrics, found)
	}
	if reader.bytesRead != int64(len(record))*records {
		t.Fatalf("bytes read = %d, want %d", reader.bytesRead, int64(len(record))*records)
	}
	if reader.maxRequest > 64*1024 {
		t.Fatalf("maximum read request = %d, want bounded streaming reads", reader.maxRequest)
	}
}

func TestParseCodexUsageMetricsReaderAcceptsMaximumSizedFinalRecord(t *testing.T) {
	event := `{"usage":{"input_tokens":7,"output_tokens":5,"duration_ms":1500}}`
	record := event + strings.Repeat(" ", jsonl.MaxRecordBytes-len(event))
	metrics, found, err := ParseCodexUsageMetricsReader(context.Background(), strings.NewReader(record))
	if err != nil {
		t.Fatalf("ParseCodexUsageMetricsReader() error = %v", err)
	}
	want := Metrics{InputTokens: 7, OutputTokens: 5, DurationSeconds: 2}
	if !found || metrics != want {
		t.Fatalf("metrics = %+v, found = %t, want %+v/true", metrics, found, want)
	}
}

func TestParseCodexUsageMetricsReaderRejectsOversizedRecordWithSharedContract(t *testing.T) {
	event := `{"usage":{"input_tokens":1}}`
	record := event + strings.Repeat(" ", jsonl.MaxRecordBytes-len(event)+1)
	_, found, err := ParseCodexUsageMetricsReader(context.Background(), strings.NewReader(record))
	if found || !errors.Is(err, jsonl.ErrRecordTooLarge) {
		t.Fatalf("found = %t, error = %v, want shared record-size error", found, err)
	}
	var sizeErr *jsonl.RecordTooLargeError
	if !errors.As(err, &sizeErr) || sizeErr.Record != 1 || sizeErr.Limit != jsonl.MaxRecordBytes {
		t.Fatalf("record size error = %#v", err)
	}
}

func TestMalformedMiddleRecordReturnsDiagnosticAndPreservesReceipt(t *testing.T) {
	stream := strings.Join([]string{
		`{"usage":{"input_tokens":2,"output_tokens":3}}`,
		`not-json`,
		`{"usage":{"input_tokens":5,"output_tokens":7}}`,
	}, "\n")
	metrics, found, err := ParseCodexUsageMetricsReader(context.Background(), strings.NewReader(stream))
	if !found || metrics != (Metrics{InputTokens: 7, OutputTokens: 10}) {
		t.Fatalf("partial metrics = %+v, found = %t", metrics, found)
	}
	var malformed *MalformedCodexJSONLError
	if !errors.As(err, &malformed) || malformed.FirstRecord != 2 || malformed.Count != 1 || !errors.Is(err, ErrMalformedCodexJSONL) {
		t.Fatalf("malformed diagnostic = %#v", err)
	}

	original := []byte(validReceiptContent())
	updated, parsed, changed, rewriteErr := RewriteMetricsFromCodexJSONLReader(context.Background(), original, strings.NewReader(stream))
	if !errors.Is(rewriteErr, ErrMalformedCodexJSONL) || changed {
		t.Fatalf("rewrite error/changed = %v/%t", rewriteErr, changed)
	}
	if string(updated) != string(original) || parsed.Metrics != (Metrics{InputTokens: 11, OutputTokens: 7, DurationSeconds: 3}) {
		t.Fatalf("receipt changed during degraded metrics extraction")
	}
}

func TestParseCodexUsageMetricsReaderRejectsNumericOverflow(t *testing.T) {
	maximum := strconv.Itoa(maxIntValue())
	tests := []struct {
		name       string
		stream     string
		wantRecord int
	}{
		{
			name:       "individual value",
			stream:     `{"usage":{"input_tokens":` + overflowIntegerText() + `}}`,
			wantRecord: 1,
		},
		{
			name: "aggregate total",
			stream: strings.Join([]string{
				`{"usage":{"input_tokens":` + maximum + `}}`,
				`{"usage":{"input_tokens":1}}`,
			}, "\n"),
			wantRecord: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, found, err := ParseCodexUsageMetricsReader(context.Background(), strings.NewReader(tt.stream))
			if found || !errors.Is(err, ErrCodexUsageOverflow) {
				t.Fatalf("found = %t, error = %v, want overflow", found, err)
			}
			var overflow *CodexUsageOverflowError
			if !errors.As(err, &overflow) || overflow.Record != tt.wantRecord || overflow.Field != "input_tokens" {
				t.Fatalf("overflow diagnostic = %#v", err)
			}
		})
	}
}

func TestParseCodexUsageMetricsReadCloserCancellationClosesBlockedSource(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reader := newBlockingReadCloser()
	type parseResult struct {
		err error
	}
	result := make(chan parseResult, 1)
	go func() {
		_, _, err := parseCodexUsageMetricsReadCloser(ctx, reader)
		result <- parseResult{err: err}
	}()
	select {
	case <-reader.started:
	case <-time.After(time.Second):
		t.Fatal("parser did not start reading")
	}
	cancel()
	select {
	case got := <-result:
		if !errors.Is(got.err, context.Canceled) {
			t.Fatalf("parse error = %v, want context cancellation", got.err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not interrupt parsing")
	}
	if !reader.isClosed() {
		t.Fatal("owned metrics source was not closed")
	}
}

func TestParseCodexUsageMetricsFileReportsUnreadableArtifact(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.jsonl")
	_, found, err := ParseCodexUsageMetricsFile(context.Background(), missing)
	if found || !errors.Is(err, ErrCodexJSONLSource) || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("found = %t, error = %v, want source/not-exist errors", found, err)
	}
	var sourceErr *CodexJSONLSourceError
	if !errors.As(err, &sourceErr) || sourceErr.Operation != "open" || sourceErr.Path != missing {
		t.Fatalf("source diagnostic = %#v", err)
	}
}

func TestParseCodexUsageMetricsFileLeavesMalformedAuditStreamUntouched(t *testing.T) {
	path := filepath.Join(t.TempDir(), "codex.jsonl")
	original := []byte("{\"usage\":{\"input_tokens\":3}}\nnot-json\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	metrics, found, err := ParseCodexUsageMetricsFile(context.Background(), path)
	if !found || metrics.InputTokens != 3 || !errors.Is(err, ErrMalformedCodexJSONL) {
		t.Fatalf("metrics/found/error = %+v/%t/%v", metrics, found, err)
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(after) != string(original) {
		t.Fatalf("authoritative JSONL changed during degraded metrics parsing")
	}
}

func BenchmarkParseCodexUsageMetricsLargeGeneratedStream(b *testing.B) {
	event := `{"usage":{"input_tokens":1,"output_tokens":1}}`
	record := []byte(event + strings.Repeat(" ", 256*1024-len(event)-1) + "\n")
	const records = 64
	b.ReportAllocs()
	b.SetBytes(int64(len(record) * records))
	for i := 0; i < b.N; i++ {
		reader := &repeatingRecordReader{record: record, remaining: records}
		if _, _, err := ParseCodexUsageMetricsReader(context.Background(), reader); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseCodexUsageMetricsMultiGiBGeneratedStream(b *testing.B) {
	record := []byte("{}" + strings.Repeat(" ", jsonl.MaxRecordBytes-3) + "\n")
	const records = 2049
	b.ReportAllocs()
	b.SetBytes(int64(len(record)) * records)
	for i := 0; i < b.N; i++ {
		reader := &repeatingRecordReader{record: record, remaining: records}
		if _, _, err := ParseCodexUsageMetricsReader(context.Background(), reader); err != nil {
			b.Fatal(err)
		}
	}
}

type repeatingRecordReader struct {
	record     []byte
	remaining  int
	offset     int
	bytesRead  int64
	maxRequest int
}

func (r *repeatingRecordReader) Read(p []byte) (int, error) {
	if len(p) > r.maxRequest {
		r.maxRequest = len(p)
	}
	if r.remaining == 0 {
		return 0, io.EOF
	}
	written := 0
	for written < len(p) && r.remaining > 0 {
		copied := copy(p[written:], r.record[r.offset:])
		written += copied
		r.offset += copied
		if r.offset == len(r.record) {
			r.offset = 0
			r.remaining--
		}
	}
	r.bytesRead += int64(written)
	return written, nil
}

func overflowIntegerText() string {
	if strconv.IntSize == 32 {
		return strconv.FormatInt(int64(maxIntValue())+1, 10)
	}
	return "9223372036854775808"
}

type blockingReadCloser struct {
	started  chan struct{}
	closed   chan struct{}
	once     sync.Once
	mu       sync.Mutex
	didClose bool
}

func newBlockingReadCloser() *blockingReadCloser {
	return &blockingReadCloser{started: make(chan struct{}), closed: make(chan struct{})}
}

func (r *blockingReadCloser) Read([]byte) (int, error) {
	r.once.Do(func() { close(r.started) })
	<-r.closed
	return 0, io.ErrClosedPipe
}

func (r *blockingReadCloser) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.didClose {
		r.didClose = true
		close(r.closed)
	}
	return nil
}

func (r *blockingReadCloser) isClosed() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.didClose
}
