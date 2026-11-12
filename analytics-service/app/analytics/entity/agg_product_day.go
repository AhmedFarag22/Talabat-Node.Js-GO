package entity

import "time"

// AggProductDay is one document in the agg_product_day collection — a per-day
// rollup of a single product's sales within a restaurant. Populated from the
// line items of order.placed events (one upsert per line, batched in a single
// BulkWrite — see repository).
//
// UnitsSum sums quantity; RevenueSum sums lineTotal (minor units). OrdersCount
// counts how many orders included this product that day.
type AggProductDay struct {
	RestaurantID int64  `bson:"restaurant_id"`
	ProductID    int64  `bson:"product_id"`
	Date         string `bson:"date"` // "YYYY-MM-DD" UTC
	Currency     string `bson:"currency"`

	UnitsSum    int64 `bson:"units_sum"`
	RevenueSum  int64 `bson:"revenue_sum"`
	OrdersCount int64 `bson:"orders_count"`

	UpdatedAt time.Time `bson:"updated_at"`
}
