// Package crypto holds field-level encryption for patient data.
//
// Encryption uses MongoDB explicit client-side field level encryption (CSFLE),
// which needs the libmongocrypt C library and the "cse" build tag. The tag is
// opt-in on purpose: a normal "go build", "go test" and the linter work without
// the C library, so every contributor can run the base suite. Run the
// encryption tests with "make test-crypto"; the shipped binaries are built
// with "-tags cse".
package crypto
