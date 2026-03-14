package memory

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	memoryNodesTable = "gogoclaw_memory_nodes"
	memoryEdgesTable = "gogoclaw_memory_edges"
)

// Store handles persistence of memory nodes (metadata in SQLite) and edges.
// Embedding vectors are managed by the vectorstore.Service that shares the same DB.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Initialize() error {
	if _, err := s.db.Exec(`
		create table if not exists ` + memoryNodesTable + ` (
			id text primary key,
			kind text not null default 'short_term',
			status text not null default 'active',
			level integer not null default 0,
			summary text not null default '',
			who text not null default '',
			what text not null default '',
			"when" text not null default '',
			"where" text not null default '',
			why text not null default '',
			how text not null default '',
			result text not null default '',
			ref_count integer not null default 0,
			source_node_ids text not null default '[]',
			session_id text not null default '',
			created_at text not null default (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
			updated_at text not null default (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
		)
	`); err != nil {
		return fmt.Errorf("create memory nodes table: %w", err)
	}

	if _, err := s.db.Exec(`
		create table if not exists ` + memoryEdgesTable + ` (
			source_id text not null,
			target_id text not null,
			weight real not null default 0,
			created_at text not null default (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
			primary key (source_id, target_id),
			foreign key (source_id) references ` + memoryNodesTable + `(id),
			foreign key (target_id) references ` + memoryNodesTable + `(id)
		)
	`); err != nil {
		return fmt.Errorf("create memory edges table: %w", err)
	}

	return nil
}

func (s *Store) InsertNode(node MemoryNode) error {
	sourceIDs, err := json.Marshal(node.SourceNodeIDs)
	if err != nil {
		return fmt.Errorf("marshal source node ids: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)

	if _, err := s.db.Exec(`
		insert into `+memoryNodesTable+` (
			id, kind, status, level, summary, who, what, "when", "where", why, how, result,
			ref_count, source_node_ids, session_id, created_at, updated_at
		) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		node.ID, string(node.Kind), string(node.Status), node.Level,
		node.Summary, node.Who, node.What, node.When, node.Where, node.Why, node.How, node.Result,
		node.RefCount, string(sourceIDs), node.SessionID, now, now,
	); err != nil {
		return fmt.Errorf("insert memory node %s: %w", node.ID, err)
	}
	return nil
}

func (s *Store) UpdateNodeStatus(nodeID string, status NodeStatus) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(`
		update `+memoryNodesTable+` set status = ?, updated_at = ? where id = ?
	`, string(status), now, nodeID); err != nil {
		return fmt.Errorf("update node status %s: %w", nodeID, err)
	}
	return nil
}

func (s *Store) IncrementRefCount(nodeID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(`
		update `+memoryNodesTable+` set ref_count = ref_count + 1, updated_at = ? where id = ?
	`, now, nodeID); err != nil {
		return fmt.Errorf("increment ref count for %s: %w", nodeID, err)
	}
	return nil
}

func (s *Store) GetNode(nodeID string) (*MemoryNode, error) {
	row := s.db.QueryRow(`
		select id, kind, status, level, summary, who, what, "when", "where", why, how, result,
			ref_count, source_node_ids, session_id, created_at, updated_at
		from `+memoryNodesTable+` where id = ?
	`, nodeID)
	return scanNode(row)
}

func (s *Store) ListActiveNodesByKindAndLevel(kind NodeKind, level int) ([]MemoryNode, error) {
	rows, err := s.db.Query(`
		select id, kind, status, level, summary, who, what, "when", "where", why, how, result,
			ref_count, source_node_ids, session_id, created_at, updated_at
		from `+memoryNodesTable+`
		where kind = ? and level = ? and status = ?
		order by created_at desc
	`, string(kind), level, string(NodeStatusActive))
	if err != nil {
		return nil, fmt.Errorf("list active nodes: %w", err)
	}
	defer rows.Close()
	return scanNodes(rows)
}

func (s *Store) ListActiveNodeIDs(kind NodeKind, level int) ([]string, error) {
	rows, err := s.db.Query(`
		select id from `+memoryNodesTable+`
		where kind = ? and level = ? and status = ?
	`, string(kind), level, string(NodeStatusActive))
	if err != nil {
		return nil, fmt.Errorf("list active node ids: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) InsertEdge(edge MemoryEdge) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(`
		insert or ignore into `+memoryEdgesTable+` (source_id, target_id, weight, created_at)
		values (?, ?, ?, ?)
	`, edge.SourceID, edge.TargetID, edge.Weight, now); err != nil {
		return fmt.Errorf("insert memory edge %s->%s: %w", edge.SourceID, edge.TargetID, err)
	}
	return nil
}

func (s *Store) ListEdgesForNodes(nodeIDs []string) ([]MemoryEdge, error) {
	if len(nodeIDs) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(nodeIDs))
	args := make([]any, 0, len(nodeIDs)*2)
	for i, id := range nodeIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	inClause := strings.Join(placeholders, ",")
	// duplicate args for both source_id and target_id IN clauses
	args = append(args, args...)

	rows, err := s.db.Query(`
		select source_id, target_id, weight, created_at
		from `+memoryEdgesTable+`
		where source_id in (`+inClause+`) and target_id in (`+inClause+`)
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list edges for nodes: %w", err)
	}
	defer rows.Close()

	var edges []MemoryEdge
	for rows.Next() {
		var edge MemoryEdge
		var createdAt string
		if err := rows.Scan(&edge.SourceID, &edge.TargetID, &edge.Weight, &createdAt); err != nil {
			return nil, err
		}
		parsedTime, parseErr := time.Parse(time.RFC3339Nano, createdAt)
		if parseErr != nil {
			return nil, fmt.Errorf("parse edge created_at %q: %w", createdAt, parseErr)
		}
		edge.CreatedAt = parsedTime
		edges = append(edges, edge)
	}
	return edges, rows.Err()
}

func (s *Store) GetNodesByIDs(nodeIDs []string) ([]MemoryNode, error) {
	if len(nodeIDs) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(nodeIDs))
	args := make([]any, len(nodeIDs))
	for i, id := range nodeIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	rows, err := s.db.Query(`
		select id, kind, status, level, summary, who, what, "when", "where", why, how, result,
			ref_count, source_node_ids, session_id, created_at, updated_at
		from `+memoryNodesTable+`
		where id in (`+strings.Join(placeholders, ",")+`)
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("get nodes by ids: %w", err)
	}
	defer rows.Close()
	return scanNodes(rows)
}

type scannable interface {
	Scan(dest ...any) error
}

func scanNode(row scannable) (*MemoryNode, error) {
	var node MemoryNode
	var kindStr, statusStr string
	var sourceIDsJSON string
	var createdAt, updatedAt string

	if err := row.Scan(
		&node.ID, &kindStr, &statusStr, &node.Level,
		&node.Summary, &node.Who, &node.What, &node.When, &node.Where, &node.Why, &node.How, &node.Result,
		&node.RefCount, &sourceIDsJSON, &node.SessionID, &createdAt, &updatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	node.Kind = NodeKind(kindStr)
	node.Status = NodeStatus(statusStr)
	var parseErr error
	node.CreatedAt, parseErr = time.Parse(time.RFC3339Nano, createdAt)
	if parseErr != nil {
		return nil, fmt.Errorf("parse node created_at %q: %w", createdAt, parseErr)
	}
	node.UpdatedAt, parseErr = time.Parse(time.RFC3339Nano, updatedAt)
	if parseErr != nil {
		return nil, fmt.Errorf("parse node updated_at %q: %w", updatedAt, parseErr)
	}
	if sourceIDsJSON != "" {
		_ = json.Unmarshal([]byte(sourceIDsJSON), &node.SourceNodeIDs)
	}
	return &node, nil
}

func scanNodes(rows *sql.Rows) ([]MemoryNode, error) {
	var nodes []MemoryNode
	for rows.Next() {
		node, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		if node != nil {
			nodes = append(nodes, *node)
		}
	}
	return nodes, rows.Err()
}
