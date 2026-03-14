package gateway

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Neneka448/gogoclaw/internal/channels"
	"github.com/Neneka448/gogoclaw/internal/config"
	appcontext "github.com/Neneka448/gogoclaw/internal/context"
	"github.com/Neneka448/gogoclaw/internal/cron"
	"github.com/Neneka448/gogoclaw/internal/memory"
	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
	"github.com/Neneka448/gogoclaw/internal/provider"
	"github.com/Neneka448/gogoclaw/internal/session"
	"github.com/Neneka448/gogoclaw/internal/tools"
	"github.com/Neneka448/gogoclaw/internal/vectorstore"
	openai "github.com/sashabaranov/go-openai"
)

func TestGatewayStartDispatchesOutboundMessages(t *testing.T) {
	bus := messagebus.NewMessageBus()
	registry := channels.NewRegistry()
	fake := &channelsTestChannel{name: "cli", enabled: true}
	if err := registry.Register(fake); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	gw := NewGateway(appcontext.SystemContext{
		MessageBus:      bus,
		ChannelRegistry: registry,
	})
	if err := gw.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		if err := gw.Stop(); err != nil {
			t.Fatalf("Stop() error = %v", err)
		}
	})

	if err := bus.Put(messagebus.Message{ChannelID: "cli", Message: "hello", FinishReason: "stop"}, messagebus.OutboundQueue); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	deadline := time.After(2 * time.Second)
	for len(fake.snapshotReceived()) == 0 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for outbound dispatch")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	received := fake.snapshotReceived()
	if received[0].Message != "hello" {
		t.Fatalf("received[0].Message = %q, want hello", received[0].Message)
	}
}

func TestGatewayStopIsIdempotent(t *testing.T) {
	bus := messagebus.NewMessageBus()
	registry := channels.NewRegistry()
	if err := registry.Register(&channelsTestChannel{name: "cli", enabled: true}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	gw := NewGateway(appcontext.SystemContext{
		MessageBus:      bus,
		ChannelRegistry: registry,
	})
	if err := gw.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := gw.Stop(); err != nil {
		t.Fatalf("Stop() first error = %v", err)
	}
	if err := gw.Stop(); err != nil {
		t.Fatalf("Stop() second error = %v", err)
	}
}

func TestGatewayLogsBackgroundDispatchErrors(t *testing.T) {
	bus := messagebus.NewMessageBus()
	buffer := &bytes.Buffer{}
	gw := &gateway{
		context: appcontext.SystemContext{},
	}
	_ = bus
	stderr := osStderrSwap(buffer)
	defer stderr()

	gw.logBackgroundError("outbound", messagebus.Message{ChannelID: "feishu", ChatID: "oc_1", MessageID: "om_1"}, errors.New("boom"))
	if got := buffer.String(); got != "gateway outbound error: channel=feishu chat=oc_1 message_id=om_1 err=boom\n" {
		t.Fatalf("logBackgroundError() = %q", got)
	}
}

func TestGatewayCanRestartAfterStop(t *testing.T) {
	bus := messagebus.NewMessageBus()
	registry := channels.NewRegistry()
	fake := &channelsTestChannel{name: "cli", enabled: true}
	if err := registry.Register(fake); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	gw := NewGateway(appcontext.SystemContext{
		MessageBus:      bus,
		ChannelRegistry: registry,
	})
	if err := gw.Start(); err != nil {
		t.Fatalf("Start() first error = %v", err)
	}
	if err := gw.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	bus = messagebus.NewMessageBus()
	gw = NewGateway(appcontext.SystemContext{
		MessageBus:      bus,
		ChannelRegistry: registry,
	})
	if err := gw.Start(); err != nil {
		t.Fatalf("Start() second error = %v", err)
	}
	t.Cleanup(func() {
		_ = gw.Stop()
	})

	if err := bus.Put(messagebus.Message{ChannelID: "cli", Message: "restart", FinishReason: "stop"}, messagebus.OutboundQueue); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	deadline := time.After(2 * time.Second)
	for {
		received := fake.snapshotReceived()
		if len(received) > 0 && received[len(received)-1].Message == "restart" {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for restarted gateway dispatch")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestGatewayDirectProcessAndReturnInitializesMemoryRuntime(t *testing.T) {
	bus := messagebus.NewMessageBus()
	vectorStore := &fakeGatewayVectorStore{}
	memoryService := &fakeGatewayMemoryService{}
	providerStub := &fakeGatewayProvider{
		responses: []provider.LLMCommonResponse{provider.NormalizedResponse{Content: "done"}},
	}

	gw := NewGateway(appcontext.SystemContext{
		MessageBus:     bus,
		Provider:       providerStub,
		ConfigManager:  newGatewayTestConfigManager(t),
		ToolRegistry:   tools.NewToolRegistry(),
		SessionManager: session.NewSessionManager(t.TempDir()),
		VectorStore:    vectorStore,
		MemoryService:  memoryService,
		MemoryEnabled:  true,
	})
	t.Cleanup(func() {
		if err := gw.Stop(); err != nil {
			t.Fatalf("Stop() error = %v", err)
		}
	})

	responses, err := gw.DirectProcessAndReturn(messagebus.Message{
		ChannelID: "cli",
		ChatID:    "chat-1",
		SenderID:  "user-1",
		Message:   "hello",
	})
	if err != nil {
		t.Fatalf("DirectProcessAndReturn() error = %v", err)
	}
	if vectorStore.startCalls != 1 {
		t.Fatalf("vectorStore.startCalls = %d, want 1", vectorStore.startCalls)
	}
	if memoryService.initializeCalls != 1 {
		t.Fatalf("memoryService.initializeCalls = %d, want 1", memoryService.initializeCalls)
	}
	if len(responses) != 1 || responses[0].Message != "done" {
		t.Fatalf("responses = %#v, want single done message", responses)
	}
}

type channelsTestChannel struct {
	name     string
	enabled  bool
	received []messagebus.Message
	mu       sync.Mutex
}

type fakeGatewayCronManager struct {
	startCalls int
	stopCalls  int
}

type fakeGatewayProvider struct {
	responses []provider.LLMCommonResponse
}

type fakeGatewayVectorStore struct {
	startCalls int
	stopCalls  int
}

type fakeGatewayMemoryService struct {
	initializeCalls int
	mu              sync.Mutex
	sessionIDs      []string
}

type fakeGatewayCronService struct {
	manager   *fakeGatewayCronManager
	loadCalls int
	loadErr   error
}

func (manager *fakeGatewayCronManager) RegisterCron(cronTask cron.Cron) error {
	return nil
}

func (manager *fakeGatewayCronManager) GetCron(cronID string) (cron.Cron, error) {
	return nil, cron.ErrCronNotFound
}

func (manager *fakeGatewayCronManager) DeleteCron(cronID string) error {
	return nil
}

func (manager *fakeGatewayCronManager) Start() error {
	manager.startCalls++
	return nil
}

func (manager *fakeGatewayCronManager) Stop() error {
	manager.stopCalls++
	return nil
}

func (provider *fakeGatewayProvider) ChatCompletion(request openai.ChatCompletionRequest) (provider.LLMCommonResponse, error) {
	response := provider.responses[0]
	provider.responses = provider.responses[1:]
	return response, nil
}

func (store *fakeGatewayVectorStore) Start() error {
	store.startCalls++
	return nil
}

func (store *fakeGatewayVectorStore) Stop() error {
	store.stopCalls++
	return nil
}

func (store *fakeGatewayVectorStore) Path() string {
	return ""
}

func (store *fakeGatewayVectorStore) DB() *sql.DB {
	return nil
}

func (store *fakeGatewayVectorStore) Upsert(request vectorstore.UpsertRequest) error {
	return nil
}

func (store *fakeGatewayVectorStore) Delete(request vectorstore.DeleteRequest) error {
	return nil
}

func (store *fakeGatewayVectorStore) SearchTopK(request vectorstore.SearchRequest) ([]vectorstore.SearchResult, error) {
	return nil, nil
}

func (store *fakeGatewayVectorStore) SearchByThreshold(request vectorstore.ThresholdSearchRequest) ([]vectorstore.SearchResult, error) {
	return nil, nil
}

func (service *fakeGatewayMemoryService) Initialize() error {
	service.initializeCalls++
	return nil
}

func (service *fakeGatewayMemoryService) IngestSession(sessionID string, messages []openai.ChatCompletionMessage) error {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.sessionIDs = append(service.sessionIDs, sessionID)
	return nil
}

func (service *fakeGatewayMemoryService) Recall(queryText string, topK int, minSimilarity float64) ([]memory.MemoryNode, error) {
	return nil, nil
}

func (service *fakeGatewayMemoryService) GetNode(nodeID string) (*memory.MemoryNode, error) {
	return nil, nil
}

func (service *fakeGatewayCronService) EnsureRoot() error {
	return nil
}

func (service *fakeGatewayCronService) LoadAll() error {
	service.loadCalls++
	return service.loadErr
}

func (service *fakeGatewayCronService) Start() error {
	if service.manager == nil {
		return nil
	}
	return service.manager.Start()
}

func (service *fakeGatewayCronService) Stop() error {
	if service.manager == nil {
		return nil
	}
	return service.manager.Stop()
}

func (service *fakeGatewayCronService) ListCrons() ([]cron.StoredCron, error) {
	return nil, nil
}

func (service *fakeGatewayCronService) GetCron(cronID string) (*cron.StoredCron, error) {
	return nil, cron.ErrCronNotFound
}

func (service *fakeGatewayCronService) CreateCron(input cron.UpsertCronInput) (*cron.StoredCron, error) {
	return nil, nil
}

func (service *fakeGatewayCronService) UpdateCron(input cron.UpsertCronInput) (*cron.StoredCron, error) {
	return nil, nil
}

func (service *fakeGatewayCronService) DeleteCron(cronID string) error {
	return nil
}

func (service *fakeGatewayCronService) ExecuteCron(cronID string) error {
	return nil
}

func (c *channelsTestChannel) Name() string {
	return c.name
}

func (c *channelsTestChannel) Enabled() bool {
	return c.enabled
}

func (c *channelsTestChannel) Start() error {
	return nil
}

func (c *channelsTestChannel) Stop() error {
	return nil
}

func (c *channelsTestChannel) Send(message messagebus.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.received = append(c.received, message)
	return nil
}

func (c *channelsTestChannel) snapshotReceived() []messagebus.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]messagebus.Message(nil), c.received...)
}

type blockingGatewayProvider struct {
	mu            sync.Mutex
	requests      []openai.ChatCompletionRequest
	firstStarted  chan struct{}
	secondStarted chan struct{}
	releaseFirst  chan struct{}
}

type backlogGatewayProvider struct {
	mu            sync.Mutex
	requests      []string
	sharedStarted chan struct{}
	otherStarted  chan struct{}
	releaseShared chan struct{}
}

func newBlockingGatewayProvider() *blockingGatewayProvider {
	return &blockingGatewayProvider{
		firstStarted:  make(chan struct{}),
		secondStarted: make(chan struct{}),
		releaseFirst:  make(chan struct{}),
	}
}

func newBacklogGatewayProvider() *backlogGatewayProvider {
	return &backlogGatewayProvider{
		sharedStarted: make(chan struct{}),
		otherStarted:  make(chan struct{}),
		releaseShared: make(chan struct{}),
	}
}

func (stub *blockingGatewayProvider) ChatCompletion(request openai.ChatCompletionRequest) (provider.LLMCommonResponse, error) {
	stub.mu.Lock()
	callIndex := len(stub.requests)
	stub.requests = append(stub.requests, request)
	stub.mu.Unlock()

	switch callIndex {
	case 0:
		close(stub.firstStarted)
		<-stub.releaseFirst
	case 1:
		close(stub.secondStarted)
	}

	return provider.NormalizedResponse{Content: "done"}, nil
}

func (stub *blockingGatewayProvider) Requests() []openai.ChatCompletionRequest {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	requests := make([]openai.ChatCompletionRequest, len(stub.requests))
	copy(requests, stub.requests)
	return requests
}

func (stub *backlogGatewayProvider) ChatCompletion(request openai.ChatCompletionRequest) (provider.LLMCommonResponse, error) {
	content := latestUserMessage(request)

	stub.mu.Lock()
	stub.requests = append(stub.requests, content)
	stub.mu.Unlock()

	switch content {
	case "shared-0":
		close(stub.sharedStarted)
		<-stub.releaseShared
	case "other":
		close(stub.otherStarted)
	}

	return provider.NormalizedResponse{Content: "done"}, nil
}

func latestUserMessage(request openai.ChatCompletionRequest) string {
	for i := len(request.Messages) - 1; i >= 0; i-- {
		if request.Messages[i].Role == openai.ChatMessageRoleUser {
			return request.Messages[i].Content
		}
	}
	return ""
}

func closeIfOpen(ch chan struct{}) {
	select {
	case <-ch:
	default:
		close(ch)
	}
}

func osStderrSwap(buffer *bytes.Buffer) func() {
	original := stderrWriter
	stderrWriter = buffer
	return func() {
		stderrWriter = original
	}
}

func newGatewayTestConfigManager(t *testing.T) config.ConfigManager {
	t.Helper()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	defaultConfig := config.CreateDefaultConfig()
	defaultConfig.Agents.Profiles["default"] = config.ProfileConfig{
		Workspace:         tempDir,
		Provider:          "codex",
		Model:             "gpt-5.4",
		MaxTokens:         512,
		Temperature:       0.1,
		MaxToolIterations: 4,
		MemoryWindow:      10,
		MaxRetryTimes:     1,
	}

	encoded, err := json.Marshal(defaultConfig)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(configPath, encoded, 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	return config.NewConfigManager(configPath)
}

func TestGatewayStartsAndStopsCronManager(t *testing.T) {
	bus := messagebus.NewMessageBus()
	registry := channels.NewRegistry()
	manager := &fakeGatewayCronManager{}
	service := &fakeGatewayCronService{manager: manager}

	gw := NewGateway(appcontext.SystemContext{
		MessageBus:      bus,
		ChannelRegistry: registry,
		CronService:     service,
		CronEnabled:     true,
	})
	if err := gw.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if service.loadCalls != 1 {
		t.Fatalf("service.loadCalls = %d, want 1", service.loadCalls)
	}
	if manager.startCalls != 1 {
		t.Fatalf("manager.startCalls = %d, want 1", manager.startCalls)
	}
	if err := gw.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if manager.stopCalls != 1 {
		t.Fatalf("manager.stopCalls = %d, want 1", manager.stopCalls)
	}
}

func TestGatewaySerializesInboundMessagesPerSession(t *testing.T) {
	workspace := t.TempDir()
	bus := messagebus.NewMessageBus()
	providerStub := newBlockingGatewayProvider()
	sessionManager := session.NewSessionManager(workspace)

	gw := NewGateway(appcontext.SystemContext{
		MessageBus:     bus,
		Provider:       providerStub,
		ConfigManager:  newGatewayTestConfigManager(t),
		ToolRegistry:   tools.NewToolRegistry(),
		SessionManager: sessionManager,
	})
	if err := gw.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		if err := gw.Stop(); err != nil {
			t.Fatalf("Stop() error = %v", err)
		}
	})

	first := messagebus.Message{ChannelID: "cli", ChatID: "shared", SenderID: "user-1", Message: "first"}
	second := messagebus.Message{ChannelID: "cli", ChatID: "shared", SenderID: "user-1", Message: "second"}
	if err := bus.Put(first, messagebus.InboundQueue); err != nil {
		t.Fatalf("Put(first) error = %v", err)
	}
	select {
	case <-providerStub.firstStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first request")
	}

	if err := bus.Put(second, messagebus.InboundQueue); err != nil {
		t.Fatalf("Put(second) error = %v", err)
	}
	select {
	case <-providerStub.secondStarted:
		t.Fatal("second request started before first finished")
	case <-time.After(200 * time.Millisecond):
	}

	close(providerStub.releaseFirst)
	select {
	case <-providerStub.secondStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for second request")
	}

	deadline := time.After(2 * time.Second)
	for {
		currentSession, err := sessionManager.GetOrCreateSession(session.MakeSessionID("cli", "shared"), "user-1")
		if err != nil {
			t.Fatalf("GetOrCreateSession() error = %v", err)
		}
		messages := currentSession.GetMessages(10)
		if len(messages) == 4 {
			if messages[0].Content != "first" || messages[1].Content != "done" || messages[2].Content != "second" || messages[3].Content != "done" {
				t.Fatalf("messages = %#v, want serialized first/done/second/done", messages)
			}
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for serialized session, messages = %#v", messages)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestGatewayBackloggedSessionDoesNotBlockOtherSessions(t *testing.T) {
	workspace := t.TempDir()
	bus := messagebus.NewMessageBus()
	providerStub := newBacklogGatewayProvider()
	sessionManager := session.NewSessionManager(workspace)

	gw := NewGateway(appcontext.SystemContext{
		MessageBus:     bus,
		Provider:       providerStub,
		ConfigManager:  newGatewayTestConfigManager(t),
		ToolRegistry:   tools.NewToolRegistry(),
		SessionManager: sessionManager,
	}).(*gateway)
	gw.workerCount = 2
	if err := gw.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		closeIfOpen(providerStub.releaseShared)
		if err := gw.Stop(); err != nil {
			t.Fatalf("Stop() error = %v", err)
		}
	})

	if err := bus.Put(messagebus.Message{ChannelID: "cli", ChatID: "shared", SenderID: "user-1", Message: "shared-0"}, messagebus.InboundQueue); err != nil {
		t.Fatalf("Put(shared-0) error = %v", err)
	}
	select {
	case <-providerStub.sharedStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for blocked shared session")
	}

	for i := 1; i <= 40; i++ {
		if err := bus.Put(messagebus.Message{
			ChannelID: "cli",
			ChatID:    "shared",
			SenderID:  "user-1",
			Message:   "shared-" + strconv.Itoa(i),
		}, messagebus.InboundQueue); err != nil {
			t.Fatalf("Put(shared-%d) error = %v", i, err)
		}
	}
	if err := bus.Put(messagebus.Message{ChannelID: "cli", ChatID: "other", SenderID: "user-2", Message: "other"}, messagebus.InboundQueue); err != nil {
		t.Fatalf("Put(other) error = %v", err)
	}

	select {
	case <-providerStub.otherStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("other session was blocked by shared backlog")
	}
}

func TestGatewayStopWaitsForActiveSessionWorkers(t *testing.T) {
	workspace := t.TempDir()
	bus := messagebus.NewMessageBus()
	registry := channels.NewRegistry()
	channel := &channelsTestChannel{name: "cli", enabled: true}
	if err := registry.Register(channel); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	providerStub := newBlockingGatewayProvider()
	sessionManager := session.NewSessionManager(workspace)

	gw := NewGateway(appcontext.SystemContext{
		MessageBus:      bus,
		Provider:        providerStub,
		ConfigManager:   newGatewayTestConfigManager(t),
		ToolRegistry:    tools.NewToolRegistry(),
		SessionManager:  sessionManager,
		ChannelRegistry: registry,
	})
	if err := gw.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if err := bus.Put(messagebus.Message{
		ChannelID: "cli",
		ChatID:    "shutdown",
		SenderID:  "user-1",
		Message:   "finish before stop",
	}, messagebus.InboundQueue); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	select {
	case <-providerStub.firstStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first request")
	}

	stopErrCh := make(chan error, 1)
	go func() {
		stopErrCh <- gw.Stop()
	}()

	select {
	case err := <-stopErrCh:
		t.Fatalf("Stop() returned too early: %v", err)
	case <-time.After(200 * time.Millisecond):
	}

	close(providerStub.releaseFirst)
	select {
	case err := <-stopErrCh:
		if err != nil {
			t.Fatalf("Stop() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Stop()")
	}

	channel.mu.Lock()
	received := append([]messagebus.Message(nil), channel.received...)
	channel.mu.Unlock()
	if len(received) == 0 || received[len(received)-1].Message != "done" {
		t.Fatalf("received = %#v, want final outbound reply", received)
	}

	currentSession, err := sessionManager.GetOrCreateSession(session.MakeSessionID("cli", "shutdown"), "user-1")
	if err != nil {
		t.Fatalf("GetOrCreateSession() error = %v", err)
	}
	messages := currentSession.GetMessages(10)
	if len(messages) != 2 || messages[0].Content != "finish before stop" || messages[1].Content != "done" {
		t.Fatalf("messages = %#v, want persisted user/assistant pair", messages)
	}
}

func TestGatewayEnsureRuntimeReadySyncsDirtySessionsToMemory(t *testing.T) {
	workspace := t.TempDir()
	sessionManager := session.NewSessionManager(workspace)
	currentSession, err := sessionManager.GetOrCreateSession(session.MakeSessionID("cli", "recover"), "user-1")
	if err != nil {
		t.Fatalf("GetOrCreateSession() error = %v", err)
	}
	if err := currentSession.AppendMessages([]openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "remember this"},
		{Role: openai.ChatMessageRoleAssistant, Content: "stored"},
	}); err != nil {
		t.Fatalf("AppendMessages() error = %v", err)
	}
	if err := currentSession.WriteSessionFile(); err != nil {
		t.Fatalf("WriteSessionFile() error = %v", err)
	}

	memoryService := &fakeGatewayMemoryService{}
	gw := &gateway{
		context: appcontext.SystemContext{
			SessionManager: sessionManager,
			MemoryService:  memoryService,
			MemoryEnabled:  true,
			VectorStore:    &fakeGatewayVectorStore{},
		},
	}

	if err := gw.ensureRuntimeReady(); err != nil {
		t.Fatalf("ensureRuntimeReady() error = %v", err)
	}
	if memoryService.initializeCalls != 1 {
		t.Fatalf("initializeCalls = %d, want 1", memoryService.initializeCalls)
	}
	if len(memoryService.sessionIDs) != 1 || memoryService.sessionIDs[0] != "cli:recover" {
		t.Fatalf("sessionIDs = %#v, want [\"cli:recover\"]", memoryService.sessionIDs)
	}

	if err := gw.ensureRuntimeReady(); err != nil {
		t.Fatalf("second ensureRuntimeReady() error = %v", err)
	}
	if len(memoryService.sessionIDs) != 1 {
		t.Fatalf("len(sessionIDs) = %d, want 1 after digest guard", len(memoryService.sessionIDs))
	}
}
