//go:build e2e

package e2e_test

import (
	"strings"
	"testing"
)

func TestE2E_List_TimestampsFlag_ShowsCreatedAt(t *testing.T) {
	// Given — a database with an issue.
	dir := initDB(t, "TS")
	createTask(t, dir, "Timestamped task", "ts-agent")

	// When — list with --timestamps flag.
	stdout, stderr, code := runNP(t, dir, "list", "--timestamps")

	// Then — the text output includes a date-like timestamp.
	if code != 0 {
		t.Fatalf("list --timestamps failed (exit %d): %s", code, stderr)
	}
	// Expect at least a date substring like "2026-03-" in the output.
	if !strings.Contains(stdout, "2026-") {
		t.Errorf("expected timestamp in output, got:\n%s", stdout)
	}
}

func TestE2E_Search_TimestampsFlag_ShowsCreatedAt(t *testing.T) {
	// Given — a database with an issue.
	dir := initDB(t, "TS")
	createTask(t, dir, "Searchable timestamped task", "ts-agent")

	// When — search with --timestamps flag.
	stdout, stderr, code := runNP(t, dir, "search", "timestamped", "--timestamps")

	// Then — the text output includes a date-like timestamp.
	if code != 0 {
		t.Fatalf("search --timestamps failed (exit %d): %s", code, stderr)
	}
	if !strings.Contains(stdout, "2026-") {
		t.Errorf("expected timestamp in output, got:\n%s", stdout)
	}
}

func TestE2E_List_WithoutTimestamps_NoDateInOutput(t *testing.T) {
	// Given — a database with an issue.
	dir := initDB(t, "TS")
	createTask(t, dir, "No timestamp task", "ts-agent")

	// When — list without --timestamps flag.
	stdout, stderr, code := runNP(t, dir, "list")

	// Then — the text output should NOT include a timestamp.
	if code != 0 {
		t.Fatalf("list failed (exit %d): %s", code, stderr)
	}
	if strings.Contains(stdout, "2026-") {
		t.Errorf("expected no timestamp in default output, got:\n%s", stdout)
	}
}
