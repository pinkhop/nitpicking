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

	svc := core.New(store, store)
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
	refsTask1   domain.ID
	refsTask2   domain.ID
	refsTask3   domain.ID
	refsTask4   domain.ID

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

	r.refsTask1 = env.createTask(t, "Refs task 1")
	r.refsTask2 = env.createTask(t, "Refs task 2")
	env.addRelationship(t, r.refsTask1, r.refsTask2, domain.RelRefs)

	r.refsTask3 = env.createTask(t, "Refs task 3")
	r.refsTask4 = env.createTask(t, "Refs task 4")
	env.addRelationship(t, r.refsTask3, r.refsTask4, domain.RelRefs)

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

// --- Test: Backup excludes claim data (v2 format) ---

// TestBoundary_BackupRestore_ClaimedIssueRestoredAsOpen verifies that a claimed
// issue backed up in v2 format is restored as open with no active claim. Claims
// are transient and excluded from v2 backups.
func TestBoundary_BackupRestore_ClaimedIssueRestoredAsOpen(t *testing.T) {
	// Given — a database with a claimed task.
	env := newBoundaryEnv(t, "CLM")
	taskID := env.createTask(t, "Task to be claimed", func(in *driving.CreateIssueInput) {
		in.Claim = true
		in.Author = author(t, "claim-owner")
	})

	backupPath, _ := env.doBackup(t)

	// When — restore to a fresh database.
	env2 := newBoundaryEnv(t, "TMP")
	env2.doRestore(t, backupPath)

	// Then — issue should be open with no active claim (v2 backups exclude
	// claim data; claims are transient and not carried across restore).
	show, err := env2.svc.ShowIssue(env2.ctx, taskID.String())
	if err != nil {
		t.Fatalf("showing issue after restore: %v", err)
	}
	if show.State != domain.StateOpen {
		t.Errorf("issue state after restore = %v, want open", show.State)
	}
	if show.ClaimAuthor != "" {
		t.Errorf("claim author after restore = %q, want empty (claims are excluded from v2 backups)", show.ClaimAuthor)
	}
}

// TestBoundary_BackupRestore_V1BackupClaimedRestoredAsOpen verifies that a v1
// backup containing an issue with state="claimed" is restored as open in the
// v2 database. This covers the migration path: v1 backup → v2 database.
func TestBoundary_BackupRestore_V1BackupClaimedRestoredAsOpen(t *testing.T) {
	// Given — a hand-crafted v1 backup containing a claimed-state issue.
	// The prefix uses only uppercase ASCII letters to satisfy validation.
	const v1Backup = `{"prefix":"OLD","timestamp":"2026-01-01T00:00:00Z","version":1}
{"issue_id":"OLD-aaaaa","role":"task","title":"Claimed task","state":"claimed","priority":"P2","created_at":"2026-01-01T00:00:00Z","labels":[],"comments":[],"relationships":[],"claims":[{"claim_sha512":"deadbeef","author":"claimant","stale_threshold":7200000000000,"last_activity":"2026-01-01T00:00:00Z"}],"history":[]}
`
	env := newBoundaryEnv(t, "TEMP")

	// Write the backup to a gzip-compressed file.
	backupPath := writeGZIPBackup(t, env.tmpDir, v1Backup)

	// When — restore the v1 backup.
	env.doRestore(t, backupPath)

	// Then — the claimed issue should be open with no claim row.
	show, err := env.svc.ShowIssue(env.ctx, "OLD-aaaaa")
	if err != nil {
		t.Fatalf("showing issue after v1 restore: %v", err)
	}
	if show.State != domain.StateOpen {
		t.Errorf("issue state after v1 restore = %v, want open", show.State)
	}
	if show.ClaimAuthor != "" {
		t.Errorf("claim author after v1 restore = %q, want empty", show.ClaimAuthor)
	}
	if show.Title != "Claimed task" {
		t.Errorf("title after v1 restore = %q, want %q", show.Title, "Claimed task")
	}
}

// TestBoundary_BackupRestore_V1BackupClaimedReleasedHistoryFiltered verifies
// that restoring a v1 backup drops history entries with event_type="claimed"
// or event_type="released". Those event types were removed from the EventType
// enum in v2; without filtering, ParseEventType returns EventType(0) and
// np issue history renders garbled "EventType(0)" output.
//
// The test uses a hand-crafted v1 backup with three history entries:
// one "created" (must survive), one "claimed" (must be dropped), and one
// "released" (must be dropped). After restore, only the "created" entry
// should appear in the issue history.
func TestBoundary_BackupRestore_V1BackupClaimedReleasedHistoryFiltered(t *testing.T) {
	// Given — a hand-crafted v1 backup containing history entries with
	// event_type="claimed" and event_type="released" alongside a valid
	// "created" entry. The prefix uses only uppercase ASCII letters.
	const v1Backup = `{"prefix":"HIS","timestamp":"2026-01-01T00:00:00Z","version":1}
{"issue_id":"HIS-aaaaa","role":"task","title":"History test task","state":"open","priority":"P2","created_at":"2026-01-01T00:00:00Z","labels":[],"comments":[],"relationships":[],"claims":[],"history":[{"entry_id":1,"revision":0,"author":"seeder","timestamp":"2026-01-01T00:00:00Z","event_type":"created","changes":[]},{"entry_id":2,"revision":1,"author":"claimer","timestamp":"2026-01-01T01:00:00Z","event_type":"claimed","changes":[]},{"entry_id":3,"revision":2,"author":"claimer","timestamp":"2026-01-01T02:00:00Z","event_type":"released","changes":[]}]}
`
	env := newBoundaryEnv(t, "TEMP")

	backupPath := writeGZIPBackup(t, env.tmpDir, v1Backup)

	// When — restore the v1 backup into a fresh database.
	env.doRestore(t, backupPath)

	// Then — history must contain only the "created" entry; the "claimed" and
	// "released" entries must be absent so that np issue history renders cleanly.
	histOut, err := env.svc.ShowHistory(env.ctx, driving.ListHistoryInput{
		IssueID: "HIS-aaaaa",
		Limit:   -1,
	})
	if err != nil {
		t.Fatalf("ShowHistory after v1 restore: %v", err)
	}

	if len(histOut.Entries) != 1 {
		t.Errorf("history entry count = %d, want 1 (only the 'created' entry)", len(histOut.Entries))
	}

	for _, e := range histOut.Entries {
		if e.EventType == "claimed" || e.EventType == "released" {
			t.Errorf("history entry with event_type=%q survived restore; it should have been filtered", e.EventType)
		}
		if e.EventType == "EventType(0)" {
			t.Errorf("history entry has garbled event_type %q; ParseEventType returned zero value", e.EventType)
		}
	}

	if len(histOut.Entries) > 0 && histOut.Entries[0].EventType != "created" {
		t.Errorf("first history event = %q, want %q", histOut.Entries[0].EventType, "created")
	}
}

// TestBoundary_BackupRestore_LegacyCitesRelTypeTranslatedToRefs verifies that
// restoring a backup produced by v0.2.0 — which may contain relationship rows
// with rel_type="cites" or rel_type="cited_by" — succeeds without CHECK
// constraint errors and that the resulting relationships are semantically
// correct:
//
//   - "cites" rows are translated to "refs" (same semantics, renamed).
//   - "cited_by" rows are dropped (redundant because "refs" is
//     symmetric-by-inverse; storing the inverse direction would create a
//     duplicate that the v0.3.0 backup code would never produce).
func TestBoundary_BackupRestore_LegacyCitesRelTypeTranslatedToRefs(t *testing.T) {
	// Given — a hand-crafted v2 backup that contains two issues where taskA
	// has a "cites" relationship to taskB and taskB has a "cited_by"
	// relationship back to taskA (exactly the pattern a v0.2.0 database
	// would produce). The prefix uses only uppercase ASCII letters.
	//
	// Note: the backup format is v2 here because we are simulating the content
	// a v0.2.0 database would have written — the version field reflects the
	// backup-algorithm version, not the np binary version, and both v0.2.0 and
	// v0.3.0 write algorithm version 2.
	const legacyBackup = `{"prefix":"CIT","timestamp":"2026-01-01T00:00:00Z","version":2}
{"issue_id":"CIT-aaaaa","role":"task","title":"Task A (citer)","state":"open","priority":"P2","created_at":"2026-01-01T00:00:00Z","labels":[],"comments":[],"relationships":[{"target_id":"CIT-bbbbb","rel_type":"cites"}],"claims":[],"history":[]}
{"issue_id":"CIT-bbbbb","role":"task","title":"Task B (cited)","state":"open","priority":"P2","created_at":"2026-01-01T00:00:00Z","labels":[],"comments":[],"relationships":[{"target_id":"CIT-aaaaa","rel_type":"cited_by"}],"claims":[],"history":[]}
`
	env := newBoundaryEnv(t, "TEMP")
	backupPath := writeGZIPBackup(t, env.tmpDir, legacyBackup)

	// When — restore the legacy backup into a v0.3.0 database.
	env.doRestore(t, backupPath)

	// Then — both issues exist.
	showA, err := env.svc.ShowIssue(env.ctx, "CIT-aaaaa")
	if err != nil {
		t.Fatalf("ShowIssue CIT-aaaaa after restore: %v", err)
	}
	showB, err := env.svc.ShowIssue(env.ctx, "CIT-bbbbb")
	if err != nil {
		t.Fatalf("ShowIssue CIT-bbbbb after restore: %v", err)
	}

	// Task A's "cites" row must have been translated to "refs". Because "refs"
	// is symmetric, ShowIssue surfaces it from both sides. RelationshipDTO.Type
	// is the canonical string name of the relationship type.
	var aHasRefs bool
	for _, rel := range showA.Relationships {
		if rel.Type == "refs" && (rel.TargetID == "CIT-bbbbb" || rel.SourceID == "CIT-bbbbb") {
			aHasRefs = true
		}
		if rel.Type == "cites" {
			t.Errorf("CIT-aaaaa still has a 'cites' relationship — it should have been translated to 'refs'")
		}
	}
	if !aHasRefs {
		t.Errorf("CIT-aaaaa should have a 'refs' relationship to CIT-bbbbb after translating 'cites'; got relationships: %+v", showA.Relationships)
	}

	// The "cited_by" row from task B must have been dropped. Because the
	// symmetric "refs" stored from task A's side is visible to both issues,
	// task B also sees exactly one relationship (the translated refs).
	for _, rel := range showB.Relationships {
		// cited_by must not appear — it was dropped during restore.
		if rel.Type == "cited_by" {
			t.Errorf("CIT-bbbbb still has a 'cited_by' relationship — it should have been dropped")
		}
	}

	// Exactly one refs relationship should be visible from each side (the
	// translated cites row). The dropped cited_by must not create a second entry.
	if len(showA.Relationships) != 1 {
		t.Errorf("CIT-aaaaa relationship count = %d, want 1", len(showA.Relationships))
	}
	if len(showB.Relationships) != 1 {
		t.Errorf("CIT-bbbbb relationship count = %d, want 1", len(showB.Relationships))
	}
}

// writeGZIPBackup writes raw JSONL content to a gzip-compressed file and
// returns the file path.
func writeGZIPBackup(t *testing.T, tmpDir, content string) string {
	t.Helper()

	backupPath := filepath.Join(tmpDir, "v1backup.jsonl.gz")
	f, err := os.Create(backupPath)
	if err != nil {
		t.Fatalf("creating v1 backup file: %v", err)
	}
	defer func() {
		_ = f.Close()
	}()

	gzw := gzip.NewWriter(f)
	if _, err := gzw.Write([]byte(content)); err != nil {
		t.Fatalf("writing v1 backup content: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("closing gzip writer: %v", err)
	}

	return backupPath
}

// --- State capture and comparison ---

// issueSnapshot holds all observable data for a single issue that is
// expected to survive a backup/restore cycle. Claims are intentionally
// excluded — they are transient and not preserved in v2 backups.
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
		seed.blockerTask, seed.blockedTask,
		seed.refsTask1, seed.refsTask2, seed.refsTask3, seed.refsTask4,
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
