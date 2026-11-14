// Package mongodb owns the MongoDB client shared by the API and the worker.
//
// Connect fails if the primary does not answer, so a process with a wrong
// MONGO_URI stops at startup instead of failing on its first request.
package mongodb

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

// Client wraps *mongo.Client so the rest of the code depends on a small API.
type Client struct {
	c *mongo.Client
}

// Connect opens a client and verifies that the primary is reachable.
func Connect(ctx context.Context, uri string) (*Client, error) {
	c, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("mongo connect: %w", err)
	}
	cl := &Client{c: c}
	if err := cl.Ping(ctx); err != nil {
		_ = c.Disconnect(context.WithoutCancel(ctx))
		return nil, err
	}
	return cl, nil
}

// Ping checks that the primary answers. Writes and transactions go to the
// primary, so a reachable secondary alone is not enough.
func (cl *Client) Ping(ctx context.Context) error {
	if err := cl.c.Ping(ctx, readpref.Primary()); err != nil {
		return fmt.Errorf("mongo ping: %w", err)
	}
	return nil
}

// Mongo exposes the driver client for packages that need collections.
func (cl *Client) Mongo() *mongo.Client { return cl.c }

// Close disconnects and waits for in-flight operations up to ctx's deadline.
func (cl *Client) Close(ctx context.Context) error {
	if err := cl.c.Disconnect(ctx); err != nil {
		return fmt.Errorf("mongo disconnect: %w", err)
	}
	return nil
}
