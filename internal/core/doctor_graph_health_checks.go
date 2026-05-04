package core

import (
	"context"
	"fmt"
	"slices"
	"sort"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driven"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// graphIssueData is the in-memory projection of an issue used by all five graph
// health checks. It is loaded once per check in a single read transaction.
type graphIssueData struct {
	ID         string
	Role       domain.Role
	State      domain.State
	Priority   domain.Priority
	ParentID   string // empty string when the issue has no parent
	BlockerIDs []string
}

// loadGraphIssues lists all non-deleted issues and projects them into
// graphIssueData slices. The map keys are issue ID strings for O(1) lookups.
func loadGraphIssues(ctx context.Context, uow driven.UnitOfWork) ([]graphIssueData, map[string]graphIssueData, error) {
	items, _, err := uow.Issues().ListIssues(ctx, driven.IssueFilter{}, driven.OrderByID, driven.SortAscending, -1)
	if err != nil {
		return nil, nil, fmt.Errorf("listing issues: %w", err)
	}

	issues := make([]graphIssueData, 0, len(items))
	byID := make(map[string]graphIssueData, len(items))

	for _, item := range items {
		// Convert BlockerIDs to strings and sort them. Sorting makes downstream
		// graph traversals (Tarjan SCC, cycle canonicalization) deterministic
		// regardless of the storage adapter's iteration order.
		blockers := make([]string, len(item.BlockerIDs))
		for i, bid := range item.BlockerIDs {
			blockers[i] = bid.String()
		}
		sort.Strings(blockers)

		parentID := ""
		if !item.ParentID.IsZero() {
			parentID = item.ParentID.String()
		}

		d := graphIssueData{
			ID:         item.ID.String(),
			Role:       item.Role,
			State:      item.State,
			Priority:   item.Priority,
			ParentID:   parentID,
			BlockerIDs: blockers,
		}
		issues = append(issues, d)
		byID[d.ID] = d
	}

	return issues, byID, nil
}

// runBlockedByAncestor detects issues blocked by issues in their own ancestor
// chain. For each issue with non-empty BlockerIDs, it walks the parent chain
// upward; any blocker that appears in that chain produces one row per
// (issue, ancestor) pair. Rows are sorted ascending by issue ID, then by
// blocking ancestor ID for deterministic output.
func runBlockedByAncestor(ctx context.Context, svc *serviceImpl, _ driving.DoctorInput) (*doctorRunResult, error) {
	if svc.tx == nil {
		return nil, fmt.Errorf("blocked-by-ancestor: database connection unavailable (cascade should have protected this check)")
	}

	var rows []driving.BlockedByAncestorRow

	err := svc.tx.WithReadTransaction(ctx, func(uow driven.UnitOfWork) error {
		_, byID, listErr := loadGraphIssues(ctx, uow)
		if listErr != nil {
			return listErr
		}

		// Walk each issue that has blockers.
		for id, issue := range byID {
			if len(issue.BlockerIDs) == 0 {
				continue
			}

			// Collect all ancestor IDs into a set. Detect parent-chain cycles
			// by breaking when the current node has already been added —
			// parent cycles are themselves a separate data integrity issue,
			// but this check must not infinite-loop on them.
			ancestors := make(map[string]struct{})
			cur := issue.ParentID
			for cur != "" {
				if _, alreadySeen := ancestors[cur]; alreadySeen {
					break
				}
				ancestors[cur] = struct{}{}
				if parent, ok := byID[cur]; ok {
					cur = parent.ParentID
				} else {
					break
				}
			}

			if len(ancestors) == 0 {
				continue
			}

			// Emit one row per blocker that is an ancestor.
			for _, blockerID := range issue.BlockerIDs {
				if _, isAncestor := ancestors[blockerID]; isAncestor {
					rows = append(rows, driving.BlockedByAncestorRow{
						Issue:            id,
						BlockingAncestor: blockerID,
					})
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("blocked-by-ancestor: %w", err)
	}

	if len(rows) == 0 {
		return nil, nil
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Issue != rows[j].Issue {
			return rows[i].Issue < rows[j].Issue
		}
		return rows[i].BlockingAncestor < rows[j].BlockingAncestor
	})

	affected := make([]any, len(rows))
	for i, r := range rows {
		affected[i] = r
	}
	return &doctorRunResult{
		Summary:  fmt.Sprintf("%d issue(s) blocked by an ancestor", len(rows)),
		Affected: affected,
	}, nil
}

// runBlockedByClosableIssue detects issues blocked by "closable" parents —
// issues in state open whose children are all in state closed. For each issue
// with a blocker in the closable set, one row is emitted per (issue, closable
// blocker) pair. result.Meta is set to a bool indicating whether any closable
// blocker is a task (used by the registry's FixFn to append --include-tasks).
func runBlockedByClosableIssue(ctx context.Context, svc *serviceImpl, _ driving.DoctorInput) (*doctorRunResult, error) {
	if svc.tx == nil {
		return nil, fmt.Errorf("blocked-by-closable-issue: database connection unavailable (cascade should have protected this check)")
	}

	var rows []driving.BlockedByClosableIssueRow
	var anyTaskBlocker bool

	err := svc.tx.WithReadTransaction(ctx, func(uow driven.UnitOfWork) error {
		issues, byID, listErr := loadGraphIssues(ctx, uow)
		if listErr != nil {
			return listErr
		}

		// Compute the "closable" set: open issues with at least one child,
		// all of whose children are closed.
		closable := computeClosableSet(issues, byID)

		// For each issue with a blocker in the closable set, emit a finding.
		for _, issue := range issues {
			for _, blockerID := range issue.BlockerIDs {
				if _, isClosable := closable[blockerID]; !isClosable {
					continue
				}
				rows = append(rows, driving.BlockedByClosableIssueRow{
					Issue:           issue.ID,
					ClosableBlocker: blockerID,
				})
				if blocker, ok := byID[blockerID]; ok && blocker.Role == domain.RoleTask {
					anyTaskBlocker = true
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("blocked-by-closable-issue: %w", err)
	}

	if len(rows) == 0 {
		return nil, nil
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Issue != rows[j].Issue {
			return rows[i].Issue < rows[j].Issue
		}
		return rows[i].ClosableBlocker < rows[j].ClosableBlocker
	})

	affected := make([]any, len(rows))
	for i, r := range rows {
		affected[i] = r
	}
	return &doctorRunResult{
		Summary:  fmt.Sprintf("%d issue(s) blocked by a closable issue", len(rows)),
		Affected: affected,
		Meta:     anyTaskBlocker,
	}, nil
}

// computeClosableSet returns the set of issue IDs that are "closable": open
// issues that have at least one child and whose children are all closed. The
// "at least one child" gate is intentional and matches the spec: a leaf issue
// (no children) is never "closable" via the close-completed mechanism — there
// is nothing to be completed by — so it must not appear in this set even when
// it satisfies the trivial "all children are closed" predicate.
func computeClosableSet(issues []graphIssueData, byID map[string]graphIssueData) map[string]struct{} {
	// childStates maps parentID → map of child state → count.
	type childInfo struct {
		total  int
		closed int
	}
	children := make(map[string]childInfo)
	for _, issue := range issues {
		if issue.ParentID == "" {
			continue
		}
		info := children[issue.ParentID]
		info.total++
		if issue.State == domain.StateClosed {
			info.closed++
		}
		children[issue.ParentID] = info
	}

	closable := make(map[string]struct{})
	for parentID, info := range children {
		if info.total == 0 || info.closed != info.total {
			continue
		}
		parent, ok := byID[parentID]
		if !ok {
			continue
		}
		if parent.State == domain.StateOpen {
			closable[parentID] = struct{}{}
		}
	}
	return closable
}

// runBlockedByDeferredIssue detects issues blocked by deferred issues. For each
// issue with a blocker in state deferred, one row is emitted per (issue,
// deferred blocker) pair. Rows are sorted ascending by issue ID, then blocker ID.
func runBlockedByDeferredIssue(ctx context.Context, svc *serviceImpl, _ driving.DoctorInput) (*doctorRunResult, error) {
	if svc.tx == nil {
		return nil, fmt.Errorf("blocked-by-deferred-issue: database connection unavailable (cascade should have protected this check)")
	}

	var rows []driving.BlockedByDeferredIssueRow

	err := svc.tx.WithReadTransaction(ctx, func(uow driven.UnitOfWork) error {
		issues, byID, listErr := loadGraphIssues(ctx, uow)
		if listErr != nil {
			return listErr
		}

		for _, issue := range issues {
			for _, blockerID := range issue.BlockerIDs {
				blocker, ok := byID[blockerID]
				if !ok {
					continue
				}
				if blocker.State == domain.StateDeferred {
					rows = append(rows, driving.BlockedByDeferredIssueRow{
						Issue:   issue.ID,
						Blocker: blockerID,
					})
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("blocked-by-deferred-issue: %w", err)
	}

	if len(rows) == 0 {
		return nil, nil
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Issue != rows[j].Issue {
			return rows[i].Issue < rows[j].Issue
		}
		return rows[i].Blocker < rows[j].Blocker
	})

	affected := make([]any, len(rows))
	for i, r := range rows {
		affected[i] = r
	}
	return &doctorRunResult{
		Summary:  fmt.Sprintf("%d issue(s) blocked by a deferred issue", len(rows)),
		Affected: affected,
	}, nil
}

// runBlockerCycles detects cycles in the blocked-by graph using Tarjan's
// strongly connected components algorithm. Self-loops are detected separately
// before the SCC pass. Each cycle is reported as one row with the issue IDs in
// canonical order: starting with the lowest-ID issue in the cycle, then
// following blocked_by edges around the loop.
func runBlockerCycles(ctx context.Context, svc *serviceImpl, _ driving.DoctorInput) (*doctorRunResult, error) {
	if svc.tx == nil {
		return nil, fmt.Errorf("blocker-cycles: database connection unavailable (cascade should have protected this check)")
	}

	var rows []driving.BlockerCycleRow

	err := svc.tx.WithReadTransaction(ctx, func(uow driven.UnitOfWork) error {
		issues, byID, listErr := loadGraphIssues(ctx, uow)
		if listErr != nil {
			return listErr
		}

		// Self-loops: an issue with itself in BlockerIDs.
		// domain.NewRelationship rejects self-relationships, but corrupt data
		// could contain them; the doctor check must detect them.
		for _, issue := range issues {
			if slices.Contains(issue.BlockerIDs, issue.ID) {
				rows = append(rows, driving.BlockerCycleRow{
					Cycle: []string{issue.ID},
				})
			}
		}

		// Build adjacency: id → blockers (excluding self-loops already handled).
		adj := make(map[string][]string, len(byID))
		for _, issue := range issues {
			for _, blockerID := range issue.BlockerIDs {
				if blockerID == issue.ID {
					continue // self-loops handled above
				}
				adj[issue.ID] = append(adj[issue.ID], blockerID)
			}
		}

		// Tarjan's SCC on the blocked-by graph.
		cycles := tarjanSCC(adj)
		for _, scc := range cycles {
			if len(scc) < 2 {
				continue
			}
			rows = append(rows, driving.BlockerCycleRow{
				Cycle: canonicalizeCycle(scc, adj),
			})
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("blocker-cycles: %w", err)
	}

	if len(rows) == 0 {
		return nil, nil
	}

	// Sort rows by the first element of each cycle for deterministic output.
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Cycle[0] < rows[j].Cycle[0]
	})

	affected := make([]any, len(rows))
	for i, r := range rows {
		affected[i] = r
	}
	return &doctorRunResult{
		Summary:  fmt.Sprintf("%d blocker cycle(s) detected", len(rows)),
		Affected: affected,
	}, nil
}

// tarjanSCC runs Tarjan's strongly connected components algorithm on the
// directed graph described by adj and returns all SCCs of size ≥ 2 (true
// cycles, not counting self-loops which are pre-handled). The adjacency map
// should only contain nodes reachable from the issue list.
func tarjanSCC(adj map[string][]string) [][]string {
	index := make(map[string]int)
	lowlink := make(map[string]int)
	onStack := make(map[string]bool)
	var stack []string
	counter := 0
	var sccs [][]string

	var strongconnect func(v string)
	strongconnect = func(v string) {
		index[v] = counter
		lowlink[v] = counter
		counter++
		stack = append(stack, v)
		onStack[v] = true

		for _, w := range adj[v] {
			if _, visited := index[w]; !visited {
				strongconnect(w)
				if lowlink[w] < lowlink[v] {
					lowlink[v] = lowlink[w]
				}
			} else if onStack[w] {
				if index[w] < lowlink[v] {
					lowlink[v] = index[w]
				}
			}
		}

		// v is a root node; pop the SCC.
		if lowlink[v] == index[v] {
			var scc []string
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				scc = append(scc, w)
				if w == v {
					break
				}
			}
			sccs = append(sccs, scc)
		}
	}

	// Visit nodes in sorted order for deterministic behaviour across calls.
	nodes := make([]string, 0, len(adj))
	for n := range adj {
		nodes = append(nodes, n)
	}
	sort.Strings(nodes)

	for _, v := range nodes {
		if _, visited := index[v]; !visited {
			strongconnect(v)
		}
	}

	return sccs
}

// canonicalizeCycle returns the cycle members as an ordered slice starting
// with the lowest-ID member and following blocked_by edges around the loop.
// scc is an unordered set of issue IDs forming the cycle; adj is the full
// blocked-by adjacency map (with edge lists pre-sorted by loadGraphIssues).
//
// Edge-faithfulness applies only to the prefix produced by the greedy walk:
// for a simple cycle the entire result is the Hamiltonian order, but in a
// non-simple SCC the greedy walk may get stuck before visiting every member.
// Any unvisited members are then appended in sorted-ID order so the output is
// always complete. Consumers that render a cycle as "X blocked_by Y blocked_by
// Z..." therefore lie for the appended portion when the SCC is not a simple
// cycle; the spec's single-cycle abstraction is a best-effort projection of an
// arbitrary SCC, not a structural guarantee. The greedy walk's edge-pick is
// deterministic because BlockerIDs is sorted upstream, so adj's edge lists
// carry stable order across runs.
func canonicalizeCycle(scc []string, adj map[string][]string) []string {
	sccSet := make(map[string]struct{}, len(scc))
	for _, id := range scc {
		sccSet[id] = struct{}{}
	}

	// Find the minimum-ID node in the SCC.
	minID := scc[0]
	for _, id := range scc[1:] {
		if id < minID {
			minID = id
		}
	}

	// Walk blocked_by edges from minID around the cycle.
	result := make([]string, 0, len(scc))
	cur := minID
	visited := make(map[string]bool, len(scc))
	for len(result) < len(scc) {
		result = append(result, cur)
		visited[cur] = true
		nextFound := false
		for _, neighbor := range adj[cur] {
			if _, inSCC := sccSet[neighbor]; !inSCC {
				continue
			}
			if !visited[neighbor] {
				cur = neighbor
				nextFound = true
				break
			}
		}
		if !nextFound {
			break
		}
	}

	// Completeness: append any SCC members the greedy walk missed, in
	// sorted-ID order. Without this, a non-simple SCC could silently
	// truncate to a subcycle and the user would not see all entangled issues.
	if len(result) < len(scc) {
		missed := make([]string, 0, len(scc)-len(result))
		for _, id := range scc {
			if !visited[id] {
				missed = append(missed, id)
			}
		}
		sort.Strings(missed)
		result = append(result, missed...)
	}

	return result
}

// runPriorityInversions detects child issues whose priority is strictly higher
// than their parent's priority. P0 > P1 > P2 > P3; "higher" means a smaller
// integer value. One row per (child, parent) pair with strict inequality.
// Rows are sorted ascending by child ID.
func runPriorityInversions(ctx context.Context, svc *serviceImpl, _ driving.DoctorInput) (*doctorRunResult, error) {
	if svc.tx == nil {
		return nil, fmt.Errorf("priority-inversions: database connection unavailable (cascade should have protected this check)")
	}

	var rows []driving.PriorityInversionRow

	err := svc.tx.WithReadTransaction(ctx, func(uow driven.UnitOfWork) error {
		issues, byID, listErr := loadGraphIssues(ctx, uow)
		if listErr != nil {
			return listErr
		}

		for _, issue := range issues {
			if issue.ParentID == "" {
				continue
			}
			parent, ok := byID[issue.ParentID]
			if !ok {
				continue
			}
			// Smaller integer = higher priority (P0=1, P1=2, P2=3, P3=4).
			if int(issue.Priority) < int(parent.Priority) {
				rows = append(rows, driving.PriorityInversionRow{
					Issue:          issue.ID,
					Parent:         parent.ID,
					ChildPriority:  issue.Priority.String(),
					ParentPriority: parent.Priority.String(),
				})
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("priority-inversions: %w", err)
	}

	if len(rows) == 0 {
		return nil, nil
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Issue < rows[j].Issue
	})

	affected := make([]any, len(rows))
	for i, r := range rows {
		affected[i] = r
	}
	return &doctorRunResult{
		Summary:  fmt.Sprintf("%d priority inversion(s) detected", len(rows)),
		Affected: affected,
	}, nil
}
