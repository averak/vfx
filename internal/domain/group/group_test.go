package group_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/averak/vfx/internal/domain/group"
)

func TestNew_Validation(t *testing.T) {
	owner := uuid.New()
	now := time.Now()

	if _, err := group.New(uuid.New(), owner, "   ", now); !errors.Is(err, group.ErrInvalidName) {
		t.Errorf("blank name: err = %v, want ErrInvalidName", err)
	}
	if _, err := group.New(uuid.New(), owner, strings.Repeat("x", group.MaxNameLength+1), now); !errors.Is(err, group.ErrInvalidName) {
		t.Errorf("over-length name: err = %v, want ErrInvalidName", err)
	}

	// A surrounding-whitespace name is trimmed, not rejected.
	g, err := group.New(uuid.New(), owner, "  Clan  ", now)
	if err != nil {
		t.Fatalf("valid name rejected: %v", err)
	}
	if g.Name != "Clan" {
		t.Errorf("name = %q, want trimmed \"Clan\"", g.Name)
	}
	if g.OwnerID != owner {
		t.Errorf("owner not set")
	}
}
