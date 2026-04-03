// Package domain defines the core business types, error categories, and
// identity types for the nitpicking issue tracker. It includes the Issue
// entity and its associated value types (ID, State, Role, Priority, Label,
// Relationship), state machine transitions, readiness and completion
// derivation, parent hierarchy validation, soft-deletion rules, Author
// validation, agent name generation, agent instructions, backup serialisation
// structures (BackupHeader, BackupIssueRecord, etc.), and typed domain errors
// so that adapters (CLI, HTTP, etc.) can map them to appropriate exit codes
// or status codes without inspecting error messages.
package domain
