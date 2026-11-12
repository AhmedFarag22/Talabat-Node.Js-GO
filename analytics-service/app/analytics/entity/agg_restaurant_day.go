// Package entity holds plain Go structs that map 1:1 to a Mongo document.
// No methods beyond simple invariants. No DB knowledge. No DTO/wire concerns.
package entity

import "time"

// AggRestaurantDay is one document in the agg_restaurant_day collection.
// Money fields are integer minor units (e.g. 1500 = 15.00 EGP).
//
// Averages are stored as sum + count so per-day rows merge associatively
// across replays — derived values are computed at read time in the service.
//
// Counters that a given event doesn't touch are simply absent from the doc
// until first incremented; Mongo's $inc creates them on demand and a missing
// field decodes to the int64 zero value. That keeps every Upsert*Delta free
// of zero-initialisation and makes out-of-order events (delivered before
// placed) safe.
type AggRestaurantDay struct {
	RestaurantID int64  `bson:"restaurant_id"`
	Date         string `bson:"date"` // "YYYY-MM-DD" UTC
	Currency     string `bson:"currency"`

	OrdersCount int64 `bson:"orders_count"`
	RevenueSum  int64 `bson:"revenue_sum"` // gross placed revenue, minor units

	RejectedCount  int64 `bson:"rejected_count"`
	CancelledCount int64 `bson:"cancelled_count"`

	DeliveredCount  int64 `bson:"delivered_count"`
	DeliveryMsSum   int64 `bson:"delivery_ms_sum"`
	DeliveryMsCount int64 `bson:"delivery_ms_count"`

	PaymentsCount int64 `bson:"payments_count"`
	PaymentsSum   int64 `bson:"payments_sum"` // captured online amount, minor units

	UpdatedAt time.Time `bson:"updated_at"`
}
