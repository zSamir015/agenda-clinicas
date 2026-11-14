// Package tenantdb provides clinic-scoped MongoDB repositories that enforce
// tenant isolation on every query, aggregation, and mutation.
package tenantdb

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/zSamir015/agenda-clinicas/backend/internal/platform"
)

const defaultSharedDB = "operations"

var (
	// ErrNoTenant is returned when a repository method is called without a clinic in ctx.
	ErrNoTenant = errors.New("tenantdb: no clinic in context")

	// ErrClinicMismatch is returned when an inserted document carries a clinicId
	// different from the clinic in ctx.
	ErrClinicMismatch = errors.New("tenantdb: document clinicId does not match context clinic")
)

// Repo wraps a MongoDB collection and enforces clinic isolation using the
// clinic stored in context via platform.WithTenant.
type Repo[T any] struct {
	client     *mongo.Client
	sharedDB   string
	collection string
}

// NewRepo creates a clinic-scoped repository for the given collection.
// When sharedDB is empty, it defaults to "operations". Clinics on the
// dedicated tier always resolve to their own "clinic_<id>" database.
func NewRepo[T any](client *mongo.Client, sharedDB, collection string) *Repo[T] {
	if sharedDB == "" {
		sharedDB = defaultSharedDB
	}
	return &Repo[T]{
		client:     client,
		sharedDB:   sharedDB,
		collection: collection,
	}
}

// DatabaseName returns the MongoDB database name used for the given clinic.
func (r *Repo[T]) DatabaseName(c platform.Clinic) string {
	if c.Tier == platform.TierDedicated {
		return "clinic_" + c.ID.Hex()
	}
	return r.sharedDB
}

func (r *Repo[T]) resolve(ctx context.Context) (*mongo.Collection, platform.Clinic, error) {
	c, ok := platform.TenantFrom(ctx)
	if !ok || c.ID.IsZero() {
		return nil, platform.Clinic{}, ErrNoTenant
	}
	return r.client.Database(r.DatabaseName(c)).Collection(r.collection), c, nil
}

// scopeFilter wraps caller-supplied filters with $and so a filter containing
// clinicId cannot override the context clinic.
func scopeFilter(clinicID bson.ObjectID, filter any) bson.D {
	tenantCond := bson.D{{Key: "clinicId", Value: clinicID}}
	if filter == nil {
		return tenantCond
	}
	return bson.D{{Key: "$and", Value: bson.A{tenantCond, filter}}}
}

// Find returns all documents matching filter within the context clinic.
func (r *Repo[T]) Find(ctx context.Context, filter any, opts ...options.Lister[options.FindOptions]) ([]T, error) {
	col, c, err := r.resolve(ctx)
	if err != nil {
		return nil, err
	}
	cur, err := col.Find(ctx, scopeFilter(c.ID, filter), opts...)
	if err != nil {
		return nil, fmt.Errorf("tenantdb: find in %s: %w", r.collection, err)
	}
	defer func() { _ = cur.Close(ctx) }()

	var out []T
	if err := cur.All(ctx, &out); err != nil {
		return nil, fmt.Errorf("tenantdb: decode %s: %w", r.collection, err)
	}
	return out, nil
}

// FindOne returns the first document matching filter within the context clinic.
func (r *Repo[T]) FindOne(ctx context.Context, filter any, opts ...options.Lister[options.FindOneOptions]) (T, error) {
	var zero T
	col, c, err := r.resolve(ctx)
	if err != nil {
		return zero, err
	}
	var out T
	if err := col.FindOne(ctx, scopeFilter(c.ID, filter), opts...).Decode(&out); err != nil {
		return zero, fmt.Errorf("tenantdb: findOne in %s: %w", r.collection, err)
	}
	return out, nil
}

// Insert inserts a document, setting clinicId from the context clinic and
// rejecting any document that already carries a different clinicId.
func (r *Repo[T]) Insert(ctx context.Context, doc *T) error {
	col, c, err := r.resolve(ctx)
	if err != nil {
		return err
	}
	payload, err := prepareInsertDoc(doc, c.ID)
	if err != nil {
		return err
	}
	if _, err := col.InsertOne(ctx, payload); err != nil {
		return fmt.Errorf("tenantdb: insert in %s: %w", r.collection, err)
	}
	return nil
}

func prepareInsertDoc(doc any, clinicID bson.ObjectID) (bson.D, error) {
	raw, err := bson.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("tenantdb: marshal insert doc: %w", err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("tenantdb: unmarshal insert doc: %w", err)
	}
	for i, elem := range d {
		if elem.Key != "clinicId" {
			continue
		}
		existing, ok := elem.Value.(bson.ObjectID)
		if !ok {
			return nil, ErrClinicMismatch
		}
		if existing.IsZero() {
			d[i].Value = clinicID
			return d, nil
		}
		if existing != clinicID {
			return nil, ErrClinicMismatch
		}
		return d, nil
	}
	return append(d, bson.E{Key: "clinicId", Value: clinicID}), nil
}

// UpdateOne updates a single document matching filter within the context clinic.
func (r *Repo[T]) UpdateOne(ctx context.Context, filter any, update any, opts ...options.Lister[options.UpdateOneOptions]) (*mongo.UpdateResult, error) {
	col, c, err := r.resolve(ctx)
	if err != nil {
		return nil, err
	}
	res, err := col.UpdateOne(ctx, scopeFilter(c.ID, filter), update, opts...)
	if err != nil {
		return nil, fmt.Errorf("tenantdb: updateOne in %s: %w", r.collection, err)
	}
	return res, nil
}

// UpdateMany updates all documents matching filter within the context clinic.
func (r *Repo[T]) UpdateMany(ctx context.Context, filter any, update any, opts ...options.Lister[options.UpdateManyOptions]) (*mongo.UpdateResult, error) {
	col, c, err := r.resolve(ctx)
	if err != nil {
		return nil, err
	}
	res, err := col.UpdateMany(ctx, scopeFilter(c.ID, filter), update, opts...)
	if err != nil {
		return nil, fmt.Errorf("tenantdb: updateMany in %s: %w", r.collection, err)
	}
	return res, nil
}

// DeleteOne deletes a single document matching filter within the context clinic.
func (r *Repo[T]) DeleteOne(ctx context.Context, filter any, opts ...options.Lister[options.DeleteOneOptions]) (*mongo.DeleteResult, error) {
	col, c, err := r.resolve(ctx)
	if err != nil {
		return nil, err
	}
	res, err := col.DeleteOne(ctx, scopeFilter(c.ID, filter), opts...)
	if err != nil {
		return nil, fmt.Errorf("tenantdb: deleteOne in %s: %w", r.collection, err)
	}
	return res, nil
}

// DeleteMany deletes all documents matching filter within the context clinic.
func (r *Repo[T]) DeleteMany(ctx context.Context, filter any, opts ...options.Lister[options.DeleteManyOptions]) (*mongo.DeleteResult, error) {
	col, c, err := r.resolve(ctx)
	if err != nil {
		return nil, err
	}
	res, err := col.DeleteMany(ctx, scopeFilter(c.ID, filter), opts...)
	if err != nil {
		return nil, fmt.Errorf("tenantdb: deleteMany in %s: %w", r.collection, err)
	}
	return res, nil
}

// Aggregate executes an aggregation pipeline with a leading $match stage scoped
// to the context clinic.
func (r *Repo[T]) Aggregate(ctx context.Context, pipeline mongo.Pipeline, opts ...options.Lister[options.AggregateOptions]) ([]bson.M, error) {
	col, c, err := r.resolve(ctx)
	if err != nil {
		return nil, err
	}
	scoped := make(mongo.Pipeline, 0, len(pipeline)+1)
	scoped = append(scoped, bson.D{{Key: "$match", Value: bson.D{{Key: "clinicId", Value: c.ID}}}})
	scoped = append(scoped, pipeline...)

	cur, err := col.Aggregate(ctx, scoped, opts...)
	if err != nil {
		return nil, fmt.Errorf("tenantdb: aggregate in %s: %w", r.collection, err)
	}
	defer func() { _ = cur.Close(ctx) }()

	var out []bson.M
	if err := cur.All(ctx, &out); err != nil {
		return nil, fmt.Errorf("tenantdb: decode aggregate %s: %w", r.collection, err)
	}
	return out, nil
}

// ScopedProbe allows the isolation test suite to exercise any registered
// clinic-scoped repository uniformly without knowing its domain model.
type ScopedProbe interface {
	InsertSample(ctx context.Context, marker string) error
	CountVisible(ctx context.Context, filter any) (int64, error)
	UpdateSample(ctx context.Context, filter any) (int64, error)
	DeleteSample(ctx context.Context, filter any) (int64, error)
	AggregateCount(ctx context.Context) (int64, error)
}

// Registration describes a clinic-scoped repository registered for isolation verification.
type Registration struct {
	Name       string
	Collection string
	NewProbe   func(client *mongo.Client) ScopedProbe
}

var (
	registryMu sync.RWMutex
	registry   = map[string]Registration{}
)

// Register adds a repository to the global isolation test registry.
// Domain packages should register their repositories so isolation_test.go
// verifies every clinic-scoped collection.
func Register(reg Registration) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[reg.Collection] = reg
}

// Registered returns all registered clinic-scoped repositories.
func Registered() []Registration {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]Registration, 0, len(registry))
	for _, r := range registry {
		out = append(out, r)
	}
	return out
}
