//go:build integration

package auth_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MatheusCavalari/kanvas/backend/internal/auth"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/dbtest"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/gen"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/httpserver"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/jwt"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/middleware"
)

func TestAuthFlow_EndToEnd(t *testing.T) {
	pool := dbtest.NewPool(t)
	repo := auth.NewPostgresRepository(gen.New(pool))
	issuer := jwt.NewIssuer("test-secret", 15*time.Minute)
	service := auth.NewService(repo, issuer, 7*24*time.Hour)
	handler := auth.NewHandler(service, false)

	router := httpserver.NewRouter("http://localhost:5173")
	handler.RegisterRoutes(router, middleware.Auth(issuer))

	server := httptest.NewServer(router)
	defer server.Close()
	client := server.Client()

	registerBody, _ := json.Marshal(map[string]string{
		"name": "Ada Lovelace", "email": "ada@example.com", "password": "supersecret",
	})
	resp, err := client.Post(server.URL+"/auth/register", "application/json", bytes.NewReader(registerBody))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var registerResp struct {
		AccessToken string `json:"access_token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&registerResp))
	require.NotEmpty(t, registerResp.AccessToken)

	var refreshCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "refresh_token" {
			refreshCookie = c
		}
	}
	require.NotNil(t, refreshCookie)

	meReq, _ := http.NewRequest(http.MethodGet, server.URL+"/auth/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+registerResp.AccessToken)
	meResp, err := client.Do(meReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, meResp.StatusCode)

	refreshReq, _ := http.NewRequest(http.MethodPost, server.URL+"/auth/refresh", nil)
	refreshReq.AddCookie(refreshCookie)
	refreshResp, err := client.Do(refreshReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, refreshResp.StatusCode)

	logoutReq, _ := http.NewRequest(http.MethodPost, server.URL+"/auth/logout", nil)
	logoutReq.AddCookie(refreshCookie)
	logoutResp, err := client.Do(logoutReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, logoutResp.StatusCode)
}
