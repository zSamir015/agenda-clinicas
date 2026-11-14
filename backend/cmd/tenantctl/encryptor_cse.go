//go:build cse

package main

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/zSamir015/agenda-clinicas/backend/internal/crypto"
)

func newEncryptor(ctx context.Context, client *mongo.Client, masterKeyPath string, insecure bool) (crypto.Encryptor, error) {
	if insecure {
		return crypto.NewFake(), nil
	}
	key, err := crypto.LoadOrCreateMasterKey(masterKeyPath)
	if err != nil {
		return nil, fmt.Errorf("load master key %q: %w", masterKeyPath, err)
	}
	enc, err := crypto.NewClientEncryptor(ctx, client, key, crypto.DefaultKeyExpiration)
	if err != nil {
		return nil, fmt.Errorf("init client encryptor: %w", err)
	}
	return enc, nil
}
