//go:build cse

package crypto

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	keyVaultDB   = "platform"
	keyVaultColl = "keyvault"
	algRandom    = "AEAD_AES_256_CBC_HMAC_SHA_512-Random"
	algDetermin  = "AEAD_AES_256_CBC_HMAC_SHA_512-Deterministic"
)

// DefaultKeyExpiration is how long the driver keeps a decrypted data key in
// memory. It bounds how long a deleted key keeps working in a running process.
const DefaultKeyExpiration = time.Minute

// ClientEncryptor is the libmongocrypt implementation of Encryptor.
type ClientEncryptor struct {
	ce *mongo.ClientEncryption
	kv *mongo.Collection
}

var _ Encryptor = (*ClientEncryptor)(nil)

// NewClientEncryptor wires explicit encryption to client using masterKey as
// the local KMS key. It creates the unique index on keyAltNames, which makes
// concurrent EnsureKey calls safe: the loser of the race gets a duplicate key
// error and reads the winner's key.
func NewClientEncryptor(ctx context.Context, client *mongo.Client, masterKey []byte, keyExpiration time.Duration) (*ClientEncryptor, error) {
	if len(masterKey) != MasterKeySize {
		return nil, fmt.Errorf("crypto: master key is %d bytes, want %d", len(masterKey), MasterKeySize)
	}
	kv := client.Database(keyVaultDB).Collection(keyVaultColl)
	_, err := kv.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "keyAltNames", Value: 1}},
		Options: options.Index().SetUnique(true).
			SetPartialFilterExpression(bson.D{{Key: "keyAltNames", Value: bson.D{{Key: "$exists", Value: true}}}}),
	})
	if err != nil {
		return nil, fmt.Errorf("create keyvault index: %w", err)
	}
	ce, err := mongo.NewClientEncryption(client, options.ClientEncryption().
		SetKeyVaultNamespace(keyVaultDB+"."+keyVaultColl).
		SetKmsProviders(map[string]map[string]any{"local": {"key": masterKey}}).
		SetKeyExpiration(keyExpiration))
	if err != nil {
		return nil, fmt.Errorf("new client encryption: %w", err)
	}
	return &ClientEncryptor{ce: ce, kv: kv}, nil
}

// Close releases the libmongocrypt handle.
func (e *ClientEncryptor) Close(ctx context.Context) error { return e.ce.Close(ctx) }

func (e *ClientEncryptor) keyID(ctx context.Context, clinicID string) (bson.Binary, error) {
	res := e.ce.GetKeyByAltName(ctx, KeyAltName(clinicID))
	var doc struct {
		ID bson.Binary `bson:"_id"`
	}
	if err := res.Decode(&doc); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return bson.Binary{}, fmt.Errorf("clinic %s: %w", clinicID, ErrKeyNotFound)
		}
		return bson.Binary{}, fmt.Errorf("find data key: %w", err)
	}
	return doc.ID, nil
}

// EnsureKey implements Encryptor.
func (e *ClientEncryptor) EnsureKey(ctx context.Context, clinicID string) error {
	if _, err := e.keyID(ctx, clinicID); err == nil {
		return nil
	} else if !errors.Is(err, ErrKeyNotFound) {
		return err
	}
	_, err := e.ce.CreateDataKey(ctx, "local", options.DataKey().SetKeyAltNames([]string{KeyAltName(clinicID)}))
	if mongo.IsDuplicateKeyError(err) {
		return nil // another process created it first
	}
	if err != nil {
		return fmt.Errorf("create data key: %w", err)
	}
	return nil
}

func (e *ClientEncryptor) encrypt(ctx context.Context, alg, clinicID, value string) (bson.Binary, error) {
	typ, raw, err := bson.MarshalValue(value)
	if err != nil {
		return bson.Binary{}, fmt.Errorf("marshal value: %w", err)
	}
	// Check the alias first: libmongocrypt's error for a missing key is a
	// generic string that callers could not test with errors.Is.
	if _, err := e.keyID(ctx, clinicID); err != nil {
		return bson.Binary{}, err
	}
	out, err := e.ce.Encrypt(ctx, bson.RawValue{Type: typ, Value: raw},
		options.Encrypt().SetAlgorithm(alg).SetKeyAltName(KeyAltName(clinicID)))
	if err != nil {
		return bson.Binary{}, fmt.Errorf("encrypt: %w", err)
	}
	return out, nil
}

// EncryptRandom implements Encryptor.
func (e *ClientEncryptor) EncryptRandom(ctx context.Context, clinicID, value string) (bson.Binary, error) {
	return e.encrypt(ctx, algRandom, clinicID, value)
}

// EncryptDeterministic implements Encryptor.
func (e *ClientEncryptor) EncryptDeterministic(ctx context.Context, clinicID, value string) (bson.Binary, error) {
	return e.encrypt(ctx, algDetermin, clinicID, value)
}

// Decrypt implements Encryptor.
func (e *ClientEncryptor) Decrypt(ctx context.Context, ciphertext bson.Binary) (string, error) {
	v, err := e.ce.Decrypt(ctx, ciphertext)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	s, ok := v.StringValueOK()
	if !ok {
		return "", errors.New("crypto: decrypted value is not a string")
	}
	return s, nil
}

// DeleteKey implements Encryptor.
func (e *ClientEncryptor) DeleteKey(ctx context.Context, clinicID string) error {
	id, err := e.keyID(ctx, clinicID)
	if err != nil {
		return err
	}
	if _, err := e.ce.DeleteKey(ctx, id); err != nil {
		return fmt.Errorf("delete data key: %w", err)
	}
	return nil
}
