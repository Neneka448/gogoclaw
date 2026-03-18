package context

import (
	"errors"
	"testing"

	"github.com/Neneka448/gogoclaw/internal/memory"
	"github.com/Neneka448/gogoclaw/internal/vectorstore"
	openai "github.com/sashabaranov/go-openai"
)

func TestRuntimeInitializerEnsureReadyStartsVectorStoreBeforeMemory(t *testing.T) {
	vectorStore := &fakeRuntimeInitializerVectorStore{}
	memoryService := &fakeRuntimeInitializerMemoryService{}

	initializer := NewRuntimeInitializer(SystemContext{
		VectorStore:   vectorStore,
		MemoryService: memoryService,
		MemoryEnabled: true,
	})

	if err := initializer.EnsureReady(); err != nil {
		t.Fatalf("EnsureReady() error = %v", err)
	}
	if vectorStore.startCalls != 1 {
		t.Fatalf("vectorStore.startCalls = %d, want 1", vectorStore.startCalls)
	}
	if memoryService.initializeCalls != 1 {
		t.Fatalf("memoryService.initializeCalls = %d, want 1", memoryService.initializeCalls)
	}
	if vectorStore.stopCalls != 0 {
		t.Fatalf("vectorStore.stopCalls = %d, want 0", vectorStore.stopCalls)
	}
	if got := vectorStore.events; len(got) != 1 || got[0] != "start" {
		t.Fatalf("vectorStore.events = %#v, want [start]", got)
	}
	if got := memoryService.events; len(got) != 1 || got[0] != "initialize" {
		t.Fatalf("memoryService.events = %#v, want [initialize]", got)
	}
}

func TestRuntimeInitializerEnsureReadyStopsVectorStoreOnMemoryFailure(t *testing.T) {
	vectorStore := &fakeRuntimeInitializerVectorStore{}
	memoryService := &fakeRuntimeInitializerMemoryService{
		initializeErr: errors.New("memory init failed"),
	}

	initializer := NewRuntimeInitializer(SystemContext{
		VectorStore:   vectorStore,
		MemoryService: memoryService,
		MemoryEnabled: true,
	})

	err := initializer.EnsureReady()
	if err == nil || err.Error() != "memory init failed" {
		t.Fatalf("EnsureReady() error = %v, want memory init failed", err)
	}
	if vectorStore.startCalls != 1 {
		t.Fatalf("vectorStore.startCalls = %d, want 1", vectorStore.startCalls)
	}
	if memoryService.initializeCalls != 1 {
		t.Fatalf("memoryService.initializeCalls = %d, want 1", memoryService.initializeCalls)
	}
	if vectorStore.stopCalls != 1 {
		t.Fatalf("vectorStore.stopCalls = %d, want 1", vectorStore.stopCalls)
	}
}

func TestRuntimeInitializerEnsureReadySkipsDisabledMemory(t *testing.T) {
	vectorStore := &fakeRuntimeInitializerVectorStore{}
	memoryService := &fakeRuntimeInitializerMemoryService{}

	initializer := NewRuntimeInitializer(SystemContext{
		VectorStore:   vectorStore,
		MemoryService: memoryService,
		MemoryEnabled: false,
	})

	if err := initializer.EnsureReady(); err != nil {
		t.Fatalf("EnsureReady() error = %v", err)
	}
	if vectorStore.startCalls != 1 {
		t.Fatalf("vectorStore.startCalls = %d, want 1", vectorStore.startCalls)
	}
	if memoryService.initializeCalls != 0 {
		t.Fatalf("memoryService.initializeCalls = %d, want 0", memoryService.initializeCalls)
	}
}

type fakeRuntimeInitializerVectorStore struct {
	startCalls int
	stopCalls  int
	events     []string
}

func (store *fakeRuntimeInitializerVectorStore) Start() error {
	store.startCalls++
	store.events = append(store.events, "start")
	return nil
}

func (store *fakeRuntimeInitializerVectorStore) Stop() error {
	store.stopCalls++
	store.events = append(store.events, "stop")
	return nil
}

func (store *fakeRuntimeInitializerVectorStore) Path() string {
	return ""
}

func (store *fakeRuntimeInitializerVectorStore) Upsert(request vectorstore.UpsertRequest) error {
	return nil
}

func (store *fakeRuntimeInitializerVectorStore) Delete(request vectorstore.DeleteRequest) error {
	return nil
}

func (store *fakeRuntimeInitializerVectorStore) SearchTopK(request vectorstore.SearchRequest) ([]vectorstore.SearchResult, error) {
	return nil, nil
}

func (store *fakeRuntimeInitializerVectorStore) SearchByThreshold(request vectorstore.ThresholdSearchRequest) ([]vectorstore.SearchResult, error) {
	return nil, nil
}

type fakeRuntimeInitializerMemoryService struct {
	initializeCalls int
	initializeErr   error
	events          []string
}

func (service *fakeRuntimeInitializerMemoryService) Initialize() error {
	service.initializeCalls++
	service.events = append(service.events, "initialize")
	return service.initializeErr
}

func (service *fakeRuntimeInitializerMemoryService) Close() error {
	service.events = append(service.events, "close")
	return nil
}

func (service *fakeRuntimeInitializerMemoryService) IngestSession(sessionID string, messages []openai.ChatCompletionMessage) error {
	return nil
}

func (service *fakeRuntimeInitializerMemoryService) Recall(queryText string, topK int, minSimilarity float64) ([]memory.MemoryNode, error) {
	return nil, nil
}

func (service *fakeRuntimeInitializerMemoryService) GetNode(nodeID string) (*memory.MemoryNode, error) {
	return nil, nil
}
