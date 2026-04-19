package jsonl

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/pinkhop/nitpicking/internal/domain"
)

// Reader reads backup data from a JSON Lines stream. The first line
// is expected to be a domain.BackupHeader; all subsequent lines are
// domain.BackupIssueRecord values. The caller must call Close when finished.
type Reader struct {
	dec    *json.Decoder
	closer io.Closer
}

// NewReader creates a Reader that reads JSONL from r. Close delegates to
// r's Close method when reading is complete.
func NewReader(r io.ReadCloser) *Reader {
	dec := json.NewDecoder(r)
	// DisallowUnknownFields is intentionally not set — forward
	// compatibility means older readers should tolerate new fields
	// added by newer backup versions.
	return &Reader{
		dec:    dec,
		closer: r,
	}
}

// ReadHeader reads the first JSONL line as the backup metadata header.
// Must be called exactly once before any NextRecord calls.
func (r *Reader) ReadHeader() (domain.BackupHeader, error) {
	var header domain.BackupHeader
	if err := r.dec.Decode(&header); err != nil {
		return domain.BackupHeader{}, fmt.Errorf("reading backup header: %w", err)
	}
	return header, nil
}

// NextRecord returns the next issue record from the backup stream.
// When no more records are available, it returns a zero IssueRecord,
// false, and a nil error. If a parse error occurs, the error is
// non-nil and iteration should stop.
func (r *Reader) NextRecord() (domain.BackupIssueRecord, bool, error) {
	if !r.dec.More() {
		return domain.BackupIssueRecord{}, false, nil
	}

	var record domain.BackupIssueRecord
	if err := r.dec.Decode(&record); err != nil {
		return domain.BackupIssueRecord{}, false, fmt.Errorf("reading backup record: %w", err)
	}
	return record, true, nil
}

// NextRawRecord returns the raw JSON bytes for the next issue record in the
// stream without decoding them into a typed struct. This method is intended
// for callers that need to decode older backup versions whose record schema
// differs from the current domain.BackupIssueRecord (for example, restoring
// a v2 backup that carries the now-removed idempotency_key field).
//
// When no more records are available it returns nil, false, nil. On parse
// error it returns nil, false, and a non-nil error.
func (r *Reader) NextRawRecord() ([]byte, bool, error) {
	if !r.dec.More() {
		return nil, false, nil
	}

	var raw json.RawMessage
	if err := r.dec.Decode(&raw); err != nil {
		return nil, false, fmt.Errorf("reading raw backup record: %w", err)
	}
	return raw, true, nil
}

// Close releases resources held by the reader.
func (r *Reader) Close() error {
	if err := r.closer.Close(); err != nil {
		return fmt.Errorf("closing backup reader: %w", err)
	}
	return nil
}
