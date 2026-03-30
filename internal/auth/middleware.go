package auth

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
)

// Middleware creates HTTP middleware that validates JWT Bearer tokens.
// Never logs the token itself (AUTH-10).
func Middleware(validator *CognitoValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeAuthError(w, http.StatusUnauthorized, "missing Authorization header")
				slog.Warn("auth failure: missing Authorization header")
				return
			}

			if !strings.HasPrefix(authHeader, "Bearer ") {
				writeAuthError(w, http.StatusUnauthorized, "invalid Authorization header format")
				slog.Warn("auth failure: invalid Authorization header format")
				return
			}

			tokenString := strings.TrimPrefix(authHeader, "Bearer ")

			claims, err := validator.Validate(r.Context(), tokenString)
			if err != nil {
				writeAuthError(w, http.StatusUnauthorized, "invalid token")
				// Log the reason but never the token (AUTH-10)
				slog.Warn("auth failure", "reason", err.Error())
				return
			}

			ctx := WithClaims(r.Context(), claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func writeAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
