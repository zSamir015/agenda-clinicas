package migrate_test

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/zSamir015/agenda-clinicas/backend/internal/migrate"
)

func TestOperationsMigrationsSchemaAndIndexes(t *testing.T) {
	_, platformDB, opsDB := connectTestDB(t, "ops")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	runner, err := migrate.NewRunner(platformDB, migrate.OperationsMigrations())
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	if err := runner.Run(ctx, opsDB); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	// Running twice must remain idempotent.
	if err := runner.Run(ctx, opsDB); err != nil {
		t.Fatalf("second Run: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	clinicID := bson.NewObjectID()
	practitionerID := bson.NewObjectID()
	roomID := bson.NewObjectID()
	serviceID := bson.NewObjectID()

	// Valid documents succeed on all 5 operational collections.
	if _, err := opsDB.Collection("practitioners").InsertOne(ctx, bson.M{
		"_id":        practitionerID,
		"clinicId":   clinicID,
		"name":       "Dra. Elena Ríos",
		"specialty":  "Medicina General",
		"serviceIds": bson.A{serviceID},
		"active":     true,
		"createdAt":  now,
		"updatedAt":  now,
	}); err != nil {
		t.Fatalf("insert valid practitioner: %v", err)
	}

	if _, err := opsDB.Collection("rooms").InsertOne(ctx, bson.M{
		"_id":       roomID,
		"clinicId":  clinicID,
		"name":      "Consultorio 1",
		"roomType":  "general",
		"active":    true,
		"createdAt": now,
		"updatedAt": now,
	}); err != nil {
		t.Fatalf("insert valid room: %v", err)
	}

	// Duplicate room name within the same clinic must fail unique index.
	if _, err := opsDB.Collection("rooms").InsertOne(ctx, bson.M{
		"clinicId":  clinicID,
		"name":      "Consultorio 1",
		"roomType":  "general",
		"active":    true,
		"createdAt": now,
		"updatedAt": now,
	}); !mongo.IsDuplicateKeyError(err) {
		t.Fatalf("expected duplicate key error for rooms, got %v", err)
	}

	if _, err := opsDB.Collection("services").InsertOne(ctx, bson.M{
		"_id":             serviceID,
		"clinicId":        clinicID,
		"name":            "Consulta General",
		"durationMinutes": 30,
		"bufferMinutes":   5,
		"roomType":        "general",
		"paymentMode":     "deposit",
		"priceCents":      5000,
		"depositCents":    1500,
		"depositPercent":  0,
		"active":          true,
		"createdAt":       now,
		"updatedAt":       now,
	}); err != nil {
		t.Fatalf("insert valid service: %v", err)
	}

	if _, err := opsDB.Collection("availability_rules").InsertOne(ctx, bson.M{
		"clinicId":       clinicID,
		"practitionerId": practitionerID,
		"roomId":         roomID,
		"weekday":        1,
		"startMinute":    480,
		"endMinute":      1020,
		"active":         true,
		"createdAt":      now,
		"updatedAt":      now,
	}); err != nil {
		t.Fatalf("insert valid availability rule: %v", err)
	}

	if _, err := opsDB.Collection("blocks").InsertOne(ctx, bson.M{
		"clinicId":  clinicID,
		"scope":     "clinic",
		"reason":    "Día de la Separación de Panamá de Colombia",
		"source":    "holiday",
		"startAt":   now,
		"endAt":     now.Add(24 * time.Hour),
		"createdAt": now,
	}); err != nil {
		t.Fatalf("insert valid block: %v", err)
	}

	// Strict $jsonSchema validators reject malformed documents.
	invalidCases := []struct {
		coll string
		doc  bson.M
	}{
		{
			coll: "practitioners",
			doc:  bson.M{"name": "Sin clinicId", "specialty": "Pediatría", "serviceIds": bson.A{}, "active": true, "createdAt": now, "updatedAt": now},
		},
		{
			coll: "services",
			doc:  bson.M{"clinicId": clinicID, "name": "Inválido", "durationMinutes": 2, "bufferMinutes": 0, "roomType": "general", "paymentMode": "cash", "priceCents": 100, "depositCents": 0, "depositPercent": 0, "active": true, "createdAt": now, "updatedAt": now},
		},
		{
			coll: "availability_rules",
			doc:  bson.M{"clinicId": clinicID, "practitionerId": practitionerID, "roomId": roomID, "weekday": 9, "startMinute": 480, "endMinute": 600, "active": true, "createdAt": now, "updatedAt": now},
		},
		{
			coll: "blocks",
			doc:  bson.M{"clinicId": clinicID, "scope": "global", "reason": "Mal scope", "source": "manual", "startAt": now, "endAt": now, "createdAt": now},
		},
	}

	for _, tc := range invalidCases {
		if _, err := opsDB.Collection(tc.coll).InsertOne(ctx, tc.doc); err == nil {
			t.Fatalf("%s: expected $jsonSchema validation error, got nil", tc.coll)
		}
	}
}
