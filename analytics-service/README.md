# analytics-service

Go + MongoDB + RabbitMQ. Reads `order.events` from RabbitMQ, upserts daily aggregates into Mongo, serves read-only HTTP endpoints under `/api/v1/analytics/...`.

This service is a **teaching artifact**: it builds one full vertical slice (one event consumer, one aggregate collection, one GET endpoint) end-to-end, demonstrating the layered architecture used across the QuickBite platform. The rest is documented as homework.

## Quick start

```sh
# 0. mongod + rabbitmq-server running locally (services.msc → check both are "Running")

cp .env.example .env
# edit .env to set ACCESS_SECRET / INTERNAL_API_KEY to match core/order

go mod download
go run ./cmd/api
# → "starting analytics-service"
# → "mongo connected"
# → "rabbit connected"
# → "event consumer started"
# → "http listening :4100"
```

`cmd/api/main.go` is intentionally tiny — it just calls `lib/boot.Run()`. All wiring lives in `lib/boot/boot.go`.

## Verify the slice end-to-end

The `play/` directory holds dev aids (gitignored — write your own when needed). The shipping examples:

```sh
# in three terminals:
go run ./play/mock-core          # fake core-service's RBAC endpoint
go run ./cmd/api                 # the service
go run ./play/publish-test -event=order.placed -restaurant=42 -total=2500

# verify mongo got the upsert
go run ./play/check-mongo -restaurant=42

# republish the same event (dedupe test — pass the eventId from above)
go run ./play/publish-test -event=order.placed -restaurant=42 -total=2500 -repeat-id=<eventId>
# → mongo doc unchanged

# hit the GET endpoint
JWT=$(go run ./play/mint-jwt)
curl -H "Cookie: access_token=$JWT" \
     "http://localhost:4100/api/v1/analytics/restaurants/42/days?from=2026-05-01&to=2026-05-31"
```

## What's in scope (built in the video)

- One vertical slice end-to-end: `order.placed` → `agg_restaurant_day` → `GET …/days`.
- All cross-cutting infra: config, logger, JWT auth, RBAC read-through cache, AMQP consumer with DLQ, idempotency via Mongo, error envelope, correlation IDs.

## What's homework

See [`docs/implementation-plan.md`](docs/implementation-plan.md). High level:

- 3 more aggregate collections (`agg_branch_day`, `agg_product_day`, `agg_platform_day`).
- 3 more event handlers (`payment.completed`, `order.delivered`, `order.rejected`).
- 7 more GET endpoints.
- `rbac.permissions_changed` consumer for cache invalidation.

## Files to read first

If you want to learn the codebase quickly, in this order:

1. [`CLAUDE.md`](CLAUDE.md) — the rule set this codebase obeys.
2. [`docs/node-to-go-mapping.md`](docs/node-to-go-mapping.md) — Node patterns → Go equivalents.
3. [`lib/boot/boot.go`](lib/boot/boot.go) — read top to bottom. No magic. Every wire visible.
4. [`app/analytics/service/analytics.service.go`](app/analytics/service/analytics.service.go) — the slice's business logic, ~60 lines.
5. [`lib/coreevents/consumer.go`](lib/coreevents/consumer.go) — generic consumer + dedupe.

## Why no ORM?

Same reason `order-service` uses Knex query builder instead of TypeORM/Prisma: we want explicit queries we can read and reason about. The Go equivalent is using the official `mongo-driver` directly — same philosophy, different language. See [`docs/node-to-go-mapping.md`](docs/node-to-go-mapping.md).

## Why no DI framework?

Constructor injection wired in `lib/boot.Run()` is explicit, debuggable, and short. tsyringe (used by `core-service`/`order-service`) earns its keep in TS because of decorators and reflection — Go doesn't have those, and faking them with libraries (wire, uber/dig, fx) buys nothing here. Read `lib/boot/boot.go` top to bottom: every singleton's birth is visible in one function.

## License

Internal training material — not for public distribution.
