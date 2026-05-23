package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/averak/vfx/internal/domain/player"
	"github.com/averak/vfx/internal/domain/storage"
	"github.com/averak/vfx/internal/infra/db"
	"github.com/averak/vfx/internal/infra/repository"
	"github.com/averak/vfx/internal/testutils/testdb"
)

const (
	fileA       = "a.dat"
	titleConfig = "config.json"
	titleBanner = "banner.png"
	tagProd     = "prod"
)

func newSession(t *testing.T) *db.Session {
	t.Helper()
	return db.NewSession(testdb.Pool(t))
}

func mustRW(t *testing.T, s *db.Session, fn func(ctx context.Context) error) {
	t.Helper()
	if err := s.RW(t.Context(), fn); err != nil {
		t.Fatalf("RW: %v", err)
	}
}

func seedPlayer(t *testing.T, s *db.Session) uuid.UUID {
	t.Helper()
	id := uuid.New()
	mustRW(t, s, func(ctx context.Context) error {
		p, err := player.New(id, nil, time.Now().UTC())
		if err != nil {
			return err
		}
		return repository.NewPlayer().Save(ctx, p)
	})
	return id
}

func TestPlayerFile_UpsertGetListUsageDelete(t *testing.T) {
	s := newSession(t)
	owner := seedPlayer(t, s)
	repo := repository.NewPlayerFile()
	now := time.Now().UTC()

	mustRW(t, s, func(ctx context.Context) error {
		if err := repo.SaveFile(ctx, owner, &storage.File{Filename: fileA, Size: 100, Hash: "h1", UpdatedAt: now}); err != nil {
			return err
		}
		return repo.SaveFile(ctx, owner, &storage.File{Filename: "b.dat", Size: 50, Hash: "h2", UpdatedAt: now})
	})

	// SaveFile on an existing (owner, filename) overwrites size and hash rather than inserting a duplicate.
	mustRW(t, s, func(ctx context.Context) error {
		return repo.SaveFile(ctx, owner, &storage.File{Filename: fileA, Size: 200, Hash: "h1b", UpdatedAt: now.Add(time.Minute)})
	})
	mustRW(t, s, func(ctx context.Context) error {
		got, err := repo.GetFile(ctx, owner, fileA)
		if err != nil {
			return err
		}
		if got.Size != 200 || got.Hash != "h1b" {
			t.Errorf("overwrite not applied: size=%d hash=%s", got.Size, got.Hash)
		}
		return nil
	})

	mustRW(t, s, func(ctx context.Context) error {
		all, err := repo.ListFiles(ctx, owner, "")
		if err != nil {
			return err
		}
		if len(all) != 2 {
			t.Errorf("empty prefix listed %d files, want 2", len(all))
		}
		onlyA, err := repo.ListFiles(ctx, owner, "a")
		if err != nil {
			return err
		}
		if len(onlyA) != 1 || onlyA[0].Filename != fileA {
			t.Errorf("prefix \"a\" listed %+v, want [a.dat]", onlyA)
		}
		return nil
	})

	mustRW(t, s, func(ctx context.Context) error {
		total, count, err := repo.Usage(ctx, owner)
		if err != nil {
			return err
		}
		if total != 250 || count != 2 {
			t.Errorf("usage = (%d bytes, %d files), want (250, 2)", total, count)
		}
		return nil
	})

	mustRW(t, s, func(ctx context.Context) error {
		if err := repo.DeleteFile(ctx, owner, fileA); err != nil {
			return err
		}
		// A second delete of the same name must not error: the port is idempotent.
		return repo.DeleteFile(ctx, owner, fileA)
	})
	err := s.RW(t.Context(), func(ctx context.Context) error {
		_, e := repo.GetFile(ctx, owner, fileA)
		return e
	})
	if !errors.Is(err, storage.ErrFileNotFound) {
		t.Errorf("GetFile after delete: err = %v, want ErrFileNotFound", err)
	}
}

func TestPlayerFile_OwnerIsolation(t *testing.T) {
	s := newSession(t)
	alice := seedPlayer(t, s)
	bob := seedPlayer(t, s)
	repo := repository.NewPlayerFile()

	mustRW(t, s, func(ctx context.Context) error {
		return repo.SaveFile(ctx, alice, &storage.File{Filename: "save.dat", Size: 10, Hash: "h", UpdatedAt: time.Now().UTC()})
	})

	err := s.RW(t.Context(), func(ctx context.Context) error {
		_, e := repo.GetFile(ctx, bob, "save.dat")
		return e
	})
	if !errors.Is(err, storage.ErrFileNotFound) {
		t.Errorf("bob read alice's file: err = %v, want ErrFileNotFound", err)
	}
	mustRW(t, s, func(ctx context.Context) error {
		files, err := repo.ListFiles(ctx, bob, "")
		if err != nil {
			return err
		}
		if len(files) != 0 {
			t.Errorf("bob's listing leaked alice's files: %+v", files)
		}
		return nil
	})
}

func TestTitleFile_TagFilter(t *testing.T) {
	s := newSession(t)
	repo := repository.NewTitleFile()
	now := time.Now().UTC()

	mustRW(t, s, func(ctx context.Context) error {
		if err := repo.SaveFile(ctx, &storage.File{Filename: titleConfig, Size: 10, Hash: "h", UpdatedAt: now}, []string{tagProd, "config"}); err != nil {
			return err
		}
		if err := repo.SaveFile(ctx, &storage.File{Filename: titleBanner, Size: 20, Hash: "h", UpdatedAt: now}, []string{tagProd, "content"}); err != nil {
			return err
		}
		return repo.SaveFile(ctx, &storage.File{Filename: "dev.json", Size: 5, Hash: "h", UpdatedAt: now}, []string{"dev"})
	})

	tests := []struct {
		name string
		tags []string
		want []string
	}{
		{"single tag matches any file carrying it", []string{tagProd}, []string{titleBanner, titleConfig}},
		{"multiple tags require all", []string{tagProd, "config"}, []string{titleConfig}},
		{"empty tags lists everything", nil, []string{titleBanner, titleConfig, "dev.json"}},
		{"unknown tag matches nothing", []string{"missing"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mustRW(t, s, func(ctx context.Context) error {
				files, err := repo.ListFiles(ctx, tt.tags)
				if err != nil {
					return err
				}
				got := make([]string, len(files))
				for i, f := range files {
					got[i] = f.Filename
				}
				if !equalStrings(got, tt.want) {
					t.Errorf("tags %v listed %v, want %v", tt.tags, got, tt.want)
				}
				return nil
			})
		})
	}

	err := s.RW(t.Context(), func(ctx context.Context) error {
		_, e := repo.GetFile(ctx, "nope.json")
		return e
	})
	if !errors.Is(err, storage.ErrFileNotFound) {
		t.Errorf("GetFile on missing title file: err = %v, want ErrFileNotFound", err)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
