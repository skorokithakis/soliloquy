// Package hub provides a small in-process publish/subscribe hub for live
// updates. It is deliberately not persistent: the application runs as one
// process, and a subscriber only cares about events that happen while it is
// connected.
package hub

import (
	"strconv"
	"sync"
)

// Event is a named message. Data is sent to clients as-is, so producers should
// encode it as JSON (or another format the client understands).
type Event struct {
	Name string
	Data []byte
}

// Hub fans events out to subscribers by topic. Publish never blocks: a
// subscriber that cannot keep up misses events rather than stalling the
// publisher. A page reconnects with a fresh page load when that matters.
type Hub struct {
	mu     sync.RWMutex
	topics map[string]map[chan Event]struct{}
}

// New returns an empty hub.
func New() *Hub {
	return &Hub{topics: make(map[string]map[chan Event]struct{})}
}

// UserTopic is the topic carrying a user's notification badge updates.
func UserTopic(userID int64) string {
	return "user:" + strconv.FormatInt(userID, 10)
}

// ThreadTopic is the topic carrying a thread's new comments.
func ThreadTopic(threadID int64) string {
	return "thread:" + strconv.FormatInt(threadID, 10)
}

// Subscribe registers a subscriber for a topic. The returned function removes
// the subscription; callers should defer it. The channel is never closed.
func (h *Hub) Subscribe(topic string) (<-chan Event, func()) {
	ch := make(chan Event, 16)

	h.mu.Lock()
	subscribers := h.topics[topic]
	if subscribers == nil {
		subscribers = make(map[chan Event]struct{})
		h.topics[topic] = subscribers
	}
	subscribers[ch] = struct{}{}
	h.mu.Unlock()

	return ch, func() {
		h.mu.Lock()
		delete(h.topics[topic], ch)
		if len(h.topics[topic]) == 0 {
			delete(h.topics, topic)
		}
		h.mu.Unlock()
	}
}

// Publish sends an event to every subscriber of a topic. It drops the event
// for subscribers whose buffers are full.
func (h *Hub) Publish(topic string, event Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for ch := range h.topics[topic] {
		select {
		case ch <- event:
		default:
		}
	}
}
