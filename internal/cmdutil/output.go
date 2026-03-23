package cmdutil

import (
	"encoding/json"
	"fmt"
	"io"
)

// WriteJSON writes v as indented JSON to w, followed by a newline.
// This is the standard JSON output format for all commands.
func WriteJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encoding JSON: %w", err)
	}
	return nil
}
