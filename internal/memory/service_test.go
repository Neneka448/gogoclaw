package memory

import (
	"database/sql"
	"testing"

	"github.com/Neneka448/gogoclaw/internal/config"
	"github.com/Neneka448/gogoclaw/internal/provider"
	"github.com/Neneka448/gogoclaw/internal/vectorstore"
	openai "github.com/sashabaranov/go-openai"
)

type fakeMemoryVectorStore struct {
	upserted []string
	deleted  []string
}

func (store *fakeMemoryVectorStore) Start() error {
	return nil
}

func (store *fakeMemoryVectorStore) Stop() error {
	return nil
}

func (store *fakeMemoryVectorStore) Path() string {
	return ""
}

func (store *fakeMemoryVectorStore) DB() *sql.DB {
	return nil
}

func (store *fakeMemoryVectorStore) Upsert(request vectorstore.UpsertRequest) error {
	store.upserted = append(store.upserted, request.ExternalID)
	return nil
}

func (store *fakeMemoryVectorStore) Delete(request vectorstore.DeleteRequest) error {
	store.deleted = append(store.deleted, request.ExternalID)
	return nil
}

func (store *fakeMemoryVectorStore) SearchTopK(request vectorstore.SearchRequest) ([]vectorstore.SearchResult, error) {
	return nil, nil
}

func (store *fakeMemoryVectorStore) SearchByThreshold(request vectorstore.ThresholdSearchRequest) ([]vectorstore.SearchResult, error) {
	return nil, nil
}

type fakeMemoryEmbeddingProvider struct {
	lastTextParams provider.TextEmbeddingParams
}

func (embeddingProvider *fakeMemoryEmbeddingProvider) TextEmbeddings(params provider.TextEmbeddingParams) (*provider.EmbeddingResponse, error) {
	embeddingProvider.lastTextParams = params
	return &provider.EmbeddingResponse{
		Data: []provider.EmbeddingData{{
			Embedding: provider.EmbeddingVector{Values: []float64{1, 0, 0}},
		}},
	}, nil
}

func (embeddingProvider *fakeMemoryEmbeddingProvider) MultimodalEmbeddings(params provider.MultimodalEmbeddingParams) (*provider.EmbeddingResponse, error) {
	return nil, nil
}

type fakeMemoryLLM struct {
	content string
}

func (llm *fakeMemoryLLM) ChatCompletion(request openai.ChatCompletionRequest) (provider.LLMCommonResponse, error) {
	return provider.NormalizedResponse{Content: llm.content}, nil
}

func TestServiceConsolidateCommunityDeletesSourceVectors(t *testing.T) {
	store := newTestStore(t)
	for _, node := range []MemoryNode{
		{ID: "st-1", Kind: NodeKindShortTerm, Status: NodeStatusActive, Level: 0, What: "fix auth bug", Summary: "fix auth bug"},
		{ID: "st-2", Kind: NodeKindShortTerm, Status: NodeStatusActive, Level: 0, What: "fix auth bug again", Summary: "fix auth bug again"},
	} {
		if err := store.InsertNode(node); err != nil {
			t.Fatalf("InsertNode(%s) error = %v", node.ID, err)
		}
	}

	vectorStore := &fakeMemoryVectorStore{}
	cfg := config.CreateDefaultConfig().Memory
	svc := &service{
		store:         store,
		vectorStore:   vectorStore,
		embedding:     &fakeMemoryEmbeddingProvider{},
		textEmbedding: config.EmbeddingModelConfig{Model: "voyage-4-large", OutputDimension: 1024},
		summarizer: NewSummarizer(&fakeMemoryLLM{
			content: `{"who":"user","what":"repeated auth fixes","when":"recurring","where":"repo","why":"stabilize auth","how":"compare the failing cases and reuse the fix path","result":"mostly successful"}`,
		}, "test"),
		config: cfg,
	}

	if err := svc.consolidateCommunity([]string{"st-1", "st-2"}, NodeKindShortTerm, 0); err != nil {
		t.Fatalf("consolidateCommunity() error = %v", err)
	}

	if len(vectorStore.upserted) != 1 {
		t.Fatalf("len(vectorStore.upserted) = %d, want 1", len(vectorStore.upserted))
	}
	if len(vectorStore.deleted) != 2 {
		t.Fatalf("len(vectorStore.deleted) = %d, want 2", len(vectorStore.deleted))
	}

	for _, id := range []string{"st-1", "st-2"} {
		node, err := store.GetNode(id)
		if err != nil {
			t.Fatalf("GetNode(%s) error = %v", id, err)
		}
		if node == nil || node.Status != NodeStatusConsolidated {
			t.Fatalf("node %s status = %v, want consolidated", id, node.Status)
		}
	}

	activeLongTerm, err := store.ListActiveNodesByKindAndLevel(NodeKindLongTerm, 1)
	if err != nil {
		t.Fatalf("ListActiveNodesByKindAndLevel() error = %v", err)
	}
	if len(activeLongTerm) != 1 {
		t.Fatalf("len(activeLongTerm) = %d, want 1", len(activeLongTerm))
	}
}

func TestServiceInitializeRepairsActiveVectors(t *testing.T) {
	store := newTestStore(t)
	if err := store.InsertNode(MemoryNode{
		ID:      "st-repair",
		Kind:    NodeKindShortTerm,
		Status:  NodeStatusActive,
		Level:   0,
		What:    "repair missing vector",
		Summary: "repair missing vector",
	}); err != nil {
		t.Fatalf("InsertNode() error = %v", err)
	}

	vectorStore := &fakeMemoryVectorStore{}
	svc := &service{
		store:         store,
		vectorStore:   vectorStore,
		embedding:     &fakeMemoryEmbeddingProvider{},
		textEmbedding: config.EmbeddingModelConfig{Model: "voyage-4-large", OutputDimension: 1024},
		summarizer:    NewSummarizer(&fakeMemoryLLM{content: `{"who":"user","what":"repair","when":"now","where":"repo","why":"test","how":"repair active vectors","result":"success"}`}, "test"),
		config:        config.CreateDefaultConfig().Memory,
	}

	if err := svc.Initialize(); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if err := svc.Initialize(); err != nil {
		t.Fatalf("second Initialize() error = %v", err)
	}
	if len(vectorStore.upserted) != 1 || vectorStore.upserted[0] != "st-repair" {
		t.Fatalf("vectorStore.upserted = %#v, want [\"st-repair\"]", vectorStore.upserted)
	}
}

func TestServiceEmbedWithTypePassesConfiguredTextEmbedding(t *testing.T) {
	embeddingProvider := &fakeMemoryEmbeddingProvider{}
	svc := &service{
		embedding:     embeddingProvider,
		textEmbedding: config.EmbeddingModelConfig{Model: "voyage-4-large", OutputDimension: 1024},
	}

	vector, err := svc.embedWithType("remember this", provider.EmbeddingInputTypeQuery)
	if err != nil {
		t.Fatalf("embedWithType() error = %v", err)
	}
	if len(vector) != 3 {
		t.Fatalf("len(vector) = %d, want 3", len(vector))
	}
	if embeddingProvider.lastTextParams.Model != "voyage-4-large" {
		t.Fatalf("lastTextParams.Model = %q, want voyage-4-large", embeddingProvider.lastTextParams.Model)
	}
	if embeddingProvider.lastTextParams.InputType != provider.EmbeddingInputTypeQuery {
		t.Fatalf("lastTextParams.InputType = %q, want query", embeddingProvider.lastTextParams.InputType)
	}
	if len(embeddingProvider.lastTextParams.Input) != 1 || embeddingProvider.lastTextParams.Input[0] != "remember this" {
		t.Fatalf("lastTextParams.Input = %#v, want [\"remember this\"]", embeddingProvider.lastTextParams.Input)
	}
	if embeddingProvider.lastTextParams.OutputDimension == nil || *embeddingProvider.lastTextParams.OutputDimension != 1024 {
		t.Fatalf("lastTextParams.OutputDimension = %#v, want 1024", embeddingProvider.lastTextParams.OutputDimension)
	}
}

func TestServiceEmbedWithTypeRejectsMissingTextEmbeddingModel(t *testing.T) {
	svc := &service{
		embedding:     &fakeMemoryEmbeddingProvider{},
		textEmbedding: config.EmbeddingModelConfig{},
	}

	_, err := svc.embedWithType("remember this", provider.EmbeddingInputTypeDocument)
	if err == nil {
		t.Fatal("embedWithType() error = nil, want missing model error")
	}
	if err.Error() != "text embedding model is not configured" {
		t.Fatalf("embedWithType() error = %q, want text embedding model is not configured", err.Error())
	}
}
