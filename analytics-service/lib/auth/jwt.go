// Package auth verifies JWTs (same secret + shape as core/order) and exposes
// the Authenticate / RequireInternalAPIKey middlewares.
package auth

import (
	"fmt"

	"github.com/golang-jwt/jwt/v5"
	"github.com/quickbite/analytics-service/lib/appcontext"
)

// rawClaims matches the shape produced by core/order's signing code.
// Both services issue HS256 with the access secret. We keep the field set
// loose (any) for restaurant fields because they're optional.
type rawClaims struct {
	UserID         any   `json:"userId"`
	Role           string `json:"role"`
	RestaurantID   any    `json:"restaurantId,omitempty"`
	RestaurantRole string `json:"restaurantRole,omitempty"`
	BranchIDs      []any  `json:"branchIds,omitempty"`
	jwt.RegisteredClaims
}

// VerifyAccessToken validates the signature + expiry against the secret and
// returns ClaimsValue suitable for context.
func VerifyAccessToken(token, secret string) (*appcontext.ClaimsValue, error) {
	parsed, err := jwt.ParseWithClaims(token, &rawClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	rc, ok := parsed.Claims.(*rawClaims)
	if !ok || !parsed.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	branchIDs := make([]string, 0, len(rc.BranchIDs))
	for _, b := range rc.BranchIDs {
		branchIDs = append(branchIDs, toString(b))
	}

	return &appcontext.ClaimsValue{
		UserID:         toString(rc.UserID),
		Role:           rc.Role,
		RestaurantID:   toString(rc.RestaurantID),
		RestaurantRole: rc.RestaurantRole,
		BranchIDs:      branchIDs,
	}, nil
}

// toString coerces JSON numbers / strings to a string. Service IDs cross the
// wire as either depending on the issuer's marshalling — handle both.
func toString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return fmt.Sprintf("%d", int64(x))
	case int:
		return fmt.Sprintf("%d", x)
	case int64:
		return fmt.Sprintf("%d", x)
	default:
		return fmt.Sprintf("%v", x)
	}
}
