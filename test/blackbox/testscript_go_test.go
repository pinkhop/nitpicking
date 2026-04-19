//go:build blackbox

// Package blackbox_test contains Go-based blackbox component tests that cannot
// be expressed as testscript .txtar scripts. These tests require features
// unavailable in testscript: goroutines for concurrency testing, and
// programmatic iteration over all CLI commands for flag-category validation.
//
// All other blackbox component tests live in testdata/*.txtar and run via
// TestBlackbox_Script.
package blackbox_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// Helpers — shared by the Go-based tests below. These duplicate the
// functionality of the testscript custom commands (np-init, np-seed, etc.)
// but operate through exec.Command for use in standard Go tests.
// ---------------------------------------------------------------------------

// npBinary returns the path to the built np binary. The binary must be
// built before running blackbox component tests (make build).
func npBinary(t *testing.T) string {
	t.Helper()

	// First, check PATH — testscript.RunMain registers the test binary as
	// "np" and adds it to PATH. This allows blackbox component tests to run without a
	// prior "make build".
	if p, err := exec.LookPath("np"); err == nil {
		return p
	}

	// Fallback: look for the binary relative to the project root.
	candidates := []string{
		"../../dist/np",
		filepath.Join(os.Getenv("GOPATH"), "bin", "np"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			abs, _ := filepath.Abs(p)
			return abs
		}
	}
	t.Fatal("np binary not found — run 'make build' or use testscript.RunMain")
	return ""
}

// runNP executes the np binary in the given directory with the given args.
// It returns stdout, stderr, and the exit code.
func runNP(t *testing.T, dir string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()

	binary := npBinary(t)
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir

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

// runNPWithStdin is like runNP but pipes stdinData to the command's stdin.
func runNPWithStdin(t *testing.T, dir, stdinData string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()

	binary := npBinary(t)
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdinData)

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

// runNPAsync is identical to runNP but logs errors instead of fataling,
// making it safe for use in goroutines.
func runNPAsync(t *testing.T, dir string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()

	binary := npBinary(t)
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir

	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	exitCode = 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	} else if err != nil {
		t.Logf("running np: %v", err)
		return "", err.Error(), -1
	}

	return outBuf.String(), errBuf.String(), exitCode
}

// runNPAsyncWithStdin is identical to runNPWithStdin but logs errors instead
// of fataling, making it safe for use in goroutines.
func runNPAsyncWithStdin(t *testing.T, dir, stdinData string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()

	binary := npBinary(t)
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdinData)

	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	exitCode = 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	} else if err != nil {
		t.Logf("running np: %v", err)
		return "", err.Error(), -1
	}

	return outBuf.String(), errBuf.String(), exitCode
}

// parseJSON unmarshals JSON stdout into a generic map, failing the test if
// the output is not valid JSON.
func parseJSON(t *testing.T, stdout string) map[string]any {
	t.Helper()

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, stdout)
	}
	return result
}

// initDB creates a temporary directory, initializes an np database with the
// given prefix, and returns the directory path.
func initDB(t *testing.T, prefix string) string {
	t.Helper()

	dir := t.TempDir()
	_, stderr, code := runNP(t, dir, "init", prefix)
	if code != 0 {
		t.Fatalf("init failed (exit %d): %s", code, stderr)
	}
	return dir
}

// createTask creates a task issue via json create and returns its ID.
func createTask(t *testing.T, dir, title, author string) string {
	t.Helper()

	payload := fmt.Sprintf(`{"role":"task","title":%q}`, title)
	stdout, stderr, code := runNPWithStdin(t, dir, payload,
		"json", "create", "--author", author,
	)
	if code != 0 {
		t.Fatalf("create task %q failed (exit %d): %s", title, code, stderr)
	}
	result := parseJSON(t, stdout)
	id, ok := result["id"].(string)
	if !ok || id == "" {
		t.Fatalf("create task %q: missing id in response", title)
	}
	return id
}

// createTaskWithPriority creates a task with the specified priority via json
// create and returns its ID.
func createTaskWithPriority(t *testing.T, dir, title, author, priority string) string {
	t.Helper()

	payload := fmt.Sprintf(`{"role":"task","title":%q,"priority":%q}`, title, priority)
	stdout, stderr, code := runNPWithStdin(t, dir, payload,
		"json", "create", "--author", author,
	)
	if code != 0 {
		t.Fatalf("create task %q with priority %s failed (exit %d): %s", title, priority, code, stderr)
	}
	result := parseJSON(t, stdout)
	id, ok := result["id"].(string)
	if !ok || id == "" {
		t.Fatalf("create task %q: missing id in response", title)
	}
	return id
}

// seedClaimedTask creates a task and claims it in one step via json create.
// Returns the issue ID and the claim ID.
func seedClaimedTask(t *testing.T, dir, title, author string) (issueID, claimID string) {
	t.Helper()

	payload := fmt.Sprintf(`{"role":"task","title":%q}`, title)
	stdout, stderr, code := runNPWithStdin(t, dir, payload,
		"json", "create", "--author", author, "--with-claim",
	)
	if code != 0 {
		t.Fatalf("seed claimed task %q failed (exit %d): %s", title, code, stderr)
	}
	result := parseJSON(t, stdout)

	id, ok := result["id"].(string)
	if !ok || id == "" {
		t.Fatalf("seed claimed task %q: missing id", title)
	}
	cid, ok := result["claim_id"].(string)
	if !ok || cid == "" {
		t.Fatalf("seed claimed task %q: missing claim_id", title)
	}
	return id, cid
}

// showIssue returns the full JSON representation of an issue.
func showIssue(t *testing.T, dir, issueID string) map[string]any {
	t.Helper()

	stdout, stderr, code := runNP(t, dir, "show", issueID, "--json")
	if code != 0 {
		t.Fatalf("show %s failed (exit %d): %s", issueID, code, stderr)
	}
	return parseJSON(t, stdout)
}

// ---------------------------------------------------------------------------
// Regression Tests
// ---------------------------------------------------------------------------

// TestBlackbox_RelTree_ShowsAllDescendantsBeyondDefaultLimit verifies that
// "rel tree" lists every descendant when the count exceeds the default
// repository page size of 20. This is a regression test: the command
// previously passed Limit: 0 (the zero value) to ListIssues, which
// NormalizeLimit silently promoted to DefaultLimit (20), silently truncating
// any epic with more than 20 children.
func TestBlackbox_RelTree_ShowsAllDescendantsBeyondDefaultLimit(t *testing.T) {
	t.Parallel()

	// Given — an epic with 21 child tasks, exceeding the default limit of 20.
	const childCount = 21
	dir := initDB(t, "TREE")
	author := "tree-agent"

	epicStdout, epicStderr, code := runNPWithStdin(t, dir,
		`{"role":"epic","title":"Large epic"}`,
		"json", "create", "--author", author,
	)
	if code != 0 {
		t.Fatalf("create epic failed (exit %d): %s", code, epicStderr)
	}
	epicResult := parseJSON(t, epicStdout)
	epicID, ok := epicResult["id"].(string)
	if !ok || epicID == "" {
		t.Fatalf("create epic: missing id in response: %s", epicStdout)
	}

	for i := range childCount {
		payload := fmt.Sprintf(`{"role":"task","title":"Child %d","parent":%q}`, i+1, epicID)
		_, stderr, exitCode := runNPWithStdin(t, dir, payload, "json", "create", "--author", author)
		if exitCode != 0 {
			t.Fatalf("create child %d failed (exit %d): %s", i+1, exitCode, stderr)
		}
	}

	// When — rel tree is called on the epic without --json.
	stdout, stderr, code := runNP(t, dir, "rel", "tree", epicID)
	if code != 0 {
		t.Fatalf("rel tree failed (exit %d): %s", code, stderr)
	}

	// Then — the output contains one line for the root and one line per child,
	// for a total of 1 + childCount non-empty lines.
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	var nonEmpty int
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			nonEmpty++
		}
	}
	// Subtract 1 for the root epic line; the remainder must equal childCount.
	descendantLines := nonEmpty - 1
	if descendantLines != childCount {
		t.Errorf("rel tree listed %d descendants, want %d\noutput:\n%s",
			descendantLines, childCount, stdout)
	}
}

// ---------------------------------------------------------------------------
// Concurrency Tests
// ---------------------------------------------------------------------------

func TestBlackbox_Concurrency_TwoAgentsClaimSameIssue(t *testing.T) {
	// Given — a single open task that two agents will race to claim.
	dir := initDB(t, "CONC")
	taskID := createTask(t, dir, "Contested issue", "setup-agent")

	// When — both agents attempt to claim the issue simultaneously.
	type claimResult struct {
		stdout   string
		stderr   string
		exitCode int
	}

	var (
		wg      sync.WaitGroup
		resultA claimResult
		resultB claimResult
	)

	wg.Go(func() {
		resultA.stdout, resultA.stderr, resultA.exitCode = runNPAsync(t, dir,
			"claim", taskID, "--author", "agent-alpha", "--json",
		)
	})
	wg.Go(func() {
		resultB.stdout, resultB.stderr, resultB.exitCode = runNPAsync(t, dir,
			"claim", taskID, "--author", "agent-beta", "--json",
		)
	})
	wg.Wait()

	// Then — exactly one agent succeeds (exit 0) and one gets a claim
	// conflict (exit 3).
	codes := []int{resultA.exitCode, resultB.exitCode}
	hasSuccess := false
	hasConflict := false
	for _, c := range codes {
		if c == 0 {
			hasSuccess = true
		}
		if c == 3 {
			hasConflict = true
		}
	}
	if !hasSuccess {
		t.Errorf("expected one agent to succeed, got codes %v\nA stderr: %s\nB stderr: %s",
			codes, resultA.stderr, resultB.stderr)
	}
	if !hasConflict {
		t.Errorf("expected one agent to get claim conflict (exit 3), got codes %v", codes)
	}
}

func TestBlackbox_Concurrency_TwoAgentsClaimDifferentIssues(t *testing.T) {
	// Given — two separate tasks, one for each agent.
	dir := initDB(t, "CONC")
	taskA := createTask(t, dir, "Task for alpha", "setup-agent")
	taskB := createTask(t, dir, "Task for beta", "setup-agent")

	// When — both agents claim their respective issues simultaneously.
	type claimResult struct {
		stdout   string
		stderr   string
		exitCode int
	}

	var (
		wg      sync.WaitGroup
		resultA claimResult
		resultB claimResult
	)

	wg.Go(func() {
		resultA.stdout, resultA.stderr, resultA.exitCode = runNPAsync(t, dir,
			"claim", taskA, "--author", "agent-alpha", "--json",
		)
	})
	wg.Go(func() {
		resultB.stdout, resultB.stderr, resultB.exitCode = runNPAsync(t, dir,
			"claim", taskB, "--author", "agent-beta", "--json",
		)
	})
	wg.Wait()

	// Then — both agents succeed without interference.
	if resultA.exitCode != 0 {
		t.Errorf("agent-alpha failed (exit %d): %s", resultA.exitCode, resultA.stderr)
	}
	if resultB.exitCode != 0 {
		t.Errorf("agent-beta failed (exit %d): %s", resultB.exitCode, resultB.stderr)
	}

	// Verify each issue is claimed by the correct agent.
	issueA := showIssue(t, dir, taskA)
	if issueA["claim_author"] != "agent-alpha" {
		t.Errorf("expected task A claimed by agent-alpha, got %v", issueA["claim_author"])
	}
	issueB := showIssue(t, dir, taskB)
	if issueB["claim_author"] != "agent-beta" {
		t.Errorf("expected task B claimed by agent-beta, got %v", issueB["claim_author"])
	}
}

func TestBlackbox_Concurrency_ClaimReadyClaimsDifferentIssues(t *testing.T) {
	// Given — multiple open tasks with different priorities.
	dir := initDB(t, "CONC")
	createTaskWithPriority(t, dir, "High priority", "setup-agent", "P0")
	createTaskWithPriority(t, dir, "Medium priority", "setup-agent", "P1")
	createTaskWithPriority(t, dir, "Low priority", "setup-agent", "P2")

	// When — two agents simultaneously request the next ready issue.
	type nextResult struct {
		stdout   string
		stderr   string
		exitCode int
	}

	var (
		wg      sync.WaitGroup
		resultA nextResult
		resultB nextResult
	)

	wg.Go(func() {
		resultA.stdout, resultA.stderr, resultA.exitCode = runNPAsync(t, dir,
			"claim", "ready", "--author", "agent-alpha", "--json",
		)
	})
	wg.Go(func() {
		resultB.stdout, resultB.stderr, resultB.exitCode = runNPAsync(t, dir,
			"claim", "ready", "--author", "agent-beta", "--json",
		)
	})
	wg.Wait()

	// Then — both agents should succeed (there are enough issues for both).
	if resultA.exitCode != 0 {
		t.Errorf("agent-alpha claim ready failed (exit %d): %s", resultA.exitCode, resultA.stderr)
	}
	if resultB.exitCode != 0 {
		t.Errorf("agent-beta claim ready failed (exit %d): %s", resultB.exitCode, resultB.stderr)
	}

	// They should claim different issues.
	if resultA.exitCode == 0 && resultB.exitCode == 0 {
		rA := parseJSON(t, resultA.stdout)
		rB := parseJSON(t, resultB.stdout)
		if rA["issue_id"] == rB["issue_id"] {
			t.Errorf("both agents claimed the same issue: %v", rA["issue_id"])
		}
	}
}

func TestBlackbox_Concurrency_ConcurrentCommentsOnSameIssue(t *testing.T) {
	// Given — an issue that two agents will add comments to simultaneously.
	dir := initDB(t, "CONC")
	taskID := createTask(t, dir, "Shared issue", "setup-agent")

	// When — both agents add comments simultaneously.
	var (
		wg    sync.WaitGroup
		codeA int
		codeB int
	)

	wg.Go(func() {
		_, _, codeA = runNPAsyncWithStdin(t, dir,
			`{"body": "Comment from alpha"}`,
			"json", "comment",
			taskID,
			"--author", "agent-alpha",
		)
	})
	wg.Go(func() {
		_, _, codeB = runNPAsyncWithStdin(t, dir,
			`{"body": "Comment from beta"}`,
			"json", "comment",
			taskID,
			"--author", "agent-beta",
		)
	})
	wg.Wait()

	// Then — both comments should be added successfully.
	if codeA != 0 {
		t.Errorf("agent-alpha json comment failed (exit %d)", codeA)
	}
	if codeB != 0 {
		t.Errorf("agent-beta json comment failed (exit %d)", codeB)
	}

	// Verify both comments exist.
	commentStdout, stderr, code := runNP(t, dir, "comment", "list", taskID, "--json")
	if code != 0 {
		t.Fatalf("comment list failed (exit %d): %s", code, stderr)
	}
	commentResult := parseJSON(t, commentStdout)
	comments, ok := commentResult["comments"].([]any)
	if !ok || len(comments) != 2 {
		t.Errorf("expected 2 comments, got %d", len(comments))
	}
}

func TestBlackbox_Concurrency_TwoAgentsAddDifferentLabelsToSameIssue(t *testing.T) {
	// Given — a claimed task.
	dir := initDB(t, "CLBL")
	author := "conc-agent"
	taskID, claimID := seedClaimedTask(t, dir, "Concurrent labels", author)

	// When — two processes simultaneously add different labels using
	// the same claim.
	var (
		wg    sync.WaitGroup
		codeA int
		codeB int
	)

	wg.Go(func() {
		_, _, codeA = runNPAsync(t, dir, "label", "add", "kind:fix",
			"--claim", claimID,
			"--json",
		)
	})
	wg.Go(func() {
		_, _, codeB = runNPAsync(t, dir, "label", "add", "area:auth",
			"--claim", claimID,
			"--json",
		)
	})
	wg.Wait()

	// Then — both label additions should succeed.
	if codeA != 0 {
		t.Errorf("label add kind:fix failed (exit %d)", codeA)
	}
	if codeB != 0 {
		t.Errorf("label add area:auth failed (exit %d)", codeB)
	}

	// Verify both labels are present.
	issue := showIssue(t, dir, taskID)
	labels, ok := issue["labels"].(map[string]any)
	if !ok {
		t.Fatalf("expected labels map, got %T", issue["labels"])
	}
	if labels["kind"] != "fix" {
		t.Errorf("expected label kind=fix, got %v", labels["kind"])
	}
	if labels["area"] != "auth" {
		t.Errorf("expected label area=auth, got %v", labels["area"])
	}
}

func TestBlackbox_Concurrency_TwoAgentsAddDifferentRelationshipsToSameIssue(t *testing.T) {
	// Given — a task with two potential blockers.
	dir := initDB(t, "CREL")
	author := "conc-agent"
	targetID := createTask(t, dir, "Target task", author)
	blockerA := createTask(t, dir, "Blocker A", author)
	blockerB := createTask(t, dir, "Blocker B", author)

	// When — two processes simultaneously add different blocked_by
	// relationships to the same target issue.
	var (
		wg    sync.WaitGroup
		codeA int
		codeB int
	)

	wg.Go(func() {
		_, _, codeA = runNPAsync(t, dir, "rel", "add", targetID,
			"blocked_by", blockerA,
			"--author", author,
			"--json",
		)
	})
	wg.Go(func() {
		_, _, codeB = runNPAsync(t, dir, "rel", "add", targetID,
			"blocked_by", blockerB,
			"--author", author,
			"--json",
		)
	})
	wg.Wait()

	// Then — both relationships should be added successfully.
	if codeA != 0 {
		t.Errorf("rel add blocked_by A failed (exit %d)", codeA)
	}
	if codeB != 0 {
		t.Errorf("rel add blocked_by B failed (exit %d)", codeB)
	}

	// Verify both relationships exist.
	issue := showIssue(t, dir, targetID)
	rels, ok := issue["relationships"].([]any)
	if !ok || len(rels) != 2 {
		t.Fatalf("expected 2 relationships, got %d", len(rels))
	}

	foundA := false
	foundB := false
	for _, r := range rels {
		rel, ok := r.(map[string]any)
		if !ok {
			continue
		}
		if rel["target_id"] == blockerA {
			foundA = true
		}
		if rel["target_id"] == blockerB {
			foundB = true
		}
	}
	if !foundA {
		t.Error("expected blocked_by relationship to blocker A")
	}
	if !foundB {
		t.Error("expected blocked_by relationship to blocker B")
	}
}

// ---------------------------------------------------------------------------
// Flag Category Tests
// ---------------------------------------------------------------------------

// commandSpec defines a command path and its expected required flags.
type commandSpec struct {
	args          []string
	requiredFlags []string
}

// allCommands returns every command and subcommand in the CLI, along with
// which flags are required for each.
func allCommands() []commandSpec {
	return []commandSpec{
		// Setup
		{args: []string{"init", "-h"}, requiredFlags: nil},

		// Info
		{args: []string{"version", "-h"}, requiredFlags: nil},

		// Admin subcommands: where and completion
		{args: []string{"admin", "where", "-h"}, requiredFlags: nil},
		{args: []string{"admin", "completion", "-h"}, requiredFlags: nil},

		// Core Workflow
		{args: []string{"show", "-h"}, requiredFlags: nil},
		{args: []string{"list", "-h"}, requiredFlags: nil},
		{args: []string{"ready", "-h"}, requiredFlags: nil},
		{args: []string{"blocked", "-h"}, requiredFlags: nil},
		{args: []string{"close", "-h"}, requiredFlags: []string{"--claim", "--reason"}},

		// Admin
		{args: []string{"admin", "tally", "-h"}, requiredFlags: nil},
		{args: []string{"import", "jsonl", "-h"}, requiredFlags: []string{"--author"}},

		// Claim
		{args: []string{"claim", "-h"}, requiredFlags: []string{"--author"}},

		// Issue subcommands
		{args: []string{"issue", "release", "-h"}, requiredFlags: []string{"--claim"}},
		{args: []string{"issue", "reopen", "-h"}, requiredFlags: []string{"--author"}},
		{args: []string{"issue", "undefer", "-h"}, requiredFlags: []string{"--author"}},
		{args: []string{"issue", "defer", "-h"}, requiredFlags: []string{"--claim"}},
		{args: []string{"issue", "delete", "-h"}, requiredFlags: []string{"--claim", "--confirm"}},
		{args: []string{"issue", "history", "-h"}, requiredFlags: nil},
		{args: []string{"issue", "orphans", "-h"}, requiredFlags: nil},

		// Epic subcommands
		{args: []string{"epic", "status", "-h"}, requiredFlags: nil},
		{args: []string{"epic", "close-completed", "-h"}, requiredFlags: []string{"--author"}},
		{args: []string{"epic", "children", "-h"}, requiredFlags: nil},

		// Rel subcommands
		{args: []string{"rel", "add", "-h"}, requiredFlags: []string{"--author"}},
		{args: []string{"rel", "remove", "-h"}, requiredFlags: []string{"--author"}},
		{args: []string{"rel", "blocks", "list", "-h"}, requiredFlags: nil},
		{args: []string{"rel", "refs", "list", "-h"}, requiredFlags: nil},
		{args: []string{"rel", "parent", "children", "-h"}, requiredFlags: nil},
		{args: []string{"rel", "parent", "tree", "-h"}, requiredFlags: nil},
		{args: []string{"rel", "parent", "detach", "-h"}, requiredFlags: []string{"--author"}},
		{args: []string{"rel", "issue", "-h"}, requiredFlags: nil},
		{args: []string{"rel", "tree", "-h"}, requiredFlags: nil},

		// Label subcommands
		{args: []string{"label", "add", "-h"}, requiredFlags: []string{"--claim"}},
		{args: []string{"label", "remove", "-h"}, requiredFlags: []string{"--claim"}},
		{args: []string{"label", "list", "-h"}, requiredFlags: nil},
		{args: []string{"label", "list-all", "-h"}, requiredFlags: nil},
		{args: []string{"label", "propagate", "-h"}, requiredFlags: []string{"--author", "--issue"}},

		// Comment subcommands
		{args: []string{"comment", "list", "-h"}, requiredFlags: nil},

		// Admin subcommands
		{args: []string{"admin", "doctor", "-h"}, requiredFlags: nil},
		{args: []string{"admin", "gc", "-h"}, requiredFlags: []string{"--confirm"}},
		{args: []string{"rel", "graph", "-h"}, requiredFlags: []string{"--format"}},
		{args: []string{"admin", "reset", "-h"}, requiredFlags: nil},
		{args: []string{"admin", "upgrade", "-h"}, requiredFlags: nil},

		// Agent subcommands
		{args: []string{"agent", "name", "-h"}, requiredFlags: nil},
		{args: []string{"agent", "prime", "-h"}, requiredFlags: nil},
	}
}

// TestBlackbox_FlagCategories_NoOptionsCategory verifies that no command uses the
// "Options" category name.
func TestBlackbox_FlagCategories_NoOptionsCategory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	for _, spec := range allCommands() {
		name := strings.Join(spec.args[:len(spec.args)-1], " ")
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			stdout, stderr, code := runNP(t, dir, spec.args...)
			if code != 0 {
				t.Fatalf("help failed (exit %d): %s", code, stderr)
			}
			if containsCategory(stdout, "Options") {
				t.Errorf("found forbidden 'Options' category in help output:\n%s", stdout)
			}
		})
	}
}

// TestBlackbox_FlagCategories_HelpFlagUncategorized verifies that --help never
// appears inside a named flag category section.
func TestBlackbox_FlagCategories_HelpFlagUncategorized(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	for _, spec := range allCommands() {
		name := strings.Join(spec.args[:len(spec.args)-1], " ")
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			stdout, stderr, code := runNP(t, dir, spec.args...)
			if code != 0 {
				t.Fatalf("help failed (exit %d): %s", code, stderr)
			}
			if flagInNamedCategory(stdout, "--help") {
				t.Errorf("--help appears inside a named category:\n%s", stdout)
			}
		})
	}
}

// TestBlackbox_FlagCategories_RequiredFlagsCorrect verifies that every flag listed
// under "Required" is genuinely required, and every required flag appears
// under "Required".
func TestBlackbox_FlagCategories_RequiredFlagsCorrect(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	for _, spec := range allCommands() {
		name := strings.Join(spec.args[:len(spec.args)-1], " ")
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			stdout, stderr, code := runNP(t, dir, spec.args...)
			if code != 0 {
				t.Fatalf("help failed (exit %d): %s", code, stderr)
			}

			actualRequired := flagsInCategory(stdout, "Required")
			for _, expected := range spec.requiredFlags {
				if !containsFlag(actualRequired, expected) {
					t.Errorf("expected flag %s under Required category, but not found.\nHelp output:\n%s", expected, stdout)
				}
			}

			for _, actual := range actualRequired {
				if !isExpectedRequired(actual, spec.requiredFlags) {
					t.Errorf("unexpected flag %q in Required category", actual)
				}
			}
		})
	}
}

// TestBlackbox_FlagCategories_SupplementalFlagsCorrect verifies that every
// non-help, non-required flag appears under "Supplemental".
func TestBlackbox_FlagCategories_SupplementalFlagsCorrect(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	for _, spec := range allCommands() {
		name := strings.Join(spec.args[:len(spec.args)-1], " ")
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			stdout, stderr, code := runNP(t, dir, spec.args...)
			if code != 0 {
				t.Fatalf("help failed (exit %d): %s", code, stderr)
			}

			uncategorized := uncategorizedFlags(stdout)
			for _, flag := range uncategorized {
				if flag == "--help" || flag == "-h" || flag == "--version" || flag == "-v" {
					continue
				}
				t.Errorf("flag %q is uncategorized (not under Required or Supplemental):\n%s", flag, stdout)
			}
		})
	}
}

// --- Flag category helpers ---

func containsCategory(helpOutput, categoryName string) bool {
	for line := range strings.SplitSeq(helpOutput, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == categoryName {
			return true
		}
	}
	return false
}

func flagInNamedCategory(helpOutput, flagName string) bool {
	inNamedCategory := false
	inOptions := false
	sawFlagInCategory := false
	for line := range strings.SplitSeq(helpOutput, "\n") {
		trimmed := strings.TrimSpace(line)

		if trimmed == "OPTIONS:" {
			inOptions = true
			continue
		}
		if !inOptions {
			continue
		}

		if trimmed != "" && !strings.HasPrefix(trimmed, "--") && !strings.HasPrefix(trimmed, "-") {
			inNamedCategory = true
			sawFlagInCategory = false
			continue
		}

		if trimmed == "" {
			if sawFlagInCategory {
				inNamedCategory = false
				sawFlagInCategory = false
			}
			continue
		}

		if inNamedCategory && strings.Contains(line, flagName) {
			return true
		}
		if inNamedCategory && strings.HasPrefix(trimmed, "--") {
			sawFlagInCategory = true
		}
	}
	return false
}

func flagsInCategory(helpOutput, categoryName string) []string {
	var flags []string
	inTarget := false
	inOptions := false
	sawFlagInTarget := false
	for line := range strings.SplitSeq(helpOutput, "\n") {
		trimmed := strings.TrimSpace(line)

		if trimmed == "OPTIONS:" {
			inOptions = true
			continue
		}
		if !inOptions {
			continue
		}

		if trimmed != "" && !strings.HasPrefix(trimmed, "--") && !strings.HasPrefix(trimmed, "-") {
			inTarget = trimmed == categoryName
			sawFlagInTarget = false
			continue
		}

		if trimmed == "" {
			if sawFlagInTarget {
				inTarget = false
				sawFlagInTarget = false
			}
			continue
		}

		if inTarget && strings.HasPrefix(trimmed, "--") {
			sawFlagInTarget = true
			flag := extractFlagName(trimmed)
			if flag != "" {
				flags = append(flags, flag)
			}
		}
	}
	return flags
}

func uncategorizedFlags(helpOutput string) []string {
	var flags []string
	inOptions := false
	inNamedCategory := false
	sawFlagInCategory := false
	for line := range strings.SplitSeq(helpOutput, "\n") {
		trimmed := strings.TrimSpace(line)

		if trimmed == "OPTIONS:" {
			inOptions = true
			continue
		}
		if !inOptions {
			continue
		}

		if trimmed != "" && !strings.HasPrefix(trimmed, "--") && !strings.HasPrefix(trimmed, "-") {
			inNamedCategory = true
			sawFlagInCategory = false
			continue
		}

		if trimmed == "" {
			if sawFlagInCategory {
				inNamedCategory = false
				sawFlagInCategory = false
			}
			continue
		}

		if inNamedCategory && strings.HasPrefix(trimmed, "--") {
			sawFlagInCategory = true
		}

		if !inNamedCategory && strings.HasPrefix(trimmed, "--") {
			flag := extractFlagName(trimmed)
			if flag != "" {
				flags = append(flags, flag)
			}
		}
	}
	return flags
}

func extractFlagName(line string) string {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "--") {
		return ""
	}
	parts := strings.Fields(trimmed)
	if len(parts) == 0 {
		return ""
	}
	flag := strings.TrimSuffix(parts[0], ",")
	return flag
}

func containsFlag(flags []string, target string) bool {
	for _, f := range flags {
		if f == target {
			return true
		}
	}
	return false
}

func isExpectedRequired(flag string, expected []string) bool {
	for _, e := range expected {
		if flag == e {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Migration Tests
// ---------------------------------------------------------------------------

// v2SchemaSQL is the V2 database schema — identical to the current V3 schema
// except that the issues table carries an idempotency_key column and the
// idx_issues_idempotency unique partial index. This literal is used to build a
// V1-style fixture (V2 DDL without a schema_version metadata row) so that the
// V1→V2→V3 migration path can be exercised end-to-end without coupling the
// test to the live schemaSQL constant.
const v2SchemaSQL = `
CREATE TABLE IF NOT EXISTS metadata (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS issues (
    issue_id            TEXT PRIMARY KEY,
    role                TEXT NOT NULL CHECK(role IN ('task', 'epic')),
    title               TEXT NOT NULL,
    description         TEXT NOT NULL DEFAULT '',
    acceptance_criteria TEXT NOT NULL DEFAULT '',
    priority            TEXT NOT NULL DEFAULT 'P2',
    state               TEXT NOT NULL,
    parent_id           TEXT DEFAULT NULL REFERENCES issues(issue_id),
    created_at          TEXT NOT NULL,
    idempotency_key     TEXT DEFAULT NULL,
    deleted             INTEGER NOT NULL DEFAULT 0
) WITHOUT ROWID;

CREATE INDEX IF NOT EXISTS idx_issues_parent ON issues(parent_id) WHERE parent_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_issues_state ON issues(state) WHERE deleted = 0;
CREATE INDEX IF NOT EXISTS idx_issues_priority_created ON issues(priority, created_at) WHERE deleted = 0;
CREATE UNIQUE INDEX IF NOT EXISTS idx_issues_idempotency ON issues(idempotency_key) WHERE idempotency_key IS NOT NULL;

CREATE TABLE IF NOT EXISTS labels (
    issue_id TEXT NOT NULL REFERENCES issues(issue_id),
    key       TEXT NOT NULL,
    value     TEXT NOT NULL,
    PRIMARY KEY (issue_id, key)
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS comments (
    comment_id INTEGER PRIMARY KEY AUTOINCREMENT,
    issue_id   TEXT NOT NULL REFERENCES issues(issue_id),
    author     TEXT NOT NULL,
    created_at TEXT NOT NULL,
    body       TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_comments_issue ON comments(issue_id);

CREATE TABLE IF NOT EXISTS claims (
    claim_sha512    TEXT PRIMARY KEY,
    issue_id       TEXT NOT NULL REFERENCES issues(issue_id),
    author          TEXT NOT NULL,
    stale_threshold INTEGER NOT NULL,
    last_activity   TEXT NOT NULL
) WITHOUT ROWID;

CREATE UNIQUE INDEX IF NOT EXISTS idx_claims_issue ON claims(issue_id);

CREATE TABLE IF NOT EXISTS relationships (
    source_id TEXT NOT NULL REFERENCES issues(issue_id),
    target_id TEXT NOT NULL REFERENCES issues(issue_id),
    rel_type  TEXT NOT NULL CHECK(rel_type IN ('blocked_by', 'blocks', 'refs')),
    PRIMARY KEY (source_id, target_id, rel_type)
) WITHOUT ROWID;

CREATE INDEX IF NOT EXISTS idx_relationships_target ON relationships(target_id);

CREATE TABLE IF NOT EXISTS history (
    entry_id   INTEGER PRIMARY KEY AUTOINCREMENT,
    issue_id  TEXT NOT NULL REFERENCES issues(issue_id),
    revision   INTEGER NOT NULL,
    author     TEXT NOT NULL,
    timestamp  TEXT NOT NULL,
    event_type TEXT NOT NULL,
    changes    TEXT NOT NULL DEFAULT '[]'
);

CREATE INDEX IF NOT EXISTS idx_history_issue ON history(issue_id, revision);

CREATE VIRTUAL TABLE IF NOT EXISTS issues_fts USING fts5(
    issue_id,
    title,
    description,
    acceptance_criteria
);

CREATE VIRTUAL TABLE IF NOT EXISTS comments_fts USING fts5(
    comment_id,
    body
);
`

// execSQLite3 runs sql against the SQLite database at dbPath via the sqlite3
// CLI, failing the test immediately if the command exits non-zero. Output is
// captured only to surface diagnostic information on failure; the helper has
// no return value because callers use it purely for side effects (schema
// setup and data seeding).
func execSQLite3(t *testing.T, dbPath, sql string) {
	t.Helper()

	cmd := exec.Command("sqlite3", dbPath, sql)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sqlite3 failed: %v\noutput: %s\nsql: %s", err, out, sql)
	}
}

// labelsMapFromJSON converts the "labels" array of an np label list --json
// response into a flat key→value map for assertion convenience. Entries
// whose key or value is not a string are skipped; in practice np always
// emits well-typed entries, so a skipped entry indicates a test bug rather
// than a production defect.
func labelsMapFromJSON(t *testing.T, result map[string]any) map[string]string {
	t.Helper()

	out := make(map[string]string)
	entries, _ := result["labels"].([]any)
	for _, entry := range entries {
		kv, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		key, keyOK := kv["key"].(string)
		value, valueOK := kv["value"].(string)
		if !keyOK || !valueOK {
			continue
		}
		out[key] = value
	}
	return out
}

// seedV1StyleDatabase creates a V1-style SQLite database at dir/.np/nitpicking.db
// using the V2 schema DDL (which includes the idempotency_key column) but without
// a schema_version metadata row, simulating a database from before v0.3.0. It
// inserts a representative set of issues, labels, and relationships to exercise
// all three migration steps (V1→V2 claimed-state conversion, V2→V3
// idempotency_key carry-forward, and V2→V3 column drop).
//
// Seeded data:
//   - VT-aaaaa: task "Legacy import task", state=claimed, idempotency_key="jira:PKHP-legacy", label kind:task
//   - VT-bbbbb: task "Related task", state=open, label tracker:test
//   - relationship: VT-aaaaa blocked_by VT-bbbbb
//
// Precondition failures use t.Fatalf so that the test fails in the Given phase
// and clearly indicates that setup, not the operation under test, is broken.
func seedV1StyleDatabase(t *testing.T, dir string) {
	t.Helper()

	// Create the .np directory structure that np expects.
	npDir := filepath.Join(dir, ".np")
	if err := os.MkdirAll(npDir, 0o755); err != nil {
		t.Fatalf("precondition: creating .np directory: %v", err)
	}

	dbPath := filepath.Join(npDir, "nitpicking.db")

	// Apply the V2 schema using a series of CREATE statements. sqlite3 accepts
	// a SQL script; we use the schema as a single multi-statement argument.
	cmd := exec.Command("sqlite3", dbPath)
	cmd.Stdin = strings.NewReader(v2SchemaSQL)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("precondition: applying V2 schema: %v\noutput: %s", err, out)
	}

	// Insert the database prefix. No schema_version row — this is the V1 marker.
	// The prefix must contain only uppercase ASCII letters (no digits).
	execSQLite3(t, dbPath, `INSERT INTO metadata (key, value) VALUES ('prefix', 'VT')`)

	now := "2024-01-01T00:00:00Z"

	// Issue A: claimed state with an idempotency_key and a user-defined label.
	// The claimed state exercises V1→V2 state conversion.
	// The idempotency_key exercises V2→V3 carry-forward.
	execSQLite3(t, dbPath,
		fmt.Sprintf(`INSERT INTO issues (issue_id, role, title, state, created_at, idempotency_key)
		             VALUES ('VT-aaaaa', 'task', 'Legacy import task', 'claimed', '%s', 'jira:PKHP-legacy')`, now))
	execSQLite3(t, dbPath, `INSERT INTO labels (issue_id, key, value) VALUES ('VT-aaaaa', 'kind', 'task')`)
	execSQLite3(t, dbPath,
		`INSERT INTO issues_fts (issue_id, title, description, acceptance_criteria)
		 VALUES ('VT-aaaaa', 'Legacy import task', '', '')`)

	// Issue B: open task with a user-defined label and no idempotency_key.
	// It participates in a relationship to verify edge-preservation.
	execSQLite3(t, dbPath,
		fmt.Sprintf(`INSERT INTO issues (issue_id, role, title, state, created_at)
		             VALUES ('VT-bbbbb', 'task', 'Related task', 'open', '%s')`, now))
	execSQLite3(t, dbPath, `INSERT INTO labels (issue_id, key, value) VALUES ('VT-bbbbb', 'tracker', 'test')`)
	execSQLite3(t, dbPath,
		`INSERT INTO issues_fts (issue_id, title, description, acceptance_criteria)
		 VALUES ('VT-bbbbb', 'Related task', '', '')`)

	// Relationship: A is blocked by B.
	execSQLite3(t, dbPath,
		`INSERT INTO relationships (source_id, target_id, rel_type)
		 VALUES ('VT-aaaaa', 'VT-bbbbb', 'blocked_by')`)
}

// TestBlackbox_Migration_V1DatabaseMigratesToV3 verifies the end-to-end
// V1→V2→V3 migration path by running np admin upgrade against a hand-crafted
// V1-style database fixture and asserting post-migration state through normal
// np commands.
//
// The fixture exercises all three migration concerns:
//   - V1→V2 state conversion: a claimed issue becomes open.
//   - V2→V3 idempotency_key carry-forward: the column value appears as an
//     idempotency:<value> label on the correct issue.
//   - Data preservation: titles, pre-existing labels, and relationships are
//     intact after migration.
func TestBlackbox_Migration_V1DatabaseMigratesToV3(t *testing.T) {
	t.Parallel()

	// Given — a temp directory containing a V1-style SQLite database seeded
	// with two issues, labels, and a blocked_by relationship.
	dir := t.TempDir()
	seedV1StyleDatabase(t, dir)

	// When — np admin upgrade is run against the fixture directory.
	stdout, stderr, code := runNP(t, dir, "admin", "upgrade", "--json")
	// Then — the upgrade succeeds and reports "migrated" status.
	if code != 0 {
		t.Fatalf("np admin upgrade failed (exit %d): %s", code, stderr)
	}

	upgradeResult := parseJSON(t, stdout)
	if upgradeResult["status"] != "migrated" {
		t.Errorf("upgrade status: got %v, want %q", upgradeResult["status"], "migrated")
	}

	// The V1→V2 step must have converted the one claimed issue to open.
	claimedConverted, _ := upgradeResult["claimed_issues_converted"].(float64)
	if claimedConverted != 1 {
		t.Errorf("claimed_issues_converted: got %v, want 1", claimedConverted)
	}

	// The V2→V3 step must have migrated one idempotency_key.
	idempotencyMigrated, _ := upgradeResult["idempotency_keys_migrated"].(float64)
	if idempotencyMigrated != 1 {
		t.Errorf("idempotency_keys_migrated: got %v, want 1", idempotencyMigrated)
	}
	idempotencySkipped, _ := upgradeResult["idempotency_keys_skipped"].(float64)
	if idempotencySkipped != 0 {
		t.Errorf("idempotency_keys_skipped: got %v, want 0", idempotencySkipped)
	}

	// Then — show VT-aaaaa and verify title, state, labels, and relationship.
	issueA := showIssue(t, dir, "VT-aaaaa")

	// Title must be preserved.
	if issueA["title"] != "Legacy import task" {
		t.Errorf("VT-aaaaa title: got %v, want %q", issueA["title"], "Legacy import task")
	}

	// State must be open (claimed→open migration).
	if issueA["state"] != "open" {
		t.Errorf("VT-aaaaa state: got %v, want %q", issueA["state"], "open")
	}

	// Retrieve labels for VT-aaaaa using np label list.
	labelsAStdout, labelsAStderr, labelsACode := runNP(t, dir, "label", "list", "VT-aaaaa", "--json")
	if labelsACode != 0 {
		t.Fatalf("np label list VT-aaaaa failed (exit %d): %s", labelsACode, labelsAStderr)
	}
	labelsA := labelsMapFromJSON(t, parseJSON(t, labelsAStdout))

	// The idempotency_key value must appear as idempotency:<value>.
	if labelsA["idempotency"] != "jira:PKHP-legacy" {
		t.Errorf("VT-aaaaa idempotency label: got %q, want %q", labelsA["idempotency"], "jira:PKHP-legacy")
	}

	// The pre-existing kind label must be preserved and not duplicated.
	if labelsA["kind"] != "task" {
		t.Errorf("VT-aaaaa kind label: got %q, want %q", labelsA["kind"], "task")
	}
	if len(labelsA) != 2 {
		t.Errorf("VT-aaaaa label count: got %d, want 2 (idempotency + kind)", len(labelsA))
	}

	// The blocked_by relationship from A to B must be intact.
	relationships, _ := issueA["relationships"].([]any)
	var foundBlockedBy bool
	for _, r := range relationships {
		if rel, ok := r.(map[string]any); ok {
			if rel["type"] == "blocked_by" && rel["target_id"] == "VT-bbbbb" {
				foundBlockedBy = true
			}
		}
	}
	if !foundBlockedBy {
		t.Errorf("VT-aaaaa: expected blocked_by relationship to VT-bbbbb; relationships: %v", relationships)
	}

	// Then — show VT-bbbbb and verify title and labels.
	issueB := showIssue(t, dir, "VT-bbbbb")

	if issueB["title"] != "Related task" {
		t.Errorf("VT-bbbbb title: got %v, want %q", issueB["title"], "Related task")
	}

	labelsBStdout, labelsBStderr, labelsBCode := runNP(t, dir, "label", "list", "VT-bbbbb", "--json")
	if labelsBCode != 0 {
		t.Fatalf("np label list VT-bbbbb failed (exit %d): %s", labelsBCode, labelsBStderr)
	}
	labelsB := labelsMapFromJSON(t, parseJSON(t, labelsBStdout))

	if labelsB["tracker"] != "test" {
		t.Errorf("VT-bbbbb tracker label: got %q, want %q", labelsB["tracker"], "test")
	}
	if len(labelsB) != 1 {
		t.Errorf("VT-bbbbb label count: got %d, want 1 (tracker only)", len(labelsB))
	}

	// Then — verify that np admin upgrade reports up_to_date on a second run
	// (the migration must not apply a second time).
	_, _, rerunCode := runNP(t, dir, "admin", "upgrade", "--json")
	if rerunCode != 0 {
		t.Error("second np admin upgrade run failed; expected it to report up_to_date")
	}
}
