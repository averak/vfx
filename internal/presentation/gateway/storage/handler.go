package storage

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	storagev1 "github.com/averak/vfx/gen/go/vfx/v1/storage"
	"github.com/averak/vfx/gen/go/vfx/v1/storage/storageconnect"
	usecasestorage "github.com/averak/vfx/internal/usecase/storage"
)

type PlayerDataHandler struct {
	uc *usecasestorage.Usecase
}

var _ storageconnect.PlayerDataStorageServiceHandler = (*PlayerDataHandler)(nil)

func NewPlayerDataHandler(uc *usecasestorage.Usecase) *PlayerDataHandler {
	return &PlayerDataHandler{uc: uc}
}

func (h *PlayerDataHandler) QueryFiles(ctx context.Context, req *connect.Request[storagev1.QueryFilesRequest]) (*connect.Response[storagev1.QueryFilesResponse], error) {
	ownerID, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	files, err := h.uc.QueryPlayerFiles(ctx, ownerID, req.Msg.GetPrefix())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&storagev1.QueryFilesResponse{Files: toFileMetadataListPb(files)}), nil
}

func (h *PlayerDataHandler) ReadFile(ctx context.Context, req *connect.Request[storagev1.ReadFileRequest]) (*connect.Response[storagev1.ReadFileResponse], error) {
	ownerID, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	file, url, err := h.uc.ReadPlayerFile(ctx, ownerID, req.Msg.GetFilename())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&storagev1.ReadFileResponse{
		Metadata:     toFileMetadataPb(file),
		DownloadUrl:  url.URL,
		UrlExpiresAt: timestamppb.New(url.Expires),
	}), nil
}

func (h *PlayerDataHandler) WriteFile(ctx context.Context, req *connect.Request[storagev1.WriteFileRequest]) (*connect.Response[storagev1.WriteFileResponse], error) {
	ownerID, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	url, err := h.uc.WritePlayerFile(ctx, ownerID, req.Msg.GetFilename(), req.Msg.GetSize())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&storagev1.WriteFileResponse{
		UploadUrl:       url.URL,
		RequiredHeaders: url.Headers,
		UrlExpiresAt:    timestamppb.New(url.Expires),
	}), nil
}

func (h *PlayerDataHandler) CommitFile(ctx context.Context, req *connect.Request[storagev1.CommitFileRequest]) (*connect.Response[storagev1.CommitFileResponse], error) {
	ownerID, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	file, err := h.uc.CommitPlayerFile(ctx, ownerID, req.Msg.GetFilename())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&storagev1.CommitFileResponse{Metadata: toFileMetadataPb(file)}), nil
}

func (h *PlayerDataHandler) DeleteFile(ctx context.Context, req *connect.Request[storagev1.DeleteFileRequest]) (*connect.Response[storagev1.DeleteFileResponse], error) {
	ownerID, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.uc.DeletePlayerFile(ctx, ownerID, req.Msg.GetFilename()); err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&storagev1.DeleteFileResponse{}), nil
}

// TitleHandler requires authentication but not ownership: title files are global, so any logged-in player may read those visible under the tags they request.
type TitleHandler struct {
	uc *usecasestorage.Usecase
}

var _ storageconnect.TitleStorageServiceHandler = (*TitleHandler)(nil)

func NewTitleHandler(uc *usecasestorage.Usecase) *TitleHandler {
	return &TitleHandler{uc: uc}
}

func (h *TitleHandler) QueryFiles(ctx context.Context, req *connect.Request[storagev1.TitleStorageServiceQueryFilesRequest]) (*connect.Response[storagev1.TitleStorageServiceQueryFilesResponse], error) {
	if _, err := requireAuth(ctx); err != nil {
		return nil, err
	}
	files, err := h.uc.QueryTitleFiles(ctx, req.Msg.GetTags())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&storagev1.TitleStorageServiceQueryFilesResponse{Files: toFileMetadataListPb(files)}), nil
}

func (h *TitleHandler) ReadFile(ctx context.Context, req *connect.Request[storagev1.TitleStorageServiceReadFileRequest]) (*connect.Response[storagev1.TitleStorageServiceReadFileResponse], error) {
	if _, err := requireAuth(ctx); err != nil {
		return nil, err
	}
	file, url, err := h.uc.ReadTitleFile(ctx, req.Msg.GetFilename())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&storagev1.TitleStorageServiceReadFileResponse{
		Metadata:     toFileMetadataPb(file),
		DownloadUrl:  url.URL,
		UrlExpiresAt: timestamppb.New(url.Expires),
	}), nil
}
