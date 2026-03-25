//go:build e2e

package e2e_test

import (
	"encoding/json"
	"regexp"
	"testing"
)

func TestE2E_CreateJSON_TaskConformsToJSONStandards(t *testing.T) {
	// Given — a fresh database.
	dir := initDB(t, "CJ")

	// When — create a task with JSON output.
	stdout, stderr, code := runNP(t, dir, "create",
		"--role", "task",
		"--title", "Create JSON audit task",
		"--author", "create-agent",
		"--json",
	)

	// Then — the JSON body conforms to all standards.
	if code != 0 {
		t.Fatalf("create --json failed (exit %d): %s", code, stderr)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		t.Fatalf("invalid JSON: %v\nstdout: %s", err, stdout)
	}

	// AC: All keys are snake_case.
	snakeCase := regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)
	for key := range raw {
		if !snakeCase.MatchString(key) {
			t.Errorf("key %q is not snake_case", key)
		}
	}

	result := parseJSON(t, stdout)

	// AC: ID is a non-empty string.
	id, ok := result["id"].(string)
	if !ok || id == "" {
		t.Errorf("id must be a non-empty string, got %v", result["id"])
	}

	// AC: Role is a human-readable string.
	if role, ok := result["role"].(string); !ok || role != "task" {
		t.Errorf("role must be %q, got %v", "task", result["role"])
	}

	// AC: State is a human-readable string.
	if state, ok := result["state"].(string); !ok || state != "open" {
		t.Errorf("state must be %q, got %v", "open", result["state"])
	}

	// AC: created_at is UTC millisecond with Z suffix.
	createdAt, ok := result["created_at"].(string)
	if !ok || !utcMillisecondZ.MatchString(createdAt) {
		t.Errorf("created_at %q does not match UTC millisecond Z format", createdAt)
	}

	// AC: No is_deleted field.
	if _, exists := raw["is_deleted"]; exists {
		t.Errorf("is_deleted field must not be present in create JSON output")
	}
}

func TestE2E_CreateJSON_EpicConformsToJSONStandards(t *testing.T) {
	// Given — a fresh database.
	dir := initDB(t, "CE")

	// When — create an epic with JSON output.
	stdout, stderr, code := runNP(t, dir, "create",
		"--role", "epic",
		"--title", "Create JSON audit epic",
		"--author", "create-agent",
		"--json",
	)

	// Then
	if code != 0 {
		t.Fatalf("create epic --json failed (exit %d): %s", code, stderr)
	}

	result := parseJSON(t, stdout)

	if role, ok := result["role"].(string); !ok || role != "epic" {
		t.Errorf("role must be %q, got %v", "epic", result["role"])
	}

	createdAt, ok := result["created_at"].(string)
	if !ok || !utcMillisecondZ.MatchString(createdAt) {
		t.Errorf("created_at %q does not match UTC millisecond Z format", createdAt)
	}
}

func TestE2E_CreateJSON_WithClaimIncludesClaimID(t *testing.T) {
	// Given — a fresh database.
	dir := initDB(t, "CC")

	// When — create with --claim flag.
	stdout, stderr, code := runNP(t, dir, "create",
		"--role", "task",
		"--title", "Claimed at creation",
		"--author", "create-agent",
		"--claim",
		"--json",
	)

	// Then — claim_id is a non-empty string.
	if code != 0 {
		t.Fatalf("create --claim --json failed (exit %d): %s", code, stderr)
	}

	result := parseJSON(t, stdout)

	claimID, ok := result["claim_id"].(string)
	if !ok || claimID == "" {
		t.Errorf("claim_id must be a non-empty string when --claim used, got %v", result["claim_id"])
	}

	// Timestamps still conform.
	createdAt, ok := result["created_at"].(string)
	if !ok || !utcMillisecondZ.MatchString(createdAt) {
		t.Errorf("created_at %q does not match UTC millisecond Z format", createdAt)
	}
}

func TestE2E_CreateJSON_FromJSONConformsToJSONStandards(t *testing.T) {
	// Given — a fresh database.
	dir := initDB(t, "CF")

	// When — create from JSON input.
	stdout, stderr, code := runNP(t, dir, "create",
		"--from-json", `{"role":"task","title":"From JSON audit","priority":"P1"}`,
		"--author", "create-agent",
		"--json",
	)

	// Then
	if code != 0 {
		t.Fatalf("create --from-json --json failed (exit %d): %s", code, stderr)
	}

	result := parseJSON(t, stdout)

	if result["title"] != "From JSON audit" {
		t.Errorf("expected title %q, got %v", "From JSON audit", result["title"])
	}
	if result["priority"] != "P1" {
		t.Errorf("expected priority %q, got %v", "P1", result["priority"])
	}

	createdAt, ok := result["created_at"].(string)
	if !ok || !utcMillisecondZ.MatchString(createdAt) {
		t.Errorf("created_at %q does not match UTC millisecond Z format", createdAt)
	}
}
