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
		"facets": {"kind": "bug", "area": "auth"}
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
	if tj.Facets["kind"] != "bug" || tj.Facets["area"] != "auth" {
		t.Errorf("facets: got %v, want kind:bug area:auth", tj.Facets)
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
	if tj.Facets != nil {
		t.Errorf("facets: got %v, want nil", tj.Facets)
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

func TestMergeFacetsFromJSON_AllSourcesMerged(t *testing.T) {
	t.Parallel()

	// Given: facets from env, JSON, and flags with distinct keys.
	envFacets := []string{"env-key:env-val"}
	jsonFacets := []string{"json-key:json-val"}
	flagFacets := []string{"flag-key:flag-val"}

	// When
	result := mergeFacetsFromJSON(envFacets, jsonFacets, flagFacets)

	// Then: all three appear.
	if len(result) != 3 {
		t.Fatalf("expected 3 facets, got %d: %v", len(result), result)
	}
	if !slices.Contains(result, "env-key:env-val") {
		t.Errorf("missing env facet in %v", result)
	}
	if !slices.Contains(result, "json-key:json-val") {
		t.Errorf("missing json facet in %v", result)
	}
	if !slices.Contains(result, "flag-key:flag-val") {
		t.Errorf("missing flag facet in %v", result)
	}
}

func TestMergeFacetsFromJSON_FlagOverridesJSON(t *testing.T) {
	t.Parallel()

	// Given: same key in JSON and flags.
	jsonFacets := []string{"kind:bug"}
	flagFacets := []string{"kind:feature"}

	// When
	result := mergeFacetsFromJSON(nil, jsonFacets, flagFacets)

	// Then: flag wins.
	if len(result) != 1 {
		t.Fatalf("expected 1 facet, got %d: %v", len(result), result)
	}
	if result[0] != "kind:feature" {
		t.Errorf("expected kind:feature, got %q", result[0])
	}
}

func TestMergeFacetsFromJSON_JSONOverridesEnv(t *testing.T) {
	t.Parallel()

	// Given: same key in env and JSON.
	envFacets := []string{"kind:bug"}
	jsonFacets := []string{"kind:feature"}

	// When
	result := mergeFacetsFromJSON(envFacets, jsonFacets, nil)

	// Then: JSON wins.
	if len(result) != 1 {
		t.Fatalf("expected 1 facet, got %d: %v", len(result), result)
	}
	if result[0] != "kind:feature" {
		t.Errorf("expected kind:feature, got %q", result[0])
	}
}

func TestMergeFacetsFromJSON_FlagOverridesEnv(t *testing.T) {
	t.Parallel()

	// Given: same key across all three sources.
	envFacets := []string{"kind:env"}
	jsonFacets := []string{"kind:json"}
	flagFacets := []string{"kind:flag"}

	// When
	result := mergeFacetsFromJSON(envFacets, jsonFacets, flagFacets)

	// Then: flag wins over all.
	if len(result) != 1 {
		t.Fatalf("expected 1 facet, got %d: %v", len(result), result)
	}
	if result[0] != "kind:flag" {
		t.Errorf("expected kind:flag, got %q", result[0])
	}
}

func TestJsonFacetsToStrings_ConvertsFacetMap(t *testing.T) {
	t.Parallel()

	// Given
	facets := map[string]string{"kind": "bug", "area": "auth"}

	// When
	result := jsonFacetsToStrings(facets)

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

func TestJsonFacetsToStrings_NilMap_ReturnsNil(t *testing.T) {
	t.Parallel()

	// When
	result := jsonFacetsToStrings(nil)

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
