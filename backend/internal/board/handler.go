package board

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/middleware"
)

type boardService interface {
	CreateBoard(ctx context.Context, ownerID uuid.UUID, name string) (Board, error)
	ListBoards(ctx context.Context, userID uuid.UUID) ([]Board, error)
	GetBoard(ctx context.Context, boardID, requesterID uuid.UUID) (Board, error)
	RenameBoard(ctx context.Context, boardID, requesterID uuid.UUID, name string) (Board, error)
	DeleteBoard(ctx context.Context, boardID, requesterID uuid.UUID) error
	InviteMember(ctx context.Context, boardID, requesterID uuid.UUID, email string) (Member, error)
	RemoveMember(ctx context.Context, boardID, requesterID, targetUserID uuid.UUID) error
	ListMembers(ctx context.Context, boardID, requesterID uuid.UUID) ([]Member, error)
}

type Handler struct {
	service boardService
}

func NewHandler(service boardService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Route("/boards", func(r chi.Router) {
		r.Use(authMiddleware)
		r.Get("/", h.ListBoards)
		r.Post("/", h.CreateBoard)
		r.Route("/{boardID}", func(r chi.Router) {
			r.Get("/", h.GetBoard)
			r.Patch("/", h.RenameBoard)
			r.Delete("/", h.DeleteBoard)
			r.Get("/members", h.ListMembers)
			r.Post("/members", h.InviteMember)
			r.Delete("/members/{userID}", h.RemoveMember)
		})
	})
}

type boardView struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	OwnerID   string    `json:"owner_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type memberView struct {
	UserID    string    `json:"user_id"`
	Role      string    `json:"role"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

type createBoardRequest struct {
	Name string `json:"name"`
}

type renameBoardRequest struct {
	Name string `json:"name"`
}

type inviteMemberRequest struct {
	Email string `json:"email"`
}

func (h *Handler) CreateBoard(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	var req createBoardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}

	board, err := h.service.CreateBoard(r.Context(), userID, req.Name)
	if err != nil {
		h.writeBoardError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, toBoardView(board))
}

func (h *Handler) ListBoards(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	boards, err := h.service.ListBoards(r.Context(), userID)
	if err != nil {
		h.writeBoardError(w, err)
		return
	}

	views := make([]boardView, 0, len(boards))
	for _, b := range boards {
		views = append(views, toBoardView(b))
	}
	writeJSON(w, http.StatusOK, views)
}

func (h *Handler) GetBoard(w http.ResponseWriter, r *http.Request) {
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

	board, err := h.service.GetBoard(r.Context(), boardID, userID)
	if err != nil {
		h.writeBoardError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toBoardView(board))
}

func (h *Handler) RenameBoard(w http.ResponseWriter, r *http.Request) {
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

	var req renameBoardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}

	board, err := h.service.RenameBoard(r.Context(), boardID, userID, req.Name)
	if err != nil {
		h.writeBoardError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toBoardView(board))
}

func (h *Handler) DeleteBoard(w http.ResponseWriter, r *http.Request) {
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

	if err := h.service.DeleteBoard(r.Context(), boardID, userID); err != nil {
		h.writeBoardError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) InviteMember(w http.ResponseWriter, r *http.Request) {
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

	var req inviteMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "email is required")
		return
	}

	member, err := h.service.InviteMember(r.Context(), boardID, userID, req.Email)
	if err != nil {
		h.writeBoardError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, toMemberView(member))
}

func (h *Handler) RemoveMember(w http.ResponseWriter, r *http.Request) {
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

	targetUserID, err := parseUUIDParam(r, "userID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid user id")
		return
	}

	if err := h.service.RemoveMember(r.Context(), boardID, userID, targetUserID); err != nil {
		h.writeBoardError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListMembers(w http.ResponseWriter, r *http.Request) {
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

	members, err := h.service.ListMembers(r.Context(), boardID, userID)
	if err != nil {
		h.writeBoardError(w, err)
		return
	}

	views := make([]memberView, 0, len(members))
	for _, m := range members {
		views = append(views, toMemberView(m))
	}
	writeJSON(w, http.StatusOK, views)
}

func (h *Handler) writeBoardError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotAMember):
		writeError(w, http.StatusForbidden, "forbidden", err.Error())
	case errors.Is(err, ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", err.Error())
	case errors.Is(err, ErrAlreadyMember):
		writeError(w, http.StatusConflict, "already_member", err.Error())
	case errors.Is(err, ErrMemberUserNotFound):
		writeError(w, http.StatusNotFound, "user_not_found", err.Error())
	case errors.Is(err, ErrCannotRemoveOwner):
		writeError(w, http.StatusBadRequest, "cannot_remove_owner", err.Error())
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func parseUUIDParam(r *http.Request, name string) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, name))
}

func toBoardView(b Board) boardView {
	return boardView{
		ID:        b.ID.String(),
		Name:      b.Name,
		OwnerID:   b.OwnerID.String(),
		CreatedAt: b.CreatedAt,
		UpdatedAt: b.UpdatedAt,
	}
}

func toMemberView(m Member) memberView {
	return memberView{
		UserID:    m.UserID.String(),
		Role:      string(m.Role),
		Name:      m.Name,
		Email:     m.Email,
		CreatedAt: m.CreatedAt,
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
