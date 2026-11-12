package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const collEventIDs = "event_ids"

type EventIDsRepo struct {
	col *mongo.Collection
}

func NewEventIDsRepo(db *mongo.Database) *EventIDsRepo {
	return &EventIDsRepo{col: db.Collection(collEventIDs)}
}

// MarkSeen inserts a dedupe marker for an event id. Returns true on first
// sight, false on duplicate (so the caller can ack-and-skip). Backed by the
// unique index on event_id.
//
// TTL on received_at expires the marker after 7 days — long enough for any
// realistic redelivery window, short enough to keep the collection bounded.
func (r *EventIDsRepo) MarkSeen(ctx context.Context, eventID string) (bool, error) {
	doc := bson.D{
		{Key: "event_id", Value: eventID},
		{Key: "received_at", Value: time.Now().UTC()},
	}
	if _, err := r.col.InsertOne(ctx, doc); err != nil {
		if mongo.IsDuplicateKeyError(err) || isDup(err) {
			return false, nil
		}
		return false, fmt.Errorf("insert event id: %w", err)
	}
	return true, nil
}

func isDup(err error) bool {
	var we mongo.WriteException
	if errors.As(err, &we) {
		for _, e := range we.WriteErrors {
			if e.Code == 11000 {
				return true
			}
		}
	}
	return false
}
