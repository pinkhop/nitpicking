// Package history defines the HistoryEntry entity — an immutable, append-only
// record of every mutation to an issue's state. History entries enable full
// auditability and state reconstruction. Comments do not produce history entries.
package history
