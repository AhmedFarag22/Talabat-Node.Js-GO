package repository

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// EnsureIndexes declares every index this module needs. Mongo's CreateIndexes
// is idempotent, so this is our equivalent of running migrations. Called once
// at boot from lib/boot.
//
// Add new collections here when the slice grows; do not scatter CreateIndexes
// calls across repos.
func EnsureIndexes(ctx context.Context, db *mongo.Database) error {
	models := map[string][]mongo.IndexModel{
		collAggRestaurantDay: {
			// Unique upsert key — consumer writes are keyed (restaurant_id, date).
			{
				Keys:    bson.D{{Key: "restaurant_id", Value: 1}, {Key: "date", Value: 1}},
				Options: options.Index().SetName("uq_restaurant_day").SetUnique(true),
			},
			// Range queries: GET /analytics/restaurants/:id/days?from=&to=
			{
				Keys:    bson.D{{Key: "date", Value: 1}, {Key: "restaurant_id", Value: 1}},
				Options: options.Index().SetName("idx_date_restaurant"),
			},
		},
		collAggBranchDay: {
			// Unique upsert key — consumer writes are keyed (branch_id, date).
			{
				Keys:    bson.D{{Key: "branch_id", Value: 1}, {Key: "date", Value: 1}},
				Options: options.Index().SetName("uq_branch_day").SetUnique(true),
			},
			// Range queries: GET /analytics/branches/:id/days?from=&to=
			{
				Keys:    bson.D{{Key: "date", Value: 1}, {Key: "branch_id", Value: 1}},
				Options: options.Index().SetName("idx_date_branch"),
			},
		},
		collAggProductDay: {
			// Unique upsert key — one row per (restaurant, product, day).
			{
				Keys:    bson.D{{Key: "restaurant_id", Value: 1}, {Key: "product_id", Value: 1}, {Key: "date", Value: 1}},
				Options: options.Index().SetName("uq_restaurant_product_day").SetUnique(true),
			},
			// GET /analytics/products?restaurantId=&productId=&from=&to= — the
			// productId filter is optional, so this (restaurant_id, date) index
			// serves the restaurant-only case; productId narrows as a residual.
			{
				Keys:    bson.D{{Key: "restaurant_id", Value: 1}, {Key: "date", Value: 1}},
				Options: options.Index().SetName("idx_restaurant_date"),
			},
		},
		collAggPlatformDay: {
			// Unique upsert key — one row per (date, country). Date-first so a
			// bare date-range scan (country aggregated) uses the prefix.
			{
				Keys:    bson.D{{Key: "date", Value: 1}, {Key: "country_code", Value: 1}},
				Options: options.Index().SetName("uq_platform_day").SetUnique(true),
			},
			// GET /analytics/platform/days?...&country_code= — equality on
			// country, range on date.
			{
				Keys:    bson.D{{Key: "country_code", Value: 1}, {Key: "date", Value: 1}},
				Options: options.Index().SetName("idx_country_date"),
			},
		},
		collEventIDs: {
			// Idempotency guard. Unique → InsertOne returns dup-key on replay.
			{
				Keys:    bson.D{{Key: "event_id", Value: 1}},
				Options: options.Index().SetName("uq_event_id").SetUnique(true),
			},
			// TTL: drop dedupe markers after 7 days.
			{
				Keys:    bson.D{{Key: "received_at", Value: 1}},
				Options: options.Index().SetName("ttl_received_at").SetExpireAfterSeconds(7 * 24 * 3600),
			},
		},
	}
	for coll, idx := range models {
		if _, err := db.Collection(coll).Indexes().CreateMany(ctx, idx); err != nil {
			return fmt.Errorf("%s: %w", coll, err)
		}
	}
	return nil
}
