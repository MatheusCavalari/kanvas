package realtime

import (
	"context"
	"log"
	"sync"

	"github.com/google/uuid"
)

// Event is the envelope broadcast to WebSocket subscribers of a board.
type Event struct {
	Type    string      `json:"type"`
	BoardID uuid.UUID   `json:"board_id"`
	Data    interface{} `json:"data"`
}

// Hub fans out board-scoped events to any number of subscribers. It has
// no knowledge of HTTP or WebSockets — internal/card depends on it
// structurally, via its own EventPublisher interface, purely as
// something with a Publish method; the WebSocket transport lives in
// handler.go, same package, added in Task 3.
type Hub struct {
	mu      sync.RWMutex
	clients map[uuid.UUID]map[chan Event]bool
}

func NewHub() *Hub {
	return &Hub{clients: make(map[uuid.UUID]map[chan Event]bool)}
}

// subscribe registers a new subscriber channel for boardID. The returned
// channel is buffered so a slow reader doesn't block Publish; if the
// buffer fills, Publish drops the event for that subscriber rather than
// blocking every other subscriber and every REST request that publishes.
func (h *Hub) subscribe(boardID uuid.UUID) chan Event {
	ch := make(chan Event, 16)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[boardID] == nil {
		h.clients[boardID] = make(map[chan Event]bool)
	}
	h.clients[boardID][ch] = true
	return ch
}

func (h *Hub) unsubscribe(boardID uuid.UUID, ch chan Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients[boardID], ch)
	if len(h.clients[boardID]) == 0 {
		delete(h.clients, boardID)
	}
	close(ch)
}

// Publish broadcasts an event to every current subscriber of boardID. It
// never blocks on a slow subscriber and is safe to call concurrently.
func (h *Hub) Publish(ctx context.Context, boardID uuid.UUID, eventType string, payload interface{}) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	event := Event{Type: eventType, BoardID: boardID, Data: payload}
	for ch := range h.clients[boardID] {
		select {
		case ch <- event:
		default:
			log.Printf("realtime: dropping event for board %s: subscriber buffer full", boardID)
		}
	}
}

// SubscriberCount reports how many active subscribers boardID currently
// has. Exported for tests and for a future health/metrics endpoint.
func (h *Hub) SubscriberCount(boardID uuid.UUID) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients[boardID])
}
