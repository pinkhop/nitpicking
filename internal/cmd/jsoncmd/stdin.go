package jsoncmd

import (
	"encoding/json"
	"fmt"
	"io"
)

// DecodeStdin reads exactly one JSON object from r and decodes it into a value
// of type T. It rejects unknown fields and trailing content — the input must
// contain exactly one well-formed JSON object and nothing else.
//
// This is the shared entry point for all json subcommands: each defines its own
// typed struct and calls DecodeStdin[MyStruct](os.Stdin) to get a fully decoded
// value with no casting.
func DecodeStdin[T any](r io.Reader) (T, error) {
	var zero T
	if r == nil {
		return zero, fmt.Errorf("stdin is nil")
	}

	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()

	var target T
	if err := dec.Decode(&target); err != nil {
		return zero, fmt.Errorf("decoding JSON from stdin: %w", err)
	}

	// Reject trailing content — the stream must contain exactly one object.
	if dec.More() {
		return zero, fmt.Errorf("stdin contains multiple JSON values; expected exactly one object")
	}

	return target, nil
}
