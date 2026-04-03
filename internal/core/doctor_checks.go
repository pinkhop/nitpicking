package core

import (
	"fmt"

	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// checkDefinition maps finding categories to a named diagnostic check. The
// registry lives in the service layer because it defines domain metadata —
// which categories belong to which named checks and what severity each
// carries. Driving adapters should not classify findings; they only format
// the classified output.
type checkDefinition struct {
	// Name is the check's identifier shown in output.
	Name string
	// Severity is the check's severity level, determining its ordering and
	// whether it runs at all given the minimum threshold.
	Severity driving.DoctorSeverity
	// Categories lists the finding categories that belong to this check.
	Categories []string
	// PassDetail is the human-readable message when the check passes.
	PassDetail string
}

// diagnosticChecks defines the ordered list of checks doctor performs, sorted
// by severity (error first, warning second, info third).
var diagnosticChecks = []checkDefinition{
	// Error-level checks.
	{
		Name:       "storage_integrity",
		Severity:   driving.SeverityError,
		Categories: []string{"storage_integrity"},
		PassDetail: "Integrity check passed",
	},
	{
		Name:       "blocker_health",
		Severity:   driving.SeverityError,
		Categories: []string{"blocker_cycle", "blocker_deleted", "blocker_deferred", "blocker_close_completed", "blocker_dead_end"},
		PassDetail: "No blocker graph issues found",
	},
	{
		Name:       "deleted_parents",
		Severity:   driving.SeverityError,
		Categories: []string{"deleted_parent"},
		PassDetail: "No issues reference deleted parents",
	},
	{
		Name:       "virtual_labels",
		Severity:   driving.SeverityError,
		Categories: []string{"virtual_label_in_table"},
		PassDetail: "No virtual labels stored in the labels table",
	},
	// Warning-level checks.
	{
		Name:       "stale_claims", // #nosec G101 -- not a credential; diagnostic check name
		Severity:   driving.SeverityWarning,
		Categories: []string{"stale_claim"},
		PassDetail: "No stale claims found",
	},
	{
		Name:       "close_completed", // #nosec G101 -- not a credential; diagnostic check name
		Severity:   driving.SeverityWarning,
		Categories: []string{"close_completed"},
		PassDetail: "No completed epics need closing",
	},
	{
		Name:       "priority_inversion",
		Severity:   driving.SeverityWarning,
		Categories: []string{"priority_inversion"},
		PassDetail: "No priority inversions found",
	},
	{
		Name:       "closed_parents", // #nosec G101 -- not a credential; diagnostic check name
		Severity:   driving.SeverityWarning,
		Categories: []string{"closed_parent"},
		PassDetail: "No open issues have closed parents",
	},
	{
		Name:       "overdue_deferrals",
		Severity:   driving.SeverityWarning,
		Categories: []string{"overdue_deferral"},
		PassDetail: "No overdue deferrals",
	},
	{
		Name:       "instructions",
		Severity:   driving.SeverityWarning,
		Categories: []string{"instructions"},
		PassDetail: "Agent instruction files reference np",
	},
	{
		Name:       "gitignore", // #nosec G101 -- not a credential; diagnostic check name
		Severity:   driving.SeverityWarning,
		Categories: []string{"gitignore"},
		PassDetail: ".np/ directory is ignored by git",
	},
	// Info-level checks.
	{
		Name:       "long_claims",
		Severity:   driving.SeverityInfo,
		Categories: []string{"long_claim"},
		PassDetail: "No unusually long-held claims",
	},
	{
		Name:       "orphan_tasks", // #nosec G101 -- not a credential; diagnostic check name
		Severity:   driving.SeverityInfo,
		Categories: []string{"orphan_task"},
		PassDetail: "All non-bug open tasks belong to an epic",
	},
	{
		Name:       "missing_labels",
		Severity:   driving.SeverityInfo,
		Categories: []string{"missing_label"},
		PassDetail: "All open issues have a kind label",
	},
	{
		Name:       "long_deferrals",
		Severity:   driving.SeverityInfo,
		Categories: []string{"long_deferral"},
		PassDetail: "No issues deferred for more than 1 week",
	},
	{
		Name:       "gc_recommended",
		Severity:   driving.SeverityInfo,
		Categories: []string{"gc_recommended"},
		PassDetail: "Deleted issue ratio is within threshold",
	},
	{
		Name:       "blocked_by_human",
		Severity:   driving.SeverityInfo,
		Categories: []string{"blocked_by_human"},
		PassDetail: "No issues waiting on human action",
	},
	{
		Name:       "multi_claim_authors",
		Severity:   driving.SeverityInfo,
		Categories: []string{"multi_claim_author"},
		PassDetail: "No authors have multiple active claims",
	},
}

// classifyFindings applies the check registry to a set of raw findings and a
// minimum severity threshold. It returns:
//   - checks: the pass/fail/skipped status of every registered check
//   - filtered: only findings whose categories belong to active checks
//   - healthy: true when no active findings exist
func classifyFindings(findings []driving.DoctorFinding, minSeverity driving.DoctorSeverity) (checks []driving.DoctorCheckResult, filtered []driving.DoctorFinding, healthy bool) {
	// Index findings by category for fast lookup.
	categoryFindings := make(map[string][]driving.DoctorFinding)
	for _, f := range findings {
		categoryFindings[f.Category] = append(categoryFindings[f.Category], f)
	}

	// Build the set of categories that belong to active (non-skipped) checks,
	// for filtering findings.
	activeCategories := make(map[string]bool)

	checks = make([]driving.DoctorCheckResult, 0, len(diagnosticChecks))
	for _, def := range diagnosticChecks {
		if def.Severity < minSeverity {
			checks = append(checks, driving.DoctorCheckResult{
				Name:   def.Name,
				Status: "skipped",
				Detail: fmt.Sprintf("skipped (%s-level check)", def.Severity),
			})
			continue
		}

		for _, cat := range def.Categories {
			activeCategories[cat] = true
		}

		var matched []driving.DoctorFinding
		for _, cat := range def.Categories {
			matched = append(matched, categoryFindings[cat]...)
		}

		if len(matched) == 0 {
			checks = append(checks, driving.DoctorCheckResult{
				Name:   def.Name,
				Status: "pass",
				Detail: def.PassDetail,
			})
			continue
		}

		// Use the first matched finding's message as the detail.
		checks = append(checks, driving.DoctorCheckResult{
			Name:   def.Name,
			Status: "fail",
			Detail: matched[0].Message,
		})
	}

	// Filter findings to only include those from active checks.
	filtered = make([]driving.DoctorFinding, 0, len(findings))
	for _, f := range findings {
		if activeCategories[f.Category] {
			filtered = append(filtered, f)
		}
	}

	healthy = len(filtered) == 0
	return checks, filtered, healthy
}

// CountSkippedChecks returns the number of checks with status "skipped".
func CountSkippedChecks(checks []driving.DoctorCheckResult) int {
	n := 0
	for _, c := range checks {
		if c.Status == "skipped" {
			n++
		}
	}
	return n
}

// SeverityBelow returns the label for the severity level immediately below
// the given threshold. Used in skip summary messages.
func SeverityBelow(threshold driving.DoctorSeverity) string {
	switch threshold {
	case driving.SeverityError:
		return "warning"
	case driving.SeverityWarning:
		return "info"
	default:
		return "info"
	}
}
