package driven

import "github.com/pinkhop/nitpicking/internal/domain"

// BackupWriter serialises a database snapshot to an external format.
// Implementations must be safe to call sequentially: WriteHeader once,
// then WriteRecord for each issue, then Close.
type BackupWriter interface {
	// WriteHeader writes the backup metadata header. Must be called
	// exactly once before any WriteRecord calls.
	WriteHeader(header domain.BackupHeader) error

	// WriteRecord writes a single issue record to the backup stream.
	WriteRecord(record domain.BackupIssueRecord) error

	// Close flushes any buffered data and releases resources. The
	// caller must call Close after all records have been written.
	Close() error
}

// BackupReader deserialises a backup stream produced by a BackupWriter.
// Implementations must return the header first, then yield records one
// at a time via NextRecord until no more remain.
type BackupReader interface {
	// ReadHeader reads and returns the backup metadata header. Must be
	// called exactly once before any NextRecord calls.
	ReadHeader() (domain.BackupHeader, error)

	// NextRecord returns the next issue record from the backup stream.
	// Returns false when no more records are available. If an error
	// occurs, the second return value is non-nil and iteration should
	// stop.
	NextRecord() (domain.BackupIssueRecord, bool, error)

	// Close releases resources held by the reader. The caller must
	// call Close when done reading, even if NextRecord returned an
	// error.
	Close() error
}
