//go:build e2e

package e2e_test

import "testing"

func TestE2E_Priority_CreateAcceptsCaseInsensitiveAndBareNumber(t *testing.T) {
	dir := initDB(t, "PRIO")
	author := "prio-agent"

	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{"canonical uppercase", "P3", "P3"},
		{"lowercase", "p1", "P1"},
		{"bare number", "4", "P4"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// When — create a task with the given priority format.
			stdout, stderr, code := runNP(t, dir, "create",
				"--role", "task",
				"--title", "Priority test: "+tc.input,
				"--author", author,
				"--priority", tc.input,
				"--json",
			)
			if code != 0 {
				t.Fatalf("create with --priority %s failed (exit %d): %s", tc.input, code, stderr)
			}

			// Then — the issue's priority is normalized to canonical form.
			result := parseJSON(t, stdout)
			issueID, ok := result["id"].(string)
			if !ok || issueID == "" {
				t.Fatalf("missing id in create response")
			}

			issue := showIssue(t, dir, issueID)
			if issue["priority"] != tc.expected {
				t.Errorf("expected priority %s, got %v", tc.expected, issue["priority"])
			}
		})
	}
}

func TestE2E_Priority_UpdateAcceptsCaseInsensitiveAndBareNumber(t *testing.T) {
	dir := initDB(t, "PRIO")
	author := "prio-agent"

	// Given — a claimed task with default priority.
	taskID, claimID := seedClaimedTask(t, dir, "Priority update test", author)

	// When — update priority using lowercase.
	_, stderr, code := runNP(t, dir, "issue", "update", taskID,
		"--claim", claimID,
		"--priority", "p0",
		"--json",
	)
	if code != 0 {
		t.Fatalf("update with --priority p0 failed (exit %d): %s", code, stderr)
	}

	// Then — priority is normalized to P0.
	issue := showIssue(t, dir, taskID)
	if issue["priority"] != "P0" {
		t.Errorf("expected priority P0, got %v", issue["priority"])
	}
}

func TestE2E_Priority_EditAcceptsBareNumber(t *testing.T) {
	dir := initDB(t, "PRIO")
	author := "prio-agent"

	// Given — a task with default priority.
	taskID := createTask(t, dir, "Priority edit test", author)

	// When — edit priority using bare number.
	_, stderr, code := runNP(t, dir, "issue", "edit", taskID,
		"--author", author,
		"--priority", "1",
		"--json",
	)
	if code != 0 {
		t.Fatalf("edit with --priority 1 failed (exit %d): %s", code, stderr)
	}

	// Then — priority is normalized to P1.
	issue := showIssue(t, dir, taskID)
	if issue["priority"] != "P1" {
		t.Errorf("expected priority P1, got %v", issue["priority"])
	}
}
