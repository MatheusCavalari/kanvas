package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/jwt"
)

type contextKey string

const userIDContextKey contextKey = "userID"

func Auth(issuer *jwt.Issuer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			token, ok := strings.CutPrefix(header, "Bearer ")
			if !ok || token == "" {
				http.Error(w, "missing or invalid Authorization header", http.StatusUnauthorized)
				return
			}

			userID, err := issuer.ParseAccessToken(token)
			if err != nil {
				http.Error(w, "invalid or expired access token", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), userIDContextKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func UserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(userIDContextKey).(uuid.UUID)
	return id, ok
}
