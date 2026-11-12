package auth

import (
	"net/http"

	apperr "github.com/quickbite/analytics-service/lib/errors"
)

const HeaderAPIKey = "api-key"

// RequireInternalAPIKey gates service-to-service routes. Plain-equality check
// against the configured shared secret. No HMAC — broker is trusted on the
// wire. Mirrors lib/auth/api-key.ts in the Node services.
func RequireInternalAPIKey(expected string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get(HeaderAPIKey); got == "" || got != expected {
				apperr.Render(w, apperr.New("UNAUTHENTICATED", http.StatusUnauthorized, "missing or invalid api key"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
