// Package admin is the operations API's HTTP entry point.
//
// Unlike the player-facing gateway (Connect RPC), the admin API is a
// small plain-HTTP/JSON surface: it is an internal operations tool, not
// a typed client contract, and a future web UI will consume the same
// JSON. It is read-only and expected to sit behind a separate auth
// boundary (network policy, ingress auth) provided by the deployment.
package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/averak/vfx/internal/domain/player"
	usecaseadmin "github.com/averak/vfx/internal/usecase/admin"
)

// NewHandler builds the admin HTTP handler.
func NewHandler(uc *usecaseadmin.Usecase, pool *pgxpool.Pool) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := pool.Ping(r.Context()); err != nil {
			http.Error(w, "postgres unreachable: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("GET /api/players/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid player id")
			return
		}
		p, err := uc.GetPlayer(r.Context(), id)
		if err != nil {
			if errors.Is(err, player.ErrPlayerNotFound) {
				writeError(w, http.StatusNotFound, "player not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		writeJSON(w, http.StatusOK, playerView{
			ID:        p.ID.String(),
			Nickname:  p.Nickname,
			CreatedAt: p.CreatedAt,
		})
	})

	mux.HandleFunc("GET /api/matchmaking/{game_mode}", func(w http.ResponseWriter, r *http.Request) {
		mode := r.PathValue("game_mode")
		depth, err := uc.QueueDepth(r.Context(), mode)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "queue lookup failed")
			return
		}
		writeJSON(w, http.StatusOK, queueView{GameMode: mode, QueueDepth: depth})
	})

	return mux
}

type playerView struct {
	ID        string    `json:"id"`
	Nickname  *string   `json:"nickname"`
	CreatedAt time.Time `json:"created_at"`
}

type queueView struct {
	GameMode   string `json:"game_mode"`
	QueueDepth int32  `json:"queue_depth"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body) //nolint:errcheck // response is best-effort once headers are sent.
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
