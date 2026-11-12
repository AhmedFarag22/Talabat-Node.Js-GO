// Package messaging is a framework-free broker abstraction. The AMQP
// implementation lives next to it; the rest of the codebase only sees the
// interface and the small Delivery struct below.
package messaging

import "context"

// Delivery is a broker-agnostic message handed to a HandlerFunc.
type Delivery struct {
	RoutingKey string
	Body       []byte
}

// HandlerFunc consumes a single delivery. Returning nil → ack;
// returning an error → nack(no-requeue) → DLQ.
type HandlerFunc func(ctx context.Context, d Delivery) error

// ConsumerOpts describes the topology a subscriber needs.
type ConsumerOpts struct {
	Exchange    string
	Queue       string
	BindingKeys []string
	DLX         string
	DLQ         string
	Prefetch    int
}

// Broker is what the app sees. The AMQP impl is in this same package.
type Broker interface {
	// Connect opens the underlying connection. Idempotent.
	Connect(ctx context.Context) error
	// Subscribe registers a handler against the topology defined in opts.
	// Returns once the consumer is registered; messages flow on a goroutine
	// until ctx is cancelled or Close is called.
	Subscribe(ctx context.Context, opts ConsumerOpts, handler HandlerFunc) error
	// Close tears down the channel and connection. Safe to call multiple times.
	Close() error
}
