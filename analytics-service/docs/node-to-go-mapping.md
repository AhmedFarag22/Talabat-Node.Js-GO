# Node → Go Mapping

This is the most important doc in the repo if you already know `core-service` / `order-service` and you're learning Go through this codebase. Every convention you've seen on the Node side has a Go equivalent below.

> **The point of this doc.** You don't learn a new language by reading a tutorial — you learn it by mapping its idioms onto patterns you already use. This file is the dictionary.

---

## 1. Project structure

| Concern                       | Node services                              | This service                                 |
| ----------------------------- | ------------------------------------------ | -------------------------------------------- |
| Entry point                   | `src/server.ts` + `src/worker.ts`          | `cmd/api/main.go`                            |
| Composition / app             | `src/app.ts`                               | `routes.go` (top-level `BuildRouter`)        |
| Module folders                | `src/app/<module>/...`                     | `app/<module>/...`                           |
| App-aware infra               | `src/lib/...`                              | `lib/...`                                    |
| Framework-free providers      | `src/pkg/...`                              | `pkg/...`                                    |
| One file per class            | yes                                        | yes (one Go struct per file ≈ one TS class)  |

---

## 2. Language idioms

| Concept                  | TypeScript                                                 | Go                                                                                       |
| ------------------------ | ---------------------------------------------------------- | ---------------------------------------------------------------------------------------- |
| Module                   | one file = one ES module                                   | one **directory** = one package; multiple files in same package share scope              |
| Class                    | `class OrderService { … }`                                 | `type OrderService struct { … }` + methods on `*OrderService`                            |
| Constructor              | `constructor(deps) { this.deps = deps }`                   | `func NewOrderService(deps) *OrderService { return &OrderService{deps: deps} }`          |
| Decorators (`@injectable`) | tsyringe sees decorators                                  | none — wire constructors by hand in `cmd/api/main.go`                                    |
| Interface                | `interface Foo { bar(): void }`                            | `type Foo interface { Bar() }` (note: PascalCase exports, lowercase = unexported)        |
| `instanceof Error` chain | try/catch + cause                                          | `errors.As(err, &target)` / `errors.Is(err, sentinel)`                                   |
| `null` / `undefined`     | nullable union                                             | zero values + pointers (`*string` for "maybe a string")                                  |
| `async/await`            | every fn that awaits is `async`                            | every fn takes `ctx context.Context` first; no `async` keyword — concurrency is goroutines + channels |
| `Promise<T>`             | implicit return                                            | `(T, error)` tuple                                                                       |
| Throwing                 | `throw new AppError(...)`                                  | `return apperr.New(...)` — no exceptions in normal control flow                          |
| Generics                 | `class Foo<T>`                                             | `func Foo[T any](x T)` (1.18+); use sparingly                                            |
| Iteration                | `for (const x of xs)`                                      | `for _, x := range xs`                                                                    |
| Map                      | `Map<K, V>`                                                | `map[K]V` (built-in; not safe for concurrent writes — wrap with `sync.Mutex`)            |

---

## 3. Stack one-for-ones

| Concern              | Node (`order-service`)                    | Go (this service)                       | Why this Go pick                                                |
| -------------------- | ----------------------------------------- | --------------------------------------- | --------------------------------------------------------------- |
| HTTP framework       | `express` v5                              | `chi` v5                                | small, idiomatic, middleware compatible                         |
| Validation           | `class-validator` (decorators)            | `validator/v10` (struct tags)           | tag-based; no decorators in Go                                  |
| DI                   | `tsyringe`                                | constructor wiring in `main.go`         | Go's stdlib + `func` is enough; reflection-DI buys nothing here |
| Env validation       | `zod`                                     | `caarlos0/env` (struct tags)            | tag-based parsing; same fail-fast behavior                      |
| DB driver            | `knex` over `pg`                          | `mongo-driver` (official)               | low-level, no ORM — same philosophy                              |
| Cache                | `ioredis` via `RedisCacheProvider`        | (n/a in this service; in-process map)   | analytics doesn't need Redis yet                                |
| Logger               | custom `Logger` (winston-like)            | stdlib `log/slog`                       | structured, no dep                                              |
| JWT                  | `jsonwebtoken`                            | `golang-jwt/jwt`                        | matches the HS256 + claims shape                                |
| Messaging            | `amqp-connection-manager` + `amqplib`     | `amqp091-go`                            | official RabbitMQ Go client                                     |
| UUID                 | `crypto.randomUUID()`                     | `google/uuid`                           | stdlib has no UUID                                              |
| Background jobs      | Croner + `lib/jobs/scheduler`             | (n/a here) — would be a goroutine + ticker | analytics has no scheduled work in the slice                |

---

## 4. Layer-by-layer file mapping

### Config

```ts
// order-service/src/lib/config/env.ts
const baseSchema = z.object({
    PORT: z.string().default("4000"),
    RABBITMQ_URL: z.string(),
    ...
});
const parsed = baseSchema.parse(process.env);
export const env = { port: Number(parsed.PORT), ... };
```

```go
// analytics-service/lib/config/env.go
type Config struct {
    HTTP HTTPConfig
    AMQP AMQPConfig
    ...
}
type HTTPConfig struct { Port int `env:"HTTP_PORT" envDefault:"4100"` }
type AMQPConfig struct { URL string `env:"RABBITMQ_URL,required"` }

func Load() (Config, error) {
    var cfg Config
    if err := env.Parse(&cfg); err != nil { return Config{}, err }
    return cfg, nil
}
```

Same pattern: typed struct, fail-fast at boot. Decorators → struct tags.

### Logger

```ts
// custom logger w/ correlation id helpers
logger.info("event", { eventId, eventType });
```

```go
// log/slog: keys are typed
log.Info("event", slog.String("event_id", id), slog.String("event_type", t))
```

### Request context (the big one)

```ts
// order-service/src/lib/types/express.d.ts
declare module "express" {
    interface Request {
        user?: ActorContext;
        correlationId?: string;
    }
}
// auth middleware sets req.user; handlers read req.user.userId
```

```go
// analytics-service/lib/appcontext/context.go
type ctxKey int
const claimsKey ctxKey = iota

func WithClaims(ctx, c) context.Context  { return context.WithValue(ctx, claimsKey, c) }
func Claims(ctx) *ClaimsValue            { v, _ := ctx.Value(claimsKey).(*ClaimsValue); return v }

// auth middleware does ctx = appcontext.WithClaims(ctx, claims) → handler reads via Claims(ctx)
```

Same role: the request carries identity through layers without parameter pollution. In Go this is `context.Context` instead of decorating `Request`.

### Error handling

```ts
// thrown from service
throw new AppError("ORDER_NOT_FOUND", 404, "order not found");
// Express middleware catches, renders envelope
```

```go
// returned from service
return apperr.New("ORDER_NOT_FOUND", http.StatusNotFound, "order not found")
// errors.Wrap middleware catches, renders envelope
type Handler func(w, r) error
http.HandleFunc("/x", apperr.Wrap(logger, ctrl.GetX))
```

Go has no exceptions, so we go the other way — handlers explicitly return `error`, the wrapper renders. The user-visible behavior is identical.

### DTOs and validation

```ts
// class-validator
class CreateOrderRequestDTO {
    @IsInt() branchId!: number;
    @IsArray() items!: ItemDTO[];
}
// in controller: validateBody(CreateOrderRequestDTO, req.body);
```

```go
// validator/v10 with struct tags
type GetRestaurantDaysQuery struct {
    From string `validate:"required,datetime=2006-01-02"`
    To   string `validate:"required,datetime=2006-01-02"`
}
// in controller: validator.New().Struct(q)
```

### Repositories

```ts
// order-service/src/app/order/repository/order.repo.ts
export async function findOrderByPublicId(id: string, conn = db) {
    const row = await conn("orders").select(ORDER_COLUMNS).where({public_id: id}).first();
    return row ? toEntity(row) : null;
}
```

```go
// analytics-service/app/analytics/repository/agg_restaurant_day.repo.go
func (r *AggRestaurantDayRepo) FindRange(ctx, restaurantID, from, to) ([]entity.AggRestaurantDay, error) {
    cur, err := r.col.Find(ctx, bson.D{...})
    if err != nil { return nil, err }
    defer cur.Close(ctx)
    var rows []entity.AggRestaurantDay
    return rows, cur.All(ctx, &rows)
}
```

Difference: TS uses module-level functions taking `conn` (so callers can pass a trx). Go uses methods on a `Repo` struct that holds a `*mongo.Collection` (no trx in Mongo at this scale; if needed later, sessions are per-call).

### Services

```ts
@injectable()
export class OrderService {
    constructor(@inject(TOKENS.PaymentService) private payment: PaymentService) {}
    async placeOrder(...) { ... }
}
```

```go
type AnalyticsService struct {
    aggRepo *repository.AggRestaurantDayRepo
}
func NewAnalyticsService(aggRepo *repository.AggRestaurantDayRepo) *AnalyticsService {
    return &AnalyticsService{aggRepo: aggRepo}
}
func (s *AnalyticsService) OnOrderPlaced(ctx, in) error { ... }
```

### Controllers

```ts
@injectable()
export class OrderController {
    constructor(private svc: OrderService) {}
    placeOrder = async (req, res, next) => {
        try {
            const dto = await validateBody(CreateOrderRequestDTO, req.body);
            const out = await this.svc.placeOrder(req.user!, dto, req.region);
            sendSuccess(res, 201, out);
        } catch (e) { next(e); }
    };
}
```

```go
type AnalyticsController struct { svc *service.AnalyticsService; v *validator.Validate }
func (c *AnalyticsController) GetRestaurantDays(w, r) error {
    q := dto.GetRestaurantDaysQuery{From: r.URL.Query().Get("from"), To: r.URL.Query().Get("to")}
    if err := c.v.Struct(q); err != nil { return apperr.New("VALIDATION_ERROR", 400, err.Error()) }
    rows, err := c.svc.GetRestaurantDays(r.Context(), id, q.From, q.To)
    if err != nil { return err }
    apphttp.SendSuccess(w, 200, mapToDTO(rows))
    return nil
}
```

### Routes

```ts
// order-service/src/app/order/routes.ts
const router = Router();
router.post("/", authenticate, rbac("orders:create"), idempotency(), ctrl.placeOrder);
export default router;
```

```go
// analytics-service/app/analytics/controller/routes.go
func Mount(r chi.Router, ...) {
    r.Route("/analytics", func(r chi.Router) {
        r.Use(auth.Authenticate(...))
        r.Use(rbac.Require(cache, "analytics:read"))
        r.Get("/restaurants/{restaurantId}/days", apperr.Wrap(logger, ctrl.GetRestaurantDays))
    })
}
```

### Messaging — consumer

```ts
// order-service/src/lib/core-events/consumer.ts
export async function startCoreEventsConsumer(broker) {
    await broker.declareTopology(topology);
    await broker.consume(topology, handleMessage);
}
async function handleMessage(msg) {
    const env = parseEnvelope(msg);
    const fresh = await cache.trySet(`dedupe:${env.eventId}`, "1", 24*3600);
    if (!fresh) return msg.ack();
    const handler = handlers.get(env.eventType);
    try { await handler(env.payload); msg.ack(); }
    catch (e) { msg.nack(false); }
}
```

```go
// analytics-service/lib/coreevents/consumer.go
func Start(ctx, log, broker, opts, handlers, deduper) error {
    return broker.Subscribe(ctx, opts, func(ctx, d) error {
        var env Envelope; json.Unmarshal(d.Body, &env)
        fresh, _ := deduper.MarkSeen(ctx, env.EventID)
        if !fresh { return nil } // ack-skip
        h, ok := handlers[env.EventType]
        if !ok { return nil }    // unknown type → ack-skip
        return h(ctx, raw)        // err → nack(no-requeue) → DLQ
    })
}
```

Same algorithm. Differences:
- Node dedupe: Redis SETNX. Go dedupe: Mongo unique-index `InsertOne`. Same semantics, different store.
- Node handler-not-found: warn + ack. Go: warn + ack. Identical.
- Node nack: explicit `msg.nack(false)`. Go: return error → broker layer nacks.

### Messaging — publisher (lives in `order-service`, not here)

```ts
// order-service/src/lib/events/outbox-drain.ts
const trx = await db.transaction();
const rows = await claimBatch(trx, batchSize);
for (const row of rows) {
    await broker.publish(exchange, row.event_type, Buffer.from(JSON.stringify(envelope)));
    await markDispatched(trx, row.id);
}
await trx.commit();
```

There is no Go equivalent in this service — analytics is sink-only. The Node code above is what `order-service` runs to feed us. Read it once to understand the contract.

### DI container

```ts
// order-service/src/lib/di/container.ts
container.registerSingleton(TOKENS.AnalyticsService, AnalyticsService);
// resolution: container.resolve<AnalyticsService>(TOKENS.AnalyticsService)
```

```go
// analytics-service/lib/di/container.go
type Container struct {
    Cfg     config.Config
    Logger  *slog.Logger
    Mongo   *mongo.Database
    Service *service.AnalyticsService
    ...
}
// construction in main.go:
c := &di.Container{ Service: service.NewAnalyticsService(repo), ... }
```

Trade-off: tsyringe lets you forget how things are constructed and just inject. The Go pattern forces you to write the wiring once in `main.go` — but you can read every singleton's birth in 50 lines. For a 1-module service this is a clear win; the cost grows with the number of modules.

---

## 5. Common gotchas when porting

### `null` vs `nil` vs zero values

- TS: `name?: string` → could be `undefined`, `null`, or `string`.
- Go: `Name string` is `""` if missing; `Name *string` is `nil` for "no value." Don't reach for pointers reflexively — empty string / zero is usually fine.

### Date / time

- TS: `new Date()`, `Date.toISOString()`.
- Go: `time.Now().UTC().Format(time.RFC3339)`. The format string `2006-01-02 15:04:05` is **the** Go quirk — memorize it. We use `"2006-01-02"` for date-only buckets.

### Money

- TS `INT minor units` → `number`.
- Go: `int64`. Use `int64` everywhere — `int` is 32 bits on some platforms, breaks at large totals.

### Concurrent map access

- TS: single-threaded — no problem.
- Go: writing a map from two goroutines is a fatal panic. Wrap with `sync.RWMutex` (see `lib/rbac/cache.go`) or use `sync.Map`.

### Error wrapping

- TS: `throw new Error("foo")` loses no information.
- Go: prefer `fmt.Errorf("doing X: %w", err)` to keep the chain. Then `errors.Is(err, target)` walks the chain.

### Exporting

- TS: `export class Foo`.
- Go: capitalize the name. `Foo` is exported, `foo` is package-private. No keyword.

### Imports

- TS: `import { x } from "./y"`.
- Go: full module path: `import "github.com/quickbite/analytics-service/lib/foo"`. The IDE adds them automatically.

### Iteration order

- TS object properties: insertion order (mostly).
- Go maps: **random**. Sort if you need stable output.

---

## 6. When to break the mapping

Don't follow Node patterns when Go has a better-fit idiom:

- **Don't** ask AI to "port my OrderService class to Go" verbatim. Ask it to "implement an OrderService struct in Go that does X" — let the language choose.
- **Don't** wrap stdlib in unnecessary abstractions because TS does (`HttpClient` class is fine; `IHttpClient` interface for one impl is over-engineering in Go).
- **Do** use goroutines + channels for things you'd do with Promise.all + queue libraries in TS.
- **Do** prefer composition (embedding structs) over inheritance — Go has no inheritance and you don't need it.
- **Do** return `(T, error)` tuples; don't fake exceptions with panics.
