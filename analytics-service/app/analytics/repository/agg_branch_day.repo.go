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

const collAggBranchDay = "agg_branch_day"

// AggBranchDayRepo is the branch-keyed twin of AggRestaurantDayRepo. The delta
// methods mirror it field-for-field so the consumer can update both rows from
// the same event with parallel calls.
type AggBranchDayRepo struct {
	col *mongo.Collection
}

func NewAggBranchDayRepo(db *mongo.Database) *AggBranchDayRepo {
	return &AggBranchDayRepo{col: db.Collection(collAggBranchDay)}
}

func (r *AggBranchDayRepo) UpsertOrderPlacedDelta(ctx context.Context, branchID int64, date, currency string, totalMinor int64) error {
	filter := bson.D{{Key: "branch_id", Value: branchID}, {Key: "date", Value: date}}
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
			{Key: "branch_id", Value: branchID},
			{Key: "date", Value: date},
		}},
	}
	if _, err := r.col.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true)); err != nil {
		return fmt.Errorf("upsert agg_branch_day placed: %w", err)
	}
	return nil
}

func (r *AggBranchDayRepo) UpsertDeliveredDelta(ctx context.Context, branchID int64, date, currency string, deliveryMs int64) error {
	filter := bson.D{{Key: "branch_id", Value: branchID}, {Key: "date", Value: date}}
	update := bson.D{
		{Key: "$inc", Value: bson.D{
			{Key: "delivered_count", Value: 1},
			{Key: "delivery_ms_sum", Value: deliveryMs},
			{Key: "delivery_ms_count", Value: 1},
		}},
		{Key: "$set", Value: bson.D{{Key: "updated_at", Value: time.Now().UTC()}}},
		{Key: "$setOnInsert", Value: bson.D{
			{Key: "branch_id", Value: branchID},
			{Key: "date", Value: date},
			{Key: "currency", Value: currency},
		}},
	}
	if _, err := r.col.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true)); err != nil {
		return fmt.Errorf("upsert agg_branch_day delivered: %w", err)
	}
	return nil
}

func (r *AggBranchDayRepo) UpsertFailureDelta(ctx context.Context, branchID int64, date, failureField string) error {
	filter := bson.D{{Key: "branch_id", Value: branchID}, {Key: "date", Value: date}}
	update := bson.D{
		{Key: "$inc", Value: bson.D{{Key: failureField, Value: 1}}},
		{Key: "$set", Value: bson.D{{Key: "updated_at", Value: time.Now().UTC()}}},
		{Key: "$setOnInsert", Value: bson.D{
			{Key: "branch_id", Value: branchID},
			{Key: "date", Value: date},
		}},
	}
	if _, err := r.col.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true)); err != nil {
		return fmt.Errorf("upsert agg_branch_day failure: %w", err)
	}
	return nil
}

func (r *AggBranchDayRepo) UpsertPaymentDelta(ctx context.Context, branchID int64, date, currency string, amountMinor int64) error {
	filter := bson.D{{Key: "branch_id", Value: branchID}, {Key: "date", Value: date}}
	update := bson.D{
		{Key: "$inc", Value: bson.D{
			{Key: "payments_count", Value: 1},
			{Key: "payments_sum", Value: amountMinor},
		}},
		{Key: "$set", Value: bson.D{{Key: "updated_at", Value: time.Now().UTC()}}},
		{Key: "$setOnInsert", Value: bson.D{
			{Key: "branch_id", Value: branchID},
			{Key: "date", Value: date},
			{Key: "currency", Value: currency},
		}},
	}
	if _, err := r.col.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true)); err != nil {
		return fmt.Errorf("upsert agg_branch_day payment: %w", err)
	}
	return nil
}

// FindRange returns rows for one branch where date ∈ [from, to] inclusive,
// sorted by date. Uses the (date, branch_id) index.
func (r *AggBranchDayRepo) FindRange(ctx context.Context, branchID int64, fromDate, toDate string) ([]entity.AggBranchDay, error) {
	filter := bson.D{
		{Key: "branch_id", Value: branchID},
		{Key: "date", Value: bson.D{{Key: "$gte", Value: fromDate}, {Key: "$lte", Value: toDate}}},
	}
	cur, err := r.col.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "date", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("find agg_branch_day range: %w", err)
	}
	defer cur.Close(ctx)

	var rows []entity.AggBranchDay
	if err := cur.All(ctx, &rows); err != nil {
		return nil, fmt.Errorf("decode agg_branch_day range: %w", err)
	}
	return rows, nil
}
