package memory

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/domain/history"
	"github.com/pinkhop/nitpicking/internal/ports/driven"
)

// Repository is an in-memory implementation of all persistence port interfaces.
// It is safe for concurrent use.
type Repository struct {
	mu sync.RWMutex

	prefix   string
	issues   map[string]domain.Issue  // keyed by issue ID string
	comments map[int64]domain.Comment // keyed by comment ID
	claims   map[string]domain.Claim  // keyed by claim ID
	// claimsByIssue maps issue ID string → claim ID for active claims.
	claimsByIssue map[string]string
	relationships []domain.Relationship
	histories     map[string][]history.Entry // keyed by issue ID string
	nextNoteID    int64
	nextHistoryID int64
}

// NewRepository creates an empty in-memory repository.
func NewRepository() *Repository {
	return &Repository{
		issues:        make(map[string]domain.Issue),
		comments:      make(map[int64]domain.Comment),
		claims:        make(map[string]domain.Claim),
		claimsByIssue: make(map[string]string),
		histories:     make(map[string][]history.Entry),
		nextNoteID:    1,
		nextHistoryID: 1,
	}
}

// --- IssueRepository ---

func (r *Repository) CreateIssue(_ context.Context, t domain.Issue) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := t.ID().String()
	if _, exists := r.issues[key]; exists {
		return fmt.Errorf("issue %s already exists", key)
	}
	r.issues[key] = t
	return nil
}

func (r *Repository) GetIssue(_ context.Context, id domain.ID, includeDeleted bool) (domain.Issue, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.issues[id.String()]
	if !ok {
		return domain.Issue{}, domain.ErrNotFound
	}
	if t.IsDeleted() && !includeDeleted {
		return domain.Issue{}, domain.ErrNotFound
	}
	return t, nil
}

func (r *Repository) UpdateIssue(_ context.Context, t domain.Issue) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := t.ID().String()
	if _, exists := r.issues[key]; !exists {
		return domain.ErrNotFound
	}
	r.issues[key] = t
	return nil
}

func (r *Repository) ListIssues(_ context.Context, filter driven.IssueFilter, orderBy driven.IssueOrderBy, limit int) ([]driven.IssueListItem, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	limit = driven.NormalizeLimit(limit)

	var items []driven.IssueListItem
	for _, t := range r.issues {
		if !filter.IncludeDeleted && t.IsDeleted() {
			continue
		}
		if !r.matchesFilter(t, filter) {
			continue
		}
		items = append(items, r.issueToListItem(t))
	}

	r.sortIssueItems(items, orderBy)

	// Apply limit and detect hasMore.
	hasMore := false
	if limit > 0 && len(items) > limit {
		hasMore = true
		items = items[:limit]
	}

	return items, hasMore, nil
}

func (r *Repository) SearchIssues(_ context.Context, query string, filter driven.IssueFilter, orderBy driven.IssueOrderBy, limit int) ([]driven.IssueListItem, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	limit = driven.NormalizeLimit(limit)
	queryLower := strings.ToLower(query)

	var items []driven.IssueListItem
	for _, t := range r.issues {
		if !filter.IncludeDeleted && t.IsDeleted() {
			continue
		}
		if !r.matchesFilter(t, filter) {
			continue
		}
		if !r.matchesSearch(t, queryLower) {
			continue
		}
		items = append(items, r.issueToListItem(t))
	}

	r.sortIssueItems(items, orderBy)

	// Apply limit and detect hasMore.
	hasMore := false
	if limit > 0 && len(items) > limit {
		hasMore = true
		items = items[:limit]
	}

	return items, hasMore, nil
}

func (r *Repository) GetChildStatuses(_ context.Context, epicID domain.ID) ([]domain.ChildStatus, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var children []domain.ChildStatus
	for _, t := range r.issues {
		if t.IsDeleted() {
			continue
		}
		if t.ParentID() == epicID {
			blocked := r.isBlocked(t.ID())
			children = append(children, domain.ChildStatus{
				State:     t.State(),
				IsBlocked: blocked,
			})
		}
	}
	return children, nil
}

// isBlocked returns true when the issue has at least one unresolved
// blocked_by relationship (target is not closed and not deleted).
// Caller must hold r.mu.
func (r *Repository) isBlocked(id domain.ID) bool {
	for _, rel := range r.relationships {
		if rel.SourceID() == id && rel.Type() == domain.RelBlockedBy {
			for _, t := range r.issues {
				if t.ID() == rel.TargetID() && !t.IsDeleted() && t.State() != domain.StateClosed {
					return true
				}
			}
		}
	}
	return false
}

func (r *Repository) GetDescendants(_ context.Context, epicID domain.ID) ([]domain.DescendantInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.getDescendantsInternal(epicID), nil
}

func (r *Repository) HasChildren(_ context.Context, epicID domain.ID) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, t := range r.issues {
		if !t.IsDeleted() && t.ParentID() == epicID {
			return true, nil
		}
	}
	return false, nil
}

func (r *Repository) GetAncestorStatuses(_ context.Context, id domain.ID) ([]domain.AncestorStatus, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var ancestors []domain.AncestorStatus
	current := id
	visited := make(map[string]bool)

	for {
		t, ok := r.issues[current.String()]
		if !ok {
			break
		}
		parentID := t.ParentID()
		if parentID.IsZero() {
			break
		}
		if visited[parentID.String()] {
			break
		}
		visited[parentID.String()] = true

		parent, ok := r.issues[parentID.String()]
		if !ok || parent.IsDeleted() {
			break
		}
		ancestors = append(ancestors, domain.AncestorStatus{
			ID:        parent.ID(),
			State:     parent.State(),
			IsBlocked: r.isIssueBlocked(parent),
		})
		current = parentID
	}

	return ancestors, nil
}

func (r *Repository) GetParentID(_ context.Context, id domain.ID) (domain.ID, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.issues[id.String()]
	if !ok {
		return domain.ID{}, domain.ErrNotFound
	}
	return t.ParentID(), nil
}

func (r *Repository) IssueIDExists(_ context.Context, id domain.ID) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.issues[id.String()]
	return exists, nil
}

func (r *Repository) ListDistinctLabels(_ context.Context) ([]domain.Label, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := make(map[string]bool)
	var lbls []domain.Label
	for _, t := range r.issues {
		if t.IsDeleted() {
			continue
		}
		for k, v := range t.Labels().All() {
			key := k + ":" + v
			if !seen[key] {
				seen[key] = true
				lbl, _ := domain.NewLabel(k, v)
				lbls = append(lbls, lbl)
			}
		}
	}
	return lbls, nil
}

func (r *Repository) GetIssueByIdempotencyKey(_ context.Context, key string) (domain.Issue, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, t := range r.issues {
		if t.IdempotencyKey() == key && key != "" {
			return t, nil
		}
	}
	return domain.Issue{}, domain.ErrNotFound
}

func (r *Repository) GetIssueSummary(_ context.Context) (driven.IssueSummary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var s driven.IssueSummary
	for _, t := range r.issues {
		if t.IsDeleted() {
			continue
		}
		// Claims are transient local bookkeeping; a claimed issue remains open
		// as its primary state.
		switch t.State() {
		case domain.StateOpen:
			s.Open++
		case domain.StateDeferred:
			s.Deferred++
		case domain.StateClosed:
			s.Closed++
		}
		if r.isIssueReady(t) {
			s.Ready++
		}
		// Exclude closed issues from the blocked count — a closed issue's
		// blocker relationships are no longer actionable.
		if t.State() != domain.StateClosed && r.isIssueBlocked(t) {
			s.Blocked++
		}
	}
	return s, nil
}

// --- CommentRepository ---

func (r *Repository) CreateComment(_ context.Context, n domain.Comment) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := r.nextNoteID
	r.nextNoteID++

	// Reconstruct comment with the assigned ID.
	created, err := domain.NewComment(domain.NewCommentParams{
		ID:        id,
		IssueID:   n.IssueID(),
		Author:    n.Author(),
		CreatedAt: n.CreatedAt(),
		Body:      n.Body(),
	})
	if err != nil {
		return 0, err
	}

	r.comments[id] = created
	return id, nil
}

func (r *Repository) GetComment(_ context.Context, id int64) (domain.Comment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	n, ok := r.comments[id]
	if !ok {
		return domain.Comment{}, domain.ErrNotFound
	}
	return n, nil
}

func (r *Repository) ListComments(_ context.Context, issueID domain.ID, filter driven.CommentFilter, limit int) ([]domain.Comment, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	limit = driven.NormalizeLimit(limit)

	var comments []domain.Comment
	for _, n := range r.comments {
		if n.IssueID() != issueID {
			continue
		}
		if !r.matchesCommentFilter(n, filter) {
			continue
		}
		comments = append(comments, n)
	}

	slices.SortFunc(comments, func(a, b domain.Comment) int {
		return cmp.Compare(a.ID(), b.ID())
	})

	// Apply limit and detect hasMore.
	hasMore := false
	if limit > 0 && len(comments) > limit {
		hasMore = true
		comments = comments[:limit]
	}

	return comments, hasMore, nil
}

func (r *Repository) SearchComments(_ context.Context, query string, filter driven.CommentFilter, limit int) ([]domain.Comment, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	limit = driven.NormalizeLimit(limit)
	queryLower := strings.ToLower(query)

	// Build the set of issue IDs in scope. When no scope filters are
	// set, all issues are in scope (nil means "unscoped").
	scopeIDs := r.commentScopeIssueIDs(filter)

	var comments []domain.Comment
	for _, n := range r.comments {
		if scopeIDs != nil {
			if _, ok := scopeIDs[n.IssueID().String()]; !ok {
				continue
			}
		}
		if !r.matchesCommentFilter(n, filter) {
			continue
		}
		if !strings.Contains(strings.ToLower(n.Body()), queryLower) {
			continue
		}
		comments = append(comments, n)
	}

	slices.SortFunc(comments, func(a, b domain.Comment) int {
		return cmp.Compare(a.ID(), b.ID())
	})

	// Apply limit and detect hasMore.
	hasMore := false
	if limit > 0 && len(comments) > limit {
		hasMore = true
		comments = comments[:limit]
	}

	return comments, hasMore, nil
}

// --- ClaimRepository ---

func (r *Repository) CreateClaim(_ context.Context, c domain.Claim) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.claims[c.ID()] = c
	r.claimsByIssue[c.IssueID().String()] = c.ID()
	return nil
}

func (r *Repository) GetClaimByIssue(_ context.Context, issueID domain.ID) (domain.Claim, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	claimID, ok := r.claimsByIssue[issueID.String()]
	if !ok {
		return domain.Claim{}, domain.ErrNotFound
	}
	c, ok := r.claims[claimID]
	if !ok {
		return domain.Claim{}, domain.ErrNotFound
	}
	return c, nil
}

// resolveClaimID maps a claim identifier to the hash key used in the claims
// map. If the identifier is already a hash key (from a reconstructed claim),
// it is returned as-is. Otherwise it is treated as a plaintext token and
// hashed. Must be called with r.mu held.
func (r *Repository) resolveClaimID(claimID string) string {
	if _, ok := r.claims[claimID]; ok {
		return claimID
	}
	return domain.HashClaimID(claimID)
}

func (r *Repository) GetClaimByID(_ context.Context, claimID string) (domain.Claim, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Hash the plaintext claim ID to match the stored SHA-512 hash.
	hashID := domain.HashClaimID(claimID)
	c, ok := r.claims[hashID]
	if !ok {
		return domain.Claim{}, domain.ErrNotFound
	}
	return c, nil
}

func (r *Repository) InvalidateClaim(_ context.Context, claimID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// claimID may be a plaintext token or a hash — resolve to hash for lookup.
	hashID := r.resolveClaimID(claimID)
	c, ok := r.claims[hashID]
	if !ok {
		return domain.ErrNotFound
	}
	delete(r.claimsByIssue, c.IssueID().String())
	delete(r.claims, hashID)
	return nil
}

func (r *Repository) UpdateClaimStaleAt(_ context.Context, claimID string, staleAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	hashID := r.resolveClaimID(claimID)
	c, ok := r.claims[hashID]
	if !ok {
		return domain.ErrNotFound
	}
	r.claims[hashID] = c.WithStaleAt(staleAt)
	return nil
}

func (r *Repository) ListStaleClaims(_ context.Context, now time.Time) ([]domain.Claim, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var stale []domain.Claim
	for _, c := range r.claims {
		if c.IsStale(now) {
			stale = append(stale, c)
		}
	}
	return stale, nil
}

func (r *Repository) ListActiveClaims(_ context.Context, now time.Time) ([]domain.Claim, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var active []domain.Claim
	for _, c := range r.claims {
		if !c.IsStale(now) {
			active = append(active, c)
		}
	}
	return active, nil
}

// --- RelationshipRepository ---

func (r *Repository) CreateRelationship(_ context.Context, rel domain.Relationship) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, existing := range r.relationships {
		if existing.Type() != rel.Type() {
			continue
		}
		// Exact match.
		if existing.SourceID() == rel.SourceID() && existing.TargetID() == rel.TargetID() {
			return false, nil
		}
		// For symmetric types, the reverse direction also counts as a match.
		if rel.Type().IsSymmetric() &&
			existing.SourceID() == rel.TargetID() && existing.TargetID() == rel.SourceID() {
			return false, nil
		}
	}
	r.relationships = append(r.relationships, rel)
	return true, nil
}

func (r *Repository) DeleteRelationship(_ context.Context, sourceID, targetID domain.ID, relType domain.RelationType) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, existing := range r.relationships {
		if existing.Type() != relType {
			continue
		}
		// Exact match.
		if existing.SourceID() == sourceID && existing.TargetID() == targetID {
			r.relationships = slices.Delete(r.relationships, i, i+1)
			return true, nil
		}
		// For symmetric types, the reverse direction also matches.
		if relType.IsSymmetric() &&
			existing.SourceID() == targetID && existing.TargetID() == sourceID {
			r.relationships = slices.Delete(r.relationships, i, i+1)
			return true, nil
		}
	}
	return false, nil // Did not exist — idempotent.
}

func (r *Repository) ListRelationships(_ context.Context, issueID domain.ID) ([]domain.Relationship, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var rels []domain.Relationship
	for _, rel := range r.relationships {
		if rel.SourceID() == issueID {
			rels = append(rels, rel)
		} else if rel.TargetID() == issueID {
			if rel.Type().IsSymmetric() {
				// Present the symmetric relationship from this issue's perspective.
				swapped, _ := domain.NewRelationship(issueID, rel.SourceID(), rel.Type())
				rels = append(rels, swapped)
			} else {
				rels = append(rels, rel)
			}
		}
	}
	return rels, nil
}

func (r *Repository) GetBlockerStatuses(_ context.Context, issueID domain.ID) ([]domain.BlockerStatus, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.getBlockerStatusesInternal(issueID), nil
}

// --- HistoryRepository ---

func (r *Repository) AppendHistory(_ context.Context, entry history.Entry) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := r.nextHistoryID
	r.nextHistoryID++

	key := entry.IssueID().String()
	recorded := history.NewEntry(history.NewEntryParams{
		ID:        id,
		IssueID:   entry.IssueID(),
		Revision:  entry.Revision(),
		Author:    entry.Author(),
		Timestamp: entry.Timestamp(),
		EventType: entry.EventType(),
		Changes:   entry.Changes(),
	})
	r.histories[key] = append(r.histories[key], recorded)
	return id, nil
}

func (r *Repository) ListHistory(_ context.Context, issueID domain.ID, filter driven.HistoryFilter, limit int) ([]history.Entry, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	limit = driven.NormalizeLimit(limit)
	entries := r.histories[issueID.String()]

	var filtered []history.Entry
	for _, e := range entries {
		if !filter.Author.IsZero() && !e.Author().Equal(filter.Author) {
			continue
		}
		if !filter.After.IsZero() && !e.Timestamp().After(filter.After) {
			continue
		}
		if !filter.Before.IsZero() && !e.Timestamp().Before(filter.Before) {
			continue
		}
		filtered = append(filtered, e)
	}

	// Apply limit and detect hasMore.
	hasMore := false
	if limit > 0 && len(filtered) > limit {
		hasMore = true
		filtered = filtered[:limit]
	}

	return filtered, hasMore, nil
}

func (r *Repository) CountHistory(_ context.Context, issueID domain.ID) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.histories[issueID.String()]), nil
}

func (r *Repository) GetLatestHistory(_ context.Context, issueID domain.ID) (history.Entry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entries := r.histories[issueID.String()]
	if len(entries) == 0 {
		return history.Entry{}, domain.ErrNotFound
	}
	return entries[len(entries)-1], nil
}

// --- DatabaseRepository ---

func (r *Repository) InitDatabase(_ context.Context, prefix string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.prefix != "" {
		return fmt.Errorf("database already initialized with prefix %q", r.prefix)
	}
	r.prefix = prefix
	return nil
}

func (r *Repository) GetPrefix(_ context.Context) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.prefix == "" {
		return "", fmt.Errorf("database not initialized")
	}
	return r.prefix, nil
}

func (r *Repository) GC(_ context.Context, includeClosed bool) (int, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var deletedKeys, closedKeys []string
	for key, t := range r.issues {
		if t.IsDeleted() {
			deletedKeys = append(deletedKeys, key)
		} else if includeClosed && t.State() == domain.StateClosed {
			closedKeys = append(closedKeys, key)
		}
	}

	for _, key := range deletedKeys {
		r.removeIssue(key)
	}
	for _, key := range closedKeys {
		r.removeIssue(key)
	}

	return len(deletedKeys), len(closedKeys), nil
}

// removeIssue deletes an issue and its associated data from the in-memory
// store. The caller must hold r.mu.
func (r *Repository) removeIssue(key string) {
	delete(r.issues, key)
	delete(r.histories, key)
	for id, n := range r.comments {
		if n.IssueID().String() == key {
			delete(r.comments, id)
		}
	}
}

func (r *Repository) IntegrityCheck(_ context.Context) error {
	// Fake always reports healthy.
	return nil
}

// CountVirtualLabelsInTable always returns 0 in the in-memory adapter because
// it never stores virtual labels in the labels backing structure — they are
// always handled correctly.
func (r *Repository) CountVirtualLabelsInTable(_ context.Context) (int, error) {
	return 0, nil
}

// GetSchemaVersion always returns 2 in the in-memory adapter because the
// in-memory store is always freshly initialized at v2. There is no on-disk
// v1 schema to migrate.
func (r *Repository) GetSchemaVersion(_ context.Context) (int, error) {
	return 2, nil
}

// SetSchemaVersion is a no-op in the in-memory adapter. The in-memory store
// always operates at v2 and has no on-disk schema to migrate; this method
// exists to satisfy the DatabaseRepository interface.
func (r *Repository) SetSchemaVersion(_ context.Context, _ int) error {
	return nil
}

func (r *Repository) CountDeletedRatio(_ context.Context) (total, deleted int, err error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, t := range r.issues {
		total++
		if t.IsDeleted() {
			deleted++
		}
	}
	return total, deleted, nil
}

func (r *Repository) ClearAllData(_ context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.prefix = ""
	r.issues = make(map[string]domain.Issue)
	r.comments = make(map[int64]domain.Comment)
	r.claims = make(map[string]domain.Claim)
	r.claimsByIssue = make(map[string]string)
	r.relationships = nil
	r.histories = make(map[string][]history.Entry)
	r.nextNoteID = 1
	r.nextHistoryID = 1
	return nil
}

func (r *Repository) RestoreIssueRaw(_ context.Context, rec domain.BackupIssueRecord) error {
	// Fake does not need raw restore — backup/restore boundary tests use
	// the real SQLite adapter.
	return nil
}

func (r *Repository) RestoreCommentRaw(_ context.Context, _ string, _ domain.BackupCommentRecord) error {
	return nil
}

func (r *Repository) RestoreClaimRaw(_ context.Context, _ string, _ domain.BackupClaimRecord) error {
	return nil
}

func (r *Repository) RestoreRelationshipRaw(_ context.Context, _ string, _ domain.BackupRelationshipRecord) error {
	return nil
}

func (r *Repository) RestoreHistoryRaw(_ context.Context, _ string, _ domain.BackupHistoryRecord) error {
	return nil
}

func (r *Repository) RestoreLabelRaw(_ context.Context, _ string, _ domain.BackupLabelRecord) error {
	return nil
}

func (r *Repository) RebuildFTS(_ context.Context) error {
	return nil
}

// --- Internal helpers ---

func (r *Repository) matchesFilter(t domain.Issue, f driven.IssueFilter) bool {
	if len(f.Roles) > 0 && !slices.Contains(f.Roles, t.Role()) {
		return false
	}
	if len(f.States) > 0 && !slices.Contains(f.States, t.State()) {
		return false
	}
	if f.ExcludeClosed && len(f.States) == 0 && t.State() == domain.StateClosed {
		return false
	}
	if len(f.ParentIDs) > 0 && !slices.Contains(f.ParentIDs, t.ParentID()) {
		return false
	}
	if !f.DescendantsOf.IsZero() {
		if !r.isDescendantOf(t.ID(), f.DescendantsOf) {
			return false
		}
	}
	if !f.AncestorsOf.IsZero() {
		if !r.isAncestorOf(t.ID(), f.AncestorsOf) {
			return false
		}
	}
	if f.Ready {
		if !r.isIssueReady(t) {
			return false
		}
	}
	if f.Orphan && !t.ParentID().IsZero() {
		return false
	}
	if f.Blocked {
		if !r.isIssueBlocked(t) {
			return false
		}
	}
	for _, ff := range f.LabelFilters {
		val, exists := t.Labels().Get(ff.Key)
		if ff.Negate {
			if ff.Value == "" && exists {
				return false
			}
			if ff.Value != "" && exists && val == ff.Value {
				return false
			}
		} else {
			if ff.Value == "" && !exists {
				return false
			}
			if ff.Value != "" && (!exists || val != ff.Value) {
				return false
			}
		}
	}
	return true
}

func (r *Repository) isIssueReady(t domain.Issue) bool {
	blockers := r.getBlockerStatusesInternal(t.ID())
	ancestors := r.getAncestorStatusesInternal(t.ID())
	hasActiveClaim := r.hasActiveClaimInternal(t.ID())

	if t.IsTask() {
		return core.IsTaskReady(t.State(), hasActiveClaim, blockers, ancestors)
	}
	hasChildren := r.hasChildrenInternal(t.ID())
	return core.IsEpicReady(t.State(), hasActiveClaim, hasChildren, blockers, ancestors)
}

// hasActiveClaimInternal reports whether the issue has an active (non-stale)
// claim. A stale claim is treated as nonexistent for readiness and display
// purposes.
func (r *Repository) hasActiveClaimInternal(id domain.ID) bool {
	claimID, ok := r.claimsByIssue[id.String()]
	if !ok {
		return false
	}
	c, exists := r.claims[claimID]
	return exists && !c.IsStale(time.Now())
}

func (r *Repository) isIssueBlocked(t domain.Issue) bool {
	blockers := r.getBlockerStatusesInternal(t.ID())
	for _, b := range blockers {
		if !b.IsClosed && !b.IsDeleted {
			return true
		}
	}
	return false
}

func (r *Repository) getBlockerStatusesInternal(issueID domain.ID) []domain.BlockerStatus {
	var statuses []domain.BlockerStatus
	for _, rel := range r.relationships {
		if rel.SourceID() == issueID && rel.Type() == domain.RelBlockedBy {
			target, ok := r.issues[rel.TargetID().String()]
			if !ok {
				statuses = append(statuses, domain.BlockerStatus{IsDeleted: true})
				continue
			}
			statuses = append(statuses, domain.BlockerStatus{
				IsClosed:  target.State() == domain.StateClosed,
				IsDeleted: target.IsDeleted(),
			})
		}
	}
	return statuses
}

func (r *Repository) getAncestorStatusesInternal(id domain.ID) []domain.AncestorStatus {
	var ancestors []domain.AncestorStatus
	current := id
	visited := make(map[string]bool)

	for {
		t, ok := r.issues[current.String()]
		if !ok {
			break
		}
		parentID := t.ParentID()
		if parentID.IsZero() || visited[parentID.String()] {
			break
		}
		visited[parentID.String()] = true
		parent, ok := r.issues[parentID.String()]
		if !ok || parent.IsDeleted() {
			break
		}
		ancestors = append(ancestors, domain.AncestorStatus{
			ID:        parent.ID(),
			State:     parent.State(),
			IsBlocked: r.isIssueBlocked(parent),
		})
		current = parentID
	}
	return ancestors
}

func (r *Repository) isDescendantOf(id domain.ID, ancestorID domain.ID) bool {
	current := id
	visited := make(map[string]bool)
	for {
		t, ok := r.issues[current.String()]
		if !ok {
			return false
		}
		parentID := t.ParentID()
		if parentID.IsZero() || visited[parentID.String()] {
			return false
		}
		if parentID == ancestorID {
			return true
		}
		visited[parentID.String()] = true
		current = parentID
	}
}

func (r *Repository) isAncestorOf(candidateID domain.ID, childID domain.ID) bool {
	// Walk up from childID; if we hit candidateID, it's an ancestor.
	current := childID
	visited := make(map[string]bool)
	for {
		t, ok := r.issues[current.String()]
		if !ok {
			return false
		}
		parentID := t.ParentID()
		if parentID.IsZero() || visited[parentID.String()] {
			return false
		}
		if parentID == candidateID {
			return true
		}
		visited[parentID.String()] = true
		current = parentID
	}
}

func (r *Repository) hasChildrenInternal(epicID domain.ID) bool {
	for _, t := range r.issues {
		if !t.IsDeleted() && t.ParentID() == epicID {
			return true
		}
	}
	return false
}

func (r *Repository) getDescendantsInternal(epicID domain.ID) []domain.DescendantInfo {
	var descendants []domain.DescendantInfo
	for _, t := range r.issues {
		if t.IsDeleted() || t.ParentID() != epicID {
			continue
		}
		isClaimed := false
		claimedBy := ""
		if claimID, ok := r.claimsByIssue[t.ID().String()]; ok {
			if c, ok := r.claims[claimID]; ok {
				isClaimed = true
				claimedBy = c.Author().String()
			}
		}
		descendants = append(descendants, domain.DescendantInfo{
			ID:        t.ID(),
			IsClaimed: isClaimed,
			ClaimedBy: claimedBy,
		})
		if t.IsEpic() {
			descendants = append(descendants, r.getDescendantsInternal(t.ID())...)
		}
	}
	return descendants
}

func (r *Repository) matchesSearch(t domain.Issue, queryLower string) bool {
	return strings.Contains(strings.ToLower(t.Title()), queryLower) ||
		strings.Contains(strings.ToLower(t.Description()), queryLower) ||
		strings.Contains(strings.ToLower(t.AcceptanceCriteria()), queryLower)
}

// commentScopeIssueIDs builds the set of issue IDs whose comments are in scope
// for the given filter. The scope filters (IssueID, IssueIDs, ParentIDs,
// TreeIDs, LabelFilters) are OR'd — a comment's issue must appear in at least
// one scope. FollowRefs expands the combined scope by adding issues referenced
// by any already-in-scope issue. Returns nil when no scope filters are set,
// meaning all issues are in scope.
func (r *Repository) commentScopeIssueIDs(f driven.CommentFilter) map[string]struct{} {
	hasScope := !f.IssueID.IsZero() || len(f.IssueIDs) > 0 ||
		len(f.ParentIDs) > 0 || len(f.TreeIDs) > 0 || len(f.LabelFilters) > 0
	if !hasScope {
		return nil
	}

	scope := make(map[string]struct{})

	// Direct issue ID.
	if !f.IssueID.IsZero() {
		scope[f.IssueID.String()] = struct{}{}
	}

	// Multiple issue IDs.
	for _, id := range f.IssueIDs {
		scope[id.String()] = struct{}{}
	}

	// Parent scope: the parent itself plus its direct children.
	for _, parentID := range f.ParentIDs {
		scope[parentID.String()] = struct{}{}
		for key, iss := range r.issues {
			if iss.ParentID() == parentID {
				scope[key] = struct{}{}
			}
		}
	}

	// Tree scope: the root plus all descendants (recursive).
	for _, treeID := range f.TreeIDs {
		r.collectDescendants(treeID, scope)
	}

	// Label scope: issues matching label key/value criteria.
	for _, lf := range f.LabelFilters {
		for key, iss := range r.issues {
			if r.issueMatchesLabelFilter(iss, lf) {
				scope[key] = struct{}{}
			}
		}
	}

	// FollowRefs: expand scope to include issues referenced (via any
	// relationship) by already-in-scope issues.
	if f.FollowRefs {
		var extras []string
		for _, rel := range r.relationships {
			srcKey := rel.SourceID().String()
			tgtKey := rel.TargetID().String()
			if _, ok := scope[srcKey]; ok {
				extras = append(extras, tgtKey)
			}
			// Symmetric: if target is in scope, add source too.
			if _, ok := scope[tgtKey]; ok {
				extras = append(extras, srcKey)
			}
		}
		for _, e := range extras {
			scope[e] = struct{}{}
		}
	}

	return scope
}

// collectDescendants adds treeID and all its transitive descendants to scope.
func (r *Repository) collectDescendants(treeID domain.ID, scope map[string]struct{}) {
	scope[treeID.String()] = struct{}{}
	for key, iss := range r.issues {
		if iss.ParentID() == treeID {
			if _, visited := scope[key]; !visited {
				r.collectDescendants(iss.ID(), scope)
			}
		}
	}
}

// issueMatchesLabelFilter checks whether an issue's labels match a single
// LabelFilter criterion. Virtual label keys (e.g., "idempotency-key") are
// checked against the issue's dedicated field; regular labels are checked
// against the issue's LabelSet.
func (r *Repository) issueMatchesLabelFilter(iss domain.Issue, lf driven.LabelFilter) bool {
	var matched bool

	if domain.IsVirtualLabelKey(lf.Key) {
		// Only "idempotency-key" is currently virtual.
		val := iss.IdempotencyKey()
		if lf.Value == "" {
			matched = val != ""
		} else {
			matched = val == lf.Value
		}
	} else {
		v, ok := iss.Labels().Get(lf.Key)
		if lf.Value == "" {
			matched = ok // key-only wildcard match
		} else {
			matched = ok && v == lf.Value
		}
	}

	if lf.Negate {
		return !matched
	}
	return matched
}

func (r *Repository) matchesCommentFilter(n domain.Comment, f driven.CommentFilter) bool {
	if !f.Author.IsZero() && !n.Author().Equal(f.Author) {
		return false
	}
	if len(f.Authors) > 0 {
		matched := false
		for _, a := range f.Authors {
			if n.Author().Equal(a) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if !f.CreatedAfter.IsZero() && !n.CreatedAt().After(f.CreatedAfter) {
		return false
	}
	if f.AfterCommentID > 0 && n.ID() <= f.AfterCommentID {
		return false
	}
	return true
}

func (r *Repository) issueToListItem(t domain.Issue) driven.IssueListItem {
	return driven.IssueListItem{
		ID:         t.ID(),
		Role:       t.Role(),
		State:      t.State(),
		Priority:   t.Priority(),
		Title:      t.Title(),
		ParentID:   t.ParentID(),
		CreatedAt:  t.CreatedAt(),
		IsDeleted:  t.IsDeleted(),
		IsBlocked:  r.isIssueBlocked(t),
		BlockerIDs: r.directBlockerIDs(t.ID()),
	}
}

// directBlockerIDs returns the IDs of non-closed, non-deleted issues that
// directly block the given issue via blocked_by relationships.
func (r *Repository) directBlockerIDs(issueID domain.ID) []domain.ID {
	var ids []domain.ID
	for _, rel := range r.relationships {
		if rel.SourceID() == issueID && rel.Type() == domain.RelBlockedBy {
			target, ok := r.issues[rel.TargetID().String()]
			if !ok || target.IsDeleted() || target.State() == domain.StateClosed {
				continue
			}
			ids = append(ids, rel.TargetID())
		}
	}
	return ids
}

// familyAnchor returns the creation time used for family-anchored sorting.
// For child issues, it returns the parent's creation time so that children
// cluster with their parent in sorted output. For parentless issues, it
// returns the issue's own creation time.
func (r *Repository) familyAnchor(item driven.IssueListItem) time.Time {
	if !item.ParentID.IsZero() {
		if parent, ok := r.issues[item.ParentID.String()]; ok {
			return parent.CreatedAt()
		}
	}
	return item.CreatedAt
}

func (r *Repository) sortIssueItems(items []driven.IssueListItem, orderBy driven.IssueOrderBy) {
	slices.SortFunc(items, func(a, b driven.IssueListItem) int {
		switch orderBy {
		case driven.OrderByPriority:
			if c := cmp.Compare(int(a.Priority), int(b.Priority)); c != 0 {
				return c
			}
			if c := r.familyAnchor(a).Compare(r.familyAnchor(b)); c != 0 {
				return c
			}
			if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
				return c
			}
			return cmp.Compare(a.ID.String(), b.ID.String())
		case driven.OrderByCreatedAt:
			if c := r.familyAnchor(a).Compare(r.familyAnchor(b)); c != 0 {
				return c
			}
			if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
				return c
			}
			return cmp.Compare(a.ID.String(), b.ID.String())
		case driven.OrderByUpdatedAt:
			if c := r.familyAnchor(b).Compare(r.familyAnchor(a)); c != 0 {
				return c
			}
			if c := b.CreatedAt.Compare(a.CreatedAt); c != 0 {
				return c
			}
			return cmp.Compare(a.ID.String(), b.ID.String())
		default:
			return 0
		}
	})
}
