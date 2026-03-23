// Package claim defines the Claim entity and claiming business rules.
// A claim represents active ownership of a ticket, using bearer-authenticated
// random claim IDs. Claims gate all mutable ticket operations except notes
// and relationships.
package claim
