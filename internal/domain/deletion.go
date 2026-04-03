package domain

// DescendantInfo describes a descendant issue for recursive deletion checks.
type DescendantInfo struct {
	// ID is the descendant's issue ID.
	ID ID
	// IsClaimed is true if the descendant is currently claimed.
	IsClaimed bool
	// ClaimedBy is the author of the active claim, if any.
	ClaimedBy string
}
