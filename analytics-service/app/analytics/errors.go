package analytics

import (
	"net/http"

	apperr "github.com/quickbite/analytics-service/lib/errors"
)

// Module-level AppError values. Construct once, reuse — same convention as
// app/<module>/errors.ts in the Node services.
var (
	ErrInvalidDateRange = apperr.New(
		"ANALYTICS_INVALID_DATE_RANGE",
		http.StatusBadRequest,
		"invalid date range: 'from' must be <= 'to' and within 1 year",
	)
	ErrInvalidRestaurantID = apperr.New(
		"ANALYTICS_INVALID_RESTAURANT_ID",
		http.StatusBadRequest,
		"invalid restaurant id",
	)
	ErrInvalidBranchID = apperr.New(
		"ANALYTICS_INVALID_BRANCH_ID",
		http.StatusBadRequest,
		"invalid branch id",
	)
)
