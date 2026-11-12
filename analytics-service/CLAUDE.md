# CLAUDE.md — Analytics Service (Service C) Guidelines

These rules apply to the **`analytics-service`** microservice of the **QuickBite** platform. The service is implemented in **Go + MongoDB + RabbitMQ** and mirrors the layered architecture of the Node services (`core-service`, `order-service`) so the same patterns translate across languages.

When in doubt, **look at how `core-service` or `order-service` do it and find the Go analogue** — most decisions have already been made for you. The single most useful doc here is [`docs/node-to-go-mapping.md`](docs/node-to-go-mapping.md): it lists every Node convention and its Go equivalent.

---

## 1. Mission of this service

This service owns **analytics aggregates**: per-day rollups of order/payment/delivery activity, plus the read endpoints that serve dashboards.

- **Consumes** events from `order-service` via RabbitMQ (`order.events` exchange).
- **Upserts** day-grained aggregates into MongoDB (`agg_restaurant_day`, `agg_branch_day`, `agg_product_day`, `agg_platform_day`).
- **Serves** read-only HTTP endpoints under `/api/v1/analytics/...`.

It does **not** own: users, restaurants, branches, products, orders, payments. It does not write to any operational data store. It does not emit events.

---

## 2. Tech stack (locked — do not deviate)

| Concern        | Choice                                               |
| -------------- | ---------------------------------------------------- |
| Runtime        | Go 1.21+                                             |
| HTTP router    | `github.com/go-chi/chi/v5`                           |
| DB             | MongoDB driver `go.mongodb.org/mongo-driver` (v1)    |
| Messaging      | `github.com/rabbitmq/amqp091-go`                     |
| HTTP client    | stdlib `net/http` + `pkg/httpclient` wrapper         |
| Config         | `github.com/caarlos0/env/v11` (env-tagged struct)    |
| Logger         | stdlib `log/slog`                                    |
| Validation     | `github.com/go-playground/validator/v10`             |
| JWT            | `github.com/golang-jwt/jwt/v5`                       |
| UUID           | `github.com/google/uuid`                             |
| DI             | **No framework.** Explicit constructor wiring in `cmd/api/main.go`. |

**Forbidden:** GORM, Ent, any ODM. Repositories use the official mongo-driver directly with typed structs and `bson` tags. This is the deliberate Go analogue of "Knex query builder, not an ORM" — same philosophy, different language.

---

## 3. Folder structure (mirrors `core-service` / `order-service`)

```
analytics-service/
  cmd/
    api/main.go               # ~10 lines: calls lib/boot.Run() and exits
  pkg/                        # framework-free, NO imports from lib/ or app/
    mongo/client.go           # Connect / Disconnect — that's it
    messaging/                # Broker interface + amqp091 impl
    httpclient/               # net/http wrapper with retries
  lib/                        # app-aware glue: env, middleware, infra
    boot/boot.go              # wires every singleton, runs http + consumer, handles shutdown
    config/                   # env.go
    logger/                   # slog wrapper, FromContext
    appcontext/               # ctx keys (claims, correlation id) — like express.d.ts
    errors/                   # AppError + handler middleware
    http/                     # response envelope helpers
    middleware/               # correlation, access-log
    auth/                     # jwt.go, Authenticate, RequireInternalAPIKey
    rbac/                     # read-through cache + Require middleware
    coreclient/               # http client to core-service (RBAC perms)
    coreevents/               # rabbitmq consumer, envelope, payloads, EventHandler type
  app/
    analytics/
      types.go                # module-shared types (OnOrderPlacedInput, RestaurantDayRow)
      errors.go               # module-level AppError vars
      enums.go                # permission constants
      entity/                 # plain structs with bson tags
      repository/             # mongo-driver code lives here ONLY
        indexes.go            # EnsureIndexes for this module's collections
      service/                # business logic; returns AppError
      controller/             # http → service → DTO; routes.go lives here too
      dto/                    # wire shapes
      eventhandlers/          # rabbitmq event-type → service method map
  play/                       # GITIGNORED: dev aids (mock-core, mint-jwt, publish-test, check-mongo)
  go.mod
  CLAUDE.md, README.md, plan.md
  .env.example, .gitignore
  docs/                       # extended docs (see §12)
```

### `pkg/` vs `lib/` vs `app/` — strict layering

```
       app/  ── may import lib, pkg
       lib/  ── may import pkg, env; may NOT import app/<module>/*
       pkg/  ── pure providers, NO imports from lib or app, NO env, NO globals, NO app-specific knowledge
```

- `pkg/` is **framework-agnostic** AND **app-agnostic**. Mongo client knows how to connect; it does NOT know about `agg_restaurant_day` or `event_ids` collections — those names belong to the app. Index declarations live in `app/analytics/repository/indexes.go`, not in `pkg/mongo`.
- `lib/` wires `pkg/` to the app. The bootstrap that ties everything together lives in `lib/boot/boot.go` so `cmd/api/main.go` stays tiny.
- `app/` contains business modules. Cross-module calls must go through services (never reach into another module's repository).

If a `lib/*` file needs something from `app/*` — invert the dependency by defining a small interface in `lib/`. See `lib/coreevents.EventDeduper` for the canonical example: the consumer needs a "have I seen this event before?" capability, and the repository implements it implicitly via duck typing.

---

## 4. Naming conventions

### Go-specific

- Package names: short, lowercase, one word (`coreevents`, `analyticsctrl`). No underscores or camelCase.
- Files: `snake_case.go` is acceptable, but prefer descriptive nouns: `analytics.service.go`, `agg_restaurant_day.repo.go`.
- Exported types: `PascalCase`. Unexported: `camelCase`.
- Method receivers: short, consistent (`func (s *AnalyticsService) ...`, `func (r *AggRestaurantDayRepo) ...`).
- One type per file when the type is non-trivial; many tiny types in one `types.go` is fine.

### Database (MongoDB)

- Collection names: plural, `snake_case` (`agg_restaurant_day`, `event_ids`).
- Document fields: `snake_case` via `bson` tags. Booleans prefixed `is_`.
- Indexes: declared exclusively in `app/<module>/repository/indexes.go`. Every index has a name (`SetName`) so we can spot it in `db.collection.getIndexes()`. Never put collection-specific knowledge in `pkg/mongo`.
- Money: `int64` integer minor units. **Never `float`.** Currency held on the row.
- Timestamps: `time.Time` (Mongo `Date`). Always UTC. Day buckets are `"YYYY-MM-DD"` strings — lexicographically sortable, no timezone ambiguity.

### Routes

- Plural resource nouns: `/analytics/restaurants/...`, `/analytics/branches/...`.
- Versioned: `/api/v1/...`.
- `GET` only in this service — no writes from the HTTP layer.

---

## 5. Module file conventions

Every module under `app/<module>/` follows the same skeleton. See `app/analytics/` for the canonical example:

1. **`app/<module>/types.go`** (package `<module>`) — module-shared types used across subpackages (e.g. `OnOrderPlacedInput`, `RestaurantDayRow`). Pure data, no I/O.
2. **`app/<module>/errors.go`** — module-level `var Err… = apperr.New(...)` instances.
3. **`app/<module>/enums.go`** — module-scoped constants (e.g. permission strings).
4. **`entity/<name>.go`** — plain struct with `bson` tags. No methods beyond invariants. No DB knowledge.
5. **`dto/*.request.go`** — request shape with `validate` tags (go-playground/validator).
6. **`dto/*.response.go`** — wire shape. **Plain struct with `json` tags.** Decoupled from entities so we can evolve without leaking columns.
7. **`repository/<name>.repo.go`** — struct holding the `*mongo.Collection`, methods take `ctx context.Context` first. The repo is the **only** place mongo-driver imports appear.
8. **`repository/indexes.go`** — `EnsureIndexes(ctx, db)` declares every index for this module's collections. Called from `lib/boot` at startup. Idempotent.
9. **`service/<name>.service.go`** — `Service` struct constructed via `NewXxxService(...)`. Composes repos. Returns `error` (often `*AppError`). Imports `app/<module>` for shared types and errors.
10. **`controller/<name>.controller.go`** — methods receive `(w, r)` and return `error`; envelope rendered by `errors.Wrap`.
11. **`controller/routes.go`** — `func Mount(r chi.Router, ...)` wires middleware to handlers.
12. **`eventhandlers/handlers.go`** — `func For(svc) map[string]coreevents.EventHandler` builds the routing table from event type → service call.

### Inline types — **forbidden**

Never declare `type X struct{...}` or `type Y = ...` aliases inside `*.service.go`, `*.controller.go`, or `*.repo.go`. They go in:
- `app/<module>/types.go` for module-shared shapes (inputs, projections).
- `lib/<area>/types.go` (or `payloads.go`) for cross-cutting infra types (event payloads, client response shapes).

Exception: tiny one-shot types used in a single function body are fine inline.

---

## 6. Response shape — wire format rule

Every HTTP response payload MUST be shaped by a Response DTO struct declared in `dto/<module>.response.go`. Reasons (same as `order-service` CLAUDE.md):

1. Decouples the wire format from internal entities/DB documents.
2. Lets us evolve schemas without leaking field renames to clients.
3. Avoids accidentally leaking sensitive fields.

### Rules

- Response DTOs are plain structs with `json` tags.
- Controllers map service-layer projections → response DTOs before calling `apphttp.SendSuccess`.
- Money fields use integer minor units (e.g. `1500` = 15.00 EGP) with a `currency` field next to them. **No server-side formatting.**
- Timestamps are ISO-8601 UTC strings.
- Never include internal IDs, dedupe markers, or anything that isn't part of the public contract.

---

## 7. Database design rules

### Money

- Stored as `int64` minor units. Currency held on the row.
- Aggregation across millions of rows is fast.
- Day buckets are `"YYYY-MM-DD"` strings (UTC).

### Indexes

- Declared in **one** place per module: `app/<module>/repository/indexes.go`.
- Every index has a `SetName` so we can identify it. Every index has a one-line comment naming the query it supports.
- `EnsureIndexes` is called from `lib/boot.Run()` on boot. Mongo's `CreateIndexes` is idempotent — re-running is safe.
- Composite indexes follow **(equality cols, then range col)**.
- **No N+1 queries.** Repositories batch with `$in` / range queries.
- `pkg/mongo` must never know any specific collection name. Putting collection-specific knowledge there is a layering violation — push it into the owning module's repository.

### Idempotency

- Required on every event handler. Implemented via `event_ids` collection with a unique index on `event_id` + TTL on `received_at`.
- The consumer (`lib/coreevents/consumer.go`) calls `MarkSeen` before dispatching to the per-event handler. Duplicate → ack-and-skip.

### Schemas

- We do not run DDL migrations. Schema evolution is a doc-shape change, not a SQL migration. If we change a field's meaning, write a one-shot backfill under `cmd/backfill-<name>/`.

---

## 8. Cross-cutting infra

### Context (`lib/appcontext`)

- The Go equivalent of the order-service `lib/types/express.d.ts`. Handlers and consumers read `claims` and `correlationID` from `context.Context` via typed helpers. Never read JWT tokens or cookie headers directly outside `lib/auth`.

### Auth

- Same JWT contract as `core-service` and `order-service`. HS256 with the access secret. Claims shape: `userId`, `role`, optional `restaurantId`, `restaurantRole`, `branchIds`.
- Token source preference: access cookie first, then `Authorization: Bearer`.
- Internal service-to-service: `api-key` header, plain equality (`lib/auth/apikey.go`).

### RBAC

- This service has **no permissions catalog of its own**. It resolves permissions for a role by calling `core-service`'s `GET /api/internal/rbac/permissions?role=...`.
- A small in-process cache (`lib/rbac/cache.go`) keyed by role with TTL (default 5 min) prevents hammering core.
- **Homework:** wire a `rbac.permissions_changed` consumer that calls `cache.Invalidate(role)` to make changes apply faster than the TTL.

### Error handling

- Throw `*AppError` from services. Never throw plain `error` for user-visible failures.
- Module-level error instances live in `app/<module>/errors.go`. Don't construct ad-hoc `apperr.New(...)` for cases that have a stable name.
- The error middleware (`lib/errors/Wrap`) catches anything a handler returns and renders the envelope.

### Validation

- Request bodies / query strings pass through `validator.Validate.Struct(dto)` in the controller. Use `validate:` tags on the DTO.

### Async with `order-service` (RabbitMQ)

- Topology:
  - Topic exchange: `order.events` (durable, declared by `order-service`'s outbox drainer).
  - Consumer queue: `analytics-service.order-events` (durable, declared by this service).
  - Bindings: `order.#`, `payment.#`.
  - DLQ: `analytics-service.order-events.dlq` for poison messages (routed via dead-letter exchange).
- Delivery semantics: **at-least-once**. Manual ack after the handler commits.
- Dedupe via Mongo `event_ids` collection (see §7).

---

## 9. Performance & scale rules

1. **No N+1 queries.** Repositories batch.
2. **Every query backed by an index.** Run an EXPLAIN before merging.
3. **No app-side joins.** If you need joined data, query both collections with `$in` and zip in the service.
4. **Long-running work** → background command under `cmd/`, not an HTTP handler.
5. **Idempotency** on every event handler.
6. **Hot reads** (analytics dashboard) must complete in **< 200ms p95**.

---

## 10. Code style — what to avoid

- ❌ ORMs / ODMs.
- ❌ Returning entities from controllers (use response DTOs).
- ❌ Cross-module repository imports (use the other module's service).
- ❌ Business logic in controllers.
- ❌ Silent `if err != nil { _ = err }`. Either rethrow or log loudly.
- ❌ `interface{}` / `any` in service signatures (except for envelopes from JSON).
- ❌ Mutating an input DTO inside a service.
- ❌ Hidden globals. Singletons are constructed in `lib/boot.Run()` and passed by argument; no package-level state.
- ❌ Adding env vars without extending the `lib/config.Config` struct.
- ❌ Putting collection names, business types, or business logic in `pkg/`. If `pkg/` knows about your business, it's in the wrong place.
- ❌ Heavy `cmd/api/main.go`. Boot lives in `lib/boot`. Main is a process entry point, not an init function.
- ❌ Committing dev aids to the repo. One-shot scripts, manual smoke tests, debug dumpers go in `play/` (gitignored).

---

## 11. When implementing a new module

Follow this exact order — never skip ahead:

1. Shared module types in `app/<module>/{types,errors,enums}.go`.
2. Entity struct in `app/<module>/entity/`.
3. Indexes for the new collection(s) in `app/<module>/repository/indexes.go`.
4. Repository struct + methods in `app/<module>/repository/`.
5. Request and Response DTOs in `app/<module>/dto/`.
6. Service struct in `app/<module>/service/`.
7. Event handlers (if any) in `app/<module>/eventhandlers/`.
8. Controller in `app/<module>/controller/`.
9. `Mount(r, ...)` in `app/<module>/controller/routes.go`.
10. Wire the new module into `lib/boot.Run()` (single function — one new repo line, one new service line, one new `Mount` call).
11. Smoke test end-to-end with `play/publish-test` and `curl`.

Implement one module end-to-end before starting the next.

---

## 12. Reference docs (in `docs/`)

- [`docs/folder-structure.md`](docs/folder-structure.md) — annotated tree, layer rules.
- [`docs/system-design.md`](docs/system-design.md) — sync/async flows, mongo, rabbit, archival posture.
- [`docs/api-contracts.md`](docs/api-contracts.md) — endpoint-by-endpoint request/response shapes.
- [`docs/node-to-go-mapping.md`](docs/node-to-go-mapping.md) — **the teaching doc.** Every Node convention in `order-service` paired with its Go equivalent in this service.
- [`docs/ai-prompts.md`](docs/ai-prompts.md) — prompts and strategies for using AI to learn a new language by mapping it onto patterns you already know.
- [`docs/implementation-plan.md`](docs/implementation-plan.md) — phases of the build with acceptance checks.

---

## 13. Dev aids (`play/`)

The `play/` directory is **gitignored**. It holds short, throwaway Go programs that exist only to make local development easier. Right now it contains:

- `play/publish-test` — publishes one envelope shaped exactly like `order-service`'s outbox drainer emits. Use to E2E-test without spinning up Postgres and order-service.
- `play/mock-core` — fake of core-service's `/api/internal/rbac/permissions` endpoint; returns `["analytics:read"]` for any role.
- `play/mint-jwt` — signs an HS256 JWT in the same shape core/order issue.
- `play/check-mongo` — dumps `agg_restaurant_day` rows.

```sh
go run ./play/publish-test -event=order.placed -restaurant=42 -total=2500
```

Pass `-repeat-id=<uuid>` to test dedupe: the second publish with the same id should be acked-and-skipped.

These exist locally because the recorded video produces them step-by-step; cloning the repo gives you nothing under `play/` (intentional — write your own dev tools, that's part of the exercise).

---

## 14. Out of scope (do not build)

- Outbound async events from this service to anyone — analytics is sink-only in this milestone.
- A migration tool — `EnsureIndexes` does the job.
- Caching layer (Redis) — Mongo + idempotency dedupe are enough for the slice.
- WebSocket / live dashboards — out of scope.
- Read replicas / sharding — single primary for now.
