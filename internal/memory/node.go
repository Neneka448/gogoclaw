package memory

import "time"

type NodeStatus string

const (
	NodeStatusActive       NodeStatus = "active"
	NodeStatusConsolidated NodeStatus = "consolidated"
	NodeStatusArchived     NodeStatus = "archived"
)

type NodeKind string

const (
	NodeKindShortTerm NodeKind = "short_term"
	NodeKindLongTerm  NodeKind = "long_term"
)

type MemoryNode struct {
	ID            string     `json:"id"`
	Kind          NodeKind   `json:"kind"`
	Status        NodeStatus `json:"status"`
	Level         int        `json:"level"`
	Summary       string     `json:"summary"`
	Who           string     `json:"who"`
	What          string     `json:"what"`
	When          string     `json:"when"`
	Where         string     `json:"where"`
	Why           string     `json:"why"`
	How           string     `json:"how"`
	Result        string     `json:"result"`
	RefCount      int        `json:"ref_count"`
	SourceNodeIDs []string   `json:"source_node_ids,omitempty"`
	SessionID     string     `json:"session_id,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// EmbeddingText produces the text used for generating the node embedding vector.
// Uses structured 5W1H format so similar scenarios naturally cluster.
func (node *MemoryNode) EmbeddingText() string {
	text := ""
	if node.Who != "" {
		text += "Who: " + node.Who + "\n"
	}
	if node.What != "" {
		text += "What: " + node.What + "\n"
	}
	if node.When != "" {
		text += "When: " + node.When + "\n"
	}
	if node.Where != "" {
		text += "Where: " + node.Where + "\n"
	}
	if node.Why != "" {
		text += "Why: " + node.Why + "\n"
	}
	if node.How != "" {
		text += "How: " + node.How + "\n"
	}
	if node.Result != "" {
		text += "Result: " + node.Result + "\n"
	}
	return text
}
