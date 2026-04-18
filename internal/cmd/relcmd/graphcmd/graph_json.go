package graphcmd

import (
	"encoding/json"

	"github.com/pinkhop/nitpicking/internal/domain"
)

// graphJSONIssue is the serialization structure for one issue in the JSON
// graph. Top-level array contains only root issues (no parent); children are
// recursively nested.
type graphJSONIssue struct {
	ID            string                 `json:"id"`
	Role          string                 `json:"role"`
	State         string                 `json:"state"`
	Title         string                 `json:"title"`
	Relationships graphJSONRelationships `json:"relationships"`
	Children      []graphJSONIssue       `json:"children"`
}

// graphJSONRelationships holds the three relationship categories. Keys are
// always present; values are empty arrays when no relationships exist — never
// null.
type graphJSONRelationships struct {
	Blocks    []string `json:"blocks"`
	BlockedBy []string `json:"blocked_by"`
	Refs      []string `json:"refs"`
}

// RenderGraphJSON converts a set of nodes and edges into a JSON string
// representing a nested graph structure. Root issues (those with no parent)
// appear at the top level; children are recursively nested under their parent.
// Edges are distributed to both endpoints: a blocked_by edge from A→B appears
// as blocked_by on A and blocks on B. Refs edges are symmetric and appear on
// both endpoints.
func RenderGraphJSON(nodes []GraphNode, edges []GraphEdge) string {
	// Index nodes by ID and build relationship maps.
	nodeIndex := make(map[string]*graphJSONIssue, len(nodes))
	childrenOf := make(map[string][]*graphJSONIssue)

	for _, n := range nodes {
		ji := &graphJSONIssue{
			ID:    n.ID.String(),
			Role:  n.Role.String(),
			State: n.State.String(),
			Title: n.Title,
			Relationships: graphJSONRelationships{
				Blocks:    []string{},
				BlockedBy: []string{},
				Refs:      []string{},
			},
			Children: []graphJSONIssue{},
		}
		nodeIndex[n.ID.String()] = ji

		if !n.ParentID.IsZero() {
			childrenOf[n.ParentID.String()] = append(childrenOf[n.ParentID.String()], ji)
		}
	}

	// Distribute edges to both endpoints.
	for _, e := range edges {
		src := nodeIndex[e.SourceID.String()]
		tgt := nodeIndex[e.TargetID.String()]
		if src == nil || tgt == nil {
			continue
		}

		switch e.Type {
		case domain.RelBlockedBy:
			src.Relationships.BlockedBy = append(src.Relationships.BlockedBy, e.TargetID.String())
			tgt.Relationships.Blocks = append(tgt.Relationships.Blocks, e.SourceID.String())
		case domain.RelRefs:
			// Refs is symmetric — both endpoints list each other.
			src.Relationships.Refs = append(src.Relationships.Refs, e.TargetID.String())
			tgt.Relationships.Refs = append(tgt.Relationships.Refs, e.SourceID.String())
		}
	}

	// Recursively nest children and collect root issues.
	var roots []graphJSONIssue
	for _, n := range nodes {
		attachGraphChildren(nodeIndex[n.ID.String()], childrenOf)
	}
	for _, n := range nodes {
		if n.ParentID.IsZero() {
			roots = append(roots, *nodeIndex[n.ID.String()])
		}
	}

	if roots == nil {
		roots = []graphJSONIssue{}
	}

	// Marshal to JSON. Errors from json.Marshal on known-good structures are
	// unreachable in practice, so we discard the error.
	data, _ := json.MarshalIndent(roots, "", "  ")
	return string(data)
}

// attachGraphChildren recursively populates the Children slice of a
// graphJSONIssue from the childrenOf index.
func attachGraphChildren(parent *graphJSONIssue, childrenOf map[string][]*graphJSONIssue) {
	kids := childrenOf[parent.ID]
	if len(kids) == 0 {
		return
	}
	parent.Children = make([]graphJSONIssue, 0, len(kids))
	for _, kid := range kids {
		attachGraphChildren(kid, childrenOf)
		parent.Children = append(parent.Children, *kid)
	}
}
