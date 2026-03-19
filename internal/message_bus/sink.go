package messagebus

// OutputSink abstracts the outbound message emission path.
// Foreground invocations write to the MessageBus OutboundQueue;
// background/cron invocations discard output via NoopOutputSink.
type OutputSink interface {
	Emit(message Message) error
}

type messageBusOutputSink struct {
	bus MessageBus
}

func NewMessageBusOutputSink(bus MessageBus) OutputSink {
	return &messageBusOutputSink{bus: bus}
}

func (s *messageBusOutputSink) Emit(message Message) error {
	return s.bus.Put(message, OutboundQueue)
}

type noopOutputSink struct{}

func NewNoopOutputSink() OutputSink {
	return &noopOutputSink{}
}

func (s *noopOutputSink) Emit(message Message) error {
	return nil
}
