//go:build integration

package realtime_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MatheusCavalari/kanvas/backend/internal/board"
	"github.com/MatheusCavalari/kanvas/backend/internal/card"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/dbtest"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/gen"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/httpserver"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/jwt"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/middleware"
	"github.com/MatheusCavalari/kanvas/backend/internal/realtime"
)

func TestRealtimeFlow_EndToEnd(t *testing.T) {
	pool := dbtest.NewPool(t)
	queries := gen.New(pool)
	ctx := context.Background()

	owner, err := queries.CreateUser(ctx, gen.CreateUserParams{ID: uuid.New(), Name: "Owner", Email: "owner@example.com", PasswordHash: "hashed"})
	require.NoError(t, err)

	issuer := jwt.NewIssuer("test-secret", time.Hour)
	authMiddleware := middleware.Auth(issuer)

	boardRepo := board.NewPostgresRepository(queries)
	userLookup := board.NewUserLookupAdapter(queries)
	boardService := board.NewService(boardRepo, userLookup)
	boardHandler := board.NewHandler(boardService)

	hub := realtime.NewHub()
	cardRepo := card.NewPostgresRepository(queries)
	cardService := card.NewService(cardRepo, boardService, hub)
	cardHandler := card.NewHandler(cardService)
	realtimeHandler := realtime.NewHandler(hub, issuer, boardService, nil, "http://localhost:5173")

	router := httpserver.NewRouter("http://localhost:5173")
	boardHandler.RegisterRoutes(router, authMiddleware)
	cardHandler.RegisterRoutes(router, authMiddleware)
	realtimeHandler.RegisterWSRoute(router)

	server := httptest.NewServer(router)
	defer server.Close()
	client := server.Client()

	ownerToken, err := issuer.IssueAccessToken(owner.ID)
	require.NoError(t, err)

	createBoardBody, _ := json.Marshal(map[string]string{"name": "Sprint Board"})
	createBoardReq, _ := http.NewRequest(http.MethodPost, server.URL+"/boards/", bytes.NewReader(createBoardBody))
	createBoardReq.Header.Set("Authorization", "Bearer "+ownerToken)
	createBoardResp, err := client.Do(createBoardReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, createBoardResp.StatusCode)
	var boardCreated struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(createBoardResp.Body).Decode(&boardCreated))
	_ = createBoardResp.Body.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/boards/" + boardCreated.ID + "/ws?token=" + ownerToken
	dialCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(dialCtx, wsURL, nil)
	require.NoError(t, err)
	defer func() { _ = conn.CloseNow() }()

	boardUUID, err := uuid.Parse(boardCreated.ID)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return hub.SubscriberCount(boardUUID) == 1
	}, 2*time.Second, 10*time.Millisecond)

	createColumnBody, _ := json.Marshal(map[string]string{"title": "To Do"})
	createColumnReq, _ := http.NewRequest(http.MethodPost, server.URL+"/boards/"+boardCreated.ID+"/columns/", bytes.NewReader(createColumnBody))
	createColumnReq.Header.Set("Authorization", "Bearer "+ownerToken)
	createColumnResp, err := client.Do(createColumnReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, createColumnResp.StatusCode)
	_ = createColumnResp.Body.Close()

	readCtx, readCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer readCancel()
	var event realtime.Event
	require.NoError(t, wsjson.Read(readCtx, conn, &event))
	require.Equal(t, "column.created", event.Type)
	require.Equal(t, boardUUID, event.BoardID)

	conn.Close(websocket.StatusNormalClosure, "")
}

func TestRealtimePresence_EndToEnd(t *testing.T) {
	pool := dbtest.NewPool(t)
	queries := gen.New(pool)
	ctx := context.Background()

	owner, err := queries.CreateUser(ctx, gen.CreateUserParams{ID: uuid.New(), Name: "Presence Owner", Email: "presence-owner@example.com", PasswordHash: "hashed"})
	require.NoError(t, err)

	issuer := jwt.NewIssuer("test-secret", time.Hour)
	authMiddleware := middleware.Auth(issuer)

	boardRepo := board.NewPostgresRepository(queries)
	userLookup := board.NewUserLookupAdapter(queries)
	boardService := board.NewService(boardRepo, userLookup)
	boardHandler := board.NewHandler(boardService)

	hub := realtime.NewHub()
	names := realtime.NewUserNameLookupAdapter(queries)
	realtimeHandler := realtime.NewHandler(hub, issuer, boardService, names, "http://localhost:5173")

	router := httpserver.NewRouter("http://localhost:5173")
	boardHandler.RegisterRoutes(router, authMiddleware)
	realtimeHandler.RegisterWSRoute(router)
	realtimeHandler.RegisterPresenceRoute(router, authMiddleware)

	server := httptest.NewServer(router)
	defer server.Close()
	client := server.Client()

	ownerToken, err := issuer.IssueAccessToken(owner.ID)
	require.NoError(t, err)

	createBoardBody, _ := json.Marshal(map[string]string{"name": "Presence Board"})
	createBoardReq, _ := http.NewRequest(http.MethodPost, server.URL+"/boards/", bytes.NewReader(createBoardBody))
	createBoardReq.Header.Set("Authorization", "Bearer "+ownerToken)
	createBoardResp, err := client.Do(createBoardReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, createBoardResp.StatusCode)
	var boardCreated struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(createBoardResp.Body).Decode(&boardCreated))
	_ = createBoardResp.Body.Close()
	boardUUID, err := uuid.Parse(boardCreated.ID)
	require.NoError(t, err)

	getPresence := func() []realtime.PresenceInfo {
		req, _ := http.NewRequest(http.MethodGet, server.URL+"/boards/"+boardCreated.ID+"/presence", nil)
		req.Header.Set("Authorization", "Bearer "+ownerToken)
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var out []realtime.PresenceInfo
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
		return out
	}

	// No one connected yet.
	require.Empty(t, getPresence())

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/boards/" + boardCreated.ID + "/ws?token=" + ownerToken
	dialCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(dialCtx, wsURL, nil)
	require.NoError(t, err)
	defer func() { _ = conn.CloseNow() }()

	// Connecting joins presence with the user's name resolved via
	// UserNameLookup, and is visible both through the REST endpoint and
	// hub.GetPresence directly.
	require.Eventually(t, func() bool {
		return hub.SubscriberCount(boardUUID) == 1
	}, 2*time.Second, 10*time.Millisecond)

	list := getPresence()
	require.Len(t, list, 1)
	require.Equal(t, owner.ID, list[0].UserID)
	require.Equal(t, "Presence Owner", list[0].Name)

	// A ping heartbeat should be accepted without the handler erroring or
	// the connection being torn down.
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 2*time.Second)
	require.NoError(t, wsjson.Write(pingCtx, conn, map[string]string{"type": "ping"}))
	pingCancel()

	// Give the read loop a moment to process the ping before asserting the
	// connection is still alive.
	require.Eventually(t, func() bool {
		return hub.SubscriberCount(boardUUID) == 1
	}, time.Second, 10*time.Millisecond)

	conn.Close(websocket.StatusNormalClosure, "")

	// Disconnecting leaves presence.
	require.Eventually(t, func() bool {
		return len(hub.GetPresence(boardUUID)) == 0
	}, 2*time.Second, 10*time.Millisecond)
	require.Empty(t, getPresence())
}
