//go:build blackbox

package blackbox_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rogpeppe/go-internal/testscript"
)

// testscriptCmds returns the map of custom commands available in .txtar
// scripts. Each command handles the "Given" phase of a test — initializing
// databases, seeding issues, and setting environment variables for value
// threading between commands.
func testscriptCmds() map[string]func(ts *testscript.TestScript, neg bool, args []string) {
	return map[string]func(ts *testscript.TestScript, neg bool, args []string){
		"np-init":           cmdNPInit,
		"np-seed":           cmdNPSeed,
		"np-seed-comment":   cmdNPSeedComment,
		"np-seed-rel":       cmdNPSeedRel,
		"np-seed-sql":       cmdNPSeedSQL,
		"np-capture":        cmdNPCapture,
		"np-json-has":       cmdNPJSONHas,
		"np-json-array-len": cmdNPJSONArrayLen,
		"np-json-no-field":  cmdNPJSONNoField,
		"np-comment-long":   cmdNPCommentLong,
	}
}

// cmdNPInit initializes an np workspace in $WORK with the given prefix.
//
// Usage:
//
//	np-init PREFIX
//
// Runs "np init PREFIX" in the testscript working directory. Fails the
// test if initialization does not succeed.
func cmdNPInit(ts *testscript.TestScript, neg bool, args []string) {
	if neg {
		ts.Fatalf("np-init does not support negation")
	}
	if len(args) != 1 {
		ts.Fatalf("usage: np-init PREFIX")
	}

	ts.Exec("np", "init", args[0])
}

// cmdNPSeed creates an issue and sets environment variables for subsequent
// commands to reference.
//
// Usage:
//
//	np-seed ROLE TITLE AUTHOR ENV_VAR [flags...]
//
// Supported flags:
//
//	--parent $ENV_VAR    — sets the parent issue ID
//	--claim              — also claims the issue; sets ${ENV_VAR}_CLAIM
//	--priority P0|P1|P2  — sets the priority (default: P2)
//	--label key:val  — adds a label (repeatable)
//
// On success, sets $ENV_VAR to the created issue ID. If --claim is
// specified, also sets ${ENV_VAR}_CLAIM to the claim ID.
func cmdNPSeed(ts *testscript.TestScript, neg bool, args []string) {
	if neg {
		ts.Fatalf("np-seed does not support negation")
	}
	if len(args) < 4 {
		ts.Fatalf("usage: np-seed ROLE TITLE AUTHOR ENV_VAR [--parent $VAR] [--claim] [--priority P0] [--label key:val]")
	}

	role := args[0]
	title := args[1]
	author := args[2]
	envVar := args[3]

	// Parse optional flags from remaining args.
	var parent string
	var claim bool
	var priority string
	var labels []string

	for i := 4; i < len(args); i++ {
		switch args[i] {
		case "--parent":
			i++
			if i >= len(args) {
				ts.Fatalf("--parent requires a value")
			}
			parent = args[i]
		case "--claim":
			claim = true
		case "--priority":
			i++
			if i >= len(args) {
				ts.Fatalf("--priority requires a value")
			}
			priority = args[i]
		case "--label":
			i++
			if i >= len(args) {
				ts.Fatalf("--label requires a value")
			}
			labels = append(labels, args[i])
		default:
			ts.Fatalf("unknown flag: %s", args[i])
		}
	}

	// Build the JSON payload for np json create.
	payload := map[string]any{
		"role":  role,
		"title": title,
	}
	if parent != "" {
		payload["parent"] = parent
	}
	if priority != "" {
		payload["priority"] = priority
	}
	if len(labels) > 0 {
		payload["labels"] = labels
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		ts.Fatalf("np-seed: failed to marshal JSON payload: %v", err)
	}

	// Write the JSON payload to a file in the work directory and use shell
	// redirection to pipe it to np json create. We use sh because ts.Exec
	// does not support piping stdin directly.
	inputPath := filepath.Join(ts.Getenv("WORK"), ".np-seed-input.json")
	if writeErr := os.WriteFile(inputPath, payloadBytes, 0o600); writeErr != nil {
		ts.Fatalf("np-seed: failed to write input file: %v", writeErr)
	}

	// Claiming is now a CLI flag (--with-claim) rather than a JSON field.
	claimFlag := ""
	if claim {
		claimFlag = " --with-claim"
	}
	ts.Exec("sh", "-c", fmt.Sprintf("np json create --author '%s'%s < '%s'", author, claimFlag, inputPath))
	stdout := ts.ReadFile("stdout")

	// Parse the JSON output to extract the issue ID.
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		ts.Fatalf("np-seed: invalid JSON from np json create: %v\nstdout: %s", err, stdout)
	}

	id, ok := result["id"].(string)
	if !ok || id == "" {
		ts.Fatalf("np-seed: missing 'id' in create output: %s", stdout)
	}
	ts.Setenv(envVar, id)

	if claim {
		claimID, ok := result["claim_id"].(string)
		if !ok || claimID == "" {
			ts.Fatalf("np-seed: --claim specified but no 'claim_id' in output: %s", stdout)
		}
		ts.Setenv(envVar+"_CLAIM", claimID)
	}
}

// cmdNPCapture extracts a field from the last exec's JSON stdout and sets
// an environment variable to its value.
//
// Usage:
//
//	np-capture ENV_VAR JSON_FIELD
//
// Parses stdout from the most recent exec as JSON, extracts the named
// field, and sets $ENV_VAR to its string representation. Numbers without
// fractional parts are formatted as integers (e.g., "1" not "1.000000").
func cmdNPCapture(ts *testscript.TestScript, neg bool, args []string) {
	if neg {
		ts.Fatalf("np-capture does not support negation")
	}
	if len(args) != 2 {
		ts.Fatalf("usage: np-capture ENV_VAR JSON_FIELD")
	}

	envVar := args[0]
	field := args[1]

	result := parseJSONStdout(ts)

	val, exists := result[field]
	if !exists {
		ts.Fatalf("np-capture: field %q not found in JSON output", field)
	}

	ts.Setenv(envVar, formatJSONValue(val))
}

// cmdNPJSONHas asserts that a JSON field in the last exec's stdout has the
// expected value.
//
// Usage:
//
//	np-json-has FIELD VALUE
//	! np-json-has FIELD VALUE    (asserts the field does NOT have that value)
//
// The VALUE is compared as a string after formatting the JSON value. For
// booleans, use "true" or "false". For numbers, use the integer form when
// there is no fractional part (e.g., "0" not "0.000000").
func cmdNPJSONHas(ts *testscript.TestScript, neg bool, args []string) {
	if len(args) != 2 {
		ts.Fatalf("usage: np-json-has FIELD VALUE")
	}

	field := args[0]
	expected := args[1]

	result := parseJSONStdout(ts)

	val, exists := result[field]
	if !exists {
		if neg {
			return // field absent — negated assertion passes
		}
		ts.Fatalf("np-json-has: field %q not found in JSON output", field)
	}

	actual := formatJSONValue(val)
	if neg {
		if actual == expected {
			ts.Fatalf("np-json-has: field %q has value %q (expected it NOT to)", field, expected)
		}
		return
	}

	if actual != expected {
		ts.Fatalf("np-json-has: field %q: got %q, want %q", field, actual, expected)
	}
}

// cmdNPJSONArrayLen asserts that a JSON array field in the last exec's
// stdout has exactly N elements.
//
// Usage:
//
//	np-json-array-len FIELD N
//	! np-json-array-len FIELD N    (asserts the array does NOT have N elements)
func cmdNPJSONArrayLen(ts *testscript.TestScript, neg bool, args []string) {
	if len(args) != 2 {
		ts.Fatalf("usage: np-json-array-len FIELD N")
	}

	field := args[0]
	expected, err := strconv.Atoi(args[1])
	if err != nil {
		ts.Fatalf("np-json-array-len: N must be an integer, got %q", args[1])
	}

	result := parseJSONStdout(ts)

	val, exists := result[field]
	if !exists {
		if neg {
			return
		}
		ts.Fatalf("np-json-array-len: field %q not found in JSON output", field)
	}

	arr, ok := val.([]any)
	if !ok {
		ts.Fatalf("np-json-array-len: field %q is not an array", field)
	}

	if neg {
		if len(arr) == expected {
			ts.Fatalf("np-json-array-len: field %q has %d elements (expected it NOT to)", field, expected)
		}
		return
	}

	if len(arr) != expected {
		ts.Fatalf("np-json-array-len: field %q: got %d elements, want %d", field, len(arr), expected)
	}
}

// cmdNPJSONNoField asserts that a field is absent from the last exec's
// JSON stdout.
//
// Usage:
//
//	np-json-no-field FIELD
//	! np-json-no-field FIELD    (asserts the field IS present)
func cmdNPJSONNoField(ts *testscript.TestScript, neg bool, args []string) {
	if len(args) != 1 {
		ts.Fatalf("usage: np-json-no-field FIELD")
	}

	field := args[0]
	result := parseJSONStdout(ts)

	_, exists := result[field]
	if neg {
		if !exists {
			ts.Fatalf("np-json-no-field: field %q not found (expected it to be present)", field)
		}
		return
	}

	if exists {
		ts.Fatalf("np-json-no-field: field %q is present (expected it to be absent)", field)
	}
}

// --- Helper functions for JSON parsing ---

// parseJSONStdout parses the last exec's stdout as a JSON object.
func parseJSONStdout(ts *testscript.TestScript) map[string]any {
	raw := strings.TrimSpace(ts.ReadFile("stdout"))
	var result map[string]any
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		ts.Fatalf("invalid JSON in stdout: %v\nraw: %s", err, raw)
	}
	return result
}

// formatJSONValue formats a JSON value for comparison. Numbers without
// fractional parts are formatted as integers for ergonomic matching
// (e.g., "1" instead of "1.000000").
func formatJSONValue(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case bool:
		if val {
			return "true"
		}
		return "false"
	case nil:
		return "null"
	default:
		b, _ := json.Marshal(val) // #nosec G703 — best-effort formatting for test output
		return string(b)
	}
}

// cmdNPSeedComment adds a comment to an issue.
//
// Usage:
//
//	np-seed-comment ISSUE_ENV BODY AUTHOR
//
// ISSUE_ENV is the name of an environment variable holding the issue ID
// (set by a prior np-seed call). BODY is the comment text. AUTHOR is the
// author name for the comment.
func cmdNPSeedComment(ts *testscript.TestScript, neg bool, args []string) {
	if neg {
		ts.Fatalf("np-seed-comment does not support negation")
	}
	if len(args) != 3 {
		ts.Fatalf("usage: np-seed-comment ISSUE_ENV BODY AUTHOR")
	}

	issueID := ts.Getenv(args[0])
	if issueID == "" {
		ts.Fatalf("np-seed-comment: environment variable %s is not set", args[0])
	}
	body := args[1]
	author := args[2]

	payload := map[string]string{"body": body}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		ts.Fatalf("np-seed-comment: failed to marshal JSON payload: %v", err)
	}

	inputPath := filepath.Join(ts.Getenv("WORK"), ".np-seed-comment-input.json")
	if writeErr := os.WriteFile(inputPath, payloadBytes, 0o600); writeErr != nil {
		ts.Fatalf("np-seed-comment: failed to write input file: %v", writeErr)
	}
	ts.Exec("sh", "-c", fmt.Sprintf(
		"np json comment '%s' --author '%s' < '%s'",
		issueID, author, inputPath,
	))
}

// cmdNPSeedRel adds a relationship between two issues.
//
// Usage:
//
//	np-seed-rel SOURCE_ENV REL_TYPE TARGET_ENV AUTHOR
//
// SOURCE_ENV and TARGET_ENV are names of environment variables holding issue
// IDs (set by prior np-seed calls). REL_TYPE is the relationship type
// (blocked_by, blocks, refs, parent_of, child_of).
func cmdNPSeedRel(ts *testscript.TestScript, neg bool, args []string) {
	if neg {
		ts.Fatalf("np-seed-rel does not support negation")
	}
	if len(args) != 4 {
		ts.Fatalf("usage: np-seed-rel SOURCE_ENV REL_TYPE TARGET_ENV AUTHOR")
	}

	sourceID := ts.Getenv(args[0])
	if sourceID == "" {
		ts.Fatalf("np-seed-rel: environment variable %s is not set", args[0])
	}
	relType := args[1]
	targetID := ts.Getenv(args[2])
	if targetID == "" {
		ts.Fatalf("np-seed-rel: environment variable %s is not set", args[2])
	}
	author := args[3]

	ts.Exec("np", "rel", "add",
		sourceID, relType, targetID,
		"--author", author,
		"--json",
	)
}

// cmdNPSeedSQL executes a raw SQL statement against the testscript's np
// database. This enables seeding specific data (e.g., issue IDs with known
// Crockford-confusable characters) that cannot be controlled through
// np commands.
//
// Usage:
//
//	np-seed-sql SQL_STATEMENT
//
// The SQL statement is executed against $WORK/.np/nitpicking.db using the
// sqlite3 command-line tool. Fails if sqlite3 is not on PATH or if the
// statement errors.
func cmdNPSeedSQL(ts *testscript.TestScript, neg bool, args []string) {
	if neg {
		ts.Fatalf("np-seed-sql does not support negation")
	}
	if len(args) != 1 {
		ts.Fatalf("usage: np-seed-sql SQL_STATEMENT")
	}

	dbPath := filepath.Join(ts.Getenv("WORK"), ".np", "nitpicking.db")
	cmd := exec.Command("sqlite3", dbPath, args[0])
	out, err := cmd.CombinedOutput()
	if err != nil {
		ts.Fatalf("np-seed-sql: sqlite3 failed: %v\noutput: %s\nsql: %s", err, out, args[0])
	}
}

// cmdNPCommentLong adds a comment with a very long body (~12KB) to an issue
// and verifies it persists. This enables testing large content handling
// without embedding 12KB of text in a .txtar file.
//
// Usage:
//
//	np-comment-long ISSUE_ENV AUTHOR MIN_LENGTH
//
// ISSUE_ENV is the name of an environment variable holding the issue ID.
// AUTHOR is the comment author. MIN_LENGTH is the minimum expected body
// length after round-tripping through create and list.
func cmdNPCommentLong(ts *testscript.TestScript, neg bool, args []string) {
	if neg {
		ts.Fatalf("np-comment-long does not support negation")
	}
	if len(args) != 3 {
		ts.Fatalf("usage: np-comment-long ISSUE_ENV AUTHOR MIN_LENGTH")
	}

	issueID := ts.Getenv(args[0])
	if issueID == "" {
		ts.Fatalf("np-comment-long: environment variable %s is not set", args[0])
	}
	author := args[1]
	minLength, err := strconv.Atoi(args[2])
	if err != nil {
		ts.Fatalf("np-comment-long: MIN_LENGTH must be integer, got %q", args[2])
	}

	// Generate a long body (~12KB).
	longBody := strings.TrimSpace(strings.Repeat("A long comment body. ", 600))

	// Add the comment via json comment with stdin.
	payload := map[string]string{"body": longBody}
	payloadBytes, err2 := json.Marshal(payload)
	if err2 != nil {
		ts.Fatalf("np-comment-long: failed to marshal JSON payload: %v", err2)
	}

	inputPath := filepath.Join(ts.Getenv("WORK"), ".np-comment-long-input.json")
	if writeErr := os.WriteFile(inputPath, payloadBytes, 0o600); writeErr != nil {
		ts.Fatalf("np-comment-long: failed to write input file: %v", writeErr)
	}
	ts.Exec("sh", "-c", fmt.Sprintf(
		"np json comment '%s' --author '%s' < '%s'",
		issueID, author, inputPath,
	))

	// Verify it persists via list.
	ts.Exec("np", "comment", "list",
		issueID,
		"--json",
	)
	stdout := ts.ReadFile("stdout")

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		ts.Fatalf("np-comment-long: invalid JSON from comment list: %v", err)
	}

	comments, ok := result["comments"].([]any)
	if !ok || len(comments) == 0 {
		ts.Fatalf("np-comment-long: expected at least one comment")
	}

	first, ok := comments[0].(map[string]any)
	if !ok {
		ts.Fatalf("np-comment-long: comment is not an object")
	}
	body, ok := first["body"].(string)
	if !ok {
		ts.Fatalf("np-comment-long: body is not a string")
	}
	if len(body) < minLength {
		ts.Fatalf("np-comment-long: body length %d < minimum %d", len(body), minLength)
	}
}
