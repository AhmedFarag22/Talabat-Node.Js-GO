// Package eventhandlers maps inbound RabbitMQ event types onto analytics
// service calls. Sits in the app layer so lib/coreevents stays
// business-agnostic.
package eventhandlers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/quickbite/analytics-service/app/analytics"
	"github.com/quickbite/analytics-service/app/analytics/service"
	"github.com/quickbite/analytics-service/lib/coreevents"
)

// For builds the routing table the consumer uses. To register a new event:
//  1. Add a typed payload struct in lib/coreevents/payloads.go.
//  2. Add a handle*** function below.
//  3. Add it to the map.
//
// The consumer treats unknown event types as ack-and-skip.
func For(svc *service.AnalyticsService) map[string]coreevents.EventHandler {
	return map[string]coreevents.EventHandler{
		"order.placed":      handleOrderPlaced(svc),
		"order.delivered":   handleOrderDelivered(svc),
		"order.rejected":    handleOrderFailed(svc, analytics.FailureRejected),
		"order.cancelled":   handleOrderFailed(svc, analytics.FailureCancelled),
		"payment.completed": handlePaymentCompleted(svc),
	}
}

func handleOrderPlaced(svc *service.AnalyticsService) coreevents.EventHandler {
	return func(ctx context.Context, raw json.RawMessage) error {
		var p coreevents.OrderPlacedPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			return fmt.Errorf("decode order.placed payload: %w", err)
		}
		items := make([]analytics.OrderLineInput, 0, len(p.Items))
		for _, it := range p.Items {
			items = append(items, analytics.OrderLineInput{
				ProductID: it.ProductID,
				Quantity:  it.Quantity,
				UnitPrice: it.UnitPrice,
				LineTotal: it.LineTotal,
			})
		}
		return svc.OnOrderPlaced(ctx, analytics.OnOrderPlacedInput{
			OrderID:      p.OrderID,
			RestaurantID: p.RestaurantID,
			BranchID:     p.BranchID,
			CountryCode:  p.CountryCode,
			Currency:     p.Currency,
			Total:        p.Total,
			PlacedAt:     p.PlacedAt,
			Items:        items,
		})
	}
}

func handleOrderDelivered(svc *service.AnalyticsService) coreevents.EventHandler {
	return func(ctx context.Context, raw json.RawMessage) error {
		var p coreevents.OrderDeliveredPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			return fmt.Errorf("decode order.delivered payload: %w", err)
		}
		return svc.OnOrderDelivered(ctx, analytics.OnOrderDeliveredInput{
			OrderID:      p.OrderID,
			RestaurantID: p.RestaurantID,
			BranchID:     p.BranchID,
			CountryCode:  p.CountryCode,
			Currency:     p.Currency,
			PlacedAt:     p.PlacedAt,
			DeliveredAt:  p.DeliveredAt,
		})
	}
}

func handleOrderFailed(svc *service.AnalyticsService, kind analytics.FailureKind) coreevents.EventHandler {
	return func(ctx context.Context, raw json.RawMessage) error {
		var p coreevents.OrderTransitionPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			return fmt.Errorf("decode order transition payload: %w", err)
		}
		return svc.OnOrderFailed(ctx, analytics.OnOrderFailedInput{
			OrderID:      p.OrderID,
			RestaurantID: p.RestaurantID,
			BranchID:     p.BranchID,
			CountryCode:  p.CountryCode,
			Kind:         kind,
			OccurredAt:   p.OccurredAt,
		})
	}
}

func handlePaymentCompleted(svc *service.AnalyticsService) coreevents.EventHandler {
	return func(ctx context.Context, raw json.RawMessage) error {
		var p coreevents.PaymentCompletedPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			return fmt.Errorf("decode payment.completed payload: %w", err)
		}
		return svc.OnPaymentCompleted(ctx, analytics.OnPaymentCompletedInput{
			OrderID:      p.OrderID,
			RestaurantID: p.RestaurantID,
			BranchID:     p.BranchID,
			Currency:     p.Currency,
			Amount:       p.Amount,
			CompletedAt:  p.CompletedAt,
		})
	}
}
