package storage_test

import (
	"testing"
	"time"

	"connectrpc.com/connect"

	authv1 "github.com/averak/vfx/gen/go/vfx/v1/auth"
	storagev1 "github.com/averak/vfx/gen/go/vfx/v1/storage"
	domainstorage "github.com/averak/vfx/internal/domain/storage"
	"github.com/averak/vfx/internal/testutils/testconnect"
	usecasestorage "github.com/averak/vfx/internal/usecase/storage"
)

const (
	fileX      = "x.dat"
	fileSave   = "save.dat"
	fileSecret = "secret.dat"
	hashAbc    = "abc123"
	titleMotd  = "motd.json"
)

func login(t *testing.T, srv *testconnect.Server, device string) (token, playerID string) {
	t.Helper()
	resp, err := srv.Auth.Login(t.Context(), connect.NewRequest(&authv1.LoginRequest{
		Credential: &authv1.LoginRequest_Anonymous{
			Anonymous: &authv1.AnonymousCredential{DeviceId: &device},
		},
	}))
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	return resp.Msg.GetAccessToken(), resp.Msg.GetPlayer().GetId()
}

func playerKey(playerID, filename string) string {
	return "player-data/" + playerID + "/" + filename
}

func requireCode(t *testing.T, err error, want connect.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("got nil error, want %v", want)
	}
	if got := connect.CodeOf(err); got != want {
		t.Errorf("code = %v, want %v", got, want)
	}
}

func TestPlayerData_RequiresAuth(t *testing.T) {
	srv := testconnect.New(t)
	ctx := t.Context()
	calls := map[string]func() error{
		"QueryFiles": func() error {
			_, e := srv.PlayerData.QueryFiles(ctx, connect.NewRequest(&storagev1.QueryFilesRequest{}))
			return e
		},
		"ReadFile": func() error {
			_, e := srv.PlayerData.ReadFile(ctx, connect.NewRequest(&storagev1.ReadFileRequest{Filename: fileX}))
			return e
		},
		"WriteFile": func() error {
			_, e := srv.PlayerData.WriteFile(ctx, connect.NewRequest(&storagev1.WriteFileRequest{Filename: fileX, Size: 1}))
			return e
		},
		"CommitFile": func() error {
			_, e := srv.PlayerData.CommitFile(ctx, connect.NewRequest(&storagev1.CommitFileRequest{Filename: fileX}))
			return e
		},
		"DeleteFile": func() error {
			_, e := srv.PlayerData.DeleteFile(ctx, connect.NewRequest(&storagev1.DeleteFileRequest{Filename: fileX}))
			return e
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			requireCode(t, call(), connect.CodeUnauthenticated)
		})
	}
}

func TestWriteFile_InvalidFilename(t *testing.T) {
	srv := testconnect.New(t)
	token, _ := login(t, srv, "bad-name")

	_, err := srv.PlayerData.WriteFile(t.Context(),
		testconnect.Authorize(connect.NewRequest(&storagev1.WriteFileRequest{Filename: "a/b", Size: 1}), token))
	requireCode(t, err, connect.CodeInvalidArgument)
}

func TestWriteFile_TooLarge(t *testing.T) {
	srv := testconnect.New(t)
	token, _ := login(t, srv, "too-large")

	_, err := srv.PlayerData.WriteFile(t.Context(),
		testconnect.Authorize(connect.NewRequest(&storagev1.WriteFileRequest{
			Filename: "big.dat",
			Size:     domainstorage.MaxFileSize + 1,
		}), token))
	requireCode(t, err, connect.CodeInvalidArgument)
}

func TestWriteFile_QuotaExceeded(t *testing.T) {
	srv := testconnect.New(t)
	token, _ := login(t, srv, "quota")

	// Within the per-file ceiling but over the per-player byte quota.
	_, err := srv.PlayerData.WriteFile(t.Context(),
		testconnect.Authorize(connect.NewRequest(&storagev1.WriteFileRequest{
			Filename: fileSave,
			Size:     testconnect.StorageMaxBytesPerPlayer + 1,
		}), token))
	requireCode(t, err, connect.CodeResourceExhausted)
}

func TestReadFile_NotFound(t *testing.T) {
	srv := testconnect.New(t)
	token, _ := login(t, srv, "read-missing")

	_, err := srv.PlayerData.ReadFile(t.Context(),
		testconnect.Authorize(connect.NewRequest(&storagev1.ReadFileRequest{Filename: "missing.dat"}), token))
	requireCode(t, err, connect.CodeNotFound)
}

func TestCommitFile_UploadIncomplete(t *testing.T) {
	srv := testconnect.New(t)
	token, _ := login(t, srv, "no-upload")
	ctx := t.Context()

	// Reserve an upload URL but never "upload" (no Blob.Put), so the object is absent at commit.
	if _, err := srv.PlayerData.WriteFile(ctx,
		testconnect.Authorize(connect.NewRequest(&storagev1.WriteFileRequest{Filename: fileSave, Size: 8}), token)); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := srv.PlayerData.CommitFile(ctx,
		testconnect.Authorize(connect.NewRequest(&storagev1.CommitFileRequest{Filename: fileSave}), token))
	requireCode(t, err, connect.CodeFailedPrecondition)
}

func TestDeleteFile_NotFound(t *testing.T) {
	srv := testconnect.New(t)
	token, _ := login(t, srv, "del-missing")

	_, err := srv.PlayerData.DeleteFile(t.Context(),
		testconnect.Authorize(connect.NewRequest(&storagev1.DeleteFileRequest{Filename: "ghost.dat"}), token))
	requireCode(t, err, connect.CodeNotFound)
}

// The full RPC lifecycle, asserting the proto translation at each step: the upload URL, the committed metadata (size/hash/filename), the download URL, and that delete removes both the row and the blob.
func TestPlayerData_Lifecycle(t *testing.T) {
	srv := testconnect.New(t)
	token, playerID := login(t, srv, "lifecycle")
	ctx := t.Context()
	data := []byte("save")

	write, err := srv.PlayerData.WriteFile(ctx,
		testconnect.Authorize(connect.NewRequest(&storagev1.WriteFileRequest{Filename: fileSave, Size: uint64(len(data))}), token))
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if write.Msg.GetUploadUrl() == "" {
		t.Error("WriteFile returned an empty upload URL")
	}

	srv.Blob.Put(playerKey(playerID, fileSave), usecasestorage.ObjectAttrs{Size: uint64(len(data)), Hash: hashAbc})

	commit, err := srv.PlayerData.CommitFile(ctx,
		testconnect.Authorize(connect.NewRequest(&storagev1.CommitFileRequest{Filename: fileSave}), token))
	if err != nil {
		t.Fatalf("CommitFile: %v", err)
	}
	if md := commit.Msg.GetMetadata(); md.GetFilename() != fileSave || md.GetSize() != uint64(len(data)) || md.GetHash() != hashAbc {
		t.Errorf("committed metadata = %+v", commit.Msg.GetMetadata())
	}

	query, err := srv.PlayerData.QueryFiles(ctx,
		testconnect.Authorize(connect.NewRequest(&storagev1.QueryFilesRequest{}), token))
	if err != nil {
		t.Fatalf("QueryFiles: %v", err)
	}
	if len(query.Msg.GetFiles()) != 1 || query.Msg.GetFiles()[0].GetHash() != hashAbc {
		t.Fatalf("QueryFiles = %+v", query.Msg.GetFiles())
	}

	read, err := srv.PlayerData.ReadFile(ctx,
		testconnect.Authorize(connect.NewRequest(&storagev1.ReadFileRequest{Filename: fileSave}), token))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if read.Msg.GetDownloadUrl() == "" {
		t.Error("ReadFile returned an empty download URL")
	}

	if _, err = srv.PlayerData.DeleteFile(ctx,
		testconnect.Authorize(connect.NewRequest(&storagev1.DeleteFileRequest{Filename: fileSave}), token)); err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}
	if srv.Blob.Has(playerKey(playerID, fileSave)) {
		t.Error("delete left the blob behind")
	}
	after, err := srv.PlayerData.QueryFiles(ctx,
		testconnect.Authorize(connect.NewRequest(&storagev1.QueryFilesRequest{}), token))
	if err != nil {
		t.Fatalf("QueryFiles after delete: %v", err)
	}
	if len(after.Msg.GetFiles()) != 0 {
		t.Errorf("QueryFiles after delete = %+v, want empty", after.Msg.GetFiles())
	}
}

// One player must never see or read another's files; the handler derives the owner from the token, not the request.
func TestPlayerData_OwnerScoped(t *testing.T) {
	srv := testconnect.New(t)
	ctx := t.Context()
	aliceToken, aliceID := login(t, srv, "alice")
	bobToken, _ := login(t, srv, "bob")

	if _, err := srv.PlayerData.WriteFile(ctx,
		testconnect.Authorize(connect.NewRequest(&storagev1.WriteFileRequest{Filename: fileSecret, Size: 4}), aliceToken)); err != nil {
		t.Fatalf("alice WriteFile: %v", err)
	}
	srv.Blob.Put(playerKey(aliceID, fileSecret), usecasestorage.ObjectAttrs{Size: 4, Hash: "h"})
	if _, err := srv.PlayerData.CommitFile(ctx,
		testconnect.Authorize(connect.NewRequest(&storagev1.CommitFileRequest{Filename: fileSecret}), aliceToken)); err != nil {
		t.Fatalf("alice CommitFile: %v", err)
	}

	bobFiles, err := srv.PlayerData.QueryFiles(ctx,
		testconnect.Authorize(connect.NewRequest(&storagev1.QueryFilesRequest{}), bobToken))
	if err != nil {
		t.Fatalf("bob QueryFiles: %v", err)
	}
	if len(bobFiles.Msg.GetFiles()) != 0 {
		t.Errorf("bob saw alice's files: %+v", bobFiles.Msg.GetFiles())
	}
	_, err = srv.PlayerData.ReadFile(ctx,
		testconnect.Authorize(connect.NewRequest(&storagev1.ReadFileRequest{Filename: fileSecret}), bobToken))
	requireCode(t, err, connect.CodeNotFound)
}

func TestTitle_RequiresAuth(t *testing.T) {
	srv := testconnect.New(t)
	ctx := t.Context()

	_, qErr := srv.Title.QueryFiles(ctx, connect.NewRequest(&storagev1.TitleStorageServiceQueryFilesRequest{}))
	requireCode(t, qErr, connect.CodeUnauthenticated)

	_, rErr := srv.Title.ReadFile(ctx, connect.NewRequest(&storagev1.TitleStorageServiceReadFileRequest{Filename: "x.json"}))
	requireCode(t, rErr, connect.CodeUnauthenticated)
}

func TestTitleReadFile_NotFound(t *testing.T) {
	srv := testconnect.New(t)
	token, _ := login(t, srv, "title-missing")

	_, err := srv.Title.ReadFile(t.Context(),
		testconnect.Authorize(connect.NewRequest(&storagev1.TitleStorageServiceReadFileRequest{Filename: "missing.json"}), token))
	requireCode(t, err, connect.CodeNotFound)
}

func TestTitle_QueryAndRead(t *testing.T) {
	srv := testconnect.New(t)
	token, _ := login(t, srv, "title-reader")
	ctx := t.Context()

	srv.SeedTitleFile(t, &domainstorage.File{
		Filename:  titleMotd,
		Size:      16,
		Hash:      "deadbeef",
		UpdatedAt: time.Now().UTC(),
	}, []string{"prod"})

	query, err := srv.Title.QueryFiles(ctx,
		testconnect.Authorize(connect.NewRequest(&storagev1.TitleStorageServiceQueryFilesRequest{Tags: []string{"prod"}}), token))
	if err != nil {
		t.Fatalf("Title QueryFiles: %v", err)
	}
	if len(query.Msg.GetFiles()) != 1 || query.Msg.GetFiles()[0].GetFilename() != titleMotd {
		t.Fatalf("Title QueryFiles = %+v", query.Msg.GetFiles())
	}

	read, err := srv.Title.ReadFile(ctx,
		testconnect.Authorize(connect.NewRequest(&storagev1.TitleStorageServiceReadFileRequest{Filename: titleMotd}), token))
	if err != nil {
		t.Fatalf("Title ReadFile: %v", err)
	}
	if read.Msg.GetDownloadUrl() == "" {
		t.Error("Title ReadFile returned an empty download URL")
	}
	if read.Msg.GetMetadata().GetHash() != "deadbeef" {
		t.Errorf("Title ReadFile metadata hash = %q, want deadbeef", read.Msg.GetMetadata().GetHash())
	}
}
