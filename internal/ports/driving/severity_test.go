package driving_test

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// TestParseDoctorSeverity_ValidValues confirms that the two valid severity
// strings each round-trip through ParseDoctorSeverity.
func TestParseDoctorSeverity_ValidValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  driving.DoctorSeverity
	}{
		{input: "error", want: driving.SeverityError},
		{input: "warning", want: driving.SeverityWarning},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()

			// When
			got, err := driving.ParseDoctorSeverity(tc.input)
			// Then — no error and the returned value equals the constant.
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("ParseDoctorSeverity(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// TestParseDoctorSeverity_InfoRejected confirms that "info" is no longer a
// valid severity value — the two-level model (error, warning) replaced it.
func TestParseDoctorSeverity_InfoRejected(t *testing.T) {
	t.Parallel()

	// When
	_, err := driving.ParseDoctorSeverity("info")

	// Then — an error is returned.
	if err == nil {
		t.Fatal("expected error for removed severity value \"info\", got nil")
	}
}

// TestParseDoctorSeverity_UnknownValue_ReturnsError confirms that any
// unrecognised string produces an error.
func TestParseDoctorSeverity_UnknownValue_ReturnsError(t *testing.T) {
	t.Parallel()

	// When
	_, err := driving.ParseDoctorSeverity("critical")

	// Then
	if err == nil {
		t.Fatal("expected error for unknown severity value")
	}
}

// TestDoctorSeverity_String confirms each constant produces its string label.
func TestDoctorSeverity_String(t *testing.T) {
	t.Parallel()

	cases := []struct {
		severity driving.DoctorSeverity
		want     string
	}{
		{severity: driving.SeverityError, want: "error"},
		{severity: driving.SeverityWarning, want: "warning"},
	}

	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()

			// When
			got := tc.severity.String()

			// Then
			if got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}
