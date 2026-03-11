package messagebus

import "fmt"

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
	inboundQueue  chan Message
	outboundQueue chan Message
}

type Message struct {
	ChannelID    string
	Message      string
	MessageID    string
	MessageType  string
	ChatID       string
	SenderID     string
	FinishReason string
}

func NewMessageBus() MessageBus {
	return &messageBus{
		inboundQueue:  make(chan Message, 1024),
		outboundQueue: make(chan Message, 1024),
	}
}

func (mb *messageBus) Put(message Message, queueType QueueType) error {
	switch queueType {
	case InboundQueue:
		mb.inboundQueue <- message
	case OutboundQueue:
		mb.outboundQueue <- message
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
	close(mb.inboundQueue)
	close(mb.outboundQueue)
	return nil
}
