// Package analytics holds the module's shared types, errors, and enums —
// everything that subpackages (service, controller, eventhandlers) need to
// agree on. Pure data; no I/O.
package analytics

// ---------------------------------------------------------------------------
// Inbound event inputs — the slice of each upstream event the service needs,
// decoupled from the wire payload so service signatures stay stable as the
// upstream events grow.
// ---------------------------------------------------------------------------

// OnOrderPlacedInput is the parsed order.placed event.
type OnOrderPlacedInput struct {
	OrderID      string
	RestaurantID int64
	BranchID     int64
	CountryCode  string
	Currency     string
	Total        int64  // integer minor units
	PlacedAt     string // ISO-8601 UTC
	Items        []OrderLineInput
}

// OrderLineInput is one line of a placed order, used for per-product rollups.
type OrderLineInput struct {
	ProductID int64
	Quantity  int64
	UnitPrice int64 // minor units
	LineTotal int64 // minor units
}

// OnOrderDeliveredInput is the parsed order.delivered event. PlacedAt keys the
// row (same day order.placed used); DeliveredAt - PlacedAt is the delivery time.
type OnOrderDeliveredInput struct {
	OrderID      string
	RestaurantID int64
	BranchID     int64
	CountryCode  string
	Currency     string
	PlacedAt     string // ISO-8601 UTC
	DeliveredAt  string // ISO-8601 UTC
}

// OnOrderFailedInput is the parsed order.rejected / order.cancelled event.
// Kind selects which counter to bump.
type OnOrderFailedInput struct {
	OrderID      string
	RestaurantID int64
	BranchID     int64
	CountryCode  string
	Kind         FailureKind
	OccurredAt   string // ISO-8601 UTC
}

// OnPaymentCompletedInput is the parsed payment.completed event.
type OnPaymentCompletedInput struct {
	OrderID      string
	RestaurantID int64
	BranchID     int64
	Currency     string
	Amount       int64  // minor units
	CompletedAt  string // ISO-8601 UTC
}

// FailureKind discriminates the two failure counters.
type FailureKind string

const (
	FailureRejected  FailureKind = "rejected"
	FailureCancelled FailureKind = "cancelled"
)

// ---------------------------------------------------------------------------
// Read projections — what the service hands the controller. Derived fields
// (averages, sums) are computed here, never in the repo or controller.
// ---------------------------------------------------------------------------

// DayRow is the shared per-day revenue projection used by both the restaurant
// and branch "days" endpoints (identical shape).
type DayRow struct {
	Date          string
	OrdersCount   int64
	RevenueMinor  int64
	Currency      string
	AvgOrderMinor int64
}

// RestaurantDayRow is retained as an alias so existing callers keep compiling.
type RestaurantDayRow = DayRow

// PlatformDayRow is a per-day platform projection. CountryCode is empty when
// the row is aggregated across all countries.
type PlatformDayRow struct {
	Date          string
	CountryCode   string
	OrdersCount   int64
	RevenueMinor  int64
	Currency      string
	AvgOrderMinor int64
}

// ProductDayRow is a per-day, per-product projection.
type ProductDayRow struct {
	Date         string
	ProductID    int64
	UnitsCount   int64
	RevenueMinor int64
	OrdersCount  int64
	Currency     string
}

// FailureDayRow is a per-day failure projection (restaurant or platform).
type FailureDayRow struct {
	Date           string
	RejectedCount  int64
	CancelledCount int64
	FailedCount    int64 // rejected + cancelled
}

// DeliveryAvgDayRow is a per-day delivery-time projection. AvgDeliveryMs is 0
// on days with no deliveries.
type DeliveryAvgDayRow struct {
	Date            string
	DeliveredCount  int64
	AvgDeliveryMs   int64
	AvgDeliveryMins float64
}
