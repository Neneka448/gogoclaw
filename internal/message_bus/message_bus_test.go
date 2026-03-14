package messagebus

import "testing"

func TestMessageBusPutAfterCloseReturnsError(t *testing.T) {
	bus := NewMessageBus()
	if err := bus.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := bus.Put(Message{Message: "late"}, InboundQueue); err == nil {
		t.Fatal("Put() error = nil, want closed bus error")
	}
}

func TestMessageBusCloseIsIdempotent(t *testing.T) {
	bus := NewMessageBus()
	if err := bus.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	if err := bus.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}
