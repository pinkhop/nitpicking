//go:build boundary

package sqlite_test

import (
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/backup/jsonl"
	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/sqlite"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- Helpers ---

// boundaryEnv holds a test database and service for backup/restore tests.
type boundaryEnv struct {
	store  *sqlite.Store
	svc    driving.Service
	ctx    context.Context
	tmpDir string
}

// newBoundaryEnv creates a fresh database, initialises it with the given
// prefix, and returns the test environment.
func newBoundaryEnv(t *testing.T, prefix string) *boundaryEnv {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := sqlite.Create(dbPath)
	if err != nil {
		t.Fatalf("creating database: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	svc := core.New(store)
	ctx := t.Context()
	if err := svc.Init(ctx, prefix); err != nil {
		t.Fatalf("initializing database: %v", err)
	}

	return &boundaryEnv{store: store, svc: svc, ctx: ctx, tmpDir: tmpDir}
}

// createTask creates a task and returns its ID.
func (e *boundaryEnv) createTask(t *testing.T, title string, opts ...func(*driving.CreateIssueInput)) domain.ID {
	t.Helper()
	input := driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  title,
		Author: author(t, "seeder"),
	}
	for _, opt := range opts {
		opt(&input)
	}
	out, err := e.svc.CreateIssue(e.ctx, input)
	if err != nil {
		t.Fatalf("creating task %q: %v", title, err)
	}
	return out.Issue.ID()
}

// createEpic creates an epic and returns its ID.
func (e *boundaryEnv) createEpic(t *testing.T, title string, opts ...func(*driving.CreateIssueInput)) domain.ID {
	t.Helper()
	input := driving.CreateIssueInput{
		Role:   domain.RoleEpic,
		Title:  title,
		Author: author(t, "seeder"),
	}
	for _, opt := range opts {
		opt(&input)
	}
	out, err := e.svc.CreateIssue(e.ctx, input)
	if err != nil {
		t.Fatalf("creating epic %q: %v", title, err)
	}
	return out.Issue.ID()
}

// claimAndClose claims a task and closes it.
func (e *boundaryEnv) claimAndClose(t *testing.T, id domain.ID) {
	t.Helper()
	a := author(t, "closer")
	out, err := e.svc.ClaimByID(e.ctx, driving.ClaimInput{IssueID: id.String(), Author: a})
	if err != nil {
		t.Fatalf("claiming %s: %v", id, err)
	}
	if err := e.svc.TransitionState(e.ctx, driving.TransitionInput{
		IssueID: id.String(), ClaimID: out.ClaimID, Action: driving.ActionClose,
	}); err != nil {
		t.Fatalf("closing %s: %v", id, err)
	}
}

// claimAndDefer claims a task and defers it.
func (e *boundaryEnv) claimAndDefer(t *testing.T, id domain.ID) {
	t.Helper()
	a := author(t, "deferer")
	out, err := e.svc.ClaimByID(e.ctx, driving.ClaimInput{IssueID: id.String(), Author: a})
	if err != nil {
		t.Fatalf("claiming %s: %v", id, err)
	}
	if err := e.svc.TransitionState(e.ctx, driving.TransitionInput{
		IssueID: id.String(), ClaimID: out.ClaimID, Action: driving.ActionDefer,
	}); err != nil {
		t.Fatalf("deferring %s: %v", id, err)
	}
}

// claimAndDelete claims a task and soft-deletes it.
func (e *boundaryEnv) claimAndDelete(t *testing.T, id domain.ID) {
	t.Helper()
	a := author(t, "deleter")
	out, err := e.svc.ClaimByID(e.ctx, driving.ClaimInput{IssueID: id.String(), Author: a})
	if err != nil {
		t.Fatalf("claiming %s: %v", id, err)
	}
	if err := e.svc.DeleteIssue(e.ctx, driving.DeleteInput{
		IssueID: id.String(), ClaimID: out.ClaimID,
	}); err != nil {
		t.Fatalf("deleting %s: %v", id, err)
	}
}

// addComment adds a comment to an domain.
func (e *boundaryEnv) addComment(t *testing.T, id domain.ID, body string) {
	t.Helper()
	_, err := e.svc.AddComment(e.ctx, driving.AddCommentInput{
		IssueID: id.String(),
		Author:  author(t, "commenter"),
		Body:    body,
	})
	if err != nil {
		t.Fatalf("adding comment to %s: %v", id, err)
	}
}

// addRelationship adds a relationship from source to target.
func (e *boundaryEnv) addRelationship(t *testing.T, source, target domain.ID, relType domain.RelationType) {
	t.Helper()
	if err := e.svc.AddRelationship(e.ctx, source.String(), driving.RelationshipInput{
		Type: relType, TargetID: target.String(),
	}, author(t, "relator")); err != nil {
		t.Fatalf("adding relationship %s -> %s: %v", source, target, err)
	}
}

// doBackup performs a gzip-compressed backup to a file and returns the
// file path and backup output.
func (e *boundaryEnv) doBackup(t *testing.T) (string, driving.BackupOutput) {
	t.Helper()

	backupPath := filepath.Join(e.tmpDir, "backup.jsonl.gz")
	file, err := os.Create(backupPath)
	if err != nil {
		t.Fatalf("creating backup file: %v", err)
	}

	gzw := gzip.NewWriter(file)
	w := jsonl.NewWriter(gzw)
	out, err := e.svc.Backup(e.ctx, driving.BackupInput{Writer: w})
	if err != nil {
		_ = w.Close()
		_ = file.Close()
		t.Fatalf("backup failed: %v", err)
	}
	if err := w.Close(); err != nil {
		_ = file.Close()
		t.Fatalf("closing gzip writer: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("closing backup file: %v", err)
	}
	return backupPath, out
}

// doRestore restores from a gzip-compressed backup file.
func (e *boundaryEnv) doRestore(t *testing.T, backupPath string) {
	t.Helper()

	file, err := os.Open(backupPath)
	if err != nil {
		t.Fatalf("opening backup file: %v", err)
	}
	defer func() {
		_ = file.Close()
	}()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("creating gzip reader: %v", err)
	}
	r := jsonl.NewReader(gzr)
	defer func() {
		_ = r.Close()
	}()

	if err := e.svc.Restore(e.ctx, driving.RestoreInput{Reader: r}); err != nil {
		t.Fatalf("restore failed: %v", err)
	}
}

// --- Seeding ---

// seedResult holds the IDs of issues created by seedDatabase.
type seedResult struct {
	// Tasks in various states.
	openTask     domain.ID
	closedTask   domain.ID
	deferredTask domain.ID
	claimedTask  domain.ID
	deletedTask  domain.ID

	// Epics.
	epic         domain.ID
	childTask1   domain.ID
	childTask2   domain.ID
	subEpic      domain.ID
	subEpicChild domain.ID

	// Issues with relationships.
	blockerTask domain.ID
	blockedTask domain.ID
	citingTask  domain.ID
	citedTask   domain.ID
	refsTask1   domain.ID
	refsTask2   domain.ID

	// Issues with labels.
	labeledTask    domain.ID
	multiLabelTask domain.ID

	// Issues with comments.
	commentedTask domain.ID

	// Issue with idempotency key.
	idempotentTask domain.ID
}

// seedDatabase creates a comprehensive set of issues representing
// every conceivable state, relationship type, label configuration,
// and history depth. Deleted issues are included to verify they are
// excluded from backup.
func seedDatabase(t *testing.T, env *boundaryEnv) seedResult {
	t.Helper()

	var r seedResult

	// --- Tasks in every state ---
	r.openTask = env.createTask(t, "Open task")

	r.closedTask = env.createTask(t, "Closed task")
	env.claimAndClose(t, r.closedTask)

	r.deferredTask = env.createTask(t, "Deferred task")
	env.claimAndDefer(t, r.deferredTask)

	// Claimed task (remains claimed).
	r.claimedTask = env.createTask(t, "Claimed task", func(in *driving.CreateIssueInput) {
		in.Claim = true
	})

	// Deleted task (should NOT appear in backup).
	r.deletedTask = env.createTask(t, "Deleted task")
	env.claimAndDelete(t, r.deletedTask)

	// --- Epic hierarchy ---
	r.epic = env.createEpic(t, "Top-level epic")
	r.childTask1 = env.createTask(t, "Child task 1", func(in *driving.CreateIssueInput) {
		in.ParentID = r.epic.String()
	})
	r.childTask2 = env.createTask(t, "Child task 2", func(in *driving.CreateIssueInput) {
		in.ParentID = r.epic.String()
	})
	r.subEpic = env.createEpic(t, "Sub-epic", func(in *driving.CreateIssueInput) {
		in.ParentID = r.epic.String()
	})
	r.subEpicChild = env.createTask(t, "Sub-epic child", func(in *driving.CreateIssueInput) {
		in.ParentID = r.subEpic.String()
	})

	// --- Every relationship type ---
	r.blockerTask = env.createTask(t, "Blocker task")
	r.blockedTask = env.createTask(t, "Blocked task")
	env.addRelationship(t, r.blockedTask, r.blockerTask, domain.RelBlockedBy)

	r.citingTask = env.createTask(t, "Citing task")
	r.citedTask = env.createTask(t, "Cited task")
	env.addRelationship(t, r.citingTask, r.citedTask, domain.RelCites)

	r.refsTask1 = env.createTask(t, "Refs task 1")
	r.refsTask2 = env.createTask(t, "Refs task 2")
	env.addRelationship(t, r.refsTask1, r.refsTask2, domain.RelRefs)

	// --- Labels ---
	r.labeledTask = env.createTask(t, "Labeled task", func(in *driving.CreateIssueInput) {
		in.Labels = []driving.LabelInput{{Key: "kind", Value: "bug"}}
	})
	r.multiLabelTask = env.createTask(t, "Multi-labeled task", func(in *driving.CreateIssueInput) {
		in.Labels = []driving.LabelInput{
			{Key: "kind", Value: "feat"},
			{Key: "area", Value: "backend"},
			{Key: "priority-override", Value: "critical"},
		}
	})

	// --- Comments ---
	r.commentedTask = env.createTask(t, "Commented task")
	env.addComment(t, r.commentedTask, "First comment")
	env.addComment(t, r.commentedTask, "Second comment with <HTML> & special chars")
	env.addComment(t, r.commentedTask, "Third comment")

	// Add a comment to the closed task too.
	env.addComment(t, r.closedTask, "Post-close comment")

	// --- Idempotency key ---
	r.idempotentTask = env.createTask(t, "Idempotent task", func(in *driving.CreateIssueInput) {
		in.IdempotencyKey = "unique-key-12345"
	})

	// --- Rich history via updates ---
	// Update the open task's description and priority to generate history entries.
	claimOut, err := env.svc.ClaimByID(env.ctx, driving.ClaimInput{
		IssueID: r.openTask.String(), Author: author(t, "updater"),
	})
	if err != nil {
		t.Fatalf("claiming open task for updates: %v", err)
	}
	newTitle := "Updated open task title"
	newDesc := "A detailed description"
	newPriority := domain.P1
	if err := env.svc.UpdateIssue(env.ctx, driving.UpdateIssueInput{
		IssueID:     r.openTask.String(),
		ClaimID:     claimOut.ClaimID,
		Title:       &newTitle,
		Description: &newDesc,
		Priority:    &newPriority,
	}); err != nil {
		t.Fatalf("updating open task: %v", err)
	}
	// Release so it goes back to open.
	if err := env.svc.TransitionState(env.ctx, driving.TransitionInput{
		IssueID: r.openTask.String(), ClaimID: claimOut.ClaimID, Action: driving.ActionRelease,
	}); err != nil {
		t.Fatalf("releasing open task: %v", err)
	}

	return r
}

// --- Test: Simple backup then restore ---

func TestBoundary_BackupRestore_SimpleRoundTrip(t *testing.T) {
	// Given — a seeded database.
	env := newBoundaryEnv(t, "BR")
	seed := seedDatabase(t, env)

	// Capture expected state before backup.
	preBackupState := captureState(t, env, seed)

	// When — backup then restore to a fresh database.
	backupPath, backupOut := env.doBackup(t)

	// Verify deleted issue is not in backup.
	if backupOut.IssueCount == 0 {
		t.Fatal("backup produced zero issues")
	}

	// Restore to a fresh database.
	env2 := newBoundaryEnv(t, "IGNORED") // prefix will be overwritten by restore
	env2.doRestore(t, backupPath)

	// Then — verify all data was reconstructed.
	postRestoreState := captureState(t, env2, seed)

	compareStates(t, preBackupState, postRestoreState)

	// Verify deleted issue was NOT restored.
	_, err := env2.svc.ShowIssue(env2.ctx, seed.deletedTask.String())
	if err == nil {
		t.Error("deleted task was restored — should have been excluded from backup")
	}

	// Verify prefix was restored.
	prefix, err := env2.svc.GetPrefix(env2.ctx)
	if err != nil {
		t.Fatalf("getting prefix after restore: %v", err)
	}
	if prefix != "BR" {
		t.Errorf("prefix after restore = %q, want %q", prefix, "BR")
	}

	// Verify FTS works after restore.
	searchOut, err := env2.svc.SearchIssues(env2.ctx, driving.SearchIssuesInput{
		Query: "Updated open task",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("searching after restore: %v", err)
	}
	if len(searchOut.Items) == 0 {
		t.Error("FTS search found no results after restore — FTS rebuild may have failed")
	}
}

// --- Test: Restore over modified database ---

func TestBoundary_BackupRestore_RestoreOverModifiedDatabase(t *testing.T) {
	// Given — a seeded database.
	env := newBoundaryEnv(t, "MOD")
	seed := seedDatabase(t, env)

	// Capture state and backup.
	preBackupState := captureState(t, env, seed)
	backupPath, _ := env.doBackup(t)

	// When — modify the database after backup.

	// Add new issues.
	newTask := env.createTask(t, "Post-backup task")
	env.addComment(t, newTask, "A comment on the new task")

	// Close an open domain.
	env.claimAndClose(t, seed.childTask1)

	// Delete another domain.
	env.claimAndDelete(t, seed.childTask2)

	// Add a comment to an existing domain.
	env.addComment(t, seed.openTask, "Post-backup comment")

	// Now restore from the backup.
	env.doRestore(t, backupPath)

	// Then — state should match the moment of backup, not the modified state.
	postRestoreState := captureState(t, env, seed)
	compareStates(t, preBackupState, postRestoreState)

	// The post-backup task should not exist after restore.
	_, err := env.svc.ShowIssue(env.ctx, newTask.String())
	if err == nil {
		t.Error("post-backup task still exists after restore — database was not cleared")
	}
}

// --- Test: Empty database backup and restore ---

func TestBoundary_BackupRestore_EmptyDatabase(t *testing.T) {
	// Given — an empty database.
	env := newBoundaryEnv(t, "EMPTY")

	// When — backup then restore.
	backupPath, backupOut := env.doBackup(t)
	if backupOut.IssueCount != 0 {
		t.Errorf("backup of empty database produced %d issues, want 0", backupOut.IssueCount)
	}

	// Restore to a fresh database.
	env2 := newBoundaryEnv(t, "TEMP")
	env2.doRestore(t, backupPath)

	// Then — database should be empty with correct prefix.
	prefix, err := env2.svc.GetPrefix(env2.ctx)
	if err != nil {
		t.Fatalf("getting prefix: %v", err)
	}
	if prefix != "EMPTY" {
		t.Errorf("prefix = %q, want %q", prefix, "EMPTY")
	}

	listOut, err := env2.svc.ListIssues(env2.ctx, driving.ListIssuesInput{Limit: 10})
	if err != nil {
		t.Fatalf("listing issues: %v", err)
	}
	if len(listOut.Items) != 0 {
		t.Errorf("expected 0 issues after restoring empty backup, got %d", len(listOut.Items))
	}
}

// --- Test: Double restore ---

func TestBoundary_BackupRestore_DoubleRestore(t *testing.T) {
	// Given — a seeded database and its backup.
	env := newBoundaryEnv(t, "DBL")
	seed := seedDatabase(t, env)
	preBackupState := captureState(t, env, seed)
	backupPath, _ := env.doBackup(t)

	// When — restore twice.
	env.doRestore(t, backupPath)
	env.doRestore(t, backupPath)

	// Then — state should still match the original.
	postDoubleRestore := captureState(t, env, seed)
	compareStates(t, preBackupState, postDoubleRestore)
}

// --- Test: Comment FTS after restore ---

func TestBoundary_BackupRestore_CommentSearchAfterRestore(t *testing.T) {
	// Given — a database with comments.
	env := newBoundaryEnv(t, "CFS")
	commentedTask := env.createTask(t, "Task with searchable comment")
	env.addComment(t, commentedTask, "This comment contains uniquesearchabletermxyz")

	backupPath, _ := env.doBackup(t)

	// When — restore.
	env2 := newBoundaryEnv(t, "TMP")
	env2.doRestore(t, backupPath)

	// Then — comment search should work.
	searchOut, err := env2.svc.SearchComments(env2.ctx, driving.SearchCommentsInput{
		Query: "uniquesearchabletermxyz",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("searching comments after restore: %v", err)
	}
	if len(searchOut.Comments) == 0 {
		t.Error("comment FTS search found no results after restore")
	}
}

// --- Test: Backup preserves claim state ---

func TestBoundary_BackupRestore_ClaimedIssuePreserved(t *testing.T) {
	// Given — a database with a claimed task.
	env := newBoundaryEnv(t, "CLM")
	taskID := env.createTask(t, "Task to be claimed", func(in *driving.CreateIssueInput) {
		in.Claim = true
		in.Author = author(t, "claim-owner")
	})

	backupPath, _ := env.doBackup(t)

	// When — restore.
	env2 := newBoundaryEnv(t, "TMP")
	env2.doRestore(t, backupPath)

	// Then — issue should show as claimed.
	show, err := env2.svc.ShowIssue(env2.ctx, taskID.String())
	if err != nil {
		t.Fatalf("showing issue after restore: %v", err)
	}
	if show.ClaimAuthor != "claim-owner" {
		t.Errorf("claim author after restore = %q, want %q", show.ClaimAuthor, "claim-owner")
	}
	if show.State != domain.StateClaimed {
		t.Errorf("issue state after restore = %v, want %v", show.State, domain.StateClaimed)
	}
}

// --- State capture and comparison ---

// issueSnapshot holds all observable data for a single domain.
type issueSnapshot struct {
	id                 string
	role               string
	title              string
	description        string
	acceptanceCriteria string
	priority           string
	state              string
	parentID           string
	idempotencyKey     string
	labels             map[string]string
	commentCount       int
	commentBodies      []string
	relationshipCount  int
	historyCount       int
	claimAuthor        string
}

// databaseState is a map of issue ID to snapshot.
type databaseState map[string]issueSnapshot

// captureState reads the full state of all issues in the seed set
// from the database.
func captureState(t *testing.T, env *boundaryEnv, seed seedResult) databaseState {
	t.Helper()

	ids := []domain.ID{
		seed.openTask, seed.closedTask, seed.deferredTask, seed.claimedTask,
		seed.epic, seed.childTask1, seed.childTask2, seed.subEpic, seed.subEpicChild,
		seed.blockerTask, seed.blockedTask, seed.citingTask, seed.citedTask,
		seed.refsTask1, seed.refsTask2,
		seed.labeledTask, seed.multiLabelTask,
		seed.commentedTask, seed.idempotentTask,
	}

	state := make(databaseState)
	for _, id := range ids {
		show, err := env.svc.ShowIssue(env.ctx, id.String())
		if err != nil {
			t.Fatalf("capturing state for %s: %v", id, err)
		}
		// Capture labels.
		labels := make(map[string]string)
		for k, v := range show.Labels {
			labels[k] = v
		}

		// Capture comments.
		commentsOut, err := env.svc.ListComments(env.ctx, driving.ListCommentsInput{
			IssueID: id.String(), Limit: -1,
		})
		if err != nil {
			t.Fatalf("listing comments for %s: %v", id, err)
		}
		var bodies []string
		for _, c := range commentsOut.Comments {
			bodies = append(bodies, c.Body)
		}

		// Capture history count.
		histOut, err := env.svc.ShowHistory(env.ctx, driving.ListHistoryInput{
			IssueID: id.String(), Limit: -1,
		})
		if err != nil {
			t.Fatalf("listing history for %s: %v", id, err)
		}

		snap := issueSnapshot{
			id:                 show.ID,
			role:               show.Role.String(),
			title:              show.Title,
			description:        show.Description,
			acceptanceCriteria: show.AcceptanceCriteria,
			priority:           show.Priority.String(),
			state:              show.State.String(),
			parentID:           show.ParentID,
			labels:             labels,
			commentCount:       len(commentsOut.Comments),
			commentBodies:      bodies,
			relationshipCount:  len(show.Relationships),
			historyCount:       len(histOut.Entries),
			claimAuthor:        show.ClaimAuthor,
		}

		state[show.ID] = snap
	}
	return state
}

// compareStates checks that two database states are equivalent.
func compareStates(t *testing.T, expected, actual databaseState) {
	t.Helper()

	if len(expected) != len(actual) {
		t.Errorf("issue count: expected %d, got %d", len(expected), len(actual))
	}

	for id, exp := range expected {
		act, ok := actual[id]
		if !ok {
			t.Errorf("issue %s: missing after restore", id)
			continue
		}

		if exp.role != act.role {
			t.Errorf("issue %s role: expected %q, got %q", id, exp.role, act.role)
		}
		if exp.title != act.title {
			t.Errorf("issue %s title: expected %q, got %q", id, exp.title, act.title)
		}
		if exp.description != act.description {
			t.Errorf("issue %s description: expected %q, got %q", id, exp.description, act.description)
		}
		if exp.acceptanceCriteria != act.acceptanceCriteria {
			t.Errorf("issue %s acceptance_criteria: expected %q, got %q", id, exp.acceptanceCriteria, act.acceptanceCriteria)
		}
		if exp.priority != act.priority {
			t.Errorf("issue %s priority: expected %q, got %q", id, exp.priority, act.priority)
		}
		if exp.state != act.state {
			t.Errorf("issue %s state: expected %q, got %q", id, exp.state, act.state)
		}
		if exp.parentID != act.parentID {
			t.Errorf("issue %s parent_id: expected %q, got %q", id, exp.parentID, act.parentID)
		}
		if exp.idempotencyKey != act.idempotencyKey {
			t.Errorf("issue %s idempotency_key: expected %q, got %q", id, exp.idempotencyKey, act.idempotencyKey)
		}
		if exp.commentCount != act.commentCount {
			t.Errorf("issue %s comment count: expected %d, got %d", id, exp.commentCount, act.commentCount)
		}
		if exp.relationshipCount != act.relationshipCount {
			t.Errorf("issue %s relationship count: expected %d, got %d", id, exp.relationshipCount, act.relationshipCount)
		}
		if exp.historyCount != act.historyCount {
			t.Errorf("issue %s history count: expected %d, got %d", id, exp.historyCount, act.historyCount)
		}
		if exp.claimAuthor != act.claimAuthor {
			t.Errorf("issue %s claim author: expected %q, got %q", id, exp.claimAuthor, act.claimAuthor)
		}

		// Compare labels.
		if len(exp.labels) != len(act.labels) {
			t.Errorf("issue %s label count: expected %d, got %d", id, len(exp.labels), len(act.labels))
		}
		for k, ev := range exp.labels {
			if av, ok := act.labels[k]; !ok {
				t.Errorf("issue %s label %q: missing after restore", id, k)
			} else if ev != av {
				t.Errorf("issue %s label %q: expected %q, got %q", id, k, ev, av)
			}
		}

		// Compare comment bodies.
		if len(exp.commentBodies) != len(act.commentBodies) {
			t.Errorf("issue %s comment body count mismatch: expected %d, got %d", id, len(exp.commentBodies), len(act.commentBodies))
		} else {
			for i, eb := range exp.commentBodies {
				if i < len(act.commentBodies) && eb != act.commentBodies[i] {
					t.Errorf("issue %s comment[%d] body: expected %q, got %q", id, i, eb, act.commentBodies[i])
				}
			}
		}
	}
}
