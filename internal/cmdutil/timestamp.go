package cmdutil

import "time"

// jsonTimestampFormat is the canonical JSON timestamp layout: UTC, millisecond
// precision, literal Z suffix. Example: "2026-03-24T02:41:40.000Z".
const jsonTimestampFormat = "2006-01-02T15:04:05.000Z"

// FormatJSONTimestamp formats t as a UTC timestamp with millisecond precision
// and a Z suffix, suitable for JSON output. Returns the empty string when t
// is the zero value, so callers can use omitzero struct tags to suppress it.
func FormatJSONTimestamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(jsonTimestampFormat)
}
