package messagebus

import "testing"

func TestMessageBusOutputSinkEmitsToOutboundQueue(t *testing.T) {
	bus := NewMessageBus()
	sink := NewMessageBusOutputSink(bus)

	msg := Message{ChannelID: "cli", Message: "hello", FinishReason: "stop"}
	if err := sink.Emit(msg); err != nil {
		t.Fatalf("Emit() error = %v", err)
	}

	outCh, err := bus.Get(OutboundQueue)
	if err != nil {
		t.Fatalf("Get(OutboundQueue) error = %v", err)
	}
	got := <-outCh
	if got.Message != "hello" || got.ChannelID != "cli" {
		t.Fatalf("got = %#v, want hello/cli", got)
	}
}

func TestNoopOutputSinkDiscardsMessages(t *testing.T) {
	sink := NewNoopOutputSink()
	if err := sink.Emit(Message{Message: "ignored"}); err != nil {
		t.Fatalf("Emit() error = %v", err)
	}
}

func TestMessageBusOutputSinkReturnsErrorAfterBusClose(t *testing.T) {
	bus := NewMessageBus()
	sink := NewMessageBusOutputSink(bus)
	if err := bus.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := sink.Emit(Message{Message: "late"}); err == nil {
		t.Fatal("Emit() after bus close should return error")
	}
}
