//go:build e2e

package e2e_test

import (
	"encoding/json"
	"testing"
)

func TestE2E_DimensionEnvVar_CreateMergesNPDIMENSIONS(t *testing.T) {
	// Given — NP_DIMENSIONS env var contains default dimensions.
	dir := initDB(t, "FENV")
	author := "dimension-agent"

	// When — create an issue with NP_DIMENSIONS set and no explicit --dimension flags.
	stdout, stderr, code := runNPWithEnv(t, dir,
		[]string{"NP_DIMENSIONS=repo:auth kind:feature"},
		"create",
		"--role", "task",
		"--title", "Dimension env test",
		"--author", author,
		"--json",
	)
	if code != 0 {
		t.Fatalf("create with NP_DIMENSIONS failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)
	issueID := result["id"].(string)

	// Then — the issue appears when filtering by the env-provided dimension.
	listStdout, stderr, code := runNP(t, dir, "list",
		"--dimension", "repo:auth",
		"--json",
	)
	if code != 0 {
		t.Fatalf("list --dimension repo:auth failed (exit %d): %s", code, stderr)
	}

	listResult := parseJSON(t, listStdout)
	items := listResult["items"].([]any)
	found := false
	for _, item := range items {
		m := item.(map[string]any)
		if m["id"] == issueID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected issue %s to appear in list filtered by repo:auth", issueID)
	}
}

func TestE2E_DimensionEnvVar_ExplicitFlagOverridesEnv(t *testing.T) {
	// Given — NP_DIMENSIONS has kind:feature but --dimension sets kind:fix.
	dir := initDB(t, "FOVR")
	author := "dimension-agent"

	// When — create with explicit --dimension that conflicts with env var.
	stdout, stderr, code := runNPWithEnv(t, dir,
		[]string{"NP_DIMENSIONS=kind:feature repo:auth"},
		"create",
		"--role", "task",
		"--title", "Dimension override test",
		"--author", author,
		"--dimension", "kind:fix",
		"--json",
	)
	if code != 0 {
		t.Fatalf("create with NP_DIMENSIONS + --dimension failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)
	issueID := result["id"].(string)

	// Then — the issue appears with kind:fix (explicit) not kind:feature (env).
	listFixStdout, _, code := runNP(t, dir, "list",
		"--dimension", "kind:fix",
		"--json",
	)
	if code != 0 {
		t.Fatalf("list --dimension kind:fix failed")
	}
	fixResult := parseJSON(t, listFixStdout)
	fixItems := fixResult["items"].([]any)
	foundFix := false
	for _, item := range fixItems {
		m := item.(map[string]any)
		if m["id"] == issueID {
			foundFix = true
			break
		}
	}
	if !foundFix {
		t.Errorf("expected issue %s with kind:fix (explicit override)", issueID)
	}

	// The issue should NOT appear when filtering by kind:feature (env value
	// was overridden).
	listFeatureStdout, _, code := runNP(t, dir, "list",
		"--dimension", "kind:feature",
		"--json",
	)
	if code == 0 {
		var featureResult map[string]any
		if err := json.Unmarshal([]byte(listFeatureStdout), &featureResult); err == nil {
			if featureItems, ok := featureResult["items"].([]any); ok {
				for _, item := range featureItems {
					m := item.(map[string]any)
					if m["id"] == issueID {
						t.Errorf("issue %s should not have kind:feature (overridden by --dimension kind:fix)", issueID)
					}
				}
			}
		}
	}
}
