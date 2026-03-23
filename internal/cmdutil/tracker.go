package cmdutil

import (
	"fmt"

	"github.com/pinkhop/nitpicking/internal/app/service"
)

// NewTracker constructs the application service from the Factory's Store
// connection. This is a convenience function that prevents boilerplate in
// every command — the Factory provides the architecture-neutral database
// connection; this function wires it into the application layer.
func NewTracker(f *Factory) (service.Service, error) {
	store, err := f.Store()
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	return service.New(store), nil
}
