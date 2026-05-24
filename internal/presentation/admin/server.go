// Package admin is the operations API's HTTP entry point.
//
// Unlike the player-facing gateway (Connect RPC), the admin API is a small plain-HTTP/JSON surface: it is an internal operations tool, not a typed client contract, and a future web UI will consume the same JSON.
// It is mostly read-only inspection (players, queues); the one write surface is title-file publishing, which is an operator action with no player-facing equivalent.
// The API is expected to sit behind a separate auth boundary (network policy, ingress auth) provided by the deployment.
package admin

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/averak/vfx/internal/domain/player"
	domainstorage "github.com/averak/vfx/internal/domain/storage"
	usecaseadmin "github.com/averak/vfx/internal/usecase/admin"
	usecasestorage "github.com/averak/vfx/internal/usecase/storage"
)

// maxTitleFileBytes caps an in-request title upload; title content is small config/assets, and the operator API proxies the bytes rather than streaming, so a bound keeps memory predictable.
const maxTitleFileBytes = 8 << 20 // 8 MiB

// NewHandler gates /api behind a bearer token when authToken is non-empty, while leaving the health probes open so orchestrators reach them without credentials.
// storageUC may be nil, in which case the title-file endpoints are not mounted (no object store configured).
func NewHandler(uc *usecaseadmin.Usecase, storageUC *usecasestorage.Usecase, pool *pgxpool.Pool, authToken string) http.Handler {
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

	mux.HandleFunc("GET /api/players/{id}", requireToken(authToken, func(w http.ResponseWriter, r *http.Request) {
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
	}))

	mux.HandleFunc("GET /api/matchmaking/{game_mode}", requireToken(authToken, func(w http.ResponseWriter, r *http.Request) {
		mode := r.PathValue("game_mode")
		depth, err := uc.QueueDepth(r.Context(), mode)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "queue lookup failed")
			return
		}
		writeJSON(w, http.StatusOK, queueView{GameMode: mode, QueueDepth: depth})
	}))

	if storageUC != nil {
		// Publish (or replace) a title file: body is the raw bytes, ?tags=a,b sets its tags, Content-Type is preserved for download.
		mux.HandleFunc("PUT /api/title-files/{name}", requireToken(authToken, func(w http.ResponseWriter, r *http.Request) {
			name := r.PathValue("name")
			tags := splitTags(r.URL.Query().Get("tags"))
			data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxTitleFileBytes))
			if err != nil {
				writeError(w, http.StatusRequestEntityTooLarge, "title file too large or unreadable")
				return
			}
			contentType := r.Header.Get("Content-Type")
			if contentType == "" {
				contentType = "application/octet-stream"
			}
			file, err := storageUC.PublishTitleFile(r.Context(), name, tags, data, contentType)
			if err != nil {
				writeStorageError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, titleFileView{
				Filename:   file.Filename,
				Size:       file.Size,
				Hash:       file.Hash,
				Tags:       tags,
				ModifiedAt: file.ModifiedAt,
			})
		}))

		mux.HandleFunc("DELETE /api/title-files/{name}", requireToken(authToken, func(w http.ResponseWriter, r *http.Request) {
			if err := storageUC.DeleteTitleFile(r.Context(), r.PathValue("name")); err != nil {
				writeStorageError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}))
	}

	return mux
}

// splitTags parses the comma-separated ?tags value, dropping empties so "" yields no tags (not [""]).
func splitTags(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	tags := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}

func writeStorageError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domainstorage.ErrInvalidFilename), errors.Is(err, domainstorage.ErrFileTooLarge):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, domainstorage.ErrFileNotFound):
		writeError(w, http.StatusNotFound, "title file not found")
	default:
		writeError(w, http.StatusInternalServerError, "title file operation failed")
	}
}

// requireToken wraps h with a bearer-token check.
// An empty configured token disables the check (the deployment's network boundary is then the only guard).
// The compare is constant-time to avoid leaking the token through timing.
func requireToken(token string, h http.HandlerFunc) http.HandlerFunc {
	if token == "" {
		return h
	}
	want := []byte("Bearer " + token)
	return func(w http.ResponseWriter, r *http.Request) {
		got := []byte(r.Header.Get("Authorization"))
		if subtle.ConstantTimeCompare(got, want) != 1 {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		h(w, r)
	}
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

type titleFileView struct {
	Filename   string    `json:"filename"`
	Size       uint64    `json:"size"`
	Hash       string    `json:"hash"`
	Tags       []string  `json:"tags"`
	ModifiedAt time.Time `json:"modified_at"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body) //nolint:errcheck // response is best-effort once headers are sent.
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
