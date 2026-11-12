# Implementation plan — phases

This is the build order. Each phase has an acceptance check; finish all of them before moving on. The video records phases 1–6 live; phases 7+ are homework.

---

## Phase 1 — Bootstrap

**Goal:** the binary boots, logs structured JSON, exits cleanly on Ctrl-C.

Files:
- `cmd/api/main.go` — ~10 lines, calls `lib/boot.Run()`.
- `lib/boot/boot.go` — skeleton: load env → log → wait on signal → exit.
- `lib/config/env.go`
- `lib/logger/logger.go`
- `.env.example`

**Acceptance:**
- `go run ./cmd/api` prints a JSON line `{"level":"INFO","msg":"starting analytics-service",...}`.
- `Ctrl-C` triggers `{"level":"INFO","msg":"shutdown complete"}`.

---

## Phase 2 — Mongo connection + indexes

**Goal:** Mongo connects on boot; indexes appear on a fresh DB.

Files:
- `pkg/mongo/client.go` — generic Connect, no app knowledge.
- `app/analytics/repository/indexes.go` — `EnsureIndexes` for this module's collections (`agg_restaurant_day` + `event_ids`).
- Extend `lib/boot.Run()`: after Connect, call `repository.EnsureIndexes(ctx, db)`.

**Acceptance:**
- Boot, then in `mongosh analytics`: `db.agg_restaurant_day.getIndexes()` shows the unique `(restaurant_id, date)` and the range-query `(date, restaurant_id)` indexes.
- `db.event_ids.getIndexes()` shows the unique `event_id` and the TTL on `received_at`.

---

## Phase 3 — RabbitMQ consumer skeleton

**Goal:** consumer subscribes to a queue and logs a line per message. No handler yet — just envelope decode.

Files:
- `pkg/messaging/types.go`
- `pkg/messaging/amqp.go`
- `lib/coreevents/consumer.go` (with no handlers map; just decode + ack)
- `lib/coreevents/payloads.go` (just `Envelope` + `EventHandler` type)
- Extend `lib/boot.Run()`: `broker := pkgmessaging.NewAMQPBroker(...)` + `coreevents.Start(...)`.

**Acceptance:**
- Boot the service.
- In a second terminal: `rabbitmqadmin publish exchange=order.events routing_key=order.placed payload='{"eventId":"a","eventType":"order.placed","payload":{}}'`
- Service logs `event consumer started` and then a line acknowledging the delivery.

---

## Phase 4 — Idempotency + handler dispatch

**Goal:** consumer dedupes via `event_ids`, dispatches to a per-event handler, no-handler events are ack-skipped.

Files:
- `app/analytics/repository/event_ids.repo.go`
- `app/analytics/eventhandlers/handlers.go` (empty map for now)
- Wire `EventIDsRepo` + `eventhandlers.For(svc)` into the consumer

**Acceptance:**
- Publishing the same `eventId` twice → second log line is `event: duplicate, ack-skip`.
- Publishing an unknown event type → log line is `event: no handler, ack-skip`.

---

## Phase 5 — One slice end-to-end (the consumer side)

**Goal:** `order.placed` updates an aggregate row.

Files:
- `app/analytics/{types,errors,enums}.go` (package `analytics`) — shared module types and errors.
- `app/analytics/entity/agg_restaurant_day.go`
- `app/analytics/repository/agg_restaurant_day.repo.go`
- `app/analytics/service/analytics.service.go`
- `app/analytics/eventhandlers/handlers.go` (register `order.placed` → `OnOrderPlaced`)

**Acceptance:**
- Publish `order.placed` for restaurant 42 with `total: 2500`.
- `mongosh: db.agg_restaurant_day.findOne({restaurant_id: 42})` shows `orders_count: 1`, `revenue_sum: 2500`.
- Publish three more events for the same restaurant on the same day → `orders_count: 4`, `revenue_sum` sums correctly.

---

## Phase 6 — One slice end-to-end (the HTTP side)

**Goal:** `GET /analytics/restaurants/:id/days` returns the aggregate.

Files:
- `lib/appcontext/context.go`
- `lib/auth/{jwt.go, middleware.go, apikey.go}`
- `lib/rbac/{cache.go, middleware.go}`
- `lib/coreclient/{client.go, rbac.go, types.go}`
- `lib/errors/{apperror.go, handler.go}`
- `lib/http/response.go`
- `lib/middleware/correlation.go`
- `app/analytics/dto/{analytics.request.go, analytics.response.go}`
- `app/analytics/controller/{analytics.controller.go, routes.go}`
- Extend `lib/boot.Run()`: build the chi router, mount `analytics.Mount`, start the HTTP server, graceful shutdown.

**Acceptance:**
- `curl http://localhost:4100/health` → 200 envelope.
- `curl …/api/v1/analytics/restaurants/42/days` → 401.
- With cookie + permission: 200 with the expected aggregate row.
- Without permission: 403.
- `from > to`: 400 + `ANALYTICS_INVALID_DATE_RANGE`.

---

## Phase 7 — Other aggregate collections (homework)

Mirror Phase 5 for: `agg_branch_day`, `agg_product_day`, `agg_platform_day`. Add their indexes to `app/analytics/repository/indexes.go`. Add their entities, repos, and service methods.

**Acceptance:** publishing one `order.placed` updates **all** four aggregates in a single Mongo trip per collection (no N+1 across collections). For `agg_product_day`, use `BulkWrite` since it's one upsert per line item.

---

## Phase 8 — Other event handlers (homework)

Implement `payment.completed`, `order.delivered`, `order.rejected`. Notes:

- `payment.completed`: should it update the same `agg_restaurant_day` row that `order.placed` already updated, or is it a separate metric? Decide and justify.
- `order.delivered`: contains a `deliveredAt` and a reference to the `placedAt` (or the original order id). You can compute `delivery_ms = deliveredAt - placedAt` and `$inc` it into `delivery_ms_sum` + `delivery_ms_count` on the existing row. Beware out-of-order delivery (delivered arrives before placed has been processed) — handle it.
- `order.rejected`: separate counter (`rejects_count`) on the same row; the `failures` endpoints derive from it.

---

## Phase 9 — Other endpoints (homework)

7 more endpoints, all GET, all read-only. Most are slight variations on `GetRestaurantDays`. The failure-rate and delivery-avg ones are derived from existing aggregate fields — no new collections.

---

## Phase 10 — RBAC cache invalidation (homework)

Wire a `rbac.permissions_changed` consumer that calls `rbacCache.Invalidate(role)`. Update `RABBITMQ_ORDER_EVENTS_BINDINGS` to include `rbac.#` (or rename it to a more general "all events" name). Decide whether to keep RBAC events on `order.events` or have core publish them on a separate `core.events` exchange — the latter is cleaner.

---

## Phase 11 — Backfill (bonus)

Write `cmd/backfill-aggs/main.go`: reads orders from `order-service`'s hot DB for a date range, replays them through the analytics handlers (or directly through the service). Idempotent (the `event_ids` dedupe protects you on re-runs).

---

## Acceptance check before recording the video

Phases 1–6 done, plus:

- [ ] `go build ./...` clean.
- [ ] `go vet ./...` clean.
- [ ] Boot with no env file: errors fail fast and identify the missing var.
- [ ] Boot with mongo down: clear "mongo connect" error message.
- [ ] Boot with rabbit down: warns on boot, retries; once rabbit comes back, consumer attaches.
- [ ] Publish 5 events with the same id: only one shows up in mongo.
- [ ] Publish 5 events with different ids: count = 5, revenue summed correctly.
- [ ] Hit the GET endpoint with no token (401), wrong perm (403), correct (200), bad range (400).
