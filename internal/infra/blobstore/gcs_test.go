package blobstore_test

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/google/uuid"

	"github.com/averak/vfx/internal/infra/blobstore"
	usecasestorage "github.com/averak/vfx/internal/usecase/storage"
)

// These tests run against fake-gcs-server, which the compose stack provides.
// They are skipped when STORAGE_EMULATOR_HOST is unset, so the pure-logic suite still passes on a machine without it.
func emulatorHost(t *testing.T) string {
	t.Helper()
	host := os.Getenv("STORAGE_EMULATOR_HOST")
	if host == "" {
		t.Skip("STORAGE_EMULATOR_HOST not set; skipping blob-store integration test")
	}
	return host
}

func newBucket(t *testing.T) *blobstore.GCS {
	t.Helper()
	emulatorHost(t)
	ctx := t.Context()

	bucket := "vfx-test-" + uuid.NewString()
	client, err := storage.NewClient(ctx)
	if err != nil {
		t.Fatalf("storage client: %v", err)
	}
	defer client.Close()
	if err = client.Bucket(bucket).Create(ctx, "vfx-test", nil); err != nil {
		t.Fatalf("create bucket: %v", err)
	}

	gcs, cleanup, err := blobstore.NewGCS(ctx, blobstore.Config{Bucket: bucket, Emulated: true})
	if err != nil {
		t.Fatalf("NewGCS: %v", err)
	}
	t.Cleanup(cleanup)
	return gcs
}

// The full byte round-trip: a signed PUT stores the bytes, Stat reports the right size and md5, a signed GET reads them back, and Delete makes the object vanish.
func TestGCS_RoundTrip(t *testing.T) {
	gcs := newBucket(t)
	ctx := t.Context()
	key := "player-data/" + uuid.NewString() + "/save.dat"
	body := []byte("hello vfx storage")

	up, err := gcs.SignUpload(ctx, key, 5*time.Minute)
	if err != nil {
		t.Fatalf("SignUpload: %v", err)
	}
	doRequest(t, up, body)

	attrs, err := gcs.Stat(ctx, key)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if attrs.Size != uint64(len(body)) {
		t.Errorf("Stat size = %d, want %d", attrs.Size, len(body))
	}
	sum := md5.Sum(body)
	if attrs.Hash != hex.EncodeToString(sum[:]) {
		t.Errorf("Stat hash = %s, want md5 %s", attrs.Hash, hex.EncodeToString(sum[:]))
	}

	down, err := gcs.SignDownload(ctx, key, 5*time.Minute)
	if err != nil {
		t.Fatalf("SignDownload: %v", err)
	}
	got := doRequest(t, down, nil)
	if !bytes.Equal(got, body) {
		t.Errorf("download = %q, want %q", got, body)
	}

	if err := gcs.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := gcs.Stat(ctx, key); !errors.Is(err, usecasestorage.ErrObjectNotFound) {
		t.Errorf("Stat after delete: err = %v, want ErrObjectNotFound", err)
	}
}

func TestGCS_StatMissingIsNotFound(t *testing.T) {
	gcs := newBucket(t)
	if _, err := gcs.Stat(t.Context(), "player-data/none/ghost.dat"); !errors.Is(err, usecasestorage.ErrObjectNotFound) {
		t.Errorf("Stat on missing object: err = %v, want ErrObjectNotFound", err)
	}
}

// Delete of an absent object is success: the caller only wants it gone.
func TestGCS_DeleteMissingIsNoop(t *testing.T) {
	gcs := newBucket(t)
	if err := gcs.Delete(t.Context(), "player-data/none/ghost.dat"); err != nil {
		t.Errorf("Delete of missing object returned %v, want nil", err)
	}
}

func doRequest(t *testing.T, u usecasestorage.SignedURL, body []byte) []byte {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(u.Method, u.URL, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	for k, v := range u.Headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", u.Method, u.URL, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		t.Fatalf("%s returned %d: %s", u.Method, resp.StatusCode, data)
	}
	return data
}
