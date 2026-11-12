// Package dto holds request and response shapes. Validation tags use
// go-playground/validator (the equivalent of class-validator in Node).
//
// Inbound DTOs are validated by the controller; outbound DTOs are plain
// structs with json tags so the wire format is decoupled from entities.
package dto

// DateRangeQuery is the shared from/to query-string DTO. `to` must parse as a
// date and be >= `from` (range bound is enforced in the service so the error
// code stays domain-specific).
type DateRangeQuery struct {
	From string `validate:"required,datetime=2006-01-02"`
	To   string `validate:"required,datetime=2006-01-02"`
}

// GetRestaurantDaysQuery is the query for the restaurant/branch days,
// failures, and delivery-avg endpoints. Retained as a named alias for the
// original video slice.
type GetRestaurantDaysQuery = DateRangeQuery

// GetProductsQuery is the query for GET /analytics/products. restaurantId is
// required; productId is an optional filter. from/to bound the range.
type GetProductsQuery struct {
	RestaurantID int64  `validate:"required,gt=0"`
	ProductID    *int64 `validate:"omitempty,gt=0"`
	From         string `validate:"required,datetime=2006-01-02"`
	To           string `validate:"required,datetime=2006-01-02"`
}

// GetPlatformDaysQuery is the query for GET /analytics/platform/days.
// CountryCode is an optional filter; empty means aggregate across countries.
type GetPlatformDaysQuery struct {
	From        string `validate:"required,datetime=2006-01-02"`
	To          string `validate:"required,datetime=2006-01-02"`
	CountryCode string `validate:"omitempty,alpha,min=2,max=3"`
}

// GetActiveRestaurantsQuery is the query for
// GET /analytics/platform/active-restaurants.
type GetActiveRestaurantsQuery struct {
	Date string `validate:"required,datetime=2006-01-02"`
}
