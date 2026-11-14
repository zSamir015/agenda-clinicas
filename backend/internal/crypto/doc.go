// Package crypto holds field-level encryption for patient data.
//
// Encryption uses MongoDB explicit client-side field level encryption (CSFLE),
// which needs the libmongocrypt C library and the "cse" build tag. The tag is
// opt-in on purpose: a normal "go build", "go test" and the linter work without
// the C library, so every contributor can run the base suite. Run the
// encryption tests with "make test-crypto"; the shipped binaries are built
// with "-tags cse".
//
// The Encryptor interface is the only thing other packages depend on. Fake
// implements it in memory for their tests; ClientEncryptor (cse tag) is the
// real one. Each clinic has its own data key, found by the alias
// "clinic:<id>" in platform.keyvault and wrapped by the master key. Deleting
// the key makes the clinic's data unreadable for new processes; a process that
// cached the key can still decrypt until it expires (ADR 0007).
package crypto
