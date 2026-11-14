package crypto

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ErrKeyNotFound is returned when a clinic has no data key, either because
// EnsureKey was never called or because DeleteKey destroyed it.
var ErrKeyNotFound = errors.New("crypto: data key not found")

// Encryptor encrypts individual field values with the data key (DEK) of one
// clinic. It is an interface so the rest of the code (repositories, services)
// builds and tests without libmongocrypt: the real implementation needs the
// "cse" build tag, and Fake covers everything else.
//
// Values are strings. Fields that are not text (a birth date, for example) are
// formatted by the domain layer before encrypting.
//
// Deleting a key (crypto-shredding) makes the data unreadable for processes
// that start afterwards. A process that already holds the key in its cache can
// still decrypt until the cache entry expires (ADR 0007).
type Encryptor interface {
	// EnsureKey creates the clinic's data key if it does not exist. It is safe
	// to call many times and from many processes at once.
	EnsureKey(ctx context.Context, clinicID string) error
	// EncryptRandom produces a different ciphertext on every call. It cannot be
	// searched. Use it by default.
	EncryptRandom(ctx context.Context, clinicID, value string) (bson.Binary, error)
	// EncryptDeterministic always produces the same ciphertext for the same
	// clinic and value, so it supports equality queries. It leaks equality, so
	// use it only where search is needed (the national ID number).
	EncryptDeterministic(ctx context.Context, clinicID, value string) (bson.Binary, error)
	// Decrypt recovers the value. The ciphertext carries the id of its key, so
	// it needs no clinic id.
	Decrypt(ctx context.Context, ciphertext bson.Binary) (string, error)
	// DeleteKey destroys the clinic's data key. It returns ErrKeyNotFound if
	// there is none.
	DeleteKey(ctx context.Context, clinicID string) error
}

// KeyAltName is the alias under which a clinic's data key is stored in
// platform.keyvault. Aliases let us find the key without storing its id.
func KeyAltName(clinicID string) string { return "clinic:" + clinicID }
