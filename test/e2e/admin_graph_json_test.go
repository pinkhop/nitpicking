//go:build e2e

package e2e_test

import (
	"regexp"
	"testing"
)

func TestE2E_AdminGraphJSON_Shape(t *testing.T) {
	// Given — a database with issues.
	dir := initDB(t, "GR")
	createTask(t, dir, "Graph audit task", "graph-audit")

	// When — generate graph with --json.
	stdout, stderr, code := runNP(t, dir, "admin", "graph", "--json")

	// Then
	if code != 0 {
		t.Fatalf("admin graph --json failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)

	snakeCaseRE := regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)
	for key := range result {
		if !snakeCaseRE.MatchString(key) {
			t.Errorf("key %q is not snake_case", key)
		}
	}

	dot, ok := result["dot"].(string)
	if !ok || dot == "" {
		t.Errorf("dot should be a non-empty string, got %v (%T)", result["dot"], result["dot"])
	}

	if _, found := result["is_deleted"]; found {
		t.Error("is_deleted field must not be present")
	}

	// No PascalCase leaks.
	if _, found := result["DOT"]; found {
		t.Error("PascalCase key 'DOT' leaked")
	}
}
