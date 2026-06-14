// Package bus distribui eventos do app (sugestões, sessões, aprovações)
// para assinantes (conexões WebSocket da UI).
package bus

import "sync"

type Event struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

type Bus struct {
	mu   sync.Mutex
	subs map[int]chan Event
	next int
}

func New() *Bus { return &Bus{subs: map[int]chan Event{}} }

func (b *Bus) Subscribe() (<-chan Event, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := b.next
	b.next++
	ch := make(chan Event, 64)
	b.subs[id] = ch
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if _, ok := b.subs[id]; ok {
			delete(b.subs, id)
			close(ch)
		}
	}
}

func (b *Bus) Publish(ev Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subs {
		select {
		case ch <- ev:
		default: // assinante lento: descarta em vez de travar
		}
	}
}
