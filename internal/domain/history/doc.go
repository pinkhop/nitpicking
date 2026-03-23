// Package history defines the HistoryEntry entity — an immutable, append-only
// record of every mutation to a ticket's state. History entries enable full
// auditability and state reconstruction. Notes do not produce history entries.
package history
