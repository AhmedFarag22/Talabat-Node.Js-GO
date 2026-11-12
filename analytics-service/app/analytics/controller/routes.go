package controller

import (
	"log/slog"

	"github.com/go-chi/chi/v5"

	"github.com/quickbite/analytics-service/app/analytics"
	"github.com/quickbite/analytics-service/lib/auth"
	"github.com/quickbite/analytics-service/lib/config"
	apperr "github.com/quickbite/analytics-service/lib/errors"
	"github.com/quickbite/analytics-service/lib/rbac"
)

// Mount wires the analytics module's routes onto the parent router. Every route
// is GET-only and guarded by Authenticate → RBAC("analytics:read").
func Mount(r chi.Router, logger *slog.Logger, cfg config.Config, rbacCache *rbac.Cache, ctrl *AnalyticsController) {
	r.Route("/analytics", func(r chi.Router) {
		r.Use(auth.Authenticate(cfg.JWT.AccessSecret, cfg.JWT.CookieName))
		r.Use(rbac.Require(rbacCache, analytics.PermAnalyticsRead))

		// Restaurant-scoped.
		r.Get("/restaurants/{restaurantId}/days", apperr.Wrap(logger, ctrl.GetRestaurantDays))
		r.Get("/restaurants/{restaurantId}/failures", apperr.Wrap(logger, ctrl.GetRestaurantFailures))
		r.Get("/restaurants/{restaurantId}/delivery-avg", apperr.Wrap(logger, ctrl.GetRestaurantDeliveryAvg))

		// Branch-scoped.
		r.Get("/branches/{branchId}/days", apperr.Wrap(logger, ctrl.GetBranchDays))

		// Product rollups (filters are query params, non-strict).
		r.Get("/products", apperr.Wrap(logger, ctrl.GetProducts))

		// Platform-wide.
		r.Get("/platform/days", apperr.Wrap(logger, ctrl.GetPlatformDays))
		r.Get("/platform/failures", apperr.Wrap(logger, ctrl.GetPlatformFailures))
		r.Get("/platform/active-restaurants", apperr.Wrap(logger, ctrl.GetActiveRestaurants))
	})
}
