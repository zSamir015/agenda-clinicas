package platform

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ClinicRepository stores clinics in platform.clinics.
type ClinicRepository struct {
	col *mongo.Collection
}

// NewClinicRepository returns a repository over db and makes sure the unique
// slug index exists. Creating an existing index is a no-op, so this is safe on
// every start; the migration runner owns it long term (task 4).
func NewClinicRepository(ctx context.Context, db *mongo.Database) (*ClinicRepository, error) {
	col := db.Collection("clinics")
	_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "slug", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("slug_unique"),
	})
	if err != nil {
		return nil, fmt.Errorf("create clinics slug index: %w", err)
	}
	return &ClinicRepository{col: col}, nil
}

// Create stores c and returns it with ID and CreatedAt filled in.
func (r *ClinicRepository) Create(ctx context.Context, c Clinic) (Clinic, error) {
	c.CreatedAt = time.Now().UTC().Truncate(time.Millisecond)
	res, err := r.col.InsertOne(ctx, c)
	if mongo.IsDuplicateKeyError(err) {
		return Clinic{}, fmt.Errorf("%w: %s", ErrSlugTaken, c.Slug)
	}
	if err != nil {
		return Clinic{}, fmt.Errorf("insert clinic: %w", err)
	}
	c.ID = res.InsertedID.(bson.ObjectID)
	return c, nil
}

// BySlug returns the clinic with the given slug or ErrTenantNotFound.
func (r *ClinicRepository) BySlug(ctx context.Context, slug string) (Clinic, error) {
	var c Clinic
	err := r.col.FindOne(ctx, bson.D{{Key: "slug", Value: slug}}).Decode(&c)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return Clinic{}, fmt.Errorf("%w: %s", ErrTenantNotFound, slug)
	}
	if err != nil {
		return Clinic{}, fmt.Errorf("find clinic: %w", err)
	}
	return c, nil
}

// List returns every clinic ordered by slug.
func (r *ClinicRepository) List(ctx context.Context) ([]Clinic, error) {
	cur, err := r.col.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "slug", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list clinics: %w", err)
	}
	var out []Clinic
	if err := cur.All(ctx, &out); err != nil {
		return nil, fmt.Errorf("decode clinics: %w", err)
	}
	return out, nil
}
