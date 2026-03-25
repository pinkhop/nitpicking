//go:build e2e

package e2e_test

import (
	"regexp"
	"testing"
)

func TestE2E_RelCyclesJSON_NoCycles_Shape(t *testing.T) {
	// Given — a database with no cyclic relationships.
	dir := initDB(t, "RC")
	author := "cycles-audit"
	taskA := createTask(t, dir, "Task A", author)
	taskB := createTask(t, dir, "Task B", author)
	runNP(t, dir, "rel", "add", taskA, "blocked_by", taskB, "--author", author, "--json")

	// When — check for cycles.
	stdout, stderr, code := runNP(t, dir, "rel", "cycles", "--json")

	// Then — no cycles, correct shape.
	if code != 0 {
		t.Fatalf("rel cycles --json failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)

	snakeCaseRE := regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)
	for key := range result {
		if !snakeCaseRE.MatchString(key) {
			t.Errorf("key %q is not snake_case", key)
		}
	}

	findings, ok := result["findings"].([]any)
	if !ok {
		t.Fatalf("expected findings array, got %T", result["findings"])
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(findings))
	}

	count, ok := result["count"].(float64)
	if !ok || count != 0 {
		t.Errorf("expected count 0, got %v", result["count"])
	}

	if _, found := result["is_deleted"]; found {
		t.Error("is_deleted field must not be present")
	}
}

func TestE2E_RelCyclesJSON_WithCycle_Shape(t *testing.T) {
	// Given — a database with a cyclic blocked_by relationship.
	dir := initDB(t, "RC")
	author := "cycles-audit"
	taskA := createTask(t, dir, "Task A", author)
	taskB := createTask(t, dir, "Task B", author)
	runNP(t, dir, "rel", "add", taskA, "blocked_by", taskB, "--author", author, "--json")
	runNP(t, dir, "rel", "add", taskB, "blocked_by", taskA, "--author", author, "--json")

	// When — check for cycles.
	stdout, stderr, code := runNP(t, dir, "rel", "cycles", "--json")

	// Then — cycle found, correct shape.
	if code != 0 {
		t.Fatalf("rel cycles --json failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)

	snakeCaseRE := regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)
	for key := range result {
		if !snakeCaseRE.MatchString(key) {
			t.Errorf("top-level key %q is not snake_case", key)
		}
	}

	findings, ok := result["findings"].([]any)
	if !ok {
		t.Fatalf("expected findings array, got %T", result["findings"])
	}
	if len(findings) == 0 {
		t.Error("expected at least 1 cycle finding")
	}

	// Validate finding shape.
	for i, raw := range findings {
		finding, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("findings[%d]: expected object, got %T", i, raw)
		}
		for key := range finding {
			if !snakeCaseRE.MatchString(key) {
				t.Errorf("findings[%d]: key %q is not snake_case", i, key)
			}
		}
		if _, ok := finding["category"].(string); !ok {
			t.Errorf("findings[%d]: category should be a string", i)
		}
		if _, ok := finding["severity"].(string); !ok {
			t.Errorf("findings[%d]: severity should be a string", i)
		}
		if _, ok := finding["message"].(string); !ok {
			t.Errorf("findings[%d]: message should be a string", i)
		}

		// No PascalCase leaks.
		for _, banned := range []string{"Category", "Severity", "Message", "IssueIDs", "Suggestion"} {
			if _, found := finding[banned]; found {
				t.Errorf("findings[%d]: PascalCase key %q leaked", i, banned)
			}
		}
	}

	count, ok := result["count"].(float64)
	if !ok || count == 0 {
		t.Errorf("expected count > 0, got %v", result["count"])
	}
}
