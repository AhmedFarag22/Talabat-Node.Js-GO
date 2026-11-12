// Package controller turns HTTP into service calls and back. Validates query
// params, calls the service, maps to DTOs, sends the envelope.
package controller

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	"github.com/quickbite/analytics-service/app/analytics"
	"github.com/quickbite/analytics-service/app/analytics/dto"
	"github.com/quickbite/analytics-service/app/analytics/service"
	apperr "github.com/quickbite/analytics-service/lib/errors"
	apphttp "github.com/quickbite/analytics-service/lib/http"
)

type AnalyticsController struct {
	svc       *service.AnalyticsService
	validator *validator.Validate
}

func NewAnalyticsController(svc *service.AnalyticsService, v *validator.Validate) *AnalyticsController {
	return &AnalyticsController{svc: svc, validator: v}
}

// GetRestaurantDays handles GET /analytics/restaurants/:restaurantId/days?from=&to=.
func (c *AnalyticsController) GetRestaurantDays(w http.ResponseWriter, r *http.Request) error {
	restaurantID, err := pathInt(r, "restaurantId")
	if err != nil {
		return analytics.ErrInvalidRestaurantID
	}
	q, err := c.dateRange(r)
	if err != nil {
		return err
	}
	rows, err := c.svc.GetRestaurantDays(r.Context(), restaurantID, q.From, q.To)
	if err != nil {
		return err
	}
	apphttp.SendSuccess(w, http.StatusOK, toDayDTOs(rows))
	return nil
}

// GetBranchDays handles GET /analytics/branches/:branchId/days?from=&to=.
func (c *AnalyticsController) GetBranchDays(w http.ResponseWriter, r *http.Request) error {
	branchID, err := pathInt(r, "branchId")
	if err != nil {
		return analytics.ErrInvalidBranchID
	}
	q, err := c.dateRange(r)
	if err != nil {
		return err
	}
	rows, err := c.svc.GetBranchDays(r.Context(), branchID, q.From, q.To)
	if err != nil {
		return err
	}
	apphttp.SendSuccess(w, http.StatusOK, toDayDTOs(rows))
	return nil
}

// GetProducts handles GET /analytics/products?restaurantId=&productId=&from=&to=.
func (c *AnalyticsController) GetProducts(w http.ResponseWriter, r *http.Request) error {
	restaurantID, err := strconv.ParseInt(r.URL.Query().Get("restaurantId"), 10, 64)
	if err != nil {
		return analytics.ErrInvalidRestaurantID
	}
	var productID *int64
	if raw := r.URL.Query().Get("productId"); raw != "" {
		pid, perr := strconv.ParseInt(raw, 10, 64)
		if perr != nil {
			return apperr.New("VALIDATION_ERROR", http.StatusBadRequest, "invalid productId")
		}
		productID = &pid
	}
	q := dto.GetProductsQuery{
		RestaurantID: restaurantID,
		ProductID:    productID,
		From:         r.URL.Query().Get("from"),
		To:           r.URL.Query().Get("to"),
	}
	if err := c.validator.Struct(q); err != nil {
		return apperr.New("VALIDATION_ERROR", http.StatusBadRequest, "invalid query: "+err.Error())
	}

	rows, err := c.svc.GetProducts(r.Context(), q.RestaurantID, q.ProductID, q.From, q.To)
	if err != nil {
		return err
	}
	out := make([]dto.ProductDayResponseDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, dto.ProductDayResponseDTO{
			Date:         row.Date,
			ProductID:    row.ProductID,
			UnitsCount:   row.UnitsCount,
			RevenueMinor: row.RevenueMinor,
			OrdersCount:  row.OrdersCount,
			Currency:     row.Currency,
		})
	}
	apphttp.SendSuccess(w, http.StatusOK, out)
	return nil
}

// GetPlatformDays handles GET /analytics/platform/days?from=&to=&country_code=.
func (c *AnalyticsController) GetPlatformDays(w http.ResponseWriter, r *http.Request) error {
	q := dto.GetPlatformDaysQuery{
		From:        r.URL.Query().Get("from"),
		To:          r.URL.Query().Get("to"),
		CountryCode: r.URL.Query().Get("country_code"),
	}
	if err := c.validator.Struct(q); err != nil {
		return apperr.New("VALIDATION_ERROR", http.StatusBadRequest, "invalid query: "+err.Error())
	}
	rows, err := c.svc.GetPlatformDays(r.Context(), q.From, q.To, q.CountryCode)
	if err != nil {
		return err
	}
	out := make([]dto.PlatformDayResponseDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, dto.PlatformDayResponseDTO{
			Date:          row.Date,
			CountryCode:   row.CountryCode,
			OrdersCount:   row.OrdersCount,
			RevenueMinor:  row.RevenueMinor,
			Currency:      row.Currency,
			AvgOrderMinor: row.AvgOrderMinor,
		})
	}
	apphttp.SendSuccess(w, http.StatusOK, out)
	return nil
}

// GetRestaurantFailures handles GET /analytics/restaurants/:restaurantId/failures.
func (c *AnalyticsController) GetRestaurantFailures(w http.ResponseWriter, r *http.Request) error {
	restaurantID, err := pathInt(r, "restaurantId")
	if err != nil {
		return analytics.ErrInvalidRestaurantID
	}
	q, err := c.dateRange(r)
	if err != nil {
		return err
	}
	rows, err := c.svc.GetRestaurantFailures(r.Context(), restaurantID, q.From, q.To)
	if err != nil {
		return err
	}
	apphttp.SendSuccess(w, http.StatusOK, toFailureDTOs(rows))
	return nil
}

// GetPlatformFailures handles GET /analytics/platform/failures.
func (c *AnalyticsController) GetPlatformFailures(w http.ResponseWriter, r *http.Request) error {
	q, err := c.dateRange(r)
	if err != nil {
		return err
	}
	rows, err := c.svc.GetPlatformFailures(r.Context(), q.From, q.To)
	if err != nil {
		return err
	}
	apphttp.SendSuccess(w, http.StatusOK, toFailureDTOs(rows))
	return nil
}

// GetRestaurantDeliveryAvg handles GET /analytics/restaurants/:restaurantId/delivery-avg.
func (c *AnalyticsController) GetRestaurantDeliveryAvg(w http.ResponseWriter, r *http.Request) error {
	restaurantID, err := pathInt(r, "restaurantId")
	if err != nil {
		return analytics.ErrInvalidRestaurantID
	}
	q, err := c.dateRange(r)
	if err != nil {
		return err
	}
	rows, err := c.svc.GetRestaurantDeliveryAvg(r.Context(), restaurantID, q.From, q.To)
	if err != nil {
		return err
	}
	out := make([]dto.DeliveryAvgDayResponseDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, dto.DeliveryAvgDayResponseDTO{
			Date:            row.Date,
			DeliveredCount:  row.DeliveredCount,
			AvgDeliveryMs:   row.AvgDeliveryMs,
			AvgDeliveryMins: row.AvgDeliveryMins,
		})
	}
	apphttp.SendSuccess(w, http.StatusOK, out)
	return nil
}

// GetActiveRestaurants handles GET /analytics/platform/active-restaurants?date=.
func (c *AnalyticsController) GetActiveRestaurants(w http.ResponseWriter, r *http.Request) error {
	q := dto.GetActiveRestaurantsQuery{Date: r.URL.Query().Get("date")}
	if err := c.validator.Struct(q); err != nil {
		return apperr.New("VALIDATION_ERROR", http.StatusBadRequest, "invalid query: "+err.Error())
	}
	count, err := c.svc.GetActiveRestaurants(r.Context(), q.Date)
	if err != nil {
		return err
	}
	apphttp.SendSuccess(w, http.StatusOK, dto.ActiveRestaurantsResponseDTO{
		Date:              q.Date,
		ActiveRestaurants: count,
	})
	return nil
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// dateRange parses + validates the shared from/to query DTO.
func (c *AnalyticsController) dateRange(r *http.Request) (dto.DateRangeQuery, error) {
	q := dto.DateRangeQuery{
		From: r.URL.Query().Get("from"),
		To:   r.URL.Query().Get("to"),
	}
	if err := c.validator.Struct(q); err != nil {
		return q, apperr.New("VALIDATION_ERROR", http.StatusBadRequest, "invalid query: "+err.Error())
	}
	return q, nil
}

func pathInt(r *http.Request, name string) (int64, error) {
	v, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	if err != nil || v <= 0 {
		return 0, strconv.ErrSyntax
	}
	return v, nil
}

func toDayDTOs(rows []analytics.DayRow) []dto.AnalyticsDayResponseDTO {
	out := make([]dto.AnalyticsDayResponseDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, dto.AnalyticsDayResponseDTO{
			Date:          row.Date,
			OrdersCount:   row.OrdersCount,
			RevenueMinor:  row.RevenueMinor,
			Currency:      row.Currency,
			AvgOrderMinor: row.AvgOrderMinor,
		})
	}
	return out
}

func toFailureDTOs(rows []analytics.FailureDayRow) []dto.FailureDayResponseDTO {
	out := make([]dto.FailureDayResponseDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, dto.FailureDayResponseDTO{
			Date:           row.Date,
			RejectedCount:  row.RejectedCount,
			CancelledCount: row.CancelledCount,
			FailedCount:    row.FailedCount,
		})
	}
	return out
}
