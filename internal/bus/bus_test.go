package bus

import (
	"testing"
	"time"
)

func TestPubSub(t *testing.T) {
	b := New()
	ch, cancel := b.Subscribe()
	defer cancel()
	b.Publish(Event{Type: "suggestion.created", Payload: map[string]any{"id": "1"}})
	select {
	case ev := <-ch:
		if ev.Type != "suggestion.created" {
			t.Fatalf("type = %q", ev.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
	cancel()
	b.Publish(Event{Type: "x"}) // não deve travar com assinante cancelado
}
