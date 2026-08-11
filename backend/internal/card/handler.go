package card

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

type cardService interface {
	CreateColumn(ctx context.Context, boardID, requesterID uuid.UUID, title string) (Column, error)
	RenameColumn(ctx context.Context, columnID, requesterID uuid.UUID, title string) (Column, error)
	DeleteColumn(ctx context.Context, columnID, requesterID uuid.UUID) error
	ReorderColumns(ctx context.Context, boardID, requesterID uuid.UUID, orderedColumnIDs []uuid.UUID) error
	ListBoardColumns(ctx context.Context, boardID, requesterID uuid.UUID) ([]ColumnWithCards, error)
	CreateCard(ctx context.Context, columnID, requesterID uuid.UUID, title, description string, assigneeID *uuid.UUID, dueDate *time.Time) (Card, error)
	UpdateCard(ctx context.Context, cardID, requesterID uuid.UUID, title, description string, assigneeID *uuid.UUID, dueDate *time.Time) (Card, error)
	DeleteCard(ctx context.Context, cardID, requesterID uuid.UUID) error
	MoveCard(ctx context.Context, cardID, requesterID, targetColumnID uuid.UUID, targetPosition int) (Card, error)
}

type Handler struct {
	service cardService
}

func NewHandler(service cardService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Route("/boards/{boardID}/columns", func(r chi.Router) {
		r.Use(authMiddleware)
		r.Get("/", h.ListBoardColumns)
		r.Post("/", h.CreateColumn)
		r.Patch("/reorder", h.ReorderColumns)
		r.Patch("/{columnID}", h.RenameColumn)
		r.Delete("/{columnID}", h.DeleteColumn)
	})
	r.Route("/cards", func(r chi.Router) {
		r.Use(authMiddleware)
		r.Post("/", h.CreateCard)
		r.Route("/{cardID}", func(r chi.Router) {
			r.Patch("/", h.UpdateCard)
			r.Delete("/", h.DeleteCard)
			r.Patch("/move", h.MoveCard)
		})
	})
}

type columnView struct {
	ID        string     `json:"id"`
	BoardID   string     `json:"board_id"`
	Title     string     `json:"title"`
	Position  int32      `json:"position"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	Cards     []cardView `json:"cards,omitempty"`
}

type cardView struct {
	ID          string     `json:"id"`
	ColumnID    string     `json:"column_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Position    int32      `json:"position"`
	AssigneeID  *string    `json:"assignee_id"`
	DueDate     *time.Time `json:"due_date"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type createColumnRequest struct {
	Title string `json:"title"`
}

type renameColumnRequest struct {
	Title string `json:"title"`
}

type reorderColumnsRequest struct {
	ColumnIDs []uuid.UUID `json:"column_ids"`
}

type createCardRequest struct {
	ColumnID    uuid.UUID  `json:"column_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	AssigneeID  *uuid.UUID `json:"assignee_id"`
	DueDate     *time.Time `json:"due_date"`
}

type updateCardRequest struct {
	Title       string     `json:"title"`
	Description string     `json:"description"`
	AssigneeID  *uuid.UUID `json:"assignee_id"`
	DueDate     *time.Time `json:"due_date"`
}

type moveCardRequest struct {
	ColumnID uuid.UUID `json:"column_id"`
	Position int       `json:"position"`
}

func (h *Handler) CreateColumn(w http.ResponseWriter, r *http.Request) {
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

	var req createColumnRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "title is required")
		return
	}

	column, err := h.service.CreateColumn(r.Context(), boardID, userID, req.Title)
	if err != nil {
		h.writeCardError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, toColumnView(column, nil))
}

func (h *Handler) RenameColumn(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	columnID, err := parseUUIDParam(r, "columnID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid column id")
		return
	}

	var req renameColumnRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "title is required")
		return
	}

	column, err := h.service.RenameColumn(r.Context(), columnID, userID, req.Title)
	if err != nil {
		h.writeCardError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toColumnView(column, nil))
}

func (h *Handler) DeleteColumn(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	columnID, err := parseUUIDParam(r, "columnID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid column id")
		return
	}

	if err := h.service.DeleteColumn(r.Context(), columnID, userID); err != nil {
		h.writeCardError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ReorderColumns(w http.ResponseWriter, r *http.Request) {
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

	var req reorderColumnsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}

	if err := h.service.ReorderColumns(r.Context(), boardID, userID, req.ColumnIDs); err != nil {
		h.writeCardError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListBoardColumns(w http.ResponseWriter, r *http.Request) {
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

	columns, err := h.service.ListBoardColumns(r.Context(), boardID, userID)
	if err != nil {
		h.writeCardError(w, err)
		return
	}

	views := make([]columnView, 0, len(columns))
	for _, c := range columns {
		views = append(views, toColumnView(c.Column, c.Cards))
	}
	writeJSON(w, http.StatusOK, views)
}

func (h *Handler) CreateCard(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	var req createCardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "title is required")
		return
	}

	card, err := h.service.CreateCard(r.Context(), req.ColumnID, userID, req.Title, req.Description, req.AssigneeID, req.DueDate)
	if err != nil {
		h.writeCardError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, toCardView(card))
}

func (h *Handler) UpdateCard(w http.ResponseWriter, r *http.Request) {
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

	var req updateCardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "title is required")
		return
	}

	card, err := h.service.UpdateCard(r.Context(), cardID, userID, req.Title, req.Description, req.AssigneeID, req.DueDate)
	if err != nil {
		h.writeCardError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toCardView(card))
}

func (h *Handler) DeleteCard(w http.ResponseWriter, r *http.Request) {
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

	if err := h.service.DeleteCard(r.Context(), cardID, userID); err != nil {
		h.writeCardError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) MoveCard(w http.ResponseWriter, r *http.Request) {
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

	var req moveCardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}

	card, err := h.service.MoveCard(r.Context(), cardID, userID, req.ColumnID, req.Position)
	if err != nil {
		h.writeCardError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toCardView(card))
}

func (h *Handler) writeCardError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, board.ErrNotAMember), errors.Is(err, board.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", err.Error())
	case errors.Is(err, ErrColumnNotFound), errors.Is(err, ErrCardNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrAssigneeNotFound):
		writeError(w, http.StatusBadRequest, "assignee_not_found", err.Error())
	case errors.Is(err, ErrInvalidReorder):
		writeError(w, http.StatusBadRequest, "invalid_reorder", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func parseUUIDParam(r *http.Request, name string) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, name))
}

func toColumnView(c Column, cards []Card) columnView {
	view := columnView{
		ID:        c.ID.String(),
		BoardID:   c.BoardID.String(),
		Title:     c.Title,
		Position:  c.Position,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
	if cards != nil {
		views := make([]cardView, 0, len(cards))
		for _, card := range cards {
			views = append(views, toCardView(card))
		}
		view.Cards = views
	}
	return view
}

func toCardView(c Card) cardView {
	var assigneeID *string
	if c.AssigneeID != nil {
		s := c.AssigneeID.String()
		assigneeID = &s
	}
	return cardView{
		ID:          c.ID.String(),
		ColumnID:    c.ColumnID.String(),
		Title:       c.Title,
		Description: c.Description,
		Position:    c.Position,
		AssigneeID:  assigneeID,
		DueDate:     c.DueDate,
		CreatedAt:   c.CreatedAt,
		UpdatedAt:   c.UpdatedAt,
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
