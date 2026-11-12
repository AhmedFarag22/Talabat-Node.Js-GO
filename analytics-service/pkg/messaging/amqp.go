package messaging

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// AMQPBroker is the rabbitmq implementation of Broker. One TCP connection,
// separate channels for publishing and consuming.
type AMQPBroker struct {
	url     string
	mu      sync.Mutex
	conn    *amqp.Connection
	channel *amqp.Channel
}

func NewAMQPBroker(url string) *AMQPBroker {
	return &AMQPBroker{url: url}
}

func (b *AMQPBroker) Connect(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.conn != nil && !b.conn.IsClosed() {
		return nil
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(10 * time.Second)
	}
	for {
		conn, err := amqp.Dial(b.url)
		if err == nil {
			ch, chErr := conn.Channel()
			if chErr != nil {
				_ = conn.Close()
				return fmt.Errorf("amqp open channel: %w", chErr)
			}
			b.conn = conn
			b.channel = ch
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("amqp dial: %w", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func (b *AMQPBroker) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	var firstErr error
	if b.channel != nil {
		if err := b.channel.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		b.channel = nil
	}
	if b.conn != nil {
		if err := b.conn.Close(); err != nil && firstErr == nil && !errors.Is(err, amqp.ErrClosed) {
			firstErr = err
		}
		b.conn = nil
	}
	return firstErr
}

// Subscribe asserts topology, opens a dedicated consumer channel with prefetch,
// and dispatches deliveries to handler on its own goroutine. ack/nack lifecycle
// is handled here — handler just returns nil/err.
func (b *AMQPBroker) Subscribe(ctx context.Context, opts ConsumerOpts, handler HandlerFunc) error {
	if err := b.Connect(ctx); err != nil {
		return err
	}

	if err := b.assertTopology(opts); err != nil {
		return err
	}

	consumerCh, err := b.conn.Channel()
	if err != nil {
		return fmt.Errorf("amqp consumer channel: %w", err)
	}
	if err := consumerCh.Qos(opts.Prefetch, 0, false); err != nil {
		_ = consumerCh.Close()
		return fmt.Errorf("amqp qos: %w", err)
	}

	deliveries, err := consumerCh.Consume(opts.Queue, "", false, false, false, false, nil)
	if err != nil {
		_ = consumerCh.Close()
		return fmt.Errorf("amqp consume: %w", err)
	}

	go b.dispatchLoop(ctx, consumerCh, deliveries, handler)
	return nil
}

func (b *AMQPBroker) dispatchLoop(ctx context.Context, ch *amqp.Channel, deliveries <-chan amqp.Delivery, handler HandlerFunc) {
	defer func() { _ = ch.Close() }()
	for {
		select {
		case <-ctx.Done():
			return
		case d, ok := <-deliveries:
			if !ok {
				return
			}
			err := handler(ctx, Delivery{RoutingKey: d.RoutingKey, Body: d.Body})
			if err != nil {
				_ = d.Nack(false, false)
				continue
			}
			_ = d.Ack(false)
		}
	}
}

// assertTopology declares the exchange, dead-letter (if configured), and queue
// with bindings. Idempotent — safe to call on every Subscribe.
func (b *AMQPBroker) assertTopology(opts ConsumerOpts) error {
	if err := b.channel.ExchangeDeclare(opts.Exchange, "topic", true, false, false, false, nil); err != nil {
		return fmt.Errorf("amqp exchange declare: %w", err)
	}

	args := amqp.Table{}
	if opts.DLX != "" {
		if err := b.channel.ExchangeDeclare(opts.DLX, "topic", true, false, false, false, nil); err != nil {
			return fmt.Errorf("amqp dlx declare: %w", err)
		}
		if opts.DLQ != "" {
			if _, err := b.channel.QueueDeclare(opts.DLQ, true, false, false, false, nil); err != nil {
				return fmt.Errorf("amqp dlq declare: %w", err)
			}
			if err := b.channel.QueueBind(opts.DLQ, "#", opts.DLX, false, nil); err != nil {
				return fmt.Errorf("amqp dlq bind: %w", err)
			}
		}
		args["x-dead-letter-exchange"] = opts.DLX
	}

	if _, err := b.channel.QueueDeclare(opts.Queue, true, false, false, false, args); err != nil {
		return fmt.Errorf("amqp queue declare: %w", err)
	}
	for _, key := range opts.BindingKeys {
		if err := b.channel.QueueBind(opts.Queue, key, opts.Exchange, false, nil); err != nil {
			return fmt.Errorf("amqp queue bind %q: %w", key, err)
		}
	}
	return nil
}
