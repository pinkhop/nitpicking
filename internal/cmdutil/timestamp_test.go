package cmdutil_test

import (
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
)

func TestFormatJSONTimestamp_UTCMillisecondZSuffix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   time.Time
		want string
	}{
		{
			name: "zero value returns empty",
			in:   time.Time{},
			want: "",
		},
		{
			name: "UTC time formatted with milliseconds and Z",
			in:   time.Date(2026, 3, 24, 2, 41, 40, 0, time.UTC),
			want: "2026-03-24T02:41:40.000Z",
		},
		{
			name: "non-UTC time converted to UTC",
			in:   time.Date(2026, 3, 24, 21, 41, 40, 0, time.FixedZone("CDT", -5*3600)),
			want: "2026-03-25T02:41:40.000Z",
		},
		{
			name: "sub-millisecond truncated to milliseconds",
			in:   time.Date(2026, 3, 24, 2, 41, 40, 123456789, time.UTC),
			want: "2026-03-24T02:41:40.123Z",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			got := cmdutil.FormatJSONTimestamp(tc.in)

			// Then
			if got != tc.want {
				t.Errorf("FormatJSONTimestamp(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
