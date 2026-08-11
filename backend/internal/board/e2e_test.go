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
	stranger, err := queries.CreateUser(ctx, gen.CreateUserParams{ID: uuid.New(), Name: "Stranger", Email: "stranger@example.com", PasswordHash: "hashed"})
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
	inviteeToken, err := issuer.IssueAccessToken(invitee.ID)
	require.NoError(t, err)
	strangerToken, err := issuer.IssueAccessToken(stranger.ID)
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

	// GET /boards as the owner must include the newly created board.
	ownerListReq, _ := http.NewRequest(http.MethodGet, server.URL+"/boards/", nil)
	ownerListReq.Header.Set("Authorization", "Bearer "+ownerToken)
	ownerListResp, err := client.Do(ownerListReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, ownerListResp.StatusCode)
	var ownerBoards []struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(ownerListResp.Body).Decode(&ownerBoards))
	_ = ownerListResp.Body.Close()
	found := false
	for _, b := range ownerBoards {
		if b.ID == created.ID {
			found = true
		}
	}
	require.True(t, found, "owner's board list should contain the created board")

	// GET /boards as a completely unrelated user (never invited to
	// anything) must come back empty — proving ListBoardsForUser doesn't
	// leak boards across users over real HTTP.
	strangerListReq, _ := http.NewRequest(http.MethodGet, server.URL+"/boards/", nil)
	strangerListReq.Header.Set("Authorization", "Bearer "+strangerToken)
	strangerListResp, err := client.Do(strangerListReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, strangerListResp.StatusCode)
	var strangerBoards []struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(strangerListResp.Body).Decode(&strangerBoards))
	_ = strangerListResp.Body.Close()
	require.Empty(t, strangerBoards)

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

	// A plain member (the invitee) can view the board.
	inviteeGetReq, _ := http.NewRequest(http.MethodGet, server.URL+"/boards/"+created.ID, nil)
	inviteeGetReq.Header.Set("Authorization", "Bearer "+inviteeToken)
	inviteeGetResp, err := client.Do(inviteeGetReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, inviteeGetResp.StatusCode)
	_ = inviteeGetResp.Body.Close()

	// A plain member cannot delete the board.
	inviteeDeleteReq, _ := http.NewRequest(http.MethodDelete, server.URL+"/boards/"+created.ID, nil)
	inviteeDeleteReq.Header.Set("Authorization", "Bearer "+inviteeToken)
	inviteeDeleteResp, err := client.Do(inviteeDeleteReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, inviteeDeleteResp.StatusCode)
	_ = inviteeDeleteResp.Body.Close()

	// A stranger (never a member) cannot view the board.
	strangerGetReq, _ := http.NewRequest(http.MethodGet, server.URL+"/boards/"+created.ID, nil)
	strangerGetReq.Header.Set("Authorization", "Bearer "+strangerToken)
	strangerGetResp, err := client.Do(strangerGetReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, strangerGetResp.StatusCode)
	_ = strangerGetResp.Body.Close()

	removeReq, _ := http.NewRequest(http.MethodDelete, server.URL+"/boards/"+created.ID+"/members/"+invitee.ID.String(), nil)
	removeReq.Header.Set("Authorization", "Bearer "+ownerToken)
	removeResp, err := client.Do(removeReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, removeResp.StatusCode)
	_ = removeResp.Body.Close()

	// After removal, the (now-removed) invitee's access is revoked.
	removedGetReq, _ := http.NewRequest(http.MethodGet, server.URL+"/boards/"+created.ID, nil)
	removedGetReq.Header.Set("Authorization", "Bearer "+inviteeToken)
	removedGetResp, err := client.Do(removedGetReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, removedGetResp.StatusCode)
	_ = removedGetResp.Body.Close()

	deleteReq, _ := http.NewRequest(http.MethodDelete, server.URL+"/boards/"+created.ID, nil)
	deleteReq.Header.Set("Authorization", "Bearer "+ownerToken)
	deleteResp, err := client.Do(deleteReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, deleteResp.StatusCode)
	_ = deleteResp.Body.Close()
}
