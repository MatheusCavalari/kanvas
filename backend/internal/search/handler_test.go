package search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/jwt"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/middleware"
)

type fakeBoardAuthorizer struct {
	ensureMemberFn func(ctx context.Context, boardID, userID uuid.UUID) error
}

func (f *fakeBoardAuthorizer) EnsureMember(ctx context.Context, boardID, userID uuid.UUID) error {
	return f.ensureMemberFn(ctx, boardID, userID)
}

func setupTestRouter(board BoardAuthorizer) (chi.Router, func(userID uuid.UUID) string) {
	issuer := jwt.NewIssuer("test-secret", time.Hour)
	h := NewHandler(nil, board)
	r := chi.NewRouter()
	h.RegisterRoutes(r, middleware.Auth(issuer))
	tokenFor := func(userID uuid.UUID) string {
		token, err := issuer.IssueAccessToken(userID)
		if err != nil {
			panic(err)
		}
		return token
	}
	return r, tokenFor
}

func TestHandler_Search_Unauthorized(t *testing.T) {
	boardID := uuid.New()
	board := &fakeBoardAuthorizer{
		ensureMemberFn: func(ctx context.Context, bID, userID uuid.UUID) error {
			t.Fatal("EnsureMember should not be called without auth")
			return nil
		},
	}
	r, _ := setupTestRouter(board)

	req := httptest.NewRequest(http.MethodGet, "/boards/"+boardID.String()+"/search?q=hello", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHandler_Search_InvalidBoardID(t *testing.T) {
	board := &fakeBoardAuthorizer{
		ensureMemberFn: func(ctx context.Context, bID, userID uuid.UUID) error {
			t.Fatal("EnsureMember should not be called for an invalid board id")
			return nil
		},
	}
	r, tokenFor := setupTestRouter(board)
	requester := uuid.New()

	req := httptest.NewRequest(http.MethodGet, "/boards/not-a-uuid/search?q=hello", nil)
	req.Header.Set("Authorization", "Bearer "+tokenFor(requester))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_Search_Forbidden(t *testing.T) {
	boardID := uuid.New()
	requester := uuid.New()
	board := &fakeBoardAuthorizer{
		ensureMemberFn: func(ctx context.Context, bID, userID uuid.UUID) error {
			require.Equal(t, boardID, bID)
			require.Equal(t, requester, userID)
			return context.DeadlineExceeded
		},
	}
	r, tokenFor := setupTestRouter(board)

	req := httptest.NewRequest(http.MethodGet, "/boards/"+boardID.String()+"/search?q=hello", nil)
	req.Header.Set("Authorization", "Bearer "+tokenFor(requester))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestHandler_Search_QueryTooShort(t *testing.T) {
	boardID := uuid.New()
	requester := uuid.New()
	board := &fakeBoardAuthorizer{
		ensureMemberFn: func(ctx context.Context, bID, userID uuid.UUID) error {
			return nil
		},
	}
	r, tokenFor := setupTestRouter(board)

	req := httptest.NewRequest(http.MethodGet, "/boards/"+boardID.String()+"/search?q=a", nil)
	req.Header.Set("Authorization", "Bearer "+tokenFor(requester))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)

	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	require.Equal(t, "invalid_request", body.Error.Code)
}

func TestHandler_Search_MissingQuery(t *testing.T) {
	boardID := uuid.New()
	requester := uuid.New()
	board := &fakeBoardAuthorizer{
		ensureMemberFn: func(ctx context.Context, bID, userID uuid.UUID) error {
			return nil
		},
	}
	r, tokenFor := setupTestRouter(board)

	req := httptest.NewRequest(http.MethodGet, "/boards/"+boardID.String()+"/search", nil)
	req.Header.Set("Authorization", "Bearer "+tokenFor(requester))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}
