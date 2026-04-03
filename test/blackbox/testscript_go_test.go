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
		{args: []string{"rel", "blocks", "list", "-h"}, requiredFlags: nil},
		{args: []string{"rel", "blocks", "unblock", "-h"}, requiredFlags: []string{"--author"}},
		{args: []string{"rel", "refs", "list", "-h"}, requiredFlags: nil},
		{args: []string{"rel", "refs", "unref", "-h"}, requiredFlags: []string{"--author"}},
		{args: []string{"rel", "parent", "children", "-h"}, requiredFlags: nil},
		{args: []string{"rel", "parent", "tree", "-h"}, requiredFlags: nil},
		{args: []string{"rel", "parent", "detach", "-h"}, requiredFlags: []string{"--author"}},
		{args: []string{"rel", "list", "-h"}, requiredFlags: nil},
		{args: []string{"rel", "tree", "-h"}, requiredFlags: nil},
		{args: []string{"rel", "cycles", "-h"}, requiredFlags: nil},

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
