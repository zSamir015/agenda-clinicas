package crypto_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zSamir015/agenda-clinicas/backend/internal/crypto"
)

func TestLoadOrCreateMasterKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "master.key")
	first, err := crypto.LoadOrCreateMasterKey(path)
	if err != nil || len(first) != crypto.MasterKeySize {
		t.Fatalf("create: %v len=%d", err, len(first))
	}
	second, err := crypto.LoadOrCreateMasterKey(path)
	if err != nil || string(first) != string(second) {
		t.Fatalf("second call must load the same key: %v", err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestLoadMasterKeyRejectsBadFiles(t *testing.T) {
	dir := t.TempDir()
	open := filepath.Join(dir, "open.key")
	if err := os.WriteFile(open, make([]byte, crypto.MasterKeySize), 0o644); err != nil { //nolint:gosec // a readable key file is exactly what this test needs
		t.Fatal(err)
	}
	if _, err := crypto.LoadMasterKey(open); err == nil {
		t.Error("world-readable key must be rejected")
	}
	short := filepath.Join(dir, "short.key")
	if err := os.WriteFile(short, []byte("tiny"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := crypto.LoadMasterKey(short); err == nil {
		t.Error("wrong size must be rejected")
	}
	if _, err := crypto.LoadMasterKey(filepath.Join(dir, "missing")); err == nil {
		t.Error("missing file must be rejected")
	}
}
