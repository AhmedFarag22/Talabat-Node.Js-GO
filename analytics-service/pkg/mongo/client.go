// Package mongo wraps the official mongo-driver to expose a single
// `Connect` returning a database handle and a close function. App-agnostic.
package mongo

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Connect dials the cluster, pings to verify, and returns the database handle
// plus a closer the caller wires into graceful shutdown.
func Connect(ctx context.Context, uri, dbName string) (*mongo.Database, func(context.Context) error, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(dialCtx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, nil, fmt.Errorf("mongo connect: %w", err)
	}
	if err := client.Ping(dialCtx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, nil, fmt.Errorf("mongo ping: %w", err)
	}
	return client.Database(dbName), client.Disconnect, nil
}
