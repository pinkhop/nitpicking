# Investigation: Terminal Column-Width Awareness for Text Output

## Third-Party Libraries

**`golang.org/x/term` (already a direct dependency at v0.41.0)**

Provides `IsTerminal(fd)` and `GetSize(fd) (width, height, err)` — the two functions needed. Maintained by the Go team; last release 2026-03-10. Single transitive dependency (`golang.org/x/sys`, already in the module graph). Covers macOS, Linux, FreeBSD, Windows, and most other platforms. On unsupported platforms, `GetSize` returns an error — safe to handle.

**Verdict: Use this. No new dependencies required.**

**`github.com/mattn/go-isatty` (indirect dependency)**

Only provides TTY detection, not terminal size. Irrelevant for width detection.

**stdlib alone**

Go's stdlib has no terminal width detection. The `syscall` package exposes raw ioctl on some platforms, but `x/term` wraps this correctly across all platforms.

**Others considered (not recommended):** `muesli/termenv` (too large), `nsf/termbox-go` (TUI framework — overkill), `buger/goterm` (unmaintained).

## Scope

The change is localised:

- **1 file touched centrally:** `internal/iostreams/iostreams.go` — add `TerminalWidth() int` method.
- **~7 list/search command files touched:** Each renderer would call a shared truncation helper before writing titles to the tabwriter. The change is mechanical — replace `item.Title` with `truncate(item.Title, availableWidth)`.
- **~1 new file:** A small utility for word wrapping and truncation (~40–60 lines), placed in `internal/iostreams` or a new `internal/termutil` package.
- **Percentage of codebase:** ~10 files out of ~100 source files (~10%). All changes are in the adapter layer.

## Impact

- **Lines added:** ~80–100 lines total (width detection, word wrap function, truncation function, and integration into renderers).
- **Lines removed:** 0 (this adds capability without removing existing code).
- **Localization:** Highly localised. Width detection lives in `IOStreams`; wrapping/truncation live in a utility function. Renderers only gain a `truncate()` call.

## Portability

| Context | `GetSize` | Fallback |
|---------|-----------|----------|
| macOS / Linux TTY | Works (ioctl TIOCGWINSZ) | N/A |
| Windows | Works (GetConsoleScreenBufferInfo) | N/A |
| Piped (`np list \| head`) | Error (non-TTY) | No truncation (full-width output) |
| CI (GitHub Actions, etc.) | Error (non-TTY) | No truncation |
| `$COLUMNS` set explicitly | Skip `GetSize`, use `$COLUMNS` | Allows manual override |
| All other | Error | Default to 80 columns |

**Key design point:** Width detection should only engage when `IsStdoutTTY()` is true. When piped, output should be full-width — downstream tools (grep, jq, etc.) need the full content.

## Maintenance Burden

**Low — one-time, centralised change.** The width detection lives in `IOStreams` and the truncation utility is called at render time. New commands that display tabular output would call the same truncation helper. No ongoing complexity beyond "remember to truncate titles".

Word wrapping for descriptions and comment bodies is slightly more complex but applies only to the `show` command's detail view — a single call site.

## Recommendation

**Go — but defer to a future ticket.** The implementation is straightforward, adds ~100 lines, requires no new dependencies (`x/term` is already present), and is fully localised. The main value is preventing broken table layouts with long titles, which is a genuine (if minor) usability issue.

However, human-readable output is secondary to JSON/agent workflows — this is polish, not a blocker. Recommended approach:

1. Add `TerminalWidth() int` to `IOStreams` (fallback chain: `$COLUMNS` → `term.GetSize` → 80).
2. Add `TruncateText(s string, maxWidth int) string` utility (truncate with `…` suffix).
3. Apply to all list-style renderers: title is always the last column; truncate to remaining width.
4. Optionally, add `WrapText(s string, width int) string` for the `show` command's description field.
