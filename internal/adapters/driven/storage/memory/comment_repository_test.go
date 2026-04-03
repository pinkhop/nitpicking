package memory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driven"
)

// ---------------------------------------------------------------------------
// helpers (unique to this file — shared helpers like mustTask, mustAuthor,
// mustEpic, mustLabel, mustRelationship are in issue_repository_test.go)
// ---------------------------------------------------------------------------

// mustParseID parses a known-good issue ID string or fails the test.
func mustParseID(t *testing.T, s string) domain.ID {
	t.Helper()
	id, err := domain.ParseID(s)
	if err != nil {
		t.Fatalf("precondition: parse issue ID %q: %v", s, err)
	}
	return id
}

// mustComment creates a Comment or fails the test.
func mustComment(t *testing.T, p domain.NewCommentParams) domain.Comment {
	t.Helper()
	c, err := domain.NewComment(p)
	if err != nil {
		t.Fatalf("precondition: create comment: %v", err)
	}
	return c
}

// seedTask creates a task and stores it in the repository. Uses a fixed
// creation time so that the issue_repository_test.go mustTask helper (which
// requires a time parameter) is not duplicated.
func seedTask(t *testing.T, ctx context.Context, repo *memory.Repository, id domain.ID, title string) domain.Issue {
	t.Helper()
	iss := mustTask(t, id, title, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err := repo.CreateIssue(ctx, iss); err != nil {
		t.Fatalf("precondition: store task %q: %v", id, err)
	}
	return iss
}

// seedComment creates and persists a comment, returning it with the assigned ID.
func seedComment(t *testing.T, ctx context.Context, repo *memory.Repository, issueID domain.ID, author domain.Author, body string, createdAt time.Time) domain.Comment {
	t.Helper()
	c := mustComment(t, domain.NewCommentParams{
		IssueID:   issueID,
		Author:    author,
		Body:      body,
		CreatedAt: createdAt,
	})
	id, err := repo.CreateComment(ctx, c)
	if err != nil {
		t.Fatalf("precondition: create comment: %v", err)
	}
	stored, err := repo.GetComment(ctx, id)
	if err != nil {
		t.Fatalf("precondition: get comment %d: %v", id, err)
	}
	return stored
}

// ---------------------------------------------------------------------------
// CreateComment
// ---------------------------------------------------------------------------

func TestCreateComment_AssignsSequentialIDs(t *testing.T) {
	t.Parallel()

	// Given
	ctx := context.Background()
	repo := memory.NewRepository()
	issueID := mustParseID(t, "NP-abc01")
	seedTask(t, ctx, repo, issueID, "Test issue")
	author := mustAuthor(t, "alice")

	c1 := mustComment(t, domain.NewCommentParams{
		IssueID:   issueID,
		Author:    author,
		Body:      "First comment",
		CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	c2 := mustComment(t, domain.NewCommentParams{
		IssueID:   issueID,
		Author:    author,
		Body:      "Second comment",
		CreatedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
	})

	// When
	id1, err1 := repo.CreateComment(ctx, c1)
	id2, err2 := repo.CreateComment(ctx, c2)

	// Then
	if err1 != nil {
		t.Fatalf("CreateComment(c1): %v", err1)
	}
	if err2 != nil {
		t.Fatalf("CreateComment(c2): %v", err2)
	}
	if id1 >= id2 {
		t.Errorf("expected id1 (%d) < id2 (%d): IDs should be sequential", id1, id2)
	}
}

func TestCreateComment_PreservesFields(t *testing.T) {
	t.Parallel()

	// Given
	ctx := context.Background()
	repo := memory.NewRepository()
	issueID := mustParseID(t, "NP-abc02")
	seedTask(t, ctx, repo, issueID, "Test issue")
	author := mustAuthor(t, "bob")
	createdAt := time.Date(2026, 3, 15, 10, 30, 0, 0, time.UTC)

	c := mustComment(t, domain.NewCommentParams{
		IssueID:   issueID,
		Author:    author,
		Body:      "My comment body",
		CreatedAt: createdAt,
	})

	// When
	id, err := repo.CreateComment(ctx, c)
	// Then
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}

	stored, err := repo.GetComment(ctx, id)
	if err != nil {
		t.Fatalf("GetComment(%d): %v", id, err)
	}
	if stored.IssueID() != issueID {
		t.Errorf("IssueID = %s, want %s", stored.IssueID(), issueID)
	}
	if !stored.Author().Equal(author) {
		t.Errorf("Author = %s, want %s", stored.Author(), author)
	}
	if stored.Body() != "My comment body" {
		t.Errorf("Body = %q, want %q", stored.Body(), "My comment body")
	}
	if !stored.CreatedAt().Equal(createdAt) {
		t.Errorf("CreatedAt = %v, want %v", stored.CreatedAt(), createdAt)
	}
}

// ---------------------------------------------------------------------------
// GetComment
// ---------------------------------------------------------------------------

func TestGetComment_Exists_ReturnsComment(t *testing.T) {
	t.Parallel()

	// Given
	ctx := context.Background()
	repo := memory.NewRepository()
	issueID := mustParseID(t, "NP-abc03")
	seedTask(t, ctx, repo, issueID, "Test issue")
	author := mustAuthor(t, "alice")

	c := seedComment(t, ctx, repo, issueID, author, "Hello", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	// When
	got, err := repo.GetComment(ctx, c.ID())
	// Then
	if err != nil {
		t.Fatalf("GetComment: %v", err)
	}
	if got.ID() != c.ID() {
		t.Errorf("ID = %d, want %d", got.ID(), c.ID())
	}
	if got.Body() != "Hello" {
		t.Errorf("Body = %q, want %q", got.Body(), "Hello")
	}
}

func TestGetComment_NotFound_ReturnsErrNotFound(t *testing.T) {
	t.Parallel()

	// Given
	ctx := context.Background()
	repo := memory.NewRepository()

	// When
	_, err := repo.GetComment(ctx, 99999)

	// Then
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("err = %v, want domain.ErrNotFound", err)
	}
}

// ---------------------------------------------------------------------------
// ListComments — basic
// ---------------------------------------------------------------------------

func TestListComments_ReturnsCommentsForIssue(t *testing.T) {
	t.Parallel()

	// Given
	ctx := context.Background()
	repo := memory.NewRepository()
	issueA := mustParseID(t, "NP-abc04")
	issueB := mustParseID(t, "NP-abc05")
	seedTask(t, ctx, repo, issueA, "Issue A")
	seedTask(t, ctx, repo, issueB, "Issue B")
	author := mustAuthor(t, "alice")

	seedComment(t, ctx, repo, issueA, author, "Comment on A", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	seedComment(t, ctx, repo, issueB, author, "Comment on B", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	seedComment(t, ctx, repo, issueA, author, "Another on A", time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))

	// When
	items, hasMore, err := repo.ListComments(ctx, issueA, driven.CommentFilter{}, 0)
	// Then
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if hasMore {
		t.Error("expected hasMore=false")
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	// Verify sorted by ID ascending.
	if items[0].ID() >= items[1].ID() {
		t.Errorf("expected items sorted by ID ascending: %d >= %d", items[0].ID(), items[1].ID())
	}
}

func TestListComments_EmptyIssue_ReturnsEmptySlice(t *testing.T) {
	t.Parallel()

	// Given
	ctx := context.Background()
	repo := memory.NewRepository()
	issueID := mustParseID(t, "NP-abc06")
	seedTask(t, ctx, repo, issueID, "Empty issue")

	// When
	items, hasMore, err := repo.ListComments(ctx, issueID, driven.CommentFilter{}, 0)
	// Then
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if hasMore {
		t.Error("expected hasMore=false")
	}
	if len(items) != 0 {
		t.Errorf("len(items) = %d, want 0", len(items))
	}
}

// ---------------------------------------------------------------------------
// ListComments — pagination
// ---------------------------------------------------------------------------

func TestListComments_LimitExceeded_ReturnsHasMore(t *testing.T) {
	t.Parallel()

	// Given
	ctx := context.Background()
	repo := memory.NewRepository()
	issueID := mustParseID(t, "NP-abc07")
	seedTask(t, ctx, repo, issueID, "Test issue")
	author := mustAuthor(t, "alice")

	for i := range 5 {
		seedComment(t, ctx, repo, issueID, author, "Comment",
			time.Date(2026, 1, 1+i, 0, 0, 0, 0, time.UTC))
	}

	// When
	items, hasMore, err := repo.ListComments(ctx, issueID, driven.CommentFilter{}, 3)
	// Then
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if !hasMore {
		t.Error("expected hasMore=true")
	}
	if len(items) != 3 {
		t.Errorf("len(items) = %d, want 3", len(items))
	}
}

func TestListComments_ZeroLimit_UsesDefault(t *testing.T) {
	t.Parallel()

	// Given — create exactly DefaultLimit+1 comments
	ctx := context.Background()
	repo := memory.NewRepository()
	issueID := mustParseID(t, "NP-abc08")
	seedTask(t, ctx, repo, issueID, "Test issue")
	author := mustAuthor(t, "alice")

	for i := range driven.DefaultLimit + 1 {
		seedComment(t, ctx, repo, issueID, author, "Comment",
			time.Date(2026, 1, 1, i, 0, 0, 0, time.UTC))
	}

	// When — pass 0 for limit
	items, hasMore, err := repo.ListComments(ctx, issueID, driven.CommentFilter{}, 0)
	// Then
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if !hasMore {
		t.Error("expected hasMore=true with DefaultLimit+1 comments")
	}
	if len(items) != driven.DefaultLimit {
		t.Errorf("len(items) = %d, want %d", len(items), driven.DefaultLimit)
	}
}

// ---------------------------------------------------------------------------
// ListComments — filter: Author
// ---------------------------------------------------------------------------

func TestListComments_FilterAuthor_ReturnsMatchingOnly(t *testing.T) {
	t.Parallel()

	// Given
	ctx := context.Background()
	repo := memory.NewRepository()
	issueID := mustParseID(t, "NP-abc09")
	seedTask(t, ctx, repo, issueID, "Test issue")
	alice := mustAuthor(t, "alice")
	bob := mustAuthor(t, "bob")

	seedComment(t, ctx, repo, issueID, alice, "Alice's comment", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	seedComment(t, ctx, repo, issueID, bob, "Bob's comment", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))

	// When
	items, _, err := repo.ListComments(ctx, issueID, driven.CommentFilter{Author: alice}, 0)
	// Then
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if !items[0].Author().Equal(alice) {
		t.Errorf("Author = %s, want %s", items[0].Author(), alice)
	}
}

// ---------------------------------------------------------------------------
// ListComments — filter: Authors (multiple)
// ---------------------------------------------------------------------------

func TestListComments_FilterAuthors_ReturnsAnyMatch(t *testing.T) {
	t.Parallel()

	// Given
	ctx := context.Background()
	repo := memory.NewRepository()
	issueID := mustParseID(t, "NP-abc10")
	seedTask(t, ctx, repo, issueID, "Test issue")
	alice := mustAuthor(t, "alice")
	bob := mustAuthor(t, "bob")
	charlie := mustAuthor(t, "charlie")

	seedComment(t, ctx, repo, issueID, alice, "Alice", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	seedComment(t, ctx, repo, issueID, bob, "Bob", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	seedComment(t, ctx, repo, issueID, charlie, "Charlie", time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))

	// When
	items, _, err := repo.ListComments(ctx, issueID, driven.CommentFilter{
		Authors: []domain.Author{alice, charlie},
	}, 0)
	// Then
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
}

// ---------------------------------------------------------------------------
// ListComments — filter: CreatedAfter
// ---------------------------------------------------------------------------

func TestListComments_FilterCreatedAfter_ExcludesOlderComments(t *testing.T) {
	t.Parallel()

	// Given
	ctx := context.Background()
	repo := memory.NewRepository()
	issueID := mustParseID(t, "NP-abc11")
	seedTask(t, ctx, repo, issueID, "Test issue")
	author := mustAuthor(t, "alice")

	seedComment(t, ctx, repo, issueID, author, "Old", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	seedComment(t, ctx, repo, issueID, author, "New", time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))

	// When
	cutoff := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	items, _, err := repo.ListComments(ctx, issueID, driven.CommentFilter{CreatedAfter: cutoff}, 0)
	// Then
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Body() != "New" {
		t.Errorf("Body = %q, want %q", items[0].Body(), "New")
	}
}

// ---------------------------------------------------------------------------
// ListComments — filter: AfterCommentID
// ---------------------------------------------------------------------------

func TestListComments_FilterAfterCommentID_ExcludesEarlierIDs(t *testing.T) {
	t.Parallel()

	// Given
	ctx := context.Background()
	repo := memory.NewRepository()
	issueID := mustParseID(t, "NP-abc12")
	seedTask(t, ctx, repo, issueID, "Test issue")
	author := mustAuthor(t, "alice")

	c1 := seedComment(t, ctx, repo, issueID, author, "First", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	seedComment(t, ctx, repo, issueID, author, "Second", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	seedComment(t, ctx, repo, issueID, author, "Third", time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))

	// When
	items, _, err := repo.ListComments(ctx, issueID, driven.CommentFilter{AfterCommentID: c1.ID()}, 0)
	// Then
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	for _, item := range items {
		if item.ID() <= c1.ID() {
			t.Errorf("item.ID() = %d, want > %d", item.ID(), c1.ID())
		}
	}
}

// ---------------------------------------------------------------------------
// SearchComments — basic text matching
// ---------------------------------------------------------------------------

func TestSearchComments_MatchesBodyText(t *testing.T) {
	t.Parallel()

	// Given
	ctx := context.Background()
	repo := memory.NewRepository()
	issueID := mustParseID(t, "NP-abc13")
	seedTask(t, ctx, repo, issueID, "Test issue")
	author := mustAuthor(t, "alice")

	seedComment(t, ctx, repo, issueID, author, "The fix is in auth.go", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	seedComment(t, ctx, repo, issueID, author, "Nothing relevant here", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))

	// When
	items, _, err := repo.SearchComments(ctx, "auth.go", driven.CommentFilter{}, 0)
	// Then
	if err != nil {
		t.Fatalf("SearchComments: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Body() != "The fix is in auth.go" {
		t.Errorf("Body = %q, want match", items[0].Body())
	}
}

func TestSearchComments_CaseInsensitive(t *testing.T) {
	t.Parallel()

	// Given
	ctx := context.Background()
	repo := memory.NewRepository()
	issueID := mustParseID(t, "NP-abc14")
	seedTask(t, ctx, repo, issueID, "Test issue")
	author := mustAuthor(t, "alice")

	seedComment(t, ctx, repo, issueID, author, "Found the BUG in parser", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	// When
	items, _, err := repo.SearchComments(ctx, "bug", driven.CommentFilter{}, 0)
	// Then
	if err != nil {
		t.Fatalf("SearchComments: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("len(items) = %d, want 1", len(items))
	}
}

func TestSearchComments_NoMatch_ReturnsEmpty(t *testing.T) {
	t.Parallel()

	// Given
	ctx := context.Background()
	repo := memory.NewRepository()
	issueID := mustParseID(t, "NP-abc15")
	seedTask(t, ctx, repo, issueID, "Test issue")
	author := mustAuthor(t, "alice")

	seedComment(t, ctx, repo, issueID, author, "Some text", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	// When
	items, _, err := repo.SearchComments(ctx, "nonexistent", driven.CommentFilter{}, 0)
	// Then
	if err != nil {
		t.Fatalf("SearchComments: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("len(items) = %d, want 0", len(items))
	}
}

// ---------------------------------------------------------------------------
// SearchComments — filter: IssueID
// ---------------------------------------------------------------------------

func TestSearchComments_FilterIssueID_ScopesToOneIssue(t *testing.T) {
	t.Parallel()

	// Given
	ctx := context.Background()
	repo := memory.NewRepository()
	issueA := mustParseID(t, "NP-abc16")
	issueB := mustParseID(t, "NP-abc17")
	seedTask(t, ctx, repo, issueA, "Issue A")
	seedTask(t, ctx, repo, issueB, "Issue B")
	author := mustAuthor(t, "alice")

	seedComment(t, ctx, repo, issueA, author, "bug fix", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	seedComment(t, ctx, repo, issueB, author, "bug workaround", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))

	// When
	items, _, err := repo.SearchComments(ctx, "bug", driven.CommentFilter{IssueID: issueA}, 0)
	// Then
	if err != nil {
		t.Fatalf("SearchComments: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].IssueID() != issueA {
		t.Errorf("IssueID = %s, want %s", items[0].IssueID(), issueA)
	}
}

// ---------------------------------------------------------------------------
// SearchComments — filter: IssueIDs (multiple)
// ---------------------------------------------------------------------------

func TestSearchComments_FilterIssueIDs_ScopesToMultipleIssues(t *testing.T) {
	t.Parallel()

	// Given
	ctx := context.Background()
	repo := memory.NewRepository()
	issueA := mustParseID(t, "NP-abc18")
	issueB := mustParseID(t, "NP-abc19")
	issueC := mustParseID(t, "NP-abc20")
	seedTask(t, ctx, repo, issueA, "Issue A")
	seedTask(t, ctx, repo, issueB, "Issue B")
	seedTask(t, ctx, repo, issueC, "Issue C")
	author := mustAuthor(t, "alice")

	seedComment(t, ctx, repo, issueA, author, "bug in A", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	seedComment(t, ctx, repo, issueB, author, "bug in B", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	seedComment(t, ctx, repo, issueC, author, "bug in C", time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))

	// When
	items, _, err := repo.SearchComments(ctx, "bug", driven.CommentFilter{
		IssueIDs: []domain.ID{issueA, issueB},
	}, 0)
	// Then
	if err != nil {
		t.Fatalf("SearchComments: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("len(items) = %d, want 2", len(items))
	}
}

// ---------------------------------------------------------------------------
// SearchComments — filter: ParentIDs
// ---------------------------------------------------------------------------

func TestSearchComments_FilterParentIDs_ScopesToChildrenAndParent(t *testing.T) {
	t.Parallel()

	// Given
	ctx := context.Background()
	repo := memory.NewRepository()
	epicID := mustParseID(t, "NP-abc21")
	childID := mustParseID(t, "NP-abc22")
	unrelatedID := mustParseID(t, "NP-abc23")
	author := mustAuthor(t, "alice")

	// Create an epic, a child of that epic, and an unrelated task.
	epic, err := domain.NewEpic(domain.NewEpicParams{ID: epicID, Title: "Parent epic"})
	if err != nil {
		t.Fatalf("NewEpic: %v", err)
	}
	if err := repo.CreateIssue(ctx, epic); err != nil {
		t.Fatalf("CreateIssue(epic): %v", err)
	}

	child, err := domain.NewTask(domain.NewTaskParams{ID: childID, Title: "Child task", ParentID: epicID})
	if err != nil {
		t.Fatalf("NewTask(child): %v", err)
	}
	if err := repo.CreateIssue(ctx, child); err != nil {
		t.Fatalf("CreateIssue(child): %v", err)
	}

	seedTask(t, ctx, repo, unrelatedID, "Unrelated task")

	seedComment(t, ctx, repo, epicID, author, "bug on epic", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	seedComment(t, ctx, repo, childID, author, "bug on child", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	seedComment(t, ctx, repo, unrelatedID, author, "bug elsewhere", time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))

	// When
	items, _, err := repo.SearchComments(ctx, "bug", driven.CommentFilter{
		ParentIDs: []domain.ID{epicID},
	}, 0)
	// Then
	if err != nil {
		t.Fatalf("SearchComments: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("len(items) = %d, want 2 (epic + child)", len(items))
	}
}

// ---------------------------------------------------------------------------
// SearchComments — filter: TreeIDs
// ---------------------------------------------------------------------------

func TestSearchComments_FilterTreeIDs_ScopesToEntireSubtree(t *testing.T) {
	t.Parallel()

	// Given
	ctx := context.Background()
	repo := memory.NewRepository()
	rootID := mustParseID(t, "NP-abc24")
	childID := mustParseID(t, "NP-abc25")
	grandchildID := mustParseID(t, "NP-abc26")
	unrelatedID := mustParseID(t, "NP-abc27")
	author := mustAuthor(t, "alice")

	rootIssue, err := domain.NewEpic(domain.NewEpicParams{ID: rootID, Title: "Root"})
	if err != nil {
		t.Fatalf("NewEpic(root): %v", err)
	}
	if err := repo.CreateIssue(ctx, rootIssue); err != nil {
		t.Fatalf("CreateIssue(root): %v", err)
	}

	childIssue, err := domain.NewEpic(domain.NewEpicParams{ID: childID, Title: "Child", ParentID: rootID})
	if err != nil {
		t.Fatalf("NewEpic(child): %v", err)
	}
	if err := repo.CreateIssue(ctx, childIssue); err != nil {
		t.Fatalf("CreateIssue(child): %v", err)
	}

	gcIssue, err := domain.NewTask(domain.NewTaskParams{ID: grandchildID, Title: "Grandchild", ParentID: childID})
	if err != nil {
		t.Fatalf("NewTask(grandchild): %v", err)
	}
	if err := repo.CreateIssue(ctx, gcIssue); err != nil {
		t.Fatalf("CreateIssue(grandchild): %v", err)
	}

	seedTask(t, ctx, repo, unrelatedID, "Unrelated")

	seedComment(t, ctx, repo, rootID, author, "bug in root", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	seedComment(t, ctx, repo, childID, author, "bug in child", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	seedComment(t, ctx, repo, grandchildID, author, "bug in grandchild", time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	seedComment(t, ctx, repo, unrelatedID, author, "bug elsewhere", time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC))

	// When
	items, _, err := repo.SearchComments(ctx, "bug", driven.CommentFilter{
		TreeIDs: []domain.ID{rootID},
	}, 0)
	// Then
	if err != nil {
		t.Fatalf("SearchComments: %v", err)
	}
	if len(items) != 3 {
		t.Errorf("len(items) = %d, want 3 (root + child + grandchild)", len(items))
	}
}

// ---------------------------------------------------------------------------
// SearchComments — filter: LabelFilters
// ---------------------------------------------------------------------------

func TestSearchComments_FilterLabelFilters_ScopesToLabeledIssues(t *testing.T) {
	t.Parallel()

	// Given
	ctx := context.Background()
	repo := memory.NewRepository()
	bugID := mustParseID(t, "NP-abc28")
	featID := mustParseID(t, "NP-abc29")
	author := mustAuthor(t, "alice")

	bugLabel, err := domain.NewLabel("kind", "bug")
	if err != nil {
		t.Fatalf("NewLabel(kind, bug): %v", err)
	}
	bugIssue, err := domain.NewTask(domain.NewTaskParams{
		ID:     bugID,
		Title:  "Bug task",
		Labels: domain.NewLabelSet().Set(bugLabel),
	})
	if err != nil {
		t.Fatalf("NewTask(bug): %v", err)
	}
	if err := repo.CreateIssue(ctx, bugIssue); err != nil {
		t.Fatalf("CreateIssue(bug): %v", err)
	}

	featLabel, err := domain.NewLabel("kind", "feat")
	if err != nil {
		t.Fatalf("NewLabel(kind, feat): %v", err)
	}
	featIssue, err := domain.NewTask(domain.NewTaskParams{
		ID:     featID,
		Title:  "Feature task",
		Labels: domain.NewLabelSet().Set(featLabel),
	})
	if err != nil {
		t.Fatalf("NewTask(feat): %v", err)
	}
	if err := repo.CreateIssue(ctx, featIssue); err != nil {
		t.Fatalf("CreateIssue(feat): %v", err)
	}

	seedComment(t, ctx, repo, bugID, author, "fix comment", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	seedComment(t, ctx, repo, featID, author, "fix comment", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))

	// When
	items, _, err := repo.SearchComments(ctx, "fix", driven.CommentFilter{
		LabelFilters: []driven.LabelFilter{{Key: "kind", Value: "bug"}},
	}, 0)
	// Then
	if err != nil {
		t.Fatalf("SearchComments: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].IssueID() != bugID {
		t.Errorf("IssueID = %s, want %s", items[0].IssueID(), bugID)
	}
}

// ---------------------------------------------------------------------------
// SearchComments — filter: FollowRefs
// ---------------------------------------------------------------------------

func TestSearchComments_FilterFollowRefs_IncludesReferencedIssues(t *testing.T) {
	t.Parallel()

	// Given
	ctx := context.Background()
	repo := memory.NewRepository()
	sourceID := mustParseID(t, "NP-abc30")
	targetID := mustParseID(t, "NP-abc31")
	unrelatedID := mustParseID(t, "NP-abc32")
	author := mustAuthor(t, "alice")

	seedTask(t, ctx, repo, sourceID, "Source issue")
	seedTask(t, ctx, repo, targetID, "Target issue")
	seedTask(t, ctx, repo, unrelatedID, "Unrelated issue")

	// Create a ref relationship from source → target.
	rel, err := domain.NewRelationship(sourceID, targetID, domain.RelRefs)
	if err != nil {
		t.Fatalf("NewRelationship: %v", err)
	}
	if _, err := repo.CreateRelationship(ctx, rel); err != nil {
		t.Fatalf("CreateRelationship: %v", err)
	}

	seedComment(t, ctx, repo, sourceID, author, "fix here", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	seedComment(t, ctx, repo, targetID, author, "fix there", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	seedComment(t, ctx, repo, unrelatedID, author, "fix unrelated", time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))

	// When — scope to source issue, follow refs to include target.
	items, _, err := repo.SearchComments(ctx, "fix", driven.CommentFilter{
		IssueID:    sourceID,
		FollowRefs: true,
	}, 0)
	// Then
	if err != nil {
		t.Fatalf("SearchComments: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("len(items) = %d, want 2 (source + referenced target)", len(items))
	}
}

// ---------------------------------------------------------------------------
// SearchComments — pagination
// ---------------------------------------------------------------------------

func TestSearchComments_Pagination_LimitAndHasMore(t *testing.T) {
	t.Parallel()

	// Given
	ctx := context.Background()
	repo := memory.NewRepository()
	issueID := mustParseID(t, "NP-abc33")
	seedTask(t, ctx, repo, issueID, "Test issue")
	author := mustAuthor(t, "alice")

	for i := range 5 {
		seedComment(t, ctx, repo, issueID, author, "matching text",
			time.Date(2026, 1, 1+i, 0, 0, 0, 0, time.UTC))
	}

	// When
	items, hasMore, err := repo.SearchComments(ctx, "matching", driven.CommentFilter{}, 3)
	// Then
	if err != nil {
		t.Fatalf("SearchComments: %v", err)
	}
	if !hasMore {
		t.Error("expected hasMore=true")
	}
	if len(items) != 3 {
		t.Errorf("len(items) = %d, want 3", len(items))
	}
}

// ---------------------------------------------------------------------------
// SearchComments — combined filters
// ---------------------------------------------------------------------------

func TestSearchComments_CombinedAuthorAndIssueID(t *testing.T) {
	t.Parallel()

	// Given
	ctx := context.Background()
	repo := memory.NewRepository()
	issueA := mustParseID(t, "NP-abc34")
	issueB := mustParseID(t, "NP-abc35")
	seedTask(t, ctx, repo, issueA, "Issue A")
	seedTask(t, ctx, repo, issueB, "Issue B")
	alice := mustAuthor(t, "alice")
	bob := mustAuthor(t, "bob")

	seedComment(t, ctx, repo, issueA, alice, "bug fix", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	seedComment(t, ctx, repo, issueA, bob, "bug fix", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	seedComment(t, ctx, repo, issueB, alice, "bug fix", time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))

	// When — scope to issueA and alice only
	items, _, err := repo.SearchComments(ctx, "bug", driven.CommentFilter{
		IssueID: issueA,
		Author:  alice,
	}, 0)
	// Then
	if err != nil {
		t.Fatalf("SearchComments: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if !items[0].Author().Equal(alice) {
		t.Errorf("Author = %s, want alice", items[0].Author())
	}
}
