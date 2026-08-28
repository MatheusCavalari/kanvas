//go:build integration

package comment_test

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
	"github.com/MatheusCavalari/kanvas/backend/internal/comment"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/dbtest"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/gen"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/httpserver"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/jwt"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/middleware"
	"github.com/MatheusCavalari/kanvas/backend/internal/realtime"
)

func TestCommentFlow_EndToEnd(t *testing.T) {
	pool := dbtest.NewPool(t)
	queries := gen.New(pool)
	ctx := context.Background()

	owner, err := queries.CreateUser(ctx, gen.CreateUserParams{ID: uuid.New(), Name: "Owner", Email: "comment-owner@example.com", PasswordHash: "hashed"})
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

	commentRepo := comment.NewPostgresRepository(queries)
	commentService := comment.NewService(commentRepo, commentRepo, boardService, boardService, hub)
	commentHandler := comment.NewHandler(commentService)

	router := httpserver.NewRouter("http://localhost:5173")
	authMiddleware := middleware.Auth(issuer)
	boardHandler.RegisterRoutes(router, authMiddleware)
	cardHandler.RegisterRoutes(router, authMiddleware)
	commentHandler.RegisterRoutes(router, authMiddleware)

	server := httptest.NewServer(router)
	defer server.Close()
	client := server.Client()

	ownerToken, err := issuer.IssueAccessToken(owner.ID)
	require.NoError(t, err)

	// Create a board, column, and card to comment on.
	createBoardBody, _ := json.Marshal(map[string]string{"name": "Comment Board"})
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

	createCardBody, _ := json.Marshal(map[string]interface{}{
		"column_id": columnCreated.ID,
		"title":     "Discuss this",
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

	// A stranger (not a board member) cannot comment.
	stranger, err := queries.CreateUser(ctx, gen.CreateUserParams{ID: uuid.New(), Name: "Stranger", Email: "comment-stranger@example.com", PasswordHash: "hashed"})
	require.NoError(t, err)
	strangerToken, err := issuer.IssueAccessToken(stranger.ID)
	require.NoError(t, err)

	strangerCommentBody, _ := json.Marshal(map[string]string{"body": "sneaking in"})
	strangerCommentReq, _ := http.NewRequest(http.MethodPost, server.URL+"/cards/"+cardCreated.ID+"/comments/", bytes.NewReader(strangerCommentBody))
	strangerCommentReq.Header.Set("Authorization", "Bearer "+strangerToken)
	strangerCommentResp, err := client.Do(strangerCommentReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, strangerCommentResp.StatusCode)
	_ = strangerCommentResp.Body.Close()

	// Invite the stranger onto the board so we can test author-only edit
	// rules and owner-can-delete rules with a real second member.
	inviteBody, _ := json.Marshal(map[string]string{"email": "comment-stranger@example.com"})
	inviteReq, _ := http.NewRequest(http.MethodPost, server.URL+"/boards/"+boardCreated.ID+"/members", bytes.NewReader(inviteBody))
	inviteReq.Header.Set("Authorization", "Bearer "+ownerToken)
	inviteResp, err := client.Do(inviteReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, inviteResp.StatusCode)
	_ = inviteResp.Body.Close()

	// Owner creates a comment.
	createCommentBody, _ := json.Marshal(map[string]string{"body": "First comment"})
	createCommentReq, _ := http.NewRequest(http.MethodPost, server.URL+"/cards/"+cardCreated.ID+"/comments/", bytes.NewReader(createCommentBody))
	createCommentReq.Header.Set("Authorization", "Bearer "+ownerToken)
	createCommentResp, err := client.Do(createCommentReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, createCommentResp.StatusCode)
	var commentCreated struct {
		ID        string `json:"id"`
		Body      string `json:"body"`
		CreatedAt string `json:"created_at"`
	}
	require.NoError(t, json.NewDecoder(createCommentResp.Body).Decode(&commentCreated))
	_ = createCommentResp.Body.Close()
	require.Equal(t, "First comment", commentCreated.Body)

	// Member (now a real board member) cannot edit the owner's comment.
	memberEditBody, _ := json.Marshal(map[string]string{"body": "hijacked"})
	memberEditReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/comments/"+commentCreated.ID, bytes.NewReader(memberEditBody))
	memberEditReq.Header.Set("Authorization", "Bearer "+strangerToken)
	memberEditResp, err := client.Do(memberEditReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, memberEditResp.StatusCode)
	_ = memberEditResp.Body.Close()

	// Author edits their own comment.
	ownerEditBody, _ := json.Marshal(map[string]string{"body": "Edited by author"})
	ownerEditReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/comments/"+commentCreated.ID, bytes.NewReader(ownerEditBody))
	ownerEditReq.Header.Set("Authorization", "Bearer "+ownerToken)
	ownerEditResp, err := client.Do(ownerEditReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, ownerEditResp.StatusCode)
	var commentEdited struct {
		Body string `json:"body"`
	}
	require.NoError(t, json.NewDecoder(ownerEditResp.Body).Decode(&commentEdited))
	_ = ownerEditResp.Body.Close()
	require.Equal(t, "Edited by author", commentEdited.Body)

	// Create several more comments to exercise cursor pagination.
	for i := 0; i < 4; i++ {
		body, _ := json.Marshal(map[string]string{"body": "comment"})
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/cards/"+cardCreated.ID+"/comments/", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+ownerToken)
		resp, err := client.Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		_ = resp.Body.Close()
		time.Sleep(2 * time.Millisecond)
	}

	// Total comments now: 1 (edited) + 4 = 5. Page through with limit=2.
	type listResp struct {
		Comments []struct {
			ID string `json:"id"`
		} `json:"comments"`
		NextCursor string `json:"next_cursor"`
	}

	var allIDs []string
	cursor := ""
	for {
		url := server.URL + "/cards/" + cardCreated.ID + "/comments/?limit=2"
		if cursor != "" {
			url += "&cursor=" + cursor
		}
		listReq, _ := http.NewRequest(http.MethodGet, url, nil)
		listReq.Header.Set("Authorization", "Bearer "+ownerToken)
		listResp2, err := client.Do(listReq)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, listResp2.StatusCode)
		var page listResp
		require.NoError(t, json.NewDecoder(listResp2.Body).Decode(&page))
		_ = listResp2.Body.Close()

		for _, c := range page.Comments {
			allIDs = append(allIDs, c.ID)
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	require.Len(t, allIDs, 5)
	require.Equal(t, commentCreated.ID, allIDs[0])

	// Board owner can delete a comment even though they're not the author.
	deleteByOwnerReq, _ := http.NewRequest(http.MethodDelete, server.URL+"/comments/"+commentCreated.ID, nil)
	deleteByOwnerReq.Header.Set("Authorization", "Bearer "+ownerToken)
	deleteByOwnerResp, err := client.Do(deleteByOwnerReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, deleteByOwnerResp.StatusCode)
	_ = deleteByOwnerResp.Body.Close()

	// Deleting again returns not found.
	deleteAgainReq, _ := http.NewRequest(http.MethodDelete, server.URL+"/comments/"+commentCreated.ID, nil)
	deleteAgainReq.Header.Set("Authorization", "Bearer "+ownerToken)
	deleteAgainResp, err := client.Do(deleteAgainReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, deleteAgainResp.StatusCode)
	_ = deleteAgainResp.Body.Close()
}
