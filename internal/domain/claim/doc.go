// Package claim defines the Claim entity and claiming business rules.
// A claim represents active ownership of an issue, using bearer-authenticated
// random claim IDs. Claims gate all mutable issue operations except comments
// and relationships.
package claim
