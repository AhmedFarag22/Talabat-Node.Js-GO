// Package service holds the analytics business logic. Composes repositories
// and decides what the response shape should mean (e.g. avg = sum/count).
// Returns AppError. Knows nothing about HTTP or RabbitMQ.
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/quickbite/analytics-service/app/analytics"
	"github.com/quickbite/analytics-service/app/analytics/repository"
)

// AnalyticsService composes every aggregate repository. Event handlers fan one
// upstream event out to the restaurant / branch / platform / product rows;
// read methods project the stored sums into response rows.
type AnalyticsService struct {
	restaurantRepo *repository.AggRestaurantDayRepo
	branchRepo     *repository.AggBranchDayRepo
	productRepo    *repository.AggProductDayRepo
	platformRepo   *repository.AggPlatformDayRepo
}

func NewAnalyticsService(
	restaurantRepo *repository.AggRestaurantDayRepo,
	branchRepo *repository.AggBranchDayRepo,
	productRepo *repository.AggProductDayRepo,
	platformRepo *repository.AggPlatformDayRepo,
) *AnalyticsService {
	return &AnalyticsService{
		restaurantRepo: restaurantRepo,
		branchRepo:     branchRepo,
		productRepo:    productRepo,
		platformRepo:   platformRepo,
	}
}

const maxRangeDays = 366 * 24 * time.Hour

// ---------------------------------------------------------------------------
// Event handlers (write path)
// ---------------------------------------------------------------------------

// OnOrderPlaced merges one order.placed event into the restaurant, branch and
// platform day rows, plus one per-product row per line item. The day key is
// derived from placedAt in UTC so events from different shards agree on the
// same day boundary.
func (s *AnalyticsService) OnOrderPlaced(ctx context.Context, in analytics.OnOrderPlacedInput) error {
	date, err := dayKey(in.PlacedAt)
	if err != nil {
		return fmt.Errorf("order.placed placed_at: %w", err)
	}
	if err := s.restaurantRepo.UpsertOrderPlacedDelta(ctx, in.RestaurantID, date, in.Currency, in.Total); err != nil {
		return err
	}
	if err := s.branchRepo.UpsertOrderPlacedDelta(ctx, in.BranchID, date, in.Currency, in.Total); err != nil {
		return err
	}
	if err := s.platformRepo.UpsertOrderPlacedDelta(ctx, date, in.CountryCode, in.Currency, in.Total); err != nil {
		return err
	}
	if err := s.productRepo.BulkUpsertOrderLines(ctx, in.RestaurantID, date, in.Currency, in.Items); err != nil {
		return err
	}
	return nil
}

// OnOrderDelivered records delivery completion + delivery time on the same day
// row order.placed created (keyed by placed-day). Out-of-order delivery (event
// arrives before placed) is safe: the upsert creates the row, and a later
// placed event fills in revenue/currency.
func (s *AnalyticsService) OnOrderDelivered(ctx context.Context, in analytics.OnOrderDeliveredInput) error {
	date, err := dayKey(in.PlacedAt)
	if err != nil {
		return fmt.Errorf("order.delivered placed_at: %w", err)
	}
	placedAt, err := time.Parse(time.RFC3339, in.PlacedAt)
	if err != nil {
		return fmt.Errorf("order.delivered placed_at parse: %w", err)
	}
	deliveredAt, err := time.Parse(time.RFC3339, in.DeliveredAt)
	if err != nil {
		return fmt.Errorf("order.delivered delivered_at parse: %w", err)
	}
	deliveryMs := deliveredAt.Sub(placedAt).Milliseconds()
	if deliveryMs < 0 {
		deliveryMs = 0 // clock skew / out-of-order timestamps — don't poison the average
	}
	if err := s.restaurantRepo.UpsertDeliveredDelta(ctx, in.RestaurantID, date, in.Currency, deliveryMs); err != nil {
		return err
	}
	if err := s.branchRepo.UpsertDeliveredDelta(ctx, in.BranchID, date, in.Currency, deliveryMs); err != nil {
		return err
	}
	if err := s.platformRepo.UpsertDeliveredDelta(ctx, date, in.CountryCode, in.Currency, deliveryMs); err != nil {
		return err
	}
	return nil
}

// OnOrderFailed bumps the rejected/cancelled counter on the restaurant, branch
// and platform rows for the day the failure occurred.
func (s *AnalyticsService) OnOrderFailed(ctx context.Context, in analytics.OnOrderFailedInput) error {
	date, err := dayKey(in.OccurredAt)
	if err != nil {
		return fmt.Errorf("order failure occurred_at: %w", err)
	}
	field, err := failureField(in.Kind)
	if err != nil {
		return err
	}
	if err := s.restaurantRepo.UpsertFailureDelta(ctx, in.RestaurantID, date, field); err != nil {
		return err
	}
	if err := s.branchRepo.UpsertFailureDelta(ctx, in.BranchID, date, field); err != nil {
		return err
	}
	if err := s.platformRepo.UpsertFailureDelta(ctx, date, in.CountryCode, field); err != nil {
		return err
	}
	return nil
}

// OnPaymentCompleted records a captured online payment on the restaurant and
// branch rows. Tracked separately from revenue so it never double-counts the
// order total order.placed already booked.
func (s *AnalyticsService) OnPaymentCompleted(ctx context.Context, in analytics.OnPaymentCompletedInput) error {
	date, err := dayKey(in.CompletedAt)
	if err != nil {
		return fmt.Errorf("payment.completed completed_at: %w", err)
	}
	if err := s.restaurantRepo.UpsertPaymentDelta(ctx, in.RestaurantID, date, in.Currency, in.Amount); err != nil {
		return err
	}
	if err := s.branchRepo.UpsertPaymentDelta(ctx, in.BranchID, date, in.Currency, in.Amount); err != nil {
		return err
	}
	return nil
}

// ---------------------------------------------------------------------------
// Read methods
// ---------------------------------------------------------------------------

// GetRestaurantDays returns the per-day revenue rows for one restaurant.
func (s *AnalyticsService) GetRestaurantDays(ctx context.Context, restaurantID int64, from, to string) ([]analytics.DayRow, error) {
	if restaurantID <= 0 {
		return nil, analytics.ErrInvalidRestaurantID
	}
	if err := validateRange(from, to); err != nil {
		return nil, err
	}
	rows, err := s.restaurantRepo.FindRange(ctx, restaurantID, from, to)
	if err != nil {
		return nil, err
	}
	out := make([]analytics.DayRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, analytics.DayRow{
			Date:          r.Date,
			OrdersCount:   r.OrdersCount,
			RevenueMinor:  r.RevenueSum,
			Currency:      r.Currency,
			AvgOrderMinor: avg(r.RevenueSum, r.OrdersCount),
		})
	}
	return out, nil
}

// GetBranchDays returns the per-day revenue rows for one branch.
func (s *AnalyticsService) GetBranchDays(ctx context.Context, branchID int64, from, to string) ([]analytics.DayRow, error) {
	if branchID <= 0 {
		return nil, analytics.ErrInvalidBranchID
	}
	if err := validateRange(from, to); err != nil {
		return nil, err
	}
	rows, err := s.branchRepo.FindRange(ctx, branchID, from, to)
	if err != nil {
		return nil, err
	}
	out := make([]analytics.DayRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, analytics.DayRow{
			Date:          r.Date,
			OrdersCount:   r.OrdersCount,
			RevenueMinor:  r.RevenueSum,
			Currency:      r.Currency,
			AvgOrderMinor: avg(r.RevenueSum, r.OrdersCount),
		})
	}
	return out, nil
}

// GetProducts returns per-product day rows for a restaurant, optionally
// filtered to one product.
func (s *AnalyticsService) GetProducts(ctx context.Context, restaurantID int64, productID *int64, from, to string) ([]analytics.ProductDayRow, error) {
	if restaurantID <= 0 {
		return nil, analytics.ErrInvalidRestaurantID
	}
	if err := validateRange(from, to); err != nil {
		return nil, err
	}
	rows, err := s.productRepo.FindRange(ctx, restaurantID, productID, from, to)
	if err != nil {
		return nil, err
	}
	out := make([]analytics.ProductDayRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, analytics.ProductDayRow{
			Date:         r.Date,
			ProductID:    r.ProductID,
			UnitsCount:   r.UnitsSum,
			RevenueMinor: r.RevenueSum,
			OrdersCount:  r.OrdersCount,
			Currency:     r.Currency,
		})
	}
	return out, nil
}

// GetPlatformDays returns whole-platform per-day rows. countryCode == "" means
// aggregate across all countries.
func (s *AnalyticsService) GetPlatformDays(ctx context.Context, from, to, countryCode string) ([]analytics.PlatformDayRow, error) {
	if err := validateRange(from, to); err != nil {
		return nil, err
	}
	var country *string
	if countryCode != "" {
		country = &countryCode
	}
	rows, err := s.platformRepo.FindRange(ctx, from, to, country)
	if err != nil {
		return nil, err
	}
	out := make([]analytics.PlatformDayRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, analytics.PlatformDayRow{
			Date:          r.Date,
			CountryCode:   r.CountryCode,
			OrdersCount:   r.OrdersCount,
			RevenueMinor:  r.RevenueSum,
			Currency:      r.Currency,
			AvgOrderMinor: avg(r.RevenueSum, r.OrdersCount),
		})
	}
	return out, nil
}

// GetRestaurantFailures returns per-day failure counts for one restaurant.
func (s *AnalyticsService) GetRestaurantFailures(ctx context.Context, restaurantID int64, from, to string) ([]analytics.FailureDayRow, error) {
	if restaurantID <= 0 {
		return nil, analytics.ErrInvalidRestaurantID
	}
	if err := validateRange(from, to); err != nil {
		return nil, err
	}
	rows, err := s.restaurantRepo.FindRange(ctx, restaurantID, from, to)
	if err != nil {
		return nil, err
	}
	out := make([]analytics.FailureDayRow, 0, len(rows))
	for _, r := range rows {
		if r.RejectedCount == 0 && r.CancelledCount == 0 {
			continue // a pure-revenue day with no failures isn't a failure row
		}
		out = append(out, analytics.FailureDayRow{
			Date:           r.Date,
			RejectedCount:  r.RejectedCount,
			CancelledCount: r.CancelledCount,
			FailedCount:    r.RejectedCount + r.CancelledCount,
		})
	}
	return out, nil
}

// GetPlatformFailures returns per-day failure counts platform-wide (summed
// across countries).
func (s *AnalyticsService) GetPlatformFailures(ctx context.Context, from, to string) ([]analytics.FailureDayRow, error) {
	if err := validateRange(from, to); err != nil {
		return nil, err
	}
	rows, err := s.platformRepo.FindRange(ctx, from, to, nil)
	if err != nil {
		return nil, err
	}
	out := make([]analytics.FailureDayRow, 0, len(rows))
	for _, r := range rows {
		if r.RejectedCount == 0 && r.CancelledCount == 0 {
			continue
		}
		out = append(out, analytics.FailureDayRow{
			Date:           r.Date,
			RejectedCount:  r.RejectedCount,
			CancelledCount: r.CancelledCount,
			FailedCount:    r.RejectedCount + r.CancelledCount,
		})
	}
	return out, nil
}

// GetRestaurantDeliveryAvg returns per-day average delivery time for one
// restaurant, derived from delivery_ms_sum / delivery_ms_count.
func (s *AnalyticsService) GetRestaurantDeliveryAvg(ctx context.Context, restaurantID int64, from, to string) ([]analytics.DeliveryAvgDayRow, error) {
	if restaurantID <= 0 {
		return nil, analytics.ErrInvalidRestaurantID
	}
	if err := validateRange(from, to); err != nil {
		return nil, err
	}
	rows, err := s.restaurantRepo.FindRange(ctx, restaurantID, from, to)
	if err != nil {
		return nil, err
	}
	out := make([]analytics.DeliveryAvgDayRow, 0, len(rows))
	for _, r := range rows {
		if r.DeliveryMsCount == 0 {
			continue // no deliveries that day → not a delivery-avg row
		}
		avgMs := r.DeliveryMsSum / r.DeliveryMsCount
		out = append(out, analytics.DeliveryAvgDayRow{
			Date:            r.Date,
			DeliveredCount:  r.DeliveredCount,
			AvgDeliveryMs:   avgMs,
			AvgDeliveryMins: float64(avgMs) / 60000.0,
		})
	}
	return out, nil
}

// GetActiveRestaurants returns the number of restaurants that placed at least
// one order on the given day.
func (s *AnalyticsService) GetActiveRestaurants(ctx context.Context, date string) (int64, error) {
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return 0, analytics.ErrInvalidDateRange
	}
	return s.restaurantRepo.CountActiveOnDate(ctx, date)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// dayKey derives the "YYYY-MM-DD" UTC bucket from an RFC3339 timestamp.
func dayKey(ts string) (string, error) {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return "", fmt.Errorf("parse timestamp %q: %w", ts, err)
	}
	return t.UTC().Format("2006-01-02"), nil
}

// validateRange enforces from/to parse as dates, to >= from, and the span is
// within one year. Returns the domain ErrInvalidDateRange on any violation.
func validateRange(from, to string) error {
	fromT, err := time.Parse("2006-01-02", from)
	if err != nil {
		return analytics.ErrInvalidDateRange
	}
	toT, err := time.Parse("2006-01-02", to)
	if err != nil {
		return analytics.ErrInvalidDateRange
	}
	if toT.Before(fromT) || toT.Sub(fromT) > maxRangeDays {
		return analytics.ErrInvalidDateRange
	}
	return nil
}

func failureField(kind analytics.FailureKind) (string, error) {
	switch kind {
	case analytics.FailureRejected:
		return "rejected_count", nil
	case analytics.FailureCancelled:
		return "cancelled_count", nil
	default:
		return "", fmt.Errorf("unknown failure kind %q", kind)
	}
}

func avg(sum, count int64) int64 {
	if count <= 0 {
		return 0
	}
	return sum / count
}
