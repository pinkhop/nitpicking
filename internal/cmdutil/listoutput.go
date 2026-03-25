package cmdutil

import (
	"github.com/pinkhop/nitpicking/internal/domain/port"
)

// ListItemOutput is the JSON representation of a single issue in a list.
// It is shared by the list, ready, and blocked commands so that their
// --json output is structurally identical.
type ListItemOutput struct {
	ID            string `json:"id"`
	Role          string `json:"role"`
	State         string `json:"state"`
	DisplayStatus string `json:"display_status"`
	Priority      string `json:"priority"`
	Title         string `json:"title"`
	CreatedAt     string `json:"created_at"`
}

// ListOutput is the JSON representation of a list command result.
type ListOutput struct {
	Items   []ListItemOutput `json:"items"`
	HasMore bool             `json:"has_more"`
}

// ConvertListItems transforms a slice of domain IssueListItem values into
// the shared JSON output representation.
func ConvertListItems(items []port.IssueListItem) []ListItemOutput {
	out := make([]ListItemOutput, 0, len(items))
	for _, item := range items {
		out = append(out, ListItemOutput{
			ID:            item.ID.String(),
			Role:          item.Role.String(),
			State:         item.State.String(),
			DisplayStatus: item.DisplayStatus(),
			Priority:      item.Priority.String(),
			Title:         item.Title,
			CreatedAt:     FormatJSONTimestamp(item.CreatedAt),
		})
	}
	return out
}
