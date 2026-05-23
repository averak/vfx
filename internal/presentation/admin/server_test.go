package admin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/averak/vfx/internal/domain/player"
	"github.com/averak/vfx/internal/infra/db"
	"github.com/averak/vfx/internal/infra/matchqueue"
	"github.com/averak/vfx/internal/infra/repository"
	adminhandler "github.com/averak/vfx/internal/presentation/admin"
	"github.com/averak/vfx/internal/testutils/fakeblob"
	"github.com/averak/vfx/internal/testutils/testdb"
	usecaseadmin "github.com/averak/vfx/internal/usecase/admin"
	usecasestorage "github.com/averak/vfx/internal/usecase/storage"
)

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv, _ := newServerAndPool(t, "")
	return srv
}

func newServerWithToken(t *testing.T, token string) *httptest.Server {
	t.Helper()
	srv, _ := newServerAndPool(t, token)
	return srv
}

func newServerAndPool(t *testing.T, token string) (*httptest.Server, *pgxpool.Pool) {
	t.Helper()
	pool := testdb.Pool(t)
	uc := usecaseadmin.New(db.NewSession(pool), repository.NewPlayer(), matchqueue.NewInMem())
	session := db.NewSession(pool)
	storageUC := usecasestorage.New(session, session, repository.NewPlayerFile(), repository.NewTitleFile(), fakeblob.New(), usecasestorage.Config{TitlePrefix: "title"})
	srv := httptest.NewServer(adminhandler.NewHandler(uc, storageUC, pool, token))
	t.Cleanup(srv.Close)
	return srv, pool
}

func seedPlayer(t *testing.T, pool *pgxpool.Pool, nickname string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if err := db.NewSession(pool).RW(t.Context(), func(ctx context.Context) error {
		p, err := player.New(id, &nickname, time.Now().UTC())
		if err != nil {
			return err
		}
		return repository.NewPlayer().Save(ctx, p)
	}); err != nil {
		t.Fatalf("seed player: %v", err)
	}
	return id
}

func TestAdmin_PublishAndDeleteTitleFile(t *testing.T) {
	srv := newServer(t)
	body := []byte(`{"motd":"hello"}`)

	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPut, srv.URL+"/api/title-files/motd.json?tags=prod,config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT status = %d, want 200", resp.StatusCode)
	}
	var view struct {
		Filename string   `json:"filename"`
		Size     uint64   `json:"size"`
		Hash     string   `json:"hash"`
		Tags     []string `json:"tags"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if view.Filename != "motd.json" || view.Size != uint64(len(body)) || view.Hash == "" {
		t.Errorf("published view = %+v", view)
	}
	if len(view.Tags) != 2 || view.Tags[0] != "prod" || view.Tags[1] != "config" {
		t.Errorf("tags = %v, want [prod config]", view.Tags)
	}

	del := func() int {
		dreq, _ := http.NewRequestWithContext(t.Context(), http.MethodDelete, srv.URL+"/api/title-files/motd.json", http.NoBody)
		dresp, derr := srv.Client().Do(dreq)
		if derr != nil {
			t.Fatalf("DELETE: %v", derr)
		}
		_ = dresp.Body.Close()
		return dresp.StatusCode
	}
	if got := del(); got != http.StatusNoContent {
		t.Errorf("DELETE status = %d, want 204", got)
	}
	// A second delete is a NotFound: the metadata is gone.
	if got := del(); got != http.StatusNotFound {
		t.Errorf("second DELETE status = %d, want 404", got)
	}
}

// With no object store configured the title-file endpoints are not mounted at all.
func TestAdmin_TitleFilesUnmountedWithoutStorage(t *testing.T) {
	pool := testdb.Pool(t)
	uc := usecaseadmin.New(db.NewSession(pool), repository.NewPlayer(), matchqueue.NewInMem())
	srv := httptest.NewServer(adminhandler.NewHandler(uc, nil, pool, ""))
	t.Cleanup(srv.Close)

	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPut, srv.URL+"/api/title-files/x.json", bytes.NewReader([]byte("{}")))
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("PUT with storage disabled = %d, want 404 (route unmounted)", resp.StatusCode)
	}
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

func TestAdmin_GetPlayerReturnsRecord(t *testing.T) {
	srv, pool := newServerAndPool(t, "")
	id := seedPlayer(t, pool, "Tester")

	resp, err := srv.Client().Get(srv.URL + "/api/players/" + id.String())
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		ID       string  `json:"id"`
		Nickname *string `json:"nickname"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.ID != id.String() {
		t.Errorf("id = %q, want %q", body.ID, id)
	}
	if body.Nickname == nil || *body.Nickname != "Tester" {
		t.Errorf("nickname = %v, want Tester", body.Nickname)
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

func TestAdmin_AuthRequiredWhenTokenSet(t *testing.T) {
	srv := newServerWithToken(t, "s3cret")
	get := func(authHeader string) int {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/matchmaking/rps", http.NoBody)
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}

	if got := get(""); got != http.StatusUnauthorized {
		t.Errorf("no token: status = %d, want 401", got)
	}
	if got := get("Bearer wrong"); got != http.StatusUnauthorized {
		t.Errorf("wrong token: status = %d, want 401", got)
	}
	if got := get("Bearer s3cret"); got != http.StatusOK {
		t.Errorf("correct token: status = %d, want 200", got)
	}

	// Probes stay open even with a token configured.
	resp, err := srv.Client().Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("/healthz with token configured: status = %d, want 200", resp.StatusCode)
	}
}
