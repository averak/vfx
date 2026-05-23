package storage_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/averak/vfx/internal/domain/player"
	domainstorage "github.com/averak/vfx/internal/domain/storage"
	"github.com/averak/vfx/internal/infra/db"
	"github.com/averak/vfx/internal/infra/repository"
	"github.com/averak/vfx/internal/stdx/clock"
	"github.com/averak/vfx/internal/testutils/fakeblob"
	"github.com/averak/vfx/internal/testutils/testdb"
	usecasestorage "github.com/averak/vfx/internal/usecase/storage"
)

func newUsecase(t *testing.T, blob usecasestorage.BlobStore) (*usecasestorage.Usecase, *db.Session) {
	t.Helper()
	session := db.NewSession(testdb.Pool(t))
	uc := usecasestorage.New(
		session,
		session,
		repository.NewPlayerFile(),
		repository.NewTitleFile(),
		blob,
		usecasestorage.Config{
			PlayerDataPrefix:  "player-data",
			TitlePrefix:       "title",
			URLTTL:            5 * time.Minute,
			MaxBytesPerPlayer: 1 << 20, // 1 MiB
			MaxFilesPerPlayer: 3,
		},
	)
	return uc, session
}

func seedPlayer(t *testing.T, s *db.Session) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if err := s.RW(t.Context(), func(ctx context.Context) error {
		p, err := player.New(id, nil, time.Now().UTC())
		if err != nil {
			return err
		}
		return repository.NewPlayer().Save(ctx, p)
	}); err != nil {
		t.Fatalf("seed player: %v", err)
	}
	return id
}

func ctxWithClock(t *testing.T) context.Context {
	t.Helper()
	return clock.With(t.Context(), time.Now().UTC())
}

// The full happy path: WriteFile issues an upload URL, the client "uploads" (put), CommitFile records the verified attrs, and the file becomes visible to Query/Read; Delete then removes both metadata and blob.
func TestPlayerDataLifecycle(t *testing.T) {
	blob := fakeblob.New()
	uc, s := newUsecase(t, blob)
	ctx := ctxWithClock(t)
	owner := seedPlayer(t, s)

	up, err := uc.WritePlayerFile(ctx, owner, "save.dat", 64)
	if err != nil {
		t.Fatalf("WritePlayerFile: %v", err)
	}
	if up.Method != http.MethodPut || up.URL == "" {
		t.Fatalf("unexpected upload URL: %+v", up)
	}
	// No metadata is written until commit.
	if files, _ := uc.QueryPlayerFiles(ctx, owner, ""); len(files) != 0 {
		t.Fatalf("WriteFile leaked metadata before commit: %+v", files)
	}

	key := "player-data/" + owner.String() + "/save.dat"
	blob.Put(key, usecasestorage.ObjectAttrs{Size: 64, Hash: "abc123"})

	committed, err := uc.CommitPlayerFile(ctx, owner, "save.dat")
	if err != nil {
		t.Fatalf("CommitPlayerFile: %v", err)
	}
	if committed.Size != 64 || committed.Hash != "abc123" {
		t.Errorf("commit recorded wrong attrs: %+v", committed)
	}

	files, err := uc.QueryPlayerFiles(ctx, owner, "")
	if err != nil {
		t.Fatalf("QueryPlayerFiles: %v", err)
	}
	if len(files) != 1 || files[0].Filename != "save.dat" {
		t.Fatalf("query after commit: %+v", files)
	}

	_, down, err := uc.ReadPlayerFile(ctx, owner, "save.dat")
	if err != nil {
		t.Fatalf("ReadPlayerFile: %v", err)
	}
	if down.Method != http.MethodGet {
		t.Errorf("download URL method = %s, want GET", down.Method)
	}

	if err := uc.DeletePlayerFile(ctx, owner, "save.dat"); err != nil {
		t.Fatalf("DeletePlayerFile: %v", err)
	}
	if blob.Has(key) {
		t.Error("delete left the blob behind")
	}
	if _, _, err := uc.ReadPlayerFile(ctx, owner, "save.dat"); !errors.Is(err, domainstorage.ErrFileNotFound) {
		t.Errorf("read after delete: err = %v, want ErrFileNotFound", err)
	}
}

// CommitPlayerFile reads the object's real size, so a client that under-declares the size at WriteFile (to slip past the precheck) is still rejected at commit.
func TestCommit_RejectsObjectThatExceedsQuota(t *testing.T) {
	blob := fakeblob.New()
	uc, s := newUsecase(t, blob)
	ctx := ctxWithClock(t)
	owner := seedPlayer(t, s)

	if _, err := uc.WritePlayerFile(ctx, owner, "big.dat", 1); err != nil {
		t.Fatalf("WritePlayerFile with a tiny declared size should pass the precheck: %v", err)
	}
	key := "player-data/" + owner.String() + "/big.dat"
	blob.Put(key, usecasestorage.ObjectAttrs{Size: 2 << 20, Hash: "x"}) // 2 MiB > 1 MiB quota

	if _, err := uc.CommitPlayerFile(ctx, owner, "big.dat"); !errors.Is(err, usecasestorage.ErrQuotaExceeded) {
		t.Errorf("commit of an over-quota object: err = %v, want ErrQuotaExceeded", err)
	}
}

func TestCommit_WithoutUploadIsIncomplete(t *testing.T) {
	uc, s := newUsecase(t, fakeblob.New())
	ctx := ctxWithClock(t)
	owner := seedPlayer(t, s)

	if _, err := uc.CommitPlayerFile(ctx, owner, "missing.dat"); !errors.Is(err, usecasestorage.ErrUploadIncomplete) {
		t.Errorf("commit without an upload: err = %v, want ErrUploadIncomplete", err)
	}
}

func TestWrite_RejectsOverFileCountQuota(t *testing.T) {
	blob := fakeblob.New()
	uc, s := newUsecase(t, blob)
	ctx := ctxWithClock(t)
	owner := seedPlayer(t, s)

	// Fill the 3-file quota.
	for _, name := range []string{"a", "b", "c"} {
		if _, err := uc.WritePlayerFile(ctx, owner, name, 1); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		key := "player-data/" + owner.String() + "/" + name
		blob.Put(key, usecasestorage.ObjectAttrs{Size: 1, Hash: "h"})
		if _, err := uc.CommitPlayerFile(ctx, owner, name); err != nil {
			t.Fatalf("commit %s: %v", name, err)
		}
	}

	// A fourth distinct file is rejected, but overwriting an existing one is fine (the count does not grow).
	if _, err := uc.WritePlayerFile(ctx, owner, "d", 1); !errors.Is(err, usecasestorage.ErrQuotaExceeded) {
		t.Errorf("fourth file: err = %v, want ErrQuotaExceeded", err)
	}
	if _, err := uc.WritePlayerFile(ctx, owner, "a", 1); err != nil {
		t.Errorf("overwrite of an existing file rejected by the file-count quota: %v", err)
	}
}

// A failing blob delete must not fail the operation: the metadata row is already gone, so the file is deleted from the client's perspective and the blob is left as an orphan for the sweep.
func TestDelete_BestEffortOnBlobError(t *testing.T) {
	blob := fakeblob.New()
	blob.DeleteErr = errors.New("blob store unavailable")
	uc, s := newUsecase(t, blob)
	ctx := ctxWithClock(t)
	owner := seedPlayer(t, s)

	if _, err := uc.WritePlayerFile(ctx, owner, "save.dat", 8); err != nil {
		t.Fatalf("write: %v", err)
	}
	key := "player-data/" + owner.String() + "/save.dat"
	blob.Put(key, usecasestorage.ObjectAttrs{Size: 8, Hash: "h"})
	if _, err := uc.CommitPlayerFile(ctx, owner, "save.dat"); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if err := uc.DeletePlayerFile(ctx, owner, "save.dat"); err != nil {
		t.Errorf("delete returned an error despite best-effort blob cleanup: %v", err)
	}
	if _, _, err := uc.ReadPlayerFile(ctx, owner, "save.dat"); !errors.Is(err, domainstorage.ErrFileNotFound) {
		t.Errorf("metadata survived a delete whose blob cleanup failed: %v", err)
	}
}

func TestDelete_UnknownFileIsNotFound(t *testing.T) {
	uc, s := newUsecase(t, fakeblob.New())
	ctx := ctxWithClock(t)
	owner := seedPlayer(t, s)

	if err := uc.DeletePlayerFile(ctx, owner, "ghost.dat"); !errors.Is(err, domainstorage.ErrFileNotFound) {
		t.Errorf("delete of unknown file: err = %v, want ErrFileNotFound", err)
	}
}
