package entity

import "time"

// AggBranchDay is one document in the agg_branch_day collection — the same
// day rollup as AggRestaurantDay but keyed by branch. Every order event that
// carries a branchId updates both rows, so the two stay consistent.
//
// See AggRestaurantDay for the "absent counter == zero" convention.
type AggBranchDay struct {
	BranchID int64  `bson:"branch_id"`
	Date     string `bson:"date"` // "YYYY-MM-DD" UTC
	Currency string `bson:"currency"`

	OrdersCount int64 `bson:"orders_count"`
	RevenueSum  int64 `bson:"revenue_sum"`

	RejectedCount  int64 `bson:"rejected_count"`
	CancelledCount int64 `bson:"cancelled_count"`

	DeliveredCount  int64 `bson:"delivered_count"`
	DeliveryMsSum   int64 `bson:"delivery_ms_sum"`
	DeliveryMsCount int64 `bson:"delivery_ms_count"`

	PaymentsCount int64 `bson:"payments_count"`
	PaymentsSum   int64 `bson:"payments_sum"`

	UpdatedAt time.Time `bson:"updated_at"`
}
