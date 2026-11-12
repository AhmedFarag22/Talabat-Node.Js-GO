package repository

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/quickbite/analytics-service/app/analytics"
	"github.com/quickbite/analytics-service/app/analytics/entity"
)

const collAggProductDay = "agg_product_day"

type AggProductDayRepo struct {
	col *mongo.Collection
}

func NewAggProductDayRepo(db *mongo.Database) *AggProductDayRepo {
	return &AggProductDayRepo{col: db.Collection(collAggProductDay)}
}

// BulkUpsertOrderLines merges every line of one order.placed event into the
// per-product day rows in a single round-trip. One UpdateOne (upsert) per
// product — batched via BulkWrite so a 20-line order is one network call, not
// 20 (the "no N+1" rule). Unordered so independent product rows don't block
// each other.
func (r *AggProductDayRepo) BulkUpsertOrderLines(ctx context.Context, restaurantID int64, date, currency string, lines []analytics.OrderLineInput) error {
	if len(lines) == 0 {
		return nil
	}
	now := time.Now().UTC()
	models := make([]mongo.WriteModel, 0, len(lines))
	for _, ln := range lines {
		filter := bson.D{
			{Key: "restaurant_id", Value: restaurantID},
			{Key: "product_id", Value: ln.ProductID},
			{Key: "date", Value: date},
		}
		update := bson.D{
			{Key: "$inc", Value: bson.D{
				{Key: "units_sum", Value: ln.Quantity},
				{Key: "revenue_sum", Value: ln.LineTotal},
				{Key: "orders_count", Value: 1},
			}},
			{Key: "$set", Value: bson.D{
				{Key: "updated_at", Value: now},
				{Key: "currency", Value: currency},
			}},
			{Key: "$setOnInsert", Value: bson.D{
				{Key: "restaurant_id", Value: restaurantID},
				{Key: "product_id", Value: ln.ProductID},
				{Key: "date", Value: date},
			}},
		}
		models = append(models, mongo.NewUpdateOneModel().SetFilter(filter).SetUpdate(update).SetUpsert(true))
	}
	if _, err := r.col.BulkWrite(ctx, models, options.BulkWrite().SetOrdered(false)); err != nil {
		return fmt.Errorf("bulk upsert agg_product_day: %w", err)
	}
	return nil
}

// FindRange returns per-product day rows for a restaurant where date ∈
// [from, to]. productID is an optional filter (nil → all products for the
// restaurant). Sorted by (date, product_id).
func (r *AggProductDayRepo) FindRange(ctx context.Context, restaurantID int64, productID *int64, fromDate, toDate string) ([]entity.AggProductDay, error) {
	filter := bson.D{
		{Key: "restaurant_id", Value: restaurantID},
		{Key: "date", Value: bson.D{{Key: "$gte", Value: fromDate}, {Key: "$lte", Value: toDate}}},
	}
	if productID != nil {
		filter = append(filter, bson.E{Key: "product_id", Value: *productID})
	}
	sort := options.Find().SetSort(bson.D{{Key: "date", Value: 1}, {Key: "product_id", Value: 1}})
	cur, err := r.col.Find(ctx, filter, sort)
	if err != nil {
		return nil, fmt.Errorf("find agg_product_day range: %w", err)
	}
	defer cur.Close(ctx)

	var rows []entity.AggProductDay
	if err := cur.All(ctx, &rows); err != nil {
		return nil, fmt.Errorf("decode agg_product_day range: %w", err)
	}
	return rows, nil
}
