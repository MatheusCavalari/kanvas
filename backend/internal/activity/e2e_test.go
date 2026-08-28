//go:build integration

package activity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MatheusCavalari/kanvas/backend/internal/activity"
	"github.com/MatheusCavalari/kanvas/backend/internal/board"
	"github.com/MatheusCavalari/kanvas/backend/internal/card"
	"github.com/MatheusCavalari/kanvas/backend/internal/label"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/dbtest"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/gen"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/httpserver"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/jwt"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/middleware"
	"github.com/MatheusCavalari/kanvas/backend/internal/realtime"
)

func TestActivityLog_EndToEnd(t *testing.T) {
	pool := dbtest.NewPool(t)
	queries := gen.New(pool)
	ctx := context.Background()

	owner, err := queries.CreateUser(ctx, gen.CreateUserParams{ID: uuid.New(), Name: "Owner", Email: "activity-owner@example.com", PasswordHash: "hashed"})
	require.NoError(t, err)

	issuer := jwt.NewIssuer("test-secret", time.Hour)

	boardRepo := board.NewPostgresRepository(queries)
	userLookup := board.NewUserLookupAdapter(queries)
	boardService := board.NewService(boardRepo, userLookup)
	boardHandler := board.NewHandler(boardService)

	hub := realtime.NewHub()

	activityRepo := activity.NewPostgresRepository(queries)
	activityRecorder := activity.NewRecorder(activityRepo, hub)
	activityHandler := activity.NewHandler(activityRepo, boardService)

	cardRepo := card.NewPostgresRepository(queries)
	cardService := card.NewService(cardRepo, boardService, activityRecorder)
	cardHandler := card.NewHandler(cardService)

	labelRepo := label.NewPostgresRepository(queries)
	labelService := label.NewService(labelRepo, boardService, labelRepo, activityRecorder)
	labelHandler := label.NewHandler(labelService)

	router := httpserver.NewRouter("http://localhost:5173")
	authMiddleware := middleware.Auth(issuer)
	boardHandler.RegisterRoutes(router, authMiddleware)
	cardHandler.RegisterRoutes(router, authMiddleware)
	labelHandler.RegisterRoutes(router, authMiddleware)
	activityHandler.RegisterRoutes(router, authMiddleware)

	server := httptest.NewServer(router)
	defer server.Close()
	client := server.Client()

	ownerToken, err := issuer.IssueAccessToken(owner.ID)
	require.NoError(t, err)

	// Create a board and a column: two events that should each be recorded.
	createBoardBody, _ := json.Marshal(map[string]string{"name": "Activity Board"})
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

	// A stranger cannot read the activity log for a board they don't belong to.
	stranger, err := queries.CreateUser(ctx, gen.CreateUserParams{ID: uuid.New(), Name: "Stranger", Email: "activity-stranger@example.com", PasswordHash: "hashed"})
	require.NoError(t, err)
	strangerToken, err := issuer.IssueAccessToken(stranger.ID)
	require.NoError(t, err)

	strangerListReq, _ := http.NewRequest(http.MethodGet, server.URL+"/boards/"+boardCreated.ID+"/activity/", nil)
	strangerListReq.Header.Set("Authorization", "Bearer "+strangerToken)
	strangerListResp, err := client.Do(strangerListReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, strangerListResp.StatusCode)
	_ = strangerListResp.Body.Close()

	createColumnBody, _ := json.Marshal(map[string]string{"title": "To Do"})
	createColumnReq, _ := http.NewRequest(http.MethodPost, server.URL+"/boards/"+boardCreated.ID+"/columns/", bytes.NewReader(createColumnBody))
	createColumnReq.Header.Set("Authorization", "Bearer "+ownerToken)
	createColumnResp, err := client.Do(createColumnReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, createColumnResp.StatusCode)
	var columnCreated struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(createColumnResp.Body).Decode(&columnCreated))
	_ = createColumnResp.Body.Close()

	// Creating a board does NOT go through activityRecorder (board.Service
	// is wired without an EventPublisher in this app), so the first entry
	// we expect is the column creation.
	listReq, _ := http.NewRequest(http.MethodGet, server.URL+"/boards/"+boardCreated.ID+"/activity/", nil)
	listReq.Header.Set("Authorization", "Bearer "+ownerToken)
	listResp, err := client.Do(listReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, listResp.StatusCode)

	type entryView struct {
		ID         string `json:"id"`
		BoardID    string `json:"board_id"`
		ActorID    string `json:"actor_id"`
		Action     string `json:"action"`
		EntityType string `json:"entity_type"`
		EntityID   string `json:"entity_id"`
	}
	type listResponse struct {
		Entries    []entryView `json:"entries"`
		NextCursor string      `json:"next_cursor"`
	}
	var page listResponse
	require.NoError(t, json.NewDecoder(listResp.Body).Decode(&page))
	_ = listResp.Body.Close()

	require.Len(t, page.Entries, 1)
	require.Equal(t, "column", page.Entries[0].EntityType)
	require.Equal(t, "created", page.Entries[0].Action)
	require.Equal(t, columnCreated.ID, page.Entries[0].EntityID)
	require.Equal(t, owner.ID.String(), page.Entries[0].ActorID)
	require.Equal(t, boardCreated.ID, page.Entries[0].BoardID)

	// Create several labels to exercise cursor pagination (newest first).
	var labelIDs []string
	for i := 0; i < 4; i++ {
		body, _ := json.Marshal(map[string]string{"name": fmt.Sprintf("label-%d", i), "color": "#ffffff"})
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/boards/"+boardCreated.ID+"/labels/", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+ownerToken)
		resp, err := client.Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		var l struct {
			ID string `json:"id"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&l))
		_ = resp.Body.Close()
		labelIDs = append(labelIDs, l.ID)
		time.Sleep(2 * time.Millisecond)
	}

	// Total entries now: 1 (column.created) + 4 (label.created) = 5.
	// Page through with limit=2, newest first.
	var allEntityIDs []string
	cursor := ""
	for {
		url := server.URL + "/boards/" + boardCreated.ID + "/activity/?limit=2"
		if cursor != "" {
			url += "&cursor=" + cursor
		}
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("Authorization", "Bearer "+ownerToken)
		resp, err := client.Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var p listResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&p))
		_ = resp.Body.Close()

		for _, e := range p.Entries {
			allEntityIDs = append(allEntityIDs, e.EntityID)
		}
		if p.NextCursor == "" {
			break
		}
		cursor = p.NextCursor
	}

	require.Len(t, allEntityIDs, 5)
	// Newest first: the four labels (in reverse creation order), then the column.
	require.Equal(t, labelIDs[3], allEntityIDs[0])
	require.Equal(t, labelIDs[2], allEntityIDs[1])
	require.Equal(t, labelIDs[1], allEntityIDs[2])
	require.Equal(t, labelIDs[0], allEntityIDs[3])
	require.Equal(t, columnCreated.ID, allEntityIDs[4])
}
