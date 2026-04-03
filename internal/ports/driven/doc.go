// Package driven defines the driven port interfaces — the contracts that the
// core requires from its persistence and backup layers. Adapters (SQLite,
// in-memory, JSONL) implement these interfaces; the core depends only on the
// abstractions.
package driven
