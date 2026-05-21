package admin_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/averak/vfx/internal/infra/matchqueue"
	"github.com/averak/vfx/internal/infra/repository"
	adminhandler "github.com/averak/vfx/internal/presentation/admin"
	"github.com/averak/vfx/internal/stdx/db"
	"github.com/averak/vfx/internal/testutils/testdb"
	usecaseadmin "github.com/averak/vfx/internal/usecase/admin"
)

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	pool := testdb.Pool(t)
	uc := usecaseadmin.New(db.NewSession(pool), repository.NewPlayer(), matchqueue.NewInMem())
	srv := httptest.NewServer(adminhandler.NewHandler(uc, pool))
	t.Cleanup(srv.Close)
	return srv
}

func TestAdmin_HealthEndpoints(t *testing.T) {
	srv := newServer(t)
	for _, path := range []string{"/healthz", "/readyz"} {
		resp, err := srv.Client().Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, resp.StatusCode)
		}
	}
}

func TestAdmin_GetPlayerNotFound(t *testing.T) {
	srv := newServer(t)
	resp, err := srv.Client().Get(srv.URL + "/api/players/" + uuid.NewString())
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestAdmin_GetPlayerInvalidID(t *testing.T) {
	srv := newServer(t)
	resp, err := srv.Client().Get(srv.URL + "/api/players/not-a-uuid")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestAdmin_QueueDepth(t *testing.T) {
	srv := newServer(t)
	resp, err := srv.Client().Get(srv.URL + "/api/matchmaking/rps")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		GameMode   string `json:"game_mode"`
		QueueDepth int32  `json:"queue_depth"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.GameMode != "rps" {
		t.Errorf("game_mode = %q, want rps", body.GameMode)
	}
	if body.QueueDepth != 0 {
		t.Errorf("queue_depth = %d, want 0 (empty in-mem queue)", body.QueueDepth)
	}
}
