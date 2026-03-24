//go:build e2e

package e2e_test

import (
	"encoding/json"
	"testing"
)

func TestE2E_FacetEnvVar_CreateMergesNPFACETS(t *testing.T) {
	// Given — NP_FACETS env var contains default facets.
	dir := initDB(t, "FENV")
	author := "facet-agent"

	// When — create a ticket with NP_FACETS set and no explicit --facet flags.
	stdout, stderr, code := runNPWithEnv(t, dir,
		[]string{"NP_FACETS=repo:auth kind:feature"},
		"create",
		"--role", "task",
		"--title", "Facet env test",
		"--author", author,
		"--json",
	)
	if code != 0 {
		t.Fatalf("create with NP_FACETS failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)
	ticketID := result["id"].(string)

	// Then — the ticket appears when filtering by the env-provided facet.
	listStdout, stderr, code := runNP(t, dir, "list",
		"--facet", "repo:auth",
		"--json",
	)
	if code != 0 {
		t.Fatalf("list --facet repo:auth failed (exit %d): %s", code, stderr)
	}

	listResult := parseJSON(t, listStdout)
	items := listResult["items"].([]any)
	found := false
	for _, item := range items {
		m := item.(map[string]any)
		if m["id"] == ticketID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected ticket %s to appear in list filtered by repo:auth", ticketID)
	}
}

func TestE2E_FacetEnvVar_ExplicitFlagOverridesEnv(t *testing.T) {
	// Given — NP_FACETS has kind:feature but --facet sets kind:fix.
	dir := initDB(t, "FOVR")
	author := "facet-agent"

	// When — create with explicit --facet that conflicts with env var.
	stdout, stderr, code := runNPWithEnv(t, dir,
		[]string{"NP_FACETS=kind:feature repo:auth"},
		"create",
		"--role", "task",
		"--title", "Facet override test",
		"--author", author,
		"--facet", "kind:fix",
		"--json",
	)
	if code != 0 {
		t.Fatalf("create with NP_FACETS + --facet failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)
	ticketID := result["id"].(string)

	// Then — the ticket appears with kind:fix (explicit) not kind:feature (env).
	listFixStdout, _, code := runNP(t, dir, "list",
		"--facet", "kind:fix",
		"--json",
	)
	if code != 0 {
		t.Fatalf("list --facet kind:fix failed")
	}
	fixResult := parseJSON(t, listFixStdout)
	fixItems := fixResult["items"].([]any)
	foundFix := false
	for _, item := range fixItems {
		m := item.(map[string]any)
		if m["id"] == ticketID {
			foundFix = true
			break
		}
	}
	if !foundFix {
		t.Errorf("expected ticket %s with kind:fix (explicit override)", ticketID)
	}

	// The ticket should NOT appear when filtering by kind:feature (env value
	// was overridden).
	listFeatureStdout, _, code := runNP(t, dir, "list",
		"--facet", "kind:feature",
		"--json",
	)
	if code == 0 {
		var featureResult map[string]any
		if err := json.Unmarshal([]byte(listFeatureStdout), &featureResult); err == nil {
			if featureItems, ok := featureResult["items"].([]any); ok {
				for _, item := range featureItems {
					m := item.(map[string]any)
					if m["id"] == ticketID {
						t.Errorf("ticket %s should not have kind:feature (overridden by --facet kind:fix)", ticketID)
					}
				}
			}
		}
	}
}
