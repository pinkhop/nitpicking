package domain

// ValidatedRecord is a successfully validated import line with parsed domain
// types. It retains all data needed by the import pass to create issues and
// relationships without re-parsing.
type ValidatedRecord struct {
	IdempotencyKey     string
	Role               Role
	Title              string
	Description        string
	AcceptanceCriteria string
	Priority           Priority
	State              State
	Author             string
	Comment            string
	Labels             []Label
	Parent             string
	BlockedBy          []string
	Blocks             []string
	Refs               []string
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
