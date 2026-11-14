package crypto_test

import (
	"context"
	"errors"
	"testing"

	"github.com/zSamir015/agenda-clinicas/backend/internal/crypto"
)

func TestFake(t *testing.T) {
	ctx := context.Background()
	f := crypto.NewFake()
	if _, err := f.EncryptRandom(ctx, "a", "x"); !errors.Is(err, crypto.ErrKeyNotFound) {
		t.Fatalf("without key want ErrKeyNotFound, got %v", err)
	}
	_ = f.EnsureKey(ctx, "a")
	r1, _ := f.EncryptRandom(ctx, "a", "x")
	r2, _ := f.EncryptRandom(ctx, "a", "x")
	if string(r1.Data) == string(r2.Data) {
		t.Fatal("random must differ")
	}
	d1, _ := f.EncryptDeterministic(ctx, "a", "x")
	d2, _ := f.EncryptDeterministic(ctx, "a", "x")
	if string(d1.Data) != string(d2.Data) {
		t.Fatal("deterministic must repeat")
	}
	if got, err := f.Decrypt(ctx, r1); err != nil || got != "x" {
		t.Fatalf("Decrypt: %q %v", got, err)
	}
	if err := f.DeleteKey(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Decrypt(ctx, r1); !errors.Is(err, crypto.ErrKeyNotFound) {
		t.Fatalf("after delete want ErrKeyNotFound, got %v", err)
	}
}
