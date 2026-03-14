package memory

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
	mathrand "math/rand"

	"github.com/Neneka448/gogoclaw/internal/config"
	"github.com/Neneka448/gogoclaw/internal/provider"
	"github.com/Neneka448/gogoclaw/internal/vectorstore"

	openai "github.com/sashabaranov/go-openai"
)

const maxConsolidationLevel = 10

type Service interface {
	// Initialize sets up the memory tables in the shared DB.
	Initialize() error

	// IngestSession takes raw session messages and creates a short-term memory node.
	// It summarizes the session (5W1H+R), embeds it, stores it, connects edges,
	// and triggers community check for short-term nodes.
	IngestSession(sessionID string, messages []openai.ChatCompletionMessage) error

	// Recall searches the vector store for memory nodes relevant to the query text.
	// Only active nodes are returned. Each returned node's ref_count is incremented.
	Recall(queryText string, topK int, minSimilarity float64) ([]MemoryNode, error)

	// GetNode returns a single node by ID.
	GetNode(nodeID string) (*MemoryNode, error)
}

type service struct {
	mu          sync.Mutex
	store       *Store
	vectorStore vectorstore.Service
	embedding   provider.EmbeddingProvider
	summarizer  *Summarizer
	config      config.MemoryConfig
}

func NewService(
	vectorStore vectorstore.Service,
	llm provider.LLMProviderOpenaiCompatible,
	model string,
	embeddingProvider provider.EmbeddingProvider,
	memoryConfig config.MemoryConfig,
) Service {
	return &service{
		store:       nil,
		vectorStore: vectorStore,
		embedding:   embeddingProvider,
		summarizer:  NewSummarizer(llm, model),
		config:      memoryConfig,
	}
}

func (s *service) Initialize() error {
	if s.store == nil {
		db := s.vectorStore.DB()
		if db == nil {
			return fmt.Errorf("memory service initialization failed: vector store DB is not initialized")
		}
		s.store = NewStore(db)
	}
	return s.store.Initialize()
}

func (s *service) IngestSession(sessionID string, messages []openai.ChatCompletionMessage) error {
	if s.store == nil {
		if err := s.Initialize(); err != nil {
			return fmt.Errorf("initialize memory service: %w", err)
		}
	}

	sessionContent := formatSessionMessages(messages)
	if strings.TrimSpace(sessionContent) == "" {
		return nil
	}

	summary, err := s.summarizer.SummarizeSession(sessionContent)
	if err != nil {
		return fmt.Errorf("summarize session: %w", err)
	}

	node := MemoryNode{
		ID:        generateNodeID("st"),
		Kind:      NodeKindShortTerm,
		Status:    NodeStatusActive,
		Level:     0,
		Summary:   buildSummaryText(summary),
		Who:       summary.Who,
		What:      summary.What,
		When:      summary.When,
		Where:     summary.Where,
		Why:       summary.Why,
		How:       summary.How,
		Result:    summary.Result,
		SessionID: sessionID,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.insertNodeWithEdgesAndCommunityCheck(node, NodeKindShortTerm, 0)
}

func (s *service) Recall(queryText string, topK int, minSimilarity float64) ([]MemoryNode, error) {
	if s.embedding == nil {
		return nil, fmt.Errorf("embedding provider is not configured")
	}
	if topK <= 0 {
		topK = s.config.RecallTopK
	}
	if minSimilarity < 0 {
		minSimilarity = s.config.RecallMinSimilarity
	}

	if s.store == nil {
		if err := s.Initialize(); err != nil {
			return nil, fmt.Errorf("initialize memory service: %w", err)
		}
	}

	queryEmbedding, err := s.embedQuery(queryText)
	if err != nil {
		return nil, fmt.Errorf("embed recall query: %w", err)
	}

	results, err := s.vectorStore.SearchTopK(vectorstore.SearchRequest{
		StoreKind: vectorstore.StoreKindText,
		Query:     queryEmbedding,
		Limit:     topK * 3,
		Metric:    vectorstore.DistanceMetricCosine,
	})
	if err != nil {
		return nil, fmt.Errorf("vector search for recall: %w", err)
	}

	var matchedIDs []string
	for _, result := range results {
		similarity := 1.0 - result.Distance
		if similarity < minSimilarity {
			continue
		}
		matchedIDs = append(matchedIDs, result.ExternalID)
		if len(matchedIDs) >= topK*2 {
			break
		}
	}

	if len(matchedIDs) == 0 {
		return nil, nil
	}

	allNodes, err := s.store.GetNodesByIDs(matchedIDs)
	if err != nil {
		return nil, fmt.Errorf("load recalled nodes: %w", err)
	}

	var activeNodes []MemoryNode
	for _, node := range allNodes {
		if node.Status == NodeStatusActive {
			activeNodes = append(activeNodes, node)
		}
		if len(activeNodes) >= topK {
			break
		}
	}

	for _, node := range activeNodes {
		_ = s.store.IncrementRefCount(node.ID)
	}

	return activeNodes, nil
}

func (s *service) GetNode(nodeID string) (*MemoryNode, error) {
	return s.store.GetNode(nodeID)
}

// insertNodeWithEdgesAndCommunityCheck embeds the node, stores it in SQLite first,
// then upserts its vector, finds similar same-kind/same-level active nodes to create edges,
// then checks if any community has exceeded the threshold.
// Caller must hold s.mu.
func (s *service) insertNodeWithEdgesAndCommunityCheck(node MemoryNode, kind NodeKind, level int) error {
	embeddingText := node.EmbeddingText()
	if strings.TrimSpace(embeddingText) == "" {
		return nil
	}

	embedding, err := s.embed(embeddingText)
	if err != nil {
		return fmt.Errorf("embed node %s: %w", node.ID, err)
	}

	// Insert SQLite record first so a failed vector upsert leaves a recoverable row.
	if err := s.store.InsertNode(node); err != nil {
		return fmt.Errorf("insert node record: %w", err)
	}

	metaJSON, err := json.Marshal(node)
	if err != nil {
		return fmt.Errorf("marshal node metadata: %w", err)
	}
	if err := s.vectorStore.Upsert(vectorstore.UpsertRequest{
		StoreKind:    vectorstore.StoreKindText,
		ExternalID:   node.ID,
		Embedding:    embedding,
		MetadataJSON: string(metaJSON),
	}); err != nil {
		return fmt.Errorf("upsert node vector: %w", err)
	}

	if err := s.connectEdges(node, embedding, kind, level); err != nil {
		return fmt.Errorf("connect edges for %s: %w", node.ID, err)
	}

	return s.checkCommunityThreshold(node.ID, kind, level)
}

func (s *service) connectEdges(node MemoryNode, embedding []float32, kind NodeKind, level int) error {
	peerIDs, err := s.store.ListActiveNodeIDs(kind, level)
	if err != nil {
		return err
	}
	if len(peerIDs) == 0 {
		return nil
	}

	results, err := s.vectorStore.SearchByThreshold(vectorstore.ThresholdSearchRequest{
		StoreKind:  vectorstore.StoreKindText,
		Query:      embedding,
		Metric:     vectorstore.DistanceMetricCosine,
		MaxResults: 100,
		Threshold:  s.config.EdgeSimilarityThreshold,
		ExternalID: node.ID,
	})
	if err != nil {
		return fmt.Errorf("threshold search for edges: %w", err)
	}

	peerSet := make(map[string]struct{}, len(peerIDs))
	for _, id := range peerIDs {
		peerSet[id] = struct{}{}
	}

	for _, result := range results {
		if result.ExternalID == node.ID {
			continue
		}
		if _, ok := peerSet[result.ExternalID]; !ok {
			continue
		}
		similarity := 1.0 - result.Distance
		if err := s.store.InsertEdge(MemoryEdge{
			SourceID: node.ID,
			TargetID: result.ExternalID,
			Weight:   similarity,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *service) checkCommunityThreshold(nodeID string, kind NodeKind, level int) error {
	if level >= maxConsolidationLevel {
		return nil
	}
	threshold := s.config.ShortTermCommunityThreshold
	if kind == NodeKindLongTerm {
		threshold = s.config.LongTermCommunityThreshold
	}
	if threshold <= 0 {
		return nil
	}

	activeIDs, err := s.store.ListActiveNodeIDs(kind, level)
	if err != nil {
		return err
	}
	edges, err := s.store.ListEdgesForNodes(activeIDs)
	if err != nil {
		return err
	}

	community := FindCommunityContaining(nodeID, activeIDs, edges)
	if len(community) < threshold {
		return nil
	}

	return s.consolidateCommunity(community, kind, level)
}

func (s *service) consolidateCommunity(communityIDs []string, kind NodeKind, level int) error {
	nodes, err := s.store.GetNodesByIDs(communityIDs)
	if err != nil {
		return fmt.Errorf("load community nodes: %w", err)
	}
	if len(nodes) == 0 {
		return nil
	}

	summary, err := s.summarizer.SummarizeCommunity(nodes)
	if err != nil {
		return fmt.Errorf("summarize community: %w", err)
	}

	newKind := NodeKindLongTerm
	newLevel := level + 1

	consolidatedNode := MemoryNode{
		ID:            generateNodeID("lt"),
		Kind:          newKind,
		Status:        NodeStatusActive,
		Level:         newLevel,
		Summary:       buildSummaryText(summary),
		Who:           summary.Who,
		What:          summary.What,
		When:          summary.When,
		Where:         summary.Where,
		Why:           summary.Why,
		How:           summary.How,
		Result:        summary.Result,
		SourceNodeIDs: communityIDs,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}

	// Create the consolidated node FIRST — if this fails, source nodes stay active (safe).
	if err := s.insertNodeWithEdgesAndCommunityCheck(consolidatedNode, newKind, newLevel); err != nil {
		return fmt.Errorf("insert consolidated node: %w", err)
	}

	// Only mark source nodes after the new node is successfully persisted.
	markStatus := NodeStatusConsolidated
	if kind == NodeKindLongTerm {
		markStatus = NodeStatusArchived
	}
	for _, id := range communityIDs {
		if err := s.deactivateNode(id, markStatus); err != nil {
			return fmt.Errorf("deactivate node %s as %s: %w", id, markStatus, err)
		}
	}

	return nil
}

func (s *service) deactivateNode(nodeID string, status NodeStatus) error {
	if err := s.store.UpdateNodeStatus(nodeID, status); err != nil {
		return err
	}
	if err := s.vectorStore.Delete(vectorstore.DeleteRequest{
		StoreKind:  vectorstore.StoreKindText,
		ExternalID: nodeID,
	}); err != nil {
		revertErr := s.store.UpdateNodeStatus(nodeID, NodeStatusActive)
		if revertErr != nil {
			return fmt.Errorf("delete vector: %w (revert status: %v)", err, revertErr)
		}
		return fmt.Errorf("delete vector: %w", err)
	}
	return nil
}

func (s *service) embedWithType(text string, inputType provider.EmbeddingInputType) ([]float32, error) {
	if s.embedding == nil {
		return nil, fmt.Errorf("embedding provider is not configured")
	}

	resp, err := s.embedding.TextEmbeddings(provider.TextEmbeddingParams{
		Input:     []string{text},
		InputType: inputType,
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("embedding returned no data")
	}

	float64Values := resp.Data[0].Embedding.Values
	float32Values := make([]float32, len(float64Values))
	for i, v := range float64Values {
		float32Values[i] = float32(v)
	}
	return float32Values, nil
}

func (s *service) embed(text string) ([]float32, error) {
	return s.embedWithType(text, provider.EmbeddingInputTypeDocument)
}

func (s *service) embedQuery(text string) ([]float32, error) {
	return s.embedWithType(text, provider.EmbeddingInputTypeQuery)
}

func generateNodeID(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Fallback to a time-seeded pseudo-random source if crypto/rand fails.
		mathrand.Seed(time.Now().UnixNano())
		for i := range b {
			b[i] = byte(mathrand.Intn(256))
		}
	}
	return fmt.Sprintf("%s_%d_%s", prefix, time.Now().UnixMilli(), hex.EncodeToString(b))
}

func buildSummaryText(s *sessionSummaryOutput) string {
	parts := make([]string, 0, 7)
	if s.What != "" {
		parts = append(parts, s.What)
	}
	if s.Result != "" {
		parts = append(parts, "("+s.Result+")")
	}
	return strings.Join(parts, " ")
}

func formatSessionMessages(messages []openai.ChatCompletionMessage) string {
	var builder strings.Builder
	for _, msg := range messages {
		if msg.Content == "" && len(msg.ToolCalls) == 0 {
			continue
		}
		role := msg.Role
		if role == "" {
			role = "unknown"
		}
		if msg.Content != "" {
			builder.WriteString(role + ": " + msg.Content + "\n\n")
			continue
		}
		for _, tc := range msg.ToolCalls {
			name := tc.Function.Name
			args := tc.Function.Arguments
			if name == "" && args == "" {
				continue
			}
			builder.WriteString(fmt.Sprintf("%s: tool_call %s(%s)\n\n", role, name, args))
		}
	}
	return builder.String()
}
