package rbac

import (
	"net/http"

	"github.com/quickbite/analytics-service/lib/appcontext"
	apperr "github.com/quickbite/analytics-service/lib/errors"
)

// Require returns a middleware that 403s unless the authed user's role
// (looked up via the cache) includes the required permission. Mirrors
// rbac("perm") in the Node services.
func Require(cache *Cache, required string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := appcontext.Claims(r.Context())
			if claims == nil {
				apperr.Render(w, apperr.New("UNAUTHENTICATED", http.StatusUnauthorized, "missing claims"))
				return
			}
			perms, err := cache.Get(r.Context(), claims.Role)
			if err != nil {
				apperr.Render(w, apperr.New("RBAC_LOOKUP_FAILED", http.StatusServiceUnavailable, "permission lookup failed").WithCause(err))
				return
			}
			if !contains(perms, required) {
				apperr.Render(w, apperr.New("FORBIDDEN", http.StatusForbidden, "missing permission: "+required))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
