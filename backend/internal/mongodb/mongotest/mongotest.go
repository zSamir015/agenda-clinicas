// Package mongotest starts a MongoDB replica set in a container for tests.
//
// Tests run against a real MongoDB because transactions, unique indexes and
// change streams are exactly what a mock would get wrong.
package mongotest

import (
	"context"
	"net/url"
	"sync"
	"testing"

	"github.com/testcontainers/testcontainers-go/modules/mongodb"
)

// Same version as deploy/compose.yaml.
const image = "mongo:8.0"

var (
	once sync.Once
	uri  string
	err  error
)

// URI returns the connection string of a replica set shared by all tests in
// the package. Testcontainers removes the container when the test binary exits.
func URI(t testing.TB) string {
	t.Helper()
	once.Do(func() {
		ctx := context.Background()
		var c *mongodb.MongoDBContainer
		c, err = mongodb.Run(ctx, image, mongodb.WithReplicaSet("rs0"))
		if err != nil {
			return
		}
		uri, err = c.ConnectionString(ctx)
		if err == nil {
			uri, err = direct(uri)
		}
	})
	if err != nil {
		t.Fatalf("start mongo container: %v", err)
	}
	return uri
}

// direct adds directConnection=true. The replica set advertises the
// container's internal IP, which the host cannot reach (on macOS Docker runs
// inside a VM), so the driver must talk only to the address it was given
// instead of discovering members. Transactions still work because that
// single member is the primary.
func direct(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("directConnection", "true")
	u.RawQuery = q.Encode()
	return u.String(), nil
}
