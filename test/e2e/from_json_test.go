//go:build e2e

package e2e_test

import (
	"os/exec"
	"strings"
	"testing"
)

// runNPWithStdin executes the np binary with the given stdin content piped in.
func runNPWithStdin(t *testing.T, dir, stdin string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()

	binary := npBinary(t)
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdin)

	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	exitCode = 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("running np: %v", err)
	}

	return outBuf.String(), errBuf.String(), exitCode
}

func TestE2E_CreateFromJSON_InlineJSON(t *testing.T) {
	// Given
	dir := initDB(t, "TEST")

	jsonInput := `{"role":"task","title":"Inline JSON task","description":"Created via --from-json","priority":"P1"}`

	// When
	stdout, stderr, code := runNP(t, dir, "create",
		"--from-json", jsonInput,
		"--author", "e2e-agent",
		"--json",
	)

	// Then
	if code != 0 {
		t.Fatalf("create failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)
	if result["title"] != "Inline JSON task" {
		t.Errorf("title: got %v, want %q", result["title"], "Inline JSON task")
	}
	if result["priority"] != "P1" {
		t.Errorf("priority: got %v, want %q", result["priority"], "P1")
	}
}

func TestE2E_CreateFromJSON_Stdin(t *testing.T) {
	// Given
	dir := initDB(t, "TEST")

	jsonInput := `{"role":"task","title":"Stdin JSON task","priority":"P0"}`

	// When
	stdout, stderr, code := runNPWithStdin(t, dir, jsonInput, "create",
		"--from-json", "-",
		"--author", "e2e-agent",
		"--json",
	)

	// Then
	if code != 0 {
		t.Fatalf("create failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)
	if result["title"] != "Stdin JSON task" {
		t.Errorf("title: got %v, want %q", result["title"], "Stdin JSON task")
	}
	if result["priority"] != "P0" {
		t.Errorf("priority: got %v, want %q", result["priority"], "P0")
	}
}

func TestE2E_CreateFromJSON_FlagOverridesJSON(t *testing.T) {
	// Given
	dir := initDB(t, "TEST")

	jsonInput := `{"role":"task","title":"JSON title","priority":"P3"}`

	// When: --title flag overrides the JSON title.
	stdout, stderr, code := runNP(t, dir, "create",
		"--from-json", jsonInput,
		"--title", "Flag title wins",
		"--author", "e2e-agent",
		"--json",
	)

	// Then
	if code != 0 {
		t.Fatalf("create failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)
	if result["title"] != "Flag title wins" {
		t.Errorf("title: got %v, want %q", result["title"], "Flag title wins")
	}
	// Priority comes from JSON since no --priority flag.
	if result["priority"] != "P3" {
		t.Errorf("priority: got %v, want %q", result["priority"], "P3")
	}
}

func TestE2E_CreateFromJSON_ShowOutputCompatibility(t *testing.T) {
	// Given: create an issue, then pipe its show output into a new create.
	dir := initDB(t, "TEST")

	originalID := createTask(t, dir, "Original issue", "e2e-agent")
	showStdout, _, showCode := runNP(t, dir, "show", originalID, "--json")
	if showCode != 0 {
		t.Fatalf("show failed (exit %d)", showCode)
	}

	// When: pipe show output into create.
	stdout, stderr, code := runNPWithStdin(t, dir, showStdout, "create",
		"--from-json", "-",
		"--author", "e2e-agent",
		"--json",
	)

	// Then: new issue is created with the same title and role.
	if code != 0 {
		t.Fatalf("create from show output failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)
	if result["title"] != "Original issue" {
		t.Errorf("title: got %v, want %q", result["title"], "Original issue")
	}
	if result["role"] != "task" {
		t.Errorf("role: got %v, want %q", result["role"], "task")
	}
	// New issue should have a different ID.
	if result["id"] == originalID {
		t.Error("new issue should have a different ID from the original")
	}
}

func TestE2E_CreateFromJSON_InvalidJSON_Fails(t *testing.T) {
	// Given
	dir := initDB(t, "TEST")

	// When
	_, _, code := runNP(t, dir, "create",
		"--from-json", "{not valid json}",
		"--author", "e2e-agent",
		"--json",
	)

	// Then: exit code 4 (validation error).
	if code != 4 {
		t.Errorf("expected exit code 4, got %d", code)
	}
}

func TestE2E_CreateFromJSON_MissingRequiredFields_Fails(t *testing.T) {
	// Given: JSON with only description, missing role and title.
	dir := initDB(t, "TEST")

	// When
	_, _, code := runNP(t, dir, "create",
		"--from-json", `{"description":"just a description"}`,
		"--author", "e2e-agent",
		"--json",
	)

	// Then: exit code 4 (validation error — role is missing).
	if code != 4 {
		t.Errorf("expected exit code 4, got %d", code)
	}
}

func TestE2E_CreateFromJSON_DimensionsMerge(t *testing.T) {
	// Given
	dir := initDB(t, "TEST")

	jsonInput := `{"role":"task","title":"Label merge test","labels":{"kind":"bug","area":"auth"}}`

	// When: --label flag overrides kind but area comes from JSON.
	stdout, stderr, code := runNP(t, dir, "create",
		"--from-json", jsonInput,
		"--label", "kind:feature",
		"--author", "e2e-agent",
		"--claim",
		"--json",
	)

	// Then
	if code != 0 {
		t.Fatalf("create failed (exit %d): %s", code, stderr)
	}
	created := parseJSON(t, stdout)
	createdID, _ := created["id"].(string)

	// Verify dimensions via show.
	showStdout, _, _ := runNP(t, dir, "show", createdID, "--json")
	// show --json doesn't include dimensions, so we check via list with --dimension filter.
	// Verify kind:feature (flag wins over JSON's kind:bug).
	listStdout, _, listCode := runNP(t, dir, "list",
		"--dimension", "kind:feature",
		"--include-closed",
		"--json",
	)
	if listCode != 0 {
		t.Fatalf("list failed")
	}
	listResult := parseJSON(t, listStdout)
	items, _ := listResult["items"].([]any)
	if len(items) != 1 {
		t.Errorf("expected 1 issue with kind:feature, got %d", len(items))
	}

	// Verify area:auth (from JSON, no conflict).
	listStdout2, _, _ := runNP(t, dir, "list",
		"--dimension", "area:auth",
		"--include-closed",
		"--json",
	)
	listResult2 := parseJSON(t, listStdout2)
	items2, _ := listResult2["items"].([]any)
	if len(items2) != 1 {
		t.Errorf("expected 1 issue with area:auth, got %d", len(items2))
	}

	// Suppress unused variable warning for showStdout.
	_ = showStdout
}
