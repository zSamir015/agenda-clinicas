package migrate

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type collectionSpec struct {
	version int
	name    string
	coll    string
	schema  bson.D
	indexes []mongo.IndexModel
}

// OperationsMigrations returns the ordered migrations for clinic operational
// collections ("operations" for standard clinics, "clinic_<id>" for dedicated).
func OperationsMigrations() []Migration {
	specs := []collectionSpec{
		{
			version: 1,
			name:    "practitioners_schema_and_indexes",
			coll:    "practitioners",
			schema: bson.D{
				{Key: "bsonType", Value: "object"},
				{Key: "required", Value: bson.A{"clinicId", "name", "specialty", "serviceIds", "active", "createdAt", "updatedAt"}},
				{Key: "properties", Value: bson.D{
					{Key: "clinicId", Value: bson.D{{Key: "bsonType", Value: "objectId"}}},
					{Key: "userId", Value: bson.D{{Key: "bsonType", Value: "objectId"}}},
					{Key: "name", Value: bson.D{{Key: "bsonType", Value: "string"}, {Key: "minLength", Value: 1}}},
					{Key: "specialty", Value: bson.D{{Key: "bsonType", Value: "string"}, {Key: "minLength", Value: 1}}},
					{Key: "serviceIds", Value: bson.D{
						{Key: "bsonType", Value: "array"},
						{Key: "items", Value: bson.D{{Key: "bsonType", Value: "objectId"}}},
					}},
					{Key: "active", Value: bson.D{{Key: "bsonType", Value: "bool"}}},
					{Key: "createdAt", Value: bson.D{{Key: "bsonType", Value: "date"}}},
					{Key: "updatedAt", Value: bson.D{{Key: "bsonType", Value: "date"}}},
				}},
			},
			indexes: []mongo.IndexModel{
				{
					Keys:    bson.D{{Key: "clinicId", Value: 1}, {Key: "active", Value: 1}},
					Options: options.Index().SetName("clinic_active_idx"),
				},
				{
					Keys:    bson.D{{Key: "clinicId", Value: 1}, {Key: "specialty", Value: 1}},
					Options: options.Index().SetName("clinic_specialty_idx"),
				},
			},
		},
		{
			version: 2,
			name:    "rooms_schema_and_indexes",
			coll:    "rooms",
			schema: bson.D{
				{Key: "bsonType", Value: "object"},
				{Key: "required", Value: bson.A{"clinicId", "name", "roomType", "active", "createdAt", "updatedAt"}},
				{Key: "properties", Value: bson.D{
					{Key: "clinicId", Value: bson.D{{Key: "bsonType", Value: "objectId"}}},
					{Key: "name", Value: bson.D{{Key: "bsonType", Value: "string"}, {Key: "minLength", Value: 1}}},
					{Key: "roomType", Value: bson.D{{Key: "bsonType", Value: "string"}, {Key: "minLength", Value: 1}}},
					{Key: "active", Value: bson.D{{Key: "bsonType", Value: "bool"}}},
					{Key: "createdAt", Value: bson.D{{Key: "bsonType", Value: "date"}}},
					{Key: "updatedAt", Value: bson.D{{Key: "bsonType", Value: "date"}}},
				}},
			},
			indexes: []mongo.IndexModel{
				{
					Keys:    bson.D{{Key: "clinicId", Value: 1}, {Key: "name", Value: 1}},
					Options: options.Index().SetUnique(true).SetName("clinic_name_unique"),
				},
				{
					Keys:    bson.D{{Key: "clinicId", Value: 1}, {Key: "roomType", Value: 1}, {Key: "active", Value: 1}},
					Options: options.Index().SetName("clinic_type_active_idx"),
				},
			},
		},
		{
			version: 3,
			name:    "services_schema_and_indexes",
			coll:    "services",
			schema: bson.D{
				{Key: "bsonType", Value: "object"},
				{Key: "required", Value: bson.A{"clinicId", "name", "durationMinutes", "bufferMinutes", "roomType", "paymentMode", "priceCents", "depositCents", "depositPercent", "active", "createdAt", "updatedAt"}},
				{Key: "properties", Value: bson.D{
					{Key: "clinicId", Value: bson.D{{Key: "bsonType", Value: "objectId"}}},
					{Key: "name", Value: bson.D{{Key: "bsonType", Value: "string"}, {Key: "minLength", Value: 1}}},
					{Key: "durationMinutes", Value: intRangeSchema(5, 480)},
					{Key: "bufferMinutes", Value: intRangeSchema(0, 120)},
					{Key: "roomType", Value: bson.D{{Key: "bsonType", Value: "string"}, {Key: "minLength", Value: 1}}},
					{Key: "paymentMode", Value: bson.D{{Key: "bsonType", Value: "string"}, {Key: "enum", Value: bson.A{"none", "deposit", "full"}}}},
					{Key: "priceCents", Value: bson.D{{Key: "bsonType", Value: bson.A{"int", "long"}}, {Key: "minimum", Value: 0}}},
					{Key: "depositCents", Value: bson.D{{Key: "bsonType", Value: bson.A{"int", "long"}}, {Key: "minimum", Value: 0}}},
					{Key: "depositPercent", Value: intRangeSchema(0, 100)},
					{Key: "active", Value: bson.D{{Key: "bsonType", Value: "bool"}}},
					{Key: "createdAt", Value: bson.D{{Key: "bsonType", Value: "date"}}},
					{Key: "updatedAt", Value: bson.D{{Key: "bsonType", Value: "date"}}},
				}},
			},
			indexes: []mongo.IndexModel{
				{
					Keys:    bson.D{{Key: "clinicId", Value: 1}, {Key: "name", Value: 1}},
					Options: options.Index().SetUnique(true).SetName("clinic_name_unique"),
				},
				{
					Keys:    bson.D{{Key: "clinicId", Value: 1}, {Key: "active", Value: 1}},
					Options: options.Index().SetName("clinic_active_idx"),
				},
			},
		},
		{
			version: 4,
			name:    "availability_rules_schema_and_indexes",
			coll:    "availability_rules",
			schema: bson.D{
				{Key: "bsonType", Value: "object"},
				{Key: "required", Value: bson.A{"clinicId", "practitionerId", "roomId", "weekday", "startMinute", "endMinute", "active", "createdAt", "updatedAt"}},
				{Key: "properties", Value: bson.D{
					{Key: "clinicId", Value: bson.D{{Key: "bsonType", Value: "objectId"}}},
					{Key: "practitionerId", Value: bson.D{{Key: "bsonType", Value: "objectId"}}},
					{Key: "roomId", Value: bson.D{{Key: "bsonType", Value: "objectId"}}},
					{Key: "weekday", Value: intRangeSchema(0, 6)},
					{Key: "startMinute", Value: intRangeSchema(0, 1435)},
					{Key: "endMinute", Value: intRangeSchema(5, 1440)},
					{Key: "validFrom", Value: bson.D{{Key: "bsonType", Value: "date"}}},
					{Key: "validUntil", Value: bson.D{{Key: "bsonType", Value: "date"}}},
					{Key: "active", Value: bson.D{{Key: "bsonType", Value: "bool"}}},
					{Key: "createdAt", Value: bson.D{{Key: "bsonType", Value: "date"}}},
					{Key: "updatedAt", Value: bson.D{{Key: "bsonType", Value: "date"}}},
				}},
			},
			indexes: []mongo.IndexModel{
				{
					Keys:    bson.D{{Key: "clinicId", Value: 1}, {Key: "practitionerId", Value: 1}, {Key: "weekday", Value: 1}, {Key: "active", Value: 1}},
					Options: options.Index().SetName("clinic_practitioner_weekday_idx"),
				},
				{
					Keys:    bson.D{{Key: "clinicId", Value: 1}, {Key: "roomId", Value: 1}, {Key: "weekday", Value: 1}, {Key: "active", Value: 1}},
					Options: options.Index().SetName("clinic_room_weekday_idx"),
				},
			},
		},
		{
			version: 5,
			name:    "blocks_schema_and_indexes",
			coll:    "blocks",
			schema: bson.D{
				{Key: "bsonType", Value: "object"},
				{Key: "required", Value: bson.A{"clinicId", "scope", "reason", "source", "startAt", "endAt", "createdAt"}},
				{Key: "properties", Value: bson.D{
					{Key: "clinicId", Value: bson.D{{Key: "bsonType", Value: "objectId"}}},
					{Key: "scope", Value: bson.D{{Key: "bsonType", Value: "string"}, {Key: "enum", Value: bson.A{"clinic", "practitioner", "room"}}}},
					{Key: "practitionerId", Value: bson.D{{Key: "bsonType", Value: "objectId"}}},
					{Key: "roomId", Value: bson.D{{Key: "bsonType", Value: "objectId"}}},
					{Key: "reason", Value: bson.D{{Key: "bsonType", Value: "string"}, {Key: "minLength", Value: 1}}},
					{Key: "source", Value: bson.D{{Key: "bsonType", Value: "string"}, {Key: "enum", Value: bson.A{"manual", "holiday"}}}},
					{Key: "startAt", Value: bson.D{{Key: "bsonType", Value: "date"}}},
					{Key: "endAt", Value: bson.D{{Key: "bsonType", Value: "date"}}},
					{Key: "createdAt", Value: bson.D{{Key: "bsonType", Value: "date"}}},
				}},
			},
			indexes: []mongo.IndexModel{
				{
					Keys:    bson.D{{Key: "clinicId", Value: 1}, {Key: "startAt", Value: 1}, {Key: "endAt", Value: 1}},
					Options: options.Index().SetName("clinic_window_idx"),
				},
				{
					Keys:    bson.D{{Key: "clinicId", Value: 1}, {Key: "practitionerId", Value: 1}, {Key: "startAt", Value: 1}},
					Options: options.Index().SetName("clinic_practitioner_start_idx"),
				},
				{
					Keys:    bson.D{{Key: "clinicId", Value: 1}, {Key: "roomId", Value: 1}, {Key: "startAt", Value: 1}},
					Options: options.Index().SetName("clinic_room_start_idx"),
				},
			},
		},
	}

	out := make([]Migration, 0, len(specs))
	for _, spec := range specs {
		out = append(out, Migration{
			Version: spec.version,
			Name:    spec.name,
			Up: func(ctx context.Context, db *mongo.Database) error {
				return ensureCollectionSchema(ctx, db, spec)
			},
		})
	}
	return out
}

func intRangeSchema(minVal, maxVal int) bson.D {
	return bson.D{
		{Key: "bsonType", Value: bson.A{"int", "long"}},
		{Key: "minimum", Value: minVal},
		{Key: "maximum", Value: maxVal},
	}
}

func ensureCollectionSchema(ctx context.Context, db *mongo.Database, spec collectionSpec) error {
	validator := bson.D{{Key: "$jsonSchema", Value: spec.schema}}
	opts := options.CreateCollection().
		SetValidator(validator).
		SetValidationLevel("strict").
		SetValidationAction("error")

	err := db.CreateCollection(ctx, spec.coll, opts)
	if err != nil && !isNamespaceExistsError(err) {
		return fmt.Errorf("create collection %s: %w", spec.coll, err)
	}
	if err != nil {
		// Collection already exists: apply updated validator idempotently.
		if modErr := db.RunCommand(ctx, bson.D{
			{Key: "collMod", Value: spec.coll},
			{Key: "validator", Value: validator},
			{Key: "validationLevel", Value: "strict"},
			{Key: "validationAction", Value: "error"},
		}).Err(); modErr != nil {
			return fmt.Errorf("update validator on %s: %w", spec.coll, modErr)
		}
	}

	if len(spec.indexes) > 0 {
		if _, idxErr := db.Collection(spec.coll).Indexes().CreateMany(ctx, spec.indexes); idxErr != nil {
			return fmt.Errorf("create indexes on %s: %w", spec.coll, idxErr)
		}
	}
	return nil
}

func isNamespaceExistsError(err error) bool {
	var cmdErr mongo.CommandError
	if errors.As(err, &cmdErr) {
		return cmdErr.Code == 48 || cmdErr.Name == "NamespaceExists"
	}
	return false
}
