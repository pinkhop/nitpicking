package cmdutil

import (
	"context"
	"fmt"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
)

// IDResolver resolves issue ID strings that may be either full IDs
// (PREFIX-random) or bare random parts (just the 5-char Crockford string).
// It caches the database prefix on first use so that subsequent calls do not
// require additional database round-trips.
type IDResolver struct {
	svc    service.Service
	prefix string
	loaded bool
}

// NewIDResolver creates an IDResolver backed by the given service. The prefix
// is fetched lazily on the first call to Resolve.
func NewIDResolver(svc service.Service) *IDResolver {
	return &IDResolver{svc: svc}
}

// Resolve parses an issue ID string. If the string is a full ID (contains a
// separator), it is parsed directly. If it is a bare random part, the database
// prefix is fetched (and cached) and prepended before parsing.
func (r *IDResolver) Resolve(ctx context.Context, raw string) (issue.ID, error) {
	// Fast path: try parsing as a full ID first.
	id, err := issue.ParseID(raw)
	if err == nil {
		return id, nil
	}

	// Slow path: might be a bare random part — fetch the prefix.
	prefix, prefixErr := r.getPrefix(ctx)
	if prefixErr != nil {
		// If we can't get the prefix, return the original parse error since
		// it's more informative about what the user provided.
		return issue.ID{}, fmt.Errorf("invalid issue ID %q: %w", raw, err)
	}

	resolved, resolveErr := issue.ResolveID(raw, prefix)
	if resolveErr != nil {
		return issue.ID{}, resolveErr
	}
	return resolved, nil
}

// getPrefix returns the cached prefix, fetching it from the database on first
// call.
func (r *IDResolver) getPrefix(ctx context.Context) (string, error) {
	if r.loaded {
		return r.prefix, nil
	}

	prefix, err := r.svc.GetPrefix(ctx)
	if err != nil {
		return "", err
	}
	r.prefix = prefix
	r.loaded = true
	return prefix, nil
}
