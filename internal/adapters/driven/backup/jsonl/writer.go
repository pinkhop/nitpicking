package jsonl

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/pinkhop/nitpicking/internal/domain"
)

// Writer writes backup data in JSON Lines format. Each call to
// WriteHeader or WriteRecord produces exactly one line of JSON
// followed by a newline character. The caller must call Close when
// finished to flush any buffered data.
type Writer struct {
	enc    *json.Encoder
	closer io.Closer
}

// NewWriter creates a Writer that writes JSONL to w. Close delegates to
// w's Close method after all writes are complete.
func NewWriter(w io.WriteCloser) *Writer {
	enc := json.NewEncoder(w)
	// Disable HTML escaping — backup data may contain angle brackets
	// in issue titles or descriptions, and escaping them would alter
	// the stored text on round-trip.
	enc.SetEscapeHTML(false)
	return &Writer{
		enc:    enc,
		closer: w,
	}
}

// WriteHeader writes the backup metadata header as the first JSONL
// line. Must be called exactly once before any WriteRecord calls.
func (w *Writer) WriteHeader(header domain.BackupHeader) error {
	if err := w.enc.Encode(header); err != nil {
		return fmt.Errorf("writing backup header: %w", err)
	}
	return nil
}

// WriteRecord writes a single issue record as one JSONL line.
func (w *Writer) WriteRecord(record domain.BackupIssueRecord) error {
	if err := w.enc.Encode(record); err != nil {
		return fmt.Errorf("writing backup record %s: %w", record.IssueID, err)
	}
	return nil
}

// Close flushes any buffered output and closes the underlying writer.
func (w *Writer) Close() error {
	if err := w.closer.Close(); err != nil {
		return fmt.Errorf("closing backup writer: %w", err)
	}
	return nil
}
