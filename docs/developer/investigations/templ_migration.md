# templ Migration Investigation

**Date:** 2026-03-24
**Status:** No migration needed; templ is available for future use.

## Findings

The investigation found no Go template usage to migrate. The only
`text/template` string in the codebase is the root help template owned
by the urfave/cli framework — not application code that would benefit
from templ's type-safe template system.

## Current State

- `templ` is installed as a Go tool dependency (added via `go.mod` and
  the `make templ` target).
- No `.templ` files exist in the project.
- All CLI output uses direct `fmt.Fprintf` calls; there are no
  application-level templates.

## When to Use templ

templ becomes relevant when the project adds HTML output features (e.g.,
a web dashboard, HTML reports, or server-rendered views). At that point,
use templ for type-safe, component-based HTML templates instead of
`html/template`.
