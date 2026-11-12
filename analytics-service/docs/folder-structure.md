# Folder Structure (annotated)

```
analytics-service/
├── cmd/
│   └── api/main.go                 # ~10 lines: calls lib/boot.Run() and exits
│
├── pkg/                            # framework-free, NO imports from lib/ or app/
│   ├── mongo/client.go             # Connect(uri, db) → *mongo.Database. No app knowledge.
│   ├── messaging/
│   │   ├── types.go                # Broker interface, ConsumerOpts, Delivery
│   │   └── amqp.go                 # amqp091 implementation
│   └── httpclient/client.go        # net/http wrapper: timeout, JSON, retry-on-5xx
│
├── lib/                            # app-aware glue: env, middleware
│   ├── boot/boot.go                # wires everything; main.go just calls Run()
│   ├── config/env.go               # struct + Load() — equivalent of the zod schema
│   ├── logger/logger.go            # slog wrapper + FromContext
│   ├── appcontext/context.go       # ctx keys: claims, correlation_id (≈ express.d.ts)
│   ├── errors/
│   │   ├── apperror.go             # AppError type
│   │   └── handler.go              # Wrap(handler) middleware → envelope
│   ├── http/response.go            # SendSuccess, SendPaginated
│   ├── middleware/correlation.go   # Correlation + AccessLog middleware
│   ├── auth/
│   │   ├── jwt.go                  # VerifyAccessToken — same shape as core/order
│   │   ├── middleware.go           # Authenticate
│   │   └── apikey.go               # RequireInternalAPIKey (service-to-service)
│   ├── rbac/
│   │   ├── cache.go                # in-process role→permissions map + TTL
│   │   └── middleware.go           # Require("analytics:read")
│   ├── coreclient/
│   │   ├── client.go               # holds baseURL + api-key
│   │   ├── rbac.go                 # GetPermissionsForRole
│   │   └── types.go                # response shapes
│   └── coreevents/
│       ├── consumer.go             # generic Start(): dedupe → dispatch → ack/nack
│       └── payloads.go             # Envelope, per-event payload structs, EventHandler type
│
├── app/
│   └── analytics/
│       ├── types.go                # OnOrderPlacedInput, RestaurantDayRow (package analytics)
│       ├── errors.go               # ErrInvalidDateRange, ErrInvalidRestaurantID
│       ├── enums.go                # PermAnalyticsRead = "analytics:read"
│       ├── entity/agg_restaurant_day.go         # plain struct + bson tags
│       ├── repository/
│       │   ├── indexes.go                       # EnsureIndexes — this module's collections
│       │   ├── agg_restaurant_day.repo.go       # Upsert, FindRange
│       │   └── event_ids.repo.go                # MarkSeen — the dedupe collection
│       ├── service/analytics.service.go         # OnOrderPlaced, GetRestaurantDays
│       ├── controller/
│       │   ├── analytics.controller.go          # one handler: GetRestaurantDays
│       │   └── routes.go                        # Mount(r, ...) — module's routes
│       ├── dto/
│       │   ├── analytics.request.go             # GetRestaurantDaysQuery + validate tags
│       │   └── analytics.response.go            # AnalyticsDayResponseDTO
│       └── eventhandlers/handlers.go            # event type → service method map
│
├── play/                           # GITIGNORED — dev aids only
│   ├── publish-test/main.go        # fake order.events publisher
│   ├── mock-core/main.go           # fake core RBAC endpoint
│   ├── mint-jwt/main.go            # signs an HS256 JWT
│   └── check-mongo/main.go         # dumps agg_restaurant_day
│
├── go.mod
├── .env.example, .gitignore
├── CLAUDE.md, README.md, plan.md
└── docs/
```

## Layer rules (enforced by review, not by tooling)

```
       app/  ── may import lib, pkg
       lib/  ── may import pkg, env; may NOT import app/<module>/*
       pkg/  ── pure providers, NO imports from lib or app, NO env, NO globals, NO app-specific knowledge
```

The strictest reading of "no app-specific knowledge in `pkg/`": `pkg/mongo` knows how to connect, ping, and return a `*mongo.Database`. It does NOT know that this service has an `agg_restaurant_day` collection. That's app knowledge — it lives in `app/analytics/repository/indexes.go`.

### Why `cmd/api/main.go` is tiny

The process entry point should do one thing: hand control to the bootstrap and exit on its return. Putting wiring there means every new singleton requires editing the entry point — which gets crowded fast. Wiring lives in `lib/boot/boot.go`, so:

- Adding a module = edit one function in `lib/boot`.
- main.go never changes.
- The signal handling, graceful shutdown, and serve-error pipe all live with the wiring they affect.

### Why `cmd/<binary>/`?

Idiomatic Go. Every binary has its own package `main` in `cmd/<name>/`. We have exactly one production binary right now (`cmd/api`). Future scheduled work (backfills, archival) would land as `cmd/backfill-<thing>/main.go`.

### Why `play/` instead of `cmd/`?

`play/` is gitignored. The four programs in it (mock-core, mint-jwt, publish-test, check-mongo) are dev aids — they make the slice testable without spinning up Postgres + order-service, but they're not part of the service. Pollute `cmd/` with them and someone will assume they're production binaries. Convention from `order-service/CLAUDE.md` §5.

### Why is `coreevents` in `lib/` if events come from order-service, not core?

Name carried over from `order-service`'s `lib/core-events/` directory. The role is "external events that drive our state" — exact same. Renaming would diverge from the Node services without any benefit. The package-level doc comment notes the conceptual mismatch.

### Where does the equivalent of `tsyringe` live?

There isn't one. `lib/boot.Run()` is the wiring. Read it once — every singleton's construction is visible. Anything that needs a singleton receives it through its constructor (`NewAnalyticsService(aggRepo)`, etc.). No reflection, no decorators, no registration step.

### Where do shared module types live?

In `app/<module>/` itself (package matches the directory name — `analytics`). Subpackages (`service`, `controller`, `eventhandlers`) import the parent for `OnOrderPlacedInput`, `RestaurantDayRow`, `ErrInvalidDateRange`, etc. This keeps service-internal organisation clean: `service/` holds only the service struct and its methods, not the types and errors it returns.
