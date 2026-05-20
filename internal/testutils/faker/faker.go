// Package faker produces deterministic test data. Functions here MUST
// return the same value for the same input across runs and machines.
// This is what lets tests assert on concrete IDs without flake.
package faker

import (
	"github.com/google/uuid"
)

// vfxNamespace is the UUID namespace for all UUIDv5 generation in tests.
// It is not a secret and only needs to be stable across versions.
var vfxNamespace = uuid.MustParse("00000000-0000-0000-0000-000000000001")

// UUIDv5 returns the UUID derived from name under the vfx test namespace.
// Calling UUIDv5("alice") always yields the same UUID, on every machine.
//
// Use this in fixtures and assertions when you need an identifier the
// test can reference by a human name (faker.UUIDv5("player-1")) rather
// than a freshly-generated UUID that has to be plumbed back through
// assertions.
func UUIDv5(name string) uuid.UUID {
	return uuid.NewSHA1(vfxNamespace, []byte(name))
}
