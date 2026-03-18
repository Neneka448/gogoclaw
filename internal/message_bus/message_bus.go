package messagebus

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

type QueueType string

const (
	InboundQueue  QueueType = "inbound"
	OutboundQueue QueueType = "outbound"
)

type MessageBus interface {
	Put(message Message, queueType QueueType) error
	Get(queueType QueueType) (<-chan Message, error)
	Close() error
}

type Options struct {
	QueueSize  int
	PutTimeout time.Duration
}

type messageBus struct {
	mu            sync.RWMutex
	closed        bool
	inboundQueue  chan Message
	outboundQueue chan Message
	putTimeout    time.Duration
}

const (
	defaultQueueSize       = 1024
	defaultMessageBusPutTO = time.Second
)

var (
	ErrMessageBusClosed     = errors.New("message bus is closed")
	ErrMessageBusPutTimeout = errors.New("message bus put timed out")
)

type Message struct {
	ChannelID    string
	Message      string
	MessageID    string
	MessageType  string
	ChatID       string
	SenderID     string
	MediaPaths   []string
	ReplyTo      string
	FinishReason string
	Metadata     map[string]string
}

func NewMessageBus() MessageBus {
	return NewMessageBusWithOptions(Options{})
}

func NewMessageBusWithOptions(options Options) MessageBus {
	queueSize := options.QueueSize
	if queueSize <= 0 {
		queueSize = defaultQueueSize
	}
	putTimeout := options.PutTimeout
	if putTimeout <= 0 {
		putTimeout = defaultMessageBusPutTO
	}
	return &messageBus{
		inboundQueue:  make(chan Message, queueSize),
		outboundQueue: make(chan Message, queueSize),
		putTimeout:    putTimeout,
	}
}

func (mb *messageBus) Put(message Message, queueType QueueType) (err error) {
	mb.mu.RLock()
	closed := mb.closed
	inboundQueue := mb.inboundQueue
	outboundQueue := mb.outboundQueue
	mb.mu.RUnlock()
	if closed {
		return ErrMessageBusClosed
	}

	defer func() {
		if recover() != nil {
			err = ErrMessageBusClosed
		}
	}()

	timer := time.NewTimer(mb.putTimeout)
	defer timer.Stop()

	switch queueType {
	case InboundQueue:
		select {
		case inboundQueue <- message:
			return nil
		case <-timer.C:
			return fmt.Errorf("put inbound message after %s: %w", mb.putTimeout, ErrMessageBusPutTimeout)
		}
	case OutboundQueue:
		select {
		case outboundQueue <- message:
			return nil
		case <-timer.C:
			return fmt.Errorf("put outbound message after %s: %w", mb.putTimeout, ErrMessageBusPutTimeout)
		}
	default:
		return fmt.Errorf("unknown queue type: %s", queueType)
	}
}

func (mb *messageBus) Get(queueType QueueType) (<-chan Message, error) {
	switch queueType {
	case InboundQueue:
		return mb.inboundQueue, nil
	case OutboundQueue:
		return mb.outboundQueue, nil
	default:
		return nil, fmt.Errorf("unknown queue type: %s", queueType)
	}
}

func (mb *messageBus) Close() error {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	if mb.closed {
		return nil
	}
	mb.closed = true
	close(mb.inboundQueue)
	close(mb.outboundQueue)
	return nil
}
