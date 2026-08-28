package comment

import (
	"context"
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

type commentService interface {
	Create(ctx context.Context, cardID, authorID uuid.UUID, body string) (Comment, error)
	Update(ctx context.Context, commentID, requesterID uuid.UUID, body string) (Comment, error)
	Delete(ctx context.Context, commentID, requesterID uuid.UUID) error
	ListByCard(ctx context.Context, cardID, requesterID uuid.UUID, cursor time.Time, limit int) ([]Comment, error)
}

type Handler struct {
	service commentService
}

func NewHandler(service commentService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Route("/cards/{cardID}/comments", func(r chi.Router) {
		r.Use(authMiddleware)
		r.Get("/", h.ListComments)
		r.Post("/", h.CreateComment)
	})
	r.Route("/comments/{commentID}", func(r chi.Router) {
		r.Use(authMiddleware)
		r.Patch("/", h.UpdateComment)
		r.Delete("/", h.DeleteComment)
	})
}

type commentView struct {
	ID        string    `json:"id"`
	CardID    string    `json:"card_id"`
	AuthorID  string    `json:"author_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type createCommentRequest struct {
	Body string `json:"body"`
}

type updateCommentRequest struct {
	Body string `json:"body"`
}

type listCommentsResponse struct {
	Comments   []commentView `json:"comments"`
	NextCursor string        `json:"next_cursor,omitempty"`
}

func (h *Handler) CreateComment(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	cardID, err := parseUUIDParam(r, "cardID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid card id")
		return
	}

	var req createCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}

	c, err := h.service.Create(r.Context(), cardID, userID, req.Body)
	if err != nil {
		h.writeCommentError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, toCommentView(c))
}

func (h *Handler) UpdateComment(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	commentID, err := parseUUIDParam(r, "commentID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid comment id")
		return
	}

	var req updateCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}

	c, err := h.service.Update(r.Context(), commentID, userID, req.Body)
	if err != nil {
		h.writeCommentError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toCommentView(c))
}

func (h *Handler) DeleteComment(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	commentID, err := parseUUIDParam(r, "commentID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid comment id")
		return
	}

	if err := h.service.Delete(r.Context(), commentID, userID); err != nil {
		h.writeCommentError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListComments(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	cardID, err := parseUUIDParam(r, "cardID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid card id")
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

	comments, err := h.service.ListByCard(r.Context(), cardID, userID, cursor, limit)
	if err != nil {
		h.writeCommentError(w, err)
		return
	}

	views := make([]commentView, 0, len(comments))
	for _, c := range comments {
		views = append(views, toCommentView(c))
	}

	resp := listCommentsResponse{Comments: views}
	if len(comments) > 0 && len(comments) >= limit {
		resp.NextCursor = encodeCursor(comments[len(comments)-1].CreatedAt)
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) writeCommentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, board.ErrNotAMember), errors.Is(err, board.ErrForbidden), errors.Is(err, ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", err.Error())
	case errors.Is(err, ErrNotAuthor):
		writeError(w, http.StatusForbidden, "forbidden", err.Error())
	case errors.Is(err, ErrCommentNotFound), errors.Is(err, ErrCardNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrInvalidBody):
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func parseUUIDParam(r *http.Request, name string) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, name))
}

func toCommentView(c Comment) commentView {
	return commentView{
		ID:        c.ID.String(),
		CardID:    c.CardID.String(),
		AuthorID:  c.AuthorID.String(),
		Body:      c.Body,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}

// decodeCursor decodes a base64-encoded RFC3339Nano timestamp query param.
// An empty raw value yields the zero time, meaning "from the beginning".
func decodeCursor(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
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
