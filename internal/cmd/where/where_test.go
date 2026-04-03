package where_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmd/where"
)

// --- Run Tests ---

func TestRun_DiscoveredPath_PrintsParentDirectory(t *testing.T) {
	t.Parallel()

	// Given — a discovery function that returns a full DB file path
	discover := func() (string, error) {
		return "/home/user/project/.np/np.db", nil
	}

	var buf bytes.Buffer
	input := where.RunInput{
		DiscoverFunc: discover,
		WriteTo:      &buf,
	}

	// When
	err := where.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := strings.TrimSpace(buf.String())
	if output != "/home/user/project/.np" {
		t.Errorf("got %q, want %q", output, "/home/user/project/.np")
	}
}

func TestRun_JSON_ReturnsStructuredOutput(t *testing.T) {
	t.Parallel()

	// Given — a discovery function that returns a full DB file path.
	discover := func() (string, error) {
		return "/home/user/project/.np/np.db", nil
	}

	var buf bytes.Buffer
	input := where.RunInput{
		DiscoverFunc: discover,
		JSON:         true,
		WriteTo:      &buf,
	}

	// When
	err := where.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]string
	if jsonErr := json.Unmarshal(buf.Bytes(), &result); jsonErr != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", jsonErr, buf.String())
	}
	if result["path"] != "/home/user/project/.np" {
		t.Errorf("path: got %q, want %q", result["path"], "/home/user/project/.np")
	}
}

func TestRun_NoDatabase_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given — a discovery function that returns an error
	discover := func() (string, error) {
		return "", errors.New("not found")
	}

	var buf bytes.Buffer
	input := where.RunInput{
		DiscoverFunc: discover,
		WriteTo:      &buf,
	}

	// When
	err := where.Run(t.Context(), input)
	// Then
	if err == nil {
		t.Fatal("expected error when no .np/ directory found")
	}
	if !strings.Contains(err.Error(), "no .np/ directory found") {
		t.Errorf("expected error about missing .np/ directory, got: %v", err)
	}
}
