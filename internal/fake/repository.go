package fake

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/domain/claim"
	"github.com/pinkhop/nitpicking/internal/domain/comment"
	"github.com/pinkhop/nitpicking/internal/domain/history"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
	"github.com/pinkhop/nitpicking/internal/domain/port"
)

// Repository is an in-memory implementation of all persistence port interfaces.
// It is safe for concurrent use.
type Repository struct {
	mu sync.RWMutex

	prefix   string
	issues   map[string]issue.Issue    // keyed by issue ID string
	comments map[int64]comment.Comment // keyed by comment ID
	claims   map[string]claim.Claim    // keyed by claim ID
	// claimsByIssue maps issue ID string → claim ID for active claims.
	claimsByIssue map[string]string
	relationships []issue.Relationship
	histories     map[string][]history.Entry // keyed by issue ID string
	nextNoteID    int64
	nextHistoryID int64
}

// NewRepository creates an empty in-memory repository.
func NewRepository() *Repository {
	return &Repository{
		issues:        make(map[string]issue.Issue),
		comments:      make(map[int64]comment.Comment),
		claims:        make(map[string]claim.Claim),
		claimsByIssue: make(map[string]string),
		histories:     make(map[string][]history.Entry),
		nextNoteID:    1,
		nextHistoryID: 1,
	}
}

// --- IssueRepository ---

func (r *Repository) CreateIssue(_ context.Context, t issue.Issue) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := t.ID().String()
	if _, exists := r.issues[key]; exists {
		return fmt.Errorf("issue %s already exists", key)
	}
	r.issues[key] = t
	return nil
}

func (r *Repository) GetIssue(_ context.Context, id issue.ID, includeDeleted bool) (issue.Issue, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.issues[id.String()]
	if !ok {
		return issue.Issue{}, domain.ErrNotFound
	}
	if t.IsDeleted() && !includeDeleted {
		return issue.Issue{}, domain.ErrNotFound
	}
	return t, nil
}

func (r *Repository) UpdateIssue(_ context.Context, t issue.Issue) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := t.ID().String()
	if _, exists := r.issues[key]; !exists {
		return domain.ErrNotFound
	}
	r.issues[key] = t
	return nil
}

func (r *Repository) ListIssues(_ context.Context, filter port.IssueFilter, orderBy port.IssueOrderBy, limit int) ([]port.IssueListItem, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	limit = port.NormalizeLimit(limit)

	var items []port.IssueListItem
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

func (r *Repository) SearchIssues(_ context.Context, query string, filter port.IssueFilter, orderBy port.IssueOrderBy, limit int) ([]port.IssueListItem, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	limit = port.NormalizeLimit(limit)
	queryLower := strings.ToLower(query)

	var items []port.IssueListItem
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

func (r *Repository) GetChildStatuses(_ context.Context, epicID issue.ID) ([]issue.ChildStatus, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var children []issue.ChildStatus
	for _, t := range r.issues {
		if t.IsDeleted() {
			continue
		}
		if t.ParentID() == epicID {
			children = append(children, issue.ChildStatus{State: t.State()})
		}
	}
	return children, nil
}

func (r *Repository) GetDescendants(_ context.Context, epicID issue.ID) ([]issue.DescendantInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.getDescendantsInternal(epicID), nil
}

func (r *Repository) HasChildren(_ context.Context, epicID issue.ID) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, t := range r.issues {
		if !t.IsDeleted() && t.ParentID() == epicID {
			return true, nil
		}
	}
	return false, nil
}

func (r *Repository) GetAncestorStatuses(_ context.Context, id issue.ID) ([]issue.AncestorStatus, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var ancestors []issue.AncestorStatus
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
		ancestors = append(ancestors, issue.AncestorStatus{
			State:     parent.State(),
			IsBlocked: r.isIssueBlocked(parent),
		})
		current = parentID
	}

	return ancestors, nil
}

func (r *Repository) GetParentID(_ context.Context, id issue.ID) (issue.ID, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.issues[id.String()]
	if !ok {
		return issue.ID{}, domain.ErrNotFound
	}
	return t.ParentID(), nil
}

func (r *Repository) IssueIDExists(_ context.Context, id issue.ID) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.issues[id.String()]
	return exists, nil
}

func (r *Repository) ListDistinctLabels(_ context.Context) ([]issue.Label, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := make(map[string]bool)
	var dims []issue.Label
	for _, t := range r.issues {
		if t.IsDeleted() {
			continue
		}
		for k, v := range t.Labels().All() {
			key := k + ":" + v
			if !seen[key] {
				seen[key] = true
				dim, _ := issue.NewLabel(k, v)
				dims = append(dims, dim)
			}
		}
	}
	return dims, nil
}

func (r *Repository) GetIssueByIdempotencyKey(_ context.Context, key string) (issue.Issue, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, t := range r.issues {
		if t.IdempotencyKey() == key && key != "" {
			return t, nil
		}
	}
	return issue.Issue{}, domain.ErrNotFound
}

// --- CommentRepository ---

func (r *Repository) CreateComment(_ context.Context, n comment.Comment) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := r.nextNoteID
	r.nextNoteID++

	// Reconstruct comment with the assigned ID.
	created, err := comment.NewComment(comment.NewCommentParams{
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

func (r *Repository) GetComment(_ context.Context, id int64) (comment.Comment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	n, ok := r.comments[id]
	if !ok {
		return comment.Comment{}, domain.ErrNotFound
	}
	return n, nil
}

func (r *Repository) ListComments(_ context.Context, issueID issue.ID, filter port.CommentFilter, limit int) ([]comment.Comment, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	limit = port.NormalizeLimit(limit)

	var comments []comment.Comment
	for _, n := range r.comments {
		if n.IssueID() != issueID {
			continue
		}
		if !r.matchesCommentFilter(n, filter) {
			continue
		}
		comments = append(comments, n)
	}

	slices.SortFunc(comments, func(a, b comment.Comment) int {
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

func (r *Repository) SearchComments(_ context.Context, query string, filter port.CommentFilter, limit int) ([]comment.Comment, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	limit = port.NormalizeLimit(limit)
	queryLower := strings.ToLower(query)

	var comments []comment.Comment
	for _, n := range r.comments {
		if !filter.IssueID.IsZero() && n.IssueID() != filter.IssueID {
			continue
		}
		if !r.matchesCommentFilter(n, filter) {
			continue
		}
		if !strings.Contains(strings.ToLower(n.Body()), queryLower) {
			continue
		}
		comments = append(comments, n)
	}

	slices.SortFunc(comments, func(a, b comment.Comment) int {
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

func (r *Repository) CreateClaim(_ context.Context, c claim.Claim) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.claims[c.ID()] = c
	r.claimsByIssue[c.IssueID().String()] = c.ID()
	return nil
}

func (r *Repository) GetClaimByIssue(_ context.Context, issueID issue.ID) (claim.Claim, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	claimID, ok := r.claimsByIssue[issueID.String()]
	if !ok {
		return claim.Claim{}, domain.ErrNotFound
	}
	c, ok := r.claims[claimID]
	if !ok {
		return claim.Claim{}, domain.ErrNotFound
	}
	return c, nil
}

func (r *Repository) GetClaimByID(_ context.Context, claimID string) (claim.Claim, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	c, ok := r.claims[claimID]
	if !ok {
		return claim.Claim{}, domain.ErrNotFound
	}
	return c, nil
}

func (r *Repository) InvalidateClaim(_ context.Context, claimID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	c, ok := r.claims[claimID]
	if !ok {
		return domain.ErrNotFound
	}
	delete(r.claimsByIssue, c.IssueID().String())
	delete(r.claims, claimID)
	return nil
}

func (r *Repository) UpdateClaimLastActivity(_ context.Context, claimID string, lastActivity time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	c, ok := r.claims[claimID]
	if !ok {
		return domain.ErrNotFound
	}
	r.claims[claimID] = c.WithLastActivity(lastActivity)
	return nil
}

func (r *Repository) UpdateClaimThreshold(_ context.Context, claimID string, threshold time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	c, ok := r.claims[claimID]
	if !ok {
		return domain.ErrNotFound
	}
	updated, err := c.WithStaleThreshold(threshold)
	if err != nil {
		return err
	}
	r.claims[claimID] = updated
	return nil
}

func (r *Repository) ListStaleClaims(_ context.Context, now time.Time) ([]claim.Claim, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var stale []claim.Claim
	for _, c := range r.claims {
		if c.IsStale(now) {
			stale = append(stale, c)
		}
	}
	return stale, nil
}

func (r *Repository) ListActiveClaims(_ context.Context, now time.Time) ([]claim.Claim, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var active []claim.Claim
	for _, c := range r.claims {
		if !c.IsStale(now) {
			active = append(active, c)
		}
	}
	return active, nil
}

// --- RelationshipRepository ---

func (r *Repository) CreateRelationship(_ context.Context, rel issue.Relationship) (bool, error) {
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

func (r *Repository) DeleteRelationship(_ context.Context, sourceID, targetID issue.ID, relType issue.RelationType) (bool, error) {
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

func (r *Repository) ListRelationships(_ context.Context, issueID issue.ID) ([]issue.Relationship, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var rels []issue.Relationship
	for _, rel := range r.relationships {
		if rel.SourceID() == issueID {
			rels = append(rels, rel)
		} else if rel.TargetID() == issueID {
			if rel.Type().IsSymmetric() {
				// Present the symmetric relationship from this issue's perspective.
				swapped, _ := issue.NewRelationship(issueID, rel.SourceID(), rel.Type())
				rels = append(rels, swapped)
			} else {
				rels = append(rels, rel)
			}
		}
	}
	return rels, nil
}

func (r *Repository) GetBlockerStatuses(_ context.Context, issueID issue.ID) ([]issue.BlockerStatus, error) {
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

func (r *Repository) ListHistory(_ context.Context, issueID issue.ID, filter port.HistoryFilter, limit int) ([]history.Entry, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	limit = port.NormalizeLimit(limit)
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

func (r *Repository) CountHistory(_ context.Context, issueID issue.ID) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.histories[issueID.String()]), nil
}

func (r *Repository) GetLatestHistory(_ context.Context, issueID issue.ID) (history.Entry, error) {
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

func (r *Repository) GC(_ context.Context, includeClosed bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var toDelete []string
	for key, t := range r.issues {
		if t.IsDeleted() {
			toDelete = append(toDelete, key)
		} else if includeClosed && t.State() == issue.StateClosed {
			toDelete = append(toDelete, key)
		}
	}

	for _, key := range toDelete {
		delete(r.issues, key)
		delete(r.histories, key)
		// Remove related comments.
		for id, n := range r.comments {
			if n.IssueID().String() == key {
				delete(r.comments, id)
			}
		}
	}

	return nil
}

func (r *Repository) IntegrityCheck(_ context.Context) error {
	// Fake always reports healthy.
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

// --- Internal helpers ---

func (r *Repository) matchesFilter(t issue.Issue, f port.IssueFilter) bool {
	if f.Role != 0 && t.Role() != f.Role {
		return false
	}
	if len(f.States) > 0 && !slices.Contains(f.States, t.State()) {
		return false
	}
	if f.ExcludeClosed && len(f.States) == 0 && t.State() == issue.StateClosed {
		return false
	}
	if !f.ParentID.IsZero() && t.ParentID() != f.ParentID {
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

func (r *Repository) isIssueReady(t issue.Issue) bool {
	blockers := r.getBlockerStatusesInternal(t.ID())
	ancestors := r.getAncestorStatusesInternal(t.ID())

	if t.IsTask() {
		return issue.IsTaskReady(t.State(), blockers, ancestors)
	}
	hasChildren := r.hasChildrenInternal(t.ID())
	return issue.IsEpicReady(t.State(), hasChildren, blockers, ancestors)
}

func (r *Repository) isIssueBlocked(t issue.Issue) bool {
	blockers := r.getBlockerStatusesInternal(t.ID())
	for _, b := range blockers {
		if !b.IsClosed && !b.IsDeleted {
			return true
		}
	}
	return false
}

func (r *Repository) getBlockerStatusesInternal(issueID issue.ID) []issue.BlockerStatus {
	var statuses []issue.BlockerStatus
	for _, rel := range r.relationships {
		if rel.SourceID() == issueID && rel.Type() == issue.RelBlockedBy {
			target, ok := r.issues[rel.TargetID().String()]
			if !ok {
				statuses = append(statuses, issue.BlockerStatus{IsDeleted: true})
				continue
			}
			statuses = append(statuses, issue.BlockerStatus{
				IsClosed:  target.State() == issue.StateClosed,
				IsDeleted: target.IsDeleted(),
			})
		}
	}
	return statuses
}

func (r *Repository) getAncestorStatusesInternal(id issue.ID) []issue.AncestorStatus {
	var ancestors []issue.AncestorStatus
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
		ancestors = append(ancestors, issue.AncestorStatus{
			State:     parent.State(),
			IsBlocked: r.isIssueBlocked(parent),
		})
		current = parentID
	}
	return ancestors
}

func (r *Repository) isDescendantOf(id issue.ID, ancestorID issue.ID) bool {
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

func (r *Repository) isAncestorOf(candidateID issue.ID, childID issue.ID) bool {
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

func (r *Repository) hasChildrenInternal(epicID issue.ID) bool {
	for _, t := range r.issues {
		if !t.IsDeleted() && t.ParentID() == epicID {
			return true
		}
	}
	return false
}

func (r *Repository) getDescendantsInternal(epicID issue.ID) []issue.DescendantInfo {
	var descendants []issue.DescendantInfo
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
		descendants = append(descendants, issue.DescendantInfo{
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

func (r *Repository) matchesSearch(t issue.Issue, queryLower string) bool {
	return strings.Contains(strings.ToLower(t.Title()), queryLower) ||
		strings.Contains(strings.ToLower(t.Description()), queryLower) ||
		strings.Contains(strings.ToLower(t.AcceptanceCriteria()), queryLower)
}

func (r *Repository) matchesCommentFilter(n comment.Comment, f port.CommentFilter) bool {
	if !f.Author.IsZero() && !n.Author().Equal(f.Author) {
		return false
	}
	if !f.CreatedAfter.IsZero() && !n.CreatedAt().After(f.CreatedAfter) {
		return false
	}
	if f.AfterCommentID > 0 && n.ID() <= f.AfterCommentID {
		return false
	}
	return true
}

func (r *Repository) issueToListItem(t issue.Issue) port.IssueListItem {
	return port.IssueListItem{
		ID:        t.ID(),
		Role:      t.Role(),
		State:     t.State(),
		Priority:  t.Priority(),
		Title:     t.Title(),
		ParentID:  t.ParentID(),
		CreatedAt: t.CreatedAt(),
		IsDeleted: t.IsDeleted(),
		IsBlocked: r.isIssueBlocked(t),
	}
}

func (r *Repository) sortIssueItems(items []port.IssueListItem, orderBy port.IssueOrderBy) {
	slices.SortFunc(items, func(a, b port.IssueListItem) int {
		switch orderBy {
		case port.OrderByPriority:
			if c := cmp.Compare(int(a.Priority), int(b.Priority)); c != 0 {
				return c
			}
			return a.CreatedAt.Compare(b.CreatedAt)
		case port.OrderByCreatedAt:
			return a.CreatedAt.Compare(b.CreatedAt)
		case port.OrderByUpdatedAt:
			return b.CreatedAt.Compare(a.CreatedAt)
		default:
			return 0
		}
	})
}
