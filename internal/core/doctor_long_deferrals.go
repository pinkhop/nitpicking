package core

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/domain/history"
	"github.com/pinkhop/nitpicking/internal/ports/driven"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// defaultLongDeferralThreshold is the staleness threshold used when
// DoctorInput.LongDeferralThreshold is zero.
const defaultLongDeferralThreshold = 7 * 24 * time.Hour

// runLongDeferrals detects deferred issues whose last activity exceeds the
// configured staleness threshold. last_activity_at is the maximum of:
//   - the most recent history entry timestamp (captures issue updates,
//     state changes, relationship events, and comment-added events)
//   - the most recent comment timestamp (guards against out-of-band comment
//     insertions that may not produce a history entry)
//
// deferred_at is the timestamp of the most recent EventStateChanged history
// entry, which for a currently-deferred issue must be the deferral event.
// Falls back to issue.CreatedAt when history is absent.
func runLongDeferrals(ctx context.Context, svc *serviceImpl, input driving.DoctorInput) (*doctorRunResult, error) {
	if svc.tx == nil {
		return nil, fmt.Errorf("long-deferrals: database connection unavailable (cascade should have protected this check)")
	}

	threshold := input.LongDeferralThreshold
	if threshold == 0 {
		threshold = defaultLongDeferralThreshold
	}

	now := time.Now()
	var rows []driving.LongDeferralRow

	err := svc.tx.WithReadTransaction(ctx, func(uow driven.UnitOfWork) error {
		items, _, listErr := uow.Issues().ListIssues(ctx,
			driven.IssueFilter{States: []domain.State{domain.StateDeferred}},
			driven.OrderByID,
			driven.SortAscending,
			-1,
		)
		if listErr != nil {
			return fmt.Errorf("listing deferred issues: %w", listErr)
		}

		for _, item := range items {
			lastActivity, deferredAt, err := computeDeferralActivity(ctx, uow, item.ID, item.CreatedAt)
			if err != nil {
				return fmt.Errorf("computing activity for %s: %w", item.ID, err)
			}
			if now.Sub(lastActivity) > threshold {
				rows = append(rows, driving.LongDeferralRow{
					Issue:          item.ID.String(),
					DeferredAt:     deferredAt.UTC(),
					LastActivityAt: lastActivity.UTC(),
				})
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("long-deferrals: %w", err)
	}

	if len(rows) == 0 {
		return nil, nil
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].Issue < rows[j].Issue })

	affected := make([]any, len(rows))
	for i, r := range rows {
		affected[i] = r
	}
	return &doctorRunResult{
		Summary:  fmt.Sprintf("%d deferred issue(s) have had no activity for more than the staleness threshold", len(rows)),
		Affected: affected,
	}, nil
}

// computeDeferralActivity returns the last_activity_at and deferred_at
// timestamps for a deferred issue. It scans the issue's own history,
// comments, and the histories of issues that have a relationship pointing
// at this one (so target-side relationship activity counts).
// Falls back to issueCreatedAt when no history entries exist.
//
// Limitation: when a relationship where this issue was the target has been
// removed, the historical removal event lives on a source issue we may no
// longer be able to find via ListRelationships. We accept that gap in
// favour of bounded per-issue work.
func computeDeferralActivity(ctx context.Context, uow driven.UnitOfWork, issueID domain.ID, issueCreatedAt time.Time) (lastActivity, deferredAt time.Time, err error) {
	// Scan the issue's own history: tracks the overall latest timestamp
	// (last_activity_at) and the most recent transition INTO the deferred
	// state (deferred_at). The state-change check inspects the FieldChange's
	// After value rather than relying on the bare event type so a
	// close-and-reopen sequence doesn't bleed into deferred_at.
	entries, _, err := uow.History().ListHistory(ctx, issueID, driven.HistoryFilter{}, -1)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	for _, e := range entries {
		if e.Timestamp().After(lastActivity) {
			lastActivity = e.Timestamp()
		}
		if e.EventType() == history.EventStateChanged && stateChangeTargetIsDeferred(e) && e.Timestamp().After(deferredAt) {
			deferredAt = e.Timestamp()
		}
	}

	// Scan comments: comment recency must win even when the comment does
	// not have a corresponding EventCommentAdded history entry (e.g., bulk
	// import paths).
	comments, _, err := uow.Comments().ListComments(ctx, issueID, driven.CommentFilter{}, -1)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	for _, c := range comments {
		if c.CreatedAt().After(lastActivity) {
			lastActivity = c.CreatedAt()
		}
	}

	// Scan source-side history for relationships where this issue is the
	// target. Relationship events are recorded only on the source side, so
	// without this pass an event like "C blocked_by D" would not register as
	// activity on the deferred target D.
	rels, err := uow.Relationships().ListRelationships(ctx, issueID)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	for _, rel := range rels {
		if rel.SourceID() == issueID {
			continue // already covered by the issue's own history scan above
		}
		ts, scanErr := mostRecentRelationshipEventInvolving(ctx, uow, rel.SourceID(), issueID)
		if scanErr != nil {
			return time.Time{}, time.Time{}, scanErr
		}
		if ts.After(lastActivity) {
			lastActivity = ts
		}
	}

	// Fall back to issue creation time when there is no history or comment
	// data — this can happen for issues created via direct database migration.
	if lastActivity.IsZero() {
		lastActivity = issueCreatedAt
	}
	if deferredAt.IsZero() {
		deferredAt = issueCreatedAt
	}

	return lastActivity, deferredAt, nil
}

// mostRecentRelationshipEventInvolving scans sourceID's history for the
// most recent EventRelationshipAdded or EventRelationshipRemoved entry whose
// "relationship" change field references targetID. Returns the zero time
// when no such entry exists.
func mostRecentRelationshipEventInvolving(ctx context.Context, uow driven.UnitOfWork, sourceID, targetID domain.ID) (time.Time, error) {
	entries, _, err := uow.History().ListHistory(ctx, sourceID, driven.HistoryFilter{}, -1)
	if err != nil {
		return time.Time{}, err
	}
	suffix := ":" + targetID.String()
	var latest time.Time
	for _, e := range entries {
		if e.EventType() != history.EventRelationshipAdded && e.EventType() != history.EventRelationshipRemoved {
			continue
		}
		for _, ch := range e.Changes() {
			if ch.Field != "relationship" {
				continue
			}
			val := ch.After
			if val == "" {
				val = ch.Before
			}
			if !endsWithIDSuffix(val, suffix) {
				continue
			}
			if e.Timestamp().After(latest) {
				latest = e.Timestamp()
			}
		}
	}
	return latest, nil
}

// endsWithIDSuffix reports whether val ends with suffix. Defined as a
// separate predicate to keep the scan readable and to allow for future
// tightening (e.g., excluding accidental ID-prefix matches).
func endsWithIDSuffix(val, suffix string) bool {
	return len(val) >= len(suffix) && val[len(val)-len(suffix):] == suffix
}

// stateChangeTargetIsDeferred reports whether an EventStateChanged history
// entry records a transition INTO the deferred state. transitionIssue
// always records a "state" FieldChange with After being the new state's
// String() form (see internal/core/impl.go where the change is appended).
func stateChangeTargetIsDeferred(e history.Entry) bool {
	for _, ch := range e.Changes() {
		if ch.Field == "state" && ch.After == domain.StateDeferred.String() {
			return true
		}
	}
	return false
}
