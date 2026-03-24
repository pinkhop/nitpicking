//go:build e2e

package e2e_test

import "testing"

func TestE2E_AuthorEnvVar_CommentAddUsesNPAUTHOR(t *testing.T) {
	// Given — an issue exists.
	dir := initDB(t, "AENV")
	taskID := createTask(t, dir, "Author env test", "setup-agent")

	// When — add a comment using NP_AUTHOR env var instead of --author flag.
	stdout, stderr, code := runNPWithEnv(t, dir,
		[]string{"NP_AUTHOR=env-author"},
		"comment", "add",
		"--issue", taskID,
		"--body", "Comment from env author",
		"--json",
	)

	// Then — the comment is created with the env-supplied author.
	if code != 0 {
		t.Fatalf("comment add with NP_AUTHOR failed (exit %d): %s", code, stderr)
	}
	result := parseJSON(t, stdout)
	if result["author"] != "env-author" {
		t.Errorf("expected author 'env-author', got %v", result["author"])
	}
}
