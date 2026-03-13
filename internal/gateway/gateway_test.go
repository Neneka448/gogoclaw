package gateway

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/Neneka448/gogoclaw/internal/channels"
	appcontext "github.com/Neneka448/gogoclaw/internal/context"
	"github.com/Neneka448/gogoclaw/internal/cron"
	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
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
	for len(fake.received) == 0 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for outbound dispatch")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	if fake.received[0].Message != "hello" {
		t.Fatalf("fake.received[0].Message = %q, want hello", fake.received[0].Message)
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
	for len(fake.received) == 0 || fake.received[len(fake.received)-1].Message != "restart" {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for restarted gateway dispatch")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

type channelsTestChannel struct {
	name     string
	enabled  bool
	received []messagebus.Message
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
	c.received = append(c.received, message)
	return nil
}

func osStderrSwap(buffer *bytes.Buffer) func() {
	original := stderrWriter
	stderrWriter = buffer
	return func() {
		stderrWriter = original
	}
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
