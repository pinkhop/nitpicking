package domain

// ValidatedRecord is a successfully validated import line with parsed domain
// types. It retains all data needed by the import pass to create issues and
// relationships without re-parsing.
type ValidatedRecord struct {
	// IdempotencyLabel is the parsed label used for deduplication. Its string
	// form (Key():Value()) is also the intra-file reference key used to resolve
	// parent, blocked_by, blocks, and refs fields within the same import file.
	IdempotencyLabel   Label
	Role               Role
	Title              string
	Description        string
	AcceptanceCriteria string
	Priority           Priority
	State              State
	Author             string
	Comment            string
	// Claim indicates the imported issue should be immediately claimed after
	// creation. Only valid when State is open; the import service returns an
	// error if Claim is true for a non-open record.
	Claim     bool
	Labels    []Label
	Parent    string
	BlockedBy []string
	Blocks    []string
	Refs      []string
}

// ValidationResult holds the outcome of validating an entire import file.
// Errors contains per-line validation failures. Records contains the
// successfully validated lines — only populated for lines with no errors.
type ValidationResult struct {
	Errors  []LineError
	Records []ValidatedRecord
}

// HasErrors reports whether the validation produced any errors.
func (r ValidationResult) HasErrors() bool {
	return len(r.Errors) > 0
}
