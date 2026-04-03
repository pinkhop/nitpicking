// Package jsonl implements the backup driven-port interfaces using the
// JSON Lines format. Each line in the output file is a self-contained
// JSON object: the first line is the backup header (metadata), and
// every subsequent line is an issue record.
package jsonl
