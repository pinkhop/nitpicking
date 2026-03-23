// Package root constructs the root command that assembles all subcommands and
// defines cross-cutting behavior in the Before hook (log-level adjustment,
// logger injection). It also provides context-based logger propagation via
// WithLogger and LoggerFrom, allowing the Before hook to enrich the context
// with a logger that subcommands can retrieve without direct Factory access.
package root
