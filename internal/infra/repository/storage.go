package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/averak/vfx/internal/domain/storage"
	"github.com/averak/vfx/internal/infra/db"
	"github.com/averak/vfx/internal/infra/dbgen"
)

// PlayerFile is the storage implementation of [storage.PlayerFileRepository].
type PlayerFile struct{}

var _ storage.PlayerFileRepository = (*PlayerFile)(nil)

func NewPlayerFile() *PlayerFile {
	return &PlayerFile{}
}

func (PlayerFile) GetFile(ctx context.Context, ownerID uuid.UUID, filename string) (*storage.File, error) {
	tx, err := db.Tx(ctx)
	if err != nil {
		return nil, err
	}
	row, err := dbgen.New(tx).GetPlayerFile(ctx, dbgen.GetPlayerFileParams{OwnerID: ownerID, Filename: filename})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, storage.ErrFileNotFound
		}
		return nil, err
	}
	return playerFileToDomain(row), nil
}

func (PlayerFile) ListFiles(ctx context.Context, ownerID uuid.UUID, prefix string) ([]*storage.File, error) {
	tx, err := db.Tx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := dbgen.New(tx).ListPlayerFiles(ctx, dbgen.ListPlayerFilesParams{OwnerID: ownerID, Prefix: prefix})
	if err != nil {
		return nil, err
	}
	files := make([]*storage.File, 0, len(rows))
	for _, row := range rows {
		files = append(files, playerFileToDomain(row))
	}
	return files, nil
}

func (PlayerFile) SaveFile(ctx context.Context, ownerID uuid.UUID, f *storage.File) error {
	tx, err := db.Tx(ctx)
	if err != nil {
		return err
	}
	// The fresh id is used only when the row is new; ON CONFLICT (owner, filename) keeps the existing row's id and created_at, updating just size/hash/updated_at.
	_, err = dbgen.New(tx).UpsertPlayerFile(ctx, dbgen.UpsertPlayerFileParams{
		ID:       uuid.New(),
		OwnerID:  ownerID,
		Filename: f.Filename,
		//nolint:gosec // Size is bounded by storage.MaxFileSize, well within int64 range.
		Size:      int64(f.Size),
		Hash:      f.Hash,
		CreatedAt: toTimestamptz(f.UpdatedAt),
		UpdatedAt: toTimestamptz(f.UpdatedAt),
	})
	return err
}

func (PlayerFile) DeleteFile(ctx context.Context, ownerID uuid.UUID, filename string) error {
	tx, err := db.Tx(ctx)
	if err != nil {
		return err
	}
	// Deleting an absent row is a no-op, which is the idempotency the port promises.
	return dbgen.New(tx).DeletePlayerFile(ctx, dbgen.DeletePlayerFileParams{OwnerID: ownerID, Filename: filename})
}

func (PlayerFile) Usage(ctx context.Context, ownerID uuid.UUID) (totalSize uint64, count int, err error) {
	tx, err := db.Tx(ctx)
	if err != nil {
		return 0, 0, err
	}
	row, err := dbgen.New(tx).PlayerStorageUsage(ctx, ownerID)
	if err != nil {
		return 0, 0, err
	}
	//nolint:gosec // The summed size is bounded by the per-player quota and the row count is small; neither overflows.
	return uint64(row.TotalSize), int(row.FileCount), nil
}

// TitleFile is the storage implementation of [storage.TitleFileRepository], plus a SaveFile that is not part of the read-only port and exists for operator-side seeding (admin tooling, tests).
type TitleFile struct{}

var _ storage.TitleFileRepository = (*TitleFile)(nil)

func NewTitleFile() *TitleFile {
	return &TitleFile{}
}

func (TitleFile) ListFiles(ctx context.Context, tags []string) ([]*storage.File, error) {
	tx, err := db.Tx(ctx)
	if err != nil {
		return nil, err
	}
	// A nil slice encodes to SQL NULL, and "tags @> NULL" is NULL (no rows); normalize to an empty array so an empty tag set lists everything.
	if tags == nil {
		tags = []string{}
	}
	rows, err := dbgen.New(tx).ListTitleFiles(ctx, tags)
	if err != nil {
		return nil, err
	}
	files := make([]*storage.File, 0, len(rows))
	for _, row := range rows {
		files = append(files, titleFileToDomain(row))
	}
	return files, nil
}

func (TitleFile) GetFile(ctx context.Context, filename string) (*storage.File, error) {
	tx, err := db.Tx(ctx)
	if err != nil {
		return nil, err
	}
	row, err := dbgen.New(tx).GetTitleFile(ctx, filename)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, storage.ErrFileNotFound
		}
		return nil, err
	}
	return titleFileToDomain(row), nil
}

func (TitleFile) SaveFile(ctx context.Context, f *storage.File, tags []string) error {
	tx, err := db.Tx(ctx)
	if err != nil {
		return err
	}
	if tags == nil {
		tags = []string{}
	}
	_, err = dbgen.New(tx).UpsertTitleFile(ctx, dbgen.UpsertTitleFileParams{
		ID:       uuid.New(),
		Filename: f.Filename,
		//nolint:gosec // Size is bounded by storage.MaxFileSize, well within int64 range.
		Size:      int64(f.Size),
		Hash:      f.Hash,
		Tags:      tags,
		CreatedAt: toTimestamptz(f.UpdatedAt),
		UpdatedAt: toTimestamptz(f.UpdatedAt),
	})
	return err
}

func playerFileToDomain(row dbgen.PlayerFile) *storage.File {
	return &storage.File{
		Filename: row.Filename,
		//nolint:gosec // Stored sizes are non-negative and bounded by storage.MaxFileSize.
		Size:      uint64(row.Size),
		Hash:      row.Hash,
		UpdatedAt: row.UpdatedAt.Time,
	}
}

func titleFileToDomain(row dbgen.TitleFile) *storage.File {
	return &storage.File{
		Filename: row.Filename,
		//nolint:gosec // Stored sizes are non-negative and bounded by storage.MaxFileSize.
		Size:      uint64(row.Size),
		Hash:      row.Hash,
		UpdatedAt: row.UpdatedAt.Time,
	}
}
