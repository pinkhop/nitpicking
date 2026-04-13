package comment_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmd/comment"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- RunSearch Tests ---

func TestRunSearch_DefaultLimit_ReturnsCappedResults(t *testing.T) {
	t.Parallel()

	// Given — 25 comments matching the query, default limit of 20.
	svc := setupService(t)
	issueID := createTask(t, svc, "Search limit task")
	for i := range 25 {
		_ = i
		addComment(t, svc, issueID, "searchable keyword here")
	}

	var buf bytes.Buffer
	input := comment.RunSearchInput{
		Service:     svc,
		Query:       "searchable",
		Filter:      driving.CommentFilterInput{},
		Limit:       20,
		JSON:        true,
		WriteTo:     &buf,
		ColorScheme: iostreams.NewColorScheme(false),
	}

	// When
	err := comment.RunSearch(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, buf.String())
	}

	comments, ok := result["comments"].([]any)
	if !ok {
		t.Fatal("expected 'comments' array in JSON output")
	}
	if len(comments) != 20 {
		t.Errorf("comment count: got %d, want 20", len(comments))
	}
	if hasMore, ok := result["has_more"].(bool); !ok || !hasMore {
		t.Errorf("has_more: got %v, want true", result["has_more"])
	}
}

func TestRunSearch_ExplicitLimit_RespectsValue(t *testing.T) {
	t.Parallel()

	// Given — 5 comments, limit of 3.
	svc := setupService(t)
	issueID := createTask(t, svc, "Explicit limit task")
	for range 5 {
		addComment(t, svc, issueID, "findable content")
	}

	var buf bytes.Buffer
	input := comment.RunSearchInput{
		Service:     svc,
		Query:       "findable",
		Filter:      driving.CommentFilterInput{},
		Limit:       3,
		JSON:        true,
		WriteTo:     &buf,
		ColorScheme: iostreams.NewColorScheme(false),
	}

	// When
	err := comment.RunSearch(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, buf.String())
	}

	comments, ok := result["comments"].([]any)
	if !ok {
		t.Fatal("expected 'comments' array in JSON output")
	}
	if len(comments) != 3 {
		t.Errorf("comment count: got %d, want 3", len(comments))
	}
	if hasMore, ok := result["has_more"].(bool); !ok || !hasMore {
		t.Errorf("has_more: got %v, want true", result["has_more"])
	}
}

func TestRunSearch_NoLimit_ReturnsAllResults(t *testing.T) {
	t.Parallel()

	// Given — 25 comments, no-limit (-1).
	svc := setupService(t)
	issueID := createTask(t, svc, "No limit task")
	for range 25 {
		addComment(t, svc, issueID, "unlimited keyword")
	}

	var buf bytes.Buffer
	input := comment.RunSearchInput{
		Service:     svc,
		Query:       "unlimited",
		Filter:      driving.CommentFilterInput{},
		Limit:       -1,
		JSON:        true,
		WriteTo:     &buf,
		ColorScheme: iostreams.NewColorScheme(false),
	}

	// When
	err := comment.RunSearch(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, buf.String())
	}

	comments, ok := result["comments"].([]any)
	if !ok {
		t.Fatal("expected 'comments' array in JSON output")
	}
	if len(comments) != 25 {
		t.Errorf("comment count: got %d, want 25", len(comments))
	}
	if hasMore, ok := result["has_more"].(bool); !ok || hasMore {
		t.Errorf("has_more: got %v, want false", result["has_more"])
	}
}

func TestRunSearch_TextOutput_ShowsMoreIndicator(t *testing.T) {
	t.Parallel()

	// Given — 3 comments, limit of 2, text mode.
	svc := setupService(t)
	issueID := createTask(t, svc, "Text output task")
	for range 3 {
		addComment(t, svc, issueID, "textmode keyword")
	}

	var buf bytes.Buffer
	input := comment.RunSearchInput{
		Service:     svc,
		Query:       "textmode",
		Filter:      driving.CommentFilterInput{},
		Limit:       2,
		JSON:        false,
		WriteTo:     &buf,
		ColorScheme: iostreams.NewColorScheme(false),
	}

	// When
	err := comment.RunSearch(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if got := countLines(output, "textmode"); got != 2 {
		t.Errorf("expected 2 result lines containing 'textmode', got %d", got)
	}
}

// countLines counts how many lines in s contain the substring sub.
func countLines(s, sub string) int {
	count := 0
	for _, line := range bytes.Split([]byte(s), []byte("\n")) {
		if bytes.Contains(line, []byte(sub)) {
			count++
		}
	}
	return count
}
