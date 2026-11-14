package crypto

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// MasterKeySize is the size MongoDB requires for a local KMS master key.
const MasterKeySize = 96

// LoadMasterKey reads the local master key that wraps every data key. In
// development this file is the "KMS"; production uses a real KMS (phase 10)
// behind the same Encryptor. The file must be readable only by its owner,
// because anyone who reads it and the key vault can decrypt every clinic.
func LoadMasterKey(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat master key: %w", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("master key %s has mode %v, want 0600", path, info.Mode().Perm())
	}
	key, err := os.ReadFile(path) //nolint:gosec // path comes from operator configuration, not user input
	if err != nil {
		return nil, fmt.Errorf("read master key: %w", err)
	}
	if len(key) != MasterKeySize {
		return nil, fmt.Errorf("master key is %d bytes, want %d", len(key), MasterKeySize)
	}
	return key, nil
}

// LoadOrCreateMasterKey is LoadMasterKey, but creates the file with random
// bytes when it does not exist. For local development only; creating keys is
// not something a production process should do on its own.
func LoadOrCreateMasterKey(path string) ([]byte, error) {
	key := make([]byte, MasterKeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate master key: %w", err)
	}
	// O_EXCL makes creation atomic: if another process wins the race, we load its file.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // path comes from operator configuration, not user input
	if errors.Is(err, fs.ErrExist) {
		return LoadMasterKey(path)
	}
	if err != nil {
		return nil, fmt.Errorf("create master key: %w", err)
	}
	if _, err := f.Write(key); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("write master key: %w", err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("close master key: %w", err)
	}
	return key, nil
}
