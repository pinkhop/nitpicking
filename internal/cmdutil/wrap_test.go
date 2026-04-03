package cmdutil_test

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
)

func TestWordWrap_ZeroWidth_ReturnsUnchanged(t *testing.T) {
	t.Parallel()

	// Given
	text := "This is a long line that should not be wrapped when width is zero."

	// When
	result := cmdutil.WordWrap(text, 0)

	// Then
	if result != text {
		t.Errorf("expected unchanged text, got %q", result)
	}
}

func TestWordWrap_ShortLine_NoWrap(t *testing.T) {
	t.Parallel()

	// Given
	text := "Short line"

	// When
	result := cmdutil.WordWrap(text, 80)

	// Then
	if result != text {
		t.Errorf("expected %q, got %q", text, result)
	}
}

func TestWordWrap_LongLine_WrapsAtWordBoundary(t *testing.T) {
	t.Parallel()

	// Given
	text := "The quick brown fox jumps over the lazy dog"

	// When
	result := cmdutil.WordWrap(text, 20)

	// Then
	expected := "The quick brown fox\njumps over the lazy\ndog"
	if result != expected {
		t.Errorf("got:\n%s\nwant:\n%s", result, expected)
	}
}

func TestWordWrap_PreservesExistingNewlines(t *testing.T) {
	t.Parallel()

	// Given
	text := "Line one\nLine two that is quite long and should be wrapped\nLine three"

	// When
	result := cmdutil.WordWrap(text, 30)

	// Then — existing newlines preserved, long line wrapped
	if result != "Line one\nLine two that is quite long\nand should be wrapped\nLine three" {
		t.Errorf("got:\n%s", result)
	}
}

func TestWordWrap_SingleLongWord_NotBroken(t *testing.T) {
	t.Parallel()

	// Given — a single word longer than the width
	text := "supercalifragilisticexpialidocious"

	// When
	result := cmdutil.WordWrap(text, 10)

	// Then — long words are not broken mid-word
	if result != text {
		t.Errorf("expected unbroken word, got %q", result)
	}
}
