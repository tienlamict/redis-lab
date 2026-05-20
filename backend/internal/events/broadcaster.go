// Package events provides a tiny in-memory SSE pub/sub hub.
// One publisher per lab, N subscribers per channel; no persistence.
package events

import (
	"sync"
	"time"
)

// Event is the JSON payload pushed to SSE subscribers.
type Event struct {
	Lab     string         `json:"lab"`
	Type    string         `json:"type"`
	Time    time.Time      `json:"time"`
	Payload map[string]any `json:"payload,omitempty"`
}

// Broadcaster fans out events to per-lab subscribers.
type Broadcaster struct {
	mu   sync.RWMutex
	subs map[string]map[chan Event]struct{} // lab -> set of subscriber channels
}

// New returns an empty broadcaster.
func New() *Broadcaster {
	return &Broadcaster{subs: make(map[string]map[chan Event]struct{})}
}

// Subscribe registers a new listener for the given lab.
// The returned channel is buffered (16); slow consumers drop events instead of
// blocking publishers. Caller MUST call the returned unsubscribe func.
func (b *Broadcaster) Subscribe(lab string) (<-chan Event, func()) {
	ch := make(chan Event, 16)
	b.mu.Lock()
	if _, ok := b.subs[lab]; !ok {
		b.subs[lab] = make(map[chan Event]struct{})
	}
	b.subs[lab][ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		if set, ok := b.subs[lab]; ok {
			delete(set, ch)
		}
		b.mu.Unlock()
		close(ch)
	}
}

// Publish delivers e to every subscriber of e.Lab.
// Non-blocking: if a subscriber's buffer is full, the event is dropped for that
// subscriber only.
func (b *Broadcaster) Publish(e Event) {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subs[e.Lab] {
		select {
		case ch <- e:
		default:
			// drop
		}
	}
}
