//go:build cse

package crypto_test

import (
	"context"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/zSamir015/agenda-clinicas/backend/internal/crypto"
	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb/mongotest"
)

type env struct {
	client *mongo.Client
	master []byte
}

func newEnv(t *testing.T) env {
	t.Helper()
	client, err := mongo.Connect(options.Client().ApplyURI(mongotest.URI(t)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	// A clean key vault per test: keys from other tests must not leak in.
	_ = client.Database("platform").Collection("keyvault").Drop(context.Background())
	master := make([]byte, crypto.MasterKeySize)
	if _, err := rand.Read(master); err != nil {
		t.Fatal(err)
	}
	return env{client: client, master: master}
}

func (e env) encryptor(t *testing.T) *crypto.ClientEncryptor {
	t.Helper()
	enc, err := crypto.NewClientEncryptor(context.Background(), e.client, e.master, time.Minute)
	if err != nil {
		t.Fatalf("NewClientEncryptor: %v", err)
	}
	t.Cleanup(func() { _ = enc.Close(context.Background()) })
	return enc
}

func TestRandomVersusDeterministic(t *testing.T) {
	ctx := context.Background()
	enc := newEnv(t).encryptor(t)
	if err := enc.EnsureKey(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	r1, _ := enc.EncryptRandom(ctx, "a", "6000-0000")
	r2, _ := enc.EncryptRandom(ctx, "a", "6000-0000")
	if string(r1.Data) == string(r2.Data) {
		t.Error("random encryption must differ between calls")
	}
	d1, _ := enc.EncryptDeterministic(ctx, "a", "8-123-4567")
	d2, _ := enc.EncryptDeterministic(ctx, "a", "8-123-4567")
	if string(d1.Data) != string(d2.Data) {
		t.Error("deterministic encryption must repeat")
	}
	for _, ct := range []bson.Binary{r1, d1} {
		if got, err := enc.Decrypt(ctx, ct); err != nil || (got != "6000-0000" && got != "8-123-4567") {
			t.Errorf("round trip: %q %v", got, err)
		}
	}
}

func TestEqualitySearchOnDeterministicField(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	enc := e.encryptor(t)
	_ = enc.EnsureKey(ctx, "a")
	coll := e.client.Database("crypto_test").Collection("patients_" + t.Name())
	t.Cleanup(func() { _ = coll.Drop(context.Background()) })
	for _, id := range []string{"8-111-1111", "8-222-2222"} {
		ct, _ := enc.EncryptDeterministic(ctx, "a", id)
		if _, err := coll.InsertOne(ctx, bson.D{{Key: "idNumber", Value: ct}}); err != nil {
			t.Fatal(err)
		}
	}
	probe, _ := enc.EncryptDeterministic(ctx, "a", "8-222-2222")
	var got struct {
		IDNumber bson.Binary `bson:"idNumber"`
	}
	if err := coll.FindOne(ctx, bson.D{{Key: "idNumber", Value: probe}}).Decode(&got); err != nil {
		t.Fatalf("equality query: %v", err)
	}
	if plain, _ := enc.Decrypt(ctx, got.IDNumber); plain != "8-222-2222" {
		t.Fatalf("found wrong document: %q", plain)
	}
}

func TestKeysAreIndependentPerClinic(t *testing.T) {
	ctx := context.Background()
	enc := newEnv(t).encryptor(t)
	_ = enc.EnsureKey(ctx, "a")
	_ = enc.EnsureKey(ctx, "b")
	a, _ := enc.EncryptDeterministic(ctx, "a", "8-123-4567")
	b, _ := enc.EncryptDeterministic(ctx, "b", "8-123-4567")
	if string(a.Data) == string(b.Data) {
		t.Fatal("same value in two clinics must not produce the same ciphertext")
	}
}

func TestEnsureKeyIsIdempotentAndConcurrencySafe(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	enc := e.encryptor(t)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func() { errs <- enc.EnsureKey(ctx, "race") }()
	}
	for i := 0; i < 8; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("EnsureKey: %v", err)
		}
	}
	n, err := e.client.Database("platform").Collection("keyvault").CountDocuments(ctx, bson.D{})
	if err != nil || n != 1 {
		t.Fatalf("want exactly 1 key, got %d (%v)", n, err)
	}
}

func TestMissingKey(t *testing.T) {
	ctx := context.Background()
	enc := newEnv(t).encryptor(t)
	if _, err := enc.EncryptRandom(ctx, "nope", "x"); !errors.Is(err, crypto.ErrKeyNotFound) {
		t.Errorf("encrypt: want ErrKeyNotFound, got %v", err)
	}
	if err := enc.DeleteKey(ctx, "nope"); !errors.Is(err, crypto.ErrKeyNotFound) {
		t.Errorf("delete: want ErrKeyNotFound, got %v", err)
	}
}

func TestWrongMasterKeyCannotDecrypt(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	enc := e.encryptor(t)
	_ = enc.EnsureKey(ctx, "a")
	ct, _ := enc.EncryptRandom(ctx, "a", "secreto")
	other := env{client: e.client, master: make([]byte, crypto.MasterKeySize)}.encryptor(t)
	if _, err := other.Decrypt(ctx, ct); err == nil {
		t.Fatal("a different master key must not decrypt")
	}
}

// Crypto-shredding as ADR 0007 describes it: a process that starts after the
// key is deleted cannot decrypt.
func TestDeleteKeyShredsDataForNewProcesses(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	enc := e.encryptor(t)
	_ = enc.EnsureKey(ctx, "a")
	ct, _ := enc.EncryptRandom(ctx, "a", "secreto")
	if err := enc.DeleteKey(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.encryptor(t).Decrypt(ctx, ct); err == nil {
		t.Fatal("decrypt must fail for a new process after DeleteKey")
	}
	if _, err := e.encryptor(t).EncryptRandom(ctx, "a", "x"); !errors.Is(err, crypto.ErrKeyNotFound) {
		t.Fatalf("encrypt after delete: want ErrKeyNotFound, got %v", err)
	}
}
