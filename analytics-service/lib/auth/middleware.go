package auth

import (
	"net/http"
	"strings"

	"github.com/quickbite/analytics-service/lib/appcontext"
	apperr "github.com/quickbite/analytics-service/lib/errors"
)

// Authenticate parses the JWT from the access cookie or the Authorization
// header, attaches claims to the request context, and refuses if invalid.
// Mirrors lib/auth/guard.ts (authenticate) in the Node services.
func Authenticate(secret, cookieName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := readToken(r, cookieName)
			if token == "" {
				apperr.Render(w, apperr.New("UNAUTHENTICATED", http.StatusUnauthorized, "missing access token"))
				return
			}
			claims, err := VerifyAccessToken(token, secret)
			if err != nil {
				apperr.Render(w, apperr.New("UNAUTHENTICATED", http.StatusUnauthorized, "invalid access token"))
				return
			}
			ctx := appcontext.WithClaims(r.Context(), claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func readToken(r *http.Request, cookieName string) string {
	if c, err := r.Cookie(cookieName); err == nil && c.Value != "" {
		return c.Value
	}
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}
