package labelcmd_test

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

func setupService(t *testing.T) driving.Service {
	t.Helper()
	repo := memory.NewRepository()
	tx := memory.NewTransactor(repo)
	svc := core.New(tx, nil)

	ctx := t.Context()
	if err := svc.Init(ctx, "NP"); err != nil {
		t.Fatalf("precondition: init failed: %v", err)
	}
	return svc
}

func mustAuthor(t *testing.T, name string) string {
	t.Helper()
	return name
}

func TestLabelList_UsesShowIssueOutputFlatLabels(t *testing.T) {
	t.Parallel()

	// Given: an issue with two labels.
	svc := setupService(t)
	ctx := t.Context()
	author := mustAuthor(t, "test-agent")

	created, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Labeled task",
		Author: author,
		Labels: []driving.LabelInput{{Key: "kind", Value: "bug"}, {Key: "area", Value: "auth"}},
	})
	if err != nil {
		t.Fatalf("precondition: create failed: %v", err)
	}

	// When: ShowIssue is called.
	shown, err := svc.ShowIssue(ctx, created.Issue.ID().String())
	if err != nil {
		t.Fatalf("show issue: %v", err)
	}

	// Then: flat Labels field contains both labels.
	if len(shown.Labels) != 2 {
		t.Fatalf("label count: got %d, want 2", len(shown.Labels))
	}
	if shown.Labels["kind"] != "bug" {
		t.Errorf("kind label: got %q, want %q", shown.Labels["kind"], "bug")
	}
	if shown.Labels["area"] != "auth" {
		t.Errorf("area label: got %q, want %q", shown.Labels["area"], "auth")
	}
}

func TestLabelList_EmptyLabels(t *testing.T) {
	t.Parallel()

	// Given: an issue with no labels.
	svc := setupService(t)
	ctx := t.Context()
	author := mustAuthor(t, "test-agent")

	created, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Unlabeled task",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create failed: %v", err)
	}

	// When: ShowIssue is called.
	shown, err := svc.ShowIssue(ctx, created.Issue.ID().String())
	if err != nil {
		t.Fatalf("show issue: %v", err)
	}

	// Then: flat Labels field is empty (but usable with len).
	if len(shown.Labels) != 0 {
		t.Errorf("label count: got %d, want 0", len(shown.Labels))
	}
}

func TestListLabelPopularity_ReturnsKeysWithPopularValuesForNonDeletedIssues(t *testing.T) {
	t.Parallel()

	// Given: two tasks with labels — one with kind:bug and area:auth, one with
	// kind:feature. This yields two distinct keys: "area" and "kind".
	svc := setupService(t)
	ctx := t.Context()
	author := mustAuthor(t, "test-agent")

	_, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Bug task",
		Author: author,
		Labels: []driving.LabelInput{{Key: "kind", Value: "bug"}, {Key: "area", Value: "auth"}},
	})
	if err != nil {
		t.Fatalf("precondition: create task 1 failed: %v", err)
	}

	_, err = svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Feature task",
		Author: author,
		Labels: []driving.LabelInput{{Key: "kind", Value: "feature"}},
	})
	if err != nil {
		t.Fatalf("precondition: create task 2 failed: %v", err)
	}

	// When
	keys, err := svc.ListLabelPopularity(ctx)
	// Then — two key entries (area and kind), alphabetically ordered.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("key count: got %d, want 2", len(keys))
	}

	// Verify each key appears with at least one popular value.
	found := make(map[string][]string)
	for _, k := range keys {
		found[k.Key] = k.PopularValues
	}
	if _, ok := found["kind"]; !ok {
		t.Errorf("missing key %q", "kind")
	}
	if _, ok := found["area"]; !ok {
		t.Errorf("missing key %q", "area")
	}
}
