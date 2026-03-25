//go:build e2e

package e2e_test

import (
	"regexp"
	"testing"
)

// assertEpicStatusShape validates the top-level JSON shape of epic status output.
func assertEpicStatusShape(t *testing.T, result map[string]any) {
	t.Helper()

	snakeCaseRE := regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)

	// Top-level keys.
	for key := range result {
		if !snakeCaseRE.MatchString(key) {
			t.Errorf("top-level key %q is not snake_case", key)
		}
	}

	// Must have epics array and count.
	epics, ok := result["epics"].([]any)
	if !ok {
		t.Fatalf("expected epics array, got %T (%v)", result["epics"], result["epics"])
	}
	_, ok = result["count"].(float64)
	if !ok {
		t.Fatalf("expected count number, got %T (%v)", result["count"], result["count"])
	}

	// Validate each epic item.
	for i, raw := range epics {
		item, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("epics[%d]: expected object, got %T", i, raw)
		}

		// All keys snake_case.
		for key := range item {
			if !snakeCaseRE.MatchString(key) {
				t.Errorf("epics[%d]: key %q is not snake_case", i, key)
			}
		}

		// id is a non-empty string.
		id, ok := item["id"].(string)
		if !ok || id == "" {
			t.Errorf("epics[%d]: id should be a non-empty string, got %v", i, item["id"])
		}

		// Numeric fields.
		for _, key := range []string{"total_children", "closed_children", "percent"} {
			if _, ok := item[key].(float64); !ok {
				t.Errorf("epics[%d]: %s should be a number, got %v (%T)", i, key, item[key], item[key])
			}
		}

		// eligible_for_closure is a boolean.
		if _, ok := item["eligible_for_closure"].(bool); !ok {
			t.Errorf("epics[%d]: eligible_for_closure should be a boolean, got %v (%T)", i, item["eligible_for_closure"], item["eligible_for_closure"])
		}

		// No is_deleted.
		if _, found := item["is_deleted"]; found {
			t.Errorf("epics[%d]: is_deleted must not be present", i)
		}

		// No PascalCase leaks.
		for _, banned := range []string{"ID", "Title", "Total", "Closed", "Percent", "Eligible"} {
			if _, found := item[banned]; found {
				t.Errorf("epics[%d]: PascalCase key %q leaked", i, banned)
			}
		}
	}
}

func TestE2E_EpicStatusJSON_AllEpics_Shape(t *testing.T) {
	// Given — an epic with two children (one closed, one open).
	dir := initDB(t, "ES")
	author := "epic-status-audit"
	epicStdout, _, code := runNP(t, dir, "create",
		"--role", "epic", "--title", "Audit epic", "--author", author, "--json")
	if code != 0 {
		t.Fatalf("precondition: create epic failed")
	}
	epicID := parseJSON(t, epicStdout)["id"].(string)

	child1 := createTaskWithParent(t, dir, "Child A", author, epicID)
	createTaskWithParent(t, dir, "Child B", author, epicID)
	claimID := claimIssue(t, dir, child1, author)
	runNP(t, dir, "done", "--claim", claimID, "--author", author, "--reason", "done")

	// When — epic status (no args).
	stdout, stderr, code := runNP(t, dir, "epic", "status", "--json")

	// Then
	if code != 0 {
		t.Fatalf("epic status --json failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)
	assertEpicStatusShape(t, result)

	epics := result["epics"].([]any)
	if len(epics) != 1 {
		t.Errorf("expected 1 epic, got %d", len(epics))
	}
	epic := epics[0].(map[string]any)
	if epic["id"] != epicID {
		t.Errorf("expected epic id %q, got %v", epicID, epic["id"])
	}
	if epic["total_children"].(float64) != 2 {
		t.Errorf("expected 2 total_children, got %v", epic["total_children"])
	}
	if epic["closed_children"].(float64) != 1 {
		t.Errorf("expected 1 closed_children, got %v", epic["closed_children"])
	}
}

func TestE2E_EpicStatusJSON_SingleEpic_Shape(t *testing.T) {
	// Given — an epic with one child.
	dir := initDB(t, "ES")
	author := "epic-single-audit"
	epicStdout, _, code := runNP(t, dir, "create",
		"--role", "epic", "--title", "Single epic", "--author", author, "--json")
	if code != 0 {
		t.Fatalf("precondition: create epic failed")
	}
	epicID := parseJSON(t, epicStdout)["id"].(string)
	createTaskWithParent(t, dir, "Only child", author, epicID)

	// When — epic status with specific epic ID.
	stdout, stderr, code := runNP(t, dir, "epic", "status", epicID, "--json")

	// Then
	if code != 0 {
		t.Fatalf("epic status <ID> --json failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)
	assertEpicStatusShape(t, result)

	epics := result["epics"].([]any)
	if len(epics) != 1 {
		t.Errorf("expected 1 epic, got %d", len(epics))
	}
}

func TestE2E_EpicStatusJSON_EligibleOnly_Shape(t *testing.T) {
	// Given — two epics: one fully closed (eligible), one partially closed.
	dir := initDB(t, "ES")
	author := "eligible-audit"

	// Eligible epic: one child, closed.
	e1Stdout, _, _ := runNP(t, dir, "create",
		"--role", "epic", "--title", "Eligible epic", "--author", author, "--json")
	e1ID := parseJSON(t, e1Stdout)["id"].(string)
	c1 := createTaskWithParent(t, dir, "E1 child", author, e1ID)
	c1Claim := claimIssue(t, dir, c1, author)
	runNP(t, dir, "done", "--claim", c1Claim, "--author", author, "--reason", "done")

	// Non-eligible epic: one child, open.
	e2Stdout, _, _ := runNP(t, dir, "create",
		"--role", "epic", "--title", "Non-eligible epic", "--author", author, "--json")
	e2ID := parseJSON(t, e2Stdout)["id"].(string)
	createTaskWithParent(t, dir, "E2 child", author, e2ID)
	_ = e2ID

	// When — epic status --eligible-only.
	stdout, stderr, code := runNP(t, dir, "epic", "status", "--eligible-only", "--json")

	// Then — only the eligible epic appears.
	if code != 0 {
		t.Fatalf("epic status --eligible-only --json failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)
	assertEpicStatusShape(t, result)

	epics := result["epics"].([]any)
	if len(epics) != 1 {
		t.Errorf("expected 1 eligible epic, got %d", len(epics))
	}
	if len(epics) > 0 {
		epic := epics[0].(map[string]any)
		if epic["id"] != e1ID {
			t.Errorf("expected eligible epic %q, got %v", e1ID, epic["id"])
		}
		if epic["eligible_for_closure"] != true {
			t.Errorf("expected eligible_for_closure true")
		}
	}
}
