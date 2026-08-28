package activity

import (
	"context"
	"encoding/json"
	"log/slog"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/middleware"
)

// EventActivityCreated is published to the next publisher in the chain
// after an entry has been persisted, so realtime subscribers (e.g. the
// frontend's activity panel) can update live.
const EventActivityCreated = "activity.created"

// Recorder is an EventPublisher decorator that records every event it
// observes as an activity_log entry before delegating to the next
// publisher in the chain (typically the cache invalidator, which in turn
// forwards to the realtime hub).
//
// Recording requires an authenticated actor: if the context has no user ID
// (e.g. a system-generated event with no HTTP request behind it), the
// event is not recorded, but it is still delegated to next so downstream
// behavior (cache invalidation, realtime broadcast) is unaffected.
type Recorder struct {
	repo Repository
	next EventPublisher
}

func NewRecorder(repo Repository, next EventPublisher) *Recorder {
	return &Recorder{repo: repo, next: next}
}

func (rec *Recorder) Publish(ctx context.Context, boardID uuid.UUID, eventType string, payload interface{}) {
	entry, recorded := rec.record(ctx, boardID, eventType, payload)

	rec.next.Publish(ctx, boardID, eventType, payload)

	if recorded {
		rec.next.Publish(ctx, boardID, EventActivityCreated, toEntryView(entry))
	}
}

// record persists an activity_log entry for the given event, if possible.
// It returns the persisted entry and true on success. It never returns an
// error: any failure to record (no actor in context, unmarshalable
// payload, or a repository error) is logged and treated as non-fatal, so
// the event is still delegated to next.
func (rec *Recorder) record(ctx context.Context, boardID uuid.UUID, eventType string, payload interface{}) (Entry, bool) {
	actorID, ok := middleware.UserIDFromContext(ctx)
	if !ok {
		slog.Warn("activity: no actor in context, skipping recording", "board_id", boardID, "event_type", eventType)
		return Entry{}, false
	}

	entityType, action := parseEventType(eventType)
	entityID := extractEntityID(payload, boardID)

	snapshotAfter, err := json.Marshal(payload)
	if err != nil {
		slog.Warn("activity: failed to marshal event payload, skipping recording", "board_id", boardID, "event_type", eventType, "error", err)
		return Entry{}, false
	}

	entry := Entry{
		ID:            uuid.New(),
		BoardID:       boardID,
		ActorID:       actorID,
		Action:        action,
		EntityType:    entityType,
		EntityID:      entityID,
		SnapshotAfter: snapshotAfter,
		CreatedAt:     time.Now(),
	}

	if err := rec.repo.Insert(ctx, entry); err != nil {
		slog.Warn("activity: failed to persist entry", "board_id", boardID, "event_type", eventType, "error", err)
		return Entry{}, false
	}

	return entry, true
}

type entryView struct {
	ID         string    `json:"id"`
	BoardID    string    `json:"board_id"`
	ActorID    string    `json:"actor_id"`
	Action     string    `json:"action"`
	EntityType string    `json:"entity_type"`
	EntityID   string    `json:"entity_id"`
	CreatedAt  time.Time `json:"created_at"`
}

func toEntryView(e Entry) entryView {
	return entryView{
		ID:         e.ID.String(),
		BoardID:    e.BoardID.String(),
		ActorID:    e.ActorID.String(),
		Action:     e.Action,
		EntityType: e.EntityType,
		EntityID:   e.EntityID.String(),
		CreatedAt:  e.CreatedAt,
	}
}

// parseEventType splits an event type like "card.created" into its
// entity_type ("card") and action ("created"). Event types that don't
// follow the "entity.action" pattern are treated as their own entity_type
// with an empty action, rather than being rejected.
func parseEventType(eventType string) (entityType, action string) {
	parts := strings.SplitN(eventType, ".", 2)
	if len(parts) != 2 {
		return eventType, ""
	}
	return parts[0], parts[1]
}

// extractEntityID looks for the ID of the entity a payload describes.
// Payloads are shaped one of two ways by the domain services:
//   - a map[string]interface{} with an "id" key (used for delete events)
//   - a struct (or pointer to struct) with an "ID" field (used for
//     create/update events)
//
// If neither shape yields a usable UUID (e.g. bulk events like
// column.reordered that describe several entities rather than one), the
// boardID is used as a fallback so the NOT NULL entity_id column is always
// satisfiable without dropping the event.
func extractEntityID(payload interface{}, boardID uuid.UUID) uuid.UUID {
	if payload == nil {
		return boardID
	}

	if m, ok := payload.(map[string]interface{}); ok {
		if id, ok := toUUID(m["id"]); ok {
			return id
		}
		return boardID
	}

	v := reflect.ValueOf(payload)
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return boardID
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return boardID
	}

	field := v.FieldByName("ID")
	if !field.IsValid() {
		return boardID
	}
	if id, ok := toUUID(field.Interface()); ok {
		return id
	}
	return boardID
}

// toUUID converts a value that is either a uuid.UUID or a UUID-formatted
// string into a uuid.UUID.
func toUUID(v interface{}) (uuid.UUID, bool) {
	switch val := v.(type) {
	case uuid.UUID:
		return val, true
	case string:
		id, err := uuid.Parse(val)
		if err != nil {
			return uuid.UUID{}, false
		}
		return id, true
	default:
		return uuid.UUID{}, false
	}
}
