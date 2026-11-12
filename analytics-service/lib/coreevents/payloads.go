// Package coreevents holds the inbound event consumer (RabbitMQ) and the
// per-event payload shapes + handler type. Mirrors lib/core-events/* in
// order-service.
//
// Naming nit: in this service the events come from order-service, not core.
// We keep the package name "coreevents" anyway — the role of this directory
// is "external events that drive our state," exact parallel to the Node side.
package coreevents

import (
	"context"
	"encoding/json"
)

// EventHandler is what the consumer dispatches to. The handler map is built
// in app/analytics/eventhandlers and passed to Start — this package knows
// nothing about specific business modules.
type EventHandler func(ctx context.Context, payload json.RawMessage) error

// Envelope is the shape every event arrives in (matches order-service's
// outbox-drain emit).
type Envelope struct {
	EventID       string `json:"eventId"`
	EventType     string `json:"eventType"`
	OccurredAt    string `json:"occurredAt"`
	AggregateType string `json:"aggregateType"`
	AggregateID   string `json:"aggregateId"`
	Region        string `json:"region"`
	Payload       any    `json:"payload"`
}

// OrderPlacedPayload mirrors what order-service inserts into its outbox for
// EVENT_TYPES.ORDER_PLACED.
type OrderPlacedPayload struct {
	OrderID       string      `json:"orderId"`
	Region        string      `json:"region"`
	CountryCode   string      `json:"countryCode"`
	RestaurantID  int64       `json:"restaurantId"`
	BranchID      int64       `json:"branchId"`
	CustomerID    int64       `json:"customerId"`
	Status        string      `json:"status"`
	PaymentMethod string      `json:"paymentMethod"`
	Subtotal      int64       `json:"subtotal"`
	DeliveryFee   int64       `json:"deliveryFee"`
	ServiceFee    int64       `json:"serviceFee"`
	Total         int64       `json:"total"`
	Currency      string      `json:"currency"`
	Items         []OrderLine `json:"items"`
	PlacedAt      string      `json:"placedAt"`
}

// OrderLine is one line item of an order.placed event.
type OrderLine struct {
	ProductID int64 `json:"productId"`
	Quantity  int64 `json:"quantity"`
	UnitPrice int64 `json:"unitPrice"`
	LineTotal int64 `json:"lineTotal"`
}

// OrderDeliveredPayload mirrors order-service's order.delivered emit. countryCode
// and placedAt were added to the contract so analytics can attribute delivery
// time to the order's placed-day and slice by country (see docs/api-contracts).
type OrderDeliveredPayload struct {
	OrderID         string `json:"orderId"`
	Region          string `json:"region"`
	CountryCode     string `json:"countryCode"`
	RestaurantID    int64  `json:"restaurantId"`
	BranchID        int64  `json:"branchId"`
	CustomerID      int64  `json:"customerId"`
	DeliveryAgentID int64  `json:"deliveryAgentId"`
	Total           int64  `json:"total"`
	Subtotal        int64  `json:"subtotal"`
	DeliveryFee     int64  `json:"deliveryFee"`
	Commission      int64  `json:"commission"`
	Currency        string `json:"currency"`
	PaymentMethod   string `json:"paymentMethod"`
	PlacedAt        string `json:"placedAt"`
	DeliveredAt     string `json:"deliveredAt"`
}

// OrderTransitionPayload mirrors order-service's status-transition emit
// (order.accepted / order.rejected / order.cancelled). Analytics consumes the
// rejected and cancelled variants for failure counts.
type OrderTransitionPayload struct {
	OrderID      string `json:"orderId"`
	Region       string `json:"region"`
	CountryCode  string `json:"countryCode"`
	RestaurantID int64  `json:"restaurantId"`
	BranchID     int64  `json:"branchId"`
	CustomerID   int64  `json:"customerId"`
	Status       string `json:"status"`
	Reason       string `json:"reason"`
	Actor        string `json:"actor"`
	OccurredAt   string `json:"occurredAt"`
}

// PaymentCompletedPayload mirrors order-service's payment.completed emit.
type PaymentCompletedPayload struct {
	OrderID             string `json:"orderId"`
	Region              string `json:"region"`
	RestaurantID        int64  `json:"restaurantId"`
	BranchID            int64  `json:"branchId"`
	CustomerID          int64  `json:"customerId"`
	Provider            string `json:"provider"`
	ProviderReferenceID string `json:"providerReferenceId"`
	Amount              int64  `json:"amount"`
	Currency            string `json:"currency"`
	Method              string `json:"method"`
	CompletedAt         string `json:"completedAt"`
}
