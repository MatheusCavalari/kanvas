package realtime

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type fakeTokenParser struct {
	userID uuid.UUID
	err    error
}

func (f *fakeTokenParser) ParseAccessToken(token string) (uuid.UUID, error) {
	if f.err != nil {
		return uuid.UUID{}, f.err
	}
	return f.userID, nil
}

type fakeWSBoardAuthorizer struct {
	allow bool
}

func (f *fakeWSBoardAuthorizer) EnsureMember(ctx context.Context, boardID, userID uuid.UUID) error {
	if f.allow {
		return nil
	}
	return errors.New("forbidden")
}

func TestHandler_ServeWS_MissingToken(t *testing.T) {
	hub := NewHub()
	h := NewHandler(hub, &fakeTokenParser{userID: uuid.New()}, &fakeWSBoardAuthorizer{allow: true}, "http://localhost:5173")
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	server := httptest.NewServer(r)
	defer server.Close()

	resp, err := http.Get(server.URL + "/boards/" + uuid.New().String() + "/ws")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestHandler_ServeWS_NotAMember(t *testing.T) {
	hub := NewHub()
	h := NewHandler(hub, &fakeTokenParser{userID: uuid.New()}, &fakeWSBoardAuthorizer{allow: false}, "http://localhost:5173")
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	server := httptest.NewServer(r)
	defer server.Close()

	resp, err := http.Get(server.URL + "/boards/" + uuid.New().String() + "/ws?token=whatever")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestHandler_ServeWS_DeliversPublishedEvent(t *testing.T) {
	hub := NewHub()
	userID := uuid.New()
	boardID := uuid.New()
	h := NewHandler(hub, &fakeTokenParser{userID: userID}, &fakeWSBoardAuthorizer{allow: true}, "http://localhost:5173")
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	server := httptest.NewServer(r)
	defer server.Close()

	wsURL := "ws" + server.URL[len("http"):] + "/boards/" + boardID.String() + "/ws?token=whatever"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	require.NoError(t, err)
	defer func() { _ = conn.CloseNow() }()

	require.Eventually(t, func() bool {
		return hub.SubscriberCount(boardID) == 1
	}, time.Second, 10*time.Millisecond)

	hub.Publish(context.Background(), boardID, "card.created", map[string]string{"id": "abc"})

	var received Event
	require.NoError(t, wsjson.Read(ctx, conn, &received))
	require.Equal(t, "card.created", received.Type)
	require.Equal(t, boardID, received.BoardID)

	conn.Close(websocket.StatusNormalClosure, "")
}

func TestHandler_ServeWS_RejectsDisallowedOrigin(t *testing.T) {
	hub := NewHub()
	userID := uuid.New()
	boardID := uuid.New()
	h := NewHandler(hub, &fakeTokenParser{userID: userID}, &fakeWSBoardAuthorizer{allow: true}, "http://localhost:5173")
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	server := httptest.NewServer(r)
	defer server.Close()

	wsURL := "ws" + server.URL[len("http"):] + "/boards/" + boardID.String() + "/ws?token=whatever"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{"http://evil.example.com"}},
	})
	require.Error(t, err)
}
