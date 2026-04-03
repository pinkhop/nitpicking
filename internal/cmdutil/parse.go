// Package cmdutil provides shared parsing helpers for CLI commands.

package cmdutil

import (
	"fmt"
	"strings"

	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// ParseOrderBy converts a user-supplied sort string into a service-layer
// OrderBy value. Valid inputs: "priority" (default when empty), "created",
// "modified". Matching is case-insensitive.
func ParseOrderBy(s string) (driving.OrderBy, error) {
	switch strings.ToLower(s) {
	case "", "priority":
		return driving.OrderByPriority, nil
	case "created":
		return driving.OrderByCreatedAt, nil
	case "modified":
		return driving.OrderByUpdatedAt, nil
	default:
		return 0, fmt.Errorf("invalid sort order %q: must be priority, created, or modified", s)
	}
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
