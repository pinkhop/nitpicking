// Package root constructs the root command that assembles all subcommands and
// defines cross-cutting behavior in the Before hook (signal handling and schema
// version gating). Every command that opens the database is checked against the
// minimum required schema version before proceeding; np admin upgrade and np
// admin doctor are exempt so they can operate on pre-migration databases.
package root
