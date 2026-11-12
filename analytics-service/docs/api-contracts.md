# API contracts

All endpoints are mounted under `/api/v1`. All responses use the standard envelope:

**Success:**
```json
{ "success": true, "data": { ... } }
```

**Error:**
```json
{ "success": false, "error": { "code": "ERROR_CODE", "message": "human-readable" } }
```

All requests must include the access cookie or `Authorization: Bearer <jwt>` header. RBAC is checked per-route below.

---

## In the video

### `GET /api/v1/analytics/restaurants/:restaurantId/days?from=&to=`

Daily aggregate rollups for one restaurant in a date range.

| Param         | Where    | Type          | Notes                              |
| ------------- | -------- | ------------- | ---------------------------------- |
| restaurantId  | path     | int64 (>0)    | required                           |
| from          | query    | YYYY-MM-DD    | required                           |
| to            | query    | YYYY-MM-DD    | required, ≥ from, ≤ from + 366 days |

**Auth:** access cookie/Bearer + permission `analytics:read`.

**Response 200:**
```json
{
  "success": true,
  "data": [
    {
      "date": "2026-05-01",
      "ordersCount": 14,
      "revenueMinor": 28500,
      "currency": "EGP",
      "avgOrderMinor": 2035
    },
    {
      "date": "2026-05-02",
      "ordersCount": 19,
      "revenueMinor": 41200,
      "currency": "EGP",
      "avgOrderMinor": 2168
    }
  ]
}
```

**Errors:**

| Code                            | HTTP | When                                                  |
| ------------------------------- | ---- | ----------------------------------------------------- |
| `UNAUTHENTICATED`               | 401  | Missing or invalid access token                       |
| `FORBIDDEN`                     | 403  | Token is valid but role lacks `analytics:read`        |
| `ANALYTICS_INVALID_RESTAURANT_ID` | 400  | Path param is not a positive integer                  |
| `ANALYTICS_INVALID_DATE_RANGE`  | 400  | `from`/`to` malformed, `to < from`, or range > 1 year |
| `VALIDATION_ERROR`              | 400  | `from` or `to` missing                                |
| `RBAC_LOOKUP_FAILED`            | 503  | core-service unreachable for permission lookup        |
| `INTERNAL_ERROR`                | 500  | Unhandled                                             |

### `GET /health`

No auth. Returns `{ "success": true, "data": { "status": "ok" } }` if the process is up. Does **not** ping mongo or rabbit (intentional — readiness probes belong on a separate endpoint).

---

## Homework endpoints (not implemented)

To be implemented by students; same shape conventions apply.

### `GET /api/v1/analytics/branches/:branchId/days?from=&to=`
Same as `/restaurants/:id/days` but keyed on branch.

### `GET /api/v1/analytics/products?restaurantId=&productId=&from=&to=`
Per-product daily rollups. `restaurantId` and `productId` are query params (not path) because filters are non-strict.

### `GET /api/v1/analytics/platform/days?from=&to=&country_code=`
Whole-platform per-day metrics, optionally filtered by country.

### `GET /api/v1/analytics/restaurants/:restaurantId/failures?from=&to=`
Daily failure counts (rejections + cancellations).

### `GET /api/v1/analytics/platform/failures?from=&to=`
Same, platform-wide.

### `GET /api/v1/analytics/restaurants/:restaurantId/delivery-avg?from=&to=`
Average delivery time in minutes per day. Derived from `delivery_ms_sum / delivery_ms_count`.

### `GET /api/v1/analytics/platform/active-restaurants?date=`
Count of restaurants that placed at least one order on `date`.

---

## Inbound async (RabbitMQ)

Consumed from exchange `order.events` (topic), bound to `order.#` and `payment.#`. The envelope shape is fixed by `order-service`'s outbox drainer:

```json
{
  "eventId": "uuid",
  "eventType": "order.placed",
  "occurredAt": "2026-05-11T12:34:56Z",
  "aggregateType": "order",
  "aggregateId": "uuid",
  "region": "eg",
  "payload": { ... event-specific shape ... }
}
```

### `order.placed` payload

```json
{
  "orderId": "uuid",
  "region": "eg",
  "countryCode": "eg",
  "restaurantId": 42,
  "branchId": 7,
  "customerId": 101,
  "status": "placed",
  "paymentMethod": "cod",
  "subtotal": 2400,
  "deliveryFee": 100,
  "serviceFee": 10,
  "total": 2510,
  "currency": "EGP",
  "items": [
    { "productId": 1, "quantity": 2, "unitPrice": 1000, "lineTotal": 2000 },
    { "productId": 2, "quantity": 1, "unitPrice": 400,  "lineTotal": 400  }
  ],
  "placedAt": "2026-05-11T12:34:56Z"
}
```

Other payloads (`payment.completed`, `order.delivered`, `order.rejected`) — homework. The producer side is in `order-service`, see `lib/events/event-types.ts` for the canonical list.
