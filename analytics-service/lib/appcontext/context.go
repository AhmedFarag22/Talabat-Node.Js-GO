// Package appcontext is the typed surface for values that flow through
// context.Context: JWT claims, correlation id, request-scoped logger, etc.
//
// Equivalent to lib/types/express.d.ts in the Node services (req.user,
// req.correlationId). Handlers read claims here, never from headers/cookies
// directly — auth middleware is the only place that calls With*.
package appcontext

import "context"

type ctxKey int

const (
	claimsKey ctxKey = iota
	correlationIDKey
)

// Claims is the JWT payload as the rest of the service sees it. Mirrors the
// shape produced by core/order so the same access token works across services.
type ClaimsValue struct {
	UserID         string
	Role           string
	RestaurantID   string   // empty if not a restaurant user
	RestaurantRole string   // owner|manager|staff — empty if not applicable
	BranchIDs      []string // empty if not applicable
}

func WithClaims(ctx context.Context, c *ClaimsValue) context.Context {
	return context.WithValue(ctx, claimsKey, c)
}

// Claims returns the claims attached to ctx by the auth middleware, or nil.
func Claims(ctx context.Context) *ClaimsValue {
	v, _ := ctx.Value(claimsKey).(*ClaimsValue)
	return v
}

func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, correlationIDKey, id)
}

// CorrelationID returns the correlation id attached to ctx, or "".
func CorrelationID(ctx context.Context) string {
	v, _ := ctx.Value(correlationIDKey).(string)
	return v
}
