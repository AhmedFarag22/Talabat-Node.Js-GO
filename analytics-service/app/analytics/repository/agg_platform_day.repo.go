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

const collAggPlatformDay = "agg_platform_day"

type AggPlatformDayRepo struct {
	col *mongo.Collection
}

func NewAggPlatformDayRepo(db *mongo.Database) *AggPlatformDayRepo {
	return &AggPlatformDayRepo{col: db.Collection(collAggPlatformDay)}
}

func (r *AggPlatformDayRepo) UpsertOrderPlacedDelta(ctx context.Context, date, country, currency string, totalMinor int64) error {
	filter := bson.D{{Key: "date", Value: date}, {Key: "country_code", Value: country}}
	update := bson.D{
		{Key: "$inc", Value: bson.D{
			{Key: "orders_count", Value: 1},
			{Key: "revenue_sum", Value: totalMinor},
		}},
		{Key: "$set", Value: bson.D{
			{Key: "updated_at", Value: time.Now().UTC()},
			{Key: "currency", Value: currency},
		}},
		{Key: "$setOnInsert", Value: bson.D{
			{Key: "date", Value: date},
			{Key: "country_code", Value: country},
		}},
	}
	if _, err := r.col.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true)); err != nil {
		return fmt.Errorf("upsert agg_platform_day placed: %w", err)
	}
	return nil
}

func (r *AggPlatformDayRepo) UpsertDeliveredDelta(ctx context.Context, date, country, currency string, deliveryMs int64) error {
	filter := bson.D{{Key: "date", Value: date}, {Key: "country_code", Value: country}}
	update := bson.D{
		{Key: "$inc", Value: bson.D{
			{Key: "delivered_count", Value: 1},
			{Key: "delivery_ms_sum", Value: deliveryMs},
			{Key: "delivery_ms_count", Value: 1},
		}},
		{Key: "$set", Value: bson.D{{Key: "updated_at", Value: time.Now().UTC()}}},
		{Key: "$setOnInsert", Value: bson.D{
			{Key: "date", Value: date},
			{Key: "country_code", Value: country},
			{Key: "currency", Value: currency},
		}},
	}
	if _, err := r.col.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true)); err != nil {
		return fmt.Errorf("upsert agg_platform_day delivered: %w", err)
	}
	return nil
}

func (r *AggPlatformDayRepo) UpsertFailureDelta(ctx context.Context, date, country, failureField string) error {
	filter := bson.D{{Key: "date", Value: date}, {Key: "country_code", Value: country}}
	update := bson.D{
		{Key: "$inc", Value: bson.D{{Key: failureField, Value: 1}}},
		{Key: "$set", Value: bson.D{{Key: "updated_at", Value: time.Now().UTC()}}},
		{Key: "$setOnInsert", Value: bson.D{
			{Key: "date", Value: date},
			{Key: "country_code", Value: country},
		}},
	}
	if _, err := r.col.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true)); err != nil {
		return fmt.Errorf("upsert agg_platform_day failure: %w", err)
	}
	return nil
}

// FindRange returns platform day rows for date ∈ [from, to]. When country is
// non-nil it filters to that country (one row per date, country set on each).
// When country is nil it aggregates across all countries, grouping by date —
// one row per date with country_code/currency left empty (they're ambiguous
// once summed across markets).
func (r *AggPlatformDayRepo) FindRange(ctx context.Context, fromDate, toDate string, country *string) ([]entity.AggPlatformDay, error) {
	if country != nil {
		filter := bson.D{
			{Key: "country_code", Value: *country},
			{Key: "date", Value: bson.D{{Key: "$gte", Value: fromDate}, {Key: "$lte", Value: toDate}}},
		}
		cur, err := r.col.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "date", Value: 1}}))
		if err != nil {
			return nil, fmt.Errorf("find agg_platform_day range: %w", err)
		}
		defer cur.Close(ctx)
		var rows []entity.AggPlatformDay
		if err := cur.All(ctx, &rows); err != nil {
			return nil, fmt.Errorf("decode agg_platform_day range: %w", err)
		}
		return rows, nil
	}

	// Aggregated across countries: group by date, sum every counter.
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.D{
			{Key: "date", Value: bson.D{{Key: "$gte", Value: fromDate}, {Key: "$lte", Value: toDate}}},
		}}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$date"},
			{Key: "orders_count", Value: bson.D{{Key: "$sum", Value: "$orders_count"}}},
			{Key: "revenue_sum", Value: bson.D{{Key: "$sum", Value: "$revenue_sum"}}},
			{Key: "rejected_count", Value: bson.D{{Key: "$sum", Value: "$rejected_count"}}},
			{Key: "cancelled_count", Value: bson.D{{Key: "$sum", Value: "$cancelled_count"}}},
			{Key: "delivered_count", Value: bson.D{{Key: "$sum", Value: "$delivered_count"}}},
			{Key: "delivery_ms_sum", Value: bson.D{{Key: "$sum", Value: "$delivery_ms_sum"}}},
			{Key: "delivery_ms_count", Value: bson.D{{Key: "$sum", Value: "$delivery_ms_count"}}},
		}}},
		{{Key: "$project", Value: bson.D{
			{Key: "_id", Value: 0},
			{Key: "date", Value: "$_id"},
			{Key: "orders_count", Value: 1},
			{Key: "revenue_sum", Value: 1},
			{Key: "rejected_count", Value: 1},
			{Key: "cancelled_count", Value: 1},
			{Key: "delivered_count", Value: 1},
			{Key: "delivery_ms_sum", Value: 1},
			{Key: "delivery_ms_count", Value: 1},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "date", Value: 1}}}},
	}
	cur, err := r.col.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("aggregate agg_platform_day range: %w", err)
	}
	defer cur.Close(ctx)
	var rows []entity.AggPlatformDay
	if err := cur.All(ctx, &rows); err != nil {
		return nil, fmt.Errorf("decode agg_platform_day aggregate: %w", err)
	}
	return rows, nil
}
