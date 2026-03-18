package messagebus

import (
	"errors"
	"testing"
	"time"
)

func TestMessageBusPutAfterCloseReturnsError(t *testing.T) {
	bus := NewMessageBus()
	if err := bus.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := bus.Put(Message{Message: "late"}, InboundQueue); !errors.Is(err, ErrMessageBusClosed) {
		t.Fatalf("Put() error = %v, want closed bus error", err)
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

func TestMessageBusPutReturnsTimeoutWhenQueueIsFull(t *testing.T) {
	bus := NewMessageBusWithOptions(Options{
		QueueSize:  1,
		PutTimeout: 10 * time.Millisecond,
	})

	if err := bus.Put(Message{Message: "first"}, InboundQueue); err != nil {
		t.Fatalf("Put(first) error = %v", err)
	}

	start := time.Now()
	err := bus.Put(Message{Message: "second"}, InboundQueue)
	elapsed := time.Since(start)
	if !errors.Is(err, ErrMessageBusPutTimeout) {
		t.Fatalf("Put(second) error = %v, want timeout", err)
	}
	if elapsed < 10*time.Millisecond {
		t.Fatalf("Put(second) elapsed = %s, want wait for timeout", elapsed)
	}
}
