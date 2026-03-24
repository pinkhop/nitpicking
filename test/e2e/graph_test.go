//go:build e2e

package e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2E_Graph_OutputContainsDOTStructure(t *testing.T) {
	// Given: a database with some issues.
	dir := initDB(t, "TEST")
	createTask(t, dir, "Task Alpha", "e2e-agent")
	createTask(t, dir, "Task Beta", "e2e-agent")

	// When
	stdout, stderr, code := runNP(t, dir, "graph")

	// Then
	if code != 0 {
		t.Fatalf("graph failed (exit %d): %s", code, stderr)
	}
	if !strings.Contains(stdout, "digraph issues") {
		t.Error("expected digraph header")
	}
	if !strings.Contains(stdout, "Task Alpha") {
		t.Error("expected Task Alpha in output")
	}
	if !strings.Contains(stdout, "Task Beta") {
		t.Error("expected Task Beta in output")
	}
}

func TestE2E_Graph_JSON_WrapsInJSONField(t *testing.T) {
	// Given
	dir := initDB(t, "TEST")
	createTask(t, dir, "JSON test", "e2e-agent")

	// When
	stdout, stderr, code := runNP(t, dir, "graph", "--json")

	// Then
	if code != 0 {
		t.Fatalf("graph --json failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)
	dot, ok := result["dot"].(string)
	if !ok || dot == "" {
		t.Fatal("expected non-empty 'dot' field in JSON output")
	}
	if !strings.Contains(dot, "digraph issues") {
		t.Error("expected digraph header in dot field")
	}
}

func TestE2E_Graph_OutputFile_WritesToFile(t *testing.T) {
	// Given
	dir := initDB(t, "TEST")
	createTask(t, dir, "File test", "e2e-agent")
	outFile := filepath.Join(dir, "issues.dot")

	// When
	_, stderr, code := runNP(t, dir, "graph", "--output", outFile)

	// Then
	if code != 0 {
		t.Fatalf("graph --output failed (exit %d): %s", code, stderr)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("reading output file: %v", err)
	}
	if !strings.Contains(string(data), "digraph issues") {
		t.Error("expected digraph header in file")
	}
	if !strings.Contains(string(data), "File test") {
		t.Error("expected issue title in file")
	}
}

func TestE2E_Graph_RelationshipsRendered(t *testing.T) {
	// Given: two tasks with a blocked_by relationship.
	dir := initDB(t, "TEST")
	idA := createTask(t, dir, "Blocker", "e2e-agent")
	idB := createTask(t, dir, "Blocked", "e2e-agent")

	_, stderr, code := runNP(t, dir, "relate", "add", idB, "blocked_by", idA,
		"--author", "e2e-agent")
	if code != 0 {
		t.Fatalf("relate add failed (exit %d): %s", code, stderr)
	}

	// When
	stdout, _, graphCode := runNP(t, dir, "graph")

	// Then
	if graphCode != 0 {
		t.Fatalf("graph failed")
	}
	if !strings.Contains(stdout, "blocked_by") {
		t.Error("expected blocked_by edge label in output")
	}
	if !strings.Contains(stdout, "dashed") {
		t.Error("expected dashed style for blocked_by edge")
	}
}
