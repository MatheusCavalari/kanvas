package webhook_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MatheusCavalari/kanvas/backend/internal/webhook"
)

func TestDeliverer_SuccessfulDelivery_SignsAndRecordsSuccess(t *testing.T) {
	const secret = "top-secret"
	var gotBody []byte
	var gotSignature string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSignature = r.Header.Get(webhook.SignatureHeader)
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	repo := newFakeRepo()
	ctx := context.Background()

	wh, err := repo.Create(ctx, webhook.Webhook{
		ID: uuid.New(), BoardID: uuid.New(), OwnerID: uuid.New(),
		URL: server.URL, Secret: secret, Events: []string{"card.created"}, Active: true,
	})
	require.NoError(t, err)

	delivery, err := repo.CreateDelivery(ctx, webhook.Delivery{
		ID: uuid.New(), WebhookID: wh.ID, EventType: "card.created",
		Payload: []byte(`{"id":"abc"}`), Status: webhook.DeliveryStatusPending,
	})
	require.NoError(t, err)

	handler := webhook.NewDeliverer(repo)
	payload, err := json.Marshal(map[string]interface{}{
		"delivery_id": delivery.ID.String(),
		"webhook_id":  wh.ID.String(),
		"event_type":  "card.created",
		"data":        json.RawMessage(`{"id":"abc"}`),
	})
	require.NoError(t, err)

	require.NoError(t, handler(ctx, payload))

	require.JSONEq(t, `{"id":"abc"}`, string(gotBody))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(gotBody)
	require.Equal(t, hex.EncodeToString(mac.Sum(nil)), gotSignature)

	deliveries, err := repo.ListDeliveriesByWebhook(ctx, wh.ID, farFuture(), 10)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	require.Equal(t, webhook.DeliveryStatusSuccess, deliveries[0].Status)
	require.Equal(t, 1, deliveries[0].Attempts)
	require.NotNil(t, deliveries[0].ResponseCode)
	require.Equal(t, http.StatusOK, *deliveries[0].ResponseCode)
}

func TestDeliverer_FailedDelivery_ReturnsErrorAndRecordsFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	repo := newFakeRepo()
	ctx := context.Background()

	wh, err := repo.Create(ctx, webhook.Webhook{
		ID: uuid.New(), BoardID: uuid.New(), OwnerID: uuid.New(),
		URL: server.URL, Secret: "s3cr3t", Events: []string{"card.created"}, Active: true,
	})
	require.NoError(t, err)

	delivery, err := repo.CreateDelivery(ctx, webhook.Delivery{
		ID: uuid.New(), WebhookID: wh.ID, EventType: "card.created",
		Payload: []byte(`{}`), Status: webhook.DeliveryStatusPending,
	})
	require.NoError(t, err)

	handler := webhook.NewDeliverer(repo)
	payload, err := json.Marshal(map[string]interface{}{
		"delivery_id": delivery.ID.String(),
		"webhook_id":  wh.ID.String(),
		"event_type":  "card.created",
		"data":        json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	err = handler(ctx, payload)
	require.Error(t, err)

	deliveries, err := repo.ListDeliveriesByWebhook(ctx, wh.ID, farFuture(), 10)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	require.Equal(t, webhook.DeliveryStatusFailed, deliveries[0].Status)
	require.NotNil(t, deliveries[0].ResponseCode)
	require.Equal(t, http.StatusInternalServerError, *deliveries[0].ResponseCode)
}

func TestDeliverer_DeletedWebhook_NoErrorNoRetry(t *testing.T) {
	repo := newFakeRepo()
	ctx := context.Background()

	handler := webhook.NewDeliverer(repo)
	payload, err := json.Marshal(map[string]interface{}{
		"delivery_id": uuid.New().String(),
		"webhook_id":  uuid.New().String(),
		"event_type":  "card.created",
		"data":        json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	require.NoError(t, handler(ctx, payload))
}

func TestDeliverer_InactiveWebhook_NoErrorNoDelivery(t *testing.T) {
	repo := newFakeRepo()
	ctx := context.Background()

	wh, err := repo.Create(ctx, webhook.Webhook{
		ID: uuid.New(), BoardID: uuid.New(), OwnerID: uuid.New(),
		URL: "https://example.com/hook", Secret: "s3cr3t", Events: []string{"card.created"}, Active: false,
	})
	require.NoError(t, err)

	handler := webhook.NewDeliverer(repo)
	payload, err := json.Marshal(map[string]interface{}{
		"delivery_id": uuid.New().String(),
		"webhook_id":  wh.ID.String(),
		"event_type":  "card.created",
		"data":        json.RawMessage(`{}`),
	})
	require.NoError(t, err)

	require.NoError(t, handler(ctx, payload))
}
