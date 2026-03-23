// Package ticket defines the core ticket domain model: ticket IDs, roles,
// priorities, facets, state machines, the ticket entity itself, and all
// associated business rules (parent constraints, readiness, completion
// derivation, soft deletion). This package has no dependencies on persistence
// or CLI concerns.
package ticket
