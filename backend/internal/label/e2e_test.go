//go:build integration

package label_test

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
	"github.com/MatheusCavalari/kanvas/backend/internal/label"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/dbtest"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/gen"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/httpserver"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/jwt"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/middleware"
	"github.com/MatheusCavalari/kanvas/backend/internal/realtime"
)

func TestLabelFlow_EndToEnd(t *testing.T) {
	pool := dbtest.NewPool(t)
	queries := gen.New(pool)
	ctx := context.Background()

	owner, err := queries.CreateUser(ctx, gen.CreateUserParams{ID: uuid.New(), Name: "Owner", Email: "label-owner@example.com", PasswordHash: "hashed"})
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

	labelRepo := label.NewPostgresRepository(queries)
	labelService := label.NewService(labelRepo, boardService, labelRepo, hub)
	labelHandler := label.NewHandler(labelService)

	router := httpserver.NewRouter("http://localhost:5173")
	authMiddleware := middleware.Auth(issuer)
	boardHandler.RegisterRoutes(router, authMiddleware)
	cardHandler.RegisterRoutes(router, authMiddleware)
	labelHandler.RegisterRoutes(router, authMiddleware)

	server := httptest.NewServer(router)
	defer server.Close()
	client := server.Client()

	ownerToken, err := issuer.IssueAccessToken(owner.ID)
	require.NoError(t, err)

	// Create a board.
	createBoardBody, _ := json.Marshal(map[string]string{"name": "Label Board"})
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

	// Create a column and card to attach labels to.
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
		"title":     "Fix the bug",
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

	// Create a label.
	createLabelBody, _ := json.Marshal(map[string]string{"name": "Bug", "color": "#FF0000"})
	createLabelReq, _ := http.NewRequest(http.MethodPost, server.URL+"/boards/"+boardCreated.ID+"/labels/", bytes.NewReader(createLabelBody))
	createLabelReq.Header.Set("Authorization", "Bearer "+ownerToken)
	createLabelResp, err := client.Do(createLabelReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, createLabelResp.StatusCode)
	var labelCreated struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	require.NoError(t, json.NewDecoder(createLabelResp.Body).Decode(&labelCreated))
	_ = createLabelResp.Body.Close()
	require.Equal(t, "Bug", labelCreated.Name)
	require.Equal(t, "#FF0000", labelCreated.Color)

	// Duplicate name on the same board should be rejected.
	dupLabelReq, _ := http.NewRequest(http.MethodPost, server.URL+"/boards/"+boardCreated.ID+"/labels/", bytes.NewReader(createLabelBody))
	dupLabelReq.Header.Set("Authorization", "Bearer "+ownerToken)
	dupLabelResp, err := client.Do(dupLabelReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, dupLabelResp.StatusCode)
	_ = dupLabelResp.Body.Close()

	// Invalid color should be rejected.
	invalidColorBody, _ := json.Marshal(map[string]string{"name": "Other", "color": "red"})
	invalidColorReq, _ := http.NewRequest(http.MethodPost, server.URL+"/boards/"+boardCreated.ID+"/labels/", bytes.NewReader(invalidColorBody))
	invalidColorReq.Header.Set("Authorization", "Bearer "+ownerToken)
	invalidColorResp, err := client.Do(invalidColorReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, invalidColorResp.StatusCode)
	_ = invalidColorResp.Body.Close()

	// A stranger cannot create, list or attach labels on this board.
	stranger, err := queries.CreateUser(ctx, gen.CreateUserParams{ID: uuid.New(), Name: "Stranger", Email: "label-stranger@example.com", PasswordHash: "hashed"})
	require.NoError(t, err)
	strangerToken, err := issuer.IssueAccessToken(stranger.ID)
	require.NoError(t, err)

	strangerListReq, _ := http.NewRequest(http.MethodGet, server.URL+"/boards/"+boardCreated.ID+"/labels/", nil)
	strangerListReq.Header.Set("Authorization", "Bearer "+strangerToken)
	strangerListResp, err := client.Do(strangerListReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, strangerListResp.StatusCode)
	_ = strangerListResp.Body.Close()

	// List labels for the board.
	listLabelsReq, _ := http.NewRequest(http.MethodGet, server.URL+"/boards/"+boardCreated.ID+"/labels/", nil)
	listLabelsReq.Header.Set("Authorization", "Bearer "+ownerToken)
	listLabelsResp, err := client.Do(listLabelsReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, listLabelsResp.StatusCode)
	var labelsList []struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(listLabelsResp.Body).Decode(&labelsList))
	_ = listLabelsResp.Body.Close()
	require.Len(t, labelsList, 1)

	// Update the label.
	updateLabelBody, _ := json.Marshal(map[string]string{"name": "Critical Bug", "color": "#AA0000"})
	updateLabelReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/boards/"+boardCreated.ID+"/labels/"+labelCreated.ID, bytes.NewReader(updateLabelBody))
	updateLabelReq.Header.Set("Authorization", "Bearer "+ownerToken)
	updateLabelResp, err := client.Do(updateLabelReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, updateLabelResp.StatusCode)
	var labelUpdated struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	require.NoError(t, json.NewDecoder(updateLabelResp.Body).Decode(&labelUpdated))
	_ = updateLabelResp.Body.Close()
	require.Equal(t, "Critical Bug", labelUpdated.Name)
	require.Equal(t, "#AA0000", labelUpdated.Color)

	// Attach the label to the card.
	attachBody, _ := json.Marshal(map[string]string{"label_id": labelCreated.ID})
	attachReq, _ := http.NewRequest(http.MethodPost, server.URL+"/cards/"+cardCreated.ID+"/labels/", bytes.NewReader(attachBody))
	attachReq.Header.Set("Authorization", "Bearer "+ownerToken)
	attachResp, err := client.Do(attachReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, attachResp.StatusCode)
	_ = attachResp.Body.Close()

	// List card labels.
	cardLabelsReq, _ := http.NewRequest(http.MethodGet, server.URL+"/cards/"+cardCreated.ID+"/labels/", nil)
	cardLabelsReq.Header.Set("Authorization", "Bearer "+ownerToken)
	cardLabelsResp, err := client.Do(cardLabelsReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, cardLabelsResp.StatusCode)
	var cardLabels []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	require.NoError(t, json.NewDecoder(cardLabelsResp.Body).Decode(&cardLabels))
	_ = cardLabelsResp.Body.Close()
	require.Len(t, cardLabels, 1)
	require.Equal(t, labelCreated.ID, cardLabels[0].ID)

	// Detach the label from the card.
	detachReq, _ := http.NewRequest(http.MethodDelete, server.URL+"/cards/"+cardCreated.ID+"/labels/"+labelCreated.ID, nil)
	detachReq.Header.Set("Authorization", "Bearer "+ownerToken)
	detachResp, err := client.Do(detachReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, detachResp.StatusCode)
	_ = detachResp.Body.Close()

	cardLabelsAfterDetachReq, _ := http.NewRequest(http.MethodGet, server.URL+"/cards/"+cardCreated.ID+"/labels/", nil)
	cardLabelsAfterDetachReq.Header.Set("Authorization", "Bearer "+ownerToken)
	cardLabelsAfterDetachResp, err := client.Do(cardLabelsAfterDetachReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, cardLabelsAfterDetachResp.StatusCode)
	var cardLabelsAfterDetach []struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(cardLabelsAfterDetachResp.Body).Decode(&cardLabelsAfterDetach))
	_ = cardLabelsAfterDetachResp.Body.Close()
	require.Empty(t, cardLabelsAfterDetach)

	// Delete the label.
	deleteLabelReq, _ := http.NewRequest(http.MethodDelete, server.URL+"/boards/"+boardCreated.ID+"/labels/"+labelCreated.ID, nil)
	deleteLabelReq.Header.Set("Authorization", "Bearer "+ownerToken)
	deleteLabelResp, err := client.Do(deleteLabelReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, deleteLabelResp.StatusCode)
	_ = deleteLabelResp.Body.Close()

	listAfterDeleteReq, _ := http.NewRequest(http.MethodGet, server.URL+"/boards/"+boardCreated.ID+"/labels/", nil)
	listAfterDeleteReq.Header.Set("Authorization", "Bearer "+ownerToken)
	listAfterDeleteResp, err := client.Do(listAfterDeleteReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, listAfterDeleteResp.StatusCode)
	var labelsAfterDelete []struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(listAfterDeleteResp.Body).Decode(&labelsAfterDelete))
	_ = listAfterDeleteResp.Body.Close()
	require.Empty(t, labelsAfterDelete)
}
