package core

import (
	"context"
	"fmt"

	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// doctorCategory groups related doctor checks in the output.
type doctorCategory string

const (
	categoryDatabase       doctorCategory = "Database"
	categoryEnvironment    doctorCategory = "Environment"
	categoryGraphHealth    doctorCategory = "Graph health"
	categoryIssueLifecycle doctorCategory = "Issue lifecycle"
)

// doctorRunResult carries the findings from a single check execution.
// A nil result means the check passed with no findings.
type doctorRunResult struct {
	// Summary is the human-readable "N issues found" sentence for this run.
	Summary string
	// Affected holds per-check typed affected-row structs as defined in dto.go.
	// Nil for checks that have no per-issue row schema.
	Affected []any
	// NotApplicable, when true, signals that the check is not relevant to this
	// environment and must be silently omitted from all output lists (Passed,
	// Warnings, Errors, and Skipped). Used by the git-ignore check when the
	// workspace is not inside a git repository.
	NotApplicable bool
	// Meta carries check-specific metadata that FixFn (on the registry entry)
	// may inspect to compute a dynamic Fix. Nil for checks that use a static Fix.
	Meta any
}

// doctorCheckEntry describes a single diagnostic check in the doctor registry.
type doctorCheckEntry struct {
	Slug         string
	Title        string
	Category     doctorCategory
	Severity     driving.DoctorSeverity
	Description  string
	WhyItMatters string
	Fix          driving.DoctorFix
	// FixFn, when non-nil, computes the Fix dynamically from the run result and
	// overrides Fix. Used by checks whose remediation command depends on the
	// specific data found (e.g., whether --include-tasks is needed). The
	// result.Meta field carries check-specific metadata for FixFn's use.
	FixFn func(result *doctorRunResult) driving.DoctorFix
	// DependsOn lists cascade-prerequisite slugs. This check is skipped when
	// any slug in the list is in the cascade-blocked set (either failed or was
	// itself skipped). The root-cause slug propagates transitively.
	DependsOn []string
	// Run executes the check. A nil result indicates the check passed.
	// The svc parameter provides access to the transactor and other service
	// dependencies; stub implementations return (nil, nil) without using it.
	// The input carries per-call parameters such as LongDeferralThreshold.
	Run func(ctx context.Context, svc *serviceImpl, input driving.DoctorInput) (*doctorRunResult, error)
	// SkipsAll, when non-nil, is invoked after a non-nil Run result is
	// observed for an entry whose Severity is SeverityError. It returns true
	// when the findings indicate that ALL remaining checks should be skipped,
	// including those with no DependsOn entries. Used by the schema-version
	// check to implement the newer-database case, which skips even Environment
	// checks. SkipsAll is not consulted for warning-severity findings.
	SkipsAll func(result *doctorRunResult) bool
}

// doctorSchemaNewerDBMarker is included in the Affected list of schema-version
// findings when the database version is strictly newer than the binary version.
// The schema-version entry's SkipsAll function detects it to activate the
// total-skip path. Stub implementations never return this marker.
type doctorSchemaNewerDBMarker struct{}

// schemaVersionSkipsAll returns true when the schema-version result contains
// a doctorSchemaNewerDBMarker, indicating the database is newer than the
// binary. In that case the orchestrator must skip all remaining checks,
// including Environment checks that declare no DependsOn prerequisites.
func schemaVersionSkipsAll(result *doctorRunResult) bool {
	for _, a := range result.Affected {
		if _, ok := a.(doctorSchemaNewerDBMarker); ok {
			return true
		}
	}
	return false
}

// closeCompletedFixFn is the shared FixFn for registry entries whose fix is
// `np epic close-completed`. It appends --include-tasks when result.Meta is
// a bool set to true, signalling that at least one closable issue is a task.
// result must be non-nil; the doctor orchestrator guarantees this for all FixFn calls.
func closeCompletedFixFn(result *doctorRunResult) driving.DoctorFix {
	if hasTask, ok := result.Meta.(bool); ok && hasTask {
		return driving.DoctorFix{Command: "np epic close-completed --include-tasks"}
	}
	return driving.DoctorFix{Command: "np epic close-completed"}
}

// doctorRegistry returns the 16-entry doctor check registry in canonical
// display order: Database (cascade-five first, then alphabetical within
// Database), Environment, Graph health, Issue lifecycle. All Run
// implementations are stubs; per-category child tasks replace them.
func doctorRegistry() []doctorCheckEntry {
	return []doctorCheckEntry{
		// ── Database: cascade prerequisites (must remain in this order) ───────
		{
			Slug:         "dot-np-directory",
			Title:        ".np/ directory",
			Category:     categoryDatabase,
			Severity:     driving.SeverityError,
			Description:  "Confirms a `.np/` directory exists in the current working directory or an ancestor.",
			WhyItMatters: "Without a `.np/` directory, np has no database to operate on; no Database, Graph health, or Issue lifecycle check can run.",
			Fix: driving.DoctorFix{
				Instructions: "Run `np init` from the directory you want to root the workspace at. This creates a `.np/` directory and a fresh database.",
			},
			DependsOn: nil,
			Run:       runDotNpDirectory,
		},
		{
			Slug:         "database-exists",
			Title:        "Database exists",
			Category:     categoryDatabase,
			Severity:     driving.SeverityError,
			Description:  "Confirms the database file inside `.np/` exists and can be opened.",
			WhyItMatters: "Without a readable database file, no check that reads issue data can run; early detection lets the user restore from a backup before further damage.",
			Fix: driving.DoctorFix{
				Instructions: "Restore the `.np/` directory from a backup (`np admin backup` archives are gzip-compressed JSONL). As a last resort, run `np init` to create a fresh database — note that this is destructive and discards any existing data in `.np/`.",
			},
			DependsOn: []string{"dot-np-directory"},
			Run:       runDatabaseExists,
		},
		{
			Slug:         "storage-integrity",
			Title:        "Storage integrity",
			Category:     categoryDatabase,
			Severity:     driving.SeverityError,
			Description:  "Verifies the SQLite file is structurally intact.",
			WhyItMatters: "Storage corruption can cause issues to vanish or appear in inconsistent states; early detection enables backup-and-restore before damage compounds.",
			Fix: driving.DoctorFix{
				Instructions: "Back up the .np/ directory immediately (`np admin backup` or `cp -a`). Investigate the source of corruption — disk errors, abrupt termination during writes, or concurrent access by an outdated np binary are common causes. If a recent backup exists, restore from it after preserving the corrupted state for diagnosis.",
			},
			DependsOn: []string{"database-exists"},
			Run:       runStorageIntegrity,
		},
		{
			// schema-version has two distinct error cases:
			//   newer DB (binary < database) → SkipsAll returns true, every
			//     remaining check is skipped including Environment.
			//   older DB (database < binary) → SkipsAll returns false, nothing
			//     extra is skipped; remaining checks run on best-effort basis.
			// The Fix below covers both cases textually because Fix is sourced
			// from the registry entry; per-case Fix variation is a future
			// concern for the real Run implementation.
			Slug:         "schema-version",
			Title:        "Schema version",
			Category:     categoryDatabase,
			Severity:     driving.SeverityError,
			Description:  "Checks that the database schema is current and supported by this version of np.",
			WhyItMatters: "np will reject most commands until the database is migrated to the current schema.",
			Fix: driving.DoctorFix{
				Instructions: "If the database is older than the binary: run `np admin upgrade`. If the binary is older than the database: upgrade the np binary to a version that supports the database's schema version.",
			},
			DependsOn: []string{"storage-integrity"},
			Run:       runSchemaVersion,
			SkipsAll:  schemaVersionSkipsAll,
		},
		{
			// column-data-validity depends on storage-integrity (not schema-version)
			// so that it still runs when schema-version reports an older DB — the
			// spec says the older-DB case skips nothing outright.
			Slug:         "column-data-validity",
			Title:        "Column data validity",
			Category:     categoryDatabase,
			Severity:     driving.SeverityError,
			Description:  "Verifies that columns expected to hold typed data parse correctly.",
			WhyItMatters: "Data that has the right SQLite storage class but does not parse as the expected domain type — timestamps, enums, JSON-encoded blobs — produces silent errors and unpredictable behaviour in commands that read those columns.",
			Fix: driving.DoctorFix{
				Instructions: "Back up the .np/ directory immediately (`np admin backup` or `cp -a`). Investigate the source of the malformed value — common causes include manual edits to the SQLite file, partial writes from a crash, or data inserted by a binary running at a different schema version. If a recent backup exists, restore from it after preserving the malformed state for diagnosis.",
			},
			DependsOn: []string{"storage-integrity"},
			Run:       runColumnDataValidity,
		},
		// ── Database: remaining checks (alphabetical by title) ────────────────
		{
			Slug:         "closed-parent-with-open-child",
			Title:        "Closed parent with open child",
			Category:     categoryDatabase,
			Severity:     driving.SeverityError,
			Description:  "Detects closed issues that still have open or deferred children.",
			WhyItMatters: "This is a data integrity error. Commands that touch these issues may produce unpredictable output, and the closed state of the parent can no longer be trusted to mean its work is done.",
			Fix: driving.DoctorFix{
				Instructions: "Either reopen the parent (`np issue reopen <PARENT-ID> --author <name>`) if its work is not actually done, or close each non-closed child if it has been completed. The choice depends on the true state of the work — neither path is universally correct.",
			},
			DependsOn: []string{"column-data-validity"},
			Run:       runClosedParentWithOpenChild,
		},
		{
			Slug:         "invalid-parent-reference",
			Title:        "Invalid parent reference",
			Category:     categoryDatabase,
			Severity:     driving.SeverityError,
			Description:  "Detects issues that have a parent relationship, but the parent issue does not exist.",
			WhyItMatters: "This is a data integrity error. Commands that touch this issue may produce unpredictable output, and operations walking the parent chain may propagate the corruption.",
			Fix: driving.DoctorFix{
				Command: "np admin fix invalid-parent-reference --author <name>",
			},
			DependsOn: []string{"column-data-validity"},
			Run:       runInvalidParentReference,
		},
		// ── Environment (alphabetical by title) ──────────────────────────────
		{
			Slug:         "agent-instructions",
			Title:        "Agent instructions",
			Category:     categoryEnvironment,
			Severity:     driving.SeverityWarning,
			Description:  "Confirms an agent instruction file (CLAUDE.md or AGENTS.md) exists and references np.",
			WhyItMatters: "Without instructions, AI agents won't know to use np and will fall back to ad-hoc tracking that doesn't persist.",
			Fix: driving.DoctorFix{
				Instructions: "Edit your CLAUDE.md or AGENTS.md to include a reference to np. The output of `np agent prime` provides a complete, ready-to-paste section that explains the tool to AI agents.",
			},
			DependsOn: nil,
			Run:       runAgentInstructions,
		},
		{
			Slug:         "git-ignore",
			Title:        "Git ignore",
			Category:     categoryEnvironment,
			Severity:     driving.SeverityWarning,
			Description:  "Confirms the .np/ directory is excluded from git so the issue database is not committed.",
			WhyItMatters: "Committing the database to git creates merge conflicts, leaks local state to teammates, and bloats the repository.",
			Fix: driving.DoctorFix{
				Command: "np admin fix git-ignore",
			},
			DependsOn: nil,
			Run:       runGitIgnore,
		},
		// ── Graph health (alphabetical by title) ─────────────────────────────
		{
			Slug:         "blocked-by-ancestor",
			Title:        "Blocked by ancestor",
			Category:     categoryGraphHealth,
			Severity:     driving.SeverityWarning,
			Description:  "Detects issues blocked by their parent or grandparent, which creates an unresolvable dependency.",
			WhyItMatters: "Ancestor blocks form a closed loop: the issue cannot become ready until its ancestor closes, the ancestor cannot close while this issue is still open, and this issue cannot close until it is claimed and worked. No part of the loop can advance.",
			Fix: driving.DoctorFix{
				Instructions: "Either restructure the parent chain so the blocking issue is no longer an ancestor (claim the issue and update its `parent` via `np json update`), or remove the offending blocked_by relationship (`np rel blocks unblock <source> <target> --author <name>`). The right path depends on the intent that was being modelled.",
			},
			DependsOn: []string{"column-data-validity"},
			Run:       runBlockedByAncestor,
		},
		{
			// FixFn computes the command dynamically: --include-tasks is appended
			// when any closable blocker is a task, so the fix is actionable as-is.
			Slug:         "blocked-by-closable-issue",
			Title:        "Blocked by closable issue",
			Category:     categoryGraphHealth,
			Severity:     driving.SeverityWarning,
			Description:  "Detects issues blocked by an epic or parent task whose children are all closed.",
			WhyItMatters: "The blocker is already effectively done — its children are all closed — but it still registers as an open dependency. Downstream work is held back by an issue that's just waiting to be closed.",
			// Static Fix.Command preserves the `[--include-tasks]` placeholder
			// because callers that surface registry entries directly (e.g., a
			// future "list all checks" command) need to convey the conditional
			// flag. FixFn always overrides this when a real finding fires.
			Fix: driving.DoctorFix{
				Command: "np epic close-completed [--include-tasks]",
			},
			FixFn:     closeCompletedFixFn,
			DependsOn: []string{"column-data-validity"},
			Run:       runBlockedByClosableIssue,
		},
		{
			Slug:         "blocked-by-deferred-issue",
			Title:        "Blocked by deferred issue",
			Category:     categoryGraphHealth,
			Severity:     driving.SeverityWarning,
			Description:  "Detects issues blocked by issues that have been shelved.",
			WhyItMatters: "Until the deferred blocker is resumed and closed, the blocked work cannot advance at all; the deferral effectively freezes downstream progress.",
			Fix: driving.DoctorFix{
				Instructions: "Claim one of the deferred blocking issues, complete it, then close it. The blocked issues will become ready once the blocker is closed. If the deferred work is no longer relevant, undefer it long enough to close it instead.",
			},
			DependsOn: []string{"column-data-validity"},
			Run:       runBlockedByDeferredIssue,
		},
		{
			Slug:         "blocker-cycles",
			Title:        "Blocker cycles",
			Category:     categoryGraphHealth,
			Severity:     driving.SeverityWarning,
			Description:  "Detects cycles in the blocked-by graph where issues mutually block each other.",
			WhyItMatters: "No issue in the cycle can become ready, because each one waits on another in the loop. The cycle must be broken by removing a blocked-by relationship before any of them can advance.",
			Fix: driving.DoctorFix{
				Instructions: "Decide which blocked_by relationship in the cycle is least essential and remove it: `np rel blocks unblock <source> <target> --author <name>`. Removing one edge breaks the cycle and lets the remaining issues progress.",
			},
			DependsOn: []string{"column-data-validity"},
			Run:       runBlockerCycles,
		},
		{
			Slug:         "priority-inversions",
			Title:        "Priority inversions",
			Category:     categoryGraphHealth,
			Severity:     driving.SeverityWarning,
			Description:  "Detects child issues with higher priority than their parent.",
			WhyItMatters: "A child with higher priority than its parent usually means the parent's priority underestimates the importance of its own work — the parent should match or exceed its highest-priority child.",
			Fix: driving.DoctorFix{
				Instructions: "Either raise the parent's priority to match or exceed the child's (claim the parent and use `np json update --claim <CID>` with `priority`), or lower the child's priority if it was set too high. Choose based on which priority reflects the work's actual urgency.",
			},
			DependsOn: []string{"column-data-validity"},
			Run:       runPriorityInversions,
		},
		// ── Issue lifecycle (alphabetical by title) ───────────────────────────
		{
			// FixFn computes the command dynamically: --include-tasks is appended
			// when any closable parent is a task, so the fix is actionable as-is.
			Slug:         "closable-parent-issues",
			Title:        "Closable parent issues",
			Category:     categoryIssueLifecycle,
			Severity:     driving.SeverityWarning,
			Description:  "Detects issues whose children are all closed and can therefore be closed to acknowledge the work is done.",
			WhyItMatters: "Leaving completed parents open inflates active-work queries with stale entries and obscures real progress.",
			// Static Fix.Command preserves the `[--include-tasks]` placeholder
			// for callers that surface registry entries directly. FixFn overrides
			// this when a real finding fires.
			Fix: driving.DoctorFix{
				Command: "np epic close-completed [--include-tasks]",
			},
			FixFn:     closeCompletedFixFn,
			DependsOn: []string{"column-data-validity"},
			Run:       runClosableParentIssues,
		},
		{
			Slug:         "long-deferrals",
			Title:        "Long deferrals",
			Category:     categoryIssueLifecycle,
			Severity:     driving.SeverityWarning,
			Description:  "Detects issues that have been deferred and not updated for an excessive period.",
			WhyItMatters: "Long-untouched deferred issues are usually forgotten work or implicit decisions to abandon — periodic review either resurrects them or closes them honestly.",
			Fix: driving.DoctorFix{
				Instructions: "Review each long-deferred issue. If the work is still relevant, undefer it (`np issue undefer --claim <CID>`) and either work on it or set a new deferral. If the work is no longer relevant, claim and close it (`np close --claim <CID> --reason \"...\"`).",
			},
			DependsOn: []string{"column-data-validity"},
			Run:       runLongDeferrals,
		},
	}
}

// doctorDisplayOrder returns the 16 check slugs in the canonical display
// order: Database (cascade-five first, then alphabetical within Database),
// Environment, Graph health, Issue lifecycle — alphabetical within each
// non-cascade group. This order is stable across runs and identical in
// both verbose listing and JSON passed array.
func doctorDisplayOrder() []string {
	registry := doctorRegistry()
	slugs := make([]string, len(registry))
	for i, e := range registry {
		slugs[i] = e.Slug
	}
	return slugs
}

// runDoctorChecks executes the doctor checks in registry order, applying
// cascade-skip semantics. It returns a DoctorOutput with Errors, Warnings,
// Passed, and Skipped populated from the check results. MinSeverity from input
// is not applied here — display filtering is the CLI renderer's responsibility.
func runDoctorChecks(ctx context.Context, svc *serviceImpl, registry []doctorCheckEntry, input driving.DoctorInput) (driving.DoctorOutput, error) {
	// cascadeBlocked maps slug → root-cause-slug. A slug is added when it
	// emits error findings (root cause = itself) or when it is skipped because
	// a DependsOn prerequisite is in the set (root cause propagates from the
	// prerequisite). SkipsAll checks also add to this set.
	cascadeBlocked := make(map[string]string)

	// skipAllPrereq is non-empty when a check with SkipsAll active has fired.
	// All subsequent checks are skipped regardless of DependsOn.
	skipAllPrereq := ""

	out := driving.DoctorOutput{
		Errors:   []driving.DoctorFinding{},
		Warnings: []driving.DoctorFinding{},
	}

	for _, entry := range registry {
		// When SkipsAll is active, every remaining check is skipped.
		if skipAllPrereq != "" {
			out.Skipped = append(out.Skipped, driving.DoctorSkippedCheck{
				Check:        entry.Slug,
				Description:  entry.Description,
				Prerequisite: skipAllPrereq,
			})
			cascadeBlocked[entry.Slug] = skipAllPrereq
			continue
		}

		// Skip when any direct prerequisite is in the cascade-blocked set.
		// The root-cause slug propagates transitively through the blocked set.
		if rootCause, blocked := firstBlockedPrereq(entry.DependsOn, cascadeBlocked); blocked {
			out.Skipped = append(out.Skipped, driving.DoctorSkippedCheck{
				Check:        entry.Slug,
				Description:  entry.Description,
				Prerequisite: rootCause,
			})
			cascadeBlocked[entry.Slug] = rootCause
			continue
		}

		result, err := entry.Run(ctx, svc, input)
		if err != nil {
			return driving.DoctorOutput{}, fmt.Errorf("doctor check %q: %w", entry.Slug, err)
		}

		if result == nil {
			// Check passed: no findings.
			out.Passed = append(out.Passed, driving.DoctorPassedCheck{
				Check:       entry.Slug,
				Description: entry.Description,
			})
			continue
		}

		if result.NotApplicable {
			// Check is irrelevant in this environment (e.g., git-ignore outside a
			// git repository). Silently omit from all output lists — not added to
			// Passed, Warnings, Errors, or Skipped.
			continue
		}

		// Resolve the fix: FixFn takes precedence over the static Fix field.
		fix := entry.Fix
		if entry.FixFn != nil {
			fix = entry.FixFn(result)
		}

		// Check has findings — classify by severity.
		finding := driving.DoctorFinding{
			Check:        entry.Slug,
			Description:  entry.Description,
			WhyItMatters: entry.WhyItMatters,
			Summary:      result.Summary,
			Affected:     result.Affected,
			Fix:          fix,
		}

		switch entry.Severity {
		case driving.SeverityError:
			out.Errors = append(out.Errors, finding)
			// Mark as cascade root cause.
			cascadeBlocked[entry.Slug] = entry.Slug
			// Check whether this error should skip all remaining checks.
			if entry.SkipsAll != nil && entry.SkipsAll(result) {
				skipAllPrereq = entry.Slug
			}
		case driving.SeverityWarning:
			out.Warnings = append(out.Warnings, finding)
			// Warnings do not trigger cascade skipping.
		default:
			// Unknown severity indicates a registry typo or an unset
			// severity. Fail loudly rather than silently dropping the finding.
			return driving.DoctorOutput{}, fmt.Errorf("doctor check %q: unknown severity %q", entry.Slug, entry.Severity)
		}
	}

	return out, nil
}

// firstBlockedPrereq returns the root-cause slug for the first entry in
// dependsOn that is present in cascadeBlocked, and true if one is found.
// Returns ("", false) when all prerequisites are clear.
func firstBlockedPrereq(dependsOn []string, cascadeBlocked map[string]string) (rootCause string, found bool) {
	for _, dep := range dependsOn {
		if cause, ok := cascadeBlocked[dep]; ok {
			return cause, true
		}
	}
	return "", false
}

// DoctorCheckInfo holds the display metadata for a single doctor check.
// Used by the text renderer to resolve titles, categories, and descriptions
// from check slugs without requiring it to import the unexported registry.
type DoctorCheckInfo struct {
	// Slug is the hyphenated check identifier (e.g. "invalid-parent-reference").
	Slug string
	// Title is the short display name shown in output (e.g. "Invalid parent reference").
	Title string
	// Category is the human-readable category name (e.g. "Database").
	Category string
	// Description is the one-sentence check description.
	Description string
	// WhyItMatters explains the impact of a failing check.
	WhyItMatters string
}

// DoctorCheckInfoBySlug returns a slug-keyed map of DoctorCheckInfo for all
// 16 registered checks. Callers can look up display metadata without knowing
// the internal registry structure.
func DoctorCheckInfoBySlug() map[string]DoctorCheckInfo {
	reg := doctorRegistry()
	m := make(map[string]DoctorCheckInfo, len(reg))
	for _, e := range reg {
		m[e.Slug] = DoctorCheckInfo{
			Slug:         e.Slug,
			Title:        e.Title,
			Category:     string(e.Category),
			Description:  e.Description,
			WhyItMatters: e.WhyItMatters,
		}
	}
	return m
}

// DoctorCategoryOrder returns the four category names in canonical display
// order: Database, Environment, Graph health, Issue lifecycle.
func DoctorCategoryOrder() []string {
	return []string{
		string(categoryDatabase),
		string(categoryEnvironment),
		string(categoryGraphHealth),
		string(categoryIssueLifecycle),
	}
}

// DoctorSlugsInOrder returns all 16 check slugs in canonical display order.
func DoctorSlugsInOrder() []string {
	return doctorDisplayOrder()
}
