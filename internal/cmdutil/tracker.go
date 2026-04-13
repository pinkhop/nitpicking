package cmdutil

import (
	"fmt"

	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// NewTracker constructs the application service from the Factory's Store
// connection. This is a convenience function that prevents boilerplate in
// every command — the Factory provides the architecture-neutral database
// connection; this function wires it into the application layer. The SQLite
// store satisfies both driven.Transactor and driven.Migrator, so it is
// passed for both roles to enable schema migration through the service.
func NewTracker(f *Factory) (driving.Service, error) {
	store, err := f.Store()
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	return core.New(store, store), nil
}
