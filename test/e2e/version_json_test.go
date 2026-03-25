//go:build e2e

package e2e_test

import (
	"encoding/json"
	"regexp"
	"testing"
)

func TestE2E_VersionJSON_ConformsToJSONStandards(t *testing.T) {
	// Given — any directory (version doesn't need a database).
	dir := t.TempDir()

	// When — version with JSON output.
	stdout, stderr, code := runNP(t, dir, "version", "--json")

	// Then — the JSON body conforms to all standards.
	if code != 0 {
		t.Fatalf("version --json failed (exit %d): %s", code, stderr)
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

	// name is a non-empty string.
	if name, ok := result["name"].(string); !ok || name == "" {
		t.Errorf("name must be a non-empty string, got %v", result["name"])
	}

	// version is a non-empty string.
	if ver, ok := result["version"].(string); !ok || ver == "" {
		t.Errorf("version must be a non-empty string, got %v", result["version"])
	}

	// built, if present and non-null, is UTC millisecond with Z suffix.
	if built, ok := result["built"].(string); ok && built != "" {
		if !utcMillisecondZ.MatchString(built) {
			t.Errorf("built %q does not match UTC millisecond Z format", built)
		}
	}

	// No is_deleted field.
	if _, exists := raw["is_deleted"]; exists {
		t.Errorf("is_deleted field must not be present in version JSON output")
	}
}
