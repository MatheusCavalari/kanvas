package board

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

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/jwt"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/middleware"
)

type fakeBoardService struct {
	createBoardFn  func(ctx context.Context, ownerID uuid.UUID, name string) (Board, error)
	listBoardsFn   func(ctx context.Context, userID uuid.UUID) ([]Board, error)
	getBoardFn     func(ctx context.Context, boardID, requesterID uuid.UUID) (Board, error)
	renameBoardFn  func(ctx context.Context, boardID, requesterID uuid.UUID, name string) (Board, error)
	deleteBoardFn  func(ctx context.Context, boardID, requesterID uuid.UUID) error
	inviteMemberFn func(ctx context.Context, boardID, requesterID uuid.UUID, email string) (Member, error)
	removeMemberFn func(ctx context.Context, boardID, requesterID, targetUserID uuid.UUID) error
	listMembersFn  func(ctx context.Context, boardID, requesterID uuid.UUID) ([]Member, error)
}

func (f *fakeBoardService) CreateBoard(ctx context.Context, ownerID uuid.UUID, name string) (Board, error) {
	return f.createBoardFn(ctx, ownerID, name)
}
func (f *fakeBoardService) ListBoards(ctx context.Context, userID uuid.UUID) ([]Board, error) {
	return f.listBoardsFn(ctx, userID)
}
func (f *fakeBoardService) GetBoard(ctx context.Context, boardID, requesterID uuid.UUID) (Board, error) {
	return f.getBoardFn(ctx, boardID, requesterID)
}
func (f *fakeBoardService) RenameBoard(ctx context.Context, boardID, requesterID uuid.UUID, name string) (Board, error) {
	return f.renameBoardFn(ctx, boardID, requesterID, name)
}
func (f *fakeBoardService) DeleteBoard(ctx context.Context, boardID, requesterID uuid.UUID) error {
	return f.deleteBoardFn(ctx, boardID, requesterID)
}
func (f *fakeBoardService) InviteMember(ctx context.Context, boardID, requesterID uuid.UUID, email string) (Member, error) {
	return f.inviteMemberFn(ctx, boardID, requesterID, email)
}
func (f *fakeBoardService) RemoveMember(ctx context.Context, boardID, requesterID, targetUserID uuid.UUID) error {
	return f.removeMemberFn(ctx, boardID, requesterID, targetUserID)
}
func (f *fakeBoardService) ListMembers(ctx context.Context, boardID, requesterID uuid.UUID) ([]Member, error) {
	return f.listMembersFn(ctx, boardID, requesterID)
}

// setupTestRouter wires a real JWT issuer and the real auth middleware
// around the board handler, so tests exercise genuine token-based auth
// (issue a real token, send it as a real Authorization header) instead
// of bypassing the middleware — there is no way to inject a fake user ID
// into the request context from outside the middleware package, since
// its context key is unexported by design.
func setupTestRouter(svc boardService) (chi.Router, func(userID uuid.UUID) string) {
	issuer := jwt.NewIssuer("test-secret", time.Hour)
	h := NewHandler(svc)
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

func TestHandler_CreateBoard_Success(t *testing.T) {
	owner := uuid.New()
	boardID := uuid.New()
	svc := &fakeBoardService{
		createBoardFn: func(ctx context.Context, ownerID uuid.UUID, name string) (Board, error) {
			require.Equal(t, owner, ownerID)
			return Board{ID: boardID, Name: name, OwnerID: ownerID, CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil
		},
	}
	r, tokenFor := setupTestRouter(svc)

	body, _ := json.Marshal(createBoardRequest{Name: "Sprint Board"})
	req := httptest.NewRequest(http.MethodPost, "/boards/", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tokenFor(owner))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	var resp boardView
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	require.Equal(t, "Sprint Board", resp.Name)
}

func TestHandler_CreateBoard_RequiresAuth(t *testing.T) {
	svc := &fakeBoardService{}
	r, _ := setupTestRouter(svc)

	body, _ := json.Marshal(createBoardRequest{Name: "Sprint Board"})
	req := httptest.NewRequest(http.MethodPost, "/boards/", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHandler_InviteMember_Forbidden(t *testing.T) {
	requester := uuid.New()
	boardID := uuid.New()
	svc := &fakeBoardService{
		inviteMemberFn: func(ctx context.Context, bID, reqID uuid.UUID, email string) (Member, error) {
			return Member{}, ErrForbidden
		},
	}
	r, tokenFor := setupTestRouter(svc)

	body, _ := json.Marshal(inviteMemberRequest{Email: "friend@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/boards/"+boardID.String()+"/members", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tokenFor(requester))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestHandler_DeleteBoard_InvalidBoardID(t *testing.T) {
	requester := uuid.New()
	svc := &fakeBoardService{}
	r, tokenFor := setupTestRouter(svc)

	req := httptest.NewRequest(http.MethodDelete, "/boards/not-a-uuid", nil)
	req.Header.Set("Authorization", "Bearer "+tokenFor(requester))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}
