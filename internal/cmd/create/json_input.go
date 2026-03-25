package create

import (
	"encoding/json"
	"fmt"
	"io"
)

// issueJSON is the JSON structure accepted by --from-json. Field names match
// the show --json output so that piping show output into create works directly.
// Fields that are not relevant to creation (id, state, revision, etc.) are
// silently ignored via the json decoder.
type issueJSON struct {
	Role               string            `json:"role"`
	Title              string            `json:"title"`
	Description        string            `json:"description"`
	AcceptanceCriteria string            `json:"acceptance_criteria"`
	Priority           string            `json:"priority"`
	ParentID           string            `json:"parent_id"`
	Labels             map[string]string `json:"labels"`
}

// parseIssueJSON unmarshals a JSON byte slice into an issueJSON struct.
func parseIssueJSON(data []byte) (issueJSON, error) {
	var tj issueJSON
	if err := json.Unmarshal(data, &tj); err != nil {
		return issueJSON{}, fmt.Errorf("parsing --from-json: %w", err)
	}
	return tj, nil
}

// readJSONSource reads JSON data from the given value. If the value is "-",
// it reads from the provided reader (typically stdin). Otherwise, the value
// itself is treated as a JSON string.
func readJSONSource(value string, stdin io.Reader) ([]byte, error) {
	if value == "-" {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("reading JSON from stdin: %w", err)
		}
		return data, nil
	}
	return []byte(value), nil
}

// mergeDimensionsFromJSON combines dimensions from JSON, env var, and flags. Precedence
// (highest to lowest): flag dimensions, JSON dimensions, env dimensions. Dimensions with
// different keys from all sources are merged; for the same key, the
// higher-precedence source wins.
func mergeDimensionsFromJSON(envDimensions, jsonDimensions, flagDimensions []string) []string {
	// Build a map in precedence order: env (lowest), then JSON, then flags
	// (highest). Later entries overwrite earlier ones for the same key.
	seen := make(map[string]string)
	order := make([]string, 0)

	addDimensions := func(dimensions []string) {
		for _, f := range dimensions {
			key, _, ok := cutDimension(f)
			if !ok {
				continue
			}
			if _, exists := seen[key]; !exists {
				order = append(order, key)
			}
			seen[key] = f
		}
	}

	addDimensions(envDimensions)
	addDimensions(jsonDimensions)
	addDimensions(flagDimensions)

	result := make([]string, 0, len(seen))
	for _, key := range order {
		result = append(result, seen[key])
	}
	return result
}

// cutDimension splits a "key:value" string into key and the full string.
func cutDimension(s string) (key string, value string, ok bool) {
	for i := range len(s) {
		if s[i] == ':' {
			return s[:i], s[i+1:], true
		}
	}
	return "", "", false
}

// jsonLabelsToStrings converts a map of labels into "key:value" string format.
func jsonLabelsToStrings(labels map[string]string) []string {
	if len(labels) == 0 {
		return nil
	}
	result := make([]string, 0, len(labels))
	for k, v := range labels {
		result = append(result, k+":"+v)
	}
	return result
}
