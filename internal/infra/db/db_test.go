package db_test

import (
	"context"
	"errors"
	"testing"

	"github.com/averak/vfx/internal/infra/db"
)

// A repository that calls Tx outside any RW/RO scope is a programming
// error, and Tx must surface it as ErrNoTx rather than a nil transaction.
func TestTx_OutsideTransaction(t *testing.T) {
	_, err := db.Tx(context.Background())
	if !errors.Is(err, db.ErrNoTx) {
		t.Fatalf("Tx on a bare context: err = %v, want ErrNoTx", err)
	}
}
