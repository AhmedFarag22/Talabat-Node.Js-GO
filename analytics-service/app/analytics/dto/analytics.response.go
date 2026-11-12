package dto

// AnalyticsDayResponseDTO is the wire shape for one day row (restaurant or
// branch). Money is integer minor units (e.g. 1500 = 15.00 EGP) — formatting
// is the client's job.
type AnalyticsDayResponseDTO struct {
	Date          string `json:"date"`
	OrdersCount   int64  `json:"ordersCount"`
	RevenueMinor  int64  `json:"revenueMinor"`
	Currency      string `json:"currency"`
	AvgOrderMinor int64  `json:"avgOrderMinor"`
}

// PlatformDayResponseDTO is one platform day row. countryCode is empty when
// the figures are aggregated across all countries.
type PlatformDayResponseDTO struct {
	Date          string `json:"date"`
	CountryCode   string `json:"countryCode"`
	OrdersCount   int64  `json:"ordersCount"`
	RevenueMinor  int64  `json:"revenueMinor"`
	Currency      string `json:"currency"`
	AvgOrderMinor int64  `json:"avgOrderMinor"`
}

// ProductDayResponseDTO is one per-product day row.
type ProductDayResponseDTO struct {
	Date         string `json:"date"`
	ProductID    int64  `json:"productId"`
	UnitsCount   int64  `json:"unitsCount"`
	RevenueMinor int64  `json:"revenueMinor"`
	OrdersCount  int64  `json:"ordersCount"`
	Currency     string `json:"currency"`
}

// FailureDayResponseDTO is one day's failure counts.
type FailureDayResponseDTO struct {
	Date           string `json:"date"`
	RejectedCount  int64  `json:"rejectedCount"`
	CancelledCount int64  `json:"cancelledCount"`
	FailedCount    int64  `json:"failedCount"`
}

// DeliveryAvgDayResponseDTO is one day's average delivery time. Both the raw
// millisecond average and a minutes convenience value are returned.
type DeliveryAvgDayResponseDTO struct {
	Date            string  `json:"date"`
	DeliveredCount  int64   `json:"deliveredCount"`
	AvgDeliveryMs   int64   `json:"avgDeliveryMs"`
	AvgDeliveryMins float64 `json:"avgDeliveryMinutes"`
}

// ActiveRestaurantsResponseDTO is the single-object response for the
// active-restaurants endpoint.
type ActiveRestaurantsResponseDTO struct {
	Date              string `json:"date"`
	ActiveRestaurants int64  `json:"activeRestaurants"`
}
