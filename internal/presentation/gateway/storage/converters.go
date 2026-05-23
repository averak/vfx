// Package storage wires the storage services onto the usecase.
//
// Two Connect handlers live here because PlayerDataStorageService and TitleStorageService each declare QueryFiles and ReadFile, and one Go type cannot carry two methods of the same name.
// Both translate proto to domain and map domain/usecase sentinel errors to Connect codes; the business rules stay in the usecase.
package storage

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	storagev1 "github.com/averak/vfx/gen/go/vfx/v1/storage"
	domainstorage "github.com/averak/vfx/internal/domain/storage"
	"github.com/averak/vfx/internal/infra/connectrpc/authctx"
	usecasestorage "github.com/averak/vfx/internal/usecase/storage"
)

func requireAuth(ctx context.Context) (uuid.UUID, error) {
	id, ok := authctx.From(ctx)
	if !ok {
		return uuid.Nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	return id, nil
}

func toFileMetadataPb(f *domainstorage.File) *storagev1.FileMetadata {
	return &storagev1.FileMetadata{
		Filename:  f.Filename,
		Size:      f.Size,
		Hash:      f.Hash,
		UpdatedAt: timestamppb.New(f.UpdatedAt),
	}
}

func toFileMetadataListPb(files []*domainstorage.File) []*storagev1.FileMetadata {
	out := make([]*storagev1.FileMetadata, len(files))
	for i, f := range files {
		out[i] = toFileMetadataPb(f)
	}
	return out
}

// toConnectError maps domain and usecase sentinel errors to Connect codes; anything else falls through as Internal so unexpected failures stay loud.
func toConnectError(err error) error {
	switch {
	case errors.Is(err, domainstorage.ErrInvalidFilename), errors.Is(err, domainstorage.ErrFileTooLarge):
		return connect.NewError(connect.CodeInvalidArgument, err)
	case errors.Is(err, domainstorage.ErrFileNotFound):
		return connect.NewError(connect.CodeNotFound, err)
	case errors.Is(err, usecasestorage.ErrQuotaExceeded):
		return connect.NewError(connect.CodeResourceExhausted, err)
	case errors.Is(err, usecasestorage.ErrUploadIncomplete):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	default:
		return connect.NewError(connect.CodeInternal, err)
	}
}
