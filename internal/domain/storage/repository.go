package storage

import (
	"context"

	"github.com/google/uuid"
)

// PlayerFileRepository persists player-data File metadata, scoped to an owner.
// Every method keys on ownerID so one player can never observe or touch another's files.
type PlayerFileRepository interface {
	// GetFile returns ErrFileNotFound when the owner has no file by that name.
	GetFile(ctx context.Context, ownerID uuid.UUID, filename string) (*File, error)

	ListFiles(ctx context.Context, ownerID uuid.UUID, prefix string) ([]*File, error)

	// SaveFile upserts on (owner, filename): committing a new version of an existing name overwrites its metadata.
	SaveFile(ctx context.Context, ownerID uuid.UUID, f *File) error

	// DeleteFile is idempotent: deleting a name that is already gone returns nil, so a retried delete (or one racing object-store cleanup) is harmless.
	DeleteFile(ctx context.Context, ownerID uuid.UUID, filename string) error

	// Usage reports the owner's current totals for the quota check.
	Usage(ctx context.Context, ownerID uuid.UUID) (totalSize uint64, count int, err error)
}

// TitleFileRepository reads operator-written title files; there is no write method because clients never write here.
type TitleFileRepository interface {
	// ListFiles returns files carrying every tag in tags; an empty tags returns all files.
	ListFiles(ctx context.Context, tags []string) ([]*File, error)

	// GetFile returns ErrFileNotFound when no title file by that name exists.
	GetFile(ctx context.Context, filename string) (*File, error)
}
