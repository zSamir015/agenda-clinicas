// Package migrate defines the ordered migrations for the platform database.
package migrate

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// PlatformMigrations returns the ordered list of migrations for the platform database.
func PlatformMigrations() []Migration {
	return []Migration{
		{
			Version: 1,
			Name:    "clinics_slug_unique_index",
			Up: func(ctx context.Context, db *mongo.Database) error {
				model := mongo.IndexModel{
					Keys:    bson.D{{Key: "slug", Value: 1}},
					Options: options.Index().SetUnique(true).SetName("slug_unique"),
				}
				_, err := db.Collection("clinics").Indexes().CreateOne(ctx, model)
				return err
			},
		},
	}
}
