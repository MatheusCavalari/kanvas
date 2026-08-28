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

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/middleware"
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

// UserNameLookup resolves a user ID to a display name for presence. It
// exists so this package can attach a human-readable name to a
// presence.joined event without importing internal/auth or internal/board
// directly — the same structural-dependency pattern TokenParser and
// BoardAuthorizer already use here.
type UserNameLookup interface {
	GetUserName(ctx context.Context, userID uuid.UUID) (string, error)
}

type Handler struct {
	hub           *Hub
	tokens        TokenParser
	board         BoardAuthorizer
	names         UserNameLookup
	allowedOrigin string
}

func NewHandler(hub *Hub, tokens TokenParser, board BoardAuthorizer, names UserNameLookup, allowedOrigin string) *Handler {
	return &Handler{hub: hub, tokens: tokens, board: board, names: names, allowedOrigin: allowedOrigin}
}

// RegisterWSRoute registers the WebSocket upgrade endpoint. It must be
// mounted outside the app's protected route group (and outside any
// rate-limiting middleware keyed on the standard Authorization header):
// browsers cannot set custom headers on a WebSocket handshake, so the
// access token travels as a query parameter instead and is validated
// inside ServeWS itself.
func (h *Handler) RegisterWSRoute(r chi.Router) {
	r.Get("/boards/{boardID}/ws", h.ServeWS)
}

// RegisterPresenceRoute registers the presence REST endpoint behind
// authMiddleware, the same protected-route wiring every other REST
// endpoint in this codebase uses. Unlike ServeWS this is a normal HTTP
// request that can carry a Bearer header, so it belongs in the app's
// protected route group (with its rate limiting) rather than alongside
// the WebSocket route.
func (h *Handler) RegisterPresenceRoute(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.With(authMiddleware).Get("/boards/{boardID}/presence", h.GetPresence)
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

	name := h.lookupUserName(r.Context(), userID)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	h.hub.JoinPresence(boardID, userID, name)
	defer h.hub.LeavePresence(boardID, userID)

	events := h.hub.subscribe(boardID)
	defer h.hub.unsubscribe(boardID, events)

	// The read loop owns the connection's read side: it drains incoming
	// frames (this is required by the WebSocket protocol even though the
	// client mostly just sends heartbeat pings) and cancels ctx once the
	// connection is gone, which unblocks the write loop below.
	go h.readLoop(ctx, cancel, conn, boardID, userID)

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

// lookupUserName resolves userID to a display name for presence. A lookup
// failure (or no lookup configured) degrades to an empty name rather than
// failing the connection — presence is a nice-to-have, not something worth
// rejecting a WebSocket handshake over.
func (h *Handler) lookupUserName(ctx context.Context, userID uuid.UUID) string {
	if h.names == nil {
		return ""
	}
	name, err := h.names.GetUserName(ctx, userID)
	if err != nil {
		return ""
	}
	return name
}

// clientMessage is the shape of JSON frames the client sends over the
// WebSocket. Currently the only recognized type is "ping", used to refresh
// presence; anything else is read and silently ignored.
type clientMessage struct {
	Type string `json:"type"`
}

// readLoop drains incoming client frames until the connection closes or
// ctx is cancelled by the write loop, calling cancel itself so the write
// loop unblocks as soon as the read side dies (a closed/broken connection
// otherwise only shows up as a failed write, which can lag behind).
func (h *Handler) readLoop(ctx context.Context, cancel context.CancelFunc, conn *websocket.Conn, boardID, userID uuid.UUID) {
	defer cancel()
	for {
		var msg clientMessage
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			return
		}
		if msg.Type == "ping" {
			h.hub.Heartbeat(boardID, userID)
		}
	}
}

// GetPresence returns the users currently connected to boardID's
// WebSocket. Unlike ServeWS, this sits behind the shared authMiddleware
// (registered via RegisterPresenceRoute), so the caller's user ID is
// already in the request context by the time this runs.
func (h *Handler) GetPresence(w http.ResponseWriter, r *http.Request) {
	boardID, err := uuid.Parse(chi.URLParam(r, "boardID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid board id")
		return
	}

	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	if err := h.board.EnsureMember(r.Context(), boardID, userID); err != nil {
		writeError(w, http.StatusForbidden, "forbidden", "forbidden")
		return
	}

	presence := h.hub.GetPresence(boardID)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(presence)
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
