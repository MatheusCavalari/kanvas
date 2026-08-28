package realtime

import (
	"context"
	"log"
	"sync"
	"time"

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
	mu       sync.RWMutex
	clients  map[uuid.UUID]map[chan Event]bool
	presence map[uuid.UUID]map[uuid.UUID]PresenceInfo // boardID -> userID -> info
}

// PresenceInfo describes one user currently connected to a board's
// WebSocket. LastHeartbeat is excluded from JSON since it's an internal
// bookkeeping field for the reaper, not something clients need to see.
type PresenceInfo struct {
	UserID        uuid.UUID `json:"user_id"`
	Name          string    `json:"name"`
	LastHeartbeat time.Time `json:"-"`
}

// presenceStaleAfter is how long a connection can go without a heartbeat
// before the reaper considers it gone (e.g. the client crashed without a
// clean WebSocket close). presenceReapInterval is how often the reaper
// scans for stale entries.
const (
	presenceStaleAfter   = 60 * time.Second
	presenceReapInterval = 15 * time.Second
)

func NewHub() *Hub {
	return &Hub{
		clients:  make(map[uuid.UUID]map[chan Event]bool),
		presence: make(map[uuid.UUID]map[uuid.UUID]PresenceInfo),
	}
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
	// Only close ch if it's still registered. Close() may have already
	// removed and closed it during shutdown — closing it again here would
	// panic ("close of closed channel"). Checking presence and closing
	// under the same lock Close() also holds guarantees exactly one of
	// the two code paths ever closes a given channel.
	if _, ok := h.clients[boardID][ch]; !ok {
		return
	}
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

// Close shuts down the hub, closing every subscriber channel so connected
// WebSocket handlers detect the closed channel and terminate. Safe to call
// once during graceful shutdown.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for boardID, clients := range h.clients {
		for ch := range clients {
			close(ch)
			delete(clients, ch)
		}
		delete(h.clients, boardID)
	}
	for boardID := range h.presence {
		delete(h.presence, boardID)
	}
}

// JoinPresence records userID as present on boardID and broadcasts
// presence.joined to that board's subscribers. Called once per WebSocket
// connection, right after it's accepted.
func (h *Hub) JoinPresence(boardID, userID uuid.UUID, name string) {
	info := PresenceInfo{UserID: userID, Name: name, LastHeartbeat: time.Now()}

	h.mu.Lock()
	if h.presence[boardID] == nil {
		h.presence[boardID] = make(map[uuid.UUID]PresenceInfo)
	}
	h.presence[boardID][userID] = info
	h.mu.Unlock()

	h.Publish(context.Background(), boardID, "presence.joined", info)
}

// LeavePresence removes userID from boardID's presence set and broadcasts
// presence.left. It's a no-op (no broadcast) if the user wasn't present,
// which lets both the WebSocket disconnect path and the reaper call it
// idempotently without double-broadcasting.
func (h *Hub) LeavePresence(boardID, userID uuid.UUID) {
	h.mu.Lock()
	_, ok := h.presence[boardID][userID]
	if ok {
		delete(h.presence[boardID], userID)
		if len(h.presence[boardID]) == 0 {
			delete(h.presence, boardID)
		}
	}
	h.mu.Unlock()

	if !ok {
		return
	}
	h.Publish(context.Background(), boardID, "presence.left", map[string]uuid.UUID{"user_id": userID})
}

// Heartbeat refreshes userID's LastHeartbeat on boardID. It's a no-op if
// the user isn't tracked as present (e.g. a ping arriving after the
// connection was already reaped or torn down).
func (h *Hub) Heartbeat(boardID, userID uuid.UUID) {
	h.mu.Lock()
	defer h.mu.Unlock()
	info, ok := h.presence[boardID][userID]
	if !ok {
		return
	}
	info.LastHeartbeat = time.Now()
	h.presence[boardID][userID] = info
}

// GetPresence returns the current list of users present on boardID. The
// order is unspecified since it's derived from map iteration.
func (h *Hub) GetPresence(boardID uuid.UUID) []PresenceInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()
	list := make([]PresenceInfo, 0, len(h.presence[boardID]))
	for _, info := range h.presence[boardID] {
		list = append(list, info)
	}
	return list
}

// StartReaper runs until ctx is cancelled, periodically removing presence
// entries that haven't sent a heartbeat within presenceStaleAfter. This
// catches connections that disappear without a clean WebSocket close
// (crashed tab, lost network) where the handler's deferred LeavePresence
// never runs. Intended to be started as its own goroutine from main.
func (h *Hub) StartReaper(ctx context.Context) {
	ticker := time.NewTicker(presenceReapInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.reapStale()
		}
	}
}

// staleKey identifies one presence entry considered for reaping.
type staleKey struct {
	boardID uuid.UUID
	userID  uuid.UUID
}

// reapStale scans presence under a read lock to find stale entries, then
// calls LeavePresence for each outside the lock — LeavePresence takes its
// own write lock and publishes, so holding the read lock across it would
// either deadlock or serialize broadcasts under a lock unnecessarily.
func (h *Hub) reapStale() {
	now := time.Now()

	h.mu.RLock()
	var stale []staleKey
	for boardID, users := range h.presence {
		for userID, info := range users {
			if now.Sub(info.LastHeartbeat) > presenceStaleAfter {
				stale = append(stale, staleKey{boardID: boardID, userID: userID})
			}
		}
	}
	h.mu.RUnlock()

	for _, k := range stale {
		h.LeavePresence(k.boardID, k.userID)
	}
}
