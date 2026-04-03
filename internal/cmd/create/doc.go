// Package create provides the root-level "create" command, which auto-detects
// its input mode using IOStreams TTY detection: when stdin is a pipe, it
// delegates to the JSON create path; when stdin is a TTY, it launches the
// interactive form. This is the convenience entry point — the explicit-mode
// commands "json create" and "form create" continue to exist independently.
package create
