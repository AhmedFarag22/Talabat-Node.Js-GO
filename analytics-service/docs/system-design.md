# System design

## Where this service sits

```
                                ┌────────────────────────┐
                                │     core-service       │  (Node + Postgres + Redis)
                                │   users, restaurants,  │
                                │   products, RBAC       │
                                └─────────┬──────────────┘
                                          │ HTTP (sync)
                                          │ ── GET /api/internal/rbac/permissions
                                          │
┌────────────────┐  HTTP   ┌──────────────▼────────────┐
│   clients      │────────▶│      order-service        │  (Node + sharded Postgres + Redis)
│  (web, mobile) │         │  orders, payments, deliv. │
└────────────────┘         └─────────┬─────────────────┘
                                     │ outbox drain → publish
                                     │ exchange: order.events
                                     ▼
                            ┌────────────────┐
                            │   RabbitMQ     │  topic: order.events  (order.#, payment.#)
                            └────────┬───────┘
                                     │ subscribe
                                     │ queue: analytics-service.order-events
                                     ▼
                            ┌──────────────────────────┐
                            │   analytics-service      │  (Go + MongoDB)
                            │  agg_*_day collections   │
                            └──────────────────────────┘
                                     ▲
                                     │ HTTP (sync)
                                     │ GET /api/v1/analytics/...
                                     │ + auth cookie + rbac
                                     │
                            ┌────────────────┐
                            │ dashboards     │
                            └────────────────┘
```

## Why these tech choices

| Concern         | Choice               | Why                                                                   |
| --------------- | -------------------- | --------------------------------------------------------------------- |
| Language        | Go                   | Demonstrates polyglot architecture; same layered patterns translate.   |
| DB              | MongoDB              | Aggregate documents fit per-day rollups naturally; no joins needed.    |
| Messaging       | RabbitMQ (existing)  | Already used by core/order; reuse the broker, just a new exchange.    |
| Cache           | none (in-process)    | Slice doesn't need Redis. RBAC cache is a tiny in-process map.        |
| ORM             | none                 | mongo-driver direct; same philosophy as order-service's `knex` choice. |

## Sync flow: HTTP read

1. Client hits `GET /api/v1/analytics/restaurants/:id/days?from=&to=` with the access cookie issued by core/order.
2. `lib/middleware.Correlation` attaches an `X-Correlation-ID` (generates one if missing).
3. `lib/auth.Authenticate` parses the JWT, attaches `Claims` to `context.Context`. Returns 401 if the token is missing/invalid.
4. `lib/rbac.Require("analytics:read")` looks up the role's permissions via `lib/rbac.Cache` (TTL 5min). On miss, calls `core-service`'s `GET /api/internal/rbac/permissions?role=...`. Returns 403 if the permission isn't in the list.
5. `controller.GetRestaurantDays` validates query params (`from`, `to`), calls the service.
6. Service computes the date keys, asks the repo to range-query Mongo.
7. Repo returns entities; service maps to `RestaurantDayRow` (with derived `avg_order_minor`).
8. Controller maps `RestaurantDayRow` → `AnalyticsDayResponseDTO`, calls `apphttp.SendSuccess`.

Performance budget: **< 200ms p95**. The (`date`, `restaurant_id`) index makes the query an indexed range scan; expected output rows = days in range, typically ≤ 31.

## Async flow: order placed → aggregate

1. **Order side** (in `order-service`):
   - Customer places order.
   - `OrderService.placeOrder` opens a Postgres trx in the customer's region.
   - Inserts `orders` + `order_items` rows.
   - Inserts a row into `events_outbox` with `event_type = "order.placed"` and the JSON payload — **same trx**.
   - Commits.
   - A separate per-region cron job (`registerOutboxDrainJobs`, every 2s) runs `drainOutboxForRegion`:
     - `SELECT … FROM events_outbox WHERE dispatched_at IS NULL ORDER BY id LIMIT N FOR UPDATE SKIP LOCKED`.
     - For each row: publish to `order.events` with `routing_key = event_type`, persistent.
     - On success: `UPDATE … SET dispatched_at = NOW()`.
     - On publish error: `UPDATE … SET attempts = attempts + 1, last_error = ?` and bail out of the batch.

2. **Analytics side** (this service):
   - `lib/coreevents.Start` subscribed to `analytics-service.order-events` queue (durable, bound to `order.#` and `payment.#`).
   - Each delivery:
     - Decode the `Envelope` (eventId, eventType, payload).
     - `EventIDsRepo.MarkSeen(eventID)` — Mongo `InsertOne`. Unique index guarantees the second call returns `false` (duplicate).
     - If duplicate: `ack-and-skip`. Done.
     - Look up the handler in `eventhandlers.For()` map by event type. Unknown type → log + ack-skip.
     - Run handler. On success: `ack`. On error: `nack(no-requeue)` → broker routes to DLQ.

3. **Inside the order.placed handler**:
   - Decode the typed payload (`OrderPlacedPayload`).
   - Service `OnOrderPlaced` parses the UTC date from `placedAt`.
   - Repo `UpsertOrderPlacedDelta`: `UpdateOne` with `{$inc: {orders_count: 1, revenue_sum: total}}` and `upsert: true`. Atomic.

### Failure modes

| Failure                                  | What happens                                                                                                         |
| ---------------------------------------- | -------------------------------------------------------------------------------------------------------------------- |
| Order saved, broker down                 | Outbox row stays `dispatched_at IS NULL`. Drainer retries on next tick.                                              |
| Drainer publishes, then dies before mark | Next drainer pass re-publishes (we wrote to the broker twice). Analytics dedupes via `event_ids` — net effect = once. |
| Analytics processes, then crashes before ack | Broker redelivers. `MarkSeen` returns `false` (already inserted). Ack-skip.                                       |
| Malformed envelope                       | DLQ (no auto-replay). Ops investigates.                                                                              |
| Handler throws                           | DLQ. Same as above. Bug in handler code — replay after fix.                                                          |
| Same event id, two different payloads    | Treat as a producer bug. The first wins. Should never happen because `event_id` is generated per outbox insert.       |

## Idempotency in detail

We chose Mongo `event_ids` over Redis SETNX (which `order-service` uses for inbound dedupe) because:
- This service has no Redis dep yet — adding one for one purpose is overkill.
- Mongo gives us TTL natively (`SetExpireAfterSeconds`) — same eviction story as Redis.
- Unique index + `mongo.IsDuplicateKeyError` is one round-trip — same cost as SETNX.
- Survives Mongo restarts (the events_outbox on the producer side has no TTL — durable).

7-day TTL on `received_at`. Past that, an operator-driven DLQ replay would re-process; that's intentional — long-tail dedupe is operations territory, not silent automatic.

## RBAC

This service holds **no permission catalog**. It asks core-service.

- `Require("analytics:read")` middleware → `Cache.Get("system_admin")` → on miss → `coreclient.GetPermissionsForRole("system_admin")` → JSON GET to `core` with `api-key` header → response cached for `RBAC_CACHE_TTL_SEC`.
- The permission `analytics:read` must exist in core's seeded RBAC catalog. The seeding migration is in `core-service/src/migrations/...`.

### Cache invalidation (homework)

The video does not wire `rbac.permissions_changed`. Without it, a permission change in core takes up to TTL seconds (5 min by default) to take effect here. Acceptable for the slice; the homework adds the consumer that calls `cache.Invalidate(role)` for sub-second propagation.

## What's intentionally missing

- No outbound events from this service.
- No write endpoints.
- No background jobs other than the consumer goroutine.
- No Redis.
- No multi-region sharding (analytics is small — we'd shard if traffic justified it).
- No `lib/jobs/scheduler` infrastructure (no scheduled work in the slice).
