package epiccmd

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/sqlite"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

func TestChildren_MissingID_ReturnsFlagError(t *testing.T) {
	t.Parallel()

	f, _ := setupCommandTest(t)
	cmd := newChildrenCmd(f)

	err := cmd.Run(t.Context(), []string{"children"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "epic ID argument is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChildren_InvalidID_ReturnsFlagError(t *testing.T) {
	t.Parallel()

	f, _ := setupCommandTest(t)
	cmd := newChildrenCmd(f)

	err := cmd.Run(t.Context(), []string{"children", "not-an-id"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid epic ID") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChildren_EmptyResults_PrintsNoChildrenFound(t *testing.T) {
	t.Parallel()

	f, svc := setupCommandTest(t)
	epicID := createEpic(t, svc, "Empty epic")
	cmd := newChildrenCmd(f)

	err := cmd.Run(t.Context(), []string{"children", epicID.String()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := f.IOStreams.Out.(*strings.Builder).String(); !strings.Contains(got, "No children found.") {
		t.Fatalf("expected empty-state output, got: %q", got)
	}
}

func TestChildren_JSONOutput_RespectsLimitAndHasMore(t *testing.T) {
	t.Parallel()

	f, svc := setupCommandTest(t)
	epicID := createEpic(t, svc, "Epic")
	_ = createTask(t, svc, epicID, "First child")
	_ = createTask(t, svc, epicID, "Second child")
	cmd := newChildrenCmd(f)

	err := cmd.Run(t.Context(), []string{"children", epicID.String(), "--limit", "1", "--json"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out childrenOutput
	raw := f.IOStreams.Out.(*strings.Builder).String()
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, raw)
	}
	if len(out.Items) != 1 {
		t.Fatalf("items: got %d, want 1", len(out.Items))
	}
	if !out.HasMore {
		t.Fatal("expected has_more=true")
	}
}

func TestChildren_TextOutput_ShowsHasMoreHint(t *testing.T) {
	t.Parallel()

	f, svc := setupCommandTest(t)
	epicID := createEpic(t, svc, "Epic")
	_ = createTask(t, svc, epicID, "First child")
	_ = createTask(t, svc, epicID, "Second child")
	cmd := newChildrenCmd(f)

	err := cmd.Run(t.Context(), []string{"children", epicID.String(), "--limit", "1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stdout := f.IOStreams.Out.(*strings.Builder).String()
	stderr := f.IOStreams.ErrOut.(*strings.Builder).String()
	if !strings.Contains(stdout, "1 children") {
		t.Fatalf("expected child count in stdout, got: %q", stdout)
	}
	if !strings.Contains(stderr, "Showing 1 children") {
		t.Fatalf("expected pagination hint in stderr, got: %q", stderr)
	}
}

func TestStatus_SingleEpicMode_NonEpicIDReturnsError(t *testing.T) {
	t.Parallel()

	f, svc := setupCommandTest(t)
	taskID := createTask(t, svc, domain.ID{}, "Plain task")
	cmd := newStatusCmd(f)

	err := cmd.Run(t.Context(), []string{"status", taskID.String()})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "is not an epic") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStatus_CompletedOnly_FiltersToCompletedEpics(t *testing.T) {
	t.Parallel()

	f, svc := setupCommandTest(t)
	completedEpic := createEpic(t, svc, "Completed epic")
	incompleteEpic := createEpic(t, svc, "Incomplete epic")
	completedChild := createTask(t, svc, completedEpic, "Closed child")
	_ = createTask(t, svc, incompleteEpic, "Open child")
	claimAndClose(t, svc, completedChild)

	cmd := newStatusCmd(f)
	err := cmd.Run(t.Context(), []string{"status", "--completed-only", "--json"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out struct {
		Epics []epicStatusItem `json:"epics"`
		Count int              `json:"count"`
	}
	raw := f.IOStreams.Out.(*strings.Builder).String()
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, raw)
	}
	if out.Count != 1 {
		t.Fatalf("count: got %d, want 1", out.Count)
	}
	if len(out.Epics) != 1 || out.Epics[0].ID != completedEpic.String() {
		t.Fatalf("unexpected epics: %+v", out.Epics)
	}
}

func TestCloseCompleted_DryRun_JSONListsCompletedEpicsOnly(t *testing.T) {
	t.Parallel()

	f, svc := setupCommandTest(t)
	completedEpic := createEpic(t, svc, "Completed epic")
	incompleteEpic := createEpic(t, svc, "Incomplete epic")
	completedChild := createTask(t, svc, completedEpic, "Closed child")
	_ = createTask(t, svc, incompleteEpic, "Open child")
	claimAndClose(t, svc, completedChild)

	cmd := newCloseCompletedCmd(f)
	err := cmd.Run(t.Context(), []string{"close-completed", "--author", "test-agent", "--dry-run", "--json"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out struct {
		Results []closeResult `json:"results"`
		Count   int           `json:"count"`
	}
	raw := f.IOStreams.Out.(*strings.Builder).String()
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, raw)
	}
	if out.Count != 1 {
		t.Fatalf("count: got %d, want 1", out.Count)
	}
	if len(out.Results) != 1 || out.Results[0].ID != completedEpic.String() {
		t.Fatalf("unexpected results: %+v", out.Results)
	}
	if out.Results[0].Message != "dry run" {
		t.Fatalf("message: got %q, want %q", out.Results[0].Message, "dry run")
	}
}

func TestCloseCompleted_IncludeTasks_ClosesParentTaskWithAllChildrenClosed(t *testing.T) {
	t.Parallel()

	// Given — a parent task with all children closed.
	f, svc := setupCommandTest(t)
	parentID := createTask(t, svc, domain.ID{}, "Parent task")
	childID := createTask(t, svc, parentID, "Child task")
	claimAndClose(t, svc, childID)

	// When — run close-completed with --include-tasks and --json.
	cmd := newCloseCompletedCmd(f)
	err := cmd.Run(t.Context(), []string{"close-completed", "--author", "test-agent", "--include-tasks", "--json"})
	// Then — the parent task should appear in results.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out struct {
		Results []closeResult `json:"results"`
		Closed  int           `json:"closed"`
	}
	raw := f.IOStreams.Out.(*strings.Builder).String()
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, raw)
	}
	if out.Closed != 1 {
		t.Fatalf("closed: got %d, want 1", out.Closed)
	}
	if len(out.Results) != 1 || out.Results[0].ID != parentID.String() {
		t.Fatalf("unexpected results: %+v", out.Results)
	}
}

func setupCommandTest(t *testing.T) (*cmdutil.Factory, driving.Service) {
	t.Helper()

	svc, store := setupServiceWithStore(t)
	ios := newTestIOStreams()
	f := &cmdutil.Factory{
		IOStreams: ios,
		Store: func() (*sqlite.Store, error) {
			return store, nil
		},
	}
	return f, svc
}

func setupServiceWithStore(t *testing.T) (driving.Service, *sqlite.Store) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "nitpicking.db")
	store, err := sqlite.Create(dbPath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})

	svc := core.New(store)
	if err := svc.Init(t.Context(), "NP"); err != nil {
		t.Fatalf("init store: %v", err)
	}
	return svc, store
}

func newTestIOStreams() *iostreams.IOStreams {
	return &iostreams.IOStreams{
		In:     nil,
		Out:    &strings.Builder{},
		ErrOut: &strings.Builder{},
	}
}

func mustAuthor(t *testing.T, name string) string {
	t.Helper()
	return name
}

func createEpic(t *testing.T, svc driving.Service, title string) domain.ID {
	t.Helper()

	out, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleEpic,
		Title:  title,
		Author: mustAuthor(t, "test-agent"),
	})
	if err != nil {
		t.Fatalf("create epic: %v", err)
	}
	return out.Issue.ID()
}

func createTask(t *testing.T, svc driving.Service, parentID domain.ID, title string) domain.ID {
	t.Helper()

	var parentStr string
	if !parentID.IsZero() {
		parentStr = parentID.String()
	}

	out, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    title,
		ParentID: parentStr,
		Author:   mustAuthor(t, "test-agent"),
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	return out.Issue.ID()
}

func claimAndClose(t *testing.T, svc driving.Service, id domain.ID) {
	t.Helper()

	claimOut, err := svc.ClaimByID(t.Context(), driving.ClaimInput{
		IssueID: id.String(),
		Author:  mustAuthor(t, "test-agent"),
	})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := svc.TransitionState(t.Context(), driving.TransitionInput{
		IssueID: id.String(),
		ClaimID: claimOut.ClaimID,
		Action:  driving.ActionClose,
	}); err != nil {
		t.Fatalf("close: %v", err)
	}
}
