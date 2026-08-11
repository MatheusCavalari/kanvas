package realtime

import (
	"context"
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
	hub    *Hub
	tokens TokenParser
	board  BoardAuthorizer
}

func NewHandler(hub *Hub, tokens TokenParser, board BoardAuthorizer) *Handler {
	return &Handler{hub: hub, tokens: tokens, board: board}
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
		http.Error(w, "invalid board id", http.StatusBadRequest)
		return
	}

	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}

	userID, err := h.tokens.ParseAccessToken(token)
	if err != nil {
		http.Error(w, "invalid or expired token", http.StatusUnauthorized)
		return
	}

	if err := h.board.EnsureMember(r.Context(), boardID, userID); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// InsecureSkipVerify disables the Origin check. Acceptable for now:
	// there is no browser frontend yet (Phase 5+) and no cookie-based
	// auth on this endpoint (the token is an explicit query parameter,
	// not an ambient credential), so there's no CSRF-style risk this
	// check would prevent. Revisit once the frontend's origin is known
	// and replace with an explicit OriginPatterns allowlist.
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer conn.CloseNow()

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
