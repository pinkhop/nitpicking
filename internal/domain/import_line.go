package domain

// RawLine represents a single parsed but unvalidated line from a JSONL import
// file. Field names match the JSON schema defined in the JSONL import format
// specification. All fields are strings or string collections to defer
// validation to the Validate function.
type RawLine struct {
	// IdempotencyLabel is the full "key:value" label string used for deduplication.
	// It is required on every import line.
	IdempotencyLabel   string            `json:"idempotency_label"`
	Role               string            `json:"role"`
	Title              string            `json:"title"`
	Description        string            `json:"description"`
	AcceptanceCriteria string            `json:"acceptance_criteria"`
	Priority           string            `json:"priority"`
	State              string            `json:"state"`
	Author             string            `json:"author"`
	Comment            string            `json:"comment"`
	Claim              bool              `json:"claim"`
	Labels             map[string]string `json:"labels"`
	Parent             string            `json:"parent"`
	BlockedBy          []string          `json:"blocked_by"`
	Blocks             []string          `json:"blocks"`
	Refs               []string          `json:"refs"`
}
