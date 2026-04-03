//go:build boundary

package sqlite_test

import (
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- CreateComment and GetComment Roundtrip ---

func TestBoundary_CreateComment_GetComment_Roundtrip(t *testing.T) {
	// Given
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	issueID := createIntTask(t, svc, "Comment roundtrip task")

	// When
	addOut, err := svc.AddComment(ctx, driving.AddCommentInput{
		IssueID: issueID.String(),
		Author:  author(t, "alice"),
		Body:    "This is a roundtrip test comment",
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error adding comment: %v", err)
	}
	if addOut.Comment.CommentID == 0 {
		t.Error("expected non-zero comment ID")
	}
	if addOut.Comment.IssueID != issueID.String() {
		t.Errorf("issue ID: got %s, want %s", addOut.Comment.IssueID, issueID)
	}
	if addOut.Comment.Author != "alice" {
		t.Errorf("author: got %q, want %q", addOut.Comment.Author, "alice")
	}
	if addOut.Comment.Body != "This is a roundtrip test comment" {
		t.Errorf("body: got %q, want %q", addOut.Comment.Body, "This is a roundtrip test comment")
	}
	if addOut.Comment.CreatedAt.IsZero() {
		t.Error("expected non-zero created_at")
	}
}

// --- ListComments with IssueID Filter ---

func TestBoundary_ListComments_FilterByIssueID_OnlyReturnsCommentsForIssue(t *testing.T) {
	// Given
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	issueA := createIntTask(t, svc, "Issue A")
	issueB := createIntTask(t, svc, "Issue B")

	_, err := svc.AddComment(ctx, driving.AddCommentInput{
		IssueID: issueA.String(), Author: author(t, "alice"), Body: "Comment on A",
	})
	if err != nil {
		t.Fatalf("precondition: add comment to A failed: %v", err)
	}
	_, err = svc.AddComment(ctx, driving.AddCommentInput{
		IssueID: issueB.String(), Author: author(t, "alice"), Body: "Comment on B",
	})
	if err != nil {
		t.Fatalf("precondition: add comment to B failed: %v", err)
	}

	// When
	listOut, err := svc.ListComments(ctx, driving.ListCommentsInput{
		IssueID: issueA.String(),
		Limit:   -1,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(listOut.Comments) != 1 {
		t.Fatalf("comments: got %d, want 1", len(listOut.Comments))
	}
	if listOut.Comments[0].Body != "Comment on A" {
		t.Errorf("body: got %q, want %q", listOut.Comments[0].Body, "Comment on A")
	}
}

// --- ListComments with Author Filter ---

func TestBoundary_ListComments_FilterByAuthor_OnlyReturnsMatchingAuthor(t *testing.T) {
	// Given
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	issueID := createIntTask(t, svc, "Author filter task")

	_, err := svc.AddComment(ctx, driving.AddCommentInput{
		IssueID: issueID.String(), Author: author(t, "alice"), Body: "Alice's comment",
	})
	if err != nil {
		t.Fatalf("precondition: add alice comment failed: %v", err)
	}
	_, err = svc.AddComment(ctx, driving.AddCommentInput{
		IssueID: issueID.String(), Author: author(t, "bob"), Body: "Bob's comment",
	})
	if err != nil {
		t.Fatalf("precondition: add bob comment failed: %v", err)
	}

	// When
	listOut, err := svc.ListComments(ctx, driving.ListCommentsInput{
		IssueID: issueID.String(),
		Filter:  driving.CommentFilterInput{Authors: []string{"alice"}},
		Limit:   -1,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(listOut.Comments) != 1 {
		t.Fatalf("comments: got %d, want 1", len(listOut.Comments))
	}
	if listOut.Comments[0].Body != "Alice's comment" {
		t.Errorf("body: got %q, want %q", listOut.Comments[0].Body, "Alice's comment")
	}
}

// --- ListComments with CreatedAfter Filter ---

func TestBoundary_ListComments_FilterByCreatedAfter_OnlyReturnsNewerComments(t *testing.T) {
	// Given
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	issueID := createIntTask(t, svc, "CreatedAfter filter task")

	_, err := svc.AddComment(ctx, driving.AddCommentInput{
		IssueID: issueID.String(), Author: author(t, "alice"), Body: "Older comment",
	})
	if err != nil {
		t.Fatalf("precondition: add older comment failed: %v", err)
	}

	// Record a cutoff after the first comment.
	cutoff := time.Now()

	// Small delay to ensure the second comment's timestamp is strictly after the cutoff.
	time.Sleep(10 * time.Millisecond)

	_, err = svc.AddComment(ctx, driving.AddCommentInput{
		IssueID: issueID.String(), Author: author(t, "alice"), Body: "Newer comment",
	})
	if err != nil {
		t.Fatalf("precondition: add newer comment failed: %v", err)
	}

	// When
	listOut, err := svc.ListComments(ctx, driving.ListCommentsInput{
		IssueID: issueID.String(),
		Filter:  driving.CommentFilterInput{CreatedAfter: cutoff},
		Limit:   -1,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(listOut.Comments) != 1 {
		t.Fatalf("comments: got %d, want 1", len(listOut.Comments))
	}
	if listOut.Comments[0].Body != "Newer comment" {
		t.Errorf("body: got %q, want %q", listOut.Comments[0].Body, "Newer comment")
	}
}

// --- SearchComments Text Matching ---

func TestBoundary_SearchComments_MatchesBodyText(t *testing.T) {
	// Given
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	issueID := createIntTask(t, svc, "Search comments task")

	_, err := svc.AddComment(ctx, driving.AddCommentInput{
		IssueID: issueID.String(), Author: author(t, "alice"), Body: "The authentication module needs refactoring",
	})
	if err != nil {
		t.Fatalf("precondition: add matching comment failed: %v", err)
	}
	_, err = svc.AddComment(ctx, driving.AddCommentInput{
		IssueID: issueID.String(), Author: author(t, "alice"), Body: "Database schema looks fine",
	})
	if err != nil {
		t.Fatalf("precondition: add non-matching comment failed: %v", err)
	}

	// When
	searchOut, err := svc.SearchComments(ctx, driving.SearchCommentsInput{
		Query: "authentication",
		Limit: -1,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(searchOut.Comments) != 1 {
		t.Fatalf("comments: got %d, want 1", len(searchOut.Comments))
	}
	if searchOut.Comments[0].Body != "The authentication module needs refactoring" {
		t.Errorf("body: got %q, want matching comment", searchOut.Comments[0].Body)
	}
}

func TestBoundary_SearchComments_ScopedToIssue_OnlyReturnsMatchesInIssue(t *testing.T) {
	// Given
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	issueA := createIntTask(t, svc, "Search scope A")
	issueB := createIntTask(t, svc, "Search scope B")

	_, err := svc.AddComment(ctx, driving.AddCommentInput{
		IssueID: issueA.String(), Author: author(t, "alice"), Body: "Refactoring the parser logic",
	})
	if err != nil {
		t.Fatalf("precondition: add comment A failed: %v", err)
	}
	_, err = svc.AddComment(ctx, driving.AddCommentInput{
		IssueID: issueB.String(), Author: author(t, "alice"), Body: "Refactoring the serializer logic",
	})
	if err != nil {
		t.Fatalf("precondition: add comment B failed: %v", err)
	}

	// When — search scoped to issueA
	searchOut, err := svc.SearchComments(ctx, driving.SearchCommentsInput{
		Query:   "refactoring",
		IssueID: issueA.String(),
		Limit:   -1,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(searchOut.Comments) != 1 {
		t.Fatalf("comments: got %d, want 1", len(searchOut.Comments))
	}
	if searchOut.Comments[0].IssueID != issueA.String() {
		t.Errorf("issue ID: got %s, want %s", searchOut.Comments[0].IssueID, issueA)
	}
}

// --- Pagination via AfterCommentID ---

func TestBoundary_ListComments_Pagination_AfterCommentID(t *testing.T) {
	// Given
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	issueID := createIntTask(t, svc, "Pagination task")

	// Create three comments.
	for _, body := range []string{"First", "Second", "Third"} {
		_, err := svc.AddComment(ctx, driving.AddCommentInput{
			IssueID: issueID.String(), Author: author(t, "alice"), Body: body,
		})
		if err != nil {
			t.Fatalf("precondition: add comment %q failed: %v", body, err)
		}
	}

	// Fetch the first page (limit 2).
	firstPage, err := svc.ListComments(ctx, driving.ListCommentsInput{
		IssueID: issueID.String(),
		Limit:   2,
	})
	if err != nil {
		t.Fatalf("precondition: first page failed: %v", err)
	}
	if len(firstPage.Comments) != 2 {
		t.Fatalf("first page: got %d comments, want 2", len(firstPage.Comments))
	}
	if !firstPage.HasMore {
		t.Fatalf("first page: expected HasMore=true")
	}

	lastIDOnFirstPage := firstPage.Comments[len(firstPage.Comments)-1].CommentID

	// When — fetch the second page using AfterCommentID
	secondPage, err := svc.ListComments(ctx, driving.ListCommentsInput{
		IssueID: issueID.String(),
		Filter:  driving.CommentFilterInput{AfterCommentID: lastIDOnFirstPage},
		Limit:   2,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(secondPage.Comments) != 1 {
		t.Fatalf("second page: got %d comments, want 1", len(secondPage.Comments))
	}
	if secondPage.Comments[0].Body != "Third" {
		t.Errorf("second page body: got %q, want %q", secondPage.Comments[0].Body, "Third")
	}
	if secondPage.HasMore {
		t.Error("second page: expected HasMore=false")
	}
}
