//go:build e2e

package e2e_test

import (
	"os/exec"
	"strings"
	"sync"
	"testing"
)

// runNPAsync executes the np binary and sends the result through channels.
// Designed for use in concurrent test scenarios where multiple np processes
// run against the same database simultaneously.
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

func TestE2E_Concurrency_TwoAgentsClaimSameIssue(t *testing.T) {
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

	wg.Add(2)
	go func() {
		defer wg.Done()
		resultA.stdout, resultA.stderr, resultA.exitCode = runNPAsync(t, dir,
			"claim", "id", taskID, "--author", "agent-alpha", "--json",
		)
	}()
	go func() {
		defer wg.Done()
		resultB.stdout, resultB.stderr, resultB.exitCode = runNPAsync(t, dir,
			"claim", "id", taskID, "--author", "agent-beta", "--json",
		)
	}()
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

func TestE2E_Concurrency_TwoAgentsClaimDifferentIssues(t *testing.T) {
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

	wg.Add(2)
	go func() {
		defer wg.Done()
		resultA.stdout, resultA.stderr, resultA.exitCode = runNPAsync(t, dir,
			"claim", "id", taskA, "--author", "agent-alpha", "--json",
		)
	}()
	go func() {
		defer wg.Done()
		resultB.stdout, resultB.stderr, resultB.exitCode = runNPAsync(t, dir,
			"claim", "id", taskB, "--author", "agent-beta", "--json",
		)
	}()
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

func TestE2E_Concurrency_ClaimReadyClaimsDifferentIssues(t *testing.T) {
	// Given — multiple open tasks with different priorities.
	dir := initDB(t, "CONC")
	createTaskWithPriority(t, dir, "High priority", "setup-agent", "P0")
	createTaskWithPriority(t, dir, "Medium priority", "setup-agent", "P1")
	createTaskWithPriority(t, dir, "Low priority", "setup-agent", "P2")

	// When — two agents simultaneously request the next ready issue via "claim ready".
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

	wg.Add(2)
	go func() {
		defer wg.Done()
		resultA.stdout, resultA.stderr, resultA.exitCode = runNPAsync(t, dir,
			"claim", "ready", "--author", "agent-alpha", "--json",
		)
	}()
	go func() {
		defer wg.Done()
		resultB.stdout, resultB.stderr, resultB.exitCode = runNPAsync(t, dir,
			"claim", "ready", "--author", "agent-beta", "--json",
		)
	}()
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

func TestE2E_Concurrency_ConcurrentNotesOnSameIssue(t *testing.T) {
	// Given — an issue that two agents will add comments to simultaneously.
	// Comments do not require claiming, so both should succeed.
	dir := initDB(t, "CONC")
	taskID := createTask(t, dir, "Shared issue", "setup-agent")

	// When — both agents add comments simultaneously.
	var (
		wg    sync.WaitGroup
		codeA int
		codeB int
	)

	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _, codeA = runNPAsync(t, dir, "comment", "add",
			"--issue", taskID,
			"--body", "Comment from alpha",
			"--author", "agent-alpha",
			"--json",
		)
	}()
	go func() {
		defer wg.Done()
		_, _, codeB = runNPAsync(t, dir, "comment", "add",
			"--issue", taskID,
			"--body", "Comment from beta",
			"--author", "agent-beta",
			"--json",
		)
	}()
	wg.Wait()

	// Then — both comments should be added successfully.
	if codeA != 0 {
		t.Errorf("agent-alpha comment add failed (exit %d)", codeA)
	}
	if codeB != 0 {
		t.Errorf("agent-beta comment add failed (exit %d)", codeB)
	}

	// Verify both comments exist.
	commentStdout, stderr, code := runNP(t, dir, "comment", "list", "--issue", taskID, "--json")
	if code != 0 {
		t.Fatalf("comment list failed (exit %d): %s", code, stderr)
	}
	commentResult := parseJSON(t, commentStdout)
	noteCount, ok := commentResult["total_count"].(float64)
	if !ok || noteCount != 2 {
		t.Errorf("expected 2 comments, got %v", commentResult["total_count"])
	}
}
