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
	"github.com/pinkhop/nitpicking/internal/domain/history"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
	"github.com/pinkhop/nitpicking/internal/domain/note"
	"github.com/pinkhop/nitpicking/internal/domain/port"
)

// Repository is an in-memory implementation of all persistence port interfaces.
// It is safe for concurrent use.
type Repository struct {
	mu sync.RWMutex

	prefix string
	issues map[string]issue.Issue // keyed by issue ID string
	notes  map[int64]note.Note    // keyed by note ID
	claims map[string]claim.Claim // keyed by claim ID
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
		notes:         make(map[int64]note.Note),
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

func (r *Repository) ListIssues(_ context.Context, filter port.IssueFilter, orderBy port.IssueOrderBy, page port.PageRequest) ([]port.IssueListItem, port.PageResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	page = page.Normalize()

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
	total := len(items)

	// Apply keyset pagination.
	items = r.applyPagination(items, page)

	return items, port.PageResult{TotalCount: total}, nil
}

func (r *Repository) SearchIssues(_ context.Context, query string, filter port.IssueFilter, orderBy port.IssueOrderBy, page port.PageRequest) ([]port.IssueListItem, port.PageResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	page = page.Normalize()
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
	total := len(items)
	items = r.applyPagination(items, page)

	return items, port.PageResult{TotalCount: total}, nil
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
			isComplete := false
			if t.IsEpic() {
				isComplete = r.isEpicCompleteInternal(t.ID())
			}
			children = append(children, issue.ChildStatus{
				Role:       t.Role(),
				State:      t.State(),
				IsComplete: isComplete,
			})
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
		ancestors = append(ancestors, issue.AncestorStatus{State: parent.State()})
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

// --- NoteRepository ---

func (r *Repository) CreateNote(_ context.Context, n note.Note) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := r.nextNoteID
	r.nextNoteID++

	// Reconstruct note with the assigned ID.
	created, err := note.NewNote(note.NewNoteParams{
		ID:        id,
		IssueID:   n.IssueID(),
		Author:    n.Author(),
		CreatedAt: n.CreatedAt(),
		Body:      n.Body(),
	})
	if err != nil {
		return 0, err
	}

	r.notes[id] = created
	return id, nil
}

func (r *Repository) GetNote(_ context.Context, id int64) (note.Note, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	n, ok := r.notes[id]
	if !ok {
		return note.Note{}, domain.ErrNotFound
	}
	return n, nil
}

func (r *Repository) ListNotes(_ context.Context, issueID issue.ID, filter port.NoteFilter, page port.PageRequest) ([]note.Note, port.PageResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	page = page.Normalize()

	var notes []note.Note
	for _, n := range r.notes {
		if n.IssueID() != issueID {
			continue
		}
		if !r.matchesNoteFilter(n, filter) {
			continue
		}
		notes = append(notes, n)
	}

	slices.SortFunc(notes, func(a, b note.Note) int {
		return cmp.Compare(a.ID(), b.ID())
	})

	total := len(notes)
	if page.PageSize > 0 && len(notes) > page.PageSize {
		notes = notes[:page.PageSize]
	}

	return notes, port.PageResult{TotalCount: total}, nil
}

func (r *Repository) SearchNotes(_ context.Context, query string, filter port.NoteFilter, page port.PageRequest) ([]note.Note, port.PageResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	page = page.Normalize()
	queryLower := strings.ToLower(query)

	var notes []note.Note
	for _, n := range r.notes {
		if !filter.IssueID.IsZero() && n.IssueID() != filter.IssueID {
			continue
		}
		if !r.matchesNoteFilter(n, filter) {
			continue
		}
		if !strings.Contains(strings.ToLower(n.Body()), queryLower) {
			continue
		}
		notes = append(notes, n)
	}

	slices.SortFunc(notes, func(a, b note.Note) int {
		return cmp.Compare(a.ID(), b.ID())
	})

	total := len(notes)
	if page.PageSize > 0 && len(notes) > page.PageSize {
		notes = notes[:page.PageSize]
	}

	return notes, port.PageResult{TotalCount: total}, nil
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

// --- RelationshipRepository ---

func (r *Repository) CreateRelationship(_ context.Context, rel issue.Relationship) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, existing := range r.relationships {
		if existing.SourceID() == rel.SourceID() &&
			existing.TargetID() == rel.TargetID() &&
			existing.Type() == rel.Type() {
			return false, nil // Already exists — idempotent.
		}
	}
	r.relationships = append(r.relationships, rel)
	return true, nil
}

func (r *Repository) DeleteRelationship(_ context.Context, sourceID, targetID issue.ID, relType issue.RelationType) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, existing := range r.relationships {
		if existing.SourceID() == sourceID &&
			existing.TargetID() == targetID &&
			existing.Type() == relType {
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
		if rel.SourceID() == issueID || rel.TargetID() == issueID {
			rels = append(rels, rel)
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

func (r *Repository) ListHistory(_ context.Context, issueID issue.ID, filter port.HistoryFilter, page port.PageRequest) ([]history.Entry, port.PageResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	page = page.Normalize()
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

	total := len(filtered)
	if page.PageSize > 0 && len(filtered) > page.PageSize {
		filtered = filtered[:page.PageSize]
	}

	return filtered, port.PageResult{TotalCount: total}, nil
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
		// Remove related notes.
		for id, n := range r.notes {
			if n.IssueID().String() == key {
				delete(r.notes, id)
			}
		}
	}

	return nil
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
	for _, ff := range f.FacetFilters {
		val, exists := t.Facets().Get(ff.Key)
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
				IsClosed:   target.State() == issue.StateClosed,
				IsDeleted:  target.IsDeleted(),
				IsComplete: target.IsEpic() && r.isEpicCompleteInternal(target.ID()),
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
		ancestors = append(ancestors, issue.AncestorStatus{State: parent.State()})
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

func (r *Repository) isEpicCompleteInternal(epicID issue.ID) bool {
	var children []issue.ChildStatus
	for _, t := range r.issues {
		if t.IsDeleted() || t.ParentID() != epicID {
			continue
		}
		isComplete := false
		if t.IsEpic() {
			isComplete = r.isEpicCompleteInternal(t.ID())
		}
		children = append(children, issue.ChildStatus{
			Role:       t.Role(),
			State:      t.State(),
			IsComplete: isComplete,
		})
	}
	return issue.IsEpicComplete(children)
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

func (r *Repository) matchesNoteFilter(n note.Note, f port.NoteFilter) bool {
	if !f.Author.IsZero() && !n.Author().Equal(f.Author) {
		return false
	}
	if !f.CreatedAfter.IsZero() && !n.CreatedAt().After(f.CreatedAfter) {
		return false
	}
	if f.AfterNoteID > 0 && n.ID() <= f.AfterNoteID {
		return false
	}
	return true
}

func (r *Repository) issueToListItem(t issue.Issue) port.IssueListItem {
	updatedAt := t.CreatedAt()
	if entries, ok := r.histories[t.ID().String()]; ok && len(entries) > 0 {
		updatedAt = entries[len(entries)-1].Timestamp()
	}
	return port.IssueListItem{
		ID:        t.ID(),
		Role:      t.Role(),
		State:     t.State(),
		Priority:  t.Priority(),
		Title:     t.Title(),
		ParentID:  t.ParentID(),
		CreatedAt: t.CreatedAt(),
		UpdatedAt: updatedAt,
		IsDeleted: t.IsDeleted(),
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
			return b.UpdatedAt.Compare(a.UpdatedAt)
		default:
			return 0
		}
	})
}

func (r *Repository) applyPagination(items []port.IssueListItem, page port.PageRequest) []port.IssueListItem {
	if page.AfterID != "" {
		idx := slices.IndexFunc(items, func(item port.IssueListItem) bool {
			return item.ID.String() == page.AfterID
		})
		if idx >= 0 && idx+1 < len(items) {
			items = items[idx+1:]
		} else if idx >= 0 {
			return nil
		}
	}

	if page.PageSize > 0 && len(items) > page.PageSize {
		items = items[:page.PageSize]
	}
	return items
}
