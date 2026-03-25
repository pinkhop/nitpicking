//go:build e2e

package e2e_test

import (
	"encoding/json"
	"regexp"
	"testing"
)

// timestampRE matches UTC timestamps with exactly millisecond precision
// and a Z suffix, e.g. "2026-03-24T02:41:40.000Z".
var timestampRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$`)

// assertTimestamp checks that a value is a string matching the expected
// timestamp format (UTC, millisecond precision, Z suffix).
func assertTimestamp(t *testing.T, label string, val any) {
	t.Helper()
	s, ok := val.(string)
	if !ok {
		t.Errorf("%s: expected string timestamp, got %T (%v)", label, val, val)
		return
	}
	if !timestampRE.MatchString(s) {
		t.Errorf("%s: expected timestamp matching %s, got %q", label, timestampRE.String(), s)
	}
}

// assertListItemShape validates that a single list item has the correct
// JSON shape according to the audit AC.
func assertListItemShape(t *testing.T, label string, raw any) {
	t.Helper()
	item, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("%s: expected object, got %T", label, raw)
	}

	// AC1: All keys must be snake_case.
	snakeCaseRE := regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)
	for key := range item {
		if !snakeCaseRE.MatchString(key) {
			t.Errorf("%s: key %q is not snake_case", label, key)
		}
	}

	// AC2: ID is a string, never empty object.
	id, ok := item["id"].(string)
	if !ok || id == "" {
		t.Errorf("%s: id should be a non-empty string, got %v (%T)", label, item["id"], item["id"])
	}

	// AC3: Role and state are human-readable strings.
	validRoles := map[string]bool{"task": true, "epic": true}
	role, ok := item["role"].(string)
	if !ok || !validRoles[role] {
		t.Errorf("%s: role should be task or epic, got %v (%T)", label, item["role"], item["role"])
	}

	validStates := map[string]bool{"open": true, "claimed": true, "closed": true, "deferred": true}
	state, ok := item["state"].(string)
	if !ok || !validStates[state] {
		t.Errorf("%s: state should be a valid state string, got %v (%T)", label, item["state"], item["state"])
	}

	// display_status may also be "blocked".
	validDisplayStatuses := map[string]bool{"open": true, "claimed": true, "closed": true, "deferred": true, "blocked": true}
	ds, ok := item["display_status"].(string)
	if !ok || !validDisplayStatuses[ds] {
		t.Errorf("%s: display_status should be a valid status string, got %v (%T)", label, item["display_status"], item["display_status"])
	}

	// Priority should be a string like P0–P4.
	_, ok = item["priority"].(string)
	if !ok {
		t.Errorf("%s: priority should be a string, got %v (%T)", label, item["priority"], item["priority"])
	}

	// AC4: Timestamps are UTC with Z suffix, millisecond precision.
	assertTimestamp(t, label+".created_at", item["created_at"])

	// AC5: No is_deleted field.
	if _, found := item["is_deleted"]; found {
		t.Errorf("%s: is_deleted field must not be present", label)
	}

	// Also no leaked domain internals.
	for _, banned := range []string{"IsDeleted", "IsBlocked", "ParentID", "ID", "Role", "State", "Priority", "CreatedAt", "UpdatedAt"} {
		if _, found := item[banned]; found {
			t.Errorf("%s: PascalCase key %q leaked into JSON output", label, banned)
		}
	}
}

// assertListOutputShape validates the top-level list JSON shape and each
// item within it.
func assertListOutputShape(t *testing.T, stdout string) {
	t.Helper()
	var raw map[string]any
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, stdout)
	}

	// Top-level must have "items" (array) and "has_more" (bool).
	items, ok := raw["items"].([]any)
	if !ok {
		t.Fatalf("expected items array, got %T", raw["items"])
	}
	if _, ok := raw["has_more"].(bool); !ok {
		t.Fatalf("expected has_more bool, got %T (%v)", raw["has_more"], raw["has_more"])
	}

	for i, item := range items {
		assertListItemShape(t, "items["+itoa(i)+"]", item)
	}
}

// itoa is a minimal int-to-string for test labels.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	s := ""
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
}

func TestE2E_ListJSON_DefaultFlags_Shape(t *testing.T) {
	// Given — a database with a task and an epic.
	dir := initDB(t, "LJ")
	createTask(t, dir, "List audit task", "audit-agent")
	runNP(t, dir, "create", "--role", "epic", "--title", "List audit epic", "--author", "audit-agent", "--json")

	// When — list with default flags.
	stdout, _, code := runNP(t, dir, "list", "--json")

	// Then — exit 0 and correct shape.
	if code != 0 {
		t.Fatalf("list --json failed with exit code %d", code)
	}
	assertListOutputShape(t, stdout)

	result := parseJSON(t, stdout)
	items := result["items"].([]any)
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
}

func TestE2E_ListJSON_ReadyFlag_Shape(t *testing.T) {
	// Given — a task (ready) and a blocked task (not ready).
	dir := initDB(t, "LJ")
	taskA := createTask(t, dir, "Ready task", "audit-agent")
	taskB := createTask(t, dir, "Blocked task", "audit-agent")
	runNP(t, dir, "rel", "add", taskB, "blocked_by", taskA, "--author", "audit-agent", "--json")

	// When — list with --ready.
	stdout, _, code := runNP(t, dir, "list", "--ready", "--json")

	// Then — only the ready task appears.
	if code != 0 {
		t.Fatalf("list --ready --json failed with exit code %d", code)
	}
	assertListOutputShape(t, stdout)

	result := parseJSON(t, stdout)
	items := result["items"].([]any)
	if len(items) != 1 {
		t.Errorf("expected 1 ready item, got %d", len(items))
	}
	first := items[0].(map[string]any)
	if first["title"] != "Ready task" {
		t.Errorf("expected 'Ready task', got %v", first["title"])
	}
}

func TestE2E_ListJSON_IncludeClosedFlag_Shape(t *testing.T) {
	// Given — one open task and one closed task.
	dir := initDB(t, "LJ")
	createTask(t, dir, "Open task", "audit-agent")
	closedID := createTask(t, dir, "Closed task", "audit-agent")
	claimID := claimIssue(t, dir, closedID, "audit-agent")
	runNP(t, dir, "issue", "close", closedID, "--claim", claimID, "--author", "audit-agent", "--reason", "done", "--json")

	// When — list without --include-closed (default).
	stdoutDefault, _, code := runNP(t, dir, "list", "--json")
	if code != 0 {
		t.Fatalf("list --json failed with exit code %d", code)
	}

	// Then — only the open task appears.
	assertListOutputShape(t, stdoutDefault)
	resultDefault := parseJSON(t, stdoutDefault)
	itemsDefault := resultDefault["items"].([]any)
	if len(itemsDefault) != 1 {
		t.Errorf("expected 1 item (closed excluded), got %d", len(itemsDefault))
	}

	// When — list with --include-closed.
	stdoutInc, _, code := runNP(t, dir, "list", "--include-closed", "--json")
	if code != 0 {
		t.Fatalf("list --include-closed --json failed with exit code %d", code)
	}

	// Then — both tasks appear.
	assertListOutputShape(t, stdoutInc)
	resultInc := parseJSON(t, stdoutInc)
	itemsInc := resultInc["items"].([]any)
	if len(itemsInc) != 2 {
		t.Errorf("expected 2 items (include-closed), got %d", len(itemsInc))
	}
}

func TestE2E_ListJSON_StateFilter_Shape(t *testing.T) {
	// Given — one open task and one closed task.
	dir := initDB(t, "LJ")
	createTask(t, dir, "Open task", "audit-agent")
	closedID := createTask(t, dir, "Closed task", "audit-agent")
	claimID := claimIssue(t, dir, closedID, "audit-agent")
	runNP(t, dir, "issue", "close", closedID, "--claim", claimID, "--author", "audit-agent", "--reason", "done", "--json")

	// When — list with --state closed.
	stdout, _, code := runNP(t, dir, "list", "--state", "closed", "--json")

	// Then — only the closed task appears.
	if code != 0 {
		t.Fatalf("list --state closed --json failed with exit code %d", code)
	}
	assertListOutputShape(t, stdout)

	result := parseJSON(t, stdout)
	items := result["items"].([]any)
	if len(items) != 1 {
		t.Errorf("expected 1 closed item, got %d", len(items))
	}
	first := items[0].(map[string]any)
	if first["state"] != "closed" {
		t.Errorf("expected state 'closed', got %v", first["state"])
	}
}

func TestE2E_ListJSON_LabelFilter_Shape(t *testing.T) {
	// Given — two tasks with different labels.
	dir := initDB(t, "LJ")
	runNP(t, dir, "create", "--role", "task", "--title", "Bug task", "--author", "audit-agent", "--label", "kind:bug", "--json")
	runNP(t, dir, "create", "--role", "task", "--title", "Feature task", "--author", "audit-agent", "--label", "kind:feature", "--json")

	// When — list with --label kind:bug.
	stdout, _, code := runNP(t, dir, "list", "--label", "kind:bug", "--json")

	// Then — only the bug task appears.
	if code != 0 {
		t.Fatalf("list --label kind:bug --json failed with exit code %d", code)
	}
	assertListOutputShape(t, stdout)

	result := parseJSON(t, stdout)
	items := result["items"].([]any)
	if len(items) != 1 {
		t.Errorf("expected 1 item with label kind:bug, got %d", len(items))
	}
	first := items[0].(map[string]any)
	if first["title"] != "Bug task" {
		t.Errorf("expected 'Bug task', got %v", first["title"])
	}
}

func TestE2E_ReadyJSON_Shape(t *testing.T) {
	// Given — a database with two tasks, one blocked.
	dir := initDB(t, "LJ")
	taskA := createTask(t, dir, "Available task", "audit-agent")
	taskB := createTask(t, dir, "Blocked task", "audit-agent")
	runNP(t, dir, "rel", "add", taskB, "blocked_by", taskA, "--author", "audit-agent", "--json")

	// When — run the ready shortcut command.
	stdout, _, code := runNP(t, dir, "ready", "--json")

	// Then — correct JSON shape, only the ready task.
	if code != 0 {
		t.Fatalf("ready --json failed with exit code %d", code)
	}
	assertListOutputShape(t, stdout)

	result := parseJSON(t, stdout)
	items := result["items"].([]any)
	if len(items) != 1 {
		t.Errorf("expected 1 ready item, got %d", len(items))
	}
	first := items[0].(map[string]any)
	if first["title"] != "Available task" {
		t.Errorf("expected 'Available task', got %v", first["title"])
	}
}

func TestE2E_BlockedJSON_Shape(t *testing.T) {
	// Given — a database with a blocker and a blocked task.
	dir := initDB(t, "LJ")
	taskA := createTask(t, dir, "Blocker", "audit-agent")
	taskB := createTask(t, dir, "Blocked task", "audit-agent")
	runNP(t, dir, "rel", "add", taskB, "blocked_by", taskA, "--author", "audit-agent", "--json")

	// When — run the blocked shortcut command.
	stdout, _, code := runNP(t, dir, "blocked", "--json")

	// Then — correct JSON shape, only the blocked task.
	if code != 0 {
		t.Fatalf("blocked --json failed with exit code %d", code)
	}
	assertListOutputShape(t, stdout)

	result := parseJSON(t, stdout)
	items := result["items"].([]any)
	if len(items) != 1 {
		t.Errorf("expected 1 blocked item, got %d", len(items))
	}
	first := items[0].(map[string]any)
	if first["title"] != "Blocked task" {
		t.Errorf("expected 'Blocked task', got %v", first["title"])
	}
	if first["display_status"] != "blocked" {
		t.Errorf("expected display_status 'blocked', got %v", first["display_status"])
	}
}
