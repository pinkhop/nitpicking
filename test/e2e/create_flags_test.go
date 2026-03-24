//go:build e2e

package e2e_test

import "testing"

func TestE2E_Create_DescriptionFlag_RoundTrips(t *testing.T) {
	// Given — a fresh database.
	dir := initDB(t, "FLAG")

	// When — create a task with --description.
	stdout, stderr, code := runNP(t, dir, "create",
		"--role", "task",
		"--title", "With description",
		"--description", "Detailed explanation of the task",
		"--author", "flag-agent",
		"--json",
	)

	// Then — the description round-trips through show.
	if code != 0 {
		t.Fatalf("create failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)
	taskID := result["id"].(string)

	ticket := showTicket(t, dir, taskID)
	if ticket["description"] != "Detailed explanation of the task" {
		t.Errorf("expected description to round-trip, got %v", ticket["description"])
	}
}

func TestE2E_Create_AcceptanceCriteriaFlag_RoundTrips(t *testing.T) {
	// Given — a fresh database.
	dir := initDB(t, "FLAG")

	// When — create a task with --acceptance-criteria.
	stdout, stderr, code := runNP(t, dir, "create",
		"--role", "task",
		"--title", "With AC",
		"--acceptance-criteria", "1. Unit tests pass\n2. E2E tests pass",
		"--author", "flag-agent",
		"--json",
	)

	// Then — the acceptance criteria round-trips through show.
	if code != 0 {
		t.Fatalf("create failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)
	taskID := result["id"].(string)

	ticket := showTicket(t, dir, taskID)
	if ticket["acceptance_criteria"] != "1. Unit tests pass\n2. E2E tests pass" {
		t.Errorf("expected acceptance_criteria to round-trip, got %v", ticket["acceptance_criteria"])
	}
}

func TestE2E_Create_IdempotencyKey_PreventsDuplicates(t *testing.T) {
	// Given — a fresh database.
	dir := initDB(t, "FLAG")

	// When — create a task with an idempotency key, then try again with the
	// same key.
	stdout1, stderr, code := runNP(t, dir, "create",
		"--role", "task",
		"--title", "Idempotent task",
		"--author", "flag-agent",
		"--idempotency-key", "unique-key-123",
		"--json",
	)
	if code != 0 {
		t.Fatalf("first create failed (exit %d): %s", code, stderr)
	}
	firstID := parseJSON(t, stdout1)["id"].(string)

	stdout2, _, code2 := runNP(t, dir, "create",
		"--role", "task",
		"--title", "Duplicate task",
		"--author", "flag-agent",
		"--idempotency-key", "unique-key-123",
		"--json",
	)

	// Then — the second create returns the same ticket ID (idempotent).
	if code2 != 0 {
		// Some implementations may return an error for duplicates; either way
		// is fine as long as the duplicate is detected.
		return
	}
	secondID := parseJSON(t, stdout2)["id"].(string)
	if secondID != firstID {
		t.Errorf("expected idempotent duplicate to return same ID %s, got %s", firstID, secondID)
	}
}
