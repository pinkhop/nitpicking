package where_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmd/admincmd/where"
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

func TestRun_WithPrefix_PrintsPathAndPrefix(t *testing.T) {
	t.Parallel()

	// Given — a discovery function and a prefix function that both succeed.
	discover := func() (string, error) {
		return "/home/user/project/.np/np.db", nil
	}
	getPrefix := func(_ context.Context) (string, error) {
		return "PROJ", nil
	}

	var buf bytes.Buffer
	input := where.RunInput{
		DiscoverFunc: discover,
		PrefixFunc:   getPrefix,
		WriteTo:      &buf,
	}

	// When
	err := where.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "/home/user/project/.np") {
		t.Errorf("output missing path: %q", output)
	}
	if !strings.Contains(output, "PROJ") {
		t.Errorf("output missing prefix %q: %q", "PROJ", output)
	}
}

func TestRun_PrefixUnavailable_PrintsPathOnly(t *testing.T) {
	t.Parallel()

	// Given — a discovery function that succeeds and a prefix function that fails
	// (e.g., uninitialized or pre-migration database).
	discover := func() (string, error) {
		return "/home/user/project/.np/np.db", nil
	}
	getPrefix := func(_ context.Context) (string, error) {
		return "", errors.New("database not initialized")
	}

	var buf bytes.Buffer
	input := where.RunInput{
		DiscoverFunc: discover,
		PrefixFunc:   getPrefix,
		WriteTo:      &buf,
	}

	// When
	err := where.Run(t.Context(), input)
	// Then — command still succeeds; prefix is omitted.
	if err != nil {
		t.Fatalf("unexpected error when prefix unavailable: %v", err)
	}
	output := strings.TrimSpace(buf.String())
	if !strings.Contains(output, "/home/user/project/.np") {
		t.Errorf("output missing path: %q", output)
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

func TestRun_JSON_WithPrefix_IncludesPrefixField(t *testing.T) {
	t.Parallel()

	// Given — both discovery and prefix functions succeed.
	discover := func() (string, error) {
		return "/home/user/project/.np/np.db", nil
	}
	getPrefix := func(_ context.Context) (string, error) {
		return "PROJ", nil
	}

	var buf bytes.Buffer
	input := where.RunInput{
		DiscoverFunc: discover,
		PrefixFunc:   getPrefix,
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
	if result["prefix"] != "PROJ" {
		t.Errorf("prefix: got %q, want %q", result["prefix"], "PROJ")
	}
}

func TestRun_JSON_PrefixUnavailable_OmitsPrefixField(t *testing.T) {
	t.Parallel()

	// Given — discovery succeeds but prefix function returns an error.
	discover := func() (string, error) {
		return "/home/user/project/.np/np.db", nil
	}
	getPrefix := func(_ context.Context) (string, error) {
		return "", errors.New("database not initialized")
	}

	var buf bytes.Buffer
	input := where.RunInput{
		DiscoverFunc: discover,
		PrefixFunc:   getPrefix,
		JSON:         true,
		WriteTo:      &buf,
	}

	// When
	err := where.Run(t.Context(), input)
	// Then — command still succeeds; the JSON contains path but omits prefix.
	if err != nil {
		t.Fatalf("unexpected error when prefix unavailable: %v", err)
	}

	var result map[string]any
	if jsonErr := json.Unmarshal(buf.Bytes(), &result); jsonErr != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", jsonErr, buf.String())
	}
	if _, ok := result["prefix"]; ok {
		t.Errorf("expected prefix to be absent from JSON when unavailable, but found key: %v", result)
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
