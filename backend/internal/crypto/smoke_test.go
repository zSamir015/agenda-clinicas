//go:build cse

package crypto_test

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb/mongotest"
)

// TestExplicitEncryptionSmoke demuestra que CSFLE explícito funciona con la
// libmongocrypt instalada en esta máquina: llave de datos, cifrado aleatorio y
// determinista, búsqueda por igualdad, validador $jsonSchema y crypto-shredding.
// Si falla, el problema es la instalación de libmongocrypt, no el código.
func TestExplicitEncryptionSmoke(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(mongotest.URI(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Disconnect(ctx) }()

	master := make([]byte, 96)
	if _, err := rand.Read(master); err != nil {
		t.Fatal(err)
	}
	newCE := func() *mongo.ClientEncryption {
		ce, err := mongo.NewClientEncryption(client, options.ClientEncryption().
			SetKeyVaultNamespace("platform.keyvault").
			SetKmsProviders(map[string]map[string]any{"local": {"key": master}}))
		if err != nil {
			t.Fatalf("NewClientEncryption: %v", err)
		}
		return ce
	}
	ce := newCE()
	defer func() { _ = ce.Close(ctx) }()

	// Índice único de alias, como exige el key vault.
	_, err = client.Database("platform").Collection("keyvault").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "keyAltNames", Value: 1}},
		Options: options.Index().SetUnique(true).
			SetPartialFilterExpression(bson.D{{Key: "keyAltNames", Value: bson.D{{Key: "$exists", Value: true}}}}),
	})
	if err != nil {
		t.Fatal(err)
	}

	keyID, err := ce.CreateDataKey(ctx, "local", options.DataKey().SetKeyAltNames([]string{"clinic-a"}))
	if err != nil {
		t.Fatalf("CreateDataKey: %v", err)
	}

	enc := func(alg, v string) bson.Binary {
		typ, raw, err := bson.MarshalValue(v)
		if err != nil {
			t.Fatal(err)
		}
		b, err := ce.Encrypt(ctx, bson.RawValue{Type: typ, Value: raw},
			options.Encrypt().SetAlgorithm(alg).SetKeyAltName("clinic-a"))
		if err != nil {
			t.Fatalf("Encrypt: %v", err)
		}
		return b
	}
	const det = "AEAD_AES_256_CBC_HMAC_SHA_512-Deterministic"
	const rnd = "AEAD_AES_256_CBC_HMAC_SHA_512-Random"

	// Colección con validador $jsonSchema que exige binData en los campos cifrados.
	db := client.Database("smoke")
	_ = db.Collection("patients").Drop(ctx)
	err = db.CreateCollection(ctx, "patients", options.CreateCollection().SetValidator(bson.D{{Key: "$jsonSchema", Value: bson.D{
		{Key: "bsonType", Value: "object"},
		{Key: "required", Value: bson.A{"idNumber", "phone"}},
		{Key: "properties", Value: bson.D{
			{Key: "idNumber", Value: bson.D{{Key: "bsonType", Value: "binData"}}},
			{Key: "phone", Value: bson.D{{Key: "bsonType", Value: "binData"}}},
		}},
	}}}))
	if err != nil {
		t.Fatal(err)
	}
	coll := db.Collection("patients")

	if _, err := coll.InsertOne(ctx, bson.D{{Key: "idNumber", Value: enc(det, "8-123-4567")}, {Key: "phone", Value: enc(rnd, "6000-0000")}}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	// El validador rechaza texto en claro.
	if _, err := coll.InsertOne(ctx, bson.D{{Key: "idNumber", Value: "8-123-4567"}, {Key: "phone", Value: "6000-0000"}}); err == nil {
		t.Fatal("validator must reject plaintext")
	}

	// Búsqueda por igualdad sobre el campo determinista.
	var got bson.M
	if err := coll.FindOne(ctx, bson.D{{Key: "idNumber", Value: enc(det, "8-123-4567")}}).Decode(&got); err != nil {
		t.Fatalf("equality query: %v", err)
	}
	// El aleatorio no es buscable: dos cifrados del mismo valor difieren.
	a, b := enc(rnd, "6000-0000"), enc(rnd, "6000-0000")
	if string(a.Data) == string(b.Data) {
		t.Fatal("random encryption must differ between calls")
	}

	// Descifrar.
	plain, err := ce.Decrypt(ctx, got["phone"].(bson.Binary))
	if err != nil || plain.StringValue() != "6000-0000" {
		t.Fatalf("decrypt: %v %v", plain, err)
	}

	// Crypto-shredding: borrar la DEK y descifrar con un cliente nuevo falla.
	if _, err := ce.DeleteKey(ctx, keyID); err != nil {
		t.Fatalf("DeleteKey: %v", err)
	}
	fresh := newCE()
	defer func() { _ = fresh.Close(ctx) }()
	if _, err := fresh.Decrypt(ctx, got["phone"].(bson.Binary)); err == nil {
		t.Fatal("decrypt must fail after the data key is deleted")
	}
}
