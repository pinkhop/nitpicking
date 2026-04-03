package sqlite

import "strings"

// sanitizeFTS5Query converts a user-supplied search string into an FTS5
// query where each word is individually quoted and combined with implicit
// AND. Quoting each token prevents injection of FTS5 operators (AND, OR,
// NOT), column filters (title:foo), prefix queries (foo*), or NEAR groups.
// Multi-word inputs produce an AND query: "JSON" "subcommand" matches
// documents containing both words anywhere, in any order.
func sanitizeFTS5Query(query string) string {
	words := strings.Fields(query)
	if len(words) == 0 {
		return `""`
	}
	quoted := make([]string, len(words))
	for i, w := range words {
		escaped := strings.ReplaceAll(w, `"`, `""`)
		quoted[i] = `"` + escaped + `"`
	}
	return strings.Join(quoted, " ")
}
