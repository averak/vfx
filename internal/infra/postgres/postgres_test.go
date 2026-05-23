package postgres_test

import (
	"strings"
	"testing"

	"github.com/averak/vfx/internal/infra/postgres"
)

// A malformed DSN fails at config parsing, before any connection attempt, so this needs no database.
func TestNewPool_RejectsMalformedURL(t *testing.T) {
	_, err := postgres.NewPool(t.Context(), "://bad")
	if err == nil {
		t.Fatal("NewPool with a malformed URL succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "postgres:") {
		t.Errorf("error = %v, want it wrapped with the postgres prefix", err)
	}
}
