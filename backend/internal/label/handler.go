package label

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/MatheusCavalari/kanvas/backend/internal/board"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/middleware"
)

type labelService interface {
	Create(ctx context.Context, boardID, requesterID uuid.UUID, name, color string) (Label, error)
	Update(ctx context.Context, labelID, requesterID uuid.UUID, name, color string) (Label, error)
	Delete(ctx context.Context, labelID, requesterID uuid.UUID) error
	ListByBoard(ctx context.Context, boardID, requesterID uuid.UUID) ([]Label, error)
	AttachToCard(ctx context.Context, cardID, labelID, requesterID uuid.UUID) error
	DetachFromCard(ctx context.Context, cardID, labelID, requesterID uuid.UUID) error
	ListCardLabels(ctx context.Context, cardID, requesterID uuid.UUID) ([]Label, error)
}

type Handler struct {
	service labelService
}

func NewHandler(service labelService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Route("/boards/{boardID}/labels", func(r chi.Router) {
		r.Use(authMiddleware)
		r.Get("/", h.ListBoardLabels)
		r.Post("/", h.CreateLabel)
		r.Patch("/{labelID}", h.UpdateLabel)
		r.Delete("/{labelID}", h.DeleteLabel)
	})
	r.Route("/cards/{cardID}/labels", func(r chi.Router) {
		r.Use(authMiddleware)
		r.Get("/", h.ListCardLabels)
		r.Post("/", h.AttachLabel)
		r.Delete("/{labelID}", h.DetachLabel)
	})
}

type labelView struct {
	ID        string    `json:"id"`
	BoardID   string    `json:"board_id"`
	Name      string    `json:"name"`
	Color     string    `json:"color"`
	CreatedAt time.Time `json:"created_at"`
}

type createLabelRequest struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

type updateLabelRequest struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

type attachLabelRequest struct {
	LabelID uuid.UUID `json:"label_id"`
}

func (h *Handler) CreateLabel(w http.ResponseWriter, r *http.Request) {
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

	var req createLabelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}

	l, err := h.service.Create(r.Context(), boardID, userID, req.Name, req.Color)
	if err != nil {
		h.writeLabelError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, toLabelView(l))
}

func (h *Handler) UpdateLabel(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	labelID, err := parseUUIDParam(r, "labelID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid label id")
		return
	}

	var req updateLabelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}

	l, err := h.service.Update(r.Context(), labelID, userID, req.Name, req.Color)
	if err != nil {
		h.writeLabelError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toLabelView(l))
}

func (h *Handler) DeleteLabel(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	labelID, err := parseUUIDParam(r, "labelID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid label id")
		return
	}

	if err := h.service.Delete(r.Context(), labelID, userID); err != nil {
		h.writeLabelError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListBoardLabels(w http.ResponseWriter, r *http.Request) {
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

	labels, err := h.service.ListByBoard(r.Context(), boardID, userID)
	if err != nil {
		h.writeLabelError(w, err)
		return
	}

	views := make([]labelView, 0, len(labels))
	for _, l := range labels {
		views = append(views, toLabelView(l))
	}
	writeJSON(w, http.StatusOK, views)
}

func (h *Handler) AttachLabel(w http.ResponseWriter, r *http.Request) {
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

	var req attachLabelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if req.LabelID == uuid.Nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "label_id is required")
		return
	}

	if err := h.service.AttachToCard(r.Context(), cardID, req.LabelID, userID); err != nil {
		h.writeLabelError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) DetachLabel(w http.ResponseWriter, r *http.Request) {
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

	labelID, err := parseUUIDParam(r, "labelID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid label id")
		return
	}

	if err := h.service.DetachFromCard(r.Context(), cardID, labelID, userID); err != nil {
		h.writeLabelError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListCardLabels(w http.ResponseWriter, r *http.Request) {
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

	labels, err := h.service.ListCardLabels(r.Context(), cardID, userID)
	if err != nil {
		h.writeLabelError(w, err)
		return
	}

	views := make([]labelView, 0, len(labels))
	for _, l := range labels {
		views = append(views, toLabelView(l))
	}
	writeJSON(w, http.StatusOK, views)
}

func (h *Handler) writeLabelError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, board.ErrNotAMember), errors.Is(err, board.ErrForbidden), errors.Is(err, ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", err.Error())
	case errors.Is(err, ErrLabelNotFound), errors.Is(err, ErrCardNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrDuplicateName):
		writeError(w, http.StatusConflict, "duplicate_name", err.Error())
	case errors.Is(err, ErrInvalidColor), errors.Is(err, ErrInvalidName):
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func parseUUIDParam(r *http.Request, name string) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, name))
}

func toLabelView(l Label) labelView {
	return labelView{
		ID:        l.ID.String(),
		BoardID:   l.BoardID.String(),
		Name:      l.Name,
		Color:     l.Color,
		CreatedAt: l.CreatedAt,
	}
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
