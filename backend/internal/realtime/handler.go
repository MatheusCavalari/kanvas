package realtime

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// TokenParser validates a JWT access token and returns the user ID it
// was issued to. Implemented by *jwt.Issuer — this package never imports
// internal/platform/jwt directly, same decoupling pattern used elsewhere.
type TokenParser interface {
	ParseAccessToken(token string) (uuid.UUID, error)
}

// BoardAuthorizer checks board membership. Implemented by *board.Service,
// the same interface shape internal/card already depends on.
type BoardAuthorizer interface {
	EnsureMember(ctx context.Context, boardID, userID uuid.UUID) error
}

type Handler struct {
	hub           *Hub
	tokens        TokenParser
	board         BoardAuthorizer
	allowedOrigin string
}

func NewHandler(hub *Hub, tokens TokenParser, board BoardAuthorizer, allowedOrigin string) *Handler {
	return &Handler{hub: hub, tokens: tokens, board: board, allowedOrigin: allowedOrigin}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	// Deliberately NOT behind the shared authMiddleware: browsers cannot
	// set custom headers on a WebSocket handshake, so the access token
	// travels as a query parameter instead and is validated here.
	r.Get("/boards/{boardID}/ws", h.ServeWS)
}

func (h *Handler) ServeWS(w http.ResponseWriter, r *http.Request) {
	boardID, err := uuid.Parse(chi.URLParam(r, "boardID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid board id")
		return
	}

	token := r.URL.Query().Get("token")
	if token == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing token")
		return
	}

	userID, err := h.tokens.ParseAccessToken(token)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid or expired token")
		return
	}

	if err := h.board.EnsureMember(r.Context(), boardID, userID); err != nil {
		writeError(w, http.StatusForbidden, "forbidden", "forbidden")
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{h.allowedOrigin}})
	if err != nil {
		return
	}
	defer func() { _ = conn.CloseNow() }()

	ctx := conn.CloseRead(r.Context())

	events := h.hub.subscribe(boardID)
	defer h.hub.unsubscribe(boardID, events)

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := wsjson.Write(writeCtx, conn, event)
			cancel()
			if err != nil {
				return
			}
		}
	}
}

// errorResponse is the project's standard JSON error envelope. Duplicated
// here rather than imported from internal/card — this codebase's existing
// pattern is small per-package duplication over a shared error package.
type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorResponse{Error: errorBody{Code: code, Message: message}})
}
