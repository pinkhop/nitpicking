//go:build e2e

package e2e_test

import (
	"os/exec"
	"strings"
	"testing"
)

// runNPWithEnv executes the np binary with additional environment variables.
// Existing process environment is inherited; env entries override or extend it.
func runNPWithEnv(t *testing.T, dir string, env []string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()

	binary := npBinary(t)
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	cmd.Env = append(cmd.Environ(), env...)

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

func TestE2E_ClaimEnvVar_UpdateUsesNPCLAIM(t *testing.T) {
	// Given — a claimed task with a known claim ID.
	dir := initDB(t, "CLENV")
	author := "env-agent"
	taskID, claimID := seedClaimedTask(t, dir, "Env claim test", author)

	// When — update the title using NP_CLAIM env var instead of --claim flag.
	_, stderr, code := runNPWithEnv(t, dir,
		[]string{"NP_CLAIM=" + claimID},
		"update", taskID,
		"--title", "Updated via env",
		"--json",
	)

	// Then — the update succeeds.
	if code != 0 {
		t.Fatalf("update with NP_CLAIM failed (exit %d): %s", code, stderr)
	}
	issue := showIssue(t, dir, taskID)
	if issue["title"] != "Updated via env" {
		t.Errorf("expected title 'Updated via env', got %v", issue["title"])
	}
}
