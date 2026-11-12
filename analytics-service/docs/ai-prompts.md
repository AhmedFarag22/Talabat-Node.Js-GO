# Using AI to learn this service (and any new language)

This service is a teaching artifact. The recorded video shows one full vertical slice; the rest of the system is left as homework. The expectation is that you'll use AI (Claude Code, Cursor, Copilot, ChatGPT) to do the homework — the goal of the homework isn't "did you write the code by hand?" but **"can you direct an AI through a non-trivial implementation in a language you don't know yet?"**

The strategies below are what works well in practice. They're not Go-specific.

---

## 1. Use the codebase as the prompt

The single biggest mistake when learning a new language with AI is asking generic questions like "how do I write a service in Go?" You'll get textbook answers that don't match your codebase.

Instead: **point the AI at the existing slice and ask it to mirror the pattern.**

> Read `app/analytics/service/analytics.service.go`, `app/analytics/repository/agg_restaurant_day.repo.go`, and `pkg/mongo/indexes.go`.
>
> I want to add `agg_branch_day`. Implement:
> 1. The new collection's indexes in `pkg/mongo/indexes.go` (mirror the existing pattern).
> 2. An `entity` struct.
> 3. A `Repo` with `UpsertOrderPlacedDelta` and `FindRange` mirroring `AggRestaurantDayRepo`.
>
> Use the same naming, file layout, and bson tags. Do not invent new patterns.

This works because:
- The AI sees your conventions in context.
- "Mirror the pattern" is a check the AI can self-verify.
- You can review by diffing — does the new file look like its neighbour?

---

## 2. Make the AI prove it understood the existing code

Before asking for code, ask the AI to summarise what you have. This catches misunderstandings cheaply.

> In one paragraph: explain what `lib/coreevents/consumer.go` does, including how it handles duplicates, unknown event types, and handler errors. Do not write any code.

If the summary is wrong, your follow-up implementation request would have been wrong too. Iterate on understanding before iterating on code.

---

## 3. Ask for the Node analogue, then the Go translation

For anyone porting from Node: this is the single highest-leverage prompt pattern.

> The Node order-service has `lib/auth/rbac.ts` which exposes `rbac("perm")` middleware that pulls perms from a Redis cache. I want the equivalent for our analytics-service in Go. We have `lib/rbac/cache.go` (in-process map + TTL) and need to wire `Require("analytics:read")`.
>
> Show me:
> 1. What the Node code does (3 bullets).
> 2. What the Go equivalent should do (3 bullets).
> 3. Where the implementations differ and why (1 paragraph).
> 4. The Go code.

The first three steps make the AI compare the two codebases explicitly. By the time you get to step 4, you've already caught the conceptual gaps.

---

## 4. Constrain the AI with the rules from CLAUDE.md

Most "the AI wrote bad code" complaints are because the AI didn't know the rules. This codebase ships its rules in `CLAUDE.md`. Reference them.

> Read `CLAUDE.md` §6 (response shape). I want to add a `GET /analytics/platform/days?from=&to=` endpoint. Follow that section's rules exactly: response DTO in `dto/`, money in minor units, no entity leakage. Show me the file diff before writing anything.

Most agentic tools (Claude Code, Cursor's composer) will load `CLAUDE.md` automatically — but it doesn't hurt to point at the section.

---

## 5. Use the AI as a Go tutor, not a Go transpiler

Ask **why** Go does something a certain way. This builds the mental model that makes the next ask easier.

Bad:

> Convert this TS class to Go.

Good:

> In TypeScript I would write `class Foo { constructor(deps) { ... } }`. In Go I see `type Foo struct{...}` + `func NewFoo(...) *Foo`. Why? What's the trade-off? Specifically:
> - Why isn't there a single `class` keyword?
> - Why is the constructor a separate function with a `New` prefix?
> - When would I return `*Foo` vs `Foo`?

The answers will explain Go's "no inheritance, only composition" philosophy in concrete terms.

---

## 6. Get the AI to predict failures before running

Before you run the new code, ask:

> Given this implementation of `OnOrderDelivered`, what could go wrong?
> - Out-of-order delivery (delivered before placed)
> - Duplicate event id
> - Missing fields in the payload
> - Mongo connection drops mid-upsert
>
> For each: does my code handle it correctly? If not, what should change?

This catches at least two categories of bug before you launch the publisher.

---

## 7. Specific prompts for the homework

### Adding a new event handler (e.g. `payment.completed`)

```
Read:
- lib/coreevents/payloads.go
- app/analytics/eventhandlers/handlers.go
- app/analytics/service/analytics.service.go

Add a handler for `payment.completed` (event from order-service when an
online payment succeeds). The payload includes orderId, restaurantId,
amount, currency, paidAt.

Implement (mirror order.placed):
1. PaymentCompletedPayload struct in lib/coreevents/payloads.go
2. handlePaymentCompleted in app/analytics/eventhandlers/handlers.go
3. svc.OnPaymentCompleted in app/analytics/service/analytics.service.go
4. Register in eventhandlers.For(svc) map

Decide: does payment.completed update the same agg_restaurant_day row as
order.placed, or is it a separate metric? Justify in one sentence.
```

### Adding a new aggregate collection (e.g. `agg_branch_day`)

```
Mirror agg_restaurant_day end-to-end. Files to add (use exactly these names):
- pkg/mongo/indexes.go: ensureAggBranchDayIndexes
- app/analytics/entity/agg_branch_day.go
- app/analytics/repository/agg_branch_day.repo.go
- app/analytics/service: extend with branch-keyed methods (don't make a separate service)
- app/analytics/controller: GetBranchDays handler
- app/analytics/dto: BranchDayResponseDTO
- app/analytics/controller/routes.go: mount the new route

The wire shape and query params should look exactly like /restaurants/:id/days.
```

### Adding the rbac.permissions_changed consumer

```
The video skipped this on purpose. Implement it now.

1. In lib/coreevents/payloads.go: add RBACPermissionsChangedPayload {role string}.
2. In app/analytics/eventhandlers/handlers.go:
   - Add handleRBACChanged that calls rbacCache.Invalidate(payload.role).
   - rbacCache is currently held in di.Container, but eventhandlers/handlers.go
     only takes *service.AnalyticsService. Decide: extend the For() signature
     to take rbacCache too, OR inject the cache into the service. Pick one,
     justify in 2 lines.
3. Register in the For() map under "rbac.permissions_changed".
4. Make sure RABBITMQ_ORDER_EVENTS_BINDINGS in .env.example includes "rbac.#"
   (or rename the binding to a more general name).
```

---

## 8. When the AI is wrong

It will be. Specifically:

- **Hallucinated functions in `mongo-driver`.** Always sanity-check by running `go doc go.mongodb.org/mongo-driver/mongo.Collection.UpdateOne`. The AI sometimes invents method names.
- **Wrong bson tags.** It might write `bson:"restaurantId"` (camelCase) when the rest of the codebase uses `bson:"restaurant_id"`. Compare to neighbouring files.
- **Forgetting `ctx context.Context`.** Every method that does I/O takes ctx as the first arg. If the AI omits it, push back.
- **Inventing new envelope shapes.** The Envelope struct in `lib/coreevents/payloads.go` is fixed by the producer (`order-service`). Don't let the AI "improve" it.

When this happens, paste the offending diff back with `this is wrong because <reason>; fix it specifically by <change>`. Don't re-prompt from scratch — the AI already has the context.

---

## 9. The meta-skill

What this exercise teaches you isn't Go. It's:

> How to direct an AI through a non-trivial implementation in a language you don't know yet, by anchoring it to a codebase whose patterns you do know.

That's the skill that scales. Next month it'll be Rust analytics, the month after Java payments. The language changes; the technique doesn't.
