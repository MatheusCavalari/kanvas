//go:build integration

package search_test

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
	"github.com/MatheusCavalari/kanvas/backend/internal/search"
)

func TestSearchFlow_EndToEnd(t *testing.T) {
	pool := dbtest.NewPool(t)
	queries := gen.New(pool)
	ctx := context.Background()

	owner, err := queries.CreateUser(ctx, gen.CreateUserParams{ID: uuid.New(), Name: "Owner", Email: "search-owner@example.com", PasswordHash: "hashed"})
	require.NoError(t, err)

	issuer := jwt.NewIssuer("test-secret", time.Hour)

	boardRepo := board.NewPostgresRepository(queries)
	userLookup := board.NewUserLookupAdapter(queries)
	boardService := board.NewService(boardRepo, userLookup)
	boardHandler := board.NewHandler(boardService)

	hub := realtime.NewHub()

	cardRepo := card.NewPostgresRepository(queries)
	cardService := card.NewService(cardRepo, boardService, hub)
	cardHandler := card.NewHandler(cardService)

	commentRepo := comment.NewPostgresRepository(queries)
	commentService := comment.NewService(commentRepo, commentRepo, boardService, boardService, hub)
	commentHandler := comment.NewHandler(commentService)

	searchHandler := search.NewHandler(queries, boardService)

	router := httpserver.NewRouter("http://localhost:5173")
	authMiddleware := middleware.Auth(issuer)
	boardHandler.RegisterRoutes(router, authMiddleware)
	cardHandler.RegisterRoutes(router, authMiddleware)
	commentHandler.RegisterRoutes(router, authMiddleware)
	searchHandler.RegisterRoutes(router, authMiddleware)

	server := httptest.NewServer(router)
	defer server.Close()
	client := server.Client()

	ownerToken, err := issuer.IssueAccessToken(owner.ID)
	require.NoError(t, err)

	// Create a board, column, and a couple of cards to search over.
	createBoardBody, _ := json.Marshal(map[string]string{"name": "Search Board"})
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
		"column_id":   columnCreated.ID,
		"title":       "Migrate database to PostgreSQL",
		"description": "We need a full-text search index on cards and comments",
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

	unrelatedCardBody, _ := json.Marshal(map[string]interface{}{
		"column_id": columnCreated.ID,
		"title":     "Buy more coffee",
	})
	unrelatedCardReq, _ := http.NewRequest(http.MethodPost, server.URL+"/cards/", bytes.NewReader(unrelatedCardBody))
	unrelatedCardReq.Header.Set("Authorization", "Bearer "+ownerToken)
	unrelatedCardResp, err := client.Do(unrelatedCardReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, unrelatedCardResp.StatusCode)
	_ = unrelatedCardResp.Body.Close()

	// Add a comment on the first card that also matches the search term.
	createCommentBody, _ := json.Marshal(map[string]string{"body": "The PostgreSQL tsvector column is now generated automatically."})
	createCommentReq, _ := http.NewRequest(http.MethodPost, server.URL+"/cards/"+cardCreated.ID+"/comments/", bytes.NewReader(createCommentBody))
	createCommentReq.Header.Set("Authorization", "Bearer "+ownerToken)
	createCommentResp, err := client.Do(createCommentReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, createCommentResp.StatusCode)
	_ = createCommentResp.Body.Close()

	// A stranger (not a board member) cannot search the board.
	stranger, err := queries.CreateUser(ctx, gen.CreateUserParams{ID: uuid.New(), Name: "Stranger", Email: "search-stranger@example.com", PasswordHash: "hashed"})
	require.NoError(t, err)
	strangerToken, err := issuer.IssueAccessToken(stranger.ID)
	require.NoError(t, err)

	strangerSearchReq, _ := http.NewRequest(http.MethodGet, server.URL+"/boards/"+boardCreated.ID+"/search?q=postgresql", nil)
	strangerSearchReq.Header.Set("Authorization", "Bearer "+strangerToken)
	strangerSearchResp, err := client.Do(strangerSearchReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, strangerSearchResp.StatusCode)
	_ = strangerSearchResp.Body.Close()

	type searchResponse struct {
		Cards []struct {
			ID       string  `json:"id"`
			Title    string  `json:"title"`
			ColumnID string  `json:"column_id"`
			Rank     float64 `json:"rank"`
		} `json:"cards"`
		Comments []struct {
			ID          string  `json:"id"`
			CardID      string  `json:"card_id"`
			BodyExcerpt string  `json:"body_excerpt"`
			Rank        float64 `json:"rank"`
		} `json:"comments"`
	}

	// Owner searches for "postgres" across both cards and comments.
	searchReq, _ := http.NewRequest(http.MethodGet, server.URL+"/boards/"+boardCreated.ID+"/search?q=postgresql", nil)
	searchReq.Header.Set("Authorization", "Bearer "+ownerToken)
	searchResp, err := client.Do(searchReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, searchResp.StatusCode)
	var results searchResponse
	require.NoError(t, json.NewDecoder(searchResp.Body).Decode(&results))
	_ = searchResp.Body.Close()

	require.Len(t, results.Cards, 1)
	require.Equal(t, cardCreated.ID, results.Cards[0].ID)
	require.Equal(t, "Migrate database to PostgreSQL", results.Cards[0].Title)

	require.Len(t, results.Comments, 1)
	require.Equal(t, cardCreated.ID, results.Comments[0].CardID)
	require.NotEmpty(t, results.Comments[0].BodyExcerpt)

	// type=cards restricts results to cards only.
	cardsOnlyReq, _ := http.NewRequest(http.MethodGet, server.URL+"/boards/"+boardCreated.ID+"/search?q=postgresql&type=cards", nil)
	cardsOnlyReq.Header.Set("Authorization", "Bearer "+ownerToken)
	cardsOnlyResp, err := client.Do(cardsOnlyReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, cardsOnlyResp.StatusCode)
	var cardsOnly searchResponse
	require.NoError(t, json.NewDecoder(cardsOnlyResp.Body).Decode(&cardsOnly))
	_ = cardsOnlyResp.Body.Close()
	require.Len(t, cardsOnly.Cards, 1)
	require.Empty(t, cardsOnly.Comments)

	// type=comments restricts results to comments only.
	commentsOnlyReq, _ := http.NewRequest(http.MethodGet, server.URL+"/boards/"+boardCreated.ID+"/search?q=postgresql&type=comments", nil)
	commentsOnlyReq.Header.Set("Authorization", "Bearer "+ownerToken)
	commentsOnlyResp, err := client.Do(commentsOnlyReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, commentsOnlyResp.StatusCode)
	var commentsOnly searchResponse
	require.NoError(t, json.NewDecoder(commentsOnlyResp.Body).Decode(&commentsOnly))
	_ = commentsOnlyResp.Body.Close()
	require.Empty(t, commentsOnly.Cards)
	require.Len(t, commentsOnly.Comments, 1)

	// A query that matches nothing returns empty results.
	noMatchReq, _ := http.NewRequest(http.MethodGet, server.URL+"/boards/"+boardCreated.ID+"/search?q=nonexistentterm", nil)
	noMatchReq.Header.Set("Authorization", "Bearer "+ownerToken)
	noMatchResp, err := client.Do(noMatchReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, noMatchResp.StatusCode)
	var noMatch searchResponse
	require.NoError(t, json.NewDecoder(noMatchResp.Body).Decode(&noMatch))
	_ = noMatchResp.Body.Close()
	require.Empty(t, noMatch.Cards)
	require.Empty(t, noMatch.Comments)

	// A query shorter than 2 characters is rejected.
	shortQueryReq, _ := http.NewRequest(http.MethodGet, server.URL+"/boards/"+boardCreated.ID+"/search?q=a", nil)
	shortQueryReq.Header.Set("Authorization", "Bearer "+ownerToken)
	shortQueryResp, err := client.Do(shortQueryReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, shortQueryResp.StatusCode)
	_ = shortQueryResp.Body.Close()
}
