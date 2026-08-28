//go:build integration

package webhook_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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
	"github.com/MatheusCavalari/kanvas/backend/internal/webhook"
)

func TestWebhookFlow_EndToEnd(t *testing.T) {
	pool := dbtest.NewPool(t)
	queries := gen.New(pool)
	ctx := context.Background()

	owner, err := queries.CreateUser(ctx, gen.CreateUserParams{ID: uuid.New(), Name: "Owner", Email: "webhook-owner@example.com", PasswordHash: "hashed"})
	require.NoError(t, err)

	issuer := jwt.NewIssuer("test-secret", time.Hour)

	boardRepo := board.NewPostgresRepository(queries)
	userLookup := board.NewUserLookupAdapter(queries)
	boardService := board.NewService(boardRepo, userLookup)
	boardHandler := board.NewHandler(boardService)

	webhookRepo := webhook.NewPostgresRepository(queries)
	webhookService := webhook.NewService(webhookRepo, boardService)
	webhookHandler := webhook.NewHandler(webhookService)

	router := httpserver.NewRouter("http://localhost:5173")
	authMiddleware := middleware.Auth(issuer)
	boardHandler.RegisterRoutes(router, authMiddleware)
	webhookHandler.RegisterRoutes(router, authMiddleware)

	server := httptest.NewServer(router)
	defer server.Close()
	client := server.Client()

	ownerToken, err := issuer.IssueAccessToken(owner.ID)
	require.NoError(t, err)

	createBoardBody, _ := json.Marshal(map[string]string{"name": "Webhook Board"})
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

	// A regular member cannot manage webhooks.
	member, err := queries.CreateUser(ctx, gen.CreateUserParams{ID: uuid.New(), Name: "Member", Email: "webhook-member@example.com", PasswordHash: "hashed"})
	require.NoError(t, err)
	memberToken, err := issuer.IssueAccessToken(member.ID)
	require.NoError(t, err)

	inviteBody, _ := json.Marshal(map[string]string{"email": "webhook-member@example.com"})
	inviteReq, _ := http.NewRequest(http.MethodPost, server.URL+"/boards/"+boardCreated.ID+"/members", bytes.NewReader(inviteBody))
	inviteReq.Header.Set("Authorization", "Bearer "+ownerToken)
	inviteResp, err := client.Do(inviteReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, inviteResp.StatusCode)
	_ = inviteResp.Body.Close()

	memberCreateBody, _ := json.Marshal(map[string]interface{}{"url": "https://example.com/hook", "events": []string{"card.created"}})
	memberCreateReq, _ := http.NewRequest(http.MethodPost, server.URL+"/boards/"+boardCreated.ID+"/webhooks/", bytes.NewReader(memberCreateBody))
	memberCreateReq.Header.Set("Authorization", "Bearer "+memberToken)
	memberCreateResp, err := client.Do(memberCreateReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, memberCreateResp.StatusCode)
	_ = memberCreateResp.Body.Close()

	// Owner creates a webhook and gets the secret back exactly once.
	createBody, _ := json.Marshal(map[string]interface{}{"url": "https://example.com/hook", "events": []string{"card.created", "card.updated"}})
	createReq, _ := http.NewRequest(http.MethodPost, server.URL+"/boards/"+boardCreated.ID+"/webhooks/", bytes.NewReader(createBody))
	createReq.Header.Set("Authorization", "Bearer "+ownerToken)
	createResp, err := client.Do(createReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)
	var webhookCreated struct {
		ID     string   `json:"id"`
		URL    string   `json:"url"`
		Events []string `json:"events"`
		Active bool     `json:"active"`
		Secret string   `json:"secret"`
	}
	require.NoError(t, json.NewDecoder(createResp.Body).Decode(&webhookCreated))
	_ = createResp.Body.Close()
	require.Equal(t, "https://example.com/hook", webhookCreated.URL)
	require.ElementsMatch(t, []string{"card.created", "card.updated"}, webhookCreated.Events)
	require.True(t, webhookCreated.Active)
	require.Len(t, webhookCreated.Secret, 64)

	// Listing webhooks does not include the secret.
	listReq, _ := http.NewRequest(http.MethodGet, server.URL+"/boards/"+boardCreated.ID+"/webhooks/", nil)
	listReq.Header.Set("Authorization", "Bearer "+ownerToken)
	listResp, err := client.Do(listReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, listResp.StatusCode)
	rawList, err := io.ReadAll(listResp.Body)
	require.NoError(t, err)
	listBody := string(rawList)
	_ = listResp.Body.Close()
	require.NotContains(t, listBody, webhookCreated.Secret)
	require.Contains(t, listBody, webhookCreated.ID)

	// Member cannot list webhooks either.
	memberListReq, _ := http.NewRequest(http.MethodGet, server.URL+"/boards/"+boardCreated.ID+"/webhooks/", nil)
	memberListReq.Header.Set("Authorization", "Bearer "+memberToken)
	memberListResp, err := client.Do(memberListReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, memberListResp.StatusCode)
	_ = memberListResp.Body.Close()

	// Owner updates the webhook.
	updateBody, _ := json.Marshal(map[string]interface{}{"url": "https://example.com/hook2", "events": []string{"card.deleted"}, "active": false})
	updateReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/boards/"+boardCreated.ID+"/webhooks/"+webhookCreated.ID, bytes.NewReader(updateBody))
	updateReq.Header.Set("Authorization", "Bearer "+ownerToken)
	updateResp, err := client.Do(updateReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, updateResp.StatusCode)
	var webhookUpdated struct {
		URL    string   `json:"url"`
		Events []string `json:"events"`
		Active bool     `json:"active"`
	}
	require.NoError(t, json.NewDecoder(updateResp.Body).Decode(&webhookUpdated))
	_ = updateResp.Body.Close()
	require.Equal(t, "https://example.com/hook2", webhookUpdated.URL)
	require.Equal(t, []string{"card.deleted"}, webhookUpdated.Events)
	require.False(t, webhookUpdated.Active)

	// Deliveries list starts empty (no jobs were dispatched by this test).
	deliveriesReq, _ := http.NewRequest(http.MethodGet, server.URL+"/boards/"+boardCreated.ID+"/webhooks/"+webhookCreated.ID+"/deliveries", nil)
	deliveriesReq.Header.Set("Authorization", "Bearer "+ownerToken)
	deliveriesResp, err := client.Do(deliveriesReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, deliveriesResp.StatusCode)
	var deliveriesBody struct {
		Deliveries []interface{} `json:"deliveries"`
	}
	require.NoError(t, json.NewDecoder(deliveriesResp.Body).Decode(&deliveriesBody))
	_ = deliveriesResp.Body.Close()
	require.Empty(t, deliveriesBody.Deliveries)

	// Owner deletes the webhook.
	deleteReq, _ := http.NewRequest(http.MethodDelete, server.URL+"/boards/"+boardCreated.ID+"/webhooks/"+webhookCreated.ID, nil)
	deleteReq.Header.Set("Authorization", "Bearer "+ownerToken)
	deleteResp, err := client.Do(deleteReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, deleteResp.StatusCode)
	_ = deleteResp.Body.Close()

	// Deleting again returns not found.
	deleteAgainReq, _ := http.NewRequest(http.MethodDelete, server.URL+"/boards/"+boardCreated.ID+"/webhooks/"+webhookCreated.ID, nil)
	deleteAgainReq.Header.Set("Authorization", "Bearer "+ownerToken)
	deleteAgainResp, err := client.Do(deleteAgainReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, deleteAgainResp.StatusCode)
	_ = deleteAgainResp.Body.Close()
}
