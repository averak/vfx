// Package valkey wraps the Valkey client so the rest of vfx talks to a
// single, opinionated constructor.
package valkey

import (
	"fmt"

	"github.com/valkey-io/valkey-go"
)

// NewClient parses a redis:// URL and returns a connected Valkey client.
// The caller is responsible for Close on shutdown.
func NewClient(url string) (valkey.Client, error) {
	opts, err := valkey.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("valkey: parse url: %w", err)
	}
	client, err := valkey.NewClient(opts)
	if err != nil {
		return nil, fmt.Errorf("valkey: new client: %w", err)
	}
	return client, nil
}
