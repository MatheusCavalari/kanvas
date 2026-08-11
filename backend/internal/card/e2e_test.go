//go:build integration

package card_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func TestCardFlow_EndToEnd(t *testing.T) {
	pool := dbtest.NewPool(t)
	queries := gen.New(pool)
	ctx := context.Background()

	owner, err := queries.CreateUser(ctx, gen.CreateUserParams{ID: uuid.New(), Name: "Owner", Email: "owner@example.com", PasswordHash: "hashed"})
	require.NoError(t, err)

	issuer := jwt.NewIssuer("test-secret", time.Hour)

	boardRepo := board.NewPostgresRepository(queries)
	userLookup := board.NewUserLookupAdapter(queries)
	boardService := board.NewService(boardRepo, userLookup)
	boardHandler := board.NewHandler(boardService)

	cardRepo := card.NewPostgresRepository(queries)
	hub := realtime.NewHub()
	cardService := card.NewService(cardRepo, boardService, hub)
	cardHandler := card.NewHandler(cardService)

	router := httpserver.NewRouter()
	authMiddleware := middleware.Auth(issuer)
	boardHandler.RegisterRoutes(router, authMiddleware)
	cardHandler.RegisterRoutes(router, authMiddleware)

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

	// A second, uninvited user must be denied (403) by the real
	// board.Service.EnsureMember -> Postgres GetMember -> 403 chain, not
	// just by the fake authorizer used in unit tests.
	stranger, err := queries.CreateUser(ctx, gen.CreateUserParams{ID: uuid.New(), Name: "Stranger", Email: "stranger@example.com", PasswordHash: "hashed"})
	require.NoError(t, err)
	strangerToken, err := issuer.IssueAccessToken(stranger.ID)
	require.NoError(t, err)

	strangerListReq, _ := http.NewRequest(http.MethodGet, server.URL+"/boards/"+boardCreated.ID+"/columns/", nil)
	strangerListReq.Header.Set("Authorization", "Bearer "+strangerToken)
	strangerListResp, err := client.Do(strangerListReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, strangerListResp.StatusCode)
	_ = strangerListResp.Body.Close()

	strangerCreateCardBody, _ := json.Marshal(map[string]interface{}{
		"column_id": columnCreated.ID,
		"title":     "Should not be created",
	})
	strangerCreateCardReq, _ := http.NewRequest(http.MethodPost, server.URL+"/cards/", bytes.NewReader(strangerCreateCardBody))
	strangerCreateCardReq.Header.Set("Authorization", "Bearer "+strangerToken)
	strangerCreateCardResp, err := client.Do(strangerCreateCardReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, strangerCreateCardResp.StatusCode)
	_ = strangerCreateCardResp.Body.Close()

	// GetBoard and ListMembers prove board's and card's route trees
	// coexist correctly on the same chi router.
	getBoardReq, _ := http.NewRequest(http.MethodGet, server.URL+"/boards/"+boardCreated.ID+"/", nil)
	getBoardReq.Header.Set("Authorization", "Bearer "+ownerToken)
	getBoardResp, err := client.Do(getBoardReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, getBoardResp.StatusCode)
	_ = getBoardResp.Body.Close()

	getMembersReq, _ := http.NewRequest(http.MethodGet, server.URL+"/boards/"+boardCreated.ID+"/members", nil)
	getMembersReq.Header.Set("Authorization", "Bearer "+ownerToken)
	getMembersResp, err := client.Do(getMembersReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, getMembersResp.StatusCode)
	_ = getMembersResp.Body.Close()

	secondColumnBody, _ := json.Marshal(map[string]string{"title": "Doing"})
	secondColumnReq, _ := http.NewRequest(http.MethodPost, server.URL+"/boards/"+boardCreated.ID+"/columns/", bytes.NewReader(secondColumnBody))
	secondColumnReq.Header.Set("Authorization", "Bearer "+ownerToken)
	secondColumnResp, err := client.Do(secondColumnReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, secondColumnResp.StatusCode)
	var secondColumnCreated struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(secondColumnResp.Body).Decode(&secondColumnCreated))
	_ = secondColumnResp.Body.Close()

	createCardBody, _ := json.Marshal(map[string]interface{}{
		"column_id": columnCreated.ID,
		"title":     "Write the plan",
	})
	createCardReq, _ := http.NewRequest(http.MethodPost, server.URL+"/cards/", bytes.NewReader(createCardBody))
	createCardReq.Header.Set("Authorization", "Bearer "+ownerToken)
	createCardResp, err := client.Do(createCardReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, createCardResp.StatusCode)
	var cardCreated struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(createCardResp.Body).Decode(&cardCreated))
	_ = createCardResp.Body.Close()

	strangerMoveBody, _ := json.Marshal(map[string]interface{}{
		"column_id": secondColumnCreated.ID,
		"position":  0,
	})
	strangerMoveReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/cards/"+cardCreated.ID+"/move", bytes.NewReader(strangerMoveBody))
	strangerMoveReq.Header.Set("Authorization", "Bearer "+strangerToken)
	strangerMoveResp, err := client.Do(strangerMoveReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, strangerMoveResp.StatusCode)
	_ = strangerMoveResp.Body.Close()

	moveBody, _ := json.Marshal(map[string]interface{}{
		"column_id": secondColumnCreated.ID,
		"position":  0,
	})
	moveReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/cards/"+cardCreated.ID+"/move", bytes.NewReader(moveBody))
	moveReq.Header.Set("Authorization", "Bearer "+ownerToken)
	moveResp, err := client.Do(moveReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, moveResp.StatusCode)
	_ = moveResp.Body.Close()

	listReq, _ := http.NewRequest(http.MethodGet, server.URL+"/boards/"+boardCreated.ID+"/columns/", nil)
	listReq.Header.Set("Authorization", "Bearer "+ownerToken)
	listResp, err := client.Do(listReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, listResp.StatusCode)
	var columns []struct {
		ID    string `json:"id"`
		Cards []struct {
			ID string `json:"id"`
		} `json:"cards"`
	}
	require.NoError(t, json.NewDecoder(listResp.Body).Decode(&columns))
	_ = listResp.Body.Close()
	require.Len(t, columns, 2)
	require.Empty(t, columns[0].Cards)
	require.Len(t, columns[1].Cards, 1)
	require.Equal(t, cardCreated.ID, columns[1].Cards[0].ID)

	deleteReq, _ := http.NewRequest(http.MethodDelete, server.URL+"/cards/"+cardCreated.ID, nil)
	deleteReq.Header.Set("Authorization", "Bearer "+ownerToken)
	deleteResp, err := client.Do(deleteReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, deleteResp.StatusCode)
	_ = deleteResp.Body.Close()
}
