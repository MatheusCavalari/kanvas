package card

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

	"github.com/MatheusCavalari/kanvas/backend/internal/board"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/jwt"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/middleware"
)

type fakeCardService struct {
	createColumnFn     func(ctx context.Context, boardID, requesterID uuid.UUID, title string) (Column, error)
	renameColumnFn     func(ctx context.Context, columnID, requesterID uuid.UUID, title string) (Column, error)
	deleteColumnFn     func(ctx context.Context, columnID, requesterID uuid.UUID) error
	reorderColumnsFn   func(ctx context.Context, boardID, requesterID uuid.UUID, orderedColumnIDs []uuid.UUID) error
	listBoardColumnsFn func(ctx context.Context, boardID, requesterID uuid.UUID) ([]ColumnWithCards, error)
	createCardFn       func(ctx context.Context, columnID, requesterID uuid.UUID, title, description string, assigneeID *uuid.UUID, dueDate *time.Time) (Card, error)
	updateCardFn       func(ctx context.Context, cardID, requesterID uuid.UUID, title, description string, assigneeID *uuid.UUID, dueDate *time.Time) (Card, error)
	deleteCardFn       func(ctx context.Context, cardID, requesterID uuid.UUID) error
	moveCardFn         func(ctx context.Context, cardID, requesterID, targetColumnID uuid.UUID, targetPosition int) (Card, error)
}

func (f *fakeCardService) CreateColumn(ctx context.Context, boardID, requesterID uuid.UUID, title string) (Column, error) {
	return f.createColumnFn(ctx, boardID, requesterID, title)
}
func (f *fakeCardService) RenameColumn(ctx context.Context, columnID, requesterID uuid.UUID, title string) (Column, error) {
	return f.renameColumnFn(ctx, columnID, requesterID, title)
}
func (f *fakeCardService) DeleteColumn(ctx context.Context, columnID, requesterID uuid.UUID) error {
	return f.deleteColumnFn(ctx, columnID, requesterID)
}
func (f *fakeCardService) ReorderColumns(ctx context.Context, boardID, requesterID uuid.UUID, orderedColumnIDs []uuid.UUID) error {
	return f.reorderColumnsFn(ctx, boardID, requesterID, orderedColumnIDs)
}
func (f *fakeCardService) ListBoardColumns(ctx context.Context, boardID, requesterID uuid.UUID) ([]ColumnWithCards, error) {
	return f.listBoardColumnsFn(ctx, boardID, requesterID)
}
func (f *fakeCardService) CreateCard(ctx context.Context, columnID, requesterID uuid.UUID, title, description string, assigneeID *uuid.UUID, dueDate *time.Time) (Card, error) {
	return f.createCardFn(ctx, columnID, requesterID, title, description, assigneeID, dueDate)
}
func (f *fakeCardService) UpdateCard(ctx context.Context, cardID, requesterID uuid.UUID, title, description string, assigneeID *uuid.UUID, dueDate *time.Time) (Card, error) {
	return f.updateCardFn(ctx, cardID, requesterID, title, description, assigneeID, dueDate)
}
func (f *fakeCardService) DeleteCard(ctx context.Context, cardID, requesterID uuid.UUID) error {
	return f.deleteCardFn(ctx, cardID, requesterID)
}
func (f *fakeCardService) MoveCard(ctx context.Context, cardID, requesterID, targetColumnID uuid.UUID, targetPosition int) (Card, error) {
	return f.moveCardFn(ctx, cardID, requesterID, targetColumnID, targetPosition)
}

func setupTestRouter(svc cardService) (chi.Router, func(userID uuid.UUID) string) {
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

func TestHandler_CreateColumn_Success(t *testing.T) {
	boardID := uuid.New()
	requester := uuid.New()
	svc := &fakeCardService{
		createColumnFn: func(ctx context.Context, bID, reqID uuid.UUID, title string) (Column, error) {
			require.Equal(t, boardID, bID)
			return Column{ID: uuid.New(), BoardID: bID, Title: title, CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil
		},
	}
	r, tokenFor := setupTestRouter(svc)

	body, _ := json.Marshal(createColumnRequest{Title: "To Do"})
	req := httptest.NewRequest(http.MethodPost, "/boards/"+boardID.String()+"/columns/", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tokenFor(requester))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	var resp columnView
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	require.Equal(t, "To Do", resp.Title)
}

func TestHandler_CreateColumn_RequiresAuth(t *testing.T) {
	svc := &fakeCardService{}
	r, _ := setupTestRouter(svc)

	body, _ := json.Marshal(createColumnRequest{Title: "To Do"})
	req := httptest.NewRequest(http.MethodPost, "/boards/"+uuid.New().String()+"/columns/", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHandler_CreateCard_Forbidden(t *testing.T) {
	requester := uuid.New()
	svc := &fakeCardService{
		createCardFn: func(ctx context.Context, columnID, reqID uuid.UUID, title, description string, assigneeID *uuid.UUID, dueDate *time.Time) (Card, error) {
			return Card{}, board.ErrNotAMember
		},
	}
	r, tokenFor := setupTestRouter(svc)

	body, _ := json.Marshal(createCardRequest{Title: "Write plan"})
	req := httptest.NewRequest(http.MethodPost, "/cards/", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tokenFor(requester))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestHandler_MoveCard_InvalidCardID(t *testing.T) {
	requester := uuid.New()
	svc := &fakeCardService{}
	r, tokenFor := setupTestRouter(svc)

	body, _ := json.Marshal(moveCardRequest{ColumnID: uuid.New(), Position: 0})
	req := httptest.NewRequest(http.MethodPatch, "/cards/not-a-uuid/move", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tokenFor(requester))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}
