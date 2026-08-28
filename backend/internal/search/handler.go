package search

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/gen"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/middleware"
)

// BoardAuthorizer verifies that a user is a member of a board. Defined
// locally so the search package does not depend on the board package's
// concrete service.
type BoardAuthorizer interface {
	EnsureMember(ctx context.Context, boardID, userID uuid.UUID) error
}

type Handler struct {
	queries *gen.Queries
	board   BoardAuthorizer
}

func NewHandler(queries *gen.Queries, board BoardAuthorizer) *Handler {
	return &Handler{queries: queries, board: board}
}

func (h *Handler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.With(authMiddleware).Get("/boards/{boardID}/search", h.Search)
}

// Search handles GET /boards/{boardID}/search?q=...&type=cards|comments|all
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	boardID, err := uuid.Parse(chi.URLParam(r, "boardID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid board id")
		return
	}

	if err := h.board.EnsureMember(r.Context(), boardID, userID); err != nil {
		writeError(w, http.StatusForbidden, "forbidden", "forbidden")
		return
	}

	q := r.URL.Query().Get("q")
	if len(q) < 2 {
		writeError(w, http.StatusBadRequest, "invalid_request", "query must be at least 2 characters")
		return
	}

	searchType := r.URL.Query().Get("type")
	if searchType == "" {
		searchType = "all"
	}

	type cardResult struct {
		ID       string  `json:"id"`
		Title    string  `json:"title"`
		ColumnID string  `json:"column_id"`
		Rank     float64 `json:"rank"`
	}
	type commentResult struct {
		ID          string  `json:"id"`
		CardID      string  `json:"card_id"`
		BodyExcerpt string  `json:"body_excerpt"`
		Rank        float64 `json:"rank"`
	}
	type response struct {
		Cards    []cardResult    `json:"cards,omitempty"`
		Comments []commentResult `json:"comments,omitempty"`
	}

	var resp response

	if searchType == "cards" || searchType == "all" {
		cards, err := h.queries.SearchCards(r.Context(), gen.SearchCardsParams{
			PlaintoTsquery: q, BoardID: boardID,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "search failed")
			return
		}
		for _, c := range cards {
			resp.Cards = append(resp.Cards, cardResult{
				ID: c.ID.String(), Title: c.Title, ColumnID: c.ColumnID.String(), Rank: float64(c.Rank),
			})
		}
	}

	if searchType == "comments" || searchType == "all" {
		comments, err := h.queries.SearchComments(r.Context(), gen.SearchCommentsParams{
			PlaintoTsquery: q, BoardID: boardID,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "search failed")
			return
		}
		for _, c := range comments {
			resp.Comments = append(resp.Comments, commentResult{
				ID: c.ID.String(), CardID: c.CardID.String(), BodyExcerpt: string(c.BodyExcerpt), Rank: float64(c.Rank),
			})
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// Duplicated per-package helpers (project convention)
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]interface{}{
		"error": map[string]string{"code": code, "message": message},
	})
}
