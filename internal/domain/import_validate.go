package domain

import (
	"fmt"
	"strings"
)

// reservedLabelKey is the label key reserved for the idempotency_key virtual
// label. It must not appear in the labels object of an import line.
const reservedLabelKey = "idempotency-key"

// importableStates lists the states that are valid for import. The states
// "claimed" and "blocked" are excluded: "claimed" requires an active claim,
// and "blocked" is a secondary state.
var importableStates = map[string]bool{
	"open":     true,
	"deferred": true,
	"closed":   true,
}

// Validate performs a two-pass validation of the given import lines.
//
// Pass 1 collects all idempotency keys and their line indices, detecting
// duplicates. Pass 2 validates each line's fields and resolves references.
//
// The prefix parameter is the database's issue ID prefix (e.g., "NP"),
// used to distinguish issue ID references from idempotency key references.
func Validate(lines []RawLine, prefix string) ValidationResult {
	var result ValidationResult

	// Pass 1: collect idempotency keys and detect duplicates.
	keyIndex := buildKeyIndex(lines, &result)

	// Pass 2: validate each line.
	for i, line := range lines {
		record, errs := validateLine(i, line, prefix, keyIndex, lines)
		result.Errors = append(result.Errors, errs...)
		if len(errs) == 0 {
			result.Records = append(result.Records, record)
		}
	}

	return result
}

// buildKeyIndex maps each idempotency key to its first occurrence line index.
// Duplicate keys generate errors on the second and subsequent occurrences.
func buildKeyIndex(lines []RawLine, result *ValidationResult) map[string]int {
	keyIndex := make(map[string]int, len(lines))
	for i, line := range lines {
		if line.IdempotencyKey == "" {
			continue // Missing key is caught in validateLine.
		}
		if _, exists := keyIndex[line.IdempotencyKey]; exists {
			result.Errors = append(result.Errors, LineError{
				Line:    i,
				Message: fmt.Sprintf("duplicate idempotency_key %q", line.IdempotencyKey),
			})
		} else {
			keyIndex[line.IdempotencyKey] = i
		}
	}
	return keyIndex
}

// validateLine validates a single import line and returns either a validated
// record or a list of errors. All detectable errors are reported — the
// function does not short-circuit on the first failure.
func validateLine(idx int, line RawLine, prefix string, keyIndex map[string]int, allLines []RawLine) (ValidatedRecord, []LineError) {
	var errs []LineError
	var record ValidatedRecord

	// Required fields.
	if line.IdempotencyKey == "" {
		errs = append(errs, LineError{Line: idx, Message: "idempotency_key is required"})
	} else {
		record.IdempotencyKey = line.IdempotencyKey
	}

	// Role.
	if line.Role == "" {
		errs = append(errs, LineError{Line: idx, Message: "role is required"})
	} else {
		parsed, err := ParseRole(line.Role)
		if err != nil {
			errs = append(errs, LineError{Line: idx, Message: fmt.Sprintf("invalid role: %s", err)})
		} else {
			record.Role = parsed
		}
	}

	// Title.
	if line.Title == "" {
		errs = append(errs, LineError{Line: idx, Message: "title is required"})
	} else {
		record.Title = line.Title
	}

	// Optional text fields — pass through without validation.
	record.Description = line.Description
	record.AcceptanceCriteria = line.AcceptanceCriteria
	record.Comment = line.Comment

	// Priority (defaults to P2).
	if line.Priority == "" {
		record.Priority = DefaultPriority
	} else {
		parsed, err := ParsePriority(line.Priority)
		if err != nil {
			errs = append(errs, LineError{Line: idx, Message: fmt.Sprintf("invalid priority: %s", err)})
		} else {
			record.Priority = parsed
		}
	}

	// State (defaults to open; only open, deferred, closed are valid).
	if line.State == "" {
		record.State = StateOpen
	} else if !importableStates[line.State] {
		errs = append(errs, LineError{
			Line:    idx,
			Message: fmt.Sprintf("invalid state %q: must be open, deferred, or closed", line.State),
		})
	} else {
		parsed, err := ParseState(line.State)
		if err != nil {
			errs = append(errs, LineError{Line: idx, Message: fmt.Sprintf("invalid state: %s", err)})
		} else {
			record.State = parsed
		}
	}

	// Author (optional; validated only if non-empty).
	if line.Author != "" {
		if _, err := NewAuthor(line.Author); err != nil {
			errs = append(errs, LineError{Line: idx, Message: fmt.Sprintf("invalid author: %s", err)})
		} else {
			record.Author = line.Author
		}
	}

	// Labels.
	record.Labels = validateLabels(idx, line.Labels, &errs)

	// References.
	record.Parent = line.Parent
	record.BlockedBy = line.BlockedBy
	record.Blocks = line.Blocks
	record.Refs = line.Refs

	validateReferences(idx, line, prefix, keyIndex, allLines, &errs)

	return record, errs
}

// validateLabels checks each label key/value pair and rejects the reserved
// idempotency-key label key.
func validateLabels(idx int, labels map[string]string, errs *[]LineError) []Label {
	if len(labels) == 0 {
		return nil
	}

	var result []Label
	for key, value := range labels {
		if key == reservedLabelKey {
			*errs = append(*errs, LineError{
				Line:    idx,
				Message: fmt.Sprintf("label key %q is reserved; use the top-level idempotency_key field instead", reservedLabelKey),
			})
			continue
		}
		label, err := NewLabel(key, value)
		if err != nil {
			*errs = append(*errs, LineError{Line: idx, Message: fmt.Sprintf("invalid label %q: %s", key+":"+value, err)})
			continue
		}
		result = append(result, label)
	}
	return result
}

// validateReferences checks that all reference strings (parent, blocked_by,
// blocks, refs) resolve to either an intra-file idempotency key or a valid
// issue ID format. Parent references are additionally checked to ensure the
// target has role "epic" (for intra-file references).
func validateReferences(idx int, line RawLine, prefix string, keyIndex map[string]int, allLines []RawLine, errs *[]LineError) {
	// Parent.
	if line.Parent != "" {
		if isIssueIDFormat(line.Parent, prefix) {
			// Issue ID format — accepted without further validation.
			// The import pass will verify the issue exists and is an epic.
		} else if targetIdx, ok := keyIndex[line.Parent]; ok {
			// Intra-file reference — check that the target is an epic.
			if strings.ToLower(allLines[targetIdx].Role) != "epic" {
				*errs = append(*errs, LineError{
					Line:    idx,
					Message: fmt.Sprintf("parent reference %q resolves to a %s, not an epic", line.Parent, allLines[targetIdx].Role),
				})
			}
		} else {
			*errs = append(*errs, LineError{
				Line:    idx,
				Message: fmt.Sprintf("unresolvable parent reference %q: not an intra-file idempotency key or valid issue ID", line.Parent),
			})
		}
	}

	// blocked_by.
	for _, ref := range line.BlockedBy {
		validateRef(idx, "blocked_by", ref, prefix, keyIndex, errs)
	}

	// blocks.
	for _, ref := range line.Blocks {
		validateRef(idx, "blocks", ref, prefix, keyIndex, errs)
	}

	// refs.
	for _, ref := range line.Refs {
		validateRef(idx, "refs", ref, prefix, keyIndex, errs)
	}
}

// validateRef checks that a single reference string resolves to either an
// intra-file idempotency key or a valid issue ID format.
func validateRef(idx int, field, ref, prefix string, keyIndex map[string]int, errs *[]LineError) {
	if ref == "" {
		*errs = append(*errs, LineError{
			Line:    idx,
			Message: fmt.Sprintf("empty reference in %s", field),
		})
		return
	}

	if isIssueIDFormat(ref, prefix) {
		return // Issue ID format — accepted.
	}

	if _, ok := keyIndex[ref]; ok {
		return // Intra-file reference — resolved.
	}

	*errs = append(*errs, LineError{
		Line:    idx,
		Message: fmt.Sprintf("unresolvable %s reference %q: not an intra-file idempotency key or valid issue ID", field, ref),
	})
}

// isIssueIDFormat reports whether s matches the issue ID format for the given
// prefix: PREFIX-<5 Crockford Base32 chars>. This is a format check only —
// it does not verify the issue exists in the database.
func isIssueIDFormat(s, prefix string) bool {
	parsed, err := ParseID(s)
	if err != nil {
		return false
	}
	return parsed.Prefix() == prefix
}
