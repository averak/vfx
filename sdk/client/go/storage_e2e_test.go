package vfxclient_test

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/averak/vfx/internal/bootstrap"
	domainstorage "github.com/averak/vfx/internal/domain/storage"
	"github.com/averak/vfx/internal/infra/db"
	"github.com/averak/vfx/internal/infra/repository"
	"github.com/averak/vfx/internal/presentation/gateway"
	vfxclient "github.com/averak/vfx/sdk/client/go"
)

// TestStorageE2E drives the storage services through the real composition root and the SDK, against fake-gcs-server and PostgreSQL.
// It proves the SDK hides the upload/commit dance and that bytes flow client<->object-store directly while the gateway only brokers metadata and URLs.
// It is skipped unless both DATABASE_URL and STORAGE_EMULATOR_HOST are set (CI and the compose stack provide them).
func TestStorageE2E(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" || os.Getenv("STORAGE_EMULATOR_HOST") == "" {
		t.Skip("DATABASE_URL and STORAGE_EMULATOR_HOST required for the storage E2E")
	}

	ctx := t.Context()
	bucket := "vfx-e2e-" + uuid.NewString()

	gcsClient, err := storage.NewClient(ctx)
	if err != nil {
		t.Fatalf("storage client: %v", err)
	}
	defer gcsClient.Close()
	// fake-gcs-server needs the bucket to exist before the gateway signs URLs for it.
	if err = gcsClient.Bucket(bucket).Create(ctx, "vfx-e2e", nil); err != nil {
		t.Fatalf("create bucket: %v", err)
	}

	t.Setenv("VFX_STORAGE_BUCKET", bucket)
	t.Setenv("VFX_JWT_SECRET", "e2e-secret")

	container, cleanup, err := bootstrap.NewGateway(ctx)
	if err != nil {
		t.Fatalf("bootstrap gateway: %v", err)
	}
	defer cleanup()

	handler, err := gateway.NewHandler(container)
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}
	srv := httptest.NewServer(handler)
	defer srv.Close()

	c := vfxclient.New(srv.URL)
	if err = c.LoginAnonymous(ctx, "device-"+uuid.NewString(), "Alice"); err != nil {
		t.Fatalf("login: %v", err)
	}

	// Player data: one WriteFile call hides WriteFile->PUT->CommitFile.
	data := []byte("save game payload")
	if err = c.WriteFile(ctx, "save.dat", data); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	files, err := c.QueryFiles(ctx, "")
	if err != nil {
		t.Fatalf("QueryFiles: %v", err)
	}
	if len(files) != 1 || files[0].GetFilename() != "save.dat" {
		t.Fatalf("QueryFiles = %+v, want [save.dat]", files)
	}
	sum := md5.Sum(data)
	if files[0].GetHash() != hex.EncodeToString(sum[:]) {
		t.Errorf("metadata hash = %s, want md5 %s", files[0].GetHash(), hex.EncodeToString(sum[:]))
	}

	got, err := c.ReadFile(ctx, "save.dat")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("ReadFile = %q, want %q", got, data)
	}

	if err = c.DeleteFile(ctx, "save.dat"); err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}
	files, err = c.QueryFiles(ctx, "")
	if err != nil {
		t.Fatalf("QueryFiles after delete: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("QueryFiles after delete = %+v, want empty", files)
	}

	// Title storage: an operator seeds a file, the client reads it but cannot write.
	titleBody := []byte(`{"motd":"hello"}`)
	tag := "e2e-" + uuid.NewString()
	seedTitleFile(ctx, t, gcsClient, bucket, dbURL, "motd.json", titleBody, []string{tag})

	titleFiles, err := c.QueryTitleFiles(ctx, []string{tag})
	if err != nil {
		t.Fatalf("QueryTitleFiles: %v", err)
	}
	if len(titleFiles) != 1 || titleFiles[0].GetFilename() != "motd.json" {
		t.Fatalf("QueryTitleFiles = %+v, want [motd.json]", titleFiles)
	}
	titleGot, err := c.ReadTitleFile(ctx, "motd.json")
	if err != nil {
		t.Fatalf("ReadTitleFile: %v", err)
	}
	if !bytes.Equal(titleGot, titleBody) {
		t.Errorf("ReadTitleFile = %q, want %q", titleGot, titleBody)
	}
}

// seedTitleFile writes a title object straight to the store and inserts its metadata, standing in for the operator-side publish path the client cannot perform.
func seedTitleFile(ctx context.Context, t *testing.T, gcsClient *storage.Client, bucket, dbURL, filename string, body []byte, tags []string) {
	t.Helper()

	// The key must match the usecase's titleKey: "<TitlePrefix>/<filename>", with the default prefix "title".
	w := gcsClient.Bucket(bucket).Object("title/" + filename).NewWriter(ctx)
	if _, err := w.Write(body); err != nil {
		t.Fatalf("seed title object: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("seed title object close: %v", err)
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("seed pool: %v", err)
	}
	defer pool.Close()

	sum := md5.Sum(body)
	session := db.NewSession(pool)
	if err := session.RW(ctx, func(ctx context.Context) error {
		return repository.NewTitleFile().SaveFile(ctx, &domainstorage.File{
			Filename:  filename,
			Size:      uint64(len(body)),
			Hash:      hex.EncodeToString(sum[:]),
			UpdatedAt: time.Now().UTC(),
		}, tags)
	}); err != nil {
		t.Fatalf("seed title metadata: %v", err)
	}
}
