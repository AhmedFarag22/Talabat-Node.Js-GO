package entity

import "time"

// AggPlatformDay is one document in the agg_platform_day collection — a
// whole-platform day rollup, keyed by (date, country_code) so dashboards can
// either slice by country or aggregate across all of them (group by date).
//
// See AggRestaurantDay for the "absent counter == zero" convention.
type AggPlatformDay struct {
	Date        string `bson:"date"` // "YYYY-MM-DD" UTC
	CountryCode string `bson:"country_code"`
	Currency    string `bson:"currency"`

	OrdersCount int64 `bson:"orders_count"`
	RevenueSum  int64 `bson:"revenue_sum"`

	RejectedCount  int64 `bson:"rejected_count"`
	CancelledCount int64 `bson:"cancelled_count"`

	DeliveredCount  int64 `bson:"delivered_count"`
	DeliveryMsSum   int64 `bson:"delivery_ms_sum"`
	DeliveryMsCount int64 `bson:"delivery_ms_count"`

	UpdatedAt time.Time `bson:"updated_at"`
}
