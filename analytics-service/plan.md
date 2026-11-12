# Analytics Service (Service C) — Minimal Build Plan

Go + MongoDB + RabbitMQ. Same layered architecture as `order-service` (`pkg` / `lib` / `app` + `cmd` for the Go entrypoint). The goal of this service is to be a **teaching artifact**: one vertical slice end-to-end, recorded in a single 1–2 hr video. The rest is homework.

---

## 1. Scope of this build

### In the video (build live)

- Bootstrap: `cmd/api/main.go`, config, logger, graceful shutdown.
- Mongo connection + `EnsureIndexes` on boot (no migrations — see §3).
- RabbitMQ consumer subscribed to `core.events`-style exchange from order-service.
- **One** aggregate: `agg_restaurant_day` (collection).
- **One** event handler: `order_placed` → upsert into `agg_restaurant_day`.
- Idempotency: Mongo `event_ids` collection with unique index + TTL.
- Auth middleware (JWT verify, same secret/shape as core & order).
- RBAC middleware (read-through cache from `core-service` via HTTP).
- **One** GET endpoint: `GET /analytics/restaurants/:restaurantId/days?from=&to=`.
- Error handling, response envelope, request context plumbing.

### Homework (not in the video)

- 3 more aggregates: `agg_branch_day`, `agg_product_day`, `agg_platform_day`.
- 3 more events: `payment_completed`, `order_delivered`, `order_rejected`.
- 7 more endpoints (failures, delivery-avg, active-restaurants, …).
- `rbac.permissions_changed` consumer for cache invalidation. (Auth/RBAC read-through itself **is** in the video — only the invalidation event handler is HW.)
- DLQ replay tooling beyond what `amqp091-go` gives for free.
- Backfill scripts.

---

## 2. Tech stack (locked)

| Concern        | Choice                                               | Notes |
| -------------- | ---------------------------------------------------- | ----- |
| Runtime        | Go 1.22+                                             | |
| HTTP router    | `chi`                                                | minimal, idiomatic, middleware-friendly |
| DB             | MongoDB driver `go.mongodb.org/mongo-driver/v2`      | low-level; no ODM (mirrors "knex not ORM") |
| Messaging      | `github.com/rabbitmq/amqp091-go`                     | |
| HTTP client    | stdlib `net/http`                                    | |
| Config         | `github.com/caarlos0/env/v11` + struct tags          | mirrors zod env parsing |
| Logger         | stdlib `log/slog`                                    | |
| Validation     | `github.com/go-playground/validator/v10`             | tag-based, mirrors class-validator |
| JWT            | `github.com/golang-jwt/jwt/v5`                       | same secret/shape as core, order |
| UUID           | `github.com/google/uuid`                             | |
| DI             | **No framework.** Constructor wiring in `main.go`.   | |

**Forbidden:** GORM, Ent, any ODM. Repositories use the Mongo driver directly with typed structs and `bson` tags. This is the Go analogue of "Knex query builder, not an ORM."

---

## 3. No SQL migrations — but indexes are mandatory

Mongo is schemaless, so we skip `migrate-mongo` / `goose`. Instead:

- `pkg/mongo/indexes.go` declares every index for every collection in one place.
- `EnsureIndexes(ctx)` runs on startup. `CreateIndexes` is idempotent — safe to re-run.
- If a doc shape ever changes, write a one-shot backfill under `cmd/backfill-<name>/` (HW territory).

Indexes for the video slice:

```go
// agg_restaurant_day
{ Keys: bson.D{{"restaurant_id", 1}, {"date", 1}}, Options: options.Index().SetUnique(true) }
{ Keys: bson.D{{"date", 1}, {"restaurant_id", 1}} } // for range queries

// event_ids (idempotency)
{ Keys: bson.D{{"event_id", 1}}, Options: options.Index().SetUnique(true) }
{ Keys: bson.D{{"received_at", 1}}, Options: options.Index().SetExpireAfterSeconds(7*24*3600) }
```

---

## 4. Folder structure

```
analytics-service/
  cmd/
    api/
      main.go                 # bootstrap: config → logger → mongo → broker → router → http server
  pkg/                        # framework-free, no imports from lib/ or app/
    mongo/
      client.go               # Connect / Disconnect, returns *mongo.Database
      indexes.go              # EnsureIndexes(ctx, db) — declares all indexes
    messaging/
      broker.go               # Broker interface (Subscribe, Publish, Close)
      amqp.go                 # amqp091 implementation
      types.go                # Delivery, HandlerFunc
    httpclient/
      client.go               # tiny wrapper: timeouts, JSON, retry-on-5xx
  lib/                        # app-aware glue: depends on pkg/, env
    config/
      env.go                  # struct + Load() with env.Parse
    logger/
      logger.go               # slog handler + helpers (WithCorrelationID)
    appcontext/
      context.go              # ctx keys + With/Get for UserClaims, CorrelationID
    errors/
      apperror.go             # AppError{Code, Status, Message}
      handler.go              # chi middleware: recover + map errors → JSON
    http/
      response.go             # SendSuccess, SendError, SendPaginated
      pagination.go           # cursor decode/encode helpers
    middleware/
      correlation.go          # x-correlation-id in/out
      recovery.go             # panic → AppError(500)
    auth/
      jwt.go                  # Verify(token) → Claims
      middleware.go           # Authenticate (cookie or Authorization header)
      apikey.go               # RequireInternalAPIKey
    rbac/
      middleware.go           # Require("orders:read", ...) — looks up perms via cache
      cache.go                # in-process LRU + TTL of role→permissions
    coreclient/
      client.go               # holds base URL + api-key
      rbac.go                 # GET /internal/rbac/permissions?role=… → []string
      types.go                # response shapes for core
    coreevents/
      consumer.go             # binds queue, routes deliveries to handlers, manual ack, dedupe
      handlers.go             # ONLY order_placed in the video; signatures for others
      payloads.go             # event payload structs (bson/json tags)
    di/
      container.go            # struct holding all singletons; built in main.go
      tokens.go               # constants if any are needed; mostly Container does the job
  app/
    analytics/
      controller/
        analytics.controller.go
      service/
        analytics.service.go
      repository/
        agg_restaurant_day.repo.go
        event_ids.repo.go      # for idempotency
      entity/
        agg_restaurant_day.go  # struct with bson tags
      dto/
        analytics.request.go   # validator tags for query params
        analytics.response.go  # response shape (Go equivalent of Response DTO)
      types.go                 # repo input shapes, internal types
      enums.go
      errors.go                # var ErrXxx = AppError{...}
      routes.go                # mounts on the chi router
  routes.go                    # mounts /api group, attaches global middleware
  go.mod
  go.sum
  .env.example
  README.md
  plan.md                      # this file
```

Layering is the same as `order-service`:

```
app/  → may import lib, pkg
lib/  → may import pkg, env; must NOT import app/
pkg/  → no imports from lib or app, no env, no globals
```

`cmd/api/main.go` is the only place `lib/` and `app/` meet — it constructs the `di.Container` and passes it to the router.

---

## 5. Per-component minimum (what to write, what to skip)

### `cmd/api/main.go`

- Load config → init slog → connect Mongo → `EnsureIndexes` → connect AMQP → build `di.Container` → start consumer goroutine → start chi server → wait on `signal.NotifyContext` for SIGINT/SIGTERM → graceful shutdown (consumer first, server second, then mongo, then amqp).

### `lib/config`

- Single struct with `env:"..."` tags. Sections: `HTTP`, `Mongo`, `AMQP`, `JWT`, `CoreClient`, `Internal`.
- One `Load()` that calls `env.Parse` and returns `(Config, error)`. No defaults that hide bugs (port and log-level can default; secrets cannot).

### `lib/logger`

- One `slog.Logger` configured for JSON in non-dev. Helper `FromContext(ctx)` that returns a child logger with correlation id baked in.
- No custom log levels. No fancy formatting.

### `lib/appcontext`

- `type ctxKey int` constants for `claimsKey`, `correlationIDKey`.
- `WithClaims(ctx, c)` / `Claims(ctx)`; same for correlation id.
- This is the Go equivalent of the order-service `express.d.ts` — every handler reads context here, never from request headers directly.

### `lib/errors`

- `AppError{Code string; Status int; Message string; cause error}` implementing `error`.
- Handler middleware: `defer recover()` → if panic → 500; if handler set an `AppError` on the response context → render JSON envelope.
- Idiomatic Go: handlers return `error`; the middleware catches and renders. (This means using `chi`'s `http.HandlerFunc` plus a small `wrap(func(w,r) error)` helper. Show this once in the video.)

### `lib/http`

- `SendSuccess(w, status, data)`, `SendError(w, AppError)`, `SendPaginated(w, items, nextCursor)`.
- Envelope shape matches order-service so the frontend doesn't branch.

### `lib/middleware`

- `Correlation`: read `x-correlation-id` or generate one, stash in context, echo on response.
- `Recovery`: panic → `AppError`.
- (chi has a built-in `middleware.Recoverer` — reuse it; we just want our envelope on top.)

### `lib/auth`

- `jwt.go`: parse + verify with HS256 secret from config; return typed `Claims` (`UserID`, `Role`, `RestaurantID *string`, `RestaurantRole *string`, `BranchIDs []string`).
- `middleware.go`: read token from cookie (preferred, same as order) or `Authorization: Bearer`; on success, `WithClaims(ctx, c)`.
- `apikey.go`: equality check on `api-key` header against `cfg.Internal.APIKey` for service-to-service routes (none in the video — left for HW).

### `lib/rbac`

- `cache.go`: `map[role][]permission` guarded by `sync.RWMutex` with per-entry expiry. Tiny custom LRU is fine; do not pull in a library for this.
- `middleware.go`: `Require("analytics:read")` — pulls claims from context, fetches perms via `coreclient.RBAC` (cached), denies with 403 if missing.
- **Skip** the `rbac.permissions_changed` invalidation handler (HW). In the video, mention "TTL is the only invalidation we have right now — HW adds the event."

### `lib/coreclient`

- `client.go`: holds base URL, internal API key, http client, logger.
- `rbac.go`: one method `GetPermissionsForRole(ctx, role) ([]string, error)`. JSON GET, retries on 5xx via `pkg/httpclient`.
- `types.go`: response struct.

### `lib/coreevents`

- `consumer.go`:
  - Declares the consumer queue (`analytics-service.core-events`) durable, with bindings: `order.placed` only in the video — list the other routing keys in a comment for HW.
  - Manual ack only after handler success.
  - Dedupe via `event_ids` repo: if `InsertOne` returns dup-key, we've seen it → ack and skip.
  - Errors from handler → `Nack(requeue=false)` so the broker DLQs it.
- `handlers.go`:
  - One function per event type, signature `func(ctx, payload) error`.
  - In the video: only `handleOrderPlaced` is implemented; others are stubs with `return errors.New("not implemented — homework")`.
- `payloads.go`: typed structs for each event payload, with `json` tags matching order-service's emitted shape.

### `lib/di`

- `Container` struct holds: `Cfg`, `Logger`, `Mongo`, `Broker`, `CoreClient`, `RBACCache`, `AnalyticsService`, `AnalyticsController`.
- Built in `main.go` with explicit constructors. No reflection. No struct tags. This is the deliberate "no magic" answer to tsyringe.

### `pkg/mongo`

- `client.go`: `Connect(ctx, uri, dbName) (*mongo.Database, func() error, error)` — returns the db handle and a close function.
- `indexes.go`: one function per collection, called from a top-level `EnsureIndexes(ctx, db)`.

### `pkg/messaging`

- `broker.go`: `Broker` interface with `Subscribe(queue string, h HandlerFunc) error`, `Close() error`. (No `Publish` in the video — analytics doesn't emit.)
- `amqp.go`: `NewAMQPBroker(cfg)` returning the interface. Handles connection, channel, queue declare, consume loop, manual ack/nack.
- This package must not know about `coreevents` payloads or any app types — it traffics in `[]byte` and routing keys.

### `pkg/httpclient`

- A `Do` wrapper that adds: timeout from caller's context, retry on 5xx with capped exponential backoff (max 3 tries), JSON marshalling helpers.

### `app/analytics`

- **Entity** `agg_restaurant_day.go`:
  ```go
  type AggRestaurantDay struct {
      RestaurantID string    `bson:"restaurant_id"`
      Date         string    `bson:"date"`         // "YYYY-MM-DD" UTC
      OrdersCount  int64     `bson:"orders_count"`
      RevenueSum   int64     `bson:"revenue_sum"`  // minor units
      // For averages: store sum + count, derive on read.
      DeliveryMsSum   int64  `bson:"delivery_ms_sum"`
      DeliveryMsCount int64  `bson:"delivery_ms_count"`
      UpdatedAt    time.Time `bson:"updated_at"`
  }
  ```
- **Repository** `agg_restaurant_day.repo.go`:
  - `Upsert(ctx, restaurantID, date, delta) error` — `UpdateOne` with `$inc` and `upsert: true`.
  - `FindRange(ctx, restaurantID, from, to) ([]AggRestaurantDay, error)` — uses the `(date, restaurant_id)` index.
- **Repository** `event_ids.repo.go`:
  - `MarkSeen(ctx, eventID) (bool, error)` — `InsertOne`; returns `false` on duplicate (consumed already).
- **Service** `analytics.service.go`:
  - `OnOrderPlaced(ctx, evt) error` — derives the day key in UTC, calls `Upsert` with the right deltas.
  - `GetRestaurantDays(ctx, restaurantID, from, to) ([]AnalyticsDayResponseDTO, error)` — calls repo, maps to response DTO (computes derived averages here, not in the repo).
- **Controller** `analytics.controller.go`:
  - One handler: `getRestaurantDays`. Validates query, calls service, sends response. Returns `error`; envelope handled by middleware.
- **DTOs**:
  - Request: `GetRestaurantDaysQuery{From string `validate:"required,datetime=2006-01-02"`; To string `validate:"required,datetime=2006-01-02,gtefield=From"`}`.
  - Response: `AnalyticsDayResponseDTO{Date, OrdersCount, RevenueMinor, Currency, AvgDeliveryMs, ...}` — explicit field list, no leaking of internal sums unless we want them.
- **`types.go`**: `UpsertRestaurantDayInput`, `OnOrderPlacedInput` (the parsed event payload as the service sees it).
- **`errors.go`**: `var ErrInvalidDateRange = AppError{Code: "ANALYTICS_INVALID_RANGE", Status: 400, Message: "invalid date range"}`.
- **`routes.go`**: `func Mount(r chi.Router, c *di.Container)` — wires `Authenticate` → `RBAC("analytics:read")` → controller method.

### `routes.go` (top-level)

- Builds the chi router. Global middleware: correlation, slog request log, recovery, error envelope. Mounts `/api/v1/analytics` via `analytics.Mount`.

---

## 6. The one slice, end to end (what the video actually demonstrates)

```
Order Service publishes  order.placed  →  RabbitMQ
                                            │
                                            ▼
                lib/coreevents/consumer.go (queue: analytics-service.core-events)
                                            │
                                            ▼
                event_ids.MarkSeen(eventID)  ── duplicate? ack & skip
                                            │
                                            ▼
                analytics.Service.OnOrderPlaced
                                            │
                                            ▼
                agg_restaurant_day.Upsert (Mongo $inc + upsert)
                                            │
                                            ▼
                                          ack

GET /api/v1/analytics/restaurants/:id/days?from=&to=
        │
        ▼
Authenticate → RBAC("analytics:read") → controller.getRestaurantDays
        │
        ▼
service.GetRestaurantDays → repo.FindRange → DTO mapping → SendSuccess
```

That is the whole video. Every other event, aggregate, and endpoint is a copy of this with different fields.

---

## 7. Acceptance check before recording

- [ ] `go run ./cmd/api` boots, logs structured JSON, exits cleanly on Ctrl-C.
- [ ] Mongo indexes appear on a fresh DB after first boot.
- [ ] Publishing an `order.placed` test message via `rabbitmqadmin` (or a 10-line Go script in `cmd/seed/`) increments the right doc.
- [ ] Re-publishing the same event ID does **not** double-count.
- [ ] `curl /api/v1/analytics/restaurants/<id>/days?from=...&to=...` with a valid cookie/JWT returns 200 + the upserted data.
- [ ] Same call without auth → 401. With wrong role → 403.
- [ ] `from > to` → 400 with the AppError envelope.

If those seven boxes tick, the slice is recordable.

---

## 8. Homework brief (paste at end of video)

> You now have one event consumer, one aggregate, and one endpoint. Replicate the pattern:
>
> 1. Add `agg_branch_day`, `agg_product_day`, `agg_platform_day` collections + indexes in `pkg/mongo/indexes.go`.
> 2. Implement handlers for `payment_completed`, `order_delivered`, `order_rejected`. Note: `order_delivered` updates the **same row** that `order_placed` already created — handle out-of-order events.
> 3. `agg_product_day` is one upsert per line item — batch them in a single `BulkWrite`.
> 4. Add the remaining 7 endpoints. The failure-rate and delivery-avg ones are **derived** from existing aggs — no new collections needed.
> 5. Add the `rbac.permissions_changed` consumer that invalidates the in-process RBAC cache.
> 6. Bonus: write a backfill command under `cmd/backfill-aggs/` that replays a date range from order-service's hot DB.
