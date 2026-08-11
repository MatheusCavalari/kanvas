package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/jwt"
)

func TestAuth_ValidToken(t *testing.T) {
	issuer := jwt.NewIssuer("test-secret", time.Hour)
	userID := uuid.New()
	token, err := issuer.IssueAccessToken(userID)
	require.NoError(t, err)

	var gotID uuid.UUID
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID, _ = UserIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	Auth(issuer)(next).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, userID, gotID)
}

func TestAuth_MissingHeader(t *testing.T) {
	issuer := jwt.NewIssuer("test-secret", time.Hour)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	Auth(issuer)(next).ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuth_InvalidToken(t *testing.T) {
	issuer := jwt.NewIssuer("test-secret", time.Hour)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	rec := httptest.NewRecorder()

	Auth(issuer)(next).ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}
