//go:build e2e

package e2e_test

import (
	"regexp"
	"testing"
)

func TestE2E_LabelRemoveJSON_Shape(t *testing.T) {
	// Given — a task with a label.
	dir := initDB(t, "LR")
	author := "label-rm-audit"
	stdout, _, code := runNP(t, dir, "create",
		"--role", "task", "--title", "Labeled task",
		"--author", author, "--label", "kind:bug", "--claim", "--json")
	if code != 0 {
		t.Fatalf("precondition: create failed")
	}
	result := parseJSON(t, stdout)
	claimID := result["claim_id"].(string)

	// When — remove the label with --json.
	rmOut, stderr, code := runNP(t, dir, "label", "remove", "kind",
		"--claim", claimID, "--json")

	// Then
	if code != 0 {
		t.Fatalf("label remove --json failed (exit %d): %s", code, stderr)
	}
	rmResult := parseJSON(t, rmOut)

	snakeCaseRE := regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)
	for key := range rmResult {
		if !snakeCaseRE.MatchString(key) {
			t.Errorf("key %q is not snake_case", key)
		}
	}

	if _, ok := rmResult["issue_id"].(string); !ok {
		t.Errorf("issue_id should be a string, got %T", rmResult["issue_id"])
	}
	if rmResult["key"] != "kind" {
		t.Errorf("expected key 'kind', got %v", rmResult["key"])
	}
	if rmResult["action"] != "removed" {
		t.Errorf("expected action 'removed', got %v", rmResult["action"])
	}

	if _, found := rmResult["is_deleted"]; found {
		t.Error("is_deleted field must not be present")
	}
}
