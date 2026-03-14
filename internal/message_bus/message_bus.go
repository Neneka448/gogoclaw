package messagebus

import (
	"errors"
	"fmt"
	"sync"
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

type messageBus struct {
	mu            sync.RWMutex
	closed        bool
	inboundQueue  chan Message
	outboundQueue chan Message
}

var errMessageBusClosed = errors.New("message bus is closed")

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
	return &messageBus{
		inboundQueue:  make(chan Message, 1024),
		outboundQueue: make(chan Message, 1024),
	}
}

func (mb *messageBus) Put(message Message, queueType QueueType) (err error) {
	mb.mu.RLock()
	closed := mb.closed
	inboundQueue := mb.inboundQueue
	outboundQueue := mb.outboundQueue
	mb.mu.RUnlock()
	if closed {
		return errMessageBusClosed
	}

	defer func() {
		if recover() != nil {
			err = errMessageBusClosed
		}
	}()

	switch queueType {
	case InboundQueue:
		inboundQueue <- message
	case OutboundQueue:
		outboundQueue <- message
	default:
		return fmt.Errorf("unknown queue type: %s", queueType)
	}
	return nil
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
