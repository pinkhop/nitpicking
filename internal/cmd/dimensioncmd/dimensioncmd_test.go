package dimensioncmd_test

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
	"github.com/pinkhop/nitpicking/internal/fake"
)

func setupService(t *testing.T) service.Service {
	t.Helper()
	repo := fake.NewRepository()
	tx := fake.NewTransactor(repo)
	svc := service.New(tx)

	ctx := t.Context()
	if err := svc.Init(ctx, "NP"); err != nil {
		t.Fatalf("precondition: init failed: %v", err)
	}
	return svc
}

func mustAuthor(t *testing.T, name string) identity.Author {
	t.Helper()
	a, err := identity.NewAuthor(name)
	if err != nil {
		t.Fatalf("precondition: invalid author: %v", err)
	}
	return a
}

func TestListDistinctDimensions_ReturnsDimensionsFromNonDeletedIssues(t *testing.T) {
	t.Parallel()

	// Given: two tasks with dimensions.
	svc := setupService(t)
	ctx := t.Context()
	author := mustAuthor(t, "test-agent")

	dim1, _ := issue.NewDimension("kind", "bug")
	dim2, _ := issue.NewDimension("area", "auth")
	dim3, _ := issue.NewDimension("kind", "feature")

	_, err := svc.CreateIssue(ctx, service.CreateIssueInput{
		Role:       issue.RoleTask,
		Title:      "Bug task",
		Author:     author,
		Dimensions: []issue.Dimension{dim1, dim2},
	})
	if err != nil {
		t.Fatalf("precondition: create task 1 failed: %v", err)
	}

	_, err = svc.CreateIssue(ctx, service.CreateIssueInput{
		Role:       issue.RoleTask,
		Title:      "Feature task",
		Author:     author,
		Dimensions: []issue.Dimension{dim3},
	})
	if err != nil {
		t.Fatalf("precondition: create task 2 failed: %v", err)
	}

	// When
	dims, err := svc.ListDistinctDimensions(ctx)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dims) != 3 {
		t.Fatalf("dimension count: got %d, want 3", len(dims))
	}

	// Verify all three distinct key-value pairs are present.
	found := make(map[string]bool)
	for _, d := range dims {
		found[d.Key()+":"+d.Value()] = true
	}
	for _, expected := range []string{"kind:bug", "area:auth", "kind:feature"} {
		if !found[expected] {
			t.Errorf("missing dimension %q", expected)
		}
	}
}
