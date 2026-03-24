package create

import (
	"encoding/json"
	"fmt"
	"io"
)

// ticketJSON is the JSON structure accepted by --from-json. Field names match
// the show --json output so that piping show output into create works directly.
// Fields that are not relevant to creation (id, state, revision, etc.) are
// silently ignored via the json decoder.
type ticketJSON struct {
	Role               string            `json:"role"`
	Title              string            `json:"title"`
	Description        string            `json:"description"`
	AcceptanceCriteria string            `json:"acceptance_criteria"`
	Priority           string            `json:"priority"`
	ParentID           string            `json:"parent_id"`
	Facets             map[string]string `json:"facets"`
}

// parseTicketJSON unmarshals a JSON byte slice into a ticketJSON struct.
func parseTicketJSON(data []byte) (ticketJSON, error) {
	var tj ticketJSON
	if err := json.Unmarshal(data, &tj); err != nil {
		return ticketJSON{}, fmt.Errorf("parsing --from-json: %w", err)
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

// mergeFacetsFromJSON combines facets from JSON, env var, and flags. Precedence
// (highest to lowest): flag facets, JSON facets, env facets. Facets with
// different keys from all sources are merged; for the same key, the
// higher-precedence source wins.
func mergeFacetsFromJSON(envFacets, jsonFacets, flagFacets []string) []string {
	// Build a map in precedence order: env (lowest), then JSON, then flags
	// (highest). Later entries overwrite earlier ones for the same key.
	seen := make(map[string]string)
	order := make([]string, 0)

	addFacets := func(facets []string) {
		for _, f := range facets {
			key, _, ok := cutFacet(f)
			if !ok {
				continue
			}
			if _, exists := seen[key]; !exists {
				order = append(order, key)
			}
			seen[key] = f
		}
	}

	addFacets(envFacets)
	addFacets(jsonFacets)
	addFacets(flagFacets)

	result := make([]string, 0, len(seen))
	for _, key := range order {
		result = append(result, seen[key])
	}
	return result
}

// cutFacet splits a "key:value" string into key and the full string.
func cutFacet(s string) (key string, value string, ok bool) {
	for i := range len(s) {
		if s[i] == ':' {
			return s[:i], s[i+1:], true
		}
	}
	return "", "", false
}

// jsonFacetsToStrings converts a map of facets into "key:value" string format.
func jsonFacetsToStrings(facets map[string]string) []string {
	if len(facets) == 0 {
		return nil
	}
	result := make([]string, 0, len(facets))
	for k, v := range facets {
		result = append(result, k+":"+v)
	}
	return result
}
