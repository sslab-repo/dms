package auth

import (
	"encoding/json"
	"net/http"
	"strings"
)

func RequireAuth(secret string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		if header == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing Authorization header"})
			return
		}
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid Authorization header"})
			return
		}

		claims, err := VerifyJWT(strings.TrimSpace(parts[1]), secret)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired token"})
			return
		}

		next.ServeHTTP(w, r.WithContext(WithClaims(r.Context(), claims)))
	})
}

// OptionalAuth is like RequireAuth but does not reject requests without a
// token. If a valid Bearer token is present, claims are injected into the
// context; otherwise the request proceeds without claims.
func OptionalAuth(secret string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		if header != "" {
			parts := strings.SplitN(header, " ", 2)
			if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
				if claims, err := VerifyJWT(strings.TrimSpace(parts[1]), secret); err == nil {
					r = r.WithContext(WithClaims(r.Context(), claims))
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func RequireRole(secret string, role string, next http.Handler) http.Handler {
	return RequireAuth(secret, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := ClaimsFromContext(r.Context())
		if !ok || claims.Role != role {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		next.ServeHTTP(w, r)
	}))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
