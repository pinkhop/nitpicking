//go:build e2e

package e2e_test

import (
	"encoding/json"
	"regexp"
	"testing"
)

func TestE2E_ClaimReadyJSON_ConformsToJSONStandards(t *testing.T) {
	// Given — a database with a ready task.
	dir := initDB(t, "CR")
	createTask(t, dir, "Claim ready JSON audit", "claim-agent")

	// When — claim ready with JSON output.
	stdout, stderr, code := runNP(t, dir, "claim", "ready",
		"--author", "claim-agent",
		"--json",
	)

	// Then — the JSON body conforms to all standards.
	if code != 0 {
		t.Fatalf("claim ready --json failed (exit %d): %s", code, stderr)
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

	// issue_id is a non-empty string.
	if issueID, ok := result["issue_id"].(string); !ok || issueID == "" {
		t.Errorf("issue_id must be a non-empty string, got %v", result["issue_id"])
	}

	// claim_id is a non-empty string.
	if claimID, ok := result["claim_id"].(string); !ok || claimID == "" {
		t.Errorf("claim_id must be a non-empty string, got %v", result["claim_id"])
	}

	// stolen is a boolean (false for normal claim).
	stolen, ok := result["stolen"].(bool)
	if !ok {
		t.Errorf("stolen must be a boolean, got %v (%T)", result["stolen"], result["stolen"])
	}
	if stolen {
		t.Errorf("stolen must be false for normal claim, got true")
	}

	// No is_deleted field.
	if _, exists := raw["is_deleted"]; exists {
		t.Errorf("is_deleted field must not be present in claim ready JSON output")
	}
}
