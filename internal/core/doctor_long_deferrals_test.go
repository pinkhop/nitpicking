package core

import (
	"context"
	"encoding/json"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/domain/history"
	"github.com/pinkhop/nitpicking/internal/ports/driven"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- hand-rolled fakes for long-deferrals check ---

// deferralFakeIssueRepo stores issues and serves ListIssues with States
// filtering. All other IssueRepository methods panic — the long-deferrals
// check never calls them.
type deferralFakeIssueRepo struct {
	issues []domain.Issue
}

func (r *deferralFakeIssueRepo) ListIssues(_ context.Context, filter driven.IssueFilter, _ driven.IssueOrderBy, _ driven.SortDirection, _ int) ([]driven.IssueListItem, bool, error) {
	var items []driven.IssueListItem
	for _, issue := range r.issues {
		if issue.IsDeleted() {
			continue
		}
		if len(filter.States) > 0 && !slices.Contains(filter.States, issue.State()) {
			continue
		}
		items = append(items, driven.IssueListItem{
			ID:        issue.ID(),
			Role:      issue.Role(),
			State:     issue.State(),
			Priority:  issue.Priority(),
			CreatedAt: issue.CreatedAt(),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].ID.String() < items[j].ID.String()
	})
	return items, false, nil
}

func (r *deferralFakeIssueRepo) GetIssue(_ context.Context, _ domain.ID, _ bool) (domain.Issue, error) {
	panic("deferralFakeIssueRepo: GetIssue not used by long-deferrals check")
}

func (r *deferralFakeIssueRepo) CreateIssue(_ context.Context, _ domain.Issue) error {
	panic("deferralFakeIssueRepo: CreateIssue not used by long-deferrals check")
}

func (r *deferralFakeIssueRepo) UpdateIssue(_ context.Context, _ domain.Issue) error {
	panic("deferralFakeIssueRepo: UpdateIssue not used by long-deferrals check")
}

func (r *deferralFakeIssueRepo) SearchIssues(_ context.Context, _ string, _ driven.IssueFilter, _ driven.IssueOrderBy, _ driven.SortDirection, _ int) ([]driven.IssueListItem, bool, error) {
	panic("deferralFakeIssueRepo: SearchIssues not used by long-deferrals check")
}

func (r *deferralFakeIssueRepo) GetChildStatuses(_ context.Context, _ domain.ID) ([]domain.ChildStatus, error) {
	panic("deferralFakeIssueRepo: GetChildStatuses not used by long-deferrals check")
}

func (r *deferralFakeIssueRepo) GetDescendants(_ context.Context, _ domain.ID) ([]domain.DescendantInfo, error) {
	panic("deferralFakeIssueRepo: GetDescendants not used by long-deferrals check")
}

func (r *deferralFakeIssueRepo) HasChildren(_ context.Context, _ domain.ID) (bool, error) {
	panic("deferralFakeIssueRepo: HasChildren not used by long-deferrals check")
}

func (r *deferralFakeIssueRepo) GetAncestorStatuses(_ context.Context, _ domain.ID) ([]domain.AncestorStatus, error) {
	panic("deferralFakeIssueRepo: GetAncestorStatuses not used by long-deferrals check")
}

func (r *deferralFakeIssueRepo) GetParentID(_ context.Context, _ domain.ID) (domain.ID, error) {
	panic("deferralFakeIssueRepo: GetParentID not used by long-deferrals check")
}

func (r *deferralFakeIssueRepo) IssueIDExists(_ context.Context, _ domain.ID) (bool, error) {
	panic("deferralFakeIssueRepo: IssueIDExists not used by long-deferrals check")
}

func (r *deferralFakeIssueRepo) ListLabelCounts(_ context.Context) ([]domain.LabelCount, error) {
	panic("deferralFakeIssueRepo: ListLabelCounts not used by long-deferrals check")
}

func (r *deferralFakeIssueRepo) GetIssueSummary(_ context.Context) (driven.IssueSummary, error) {
	panic("deferralFakeIssueRepo: GetIssueSummary not used by long-deferrals check")
}

func (r *deferralFakeIssueRepo) GetBlockerStatuses(_ context.Context, _ domain.ID) ([]domain.BlockerStatus, error) {
	panic("deferralFakeIssueRepo: GetBlockerStatuses not used by long-deferrals check")
}

// deferralFakeHistoryRepo stores history entries keyed by issue ID string.
// It implements the minimal HistoryRepository methods needed by the
// long-deferrals check (ListHistory, GetLatestHistory).
type deferralFakeHistoryRepo struct {
	entries map[string][]history.Entry // keyed by issue ID string, chronological order
}

func (r *deferralFakeHistoryRepo) append(issueID domain.ID, ts time.Time, et history.EventType, changes ...history.FieldChange) {
	key := issueID.String()
	if r.entries == nil {
		r.entries = make(map[string][]history.Entry)
	}
	a, err := domain.NewAuthor("agent-test")
	if err != nil {
		panic("deferralFakeHistoryRepo: NewAuthor: " + err.Error())
	}
	e := history.NewEntry(history.NewEntryParams{
		IssueID:   issueID,
		Revision:  len(r.entries[key]),
		Author:    a,
		Timestamp: ts,
		EventType: et,
		Changes:   changes,
	})
	r.entries[key] = append(r.entries[key], e)
}

func (r *deferralFakeHistoryRepo) ListHistory(_ context.Context, issueID domain.ID, _ driven.HistoryFilter, _ int) ([]history.Entry, bool, error) {
	if r.entries == nil {
		return nil, false, nil
	}
	return r.entries[issueID.String()], false, nil
}

func (r *deferralFakeHistoryRepo) GetLatestHistory(_ context.Context, issueID domain.ID) (history.Entry, error) {
	if r.entries == nil {
		return history.Entry{}, domain.ErrNotFound
	}
	entries := r.entries[issueID.String()]
	if len(entries) == 0 {
		return history.Entry{}, domain.ErrNotFound
	}
	return entries[len(entries)-1], nil
}

func (r *deferralFakeHistoryRepo) AppendHistory(_ context.Context, _ history.Entry) (int64, error) {
	panic("deferralFakeHistoryRepo: AppendHistory not used by long-deferrals check")
}

func (r *deferralFakeHistoryRepo) CountHistory(_ context.Context, _ domain.ID) (int, error) {
	panic("deferralFakeHistoryRepo: CountHistory not used by long-deferrals check")
}

// deferralFakeCommentRepo stores comments keyed by issue ID string.
type deferralFakeCommentRepo struct {
	comments map[string][]domain.Comment // keyed by issue ID string
}

func (r *deferralFakeCommentRepo) addComment(issueID domain.ID, ts time.Time) {
	key := issueID.String()
	if r.comments == nil {
		r.comments = make(map[string][]domain.Comment)
	}
	a, err := domain.NewAuthor("agent-test")
	if err != nil {
		panic("deferralFakeCommentRepo: NewAuthor: " + err.Error())
	}
	c, err := domain.NewComment(domain.NewCommentParams{
		IssueID:   issueID,
		Author:    a,
		Body:      "test comment",
		CreatedAt: ts,
	})
	if err != nil {
		panic("deferralFakeCommentRepo: NewComment: " + err.Error())
	}
	r.comments[key] = append(r.comments[key], c)
}

func (r *deferralFakeCommentRepo) ListComments(_ context.Context, issueID domain.ID, _ driven.CommentFilter, _ int) ([]domain.Comment, bool, error) {
	if r.comments == nil {
		return nil, false, nil
	}
	return r.comments[issueID.String()], false, nil
}

func (r *deferralFakeCommentRepo) CreateComment(_ context.Context, _ domain.Comment) (int64, error) {
	panic("deferralFakeCommentRepo: CreateComment not used by long-deferrals check")
}

func (r *deferralFakeCommentRepo) GetComment(_ context.Context, _ int64) (domain.Comment, error) {
	panic("deferralFakeCommentRepo: GetComment not used by long-deferrals check")
}

func (r *deferralFakeCommentRepo) SearchComments(_ context.Context, _ string, _ driven.CommentFilter, _ int) ([]domain.Comment, bool, error) {
	panic("deferralFakeCommentRepo: SearchComments not used by long-deferrals check")
}

// deferralFakeRelationshipRepo serves ListRelationships from a flat slice.
type deferralFakeRelationshipRepo struct {
	rels []domain.Relationship
}

func (r *deferralFakeRelationshipRepo) ListRelationships(_ context.Context, issueID domain.ID) ([]domain.Relationship, error) {
	var out []domain.Relationship
	for _, rel := range r.rels {
		if rel.SourceID() == issueID || rel.TargetID() == issueID {
			out = append(out, rel)
		}
	}
	return out, nil
}

func (r *deferralFakeRelationshipRepo) CreateRelationship(_ context.Context, _ domain.Relationship) (bool, error) {
	panic("deferralFakeRelationshipRepo: CreateRelationship not used by long-deferrals check")
}

func (r *deferralFakeRelationshipRepo) DeleteRelationship(_ context.Context, _, _ domain.ID, _ domain.RelationType) (bool, error) {
	panic("deferralFakeRelationshipRepo: DeleteRelationship not used by long-deferrals check")
}

func (r *deferralFakeRelationshipRepo) GetBlockerStatuses(_ context.Context, _ domain.ID) ([]domain.BlockerStatus, error) {
	panic("deferralFakeRelationshipRepo: GetBlockerStatuses not used by long-deferrals check")
}

// deferralFakeUnitOfWork composes the four repositories needed by the
// long-deferrals check.
type deferralFakeUnitOfWork struct {
	issues   *deferralFakeIssueRepo
	hist     *deferralFakeHistoryRepo
	comments *deferralFakeCommentRepo
	rels     *deferralFakeRelationshipRepo
}

func (u *deferralFakeUnitOfWork) Issues() driven.IssueRepository               { return u.issues }
func (u *deferralFakeUnitOfWork) History() driven.HistoryRepository            { return u.hist }
func (u *deferralFakeUnitOfWork) Comments() driven.CommentRepository           { return u.comments }
func (u *deferralFakeUnitOfWork) Relationships() driven.RelationshipRepository { return u.rels }
func (u *deferralFakeUnitOfWork) Database() driven.DatabaseRepository          { return nil }
func (u *deferralFakeUnitOfWork) Claims() driven.ClaimRepository               { return nil }

// deferralFakeTransactor runs fn synchronously against the single
// deferralFakeUnitOfWork.
type deferralFakeTransactor struct{ uow *deferralFakeUnitOfWork }

func (t *deferralFakeTransactor) WithTransaction(_ context.Context, fn func(driven.UnitOfWork) error) error {
	return fn(t.uow)
}

func (t *deferralFakeTransactor) WithReadTransaction(_ context.Context, fn func(driven.UnitOfWork) error) error {
	return fn(t.uow)
}
func (t *deferralFakeTransactor) Vacuum(_ context.Context) error { return nil }

// compile-time interface checks.
var (
	_ driven.IssueRepository   = (*deferralFakeIssueRepo)(nil)
	_ driven.HistoryRepository = (*deferralFakeHistoryRepo)(nil)
	_ driven.CommentRepository = (*deferralFakeCommentRepo)(nil)
	_ driven.UnitOfWork        = (*deferralFakeUnitOfWork)(nil)
	_ driven.Transactor        = (*deferralFakeTransactor)(nil)
)

// deferralScenario builds a deferralFakeUnitOfWork for a test:
//   - issues lists all issues to seed (open/closed/deferred all OK).
//   - historyFor maps an issue ID to its history entries in chronological order.
//   - commentsFor maps an issue ID to comment timestamps.
//   - relationships are seeded into the relationships repo as-is; ListRelationships
//     returns the rows where the queried issue is either source or target.
type deferralScenario struct {
	issues        []domain.Issue
	historyFor    map[string][]historyEntry
	commentsFor   map[string][]time.Time
	relationships []domain.Relationship
}

// historyEntry describes a single history record to seed into a fake.
type historyEntry struct {
	ts        time.Time
	eventType history.EventType
	// changes is optional; populate to test events that look at change records
	// (e.g., relationship events whose target is mentioned in the change).
	changes []history.FieldChange
}

func newDeferralSvc(scenario deferralScenario) *serviceImpl {
	issueRepo := &deferralFakeIssueRepo{issues: scenario.issues}

	histRepo := &deferralFakeHistoryRepo{}
	for idStr, entries := range scenario.historyFor {
		id := mustParseID(idStr)
		for _, e := range entries {
			histRepo.append(id, e.ts, e.eventType, e.changes...)
		}
	}

	commentRepo := &deferralFakeCommentRepo{}
	for idStr, tss := range scenario.commentsFor {
		id := mustParseID(idStr)
		for _, ts := range tss {
			commentRepo.addComment(id, ts)
		}
	}

	relRepo := &deferralFakeRelationshipRepo{rels: scenario.relationships}

	uow := &deferralFakeUnitOfWork{
		issues:   issueRepo,
		hist:     histRepo,
		comments: commentRepo,
		rels:     relRepo,
	}
	return &serviceImpl{tx: &deferralFakeTransactor{uow: uow}}
}

// buildDeferredIssue creates a deferred task with the given ID, with
// CreatedAt set to the provided timestamp. State is set to deferred.
func buildDeferredIssue(t *testing.T, idStr string, createdAt time.Time) domain.Issue {
	t.Helper()
	issue, err := domain.NewTask(domain.NewTaskParams{
		ID:        mustParseID(idStr),
		Title:     "Deferred task " + idStr,
		CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatalf("NewTask(%q): %v", idStr, err)
	}
	return issue.WithState(domain.StateDeferred)
}

// --- long-deferrals check tests ---

// TestRunLongDeferrals_NoDeferredIssues_Passes verifies that when no issues
// are in the deferred state, the check produces no findings.
func TestRunLongDeferrals_NoDeferredIssues_Passes(t *testing.T) {
	t.Parallel()

	// Given — only open issues.
	openIssue := buildTask(t, "NP-aaaaa", domain.StateOpen, domain.ID{})
	svc := newDeferralSvc(deferralScenario{issues: []domain.Issue{openIssue}})

	// When
	result, err := runLongDeferrals(t.Context(), svc, driving.DoctorInput{})
	// Then — no findings.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil (pass), got finding: %q", result.Summary)
	}
}

// TestRunLongDeferrals_EmptyDatabase_Passes verifies that an empty database
// (no issues at all) produces no findings.
func TestRunLongDeferrals_EmptyDatabase_Passes(t *testing.T) {
	t.Parallel()

	// Given — empty repository.
	svc := newDeferralSvc(deferralScenario{})

	// When
	result, err := runLongDeferrals(t.Context(), svc, driving.DoctorInput{})
	// Then — no findings.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil (pass), got finding: %q", result.Summary)
	}
}

// TestRunLongDeferrals_DeferredIssueUpdated8DaysAgo_ReturnsFinding verifies
// that a deferred issue whose most recent history entry is 8 days old exceeds
// the default 7-day threshold and produces a finding.
func TestRunLongDeferrals_DeferredIssueUpdated8DaysAgo_ReturnsFinding(t *testing.T) {
	t.Parallel()

	// Given — a deferred issue with last activity 8 days ago.
	eightDaysAgo := time.Now().Add(-8 * 24 * time.Hour)
	issue := buildDeferredIssue(t, "NP-aaaaa", eightDaysAgo)
	svc := newDeferralSvc(deferralScenario{
		issues: []domain.Issue{issue},
		historyFor: map[string][]historyEntry{
			"NP-aaaaa": {{ts: eightDaysAgo, eventType: history.EventStateChanged}},
		},
	})

	// When
	result, err := runLongDeferrals(t.Context(), svc, driving.DoctorInput{})
	// Then — one finding: 8 days > 7-day default threshold.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding, got nil")
	}
	if len(result.Affected) != 1 {
		t.Fatalf("affected rows: got %d, want 1", len(result.Affected))
	}
	row, ok := result.Affected[0].(driving.LongDeferralRow)
	if !ok {
		t.Fatalf("Affected[0] type: got %T, want LongDeferralRow", result.Affected[0])
	}
	if row.Issue != issue.ID().String() {
		t.Errorf("row.Issue: got %q, want %q", row.Issue, issue.ID())
	}
}

// TestRunLongDeferrals_DeferredIssueCommentAdded8DaysAgo_ReturnsFinding
// verifies that a deferred issue whose most recent COMMENT is 8 days old
// produces a finding even when the issue's own history is older — comment
// recency wins.
func TestRunLongDeferrals_DeferredIssueCommentAdded8DaysAgo_ReturnsFinding(t *testing.T) {
	t.Parallel()

	// Given — issue deferred 30 days ago (history); comment added 8 days ago.
	thirtyDaysAgo := time.Now().Add(-30 * 24 * time.Hour)
	eightDaysAgo := time.Now().Add(-8 * 24 * time.Hour)
	issue := buildDeferredIssue(t, "NP-aaaaa", thirtyDaysAgo)
	svc := newDeferralSvc(deferralScenario{
		issues: []domain.Issue{issue},
		historyFor: map[string][]historyEntry{
			"NP-aaaaa": {{ts: thirtyDaysAgo, eventType: history.EventStateChanged}},
		},
		commentsFor: map[string][]time.Time{
			"NP-aaaaa": {eightDaysAgo},
		},
	})

	// When
	result, err := runLongDeferrals(t.Context(), svc, driving.DoctorInput{})
	// Then — one finding whose LastActivityAt matches the comment timestamp.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding (comment recency 8 days ago), got nil")
	}
	if len(result.Affected) != 1 {
		t.Fatalf("affected rows: got %d, want 1", len(result.Affected))
	}
	row := result.Affected[0].(driving.LongDeferralRow)
	if !row.LastActivityAt.Equal(eightDaysAgo.UTC()) {
		t.Errorf("LastActivityAt: got %v, want %v (comment timestamp)", row.LastActivityAt, eightDaysAgo.UTC())
	}
	if !row.DeferredAt.Equal(thirtyDaysAgo.UTC()) {
		t.Errorf("DeferredAt: got %v, want %v (state-change timestamp)", row.DeferredAt, thirtyDaysAgo.UTC())
	}
}

// TestRunLongDeferrals_DeferredIssueRelationshipChanged8DaysAgo_ReturnsFinding
// verifies that a deferred issue last touched 8 days ago via a
// relationship-change history event produces a finding — relationship event
// recency wins over the deferral timestamp.
func TestRunLongDeferrals_DeferredIssueRelationshipChanged8DaysAgo_ReturnsFinding(t *testing.T) {
	t.Parallel()

	// Given — issue deferred 30 days ago; relationship event 8 days ago.
	thirtyDaysAgo := time.Now().Add(-30 * 24 * time.Hour)
	eightDaysAgo := time.Now().Add(-8 * 24 * time.Hour)
	issue := buildDeferredIssue(t, "NP-aaaaa", thirtyDaysAgo)
	svc := newDeferralSvc(deferralScenario{
		issues: []domain.Issue{issue},
		historyFor: map[string][]historyEntry{
			"NP-aaaaa": {
				{ts: thirtyDaysAgo, eventType: history.EventStateChanged},
				{ts: eightDaysAgo, eventType: history.EventRelationshipAdded},
			},
		},
	})

	// When
	result, err := runLongDeferrals(t.Context(), svc, driving.DoctorInput{})
	// Then — one finding whose LastActivityAt matches the relationship event.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding (relationship event recency 8 days ago), got nil")
	}
	if len(result.Affected) != 1 {
		t.Fatalf("affected rows: got %d, want 1", len(result.Affected))
	}
	row := result.Affected[0].(driving.LongDeferralRow)
	if !row.LastActivityAt.Equal(eightDaysAgo.UTC()) {
		t.Errorf("LastActivityAt: got %v, want %v (relationship event timestamp)", row.LastActivityAt, eightDaysAgo.UTC())
	}
}

// TestRunLongDeferrals_DeferredIssueIsTargetOfRelationship_ReturnsFinding
// verifies that a relationship event recorded on ANOTHER issue's history
// (where the deferred issue is the target) counts as activity. Relationship
// history is only ever recorded on the source side, so target-side activity
// must be discovered via the relationships table.
func TestRunLongDeferrals_DeferredIssueIsTargetOfRelationship_ReturnsFinding(t *testing.T) {
	t.Parallel()

	// Given — issue D deferred 30 days ago; issue C exists with a recently-
	// added "blocked_by:D" relationship recorded in C's history 8 days ago.
	thirtyDaysAgo := time.Now().Add(-30 * 24 * time.Hour)
	eightDaysAgo := time.Now().Add(-8 * 24 * time.Hour)
	deferredID := mustParseID("NP-ddddd")
	sourceID := mustParseID("NP-ccccc")
	deferred := buildDeferredIssue(t, "NP-ddddd", thirtyDaysAgo)
	source, err := domain.NewTask(domain.NewTaskParams{
		ID:    sourceID,
		Title: "Open task that depends on D",
	})
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}
	rel, err := domain.NewRelationship(sourceID, deferredID, domain.RelBlockedBy)
	if err != nil {
		t.Fatalf("NewRelationship: %v", err)
	}
	svc := newDeferralSvc(deferralScenario{
		issues: []domain.Issue{deferred, source},
		historyFor: map[string][]historyEntry{
			"NP-ddddd": {{ts: thirtyDaysAgo, eventType: history.EventStateChanged}},
			"NP-ccccc": {{
				ts:        eightDaysAgo,
				eventType: history.EventRelationshipAdded,
				changes:   []history.FieldChange{{Field: "relationship", After: "blocked_by:NP-ddddd"}},
			}},
		},
		relationships: []domain.Relationship{rel},
	})

	// When
	result, err := runLongDeferrals(t.Context(), svc, driving.DoctorInput{})
	// Then — D's last_activity_at is the source-side event 8 days ago.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding driven by target-side relationship activity, got nil")
	}
	if len(result.Affected) != 1 {
		t.Fatalf("affected rows: got %d, want 1", len(result.Affected))
	}
	row := result.Affected[0].(driving.LongDeferralRow)
	if row.Issue != deferredID.String() {
		t.Errorf("row.Issue: got %q, want %q", row.Issue, deferredID)
	}
	if !row.LastActivityAt.Equal(eightDaysAgo.UTC()) {
		t.Errorf("LastActivityAt: got %v, want %v (source-side relationship event)", row.LastActivityAt, eightDaysAgo.UTC())
	}
}

// TestRunLongDeferrals_TargetSideRelationshipFresh_NoFinding verifies that
// when the source-side relationship event is recent enough, the deferred
// target's threshold check passes — confirming the target-side recency
// actually feeds into last_activity_at and isn't simply dropped.
func TestRunLongDeferrals_TargetSideRelationshipFresh_NoFinding(t *testing.T) {
	t.Parallel()

	// Given — D deferred 30 days ago; source C added "blocks:D" 2 days ago.
	thirtyDaysAgo := time.Now().Add(-30 * 24 * time.Hour)
	twoDaysAgo := time.Now().Add(-2 * 24 * time.Hour)
	deferredID := mustParseID("NP-ddddd")
	sourceID := mustParseID("NP-ccccc")
	deferred := buildDeferredIssue(t, "NP-ddddd", thirtyDaysAgo)
	source, err := domain.NewTask(domain.NewTaskParams{ID: sourceID, Title: "Source"})
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}
	rel, err := domain.NewRelationship(sourceID, deferredID, domain.RelBlockedBy)
	if err != nil {
		t.Fatalf("NewRelationship: %v", err)
	}
	svc := newDeferralSvc(deferralScenario{
		issues: []domain.Issue{deferred, source},
		historyFor: map[string][]historyEntry{
			"NP-ddddd": {{ts: thirtyDaysAgo, eventType: history.EventStateChanged}},
			"NP-ccccc": {{
				ts:        twoDaysAgo,
				eventType: history.EventRelationshipAdded,
				changes:   []history.FieldChange{{Field: "relationship", After: "blocked_by:NP-ddddd"}},
			}},
		},
		relationships: []domain.Relationship{rel},
	})

	// When
	result, err := runLongDeferrals(t.Context(), svc, driving.DoctorInput{})
	// Then — last_activity_at is 2 days ago, well below the 7-day threshold.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil (target-side activity 2 days ago < 7-day threshold), got finding: %q", result.Summary)
	}
}

// TestRunLongDeferrals_DeferredIssueUpdated6DaysAgo_PassesAtDefaultThreshold
// verifies that a deferred issue last updated 6 days ago does NOT produce a
// finding at the default 7-day threshold.
func TestRunLongDeferrals_DeferredIssueUpdated6DaysAgo_PassesAtDefaultThreshold(t *testing.T) {
	t.Parallel()

	// Given — issue deferred 6 days ago.
	sixDaysAgo := time.Now().Add(-6 * 24 * time.Hour)
	issue := buildDeferredIssue(t, "NP-aaaaa", sixDaysAgo)
	svc := newDeferralSvc(deferralScenario{
		issues: []domain.Issue{issue},
		historyFor: map[string][]historyEntry{
			"NP-aaaaa": {{ts: sixDaysAgo, eventType: history.EventStateChanged}},
		},
	})

	// When — default 7-day threshold.
	result, err := runLongDeferrals(t.Context(), svc, driving.DoctorInput{})
	// Then — 6 days < 7-day threshold, no finding.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil (6 days < 7-day threshold), got finding: %q", result.Summary)
	}
}

// TestRunLongDeferrals_DeferredIssueUpdated6DaysAgo_FindingAtCustomThreshold
// verifies that the same 6-day-old issue triggers a finding when the threshold
// is reduced to 5 days via DoctorInput.LongDeferralThreshold.
func TestRunLongDeferrals_DeferredIssueUpdated6DaysAgo_FindingAtCustomThreshold(t *testing.T) {
	t.Parallel()

	// Given — issue deferred 6 days ago.
	sixDaysAgo := time.Now().Add(-6 * 24 * time.Hour)
	issue := buildDeferredIssue(t, "NP-aaaaa", sixDaysAgo)
	svc := newDeferralSvc(deferralScenario{
		issues: []domain.Issue{issue},
		historyFor: map[string][]historyEntry{
			"NP-aaaaa": {{ts: sixDaysAgo, eventType: history.EventStateChanged}},
		},
	})

	// When — custom 5-day threshold.
	result, err := runLongDeferrals(t.Context(), svc, driving.DoctorInput{
		LongDeferralThreshold: 5 * 24 * time.Hour,
	})
	// Then — 6 days > 5-day threshold, one finding.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding (6 days > 5-day threshold), got nil")
	}
	if len(result.Affected) != 1 {
		t.Fatalf("affected rows: got %d, want 1", len(result.Affected))
	}
	row, ok := result.Affected[0].(driving.LongDeferralRow)
	if !ok {
		t.Fatalf("Affected[0] type: got %T, want LongDeferralRow", result.Affected[0])
	}
	if row.Issue != issue.ID().String() {
		t.Errorf("row.Issue: got %q, want %q", row.Issue, issue.ID())
	}
}

// TestRunLongDeferrals_MultipleDeferredIssues_RowsSortedAscending verifies
// that when multiple stale deferred issues are found, rows are sorted
// ascending by issue ID for deterministic output.
func TestRunLongDeferrals_MultipleDeferredIssues_RowsSortedAscending(t *testing.T) {
	t.Parallel()

	// Given — two stale deferred issues with IDs NP-bbbbb and NP-aaaaa.
	eightDaysAgo := time.Now().Add(-8 * 24 * time.Hour)
	issueA := buildDeferredIssue(t, "NP-aaaaa", eightDaysAgo)
	issueB := buildDeferredIssue(t, "NP-bbbbb", eightDaysAgo)
	svc := newDeferralSvc(deferralScenario{
		issues: []domain.Issue{issueA, issueB},
		historyFor: map[string][]historyEntry{
			"NP-aaaaa": {{ts: eightDaysAgo, eventType: history.EventStateChanged}},
			"NP-bbbbb": {{ts: eightDaysAgo, eventType: history.EventStateChanged}},
		},
	})

	// When
	result, err := runLongDeferrals(t.Context(), svc, driving.DoctorInput{})
	// Then — two rows sorted ascending.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected findings, got nil")
	}
	if got := len(result.Affected); got != 2 {
		t.Fatalf("affected rows: got %d, want 2", got)
	}
	r0 := result.Affected[0].(driving.LongDeferralRow)
	r1 := result.Affected[1].(driving.LongDeferralRow)
	if r0.Issue != "NP-aaaaa" {
		t.Errorf("row[0].Issue: got %q, want %q (ascending order)", r0.Issue, "NP-aaaaa")
	}
	if r1.Issue != "NP-bbbbb" {
		t.Errorf("row[1].Issue: got %q, want %q (ascending order)", r1.Issue, "NP-bbbbb")
	}
}

// TestRunLongDeferrals_MixedStaleAndFresh_OnlyStaleReturned verifies that a
// fresh deferred issue (below the threshold) is not included when a stale one
// also exists.
func TestRunLongDeferrals_MixedStaleAndFresh_OnlyStaleReturned(t *testing.T) {
	t.Parallel()

	// Given — one stale (8 days) and one fresh (2 days) deferred issue.
	issueStale := buildDeferredIssue(t, "NP-aaaaa", time.Now().Add(-8*24*time.Hour))
	issueFresh := buildDeferredIssue(t, "NP-bbbbb", time.Now().Add(-2*24*time.Hour))
	svc := newDeferralSvc(deferralScenario{
		issues: []domain.Issue{issueStale, issueFresh},
		historyFor: map[string][]historyEntry{
			"NP-aaaaa": {{ts: time.Now().Add(-8 * 24 * time.Hour), eventType: history.EventStateChanged}},
			"NP-bbbbb": {{ts: time.Now().Add(-2 * 24 * time.Hour), eventType: history.EventStateChanged}},
		},
	})

	// When
	result, err := runLongDeferrals(t.Context(), svc, driving.DoctorInput{})
	// Then — only the stale issue appears.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding, got nil")
	}
	if got := len(result.Affected); got != 1 {
		t.Fatalf("affected rows: got %d, want 1", got)
	}
	row := result.Affected[0].(driving.LongDeferralRow)
	if row.Issue != "NP-aaaaa" {
		t.Errorf("row.Issue: got %q, want %q", row.Issue, "NP-aaaaa")
	}
}

// TestLongDeferralRow_JSONRoundTrip verifies the spec-mandated JSON field
// names and that both timestamps round-trip through JSON correctly.
func TestLongDeferralRow_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	// Given — a row with known timestamps.
	ts1 := time.Date(2025, 12, 1, 14, 22, 9, 0, time.UTC)
	ts2 := time.Date(2025, 12, 15, 9, 11, 0, 0, time.UTC)
	original := driving.LongDeferralRow{
		Issue:          "NP-old",
		DeferredAt:     ts1,
		LastActivityAt: ts2,
	}

	// When
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded driving.LongDeferralRow
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Then — round-trip preserves all fields.
	if decoded.Issue != original.Issue {
		t.Errorf("decoded.Issue: got %q, want %q", decoded.Issue, original.Issue)
	}
	if !decoded.DeferredAt.Equal(original.DeferredAt) {
		t.Errorf("decoded.DeferredAt: got %v, want %v", decoded.DeferredAt, original.DeferredAt)
	}
	if !decoded.LastActivityAt.Equal(original.LastActivityAt) {
		t.Errorf("decoded.LastActivityAt: got %v, want %v", decoded.LastActivityAt, original.LastActivityAt)
	}
}

// TestRunLongDeferrals_DeferredAtIgnoresNonDeferralStateChanges verifies that
// only state changes whose target is "deferred" populate deferred_at. A
// historical close (state→closed) followed by reopen and re-defer must yield
// the most recent defer event timestamp, not the close timestamp.
func TestRunLongDeferrals_DeferredAtIgnoresNonDeferralStateChanges(t *testing.T) {
	t.Parallel()

	// Given — issue history sequence: defer (30d ago), undefer (20d ago), defer (10d ago).
	// Note: we don't model EventUndeferred here because it doesn't matter;
	// the assertion is about EventStateChanged target filtering. We do include
	// a close→reopen→defer-style sequence using two EventStateChanged entries
	// (close at 25d ago, defer at 10d ago) to confirm the close one is filtered.
	now := time.Now()
	ago := func(days int) time.Time { return now.Add(-time.Duration(days) * 24 * time.Hour) }
	deferredID := mustParseID("NP-aaaaa")
	deferred := buildDeferredIssue(t, "NP-aaaaa", ago(30))

	deferredStr := domain.StateDeferred.String()
	closedStr := domain.StateClosed.String()

	svc := newDeferralSvc(deferralScenario{
		issues: []domain.Issue{deferred},
		historyFor: map[string][]historyEntry{
			"NP-aaaaa": {
				// Original defer 30 days ago.
				{
					ts:        ago(30),
					eventType: history.EventStateChanged,
					changes:   []history.FieldChange{{Field: "state", Before: "open", After: deferredStr}},
				},
				// Was closed 25 days ago — must NOT be picked up as deferred_at.
				{
					ts:        ago(25),
					eventType: history.EventStateChanged,
					changes:   []history.FieldChange{{Field: "state", Before: deferredStr, After: closedStr}},
				},
				// Re-deferred 10 days ago — this is the most recent deferral.
				{
					ts:        ago(10),
					eventType: history.EventStateChanged,
					changes:   []history.FieldChange{{Field: "state", Before: "open", After: deferredStr}},
				},
			},
		},
	})

	// When
	result, err := runLongDeferrals(t.Context(), svc, driving.DoctorInput{})
	// Then — finding fires (10d > 7d), and DeferredAt is the 10d-ago re-defer
	// (not the 25d-ago close).
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || len(result.Affected) != 1 {
		t.Fatalf("expected one finding, got %v", result)
	}
	row := result.Affected[0].(driving.LongDeferralRow)
	if row.Issue != deferredID.String() {
		t.Errorf("row.Issue: got %q, want %q", row.Issue, deferredID)
	}
	wantDeferredAt := ago(10).UTC()
	if !row.DeferredAt.Equal(wantDeferredAt) {
		t.Errorf("DeferredAt: got %v, want %v (most recent transition INTO deferred, ignoring close-to-closed event)",
			row.DeferredAt, wantDeferredAt)
	}
}

// TestRunLongDeferrals_RowTimestampsPopulated verifies that the LongDeferralRow
// contains non-zero DeferredAt and LastActivityAt timestamps.
func TestRunLongDeferrals_RowTimestampsPopulated(t *testing.T) {
	t.Parallel()

	// Given — a stale deferred issue.
	eightDaysAgo := time.Now().Add(-8 * 24 * time.Hour)
	issue := buildDeferredIssue(t, "NP-aaaaa", eightDaysAgo)
	svc := newDeferralSvc(deferralScenario{
		issues: []domain.Issue{issue},
		historyFor: map[string][]historyEntry{
			"NP-aaaaa": {{ts: eightDaysAgo, eventType: history.EventStateChanged}},
		},
	})

	// When
	result, err := runLongDeferrals(t.Context(), svc, driving.DoctorInput{})
	// Then — row timestamps are non-zero and in UTC.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || len(result.Affected) == 0 {
		t.Fatal("expected a finding with affected rows, got nil or empty")
	}
	row := result.Affected[0].(driving.LongDeferralRow)
	if row.DeferredAt.IsZero() {
		t.Error("DeferredAt is zero, expected a non-zero timestamp")
	}
	if row.LastActivityAt.IsZero() {
		t.Error("LastActivityAt is zero, expected a non-zero timestamp")
	}
	if row.DeferredAt.Location() != time.UTC {
		t.Errorf("DeferredAt not UTC: got %v", row.DeferredAt.Location())
	}
	if row.LastActivityAt.Location() != time.UTC {
		t.Errorf("LastActivityAt not UTC: got %v", row.LastActivityAt.Location())
	}
}
