package core

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// TestCloseCompletedFixFn_MetaTrue_IncludesTasksFlag verifies that when Meta
// is true the command includes --include-tasks.
func TestCloseCompletedFixFn_MetaTrue_IncludesTasksFlag(t *testing.T) {
	t.Parallel()

	// Given — Meta signals at least one closable issue is a task.
	result := &doctorRunResult{Meta: true}

	// When
	fix := closeCompletedFixFn(result)

	// Then
	const want = "np epic close-completed --include-tasks"
	if fix.Command != want {
		t.Errorf("Fix.Command: got %q, want %q", fix.Command, want)
	}
}

// TestCloseCompletedFixFn_MetaFalse_OmitsTasksFlag verifies that when Meta is
// false the command omits --include-tasks.
func TestCloseCompletedFixFn_MetaFalse_OmitsTasksFlag(t *testing.T) {
	t.Parallel()

	// Given — Meta signals no closable tasks.
	result := &doctorRunResult{Meta: false}

	// When
	fix := closeCompletedFixFn(result)

	// Then
	const want = "np epic close-completed"
	if fix.Command != want {
		t.Errorf("Fix.Command: got %q, want %q", fix.Command, want)
	}
}

// TestCloseCompletedFixFn_MetaNil_OmitsTasksFlag verifies that when Meta is
// nil (unset) the command omits --include-tasks.
func TestCloseCompletedFixFn_MetaNil_OmitsTasksFlag(t *testing.T) {
	t.Parallel()

	// Given — Meta not populated.
	result := &doctorRunResult{Meta: nil}

	// When
	fix := closeCompletedFixFn(result)

	// Then
	const want = "np epic close-completed"
	if fix.Command != want {
		t.Errorf("Fix.Command: got %q, want %q", fix.Command, want)
	}
}

// TestCloseCompletedFixFn_MetaWrongType_OmitsTasksFlag verifies that a
// non-bool Meta value does not trigger the --include-tasks branch.
func TestCloseCompletedFixFn_MetaWrongType_OmitsTasksFlag(t *testing.T) {
	t.Parallel()

	// Given — Meta is a string, which is not a bool.
	result := &doctorRunResult{Meta: "unexpected"}

	// When
	fix := closeCompletedFixFn(result)

	// Then — wrong type should not panic and should not include --include-tasks.
	const want = "np epic close-completed"
	if fix.Command != want {
		t.Errorf("Fix.Command: got %q, want %q", fix.Command, want)
	}
}

// Ensure closeCompletedFixFn satisfies the FixFn signature expected by
// doctorCheckEntry.
var _ func(*doctorRunResult) driving.DoctorFix = closeCompletedFixFn
