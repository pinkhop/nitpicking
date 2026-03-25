//go:build e2e

package e2e_test

import (
	"regexp"
	"testing"
)

// assertDoctorOutputShape validates the top-level doctor JSON output shape.
func assertDoctorOutputShape(t *testing.T, result map[string]any) {
	t.Helper()

	snakeCaseRE := regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)
	for key := range result {
		if !snakeCaseRE.MatchString(key) {
			t.Errorf("top-level key %q is not snake_case", key)
		}
	}

	// healthy is a boolean.
	if _, ok := result["healthy"].(bool); !ok {
		t.Errorf("healthy should be a boolean, got %T", result["healthy"])
	}

	// findings is an array.
	findings, ok := result["findings"].([]any)
	if !ok {
		t.Fatalf("expected findings array, got %T", result["findings"])
	}

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

	if _, found := result["is_deleted"]; found {
		t.Error("is_deleted field must not be present")
	}
}

func TestE2E_DoctorJSON_Default_Shape(t *testing.T) {
	// Given — a healthy database.
	dir := initDB(t, "DR")
	createTask(t, dir, "Healthy task", "doctor-audit")

	// When — run doctor with default flags.
	stdout, stderr, code := runNP(t, dir, "admin", "doctor", "--json")

	// Then
	if code != 0 {
		t.Fatalf("admin doctor --json failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)
	assertDoctorOutputShape(t, result)
}

func TestE2E_DoctorJSON_Verbose_Shape(t *testing.T) {
	// Given — a healthy database.
	dir := initDB(t, "DR")
	createTask(t, dir, "Healthy task", "doctor-audit")

	// When — run doctor with --verbose.
	stdout, stderr, code := runNP(t, dir, "admin", "doctor", "--verbose", "--json")

	// Then — checks array should be present.
	if code != 0 {
		t.Fatalf("admin doctor --verbose --json failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)
	assertDoctorOutputShape(t, result)

	snakeCaseRE := regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)

	checks, ok := result["checks"].([]any)
	if !ok || len(checks) == 0 {
		t.Fatalf("expected non-empty checks array in verbose mode, got %v", result["checks"])
	}

	for i, raw := range checks {
		check, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("checks[%d]: expected object, got %T", i, raw)
		}
		for key := range check {
			if !snakeCaseRE.MatchString(key) {
				t.Errorf("checks[%d]: key %q is not snake_case", i, key)
			}
		}
		if _, ok := check["name"].(string); !ok {
			t.Errorf("checks[%d]: name should be a string", i)
		}
		if _, ok := check["status"].(string); !ok {
			t.Errorf("checks[%d]: status should be a string", i)
		}
	}
}

func TestE2E_DoctorJSON_SeverityWarning_Shape(t *testing.T) {
	// Given — a healthy database.
	dir := initDB(t, "DR")
	createTask(t, dir, "Healthy task", "doctor-audit")

	// When — run doctor with --severity warning.
	stdout, stderr, code := runNP(t, dir, "admin", "doctor",
		"--severity", "warning", "--json")

	// Then
	if code != 0 {
		t.Fatalf("admin doctor --severity warning --json failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)
	assertDoctorOutputShape(t, result)
}

func TestE2E_DoctorJSON_SeverityError_Shape(t *testing.T) {
	// Given — a healthy database.
	dir := initDB(t, "DR")
	createTask(t, dir, "Healthy task", "doctor-audit")

	// When — run doctor with --severity error.
	stdout, stderr, code := runNP(t, dir, "admin", "doctor",
		"--severity", "error", "--json")

	// Then
	if code != 0 {
		t.Fatalf("admin doctor --severity error --json failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)
	assertDoctorOutputShape(t, result)
}
