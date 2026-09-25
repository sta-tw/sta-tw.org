//go:build integration

package events

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"sta-backend/internal/dbtest"
)

func TestHubDeliversAcrossListenNotify(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := NewHub(ctx, pool, nil)
	sub := hub.Subscribe(ctx, "chat:lounge")

	// Give the listener goroutine a moment to run LISTEN.
	time.Sleep(300 * time.Millisecond)

	if err := hub.PublishData(ctx, "chat:lounge", "chat.message", map[string]string{"body": "hi"}); err != nil {
		t.Fatalf("PublishData: %v", err)
	}
	// An event on a topic nobody is subscribed to must not arrive.
	if err := hub.PublishData(ctx, "notifications:someone", "notification.created", nil); err != nil {
		t.Fatalf("PublishData 2: %v", err)
	}

	select {
	case ev := <-sub:
		if ev.Topic != "chat:lounge" || ev.Kind != "chat.message" {
			t.Fatalf("unexpected event: %+v", ev)
		}
		var data map[string]string
		if err := json.Unmarshal(ev.Data, &data); err != nil || data["body"] != "hi" {
			t.Fatalf("event data = %s (%v)", ev.Data, err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("did not receive the published event within 3s")
	}

	select {
	case ev := <-sub:
		t.Fatalf("received an event for an unsubscribed topic: %+v", ev)
	case <-time.After(300 * time.Millisecond):
	}
}
