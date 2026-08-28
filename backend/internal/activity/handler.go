package activity

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/MatheusCavalari/kanvas/backend/internal/board"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/middleware"
)

// farFutureCursor is used as the cursor for the first page of a DESC
// listing (created_at < cursor), since there is no natural "beginning"
// value to use the way ASC listings use the zero time.
var farFutureCursor = time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)

type Handler struct {
	repo  Repository
	board BoardAuthorizer
}

func NewHandler(repo Repository, board BoardAuthorizer) *Handler {
	return &Handler{repo: repo, board: board}
}

func (h *Handler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Route("/boards/{boardID}/activity", func(r chi.Router) {
		r.Use(authMiddleware)
		r.Get("/", h.ListActivity)
	})
}

type entryDetailView struct {
	ID         string    `json:"id"`
	BoardID    string    `json:"board_id"`
	ActorID    string    `json:"actor_id"`
	Action     string    `json:"action"`
	EntityType string    `json:"entity_type"`
	EntityID   string    `json:"entity_id"`
	CreatedAt  time.Time `json:"created_at"`
}

type listActivityResponse struct {
	Entries    []entryDetailView `json:"entries"`
	NextCursor string            `json:"next_cursor,omitempty"`
}

func (h *Handler) ListActivity(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	boardID, err := parseUUIDParam(r, "boardID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid board id")
		return
	}

	if err := h.board.EnsureMember(r.Context(), boardID, userID); err != nil {
		h.writeActivityError(w, err)
		return
	}

	cursor, err := decodeCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid cursor")
		return
	}

	limit := DefaultListLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "invalid_request", "invalid limit")
			return
		}
		limit = n
	}
	if limit > MaxListLimit {
		limit = MaxListLimit
	}

	entries, err := h.repo.ListByBoard(r.Context(), boardID, cursor, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	views := make([]entryDetailView, 0, len(entries))
	for _, e := range entries {
		views = append(views, toEntryDetailView(e))
	}

	resp := listActivityResponse{Entries: views}
	if len(entries) > 0 && len(entries) >= limit {
		resp.NextCursor = encodeCursor(entries[len(entries)-1].CreatedAt)
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) writeActivityError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, board.ErrNotAMember), errors.Is(err, board.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func parseUUIDParam(r *http.Request, name string) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, name))
}

func toEntryDetailView(e Entry) entryDetailView {
	return entryDetailView{
		ID:         e.ID.String(),
		BoardID:    e.BoardID.String(),
		ActorID:    e.ActorID.String(),
		Action:     e.Action,
		EntityType: e.EntityType,
		EntityID:   e.EntityID.String(),
		CreatedAt:  e.CreatedAt,
	}
}

// decodeCursor decodes a base64-encoded RFC3339Nano timestamp query param.
// An empty raw value yields farFutureCursor, meaning "from the newest
// entry", since this list is sorted created_at DESC.
func decodeCursor(raw string) (time.Time, error) {
	if raw == "" {
		return farFutureCursor, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse(time.RFC3339Nano, string(decoded))
}

func encodeCursor(t time.Time) string {
	return base64.StdEncoding.EncodeToString([]byte(t.Format(time.RFC3339Nano)))
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{Error: errorBody{Code: code, Message: message}})
}
