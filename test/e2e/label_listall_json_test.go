//go:build e2e

package e2e_test

import (
	"regexp"
	"testing"
)

func TestE2E_LabelListAllJSON_Shape(t *testing.T) {
	// Given — tasks with different labels.
	dir := initDB(t, "LA")
	author := "listall-audit"
	runNP(t, dir, "create", "--role", "task", "--title", "Bug task",
		"--author", author, "--label", "kind:bug", "--json")
	runNP(t, dir, "create", "--role", "task", "--title", "Feature task",
		"--author", author, "--label", "kind:feature", "--json")

	// When — list-all labels with --json.
	stdout, stderr, code := runNP(t, dir, "label", "list-all", "--json")

	// Then
	if code != 0 {
		t.Fatalf("label list-all --json failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)

	snakeCaseRE := regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)
	for key := range result {
		if !snakeCaseRE.MatchString(key) {
			t.Errorf("top-level key %q is not snake_case", key)
		}
	}

	labels, ok := result["labels"].([]any)
	if !ok {
		t.Fatalf("expected labels array, got %T", result["labels"])
	}
	if len(labels) != 2 {
		t.Errorf("expected 2 labels, got %d", len(labels))
	}

	for i, raw := range labels {
		label, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("labels[%d]: expected object, got %T", i, raw)
		}
		for key := range label {
			if !snakeCaseRE.MatchString(key) {
				t.Errorf("labels[%d]: key %q is not snake_case", i, key)
			}
		}
		if _, ok := label["key"].(string); !ok {
			t.Errorf("labels[%d]: key should be a string", i)
		}
		if _, ok := label["value"].(string); !ok {
			t.Errorf("labels[%d]: value should be a string", i)
		}
	}

	count, ok := result["count"].(float64)
	if !ok || count != 2 {
		t.Errorf("expected count 2, got %v", result["count"])
	}

	if _, found := result["is_deleted"]; found {
		t.Error("is_deleted field must not be present")
	}
}
