package domain

import (
	"fmt"
	"strings"
)

// importableStates lists the primary lifecycle states that are valid for
// import. "blocked" is excluded because it is a secondary state computed from
// relationships, not a primary state. "claimed" is excluded because it is also
// a secondary state of open — an active claim is transient, local-only
// bookkeeping and is never a primary lifecycle state.
var importableStates = map[string]bool{
	"open":     true,
	"deferred": true,
	"closed":   true,
}

// Validate performs a two-pass validation of the given import lines.
//
// Pass 1 collects all idempotency labels and their line indices, detecting
// duplicates. Pass 2 validates each line's fields and resolves references.
//
// The prefix parameter is the database's issue ID prefix (e.g., "NP"),
// used to distinguish issue ID references from idempotency label references.
func Validate(lines []RawLine, prefix string) ValidationResult {
	var result ValidationResult

	// Pass 1: collect idempotency labels and detect duplicates.
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

// buildKeyIndex maps each idempotency label string (key:value) to its first
// occurrence line index. Duplicate labels generate errors on the second and
// subsequent occurrences. Lines whose idempotency_label is missing or
// malformed are skipped here and caught in validateLine.
func buildKeyIndex(lines []RawLine, result *ValidationResult) map[string]int {
	keyIndex := make(map[string]int, len(lines))
	for i, line := range lines {
		if line.IdempotencyLabel == "" {
			continue // Missing label is caught in validateLine.
		}
		// Only index well-formed "key:value" strings so that the key index
		// contains only labels that will actually resolve in reference lookups.
		if _, _, ok := strings.Cut(line.IdempotencyLabel, ":"); !ok {
			continue // Malformed label is caught in validateLine.
		}
		if _, exists := keyIndex[line.IdempotencyLabel]; exists {
			result.Errors = append(result.Errors, LineError{
				Line:    i,
				Message: fmt.Sprintf("duplicate idempotency_label %q", line.IdempotencyLabel),
			})
		} else {
			keyIndex[line.IdempotencyLabel] = i
		}
	}
	return keyIndex
}

// parseIdempotencyLabel parses a "key:value" string into a domain Label.
// It returns a non-nil error if the string is empty, missing the colon
// separator, or contains an invalid key or value.
func parseIdempotencyLabel(raw string) (Label, error) {
	if raw == "" {
		return Label{}, fmt.Errorf("idempotency_label is required")
	}
	key, value, ok := strings.Cut(raw, ":")
	if !ok {
		return Label{}, fmt.Errorf("idempotency_label must be in key:value format, got %q", raw)
	}
	label, err := NewLabel(key, value)
	if err != nil {
		return Label{}, fmt.Errorf("idempotency_label %q: %w", raw, err)
	}
	return label, nil
}

// validateLine validates a single import line and returns either a validated
// record or a list of errors. All detectable errors are reported — the
// function does not short-circuit on the first failure.
func validateLine(idx int, line RawLine, prefix string, keyIndex map[string]int, allLines []RawLine) (ValidatedRecord, []LineError) {
	var errs []LineError
	var record ValidatedRecord

	// Required field: idempotency_label.
	idempotencyLabel, err := parseIdempotencyLabel(line.IdempotencyLabel)
	if err != nil {
		errs = append(errs, LineError{Line: idx, Message: err.Error()})
	} else {
		record.IdempotencyLabel = idempotencyLabel
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

	// Claim (only valid for open state; creates a transient claim row).
	if line.Claim {
		if record.State != StateOpen {
			errs = append(errs, LineError{
				Line:    idx,
				Message: fmt.Sprintf("claim: true is only valid for state open, got %q", line.State),
			})
		} else {
			record.Claim = true
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

	// Labels — validate the labels map, then apply the cross-field rule against
	// idempotency_label. The cross-field check is only performed when both the
	// label list and the idempotency label parsed cleanly.
	record.Labels = validateLabels(idx, line.Labels, &errs)
	if err == nil {
		validateIdempotencyLabelConflict(idx, idempotencyLabel, line.Labels, &errs)
	}

	// References.
	record.Parent = line.Parent
	record.BlockedBy = line.BlockedBy
	record.Blocks = line.Blocks
	record.Refs = line.Refs

	validateReferences(idx, line, prefix, keyIndex, allLines, &errs)

	return record, errs
}

// validateIdempotencyLabelConflict checks that the idempotency_label does not
// conflict with an entry in the labels map. If the labels map contains the
// same key as the idempotency_label with a different value, the record is
// rejected — it is ambiguous which value the author intended. If the labels
// map contains the same key with the same value (a no-op duplicate), validation
// passes silently; deduplication is handled downstream.
func validateIdempotencyLabelConflict(idx int, idempotencyLabel Label, labels map[string]string, errs *[]LineError) {
	labelValue, exists := labels[idempotencyLabel.Key()]
	if !exists {
		return
	}
	if labelValue != idempotencyLabel.Value() {
		*errs = append(*errs, LineError{
			Line: idx,
			Message: fmt.Sprintf(
				"idempotency_label key %q conflicts with labels: idempotency_label has value %q but labels has value %q",
				idempotencyLabel.Key(), idempotencyLabel.Value(), labelValue,
			),
		})
	}
	// Equal values: silent no-op — downstream deduplication handles it.
}

// validateLabels checks each label key/value pair for validity.
func validateLabels(idx int, labels map[string]string, errs *[]LineError) []Label {
	if len(labels) == 0 {
		return nil
	}

	var result []Label
	for key, value := range labels {
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
// blocks, refs) resolve to either an intra-file idempotency label or a valid
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
				Message: fmt.Sprintf("unresolvable parent reference %q: not an intra-file idempotency label or valid issue ID", line.Parent),
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
// intra-file idempotency label or a valid issue ID format.
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
		Message: fmt.Sprintf("unresolvable %s reference %q: not an intra-file idempotency label or valid issue ID", field, ref),
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
