package version

import "context"

// ExportRun exposes run for black-box tests. This file is only compiled
// during testing.
var ExportRun = run

// Ensure the signature matches what tests expect.
var _ func(context.Context, *Options) error = ExportRun
