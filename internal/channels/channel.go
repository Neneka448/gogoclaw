package channels

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/Neneka448/gogoclaw/internal/config"
	messagebus "github.com/Neneka448/gogoclaw/internal/message_bus"
)

type Channel interface {
	Name() string
	Enabled() bool
	Start() error
	Stop() error
	Send(message messagebus.Message) error
}

type Registry interface {
	Register(channel Channel) error
	Get(name string) (Channel, bool)
	MustGet(name string) Channel
	All() []Channel
	StartAll() error
	StopAll() error
	Dispatch(message messagebus.Message) error
}

type registry struct {
	mu       sync.RWMutex
	channels map[string]Channel
}

func NewRegistry() Registry {
	return &registry{channels: make(map[string]Channel)}
}

func (r *registry) Register(channel Channel) error {
	if channel == nil {
		return fmt.Errorf("channel is nil")
	}
	name := strings.TrimSpace(channel.Name())
	if name == "" {
		return fmt.Errorf("channel name is empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.channels[name]; exists {
		return fmt.Errorf("channel already registered: %s", name)
	}
	r.channels[name] = channel
	return nil
}

func (r *registry) Get(name string) (Channel, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	channel, ok := r.channels[name]
	return channel, ok
}

func (r *registry) MustGet(name string) Channel {
	channel, ok := r.Get(name)
	if !ok {
		panic(fmt.Sprintf("channel not registered: %s", name))
	}
	return channel
}

func (r *registry) All() []Channel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.channels))
	for name := range r.channels {
		names = append(names, name)
	}
	sort.Strings(names)
	channels := make([]Channel, 0, len(names))
	for _, name := range names {
		channels = append(channels, r.channels[name])
	}
	return channels
}

func (r *registry) StartAll() error {
	for _, channel := range r.All() {
		if !channel.Enabled() {
			continue
		}
		if err := channel.Start(); err != nil {
			return err
		}
	}
	return nil
}

func (r *registry) StopAll() error {
	for _, channel := range r.All() {
		if err := channel.Stop(); err != nil {
			return err
		}
	}
	return nil
}

func (r *registry) Dispatch(message messagebus.Message) error {
	channel, ok := r.Get(message.ChannelID)
	if !ok {
		return fmt.Errorf("channel not found: %s", message.ChannelID)
	}
	if !channel.Enabled() {
		return fmt.Errorf("channel disabled: %s", message.ChannelID)
	}
	return channel.Send(message)
}

type CLIChannel struct {
	config config.CLIChannelConfig
	writer io.Writer
	mu     sync.Mutex
}

func NewCLIChannel(cfg config.CLIChannelConfig, writer io.Writer) Channel {
	if writer == nil {
		writer = os.Stdout
	}
	return &CLIChannel{config: cfg, writer: writer}
}

func (c *CLIChannel) Name() string {
	return "cli"
}

func (c *CLIChannel) Enabled() bool {
	return c.config.Enabled
}

func (c *CLIChannel) Start() error {
	return nil
}

func (c *CLIChannel) Stop() error {
	return nil
}

func (c *CLIChannel) Send(message messagebus.Message) error {
	if strings.TrimSpace(message.Message) == "" {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if message.FinishReason == "tool_calls" {
		_, err := fmt.Fprintf(c.writer, "[tool call]: %s\n", message.Message)
		return err
	}

	if message.FinishReason == "" {
		return nil
	}

	_, err := fmt.Fprintf(c.writer, "[message]:\n%s\n", message.Message)
	return err
}
