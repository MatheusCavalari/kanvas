//go:build integration

package board_test

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
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/dbtest"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/gen"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/httpserver"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/jwt"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/middleware"
)

func TestBoardFlow_EndToEnd(t *testing.T) {
	pool := dbtest.NewPool(t)
	queries := gen.New(pool)
	ctx := context.Background()

	owner, err := queries.CreateUser(ctx, gen.CreateUserParams{ID: uuid.New(), Name: "Owner", Email: "owner@example.com", PasswordHash: "hashed"})
	require.NoError(t, err)
	invitee, err := queries.CreateUser(ctx, gen.CreateUserParams{ID: uuid.New(), Name: "Invitee", Email: "invitee@example.com", PasswordHash: "hashed"})
	require.NoError(t, err)

	issuer := jwt.NewIssuer("test-secret", time.Hour)
	repo := board.NewPostgresRepository(queries)
	userLookup := board.NewUserLookupAdapter(queries)
	service := board.NewService(repo, userLookup)
	handler := board.NewHandler(service)

	router := httpserver.NewRouter()
	handler.RegisterRoutes(router, middleware.Auth(issuer))

	server := httptest.NewServer(router)
	defer server.Close()
	client := server.Client()

	ownerToken, err := issuer.IssueAccessToken(owner.ID)
	require.NoError(t, err)

	createBody, _ := json.Marshal(map[string]string{"name": "Sprint Board"})
	createReq, _ := http.NewRequest(http.MethodPost, server.URL+"/boards/", bytes.NewReader(createBody))
	createReq.Header.Set("Authorization", "Bearer "+ownerToken)
	createResp, err := client.Do(createReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(createResp.Body).Decode(&created))
	_ = createResp.Body.Close()

	inviteBody, _ := json.Marshal(map[string]string{"email": "invitee@example.com"})
	inviteReq, _ := http.NewRequest(http.MethodPost, server.URL+"/boards/"+created.ID+"/members", bytes.NewReader(inviteBody))
	inviteReq.Header.Set("Authorization", "Bearer "+ownerToken)
	inviteResp, err := client.Do(inviteReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, inviteResp.StatusCode)
	_ = inviteResp.Body.Close()

	listReq, _ := http.NewRequest(http.MethodGet, server.URL+"/boards/"+created.ID+"/members", nil)
	listReq.Header.Set("Authorization", "Bearer "+ownerToken)
	listResp, err := client.Do(listReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, listResp.StatusCode)

	var members []map[string]string
	require.NoError(t, json.NewDecoder(listResp.Body).Decode(&members))
	require.Len(t, members, 2)
	_ = listResp.Body.Close()

	removeReq, _ := http.NewRequest(http.MethodDelete, server.URL+"/boards/"+created.ID+"/members/"+invitee.ID.String(), nil)
	removeReq.Header.Set("Authorization", "Bearer "+ownerToken)
	removeResp, err := client.Do(removeReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, removeResp.StatusCode)
	_ = removeResp.Body.Close()

	deleteReq, _ := http.NewRequest(http.MethodDelete, server.URL+"/boards/"+created.ID, nil)
	deleteReq.Header.Set("Authorization", "Bearer "+ownerToken)
	deleteResp, err := client.Do(deleteReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, deleteResp.StatusCode)
	_ = deleteResp.Body.Close()
}
