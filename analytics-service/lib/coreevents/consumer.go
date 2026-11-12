package coreevents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/quickbite/analytics-service/pkg/messaging"
)

// EventDeduper is the small interface the consumer needs for idempotency.
// app/analytics/repository.EventIDsRepo satisfies it implicitly — Go's duck
// typing means lib doesn't import app to use it.
type EventDeduper interface {
	MarkSeen(ctx context.Context, eventID string) (bool, error)
}

// Start subscribes the consumer queue and dispatches incoming envelopes to the
// per-event handler. Returns once the subscription is registered; messages
// flow on a goroutine until ctx is cancelled.
//
// Idempotency: every event is funnelled through the deduper before the
// handler runs. Duplicates are acked-and-skipped.
func Start(
	ctx context.Context,
	logger *slog.Logger,
	broker messaging.Broker,
	opts messaging.ConsumerOpts,
	handlers map[string]EventHandler,
	deduper EventDeduper,
) error {
	dispatch := func(ctx context.Context, d messaging.Delivery) error {
		var env Envelope
		if err := json.Unmarshal(d.Body, &env); err != nil {
			logger.Error("event: malformed envelope, sending to DLQ",
				slog.String("routing_key", d.RoutingKey),
				slog.String("error", err.Error()),
			)
			return err
		}

		if env.EventID == "" || env.EventType == "" {
			logger.Error("event: missing eventId or eventType, sending to DLQ",
				slog.String("routing_key", d.RoutingKey),
			)
			return errors.New("invalid envelope")
		}

		fresh, err := deduper.MarkSeen(ctx, env.EventID)
		if err != nil {
			return fmt.Errorf("dedupe write: %w", err)
		}
		if !fresh {
			logger.Debug("event: duplicate, ack-skip",
				slog.String("event_id", env.EventID),
				slog.String("event_type", env.EventType),
			)
			return nil
		}

		handler, ok := handlers[env.EventType] // product.changed -> handleProductChanged()
		if !ok {
			logger.Warn("event: no handler, ack-skip",
				slog.String("event_type", env.EventType),
				slog.String("event_id", env.EventID),
			)
			return nil
		}

		raw, err := json.Marshal(env.Payload)
		if err != nil {
			return fmt.Errorf("re-marshal payload: %w", err)
		}
		if err := handler(ctx, raw); err != nil {
			logger.Error("event: handler failed, sending to DLQ",
				slog.String("event_type", env.EventType),
				slog.String("event_id", env.EventID),
				slog.String("error", err.Error()),
			)
			return err
		}

		logger.Info("event: handled",
			slog.String("event_type", env.EventType),
			slog.String("event_id", env.EventID),
		)
		return nil
	}

	if err := broker.Subscribe(ctx, opts, dispatch); err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}
	logger.Info("event consumer started",
		slog.String("queue", opts.Queue),
		slog.Any("bindings", opts.BindingKeys),
	)
	return nil
}
