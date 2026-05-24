package storage

import (
	"context"

	"github.com/google/uuid"
)

// Every method keys on ownerID, so one player can never observe or touch another's files.
type PlayerFileRepository interface {
	// GetFile returns ErrFileNotFound when the owner has no file by that name.
	GetFile(ctx context.Context, ownerID uuid.UUID, filename string) (*File, error)

	ListFiles(ctx context.Context, ownerID uuid.UUID, prefix string) ([]*File, error)

	// SaveFile upserts on (owner, filename), so re-committing a name overwrites its metadata.
	SaveFile(ctx context.Context, ownerID uuid.UUID, f *File) error

	// DeleteFile is idempotent: deleting an already-absent name returns nil, so a retried delete is harmless.
	DeleteFile(ctx context.Context, ownerID uuid.UUID, filename string) error

	Usage(ctx context.Context, ownerID uuid.UUID) (totalSize uint64, count int, err error)
}

// Clients only read title files; SaveFile and DeleteFile are the operator-side publish/unpublish path, not reachable from a player RPC.
type TitleFileRepository interface {
	// ListFiles returns files carrying every tag in tags (AND); an empty tags returns all files.
	ListFiles(ctx context.Context, tags []string) ([]*File, error)

	// GetFile returns ErrFileNotFound when no title file by that name exists.
	GetFile(ctx context.Context, filename string) (*File, error)

	// SaveFile upserts on filename and replaces the file's tags.
	SaveFile(ctx context.Context, f *File, tags []string) error

	// DeleteFile is idempotent: removing an already-absent name returns nil.
	DeleteFile(ctx context.Context, filename string) error
}
