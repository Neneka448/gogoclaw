package gateway

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Neneka448/gogoclaw/internal/channels"
	appcontext "github.com/Neneka448/gogoclaw/internal/context"
	"github.com/Neneka448/gogoclaw/internal/cron"
	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
)

func TestNewGatewayRequiresInvoker(t *testing.T) {
	if _, err := NewGateway(appcontext.SystemContext{}); err == nil {
		t.Fatal("NewGateway() error = nil, want invoker required")
	}
}

func TestGatewayStartDispatchesOutboundMessages(t *testing.T) {
	bus := messagebus.NewMessageBus()
	registry := channels.NewRegistry()
	fake := &channelsTestChannel{name: "cli", enabled: true}
	if err := registry.Register(fake); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	gw := mustNewGateway(t, appcontext.SystemContext{
		MessageBus:      bus,
		ChannelRegistry: registry,
		Invoker:         &fakeGatewayInvoker{},
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

	gw := mustNewGateway(t, appcontext.SystemContext{
		MessageBus:      bus,
		ChannelRegistry: registry,
		Invoker:         &fakeGatewayInvoker{},
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
	buffer := &bytes.Buffer{}
	gw := &gateway{context: appcontext.SystemContext{}}
	restoreStderr := osStderrSwap(buffer)
	defer restoreStderr()

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

	gw := mustNewGateway(t, appcontext.SystemContext{
		MessageBus:      bus,
		ChannelRegistry: registry,
		Invoker:         &fakeGatewayInvoker{},
	})
	if err := gw.Start(); err != nil {
		t.Fatalf("Start() first error = %v", err)
	}
	if err := gw.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	bus = messagebus.NewMessageBus()
	gw = mustNewGateway(t, appcontext.SystemContext{
		MessageBus:      bus,
		ChannelRegistry: registry,
		Invoker:         &fakeGatewayInvoker{},
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

func TestGatewayDirectProcessAndReturnDelegatesForegroundInvocation(t *testing.T) {
	bus := messagebus.NewMessageBus()
	invoker := &fakeGatewayInvoker{
		invokeAsyncFunc: func(request appcontext.InvocationRequest) (<-chan error, error) {
			errCh := make(chan error, 1)
			go func() {
				err := bus.Put(messagebus.Message{
					ChannelID:    request.Message.ChannelID,
					ChatID:       request.Message.ChatID,
					SenderID:     "assistant",
					Message:      "done",
					FinishReason: "stop",
				}, messagebus.OutboundQueue)
				errCh <- err
			}()
			return errCh, nil
		},
	}

	gw := mustNewGateway(t, appcontext.SystemContext{
		MessageBus: bus,
		Invoker:    invoker,
	})
	t.Cleanup(func() {
		_ = gw.Stop()
	})

	responses, err := gw.DirectProcessAndReturn(messagebus.Message{
		ChannelID: "cli",
		ChatID:    "chat-1",
		SenderID:  "user-1",
		Message:   "hello",
		Metadata:  map[string]string{"agent_profile": "worker"},
	})
	if err != nil {
		t.Fatalf("DirectProcessAndReturn() error = %v", err)
	}
	if len(responses) != 1 || responses[0].Message != "done" {
		t.Fatalf("responses = %#v, want single done message", responses)
	}
	if len(invoker.invokeAsyncCalls) != 1 {
		t.Fatalf("len(invoker.invokeAsyncCalls) = %d, want 1", len(invoker.invokeAsyncCalls))
	}

	request := invoker.invokeAsyncCalls[0]
	if request.ProfileName != "worker" {
		t.Fatalf("request.ProfileName = %q, want worker", request.ProfileName)
	}
	if request.Mode != appcontext.InvocationModeForeground {
		t.Fatalf("request.Mode = %q, want foreground", request.Mode)
	}
	if len(invoker.ensureCalls) != 0 {
		t.Fatalf("invoker.ensureCalls = %#v, want no eager profile warmup", invoker.ensureCalls)
	}
}

func TestGatewayDirectProcessAndReturnReturnsInvokerError(t *testing.T) {
	bus := messagebus.NewMessageBus()
	invoker := &fakeGatewayInvoker{
		invokeAsyncFunc: func(request appcontext.InvocationRequest) (<-chan error, error) {
			errCh := make(chan error, 1)
			errCh <- errors.New("boom")
			return errCh, nil
		},
	}

	gw := mustNewGateway(t, appcontext.SystemContext{
		MessageBus: bus,
		Invoker:    invoker,
	})
	t.Cleanup(func() {
		_ = gw.Stop()
	})

	_, err := gw.DirectProcessAndReturn(messagebus.Message{
		ChannelID: "cli",
		ChatID:    "chat-1",
		SenderID:  "user-1",
		Message:   "hello",
	})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("DirectProcessAndReturn() error = %v, want boom", err)
	}
}

func TestGatewayStartsAndStopsCronManager(t *testing.T) {
	bus := messagebus.NewMessageBus()
	registry := channels.NewRegistry()
	manager := &fakeGatewayCronManager{}
	service := &fakeGatewayCronService{manager: manager}
	invoker := &fakeGatewayInvoker{}

	gw := mustNewGateway(t, appcontext.SystemContext{
		MessageBus:      bus,
		ChannelRegistry: registry,
		CronService:     service,
		CronEnabled:     true,
		Invoker:         invoker,
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
	if len(invoker.ensureCalls) != 1 || invoker.ensureCalls[0] != "default" {
		t.Fatalf("invoker.ensureCalls = %#v, want [default]", invoker.ensureCalls)
	}
	if err := gw.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if manager.stopCalls != 1 {
		t.Fatalf("manager.stopCalls = %d, want 1", manager.stopCalls)
	}
}

func TestGatewayStartClosesInvokerWhenCronLoadFails(t *testing.T) {
	bus := messagebus.NewMessageBus()
	invoker := &fakeGatewayInvoker{}
	service := &fakeGatewayCronService{loadErr: errors.New("load failed")}

	gw := mustNewGateway(t, appcontext.SystemContext{
		MessageBus:  bus,
		CronService: service,
		CronEnabled: true,
		Invoker:     invoker,
	})
	err := gw.Start()
	if err == nil || !strings.Contains(err.Error(), "load failed") {
		t.Fatalf("Start() error = %v, want load failed", err)
	}
	if invoker.closeCalls != 1 {
		t.Fatalf("invoker.closeCalls = %d, want 1", invoker.closeCalls)
	}
	if len(invoker.ensureCalls) != 1 || invoker.ensureCalls[0] != "default" {
		t.Fatalf("invoker.ensureCalls = %#v, want [default]", invoker.ensureCalls)
	}
}

func TestGatewaySerializesInboundMessagesPerSession(t *testing.T) {
	bus := messagebus.NewMessageBus()
	invoker := newBlockingGatewayInvoker(nil)

	gw := mustNewGateway(t, appcontext.SystemContext{
		MessageBus: bus,
		Invoker:    invoker,
	})
	if err := gw.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		closeIfOpen(invoker.releaseFirst)
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
	case <-invoker.firstStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first request")
	}

	if err := bus.Put(second, messagebus.InboundQueue); err != nil {
		t.Fatalf("Put(second) error = %v", err)
	}
	select {
	case <-invoker.secondStarted:
		t.Fatal("second request started before first finished")
	case <-time.After(200 * time.Millisecond):
	}

	close(invoker.releaseFirst)
	select {
	case <-invoker.secondStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for second request")
	}

	requests := invoker.snapshotRequests()
	if len(requests) != 2 {
		t.Fatalf("len(requests) = %d, want 2", len(requests))
	}
	if requests[0].Mode != appcontext.InvocationModeBackground || requests[1].Mode != appcontext.InvocationModeBackground {
		t.Fatalf("requests modes = %#v, want background invocations", requests)
	}
}

func TestGatewayBackloggedSessionDoesNotBlockOtherSessions(t *testing.T) {
	bus := messagebus.NewMessageBus()
	invoker := newBacklogGatewayInvoker()

	gw := mustNewGateway(t, appcontext.SystemContext{
		MessageBus: bus,
		Invoker:    invoker,
	}).(*gateway)
	gw.workerCount = 2
	if err := gw.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		closeIfOpen(invoker.releaseShared)
		if err := gw.Stop(); err != nil {
			t.Fatalf("Stop() error = %v", err)
		}
	})

	if err := bus.Put(messagebus.Message{ChannelID: "cli", ChatID: "shared", SenderID: "user-1", Message: "shared-0"}, messagebus.InboundQueue); err != nil {
		t.Fatalf("Put(shared-0) error = %v", err)
	}
	select {
	case <-invoker.sharedStarted:
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
	case <-invoker.otherStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("other session was blocked by shared backlog")
	}
}

func TestGatewayStopWaitsForActiveSessionWorkers(t *testing.T) {
	bus := messagebus.NewMessageBus()
	registry := channels.NewRegistry()
	channel := &channelsTestChannel{name: "cli", enabled: true}
	if err := registry.Register(channel); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	invoker := newBlockingGatewayInvoker(bus)

	gw := mustNewGateway(t, appcontext.SystemContext{
		MessageBus:      bus,
		ChannelRegistry: registry,
		Invoker:         invoker,
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
	case <-invoker.firstStarted:
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

	close(invoker.releaseFirst)
	select {
	case err := <-stopErrCh:
		if err != nil {
			t.Fatalf("Stop() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Stop()")
	}

	received := channel.snapshotReceived()
	if len(received) == 0 || received[len(received)-1].Message != "done" {
		t.Fatalf("received = %#v, want final outbound reply", received)
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

type fakeGatewayCronService struct {
	manager   *fakeGatewayCronManager
	loadCalls int
	loadErr   error
}

type fakeGatewayInvoker struct {
	mu               sync.Mutex
	ensureCalls      []string
	closeCalls       int
	invokeCalls      []appcontext.InvocationRequest
	invokeAsyncCalls []appcontext.InvocationRequest
	invokeFunc       func(request appcontext.InvocationRequest) error
	invokeAsyncFunc  func(request appcontext.InvocationRequest) (<-chan error, error)
	ensureFunc       func(profileName string) error
	closeFunc        func() error
}

type blockingGatewayInvoker struct {
	mu            sync.Mutex
	requests      []appcontext.InvocationRequest
	firstStarted  chan struct{}
	secondStarted chan struct{}
	releaseFirst  chan struct{}
	bus           messagebus.MessageBus
}

type backlogGatewayInvoker struct {
	mu            sync.Mutex
	requests      []string
	sharedStarted chan struct{}
	otherStarted  chan struct{}
	releaseShared chan struct{}
}

func mustNewGateway(t *testing.T, systemContext appcontext.SystemContext) Gateway {
	t.Helper()

	gw, err := NewGateway(systemContext)
	if err != nil {
		t.Fatalf("NewGateway() error = %v", err)
	}
	return gw
}

func newBlockingGatewayInvoker(bus messagebus.MessageBus) *blockingGatewayInvoker {
	return &blockingGatewayInvoker{
		firstStarted:  make(chan struct{}),
		secondStarted: make(chan struct{}),
		releaseFirst:  make(chan struct{}),
		bus:           bus,
	}
}

func newBacklogGatewayInvoker() *backlogGatewayInvoker {
	return &backlogGatewayInvoker{
		sharedStarted: make(chan struct{}),
		otherStarted:  make(chan struct{}),
		releaseShared: make(chan struct{}),
	}
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

func (invoker *fakeGatewayInvoker) Invoke(request appcontext.InvocationRequest) error {
	invoker.mu.Lock()
	invoker.invokeCalls = append(invoker.invokeCalls, request)
	invokeFunc := invoker.invokeFunc
	invoker.mu.Unlock()
	if invokeFunc == nil {
		return nil
	}
	return invokeFunc(request)
}

func (invoker *fakeGatewayInvoker) InvokeAsync(request appcontext.InvocationRequest) (<-chan error, error) {
	invoker.mu.Lock()
	invoker.invokeAsyncCalls = append(invoker.invokeAsyncCalls, request)
	invokeAsyncFunc := invoker.invokeAsyncFunc
	invoker.mu.Unlock()
	if invokeAsyncFunc == nil {
		errCh := make(chan error, 1)
		errCh <- nil
		return errCh, nil
	}
	return invokeAsyncFunc(request)
}

func (invoker *fakeGatewayInvoker) EnsureProfile(profileName string) error {
	invoker.mu.Lock()
	invoker.ensureCalls = append(invoker.ensureCalls, profileName)
	ensureFunc := invoker.ensureFunc
	invoker.mu.Unlock()
	if ensureFunc == nil {
		return nil
	}
	return ensureFunc(profileName)
}

func (invoker *fakeGatewayInvoker) Close() error {
	invoker.mu.Lock()
	invoker.closeCalls++
	closeFunc := invoker.closeFunc
	invoker.mu.Unlock()
	if closeFunc == nil {
		return nil
	}
	return closeFunc()
}

func (invoker *blockingGatewayInvoker) Invoke(request appcontext.InvocationRequest) error {
	invoker.mu.Lock()
	callIndex := len(invoker.requests)
	invoker.requests = append(invoker.requests, request)
	invoker.mu.Unlock()

	switch callIndex {
	case 0:
		close(invoker.firstStarted)
		<-invoker.releaseFirst
	case 1:
		close(invoker.secondStarted)
	}

	if invoker.bus == nil {
		return nil
	}
	return invoker.bus.Put(messagebus.Message{
		ChannelID:    request.Message.ChannelID,
		ChatID:       request.Message.ChatID,
		SenderID:     "assistant",
		Message:      "done",
		FinishReason: "stop",
	}, messagebus.OutboundQueue)
}

func (invoker *blockingGatewayInvoker) InvokeAsync(request appcontext.InvocationRequest) (<-chan error, error) {
	errCh := make(chan error, 1)
	errCh <- errors.New("unexpected InvokeAsync call")
	return errCh, nil
}

func (invoker *blockingGatewayInvoker) EnsureProfile(profileName string) error {
	return nil
}

func (invoker *blockingGatewayInvoker) Close() error {
	return nil
}

func (invoker *blockingGatewayInvoker) snapshotRequests() []appcontext.InvocationRequest {
	invoker.mu.Lock()
	defer invoker.mu.Unlock()

	requests := make([]appcontext.InvocationRequest, len(invoker.requests))
	copy(requests, invoker.requests)
	return requests
}

func (invoker *backlogGatewayInvoker) Invoke(request appcontext.InvocationRequest) error {
	content := request.Message.Message

	invoker.mu.Lock()
	invoker.requests = append(invoker.requests, content)
	invoker.mu.Unlock()

	switch content {
	case "shared-0":
		close(invoker.sharedStarted)
		<-invoker.releaseShared
	case "other":
		close(invoker.otherStarted)
	}

	return nil
}

func (invoker *backlogGatewayInvoker) InvokeAsync(request appcontext.InvocationRequest) (<-chan error, error) {
	errCh := make(chan error, 1)
	errCh <- errors.New("unexpected InvokeAsync call")
	return errCh, nil
}

func (invoker *backlogGatewayInvoker) EnsureProfile(profileName string) error {
	return nil
}

func (invoker *backlogGatewayInvoker) Close() error {
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
