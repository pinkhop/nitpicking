// Package jsoncmd provides the "json" parent command, which groups
// agent-oriented subcommands that accept structured JSON input on stdin and
// produce JSON output unconditionally. The package also exports a generic
// DecodeStdin helper used by all json subcommands to decode typed payloads
// from stdin.
package jsoncmd
