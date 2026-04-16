// Package cmdutil provides shared parsing helpers for CLI commands.

package cmdutil

import (
	"fmt"
	"strings"

	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// orderNames lists the valid --order flag values in canonical display order,
// matching the column names from ValidColumnNames plus the "modified" alias.
// This slice is the single source of truth for help text and error messages.
var orderNames = []string{
	"ID", "CREATED", "PARENT_ID", "PARENT_CREATED",
	"PRIORITY", "ROLE", "STATE", "TITLE", "MODIFIED",
}

// ValidOrderNames returns a deterministic, comma-separated list of valid
// --order flag values for use in help text and error messages.
func ValidOrderNames() string {
	return strings.Join(orderNames, ", ")
}

// ParseOrderBy converts a user-supplied sort string into a service-layer
// OrderBy value and SortDirection. Valid inputs match the column names
// accepted by --columns (ID, CREATED, PARENT_ID, PARENT_CREATED, PRIORITY,
// ROLE, STATE, TITLE) plus the alias "MODIFIED". An optional ":asc" or
// ":desc" suffix controls the direction; the default is ascending. Matching
// is case-insensitive. An empty string defaults to OrderByPriority ascending.
func ParseOrderBy(s string) (driving.OrderBy, driving.SortDirection, error) {
	trimmed := strings.ToUpper(strings.TrimSpace(s))

	// Strip optional direction suffix.
	dir := driving.SortAscending
	if strings.HasSuffix(trimmed, ":ASC") {
		trimmed = strings.TrimSuffix(trimmed, ":ASC")
	} else if strings.HasSuffix(trimmed, ":DESC") {
		dir = driving.SortDescending
		trimmed = strings.TrimSuffix(trimmed, ":DESC")
	}

	switch trimmed {
	case "", "PRIORITY":
		return driving.OrderByPriority, dir, nil
	case "CREATED":
		return driving.OrderByCreatedAt, dir, nil
	case "MODIFIED":
		return driving.OrderByUpdatedAt, dir, nil
	case "ID":
		return driving.OrderByID, dir, nil
	case "ROLE":
		return driving.OrderByRole, dir, nil
	case "STATE":
		return driving.OrderByState, dir, nil
	case "TITLE":
		return driving.OrderByTitle, dir, nil
	case "PARENT_ID":
		return driving.OrderByParentID, dir, nil
	case "PARENT_CREATED":
		return driving.OrderByParentCreated, dir, nil
	default:
		return 0, driving.SortAscending, fmt.Errorf("invalid sort order %q: valid values: %s (with optional :asc or :desc suffix)", s, ValidOrderNames())
	}
}

// ResolveFlatListOrderBy parses a user-supplied --order flag value for a
// flat listing command (such as "ready" or "blocked") where family-anchored
// priority ordering is not meaningful. It behaves identically to ParseOrderBy
// except that driving.OrderByPriority is remapped to
// driving.OrderByPriorityCreated, so that same-priority issues from different
// parents interleave by creation time rather than grouping under their
// parents. Keying the remap off the parsed OrderBy ensures that all priority
// variants — "priority", "PRIORITY", "priority:asc", "priority:desc",
// whitespace-padded values, and the empty default — are handled consistently.
// An invalid sort order is returned as an error from ParseOrderBy.
func ResolveFlatListOrderBy(s string) (driving.OrderBy, driving.SortDirection, error) {
	orderBy, direction, err := ParseOrderBy(s)
	if err != nil {
		return 0, driving.SortAscending, err
	}
	if orderBy == driving.OrderByPriority {
		orderBy = driving.OrderByPriorityCreated
	}
	return orderBy, direction, nil
}

// ParseLabels parses a slice of "key:value" strings into service-layer
// LabelInput DTOs. Validation of the key and value content is deferred to
// the service layer, which constructs domain Label values internally.
func ParseLabels(raw []string) ([]driving.LabelInput, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	labels := make([]driving.LabelInput, 0, len(raw))
	for _, s := range raw {
		key, value, ok := strings.Cut(s, ":")
		if !ok {
			return nil, fmt.Errorf("invalid label %q: must be in key:value format", s)
		}
		labels = append(labels, driving.LabelInput{Key: key, Value: value})
	}

	return labels, nil
}

// ParseLabelFilters converts a slice of "key:value" strings into service-layer
// LabelFilterInput DTOs. A wildcard value ("*") matches any value for the key
// and is represented by an empty Value field. Negation ("!key:value") is
// indicated by a true Negate field.
func ParseLabelFilters(raw []string) ([]driving.LabelFilterInput, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	filters := make([]driving.LabelFilterInput, 0, len(raw))
	for _, s := range raw {
		negate := false
		expr := s
		if strings.HasPrefix(expr, "!") {
			negate = true
			expr = expr[1:]
		}

		key, value, ok := strings.Cut(expr, ":")
		if !ok {
			return nil, fmt.Errorf("invalid label filter %q: must be in key:value format", s)
		}

		ff := driving.LabelFilterInput{Key: key, Negate: negate}
		if value != "*" {
			ff.Value = value
		}
		filters = append(filters, ff)
	}

	return filters, nil
}
