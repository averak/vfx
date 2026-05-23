// Package storage orchestrates the file-storage services.
//
// The gateway is the control plane: it authorizes, enforces quotas, owns the metadata, and issues signed URLs.
// File bytes move directly between the client and the blob store over those URLs; the gateway never proxies them.
package storage

import (
	"context"
	"errors"
	"path"
	"time"

	"github.com/google/uuid"

	domainstorage "github.com/averak/vfx/internal/domain/storage"
	"github.com/averak/vfx/internal/stdx/clock"
	"github.com/averak/vfx/internal/usecase/tx"
)

var (
	ErrQuotaExceeded = errors.New("storage: player storage quota exceeded")

	// ErrUploadIncomplete is returned by CommitPlayerFile when no object is present at the key: the client never finished the upload it requested with WriteFile.
	ErrUploadIncomplete = errors.New("storage: upload not found; nothing to commit")
)

type Config struct {
	// PlayerDataPrefix and TitlePrefix namespace the two buckets within the object store; an owner id is appended under PlayerDataPrefix so one player's keys can never collide with another's.
	PlayerDataPrefix string
	TitlePrefix      string

	URLTTL time.Duration

	MaxBytesPerPlayer uint64
	MaxFilesPerPlayer int
}

type Usecase struct {
	rw         tx.ReadWriter
	ro         tx.Reader
	playerRepo domainstorage.PlayerFileRepository
	titleRepo  domainstorage.TitleFileRepository
	blobs      BlobStore
	cfg        Config
}

func New(
	rw tx.ReadWriter,
	ro tx.Reader,
	playerRepo domainstorage.PlayerFileRepository,
	titleRepo domainstorage.TitleFileRepository,
	blobs BlobStore,
	cfg Config,
) *Usecase {
	return &Usecase{
		rw:         rw,
		ro:         ro,
		playerRepo: playerRepo,
		titleRepo:  titleRepo,
		blobs:      blobs,
		cfg:        cfg,
	}
}

func (u *Usecase) QueryPlayerFiles(ctx context.Context, ownerID uuid.UUID, prefix string) ([]*domainstorage.File, error) {
	var files []*domainstorage.File
	err := u.ro.RO(ctx, func(ctx context.Context) error {
		var err error
		files, err = u.playerRepo.ListFiles(ctx, ownerID, prefix)
		return err
	})
	return files, err
}

// ReadPlayerFile returns the metadata and a download URL, or domainstorage.ErrFileNotFound when the owner has no such file.
func (u *Usecase) ReadPlayerFile(ctx context.Context, ownerID uuid.UUID, filename string) (*domainstorage.File, *SignedURL, error) {
	if err := domainstorage.ValidateFilename(filename); err != nil {
		return nil, nil, err
	}
	var file *domainstorage.File
	err := u.ro.RO(ctx, func(ctx context.Context) error {
		var err error
		file, err = u.playerRepo.GetFile(ctx, ownerID, filename)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	// Signing is a blob-store call, so it runs outside the transaction; holding a DB tx open across a network round-trip would be wasteful and is unnecessary here.
	url, err := u.blobs.SignDownload(ctx, u.playerKey(ownerID, filename), u.cfg.URLTTL)
	if err != nil {
		return nil, nil, err
	}
	return file, &url, nil
}

// WritePlayerFile issues an upload URL after a quota precheck; it does not persist any metadata.
// The precheck uses the client-declared size, and CommitPlayerFile re-checks against the object's actual size, so a client that under-declares here is still rejected at commit.
func (u *Usecase) WritePlayerFile(ctx context.Context, ownerID uuid.UUID, filename string, size uint64) (*SignedURL, error) {
	if err := domainstorage.ValidateFilename(filename); err != nil {
		return nil, err
	}
	if err := domainstorage.ValidateSize(size); err != nil {
		return nil, err
	}
	err := u.ro.RO(ctx, func(ctx context.Context) error {
		return u.checkQuota(ctx, ownerID, filename, size)
	})
	if err != nil {
		return nil, err
	}
	url, err := u.blobs.SignUpload(ctx, u.playerKey(ownerID, filename), u.cfg.URLTTL)
	if err != nil {
		return nil, err
	}
	return &url, nil
}

// CommitPlayerFile finalizes an upload: it reads the object's authoritative size and hash from the blob store and persists the metadata.
// It returns ErrUploadIncomplete when no object is present (the upload never completed), and never trusts a client-supplied size or hash.
func (u *Usecase) CommitPlayerFile(ctx context.Context, ownerID uuid.UUID, filename string) (*domainstorage.File, error) {
	if err := domainstorage.ValidateFilename(filename); err != nil {
		return nil, err
	}
	attrs, err := u.blobs.Stat(ctx, u.playerKey(ownerID, filename))
	if err != nil {
		if errors.Is(err, ErrObjectNotFound) {
			return nil, ErrUploadIncomplete
		}
		return nil, err
	}
	file, err := domainstorage.NewFile(filename, attrs.Size, attrs.Hash, clock.Now(ctx))
	if err != nil {
		return nil, err
	}
	err = u.rw.RW(ctx, func(ctx context.Context) error {
		if qErr := u.checkQuota(ctx, ownerID, filename, file.Size); qErr != nil {
			return qErr
		}
		return u.playerRepo.SaveFile(ctx, ownerID, file)
	})
	if err != nil {
		return nil, err
	}
	return file, nil
}

// DeletePlayerFile removes the file, returning domainstorage.ErrFileNotFound when the owner has no such file.
func (u *Usecase) DeletePlayerFile(ctx context.Context, ownerID uuid.UUID, filename string) error {
	if err := domainstorage.ValidateFilename(filename); err != nil {
		return err
	}
	// GetFile first so deleting an unknown name surfaces NotFound instead of silently succeeding.
	err := u.rw.RW(ctx, func(ctx context.Context) error {
		if _, err := u.playerRepo.GetFile(ctx, ownerID, filename); err != nil {
			return err
		}
		return u.playerRepo.DeleteFile(ctx, ownerID, filename)
	})
	if err != nil {
		return err
	}

	// The metadata row is deleted first (above), the blob second, on purpose.
	// The metadata is the source of truth for what exists, so once the row is gone the file is already invisible to every API; the blob delete is therefore best-effort.
	// If it fails, the bytes linger as an orphan object with no metadata row, which the bucket's lifecycle/sweep reclaims later — a transient blob-store error must not be reported back as a failed delete and resurrect a file the client already removed.
	//nolint:errcheck // Best-effort by design; see the comment above. The orphan is reclaimed out of band.
	_ = u.blobs.Delete(ctx, u.playerKey(ownerID, filename))
	return nil
}

func (u *Usecase) QueryTitleFiles(ctx context.Context, tags []string) ([]*domainstorage.File, error) {
	var files []*domainstorage.File
	err := u.ro.RO(ctx, func(ctx context.Context) error {
		var err error
		files, err = u.titleRepo.ListFiles(ctx, tags)
		return err
	})
	return files, err
}

func (u *Usecase) ReadTitleFile(ctx context.Context, filename string) (*domainstorage.File, *SignedURL, error) {
	if err := domainstorage.ValidateFilename(filename); err != nil {
		return nil, nil, err
	}
	var file *domainstorage.File
	err := u.ro.RO(ctx, func(ctx context.Context) error {
		var err error
		file, err = u.titleRepo.GetFile(ctx, filename)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	url, err := u.blobs.SignDownload(ctx, u.titleKey(filename), u.cfg.URLTTL)
	if err != nil {
		return nil, nil, err
	}
	return file, &url, nil
}

// checkQuota rejects a write that would push the owner past their byte or file-count limit.
// An overwrite replaces the existing file, so its current size frees up and the count does not grow; a new name adds one to the count.
func (u *Usecase) checkQuota(ctx context.Context, ownerID uuid.UUID, filename string, size uint64) error {
	total, count, err := u.playerRepo.Usage(ctx, ownerID)
	if err != nil {
		return err
	}
	existing, err := u.playerRepo.GetFile(ctx, ownerID, filename)
	switch {
	case err == nil:
		total -= existing.Size
	case errors.Is(err, domainstorage.ErrFileNotFound):
		count++
	default:
		return err
	}
	if total+size > u.cfg.MaxBytesPerPlayer || count > u.cfg.MaxFilesPerPlayer {
		return ErrQuotaExceeded
	}
	return nil
}

func (u *Usecase) playerKey(ownerID uuid.UUID, filename string) string {
	return path.Join(u.cfg.PlayerDataPrefix, ownerID.String(), filename)
}

func (u *Usecase) titleKey(filename string) string {
	return path.Join(u.cfg.TitlePrefix, filename)
}
