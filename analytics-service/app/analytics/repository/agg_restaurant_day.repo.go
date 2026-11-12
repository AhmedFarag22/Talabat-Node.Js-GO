// Package repository is the only place that talks to Mongo. Repos are structs
// with methods so the collection handle is held once at construction. Returns
// entities, never raw bson maps.
package repository

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/quickbite/analytics-service/app/analytics/entity"
)

const collAggRestaurantDay = "agg_restaurant_day"

type AggRestaurantDayRepo struct {
	col *mongo.Collection
}

func NewAggRestaurantDayRepo(db *mongo.Database) *AggRestaurantDayRepo {
	return &AggRestaurantDayRepo{col: db.Collection(collAggRestaurantDay)}
}

// UpsertOrderPlacedDelta merges one order.placed event into the day row keyed
// by (restaurant_id, date). Atomic via $inc + upsert; safe under concurrent
// writes for different orders on the same row.
//
// Currency is set on insert only ($setOnInsert). Day rows for the same
// restaurant + date should never see mixed currencies because a restaurant
// belongs to one country/branch tree — but if they ever do, the first writer
// wins and we'd want a follow-up alert. Out of scope for the slice.
func (r *AggRestaurantDayRepo) UpsertOrderPlacedDelta(ctx context.Context, restaurantID int64, date string, currency string, totalMinor int64) error {
	filter := bson.D{{Key: "restaurant_id", Value: restaurantID}, {Key: "date", Value: date}}
	update := bson.D{
		{Key: "$inc", Value: bson.D{
			{Key: "orders_count", Value: 1},
			{Key: "revenue_sum", Value: totalMinor},
		}},
		// currency comes from the authoritative placed event via $set (not
		// $setOnInsert) so it lands even when a delivered/failure event created
		// the row first.
		{Key: "$set", Value: bson.D{
			{Key: "updated_at", Value: time.Now().UTC()},
			{Key: "currency", Value: currency},
		}},
		{Key: "$setOnInsert", Value: bson.D{
			{Key: "restaurant_id", Value: restaurantID},
			{Key: "date", Value: date},
		}},
	}
	opts := options.Update().SetUpsert(true)
	if _, err := r.col.UpdateOne(ctx, filter, update, opts); err != nil {
		return fmt.Errorf("upsert agg_restaurant_day placed: %w", err)
	}
	return nil
}

// UpsertDeliveredDelta merges one order.delivered event: bumps delivered_count
// and the delivery-time sum/count used to derive the daily average. Keyed by
// the order's placed-day so it lands on the same row order.placed created.
func (r *AggRestaurantDayRepo) UpsertDeliveredDelta(ctx context.Context, restaurantID int64, date, currency string, deliveryMs int64) error {
	filter := bson.D{{Key: "restaurant_id", Value: restaurantID}, {Key: "date", Value: date}}
	update := bson.D{
		{Key: "$inc", Value: bson.D{
			{Key: "delivered_count", Value: 1},
			{Key: "delivery_ms_sum", Value: deliveryMs},
			{Key: "delivery_ms_count", Value: 1},
		}},
		{Key: "$set", Value: bson.D{{Key: "updated_at", Value: time.Now().UTC()}}},
		{Key: "$setOnInsert", Value: bson.D{
			{Key: "restaurant_id", Value: restaurantID},
			{Key: "date", Value: date},
			{Key: "currency", Value: currency},
		}},
	}
	if _, err := r.col.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true)); err != nil {
		return fmt.Errorf("upsert agg_restaurant_day delivered: %w", err)
	}
	return nil
}

// UpsertFailureDelta increments the rejected_count or cancelled_count counter
// (failureField), keyed by the day the failure occurred. Revenue is not
// reversed — failure counts are tracked as their own metric alongside gross
// placed revenue.
func (r *AggRestaurantDayRepo) UpsertFailureDelta(ctx context.Context, restaurantID int64, date, failureField string) error {
	filter := bson.D{{Key: "restaurant_id", Value: restaurantID}, {Key: "date", Value: date}}
	update := bson.D{
		{Key: "$inc", Value: bson.D{{Key: failureField, Value: 1}}},
		{Key: "$set", Value: bson.D{{Key: "updated_at", Value: time.Now().UTC()}}},
		{Key: "$setOnInsert", Value: bson.D{
			{Key: "restaurant_id", Value: restaurantID},
			{Key: "date", Value: date},
		}},
	}
	if _, err := r.col.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true)); err != nil {
		return fmt.Errorf("upsert agg_restaurant_day failure: %w", err)
	}
	return nil
}

// UpsertPaymentDelta records a captured online payment (payment.completed).
// Tracked separately from revenue_sum so it never double-counts the order
// total that order.placed already booked.
func (r *AggRestaurantDayRepo) UpsertPaymentDelta(ctx context.Context, restaurantID int64, date, currency string, amountMinor int64) error {
	filter := bson.D{{Key: "restaurant_id", Value: restaurantID}, {Key: "date", Value: date}}
	update := bson.D{
		{Key: "$inc", Value: bson.D{
			{Key: "payments_count", Value: 1},
			{Key: "payments_sum", Value: amountMinor},
		}},
		{Key: "$set", Value: bson.D{{Key: "updated_at", Value: time.Now().UTC()}}},
		{Key: "$setOnInsert", Value: bson.D{
			{Key: "restaurant_id", Value: restaurantID},
			{Key: "date", Value: date},
			{Key: "currency", Value: currency},
		}},
	}
	if _, err := r.col.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true)); err != nil {
		return fmt.Errorf("upsert agg_restaurant_day payment: %w", err)
	}
	return nil
}

// CountActiveOnDate counts restaurants that placed at least one order on the
// given day — i.e. agg_restaurant_day rows with orders_count > 0. Backs
// GET /analytics/platform/active-restaurants?date=. Uses the
// (date, restaurant_id) index; orders_count is a residual filter.
func (r *AggRestaurantDayRepo) CountActiveOnDate(ctx context.Context, date string) (int64, error) {
	filter := bson.D{
		{Key: "date", Value: date},
		{Key: "orders_count", Value: bson.D{{Key: "$gt", Value: 0}}},
	}
	n, err := r.col.CountDocuments(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("count active restaurants: %w", err)
	}
	return n, nil
}

// FindRange returns rows for one restaurant where date ∈ [from, to] inclusive.
// Dates are "YYYY-MM-DD" UTC strings, lexicographically sortable. Uses the
// (date, restaurant_id) index.
func (r *AggRestaurantDayRepo) FindRange(ctx context.Context, restaurantID int64, fromDate, toDate string) ([]entity.AggRestaurantDay, error) {
	filter := bson.D{
		{Key: "restaurant_id", Value: restaurantID},
		{Key: "date", Value: bson.D{{Key: "$gte", Value: fromDate}, {Key: "$lte", Value: toDate}}},
	}
	cur, err := r.col.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "date", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("find agg_restaurant_day range: %w", err)
	}
	defer cur.Close(ctx)

	var rows []entity.AggRestaurantDay
	if err := cur.All(ctx, &rows); err != nil {
		return nil, fmt.Errorf("decode agg_restaurant_day range: %w", err)
	}
	return rows, nil
}
