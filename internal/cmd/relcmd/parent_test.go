package relcmd_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/sqlite"
	"github.com/pinkhop/nitpicking/internal/cmd/relcmd"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- Test Helpers ---

// setupParentCommandTest creates a Factory backed by a real SQLite store and
// returns the Factory and the driving.Service for seeding test data.
func setupParentCommandTest(t *testing.T) (*cmdutil.Factory, driving.Service) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "nitpicking.db")
	store, err := sqlite.Create(dbPath)
	if err != nil {
		t.Fatalf("precondition: create store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})

	svc := core.New(store, store)
	if err := svc.Init(t.Context(), "NP"); err != nil {
		t.Fatalf("precondition: init store: %v", err)
	}

	ios := &iostreams.IOStreams{
		In:     nil,
		Out:    &strings.Builder{},
		ErrOut: &strings.Builder{},
	}
	f := &cmdutil.Factory{
		IOStreams: ios,
		Store: func() (*sqlite.Store, error) {
			return store, nil
		},
	}
	return f, svc
}

// createParentTestEpic creates an epic with the given title and returns its ID.
func createParentTestEpic(t *testing.T, svc driving.Service, title string) domain.ID {
	t.Helper()
	out, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleEpic,
		Title:  title,
		Author: "test-agent",
	})
	if err != nil {
		t.Fatalf("precondition: create epic: %v", err)
	}
	return out.Issue.ID()
}

// createParentTestTask creates a task with the given title and parent, returning
// its ID.
func createParentTestTask(t *testing.T, svc driving.Service, parentID domain.ID, title string) domain.ID {
	t.Helper()
	var parentStr string
	if !parentID.IsZero() {
		parentStr = parentID.String()
	}
	out, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    title,
		ParentID: parentStr,
		Author:   "test-agent",
	})
	if err != nil {
		t.Fatalf("precondition: create task: %v", err)
	}
	return out.Issue.ID()
}

// --- Children Command Tests ---

// TestChildren_LimitFlag_RespectsExplicitLimit verifies that passing --limit N
// caps the children list to N items and sets has_more when more exist.
func TestChildren_LimitFlag_RespectsExplicitLimit(t *testing.T) {
	t.Parallel()

	// Given: an epic with two children.
	f, svc := setupParentCommandTest(t)
	epicID := createParentTestEpic(t, svc, "Epic with children")
	_ = createParentTestTask(t, svc, epicID, "Child A")
	_ = createParentTestTask(t, svc, epicID, "Child B")
	cmd := relcmd.NewCmd(f)

	// When: listing children with --limit 1 --json.
	err := cmd.Run(t.Context(), []string{"rel", "parent", "children", epicID.String(), "--limit", "1", "--json"})
	// Then: only one child is returned and has_more is true.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out cmdutil.ListOutput
	raw := f.IOStreams.Out.(*strings.Builder).String()
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, raw)
	}
	if len(out.Items) != 1 {
		t.Errorf("items: got %d, want 1", len(out.Items))
	}
	if !out.HasMore {
		t.Error("expected has_more=true")
	}
}

// TestChildren_NoLimitFlag_ReturnsAllChildren verifies that --no-limit removes
// the default cap and returns all children.
func TestChildren_NoLimitFlag_ReturnsAllChildren(t *testing.T) {
	t.Parallel()

	// Given: an epic with two children.
	f, svc := setupParentCommandTest(t)
	epicID := createParentTestEpic(t, svc, "Epic with children")
	_ = createParentTestTask(t, svc, epicID, "Child A")
	_ = createParentTestTask(t, svc, epicID, "Child B")
	cmd := relcmd.NewCmd(f)

	// When: listing children with --no-limit --json.
	err := cmd.Run(t.Context(), []string{"rel", "parent", "children", epicID.String(), "--no-limit", "--json"})
	// Then: both children are returned and has_more is false.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out cmdutil.ListOutput
	raw := f.IOStreams.Out.(*strings.Builder).String()
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, raw)
	}
	if len(out.Items) != 2 {
		t.Errorf("items: got %d, want 2", len(out.Items))
	}
	if out.HasMore {
		t.Error("expected has_more=false")
	}
}

// TestChildren_DefaultLimit_MatchesDefaultLimit verifies that when neither
// --limit nor --no-limit is provided, the default limit of 20 applies.
func TestChildren_DefaultLimit_MatchesDefaultLimit(t *testing.T) {
	t.Parallel()

	// Given: an epic with two children (fewer than default limit).
	f, svc := setupParentCommandTest(t)
	epicID := createParentTestEpic(t, svc, "Epic with children")
	_ = createParentTestTask(t, svc, epicID, "Child A")
	_ = createParentTestTask(t, svc, epicID, "Child B")
	cmd := relcmd.NewCmd(f)

	// When: listing children with only --json (no limit flags).
	err := cmd.Run(t.Context(), []string{"rel", "parent", "children", epicID.String(), "--json"})
	// Then: both children are returned (under the default 20 cap).
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out cmdutil.ListOutput
	raw := f.IOStreams.Out.(*strings.Builder).String()
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, raw)
	}
	if len(out.Items) != 2 {
		t.Errorf("items: got %d, want 2", len(out.Items))
	}
	if out.HasMore {
		t.Error("expected has_more=false when count is under default limit")
	}
}

// TestChildren_InvalidLimitValue_ReturnsFlagError verifies that a non-positive
// --limit value produces a flag error.
func TestChildren_InvalidLimitValue_ReturnsFlagError(t *testing.T) {
	t.Parallel()

	// Given: an epic (children don't matter for validation).
	f, svc := setupParentCommandTest(t)
	epicID := createParentTestEpic(t, svc, "Epic")
	cmd := relcmd.NewCmd(f)

	// When: passing --limit 0.
	err := cmd.Run(t.Context(), []string{"rel", "parent", "children", epicID.String(), "--limit", "0", "--json"})

	// Then: a flag error is returned.
	if err == nil {
		t.Fatal("expected error for --limit 0, got nil")
	}
	if !strings.Contains(err.Error(), "--limit must be a positive integer") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestChildren_TextOutput_ShowsNoLimitHint verifies that truncated text output
// mentions --no-limit as the way to retrieve all results.
func TestChildren_TextOutput_ShowsNoLimitHint(t *testing.T) {
	t.Parallel()

	// Given: an epic with two children.
	f, svc := setupParentCommandTest(t)
	epicID := createParentTestEpic(t, svc, "Epic with children")
	_ = createParentTestTask(t, svc, epicID, "Child A")
	_ = createParentTestTask(t, svc, epicID, "Child B")
	cmd := relcmd.NewCmd(f)

	// When: listing children with --limit 1 (text output).
	err := cmd.Run(t.Context(), []string{"rel", "parent", "children", epicID.String(), "--limit", "1"})
	// Then: stdout mentions --no-limit as a hint.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	stdout := f.IOStreams.Out.(*strings.Builder).String()
	if !strings.Contains(stdout, "--no-limit") {
		t.Errorf("expected --no-limit hint in output, got: %q", stdout)
	}
}
