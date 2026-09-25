package events

import (
	"context"
	"testing"
	"time"
)

// A hub with no pool never connects a listener, but Subscribe / delivery / the
// shutdown fan-out still work in-process.
func newTestHub(t *testing.T) (*Hub, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	h := NewHub(ctx, nil, nil)
	t.Cleanup(cancel)
	return h, cancel
}

func TestHubDeliversToMatchingTopics(t *testing.T) {
	h, _ := newTestHub(t)
	subCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := h.Subscribe(subCtx, "chat:lounge")

	h.deliver(Event{Topic: "chat:other", Kind: "x"})
	h.deliver(Event{Topic: "chat:lounge", Kind: "chat.message"})

	select {
	case ev := <-ch:
		if ev.Topic != "chat:lounge" || ev.Kind != "chat.message" {
			t.Fatalf("unexpected event %+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("no event delivered")
	}
	select {
	case ev := <-ch:
		t.Fatalf("received a second event for a non-matching topic: %+v", ev)
	default:
	}
}

func TestHubShutdownClosesSubscribers(t *testing.T) {
	h, cancelHub := newTestHub(t)
	ch := h.Subscribe(context.Background(), "chat:lounge")

	cancelHub() // hub context done -> listenLoop exits -> closeAll

	select {
	case _, open := <-ch:
		if open {
			t.Fatal("expected the subscriber channel to be closed on hub shutdown")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("subscriber channel was not closed after hub shutdown")
	}

	// Subscribing after shutdown yields an already-closed channel, not a hang.
	after := h.Subscribe(context.Background(), "chat:lounge")
	select {
	case _, open := <-after:
		if open {
			t.Fatal("post-shutdown Subscribe returned an open channel")
		}
	case <-time.After(time.Second):
		t.Fatal("post-shutdown Subscribe did not return a closed channel")
	}
}
