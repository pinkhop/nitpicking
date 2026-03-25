package cmdutil

import (
	"context"
	"fmt"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
)

// ClaimIssueResolver resolves an issue ID from either an explicit issue ID
// argument, a claim ID (by looking up the claim's associated issue), or both.
// When both are provided, it validates they refer to the same issue.
type ClaimIssueResolver struct {
	svc      service.Service
	resolver *IDResolver
}

// NewClaimIssueResolver creates a resolver backed by the given service and ID
// resolver.
func NewClaimIssueResolver(svc service.Service, resolver *IDResolver) *ClaimIssueResolver {
	return &ClaimIssueResolver{svc: svc, resolver: resolver}
}

// Resolve determines the issue ID from the provided arguments. The caller
// provides rawIssueID (from a positional argument or --issue flag) and claimID
// (from --claim flag). At least one must be non-empty.
//
// Three cases:
//   - Only claimID provided: looks up the claim and returns its issue ID.
//   - Only rawIssueID provided: parses and returns the issue ID.
//   - Both provided: parses the issue ID, looks up the claim, and returns an
//     error if they disagree.
func (r *ClaimIssueResolver) Resolve(ctx context.Context, rawIssueID, claimID string) (issue.ID, error) {
	hasIssue := rawIssueID != ""
	hasClaim := claimID != ""

	if !hasIssue && !hasClaim {
		return issue.ID{}, fmt.Errorf("either an issue ID or --claim must be provided")
	}

	if hasIssue && !hasClaim {
		return r.resolver.Resolve(ctx, rawIssueID)
	}

	claimIssueID, err := r.svc.LookupClaimIssueID(ctx, claimID)
	if err != nil {
		return issue.ID{}, fmt.Errorf("looking up claim: %w", err)
	}

	if !hasIssue {
		return claimIssueID, nil
	}

	// Both provided — verify they agree.
	explicitID, err := r.resolver.Resolve(ctx, rawIssueID)
	if err != nil {
		return issue.ID{}, err
	}

	if explicitID != claimIssueID {
		return issue.ID{}, fmt.Errorf(
			"issue ID %s does not match claim's issue %s",
			explicitID, claimIssueID,
		)
	}

	return explicitID, nil
}
