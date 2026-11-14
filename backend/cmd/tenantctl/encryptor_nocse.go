//go:build !cse

package main

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/zSamir015/agenda-clinicas/backend/internal/crypto"
)

func newEncryptor(_ context.Context, _ *mongo.Client, _ string, insecure bool) (crypto.Encryptor, error) {
	if !insecure {
		return nil, errors.New("tenantctl: built without cse tag (libmongocrypt); rebuild with -tags cse or pass --insecure-no-encryption")
	}
	return crypto.NewFake(), nil
}
