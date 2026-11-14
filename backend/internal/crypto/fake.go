package crypto

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Fake is an in-memory Encryptor for tests of other packages. It is NOT
// encryption: values are only wrapped so tests can check that code stores
// ciphertext instead of plaintext, keeps keys per clinic and handles deleted
// keys. Real encryption is tested in encryptor_cse_test.go.
type Fake struct {
	mu   sync.Mutex
	keys map[string]bool
}

// NewFake returns an empty Fake.
func NewFake() *Fake { return &Fake{keys: map[string]bool{}} }

var _ Encryptor = (*Fake)(nil)

// EnsureKey implements Encryptor.
func (f *Fake) EnsureKey(_ context.Context, clinicID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.keys[clinicID] = true
	return nil
}

func (f *Fake) has(clinicID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.keys[clinicID]
}

func wrap(clinicID, tag, value string) bson.Binary {
	return bson.Binary{Subtype: bson.TypeBinaryEncrypted, Data: []byte(strings.Join([]string{"fake", tag, clinicID, value}, "|"))}
}

// EncryptRandom implements Encryptor. A random suffix keeps two calls different.
func (f *Fake) EncryptRandom(_ context.Context, clinicID, value string) (bson.Binary, error) {
	if !f.has(clinicID) {
		return bson.Binary{}, fmt.Errorf("clinic %s: %w", clinicID, ErrKeyNotFound)
	}
	salt := make([]byte, 4)
	_, _ = rand.Read(salt)
	return wrap(clinicID, "r"+hex.EncodeToString(salt), value), nil
}

// EncryptDeterministic implements Encryptor.
func (f *Fake) EncryptDeterministic(_ context.Context, clinicID, value string) (bson.Binary, error) {
	if !f.has(clinicID) {
		return bson.Binary{}, fmt.Errorf("clinic %s: %w", clinicID, ErrKeyNotFound)
	}
	return wrap(clinicID, "d", value), nil
}

// Decrypt implements Encryptor.
func (f *Fake) Decrypt(_ context.Context, ct bson.Binary) (string, error) {
	parts := strings.SplitN(string(ct.Data), "|", 4)
	if len(parts) != 4 || parts[0] != "fake" {
		return "", fmt.Errorf("crypto: not a fake ciphertext")
	}
	if !f.has(parts[2]) {
		return "", fmt.Errorf("clinic %s: %w", parts[2], ErrKeyNotFound)
	}
	return parts[3], nil
}

// DeleteKey implements Encryptor.
func (f *Fake) DeleteKey(_ context.Context, clinicID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.keys[clinicID] {
		return fmt.Errorf("clinic %s: %w", clinicID, ErrKeyNotFound)
	}
	delete(f.keys, clinicID)
	return nil
}
