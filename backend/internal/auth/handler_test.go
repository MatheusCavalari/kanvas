package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type fakeAuthService struct {
	registerFn func(ctx context.Context, name, email, password string) (AuthResult, error)
	loginFn    func(ctx context.Context, email, password string) (AuthResult, error)
	refreshFn  func(ctx context.Context, token string) (AuthResult, error)
	logoutFn   func(ctx context.Context, token string) error
	userByIDFn func(ctx context.Context, id uuid.UUID) (User, error)
}

func (f *fakeAuthService) Register(ctx context.Context, name, email, password string) (AuthResult, error) {
	return f.registerFn(ctx, name, email, password)
}
func (f *fakeAuthService) Login(ctx context.Context, email, password string) (AuthResult, error) {
	return f.loginFn(ctx, email, password)
}
func (f *fakeAuthService) Refresh(ctx context.Context, token string) (AuthResult, error) {
	return f.refreshFn(ctx, token)
}
func (f *fakeAuthService) Logout(ctx context.Context, token string) error {
	return f.logoutFn(ctx, token)
}
func (f *fakeAuthService) UserByID(ctx context.Context, id uuid.UUID) (User, error) {
	return f.userByIDFn(ctx, id)
}

func passthroughMiddleware(next http.Handler) http.Handler { return next }

func TestHandler_Register_Success(t *testing.T) {
	svc := &fakeAuthService{
		registerFn: func(ctx context.Context, name, email, password string) (AuthResult, error) {
			return AuthResult{
				User:             User{ID: uuid.New(), Name: name, Email: email},
				AccessToken:      "access-token",
				RefreshToken:     "refresh-token",
				RefreshExpiresAt: time.Now().Add(time.Hour),
			}, nil
		},
	}
	h := NewHandler(svc, false)
	r := chi.NewRouter()
	h.RegisterRoutes(r, passthroughMiddleware)

	body, _ := json.Marshal(registerRequest{Name: "Ada", Email: "ada@example.com", Password: "supersecret"})
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)

	var resp authResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	require.Equal(t, "access-token", resp.AccessToken)
	require.Equal(t, "ada@example.com", resp.User.Email)

	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1)
	require.Equal(t, "refresh_token", cookies[0].Name)
	require.True(t, cookies[0].HttpOnly)
}

func TestHandler_Register_DuplicateEmail(t *testing.T) {
	svc := &fakeAuthService{
		registerFn: func(ctx context.Context, name, email, password string) (AuthResult, error) {
			return AuthResult{}, ErrEmailTaken
		},
	}
	h := NewHandler(svc, false)
	r := chi.NewRouter()
	h.RegisterRoutes(r, passthroughMiddleware)

	body, _ := json.Marshal(registerRequest{Name: "Ada", Email: "ada@example.com", Password: "supersecret"})
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
}

func TestHandler_Me_RequiresAuthMiddleware(t *testing.T) {
	svc := &fakeAuthService{}
	h := NewHandler(svc, false)
	r := chi.NewRouter()
	blockAll := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		})
	}
	h.RegisterRoutes(r, blockAll)

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}
