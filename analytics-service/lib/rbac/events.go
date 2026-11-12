package rbac

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/quickbite/analytics-service/lib/coreevents"
)

// PermissionsChangedPayload is the slice of core-service's
// rbac.permissions_changed event we care about. An empty Role means "every
// role's permissions may have changed" → wipe the whole cache.
type PermissionsChangedPayload struct {
	Role string `json:"role"`
}

// InvalidationHandler returns the event handler that keeps the read-through
// RBAC cache fresh faster than its TTL. Wired into the core.events consumer in
// lib/boot. This is the homework counterpart to "TTL is the only invalidation
// we have" from the video.
func InvalidationHandler(cache *Cache, logger *slog.Logger) coreevents.EventHandler {
	return func(_ context.Context, raw json.RawMessage) error {
		var p PermissionsChangedPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			// Malformed payload: invalidate everything rather than DLQ a cache
			// hint — staleness is the only risk and InvalidateAll is cheap.
			logger.Warn("rbac.permissions_changed: bad payload, invalidating all", slog.String("error", err.Error()))
			cache.InvalidateAll()
			return nil
		}
		if p.Role == "" {
			logger.Info("rbac.permissions_changed: invalidating all roles")
			cache.InvalidateAll()
			return nil
		}
		logger.Info("rbac.permissions_changed: invalidating role", slog.String("role", p.Role))
		cache.Invalidate(p.Role)
		return nil
	}
}
