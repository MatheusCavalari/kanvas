package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/worker"
)

// deliverHTTPTimeout bounds how long the deliverer waits for the receiving
// endpoint to respond before treating the attempt as failed.
const deliverHTTPTimeout = 10 * time.Second

// deliverer executes "webhook.deliver" jobs: it looks up the webhook, POSTs
// the event payload to its URL with an HMAC-SHA256 signature, and records
// the outcome. It's returned as a worker.Handler by NewDeliverer, rather
// than exposed directly, since callers only need to register it.
type deliverer struct {
	repo   Repository
	client *http.Client
}

// NewDeliverer returns a worker.Handler for "webhook.deliver" jobs.
//
// A returned error causes the worker (see internal/platform/worker/worker.go)
// to re-enqueue the job immediately with Retries incremented, up to its
// built-in cap of 5 attempts — there is no separate backoff delay to
// implement here, since the underlying queue has no notion of a delayed
// job.
func NewDeliverer(repo Repository) worker.Handler {
	d := &deliverer{repo: repo, client: &http.Client{Timeout: deliverHTTPTimeout}}
	return d.deliver
}

func (d *deliverer) deliver(ctx context.Context, payload json.RawMessage) error {
	var job deliverJobPayload
	if err := json.Unmarshal(payload, &job); err != nil {
		return fmt.Errorf("webhook: invalid job payload: %w", err)
	}

	deliveryID, err := uuid.Parse(job.DeliveryID)
	if err != nil {
		return fmt.Errorf("webhook: invalid delivery id %q: %w", job.DeliveryID, err)
	}
	webhookID, err := uuid.Parse(job.WebhookID)
	if err != nil {
		return fmt.Errorf("webhook: invalid webhook id %q: %w", job.WebhookID, err)
	}

	wh, err := d.repo.GetByID(ctx, webhookID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			// The webhook was deleted after the job was enqueued. Nothing
			// left to deliver, and retrying won't change that.
			slog.Info("webhook: skipping delivery for deleted webhook", "webhook_id", webhookID, "delivery_id", deliveryID)
			return nil
		}
		return err
	}
	if !wh.Active {
		slog.Info("webhook: skipping delivery for inactive webhook", "webhook_id", webhookID, "delivery_id", deliveryID)
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, wh.URL, bytes.NewReader(job.Data))
	if err != nil {
		d.recordFailure(ctx, deliveryID, nil)
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(SignatureHeader, sign(wh.Secret, job.Data))

	resp, err := d.client.Do(req)
	if err != nil {
		d.recordFailure(ctx, deliveryID, nil)
		return err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	code := resp.StatusCode
	status := DeliveryStatusFailed
	if code >= 200 && code < 300 {
		status = DeliveryStatusSuccess
	}
	if _, err := d.repo.UpdateDeliveryStatus(ctx, deliveryID, status, &code, time.Now()); err != nil {
		slog.Warn("webhook: failed to record delivery outcome", "delivery_id", deliveryID, "error", err)
	}

	if status != DeliveryStatusSuccess {
		return fmt.Errorf("webhook delivery to %s failed with status %d", wh.URL, code)
	}
	return nil
}

// recordFailure records a delivery attempt that never got a response (a
// request-construction or network error), so it has no response code.
func (d *deliverer) recordFailure(ctx context.Context, deliveryID uuid.UUID, responseCode *int) {
	if _, err := d.repo.UpdateDeliveryStatus(ctx, deliveryID, DeliveryStatusFailed, responseCode, time.Now()); err != nil {
		slog.Warn("webhook: failed to record delivery failure", "delivery_id", deliveryID, "error", err)
	}
}

// sign computes the hex-encoded HMAC-SHA256 signature of body using secret,
// for the SignatureHeader.
func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
