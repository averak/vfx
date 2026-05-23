package authctx_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/averak/vfx/internal/infra/connectrpc/authctx"
)

func TestWithFrom_RoundTrip(t *testing.T) {
	id := uuid.New()
	ctx := authctx.With(context.Background(), id)
	got, ok := authctx.From(ctx)
	if !ok {
		t.Fatal("From reported no player id after With")
	}
	if got != id {
		t.Errorf("From = %v, want %v", got, id)
	}
}

func TestFrom_Absent(t *testing.T) {
	got, ok := authctx.From(context.Background())
	if ok {
		t.Errorf("From reported ok on a bare context (id=%v)", got)
	}
	if got != uuid.Nil {
		t.Errorf("From returned %v, want uuid.Nil when absent", got)
	}
}
