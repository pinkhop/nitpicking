package create

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

func TestParseIssueJSON_ValidFullInput(t *testing.T) {
	t.Parallel()

	// Given
	data := []byte(`{
		"role": "task",
		"title": "Fix login bug",
		"description": "Users cannot log in",
		"acceptance_criteria": "Login works",
		"priority": "P0",
		"parent_id": "NP-abc12",
		"dimensions": {"kind": "bug", "area": "auth"}
	}`)

	// When
	tj, err := parseIssueJSON(data)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tj.Role != "task" {
		t.Errorf("role: got %q, want %q", tj.Role, "task")
	}
	if tj.Title != "Fix login bug" {
		t.Errorf("title: got %q, want %q", tj.Title, "Fix login bug")
	}
	if tj.Description != "Users cannot log in" {
		t.Errorf("description: got %q, want %q", tj.Description, "Users cannot log in")
	}
	if tj.AcceptanceCriteria != "Login works" {
		t.Errorf("acceptance_criteria: got %q, want %q", tj.AcceptanceCriteria, "Login works")
	}
	if tj.Priority != "P0" {
		t.Errorf("priority: got %q, want %q", tj.Priority, "P0")
	}
	if tj.ParentID != "NP-abc12" {
		t.Errorf("parent_id: got %q, want %q", tj.ParentID, "NP-abc12")
	}
	if tj.Dimensions["kind"] != "bug" || tj.Dimensions["area"] != "auth" {
		t.Errorf("dimensions: got %v, want kind:bug area:auth", tj.Dimensions)
	}
}

func TestParseIssueJSON_IgnoresExtraFieldsFromShow(t *testing.T) {
	t.Parallel()

	// Given: JSON that includes show-only fields (id, state, revision, etc.).
	data := []byte(`{
		"id": "NP-xyz99",
		"role": "task",
		"title": "Something",
		"state": "claimed",
		"revision": 5,
		"author": "alice",
		"is_ready": true,
		"created_at": "2026-01-01T00:00:00Z"
	}`)

	// When
	tj, err := parseIssueJSON(data)
	// Then: no error; relevant fields extracted, extras silently dropped.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tj.Role != "task" {
		t.Errorf("role: got %q, want %q", tj.Role, "task")
	}
	if tj.Title != "Something" {
		t.Errorf("title: got %q, want %q", tj.Title, "Something")
	}
}

func TestParseIssueJSON_InvalidJSON_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	data := []byte(`{not valid json}`)

	// When
	_, err := parseIssueJSON(data)

	// Then
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), "--from-json") {
		t.Errorf("error should mention --from-json, got: %v", err)
	}
}

func TestParseIssueJSON_MinimalInput(t *testing.T) {
	t.Parallel()

	// Given: only required fields.
	data := []byte(`{"role": "epic", "title": "Auth overhaul"}`)

	// When
	tj, err := parseIssueJSON(data)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tj.Role != "epic" {
		t.Errorf("role: got %q, want %q", tj.Role, "epic")
	}
	if tj.Title != "Auth overhaul" {
		t.Errorf("title: got %q, want %q", tj.Title, "Auth overhaul")
	}
	if tj.Description != "" {
		t.Errorf("description: got %q, want empty", tj.Description)
	}
	if tj.Dimensions != nil {
		t.Errorf("dimensions: got %v, want nil", tj.Dimensions)
	}
}

func TestReadJSONSource_InlineValue(t *testing.T) {
	t.Parallel()

	// Given
	value := `{"role": "task", "title": "Inline"}`
	stdin := strings.NewReader("should not be read")

	// When
	data, err := readJSONSource(value, stdin)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != value {
		t.Errorf("got %q, want %q", string(data), value)
	}
}

func TestReadJSONSource_Stdin(t *testing.T) {
	t.Parallel()

	// Given
	stdinContent := `{"role": "task", "title": "From stdin"}`
	stdin := strings.NewReader(stdinContent)

	// When
	data, err := readJSONSource("-", stdin)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != stdinContent {
		t.Errorf("got %q, want %q", string(data), stdinContent)
	}
}

func TestReadJSONSource_StdinError(t *testing.T) {
	t.Parallel()

	// Given: a reader that always fails.
	stdin := &errorReader{err: bytes.ErrTooLarge}

	// When
	_, err := readJSONSource("-", stdin)

	// Then
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "stdin") {
		t.Errorf("error should mention stdin, got: %v", err)
	}
}

func TestMergeDimensionsFromJSON_AllSourcesMerged(t *testing.T) {
	t.Parallel()

	// Given: dimensions from env, JSON, and flags with distinct keys.
	envDimensions := []string{"env-key:env-val"}
	jsonDimensions := []string{"json-key:json-val"}
	flagDimensions := []string{"flag-key:flag-val"}

	// When
	result := mergeDimensionsFromJSON(envDimensions, jsonDimensions, flagDimensions)

	// Then: all three appear.
	if len(result) != 3 {
		t.Fatalf("expected 3 dimensions, got %d: %v", len(result), result)
	}
	if !slices.Contains(result, "env-key:env-val") {
		t.Errorf("missing env dimension in %v", result)
	}
	if !slices.Contains(result, "json-key:json-val") {
		t.Errorf("missing json dimension in %v", result)
	}
	if !slices.Contains(result, "flag-key:flag-val") {
		t.Errorf("missing flag dimension in %v", result)
	}
}

func TestMergeDimensionsFromJSON_FlagOverridesJSON(t *testing.T) {
	t.Parallel()

	// Given: same key in JSON and flags.
	jsonDimensions := []string{"kind:bug"}
	flagDimensions := []string{"kind:feature"}

	// When
	result := mergeDimensionsFromJSON(nil, jsonDimensions, flagDimensions)

	// Then: flag wins.
	if len(result) != 1 {
		t.Fatalf("expected 1 dimension, got %d: %v", len(result), result)
	}
	if result[0] != "kind:feature" {
		t.Errorf("expected kind:feature, got %q", result[0])
	}
}

func TestMergeDimensionsFromJSON_JSONOverridesEnv(t *testing.T) {
	t.Parallel()

	// Given: same key in env and JSON.
	envDimensions := []string{"kind:bug"}
	jsonDimensions := []string{"kind:feature"}

	// When
	result := mergeDimensionsFromJSON(envDimensions, jsonDimensions, nil)

	// Then: JSON wins.
	if len(result) != 1 {
		t.Fatalf("expected 1 dimension, got %d: %v", len(result), result)
	}
	if result[0] != "kind:feature" {
		t.Errorf("expected kind:feature, got %q", result[0])
	}
}

func TestMergeDimensionsFromJSON_FlagOverridesEnv(t *testing.T) {
	t.Parallel()

	// Given: same key across all three sources.
	envDimensions := []string{"kind:env"}
	jsonDimensions := []string{"kind:json"}
	flagDimensions := []string{"kind:flag"}

	// When
	result := mergeDimensionsFromJSON(envDimensions, jsonDimensions, flagDimensions)

	// Then: flag wins over all.
	if len(result) != 1 {
		t.Fatalf("expected 1 dimension, got %d: %v", len(result), result)
	}
	if result[0] != "kind:flag" {
		t.Errorf("expected kind:flag, got %q", result[0])
	}
}

func TestJsonDimensionsToStrings_ConvertsDimensionMap(t *testing.T) {
	t.Parallel()

	// Given
	dimensions := map[string]string{"kind": "bug", "area": "auth"}

	// When
	result := jsonLabelsToStrings(dimensions)

	// Then
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}
	slices.Sort(result)
	if result[0] != "area:auth" {
		t.Errorf("expected area:auth, got %q", result[0])
	}
	if result[1] != "kind:bug" {
		t.Errorf("expected kind:bug, got %q", result[1])
	}
}

func TestJsonDimensionsToStrings_NilMap_ReturnsNil(t *testing.T) {
	t.Parallel()

	// When
	result := jsonLabelsToStrings(nil)

	// Then
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

// errorReader is a stub io.Reader that always returns an error.
type errorReader struct {
	err error
}

func (r *errorReader) Read([]byte) (int, error) {
	return 0, r.err
}
