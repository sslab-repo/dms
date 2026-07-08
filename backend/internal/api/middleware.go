package api

import "net/http"

const maxJSONBodyBytes = 1 * 1024 * 1024 // 1 MB limit for JSON API endpoints

// withSecurityHeaders wraps a handler to set defensive HTTP headers on every response.
func withSecurityHeaders(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy", "default-src 'none'")
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		next(w, r)
	}
}

// withBodyLimit limits the request body to maxJSONBodyBytes to prevent
// memory exhaustion from oversized payloads on JSON API endpoints.
func withBodyLimit(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
		next(w, r)
	}
}
