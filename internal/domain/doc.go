// Package domain defines the core business types and error categories for
// the nitpicking issue tracker. Domain errors are typed so that adapters
// (CLI, HTTP, etc.) can map them to appropriate exit codes or status codes
// without inspecting error messages.
package domain
