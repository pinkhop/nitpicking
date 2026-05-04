package cmdutil

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"time"
)

// durationTokenRe matches a single number-unit pair at the start of a string.
// The magnitude allows an optional fractional part (e.g. "1.5h") so that the
// parser accepts the full set of magnitudes time.ParseDuration accepts on its
// standard units. Units may be multi-character (e.g. ns, us, ms, µs) or
// single-character (h, m, s, d, w). The µ codepoint is included for
// time.ParseDuration compat. Fractional magnitudes on the extended d/w units
// are rejected explicitly inside ParseExtendedDuration.
var durationTokenRe = regexp.MustCompile(`^(\d+(?:\.\d+)?)([a-zµ]+)`)

// ParseExtendedDuration parses a duration string using Go's standard duration
// syntax extended with d (days = 24h) and w (weeks = 7 × 24h). Compound forms
// such as 1w3d, 1d12h, and 1w3d2h are accepted by summing each token from
// left to right. Standard Go units (ns, us/µs, ms, s, m, h) are handled by
// time.ParseDuration. An empty string, an unrecognised token, or an arithmetic
// overflow returns a non-nil error — matching the safety guarantees of the
// stdlib parser.
func ParseExtendedDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, fmt.Errorf("invalid duration %q: empty string", s)
	}
	var total time.Duration
	rest := s
	for rest != "" {
		m := durationTokenRe.FindStringSubmatch(rest)
		if m == nil {
			return 0, fmt.Errorf("invalid duration %q: unexpected token at %q", s, rest)
		}
		numStr, unit := m[1], m[2]
		var part time.Duration
		switch unit {
		case "w", "d":
			// Extended units use int64 arithmetic; fractional magnitudes are
			// rejected so callers get a clear error rather than a silent
			// truncation. Standard Go units accept fractional magnitudes via
			// the time.ParseDuration path below.
			val, parseErr := strconv.ParseInt(numStr, 10, 64)
			if parseErr != nil {
				return 0, fmt.Errorf("invalid duration %q: %s units must be a non-negative integer, got %q", s, unit, numStr)
			}
			unitDur := 24 * time.Hour
			if unit == "w" {
				unitDur = 7 * 24 * time.Hour
			}
			p, ok := mulDuration(val, unitDur)
			if !ok {
				return 0, fmt.Errorf("invalid duration %q: numeric overflow at %q", s, m[0])
			}
			part = p
		default:
			d, err := time.ParseDuration(m[0])
			if err != nil {
				return 0, fmt.Errorf("invalid duration %q: %w", s, err)
			}
			part = d
		}
		sum, ok := addDuration(total, part)
		if !ok {
			return 0, fmt.Errorf("invalid duration %q: numeric overflow", s)
		}
		total = sum
		rest = rest[len(m[0]):]
	}
	return total, nil
}

// mulDuration multiplies val by unit and reports overflow.
func mulDuration(val int64, unit time.Duration) (time.Duration, bool) {
	if val == 0 {
		return 0, true
	}
	if val > 0 && val > int64(math.MaxInt64)/int64(unit) {
		return 0, false
	}
	return time.Duration(val) * unit, true
}

// addDuration adds two durations and reports overflow.
func addDuration(a, b time.Duration) (time.Duration, bool) {
	if b > 0 && a > math.MaxInt64-b {
		return 0, false
	}
	return a + b, true
}
