package webhook

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/middleware"
)

// farFutureCursor is used as the cursor for the first page of a DESC
// listing (created_at < cursor), since there is no natural "beginning"
// value to use the way ASC listings use the zero time.
var farFutureCursor = time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)

type webhookService interface {
	Create(ctx context.Context, boardID, requesterID uuid.UUID, rawURL string, events []string) (Webhook, error)
	Update(ctx context.Context, boardID, webhookID, requesterID uuid.UUID, rawURL string, events []string, active bool) (Webhook, error)
	Delete(ctx context.Context, boardID, webhookID, requesterID uuid.UUID) error
	ListByBoard(ctx context.Context, boardID, requesterID uuid.UUID) ([]Webhook, error)
	ListDeliveries(ctx context.Context, boardID, webhookID, requesterID uuid.UUID, cursor time.Time, limit int) ([]Delivery, error)
}

type Handler struct {
	service webhookService
}

func NewHandler(service webhookService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Route("/boards/{boardID}/webhooks", func(r chi.Router) {
		r.Use(authMiddleware)
		r.Get("/", h.ListWebhooks)
		r.Post("/", h.CreateWebhook)
		r.Route("/{webhookID}", func(r chi.Router) {
			r.Patch("/", h.UpdateWebhook)
			r.Delete("/", h.DeleteWebhook)
			r.Get("/deliveries", h.ListDeliveries)
		})
	})
}

// webhookView is the representation returned by list/update — it never
// includes the secret, since that's only handed back once, at creation
// time (see webhookCreatedView).
type webhookView struct {
	ID        string    `json:"id"`
	BoardID   string    `json:"board_id"`
	URL       string    `json:"url"`
	Events    []string  `json:"events"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
}

type webhookCreatedView struct {
	webhookView
	Secret string `json:"secret"`
}

type deliveryView struct {
	ID            string     `json:"id"`
	WebhookID     string     `json:"webhook_id"`
	EventType     string     `json:"event_type"`
	Status        string     `json:"status"`
	Attempts      int        `json:"attempts"`
	ResponseCode  *int       `json:"response_code,omitempty"`
	LastAttemptAt *time.Time `json:"last_attempt_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

type createWebhookRequest struct {
	URL    string   `json:"url"`
	Events []string `json:"events"`
}

type updateWebhookRequest struct {
	URL    string   `json:"url"`
	Events []string `json:"events"`
	Active bool     `json:"active"`
}

type listDeliveriesResponse struct {
	Deliveries []deliveryView `json:"deliveries"`
	NextCursor string         `json:"next_cursor,omitempty"`
}

func (h *Handler) CreateWebhook(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	boardID, err := parseUUIDParam(r, "boardID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid board id")
		return
	}

	var req createWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}

	created, err := h.service.Create(r.Context(), boardID, userID, req.URL, req.Events)
	if err != nil {
		h.writeWebhookError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, webhookCreatedView{webhookView: toWebhookView(created), Secret: created.Secret})
}

func (h *Handler) ListWebhooks(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	boardID, err := parseUUIDParam(r, "boardID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid board id")
		return
	}

	webhooks, err := h.service.ListByBoard(r.Context(), boardID, userID)
	if err != nil {
		h.writeWebhookError(w, err)
		return
	}

	views := make([]webhookView, 0, len(webhooks))
	for _, wh := range webhooks {
		views = append(views, toWebhookView(wh))
	}
	writeJSON(w, http.StatusOK, views)
}

func (h *Handler) UpdateWebhook(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	boardID, err := parseUUIDParam(r, "boardID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid board id")
		return
	}
	webhookID, err := parseUUIDParam(r, "webhookID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid webhook id")
		return
	}

	var req updateWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}

	updated, err := h.service.Update(r.Context(), boardID, webhookID, userID, req.URL, req.Events, req.Active)
	if err != nil {
		h.writeWebhookError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toWebhookView(updated))
}

func (h *Handler) DeleteWebhook(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	boardID, err := parseUUIDParam(r, "boardID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid board id")
		return
	}
	webhookID, err := parseUUIDParam(r, "webhookID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid webhook id")
		return
	}

	if err := h.service.Delete(r.Context(), boardID, webhookID, userID); err != nil {
		h.writeWebhookError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListDeliveries(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	boardID, err := parseUUIDParam(r, "boardID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid board id")
		return
	}
	webhookID, err := parseUUIDParam(r, "webhookID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid webhook id")
		return
	}

	cursor, err := decodeCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid cursor")
		return
	}

	limit := DefaultDeliveriesLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "invalid_request", "invalid limit")
			return
		}
		limit = n
	}

	deliveries, err := h.service.ListDeliveries(r.Context(), boardID, webhookID, userID, cursor, limit)
	if err != nil {
		h.writeWebhookError(w, err)
		return
	}

	views := make([]deliveryView, 0, len(deliveries))
	for _, d := range deliveries {
		views = append(views, toDeliveryView(d))
	}

	resp := listDeliveriesResponse{Deliveries: views}
	if len(deliveries) > 0 && len(deliveries) >= limit {
		resp.NextCursor = encodeCursor(deliveries[len(deliveries)-1].CreatedAt)
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) writeWebhookError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", err.Error())
	case errors.Is(err, ErrWebhookNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrInvalidURL), errors.Is(err, ErrInvalidEvents):
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func parseUUIDParam(r *http.Request, name string) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, name))
}

func toWebhookView(w Webhook) webhookView {
	return webhookView{
		ID:        w.ID.String(),
		BoardID:   w.BoardID.String(),
		URL:       w.URL,
		Events:    w.Events,
		Active:    w.Active,
		CreatedAt: w.CreatedAt,
	}
}

func toDeliveryView(d Delivery) deliveryView {
	return deliveryView{
		ID:            d.ID.String(),
		WebhookID:     d.WebhookID.String(),
		EventType:     d.EventType,
		Status:        d.Status,
		Attempts:      d.Attempts,
		ResponseCode:  d.ResponseCode,
		LastAttemptAt: d.LastAttemptAt,
		CreatedAt:     d.CreatedAt,
	}
}

// decodeCursor decodes a base64-encoded RFC3339Nano timestamp query param.
// An empty raw value yields farFutureCursor, meaning "from the newest
// entry", since this list is sorted created_at DESC.
func decodeCursor(raw string) (time.Time, error) {
	if raw == "" {
		return farFutureCursor, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse(time.RFC3339Nano, string(decoded))
}

func encodeCursor(t time.Time) string {
	return base64.StdEncoding.EncodeToString([]byte(t.Format(time.RFC3339Nano)))
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{Error: errorBody{Code: code, Message: message}})
}
